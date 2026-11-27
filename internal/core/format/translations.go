package format

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// TranslationsDir holds the translations, one folder a language:
// translations/<language>/<name>.yaml.
const (
	TranslationsDir    = "translations"
	translationsSchema = "v3/translations.schema.json"
)

// Translatable is the one table of the fields a translation may name, by the
// kind of entity. A path with [] is a list: the entry names one item, as in
// reasoning.pros[2]. Every field is free-text prose of the anatomy and a string
// in its schema; a test holds both.
func Translatable() map[string][]string {
	return map[string][]string{
		"rule":     {"statement", "reasoning.definition", "reasoning.pros[]", "reasoning.cons[]", "reasoning.decision", "bindings[].remedy", "exemptions[].reason"},
		"term":     {"definition"},
		"decision": {"title", "text"},
		"source":   {"title"},
	}
}

var fieldPath = regexp.MustCompile(`^([a-z_.]+)(?:\[(\d+)\](?:\.([a-z_]+))?)?$`)

// ruleText is the text of a translatable field of a rule, or the reason there
// is none.
func ruleText(r model.Rule, field string) (string, string) {
	m := fieldPath.FindStringSubmatch(field)
	if m == nil {
		return "", "rule has no field " + field + " to translate"
	}
	shape := m[1]
	idx := -1
	if m[2] != "" {
		shape += "[]"
		idx = 0
		for _, d := range m[2] {
			idx = idx*10 + int(d-'0') // the pattern holds digits only
			if idx > 1<<20 {
				return "", "rule has no field " + field + " to translate"
			}
		}
	}
	if m[3] != "" {
		shape += "." + m[3]
	}
	if !slices.Contains(Translatable()["rule"], shape) {
		return "", "rule has no field " + field + " to translate"
	}
	var text string
	var have bool
	re := r.Reasoning
	pick := func(l []string) {
		if idx < len(l) {
			text, have = l[idx], true
		}
	}
	switch {
	case shape == "statement":
		text, have = r.Statement, true
	case re != nil && shape == "reasoning.definition":
		text, have = re.Definition, re.Definition != ""
	case re != nil && shape == "reasoning.decision":
		text, have = re.Decision, re.Decision != ""
	case re != nil && shape == "reasoning.pros[]":
		pick(re.Pros)
	case re != nil && shape == "reasoning.cons[]":
		pick(re.Cons)
	case shape == "exemptions[].reason" && idx < len(r.Exemptions):
		text, have = r.Exemptions[idx].Reason, r.Exemptions[idx].Reason != ""
	case shape == "bindings[].remedy" && idx < len(r.Bindings):
		text, have = r.Bindings[idx].Remedy, r.Bindings[idx].Remedy != ""
	}
	if !have {
		return "", "the rule " + string(r.ID) + " has no " + field
	}
	return text, ""
}

// TextHash is the hash a translation records of the authoritative text it
// translated: the canon digest of the text as a string value, never of the
// file it stands in.
func TextHash(text string) model.Digest {
	d, err := canon.DigestOf(canon.String(strings.ToValidUTF8(text, "\uFFFD")))
	if err != nil {
		return "" // a Go string is always valid text to canon; unreachable
	}
	return model.Digest(d.String())
}

// transEntry is one translation with the place it is written.
type transEntry struct {
	model.Translation
	file string
	line int
}

// translations reads each translations/<language>/<name>.yaml. A file with a
// problem adds findings and the rest are still read.
func (r reader) translations() error {
	entries, err := r.tree.List(r.root, join(r.dir, TranslationsDir), model.Every)
	if err != nil {
		return err
	}
	base := join(r.dir, TranslationsDir) + "/"
	for _, e := range entries {
		rel := strings.TrimPrefix(e.Path, base)
		lang, name, nested := strings.Cut(rel, "/")
		if e.Dir || !nested || path.Ext(name) != ".yaml" || strings.Contains(name, "/") {
			if !e.Dir {
				r.add(e.Path, 0, "not a translation file; keep them as translations/<language>/<name>.yaml")
			}
			continue
		}
		doc, bad, err := readFile(r.tree, r.schema, r.root, e.Path, translationsSchema, migrations())
		if err != nil {
			return err
		}
		r.out.Findings = append(r.out.Findings, bad...)
		if doc == nil {
			continue
		}
		for _, m := range ordered.Maps(doc.List("translations")) {
			t := model.Translation{
				Entity: m.Str("entity"), Field: m.Str("field"), Language: m.Str("language"),
				Text: m.Str("text"), Hash: model.Digest(m.Str("hash")),
			}
			if t.Language != lang {
				r.add(e.Path, m.KeyLine("language"), fmt.Sprintf("language is %s and the file is in the folder of %s", t.Language, lang))
			}
			*r.trans = append(*r.trans, transEntry{Translation: t, file: e.Path, line: m.Line})
			r.out.Book.Translations = append(r.out.Book.Translations, t)
		}
	}
	return nil
}

