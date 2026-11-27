package lifecycle

import (
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// A hint names a command the tree has built, or ends in " (not built yet)" while
// the root registers no such command (the edit is refused either way; only the
// hint waits for it). A test of the root walks the hints and fails on one that
// names a command the root lacks, and on one that still says "not built" of a
// command it has. None says it now that bind is built.

// The commands of the two files no field belongs to.
const (
	bindingsCommand   = "gragas bind or gragas rebind"
	exclusionsCommand = "gragas rule exclude or gragas rule include"
)

// ruleField is one lifecycle field of a rule (S59): its name as the file
// spells it, its value, and the command that changes it.
type ruleField struct {
	name    string
	of      func(model.Rule) any
	command func(id string, before, after model.Rule) string
}

// ruleFields are a rule's lifecycle fields, S59's list: id and aliases,
// status, strength, scope and its exemptions, owner, review-by, zero-by,
// relations, origin, replaced-by, the sources list and the bindings. The
// statement, the reasoning and the pairs are prose, and free.
func ruleFields() []ruleField {
	edit := func(flag string) func(string, model.Rule, model.Rule) string {
		return func(id string, _, _ model.Rule) string {
			return strings.TrimSuffix("gragas rule edit "+id+" "+flag, " ")
		}
	}
	fixed := func(line string) func(string, model.Rule, model.Rule) string {
		return func(id string, _, _ model.Rule) string { return line + id }
	}
	return []ruleField{
		{"aliases", func(r model.Rule) any { return r.Aliases }, fixed("gragas rule renumber ")},
		{"status", func(r model.Rule) any { return r.Status }, statusCommand},
		{"strength", func(r model.Rule) any { return r.Strength }, func(id string, _, _ model.Rule) string { return "gragas rule replace " + id }},
		{"scope", func(r model.Rule) any { return r.Scope }, edit("--scope-part, --scope-path or --scope-kind")},
		{"exemptions", func(r model.Rule) any { return r.Exemptions }, edit("")},
		{"owner", func(r model.Rule) any { return r.Owner }, edit("--rule-owner")},
		{"review_by", func(r model.Rule) any { return r.ReviewBy }, edit("--review-by")},
		{"zero_by", func(r model.Rule) any { return r.ZeroBy }, edit("--zero-by")},
		{"relations", func(r model.Rule) any { return r.Relations }, edit("--relates, --conflicts, --depends or --covers")},
		{"origin", func(r model.Rule) any { return r.Origin }, func(string, model.Rule, model.Rule) string { return "" }},
		{"replaced_by", func(r model.Rule) any { return r.ReplacedBy }, func(id string, _, _ model.Rule) string { return "gragas rule replace " + id }},
		{"sources", func(r model.Rule) any { return r.Sources }, edit("--cite")},
		{"bindings", func(r model.Rule) any { return r.Bindings }, func(id string, _, _ model.Rule) string { return "gragas bind " + id }},
	}
}

// statusCommand is the command a status change is made with: retire to
// retired, restore from it, and promote up between the others. A lowering
// (active to trial or proposed, trial to proposed) has no command, so its hint
// is empty rather than a command that leaves the rule as it is.
func statusCommand(id string, before, after model.Rule) string {
	switch {
	case after.Status == model.Retired:
		return "gragas rule retire " + id
	case before.Status == model.Retired:
		return "gragas rule restore " + id
	case rank(after.Status) > rank(before.Status):
		return "gragas rule promote " + id
	}
	return ""
}

// rank orders the live statuses, lowest first.
func rank(s model.Status) int {
	switch s {
	case model.Trial:
		return 1
	case model.Active:
		return 2
	case model.Proposed, model.Retired:
		return 0
	}
	return 0
}

// newCommand is the hint for a rule that was not there.
const newCommand = "gragas rule new"

// probe is the id GuardCommands gives each hint, one no rule holds.
const probe = "<id>"

// GuardCommands is every hint the guard can give, as it gives it: each field's
// command for a rule with a probe id, each way a status changes, and the
// command of each whole file and of each entry list. A test of the root checks
// each against the commands the root registers.
func GuardCommands() []string {
	var out []string
	retired, live := model.Rule{Status: model.Retired}, model.Rule{Status: model.Active}
	for _, f := range ruleFields() {
		out = append(out, f.command(probe, live, live))
	}
	up := model.Rule{Status: model.Proposed}
	out = append(out, statusCommand(probe, live, retired), statusCommand(probe, retired, live), statusCommand(probe, up, live))
	out = append(out, bindingsCommand, exclusionsCommand, newCommand)
	out = append(out, entriesCommands(sourceEntries(), model.Source{}, model.Source{})...)
	return append(out, entriesCommands(termEntries(), model.Term{}, model.Term{Status: model.Retired})...)
}

// entriesCommands is each command a list of entries can name: the one that
// adds an entry, the one that takes it away, and each field's.
func entriesCommands[T any](e entries[T], before, after T) []string {
	out := []string{e.added(probe), e.gone(probe)}
	for _, f := range e.fields {
		out = append(out, f.command(probe, before, after))
	}
	return out
}

// GuardEdit is every lifecycle field that differs between before, the book as
// it stands, and after, the book with the proposed edit made: a rule's fields,
// a rule added or gone, the bindings/ records, the exclusions, the history,
// and a source's or a term's fields, or one added or gone. None means
// the edit touches only prose, or no rulebook file. show is a value as the
// file shows it, "" when empty or absent: two values differ when their shows
// do. The caller gives it, as the context imports no encoder.
func GuardEdit(before, after model.Book, show func(any) string) []model.Guarded {
	var out []model.Guarded
	for _, p := range guardedParts() {
		out = append(out, p.diff(before, after, show)...)
	}
	return out
}

// guardedPart is one part of the book the guard compares, by its field of
// model.Book, and the lifecycle changes a hand edit makes in it.
type guardedPart struct {
	field string
	diff  func(before, after model.Book, show func(any) string) []model.Guarded
}

// guardedParts are the parts of the book whose lifecycle fields only a
// command changes, each written with its history entry: the one list the
// guard holds. A part of model.Book is here or in UnguardedParts, with why.
func guardedParts() []guardedPart {
	return []guardedPart{
		{"Rules", func(b, a model.Book, show func(any) string) []model.Guarded {
			return guardRules(b.Rules, a.Rules, show)
		}},
		{"BindingRecords", func(b, a model.Book, show func(any) string) []model.Guarded {
			return guardWhole(show, "bindings/", "records", opaque(b.BindingRecords), opaque(a.BindingRecords), bindingsCommand)
		}},
		{"Exclusions", func(b, a model.Book, show func(any) string) []model.Guarded {
			return guardWhole(show, "exclusions.yaml", "entries", b.Exclusions, a.Exclusions, exclusionsCommand)
		}},
		{"History", func(b, a model.Book, show func(any) string) []model.Guarded {
			return guardWhole(show, "history.jsonl", "entries", b.History, a.History, "")
		}},
		{"Sources", func(b, a model.Book, show func(any) string) []model.Guarded {
			return guardEntries(sourceEntries(), b.Sources, a.Sources, show)
		}},
		{"Terms", func(b, a model.Book, show func(any) string) []model.Guarded {
			return guardEntries(termEntries(), b.Terms, a.Terms, show)
		}},
		{"Decisions", func(b, a model.Book, show func(any) string) []model.Guarded {
			return guardEntries(decisionEntries(), b.Decisions, a.Decisions, show)
		}},
	}
}

// GuardedParts are the fields of model.Book the guard compares, in its order.
func GuardedParts() []string {
	var out []string
	for _, p := range guardedParts() {
		out = append(out, p.field)
	}
	return out
}

// UnguardedParts are the fields of model.Book the guard leaves to a file
// edit, each with why. An edit of one that breaks the book still leaves a
// finding, and a finding is refused.
func UnguardedParts() map[string]string {
	return map[string]string{
		"Identity":     "rulebook.yaml: init writes it once and migrate its format; no command changes its name, version, owners or governance, so a hand edit is the only way",
		"Structure":    "structure.yaml: init writes it once and migrate its format; no command adds a layer or a kind yet",
		"Clashed":      "rules whose id another rule holds: the loader's finding, refused already",
		"Anchors":      "the guide's headings: prose (S59)",
		"Proposals":    "proposals/: propose is not built",
		"Translations": "translations/: prose (S59)",
	}
}

// entryField is one lifecycle field of an entry of a list keyed by slug: its
// name as the file spells it, its value, and the command that changes it,
// "" when none does.
type entryField[T any] struct {
	name    string
	of      func(T) any
	command func(slug string, before, after T) string
}

// entries is a list of the book keyed by slug: the entity a history entry
// names, an entry's slug and lifecycle fields, and the commands that add one
// and take one away.
type entries[T any] struct {
	entity      string
	slug        func(T) string
	fields      []entryField[T]
	added, gone func(slug string) string
}

// sourceEntries are sources.yaml's: a source's kind, title, locator and
// attribution change only through source new and source edit, which write the
// history entry. verified is the day someone last read it, set by
// hand, and no command writes it.
func sourceEntries() entries[model.Source] {
	edit := func(flag string) func(string, model.Source, model.Source) string {
		return func(slug string, _, _ model.Source) string { return "gragas source edit " + slug + " " + flag }
	}
	return entries[model.Source]{
		entity: "source",
		slug:   func(s model.Source) string { return s.Slug },
		fields: []entryField[model.Source]{
			{"kind", func(s model.Source) any { return s.Kind }, func(string, model.Source, model.Source) string { return "" }},
			{"title", func(s model.Source) any { return s.Title }, edit("--title")},
			{"locator", func(s model.Source) any { return s.Locator }, edit("--locator")},
			{"attribution", func(s model.Source) any { return s.Attribution }, edit("--attribution")},
		},
		added: func(string) string { return "gragas source new" },
		gone:  func(string) string { return "" },
	}
}

// decisionEntries are decisions.yaml's: a decision is added
// only by decision new or decision supersede, which write its history entry,
// and its chain and citations change only by supersede, which records a new
// decision and marks the old one. A decision is never edited or deleted
// (S59); its title, text, rejected options, who and date are prose, and
// rewording is a plain file edit.
func decisionEntries() entries[model.Decision] {
	supersede := func(slug string, _, _ model.Decision) string { return "gragas decision supersede " + slug }
	return entries[model.Decision]{
		entity: "decision",
		slug:   func(d model.Decision) string { return d.Slug },
		fields: []entryField[model.Decision]{
			{"supersedes", func(d model.Decision) any { return d.Supersedes }, func(_ string, _, after model.Decision) string {
				if after.Supersedes != "" {
					return "gragas decision supersede " + after.Supersedes
				}
				return ""
			}},
			{"superseded_by", func(d model.Decision) any { return d.SupersededBy }, supersede},
			{"cites", func(d model.Decision) any { return d.Cites }, supersede},
		},
		added: func(string) string { return "gragas decision new or gragas decision supersede" },
		// a superseded decision stays in the file
		gone: func(string) string { return "" },
	}
}

// termEntries are terms.yaml's: a term's literal and status change only
// through term edit and term retire. The definition is prose (S59).
func termEntries() entries[model.Term] {
	return entries[model.Term]{
		entity: "term",
		slug:   func(t model.Term) string { return t.Slug },
		fields: []entryField[model.Term]{
			{"literal", func(t model.Term) any { return t.Literal }, func(slug string, _, _ model.Term) string { return "gragas term edit " + slug + " --literal" }},
			{"status", func(t model.Term) any { return t.Status }, func(slug string, _, after model.Term) string {
				if after.Status == model.Retired {
					return "gragas term retire " + slug
				}
				return ""
			}},
		},
		added: func(string) string { return "gragas term new" },
		// a retired term stays in the file
		gone: func(slug string) string { return "gragas term retire " + slug },
	}
}

// guardEntries is every lifecycle change between the entries before and
// after, by slug: an entry added or gone, a field changed, and a slug held a
// number of times it was not held before, which the loader may let through
// and the guard does not.
func guardEntries[T any](e entries[T], before, after []T, show func(any) string) []model.Guarded {
	was, is := grouped(e.slug, before), grouped(e.slug, after)
	var out []model.Guarded
	for _, slug := range slugs(e.slug, after) {
		entity := e.entity + ":" + slug
		old, ok := was[slug]
		if !ok {
			out = append(out, model.Guarded{Entity: entity, Field: "slug", After: slug, Command: e.added(slug)})
			continue
		}
		now := is[slug]
		if len(old) != len(now) {
			out = append(out, model.Guarded{Entity: entity, Field: "slug", Before: count(len(old)), After: count(len(now)), Command: e.added(slug)})
		}
		for i := range min(len(old), len(now)) {
			out = append(out, guardFields(e, entity, slug, old[i], now[i], show)...)
		}
	}
	for _, slug := range slugs(e.slug, before) {
		if _, ok := is[slug]; !ok {
			out = append(out, model.Guarded{Entity: e.entity + ":" + slug, Field: "slug", Before: slug, Command: e.gone(slug)})
		}
	}
	return out
}

// guardFields is each lifecycle field of e that differs between two entries
// of one slug.
func guardFields[T any](e entries[T], entity, slug string, before, after T, show func(any) string) []model.Guarded {
	var out []model.Guarded
	for _, f := range e.fields {
		if b, a := show(f.of(before)), show(f.of(after)); b != a {
			out = append(out, model.Guarded{Entity: entity, Field: f.name, Before: b, After: a, Command: f.command(slug, before, after)})
		}
	}
	return out
}

// grouped is the entries of each slug, in their order.
func grouped[T any](slug func(T) string, in []T) map[string][]T {
	out := map[string][]T{}
	for _, x := range in {
		out[slug(x)] = append(out[slug(x)], x)
	}
	return out
}

// slugs is each slug of in once, in the order it first stands.
func slugs[T any](slug func(T) string, in []T) []string {
	var out []string
	seen := map[string]bool{}
	for _, x := range in {
		if s := slug(x); !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Values is every lifecycle value of b, by entity and field, as show shows
// it: each rule's id and each of its lifecycle fields, each exclusion by its
// key with every record written under it, each bindings/ record's digest by
// its path, and each source and term by its slug with each of its lifecycle
// fields.
// A value absent or empty is left out. Two books hold the same lifecycle data
// when their values are equal, key by key, whatever order their lists are
// in. A merge's three-way rule is held over these: what
// GuardEdit sums up, a rule added as its id or a list as its count, is here
// in full. history.jsonl is left out: its lines are held byte for byte.
func Values(b model.Book, show func(any) string) map[string]model.Guarded {
	out := map[string]model.Guarded{}
	put := func(entity, field, value string) {
		if value != "" {
			out[entity+"\x00"+field] = model.Guarded{Entity: entity, Field: field, After: value}
		}
	}
	for _, r := range b.Rules {
		id := string(r.ID)
		put("rule:"+id, "id", id)
		for _, f := range ruleFields() {
			put("rule:"+id, f.name, show(f.of(r)))
		}
	}
	for _, k := range exclusionsByKey(b.Exclusions) {
		var v string
		if len(k.records) == 1 {
			v = show(k.records[0])
		} else {
			v = show(k.records)
		}
		put("exclusions.yaml", "the exclusion "+k.key, v)
	}
	for _, r := range b.BindingRecords {
		put("bindings/", r.Path, string(r.Digest))
	}
	entryValues(sourceEntries(), b.Sources, show, put)
	entryValues(termEntries(), b.Terms, show, put)
	entryValues(decisionEntries(), b.Decisions, show, put)
	return out
}

// entryValues puts each entry of e by its slug: the slug, as itself or as
// the number of times it is held when that is more than once, and each
// lifecycle field, of every entry that holds the slug. A merge then holds a source's locator or a term's literal as it holds
// a rule's fields.
func entryValues[T any](e entries[T], in []T, show func(any) string, put func(entity, field, value string)) {
	by := grouped(e.slug, in)
	for _, slug := range slugs(e.slug, in) {
		entity := e.entity + ":" + slug
		held := by[slug]
		if len(held) == 1 {
			put(entity, "slug", slug)
		} else {
			put(entity, "slug", count(len(held)))
		}
		for _, f := range e.fields {
			vs := make([]string, len(held))
			for i, x := range held {
				vs[i] = show(f.of(x))
			}
			if len(vs) == 1 {
				put(entity, f.name, vs[0])
			} else {
				put(entity, f.name, show(vs))
			}
		}
	}
}

// keyed is the records of exclusions.yaml that share one key, in file order.
type keyed struct {
	key     string
	records []model.Exclusion
}

// exclusionsByKey groups the exclusions by key, overlay/rule/number. The
// loader keeps a key written twice and lists it as a finding (S4), and
// openExclusions reads every record of it, so a value holds every record
// under its key, never only the last: a record written above a lifted one
// is a change of that key's value.
func exclusionsByKey(es []model.Exclusion) []keyed {
	var out []keyed
	at := map[string]int{}
	for _, e := range es {
		key := fmt.Sprintf("%s/%s/%d", e.Overlay, e.Rule, e.Number)
		i, ok := at[key]
		if !ok {
			i = len(out)
			at[key] = i
			out = append(out, keyed{key: key})
		}
		out[i].records = append(out[i].records, e)
	}
	return out
}

func guardRules(before, after []model.Rule, show func(any) string) []model.Guarded {
	was := map[model.RuleID]model.Rule{}
	for _, r := range before {
		was[r.ID] = r
	}
	var out []model.Guarded
	seen := map[model.RuleID]bool{}
	for _, r := range after {
		id := string(r.ID)
		seen[r.ID] = true
		old, ok := was[r.ID]
		if !ok {
			out = append(out, model.Guarded{Entity: "rule:" + id, Field: "id", After: id, Command: newCommand})
			continue
		}
		for _, f := range ruleFields() {
			if b, a := show(f.of(old)), show(f.of(r)); b != a {
				out = append(out, model.Guarded{Entity: "rule:" + id, Field: f.name, Before: b, After: a, Command: f.command(id, old, r)})
			}
		}
	}
	for _, r := range before {
		if !seen[r.ID] {
			id := string(r.ID)
			out = append(out, model.Guarded{Entity: "rule:" + id, Field: "id", Before: id, Command: "gragas rule retire " + id})
		}
	}
	return out
}

// guardWhole is one finding for a list that differs at all: its entries are
// lifecycle data whole, written only by the commands that record a change.
func guardWhole[T any](show func(any) string, entity, field string, before, after []T, command string) []model.Guarded {
	b, a := show(before), show(after)
	if b == a {
		return nil
	}
	return []model.Guarded{{Entity: entity, Field: field, Before: count(len(before)), After: count(len(after)), Command: command}}
}

// opaque is the records' paths and digests, which is all of them that a
// change can touch.
func opaque(rs []model.Opaque) [][2]string {
	out := make([][2]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, [2]string{r.Path, string(r.Digest)})
	}
	slices.SortFunc(out, func(x, y [2]string) int {
		if x[0] < y[0] {
			return -1
		}
		if x[0] > y[0] {
			return 1
		}
		return 0
	})
	return out
}

func count(n int) string { return model.Plural(n, "entry") }
