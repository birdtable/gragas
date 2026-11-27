package format

import (
	"encoding/json"
	"fmt"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func loadFiles(t *testing.T, files map[string]string) Loaded {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	got, err := load(t, w, w.Seed("repo", files))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAnUnchangedOriginalLeavesItsTranslationsCurrent(t *testing.T) {
	t.Parallel()
	got := loadFiles(t, fullBook(t))
	if len(got.Findings) != 0 || len(got.Notices) != 1 || got.Notices[0].File != NamesNoticeFile {
		t.Fatalf("findings = %v, notices = %v; want only the names notice", got.Findings, got.Notices)
	}
}

func TestChangingAnAuthoritativeStatementMarksItsTranslationStaleAsANotice(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	files["language/go/rules/errors.yaml"] = strings.Replace(files["language/go/rules/errors.yaml"], "Wrap an error with what was being done.", "Wrap an error with the step that failed.", 1)
	got := loadFiles(t, files)
	if len(got.Findings) != 0 {
		t.Fatalf("a stale translation is a finding: %v", got.Findings)
	}
	var text []string
	for _, n := range got.Notices {
		if n.File != NamesNoticeFile {
			text = append(text, n.String())
		}
	}
	configtest.Golden(t, "stale.golden", []byte(strings.Join(text, "\n")+"\n"))
	if len(text) != 1 || !strings.Contains(text[0], "translations/fr/errors.yaml:3: the translation of rule:go.errors.1 statement into fr is stale") {
		t.Errorf("notices = %q", text)
	}
}

func TestATranslationOfNothingIsAFindingWithItsLine(t *testing.T) {
	t.Parallel()
	const h = "sha256:2eeaad265b9cd5c91e2783a32f74a5d2dc23b123c29cc116289d71c7d0a1d690"
	entry := func(entity, field, lang string) string {
		return fmt.Sprintf("  - entity: %s\n    field: %s\n    language: %s\n    text: x\n    hash: %s\n", entity, field, lang, h)
	}
	cases := []struct{ name, entries, want string }{
		{"a rule not in the book", entry("rule:go.nope.1", "statement", "fr"), "translations/fr/errors.yaml:3: the rule go.nope.1 is not in the book"},
		{"a field with none", entry("rule:go.errors.1", "owner", "fr"), "translations/fr/errors.yaml:3: rule has no field owner to translate"},
		{"an entity that is not kind:key", entry("go.errors.1", "statement", "fr"), "translations/fr/errors.yaml:3: entity go.errors.1 is not kind:key"},
		{"an unknown kind", entry("overlay:x", "statement", "fr"), "translations/fr/errors.yaml:3: the entity kind overlay is not rule"},
		{"a term not in the book", entry("term:nope", "definition", "fr"), "translations/fr/errors.yaml:3: the term nope is not in the book"},
		{"the same key twice", entry("rule:go.errors.1", "statement", "fr") + entry("rule:go.errors.1", "statement", "fr"), "translations/fr/errors.yaml:8: the translation of rule:go.errors.1 statement into fr is also at translations/fr/errors.yaml:3"},
		{"the authoritative language", entry("rule:go.errors.1", "statement", "en"), "translations/fr/errors.yaml:3: this is a translation into the authoritative language en"},
	}
	ran := 0
	for _, c := range cases {
		files := fullBook(t)
		files["translations/fr/errors.yaml"] = "format: 3\ntranslations:\n" + c.entries
		got := loadFiles(t, files)
		ran++
		hits := 0
		for _, f := range got.Findings {
			if strings.Contains(f.String(), c.want) {
				hits++
			}
		}
		if hits == 0 {
			t.Errorf("%s: findings = %v; want one holding %q", c.name, got.Findings, c.want)
		}
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d", ran, len(cases))
	}
}

func TestATranslationInTheFolderOfAnotherLanguageIsAFinding(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	files["translations/de/errors.yaml"] = files["translations/fr/errors.yaml"]
	got := loadFiles(t, files)
	if len(got.Findings) == 0 || !strings.Contains(got.Findings[0].Msg, "language is fr and the file is in the folder of de") {
		t.Errorf("findings = %v", got.Findings)
	}
}

func TestATextInALanguageWithNoTranslationFallsBackWithAMark(t *testing.T) {
	t.Parallel()
	got := loadFiles(t, fullBook(t))
	texts := NewTexts(got.Book)
	fr, ok := texts.In("rule:go.errors.1", "statement", "fr")
	if !ok || fr.Fallback || fr.Stale || fr.Language != "fr" || !strings.HasPrefix(fr.Text, "Enveloppez") {
		t.Errorf("fr = %+v, %v", fr, ok)
	}
	de, ok := texts.In("rule:go.errors.1", "statement", "de")
	if !ok || !de.Fallback || de.Language != "en" || de.Text != "Wrap an error with what was being done." {
		t.Errorf("de = %+v, %v", de, ok)
	}
	if _, ok := texts.In("rule:go.nope.1", "statement", "fr"); ok {
		t.Error("a missing entity has a text")
	}
	if en, _ := texts.In("rule:go.errors.1", "statement", "en"); en.Fallback {
		t.Error("the authoritative language is not a fallback")
	}
}

// TestStalenessFollowsEdits holds the property over many edits: a translation
// is stale exactly when the hash it records is not the hash of the text now.
func TestStalenessFollowsEdits(t *testing.T) {
	t.Parallel()
	original := "Wrap an error with what was being done."
	recorded := TextHash(original)
	seen := map[model.Digest]bool{}
	checked := 0
	for i := range 200 {
		text := original
		if i%2 == 1 {
			text = fmt.Sprintf("%s Edit %d.", strings.TrimSuffix(original, "."), i)
		}
		texts := NewTexts(model.Book{
			Identity: model.RulebookIdentity{Language: "en"},
			Rules:    []model.Rule{{ID: "go.errors.1", Statement: text}},
			Translations: []model.Translation{
				{Entity: "rule:go.errors.1", Field: "statement", Language: "fr", Text: "x", Hash: recorded},
			},
		})
		got, ok := texts.In("rule:go.errors.1", "statement", "fr")
		if !ok || got.Stale != (text != original) {
			t.Fatalf("edit %d: stale = %v for %q", i, got.Stale, text)
		}
		seen[TextHash(text)] = true
		checked++
	}
	if checked != 200 || len(seen) != 101 {
		t.Errorf("checked %d edits with %d distinct hashes; want 200 and 101", checked, len(seen))
	}
}

func TestTheHashIsOfTheTextAsAJSONString(t *testing.T) {
	t.Parallel()
	// the sha256 of the bytes "a" in quotes, taken with shasum -a 256
	const want = "sha256:ac8d8342bbb2362d13f0a559a3621bb407011368895164b628a54f7fc33fc43c"
	if got := TextHash("a"); got != want {
		t.Errorf("TextHash(a) = %s; want %s", got, want)
	}
}

func FuzzTextHash(f *testing.F) {
	for _, s := range []string{"", "Wrap the error.", "é\u0000\"\\", "\xff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		a, b := TextHash(s), TextHash(s)
		if a != b || !strings.HasPrefix(string(a), "sha256:") || len(a) != 71 {
			t.Fatalf("TextHash(%q) = %q, %q", s, a, b)
		}
	})
}

// TestEveryTranslatableFieldIsAStringInTheSchema holds the table to the schemas:
// each field it names is at its place in the entity's schema and is a string.
func TestEveryTranslatableFieldIsAStringInTheSchema(t *testing.T) {
	t.Parallel()
	const dir = "../../host/assets/format/schema/v3/"
	files := map[string]string{"rule": "rule", "term": "terms", "decision": "decisions", "source": "sources"}
	roots := map[string]string{"rule": "", "term": "terms", "decision": "decisions", "source": "sources"}
	read := func(name string) map[string]any {
		t.Helper()
		b, err := os.ReadFile(dir + name + ".schema.json")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	// deref follows a $ref to its schema, in the file it names or the one it is in.
	var deref func(node map[string]any, file string) (map[string]any, string)
	deref = func(node map[string]any, file string) (map[string]any, string) {
		ref, ok := node["$ref"].(string)
		if !ok {
			return node, file
		}
		name, ptr, _ := strings.Cut(ref, "#")
		if name != "" {
			file = strings.TrimSuffix(name, ".schema.json")
		}
		cur := any(read(file))
		for seg := range strings.SplitSeq(strings.TrimPrefix(ptr, "/"), "/") {
			if seg == "" {
				continue
			}
			cur = cur.(map[string]any)[seg]
		}
		return deref(cur.(map[string]any), file)
	}
	prop := func(node map[string]any, name string) map[string]any {
		t.Helper()
		props, _ := node["properties"].(map[string]any)
		next, ok := props[name].(map[string]any)
		if !ok {
			t.Fatalf("the schema has no property %s", name)
		}
		return next
	}
	step := func(node map[string]any, file, seg string) (map[string]any, string) {
		node, file = deref(node, file)
		if name, isList := strings.CutSuffix(seg, "[]"); isList {
			next, nf := deref(prop(node, name), file)
			items, _ := next["items"].(map[string]any)
			return deref(items, nf)
		}
		return deref(prop(node, seg), file)
	}
	checked := 0
	for kind, fields := range Translatable() {
		for _, field := range fields {
			node, file := read(files[kind]), files[kind]
			if r := roots[kind]; r != "" {
				node, file = step(node, file, r+"[]")
			}
			for seg := range strings.SplitSeq(field, ".") {
				node, file = step(node, file, seg)
			}
			if node["type"] != "string" {
				t.Errorf("%s %s is %v in the schema; want a string", kind, field, node["type"])
			}
			checked++
		}
	}
	if checked != 11 {
		t.Errorf("checked %d fields; the table names 11", checked)
	}
}

func TestEveryTranslatableFieldOfARuleCanBeTranslatedAndGoesStale(t *testing.T) {
	t.Parallel()
	r := model.Rule{ID: "go.errors.1", Statement: "S",
		Reasoning:  &model.Reasoning{Definition: "D", Pros: []string{"P0", "P1"}, Cons: []string{"C0"}, Decision: "wrap"},
		Bindings:   []model.Binding{{Remedy: "fix it"}, {Remedy: "again"}},
		Exemptions: []model.Exemption{{Reason: "fixtures"}}}
	texts := NewTexts(model.Book{Identity: model.RulebookIdentity{Language: "en"}, Rules: []model.Rule{r}})
	want := map[string]string{"statement": "S", "reasoning.definition": "D", "reasoning.pros[1]": "P1", "reasoning.cons[0]": "C0",
		"reasoning.decision": "wrap", "bindings[1].remedy": "again", "exemptions[0].reason": "fixtures"}
	for field, text := range want {
		got, ok := texts.In("rule:go.errors.1", field, "en")
		if !ok || got.Text != text {
			t.Errorf("%s = %+v, %v; want %q", field, got, ok, text)
		}
	}
	for _, field := range []string{"reasoning.pros[2]", "bindings[2].remedy", "owner", "reasoning", "scope.paths[0]"} {
		if _, ok := texts.In("rule:go.errors.1", field, "en"); ok {
			t.Errorf("%s has a text", field)
		}
	}
	if len(want) != 7 {
		t.Fatal("the loop did not cover every field")
	}
}