// checkTranslations holds the translations to the book they translate: the
// entity and field must exist, a key (entity, field, language) is written
// once (A65), and a translation whose recorded hash is not the hash of the
// text now is stale, which is a notice and never a finding.
func (r reader) checkTranslations() {
	texts := NewTexts(r.out.Book)
	type key struct{ entity, field, language string }
	first := map[key]transEntry{}
	for _, t := range *r.trans {
		k := key{t.Entity, t.Field, t.Language}
		if at, dup := first[k]; dup {
			r.add(t.file, t.line, fmt.Sprintf("the translation of %s %s into %s is also at %s:%d; write each once", t.Entity, t.Field, t.Language, at.file, at.line))
			continue
		}
		first[k] = t
		if t.Language == r.out.Book.Identity.Language {
			r.add(t.file, t.line, "this is a translation into the authoritative language "+t.Language+"; the authoritative text is the rule's own")
			continue
		}
		now, why := texts.authoritative(t.Entity, t.Field)
		if why != "" {
			r.add(t.file, t.line, why)
			continue
		}
		if t.Hash != TextHash(now) {
			r.out.Notices = append(r.out.Notices, Finding{File: t.file, Line: t.line,
				Msg: fmt.Sprintf("the translation of %s %s into %s is stale: the %s text changed after it was translated", t.Entity, t.Field, t.Language, r.out.Book.Identity.Language),
				Fix: "translate the new text and record its hash"})
		}
	}
}

// Texts answers which text a reader in a language sees.
type Texts struct {
	rules map[model.RuleID]model.Rule
	terms map[string]model.Term
	decs  map[string]model.Decision
	srcs  map[string]model.Source
	table *Table
	lang  string
	trans map[[3]string]model.Translation
}

// Text is the text for a language: Fallback is set when it is the
// authoritative language's text standing in for a translation that is not
// there, so a rendering can mark it. Stale is set when the translation is there
// and the original changed since.
type Text struct {
	Text     string
	Language string
	Fallback bool
	Stale    bool
}

// NewTexts indexes the book's entities and translations.
func NewTexts(b model.Book) *Texts {
	t := &Texts{rules: map[model.RuleID]model.Rule{}, terms: map[string]model.Term{}, decs: map[string]model.Decision{},
		srcs: map[string]model.Source{}, table: NewTable(b.Rules), lang: b.Identity.Language, trans: map[[3]string]model.Translation{}}
	for _, x := range b.Terms {
		t.terms[x.Slug] = x
	}
	for _, x := range b.Decisions {
		t.decs[x.Slug] = x
	}
	for _, x := range b.Sources {
		t.srcs[x.Slug] = x
	}
	for _, x := range b.Translations {
		k := [3]string{x.Entity, x.Field, x.Language}
		if _, dup := t.trans[k]; !dup {
			t.trans[k] = x
		}
	}
	return t
}

// In is the text of an entity's field in a language, or the authoritative
// text marked as a fallback. ok is false when the entity or field does not
// exist.
func (t *Texts) In(entity, field, language string) (Text, bool) {
	now, why := t.authoritative(entity, field)
	if why != "" {
		return Text{}, false
	}
	if tr, has := t.trans[[3]string{entity, field, language}]; has && language != t.lang {
		return Text{Text: tr.Text, Language: language, Stale: tr.Hash != TextHash(now)}, true
	}
	return Text{Text: now, Language: t.lang, Fallback: language != t.lang}, true
}

// authoritative is the text of the field now, or the reason there is none.
func (t *Texts) authoritative(entity, field string) (string, string) {
	kind, key, ok := strings.Cut(entity, ":")
	if !ok || key == "" {
		return "", "entity " + entity + " is not kind:key, such as rule:the-id"
	}
	var fields map[string]string
	switch kind {
	case "rule":
		r, found := t.table.Resolve(model.RuleID(key))
		if !found {
			return "", "the rule " + key + " is not in the book"
		}
		return ruleText(r, field)
	case "term":
		x, found := t.terms[key]
		fields, ok = map[string]string{"definition": x.Definition}, found
	case "decision":
		x, found := t.decs[key]
		fields, ok = map[string]string{"title": x.Title, "text": x.Text}, found
	case "source":
		x, found := t.srcs[key]
		fields, ok = map[string]string{"title": x.Title}, found
	default:
		return "", "the entity kind " + kind + " is not rule, term, decision or source"
	}
	if !ok {
		return "", fmt.Sprintf("the %s %s is not in the book", kind, key)
	}
	text, has := fields[field]
	if !has || !slices.Contains(Translatable()[kind], field) {
		return "", fmt.Sprintf("%s has no field %s to translate", kind, field)
	}
	return text, ""
}
