package format

import (
	"errors"
	"fmt"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"slices"
	"strings"
	"testing"

	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// standInNames is a names list built at run time from a stand-in word
// (tests.doubles.7): no real forbidden name is in the repository.
func standInNames(t *testing.T) *Names {
	t.Helper()
	n, err := ParseNames([]byte("# made at run time\n\\balex\\b\n"))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// loadStatement loads the fixture with one rule whose statement is given, the
// terms of the fixture and the names list.
func loadStatement(t *testing.T, statement string, names *Names, budget int) Loaded {
	t.Helper()
	return loadStatementWith(t, statement, names, budget, nil)
}

// loadStatementWith is loadStatement after edit has changed the files.
func loadStatementWith(t *testing.T, statement string, names *Names, budget int, edit func(map[string]string)) Loaded {
	t.Helper()
	files := fullBook(t)
	files["language/go/rules/errors.yaml"] = "format: 3\npart: go\nconcern: errors\nrules:\n  - id: go.errors.1\n    statement: " + statement +
		"\n    strength: must\n    status: active\n    review_by: \"2027-01-31\"\n    origin: authored\n"
	files["language/go/rules/layout.yaml"] = "format: 3\npart: go\nconcern: layout\nrules:\n  - id: go.layout.1\n    statement: Keep a folder to one subject.\n    strength: must\n    status: active\n    review_by: \"2027-01-31\"\n    origin: authored\n"
	delete(files, "exclusions.yaml")
	delete(files, "translations/fr/errors.yaml")
	delete(files, "decisions.yaml")
	if edit != nil {
		edit(files)
	}
	if budget > 0 {
		files[IdentityFile] = strings.Replace(files[IdentityFile], "statement_words: 30", fmt.Sprintf("statement_words: %d", budget), 1)
	}
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	p := w.Ports()
	got, err := LoadWith(p.Tree, p.Schema, w.Seed("repo", files), Options{Names: names})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestThePlantedProseViolationsAreEachOneFindingWithALine(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, statement, want string }{
		{"clean control", "Wrap an error with what was being done.", ""},
		{"a term with a literal passes", "Run {gate} before a push, and call make check by name.", ""},
		{"two sentences", "Wrap the error. Return it after.", "the statement is 2 sentences"},
		{"passive", "Make sure each error is wrapped with context.", "the statement is passive"},
		{"not the imperative", "Every error wraps its cause.", "which is not the imperative"},
		{"a description", "Returns the error with context.", "which is not the imperative"},
		{"we", "Wrap the error so we can find it.", "says we or our"},
		{"a bare tool name outside the rulebook's terms", "Ask alex before you wrap an error.", NamesMsg},
		{"an undefined term", "Run {lint} before a push.", "the term {lint} is not defined"},
		{"over the budget", "Wrap the error with what was being done and say which step of the work failed.", "the statement is 16 words and the budget is 12"},
	}
	var listing strings.Builder
	ran := 0
	for _, c := range cases {
		got := loadStatement(t, c.statement, standInNames(t), 12)
		ran++
		listing.WriteString("## " + c.name + "\n")
		for _, f := range got.Findings {
			listing.WriteString(f.String() + "\n")
		}
		if c.want == "" {
			if len(got.Findings) != 0 {
				t.Errorf("%s: findings = %v", c.name, got.Findings)
			}
			continue
		}
		if len(got.Findings) != 1 || got.Findings[0].Line != 6 || !strings.Contains(got.Findings[0].Msg, c.want) || got.Findings[0].Fix == "" {
			t.Errorf("%s: findings = %v; want one at line 6 holding %q with a fix", c.name, got.Findings, c.want)
		}
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d", ran, len(cases))
	}
	configtest.Golden(t, "prose.golden", []byte(listing.String()))
}

func TestWithNoNamesListTheCheckIsANoticeAndNotAFinding(t *testing.T) {
	t.Parallel()
	got := loadStatement(t, "Ask alex before you wrap an error.", nil, 0)
	if len(got.Findings) != 0 || len(got.Notices) != 1 || got.Notices[0].File != NamesNoticeFile {
		t.Errorf("findings = %v, notices = %v", got.Findings, got.Notices)
	}
	if got := loadStatement(t, "Wrap the error.", standInNames(t), 0); len(got.Notices) != 0 {
		t.Errorf("a run with a list noticed %v", got.Notices)
	}
}

func TestAPatternThatDoesNotCompileIsABadNamesErrorAtItsLine(t *testing.T) {
	t.Parallel()
	_, err := ParseNames([]byte("fine\n\n(unclosed\n"))
	bad, ok := errors.AsType[*BadNames](err)
	if !ok || bad.Line != 3 {
		t.Fatalf("err = %v; want *BadNames at line 3", err)
	}
}

func TestTheNamesCheckReadsTheReasoningToo(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	files["language/go/rules/errors.yaml"] = strings.Replace(files["language/go/rules/errors.yaml"], "definition: An error says what failed and what was being done.", "definition: alex asked for it.", 1)
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	p := w.Ports()
	got, err := LoadWith(p.Tree, p.Schema, w.Seed("repo", files), Options{Names: standInNames(t)})
	if err != nil || len(got.Findings) != 1 || got.Findings[0].Msg != NamesMsg || got.Findings[0].Line != 18 {
		t.Fatalf("Load = %v, %v", got.Findings, err)
	}
}

func FuzzParseNames(f *testing.F) {
	for _, s := range []struct{ file, text string }{
		{"# made-up names\n\nacme\n\\bwidgetco\\b\n", "made by Acme"},
		{"(unclosed\n", "anything"},
		{"\n\n# only comments\n", "acme"},
		{"a{2,3}\n\x00\n", "aaaa"},
		{"4135aa9dc1b842a653dea846903ddb95bfb8c5a10c504a7fa16e10bc31d1fdf0\n", "Alex was here"},
	} {
		f.Add([]byte(s.file), s.text)
	}
	f.Fuzz(func(t *testing.T, file []byte, text string) {
		n, err := ParseNames(file)
		if err != nil {
			if _, ok := errors.AsType[*BadNames](err); !ok {
				t.Fatalf("err = %T; want *BadNames", err)
			}
			return
		}
		if m := n.Find(text); m != "" && m != MaskedName && !strings.Contains(strings.ToLower(text), strings.ToLower(m)) {
			t.Fatalf("Find(%q) = %q, which is not in the text", text, m)
		}
	})
}

func FuzzLintStatement(f *testing.F) {
	for _, s := range []string{"Wrap the error.", "Errors must be wrapped. We do.", "{x} and {", "", "...", "`code` runs"} {
		f.Add(s, 5)
	}
	f.Fuzz(func(t *testing.T, text string, budget int) {
		for _, i := range lintStatement(text, budget) {
			if i.msg == "" || i.fix == "" {
				t.Fatalf("an issue with no message or no fix for %q", text)
			}
		}
		_ = withoutTerms(text, []string{"x"})
		_ = undefinedTerms(text, map[string]bool{})
	})
}

// TestATermWhoseLiteralIsTheNameLetsTheBareNamePass plants the stand-in name
// as the literal of a term: the same text is one finding when the term is not
// defined and none when it is, so the allowance is what lets it pass. A literal
// inside a longer word hides nothing.
func TestATermWhoseLiteralIsTheNameLetsTheBareNamePass(t *testing.T) {
	t.Parallel()
	const text = "Ask alex before you wrap an error."
	if got := loadStatement(t, text, standInNames(t), 12); len(got.Findings) != 1 {
		t.Fatalf("without the term: findings = %v, want one", got.Findings)
	}
	define := func(files map[string]string) {
		files["terms.yaml"] += "  - slug: person\n    definition: The stand-in.\n    literal: alex\n"
	}
	if got := loadStatementWith(t, text, standInNames(t), 12, define); len(got.Findings) != 0 {
		t.Errorf("with the term: findings = %v, want none", got.Findings)
	}
	inside := func(files map[string]string) {
		files["terms.yaml"] += "  - slug: part\n    definition: A piece.\n    literal: le\n"
	}
	if got := loadStatementWith(t, text, standInNames(t), 12, inside); len(got.Findings) != 1 {
		t.Errorf("a literal inside a longer word: findings = %v, want one", got.Findings)
	}
}

// TestARetiredTermAllowsNeitherItsSlugNorItsLiteral: the same text that the
// defined term lets pass is three findings when the term is retired, the bare
// name the names list refuses, the term in braces that no longer defines
// anything, and the term's own literal line in terms.yaml, which
// the names check reads like every line and nothing allows once
// the term is retired. The names list is the stand-in made in the test.
// Planted failure: a retired term still feeds the names check or the lint.
func TestARetiredTermAllowsNeitherItsSlugNorItsLiteral(t *testing.T) {
	t.Parallel()
	const text = "Ask alex and {person} before you wrap an error."
	define := func(status string) func(map[string]string) {
		return func(files map[string]string) {
			files["terms.yaml"] += "  - slug: person\n    definition: The stand-in.\n    literal: alex\n" + status
		}
	}
	if got := loadStatementWith(t, text, standInNames(t), 12, define("")); len(got.Findings) != 0 {
		t.Fatalf("with the term defined: findings = %v, want none", got.Findings)
	}
	got := loadStatementWith(t, text, standInNames(t), 12, define("    status: retired\n"))
	var names []string
	for _, f := range got.Findings {
		if f.Msg == NamesMsg {
			names = append(names, fmt.Sprintf("%s:%d", f.File, f.Line))
		}
	}
	if len(got.Findings) != 3 || !slices.Equal(names, []string{"language/go/rules/errors.yaml:6", "terms.yaml:8"}) {
		t.Errorf("with the term retired: findings = %v, want three, the names ones at the statement and the literal", got.Findings)
	}
}
