package format

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/digest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// plant is the stand-in name standInNames refuses, which no file of the
// fixture holds.
const plant = "alex"

// lineHolding is the line of data that holds want, from 1, or 0 when none does.
func lineHolding(data, want string) int {
	for i, l := range strings.Split(data, "\n") {
		if strings.Contains(l, want) {
			return i + 1
		}
	}
	return 0
}

// TestTheNamesCheckReadsEveryFileTheLoadReads plants the stand-in name in
// each kind of place a rulebook holds words: a rule's statement, a field the
// old check never read (the owner, a binding's remedy, a source's title, a
// decision's title), a comment, a translation, and a file's path. Each is one
// names finding at its line, or at the file for a path, and no finding says
// the name. The clean control finds nothing, so the list is what finds each.
// Planted failure: the check reads only the fields a reader picked out, or a
// finding quotes what it matched.
func TestTheNamesCheckReadsEveryFileTheLoadReads(t *testing.T) {
	t.Parallel()
	edit := func(file, old, new string) func(map[string]string) (string, int) {
		return func(files map[string]string) (string, int) {
			if !strings.Contains(files[file], old) {
				t.Fatalf("the fixture's %s has no %q", file, old)
			}
			files[file] = strings.Replace(files[file], old, new, 1)
			return file, lineHolding(files[file], plant)
		}
	}
	const rules = "language/go/rules/errors.yaml"
	cases := []struct {
		name  string
		plant func(map[string]string) (string, int)
	}{
		{"a statement", edit(rules, "statement: Wrap an error with", "statement: Ask "+plant+", then wrap an error with")},
		{"an owner", edit(rules, "owner: team-a", "owner: "+plant)},
		{"a binding's remedy", edit(rules, "remedy: gofmt", "remedy: ask "+plant)},
		{"a source's title", edit("sources.yaml", "title: Effective Go", "title: Effective Go by "+plant)},
		{"a decision's title", edit(DecisionsFile, "title: Wrap every error", "title: Wrap every error, as "+plant+" asked")},
		// A flow mapping is in braces, as a {slug} is: only a defined
		// term's reference is blanked, so a name inside one is read.
		{"a flow mapping", edit("sources.yaml", "sources:", "sources:\n  - {slug: flow-note, kind: guide, title: Notes by "+plant+", locator: flow-note}")},
		{"an undefined reference", edit(rules, "statement: Wrap an error with", "statement: Ask {"+plant+"}, then wrap an error with")},
		{"a comment", edit("terms.yaml", "terms:", "# kept by "+plant+"\nterms:")},
		{"a translation", edit("translations/fr/errors.yaml", "format: 3", "# relu par "+plant+"\nformat: 3")},
		{"a path", func(files map[string]string) (string, int) {
			const from, to = "proposals/tighten-wrap.yaml", "proposals/tighten-" + plant + ".yaml"
			files[to] = files[from]
			delete(files, from)
			return to, 0
		}},
	}
	clean := loadNamed(t, fullBook(t))
	if len(clean.Findings) != 0 {
		t.Fatalf("the clean control: findings = %v", clean.Findings)
	}
	for _, c := range cases {
		files := fullBook(t)
		file, line := c.plant(files)
		got := loadNamed(t, files)
		if line == 0 {
			// A renamed file is a finding of its own too, which says the
			// path: coreapi masks every path while a list is held
			// (TestEveryLineOfTheReportIsMaskedWhileAListIsHeld).
			want := Finding{File: file, Msg: NamesPathMsg, Fix: "rename the file, or the folder that holds it", Names: true}
			if !slices.Contains(got.Findings, want) {
				t.Errorf("%s: findings = %v; want %v among them", c.name, got.Findings, want)
			}
			continue
		}
		want := Finding{File: file, Line: line, Msg: NamesMsg, Fix: namesFix, Names: true}
		if c.name == "an undefined reference" {
			// The undefined-term check's own finding quotes the reference,
			// as a renamed file's does its path; coreapi masks it.
			if !slices.Contains(got.Findings, want) {
				t.Errorf("%s: findings = %v; want %v among them", c.name, got.Findings, want)
			}
			continue
		}
		if !slices.Equal(got.Findings, []Finding{want}) {
			t.Errorf("%s: findings = %v; want %v alone", c.name, got.Findings, want)
		}
		for _, f := range got.Findings {
			if strings.Contains(f.Msg+f.Fix, plant) {
				t.Errorf("%s: a finding says the name: %q", c.name, f.Msg+" "+f.Fix)
			}
		}
	}
}

// loadNamed loads files with the stand-in list and fails on an error.
func loadNamed(t *testing.T, files map[string]string) Loaded {
	t.Helper()
	return loadWithNames(t, files, standInNames(t))
}

// loadWithNames loads files with the list n and fails on an error.
func loadWithNames(t *testing.T, files map[string]string, n *Names) Loaded {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	p := w.Ports()
	got, err := LoadWith(p.Tree, p.Schema, w.Seed("repo", files), Options{Names: n})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestANameAValueHoldsAndNoLineDoesIsFound plants a two-word stand-in name
// where the load's value holds it and no one line does: folded over two lines
// of a statement and of a source's title, and in a quoted statement with an
// escape for its first letter. Each is one finding at the key's line, said in
// the value's words, for a list of the pattern and for one of the hash. The
// control, the name on one line, is the line's finding; a folded value whose
// one line holds the name is that line's finding alone, not two. Planted
// failure: the check reads lines only, as it once did, and the split and the
// escape pass.
func TestANameAValueHoldsAndNoLineDoesIsFound(t *testing.T) {
	t.Parallel()
	first, second := "quo"+"r", "ve"+"x" // the stand-in, built at run time
	lists := map[string]string{
		"a pattern": `\b` + first + `\s+` + second + `\b`,
		"a hash":    digest.Hex([]byte(first + " " + second)),
	}
	const rules = "language/go/rules/errors.yaml"
	const statement = "    statement: Wrap an error with what was being done."
	cases := []struct {
		name, file, old, new string
		line                 int
		msg                  string
	}{
		{"the control: one line", rules, statement, "    statement: Ask " + first + " " + second + ", then wrap an error.", 7, NamesMsg},
		{"a statement folded over two lines", rules, statement, "    statement: >-\n      Ask " + first + "\n      " + second + ", then wrap an error.", 7, NamesValueMsg},
		{"a source's title folded over two lines", "sources.yaml", "title: Effective Go", "title: >-\n      Effective Go by " + first + "\n      " + second, 0, NamesValueMsg},
		{"a statement with an escape", rules, statement, `    statement: "Ask \x` + "71" + first[1:] + " " + second + `, then wrap an error."`, 7, NamesValueMsg},
		{"a folded value whose one line holds it", rules, statement, "    statement: >-\n      Wrap an error,\n      as " + first + " " + second + " asked.", 9, NamesMsg},
	}
	for listName, list := range lists {
		n, err := ParseNames([]byte(list + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		if got := loadWithNames(t, fullBook(t), n); len(got.Findings) != 0 {
			t.Fatalf("%s: the clean control: findings = %v", listName, got.Findings)
		}
		for _, c := range cases {
			files := fullBook(t)
			if !strings.Contains(files[c.file], c.old) {
				t.Fatalf("the fixture's %s has no %q", c.file, c.old)
			}
			files[c.file] = strings.Replace(files[c.file], c.old, c.new, 1)
			line := c.line
			if line == 0 {
				line = lineHolding(files[c.file], "title: >-")
			}
			want := []Finding{{File: c.file, Line: line, Msg: c.msg, Fix: namesFix, Names: true}}
			if got := loadWithNames(t, files, n); !slices.Equal(got.Findings, want) {
				t.Errorf("%s, %s: findings = %v; want %v alone", listName, c.name, got.Findings, want)
			}
		}
	}
}

// TestANameAGuideBlockOrAHistoryLineHoldsInAValueIsFound plants the same
// two-word stand-in where the load decodes a file that is not YAML: a fenced
// gragas block of a guide.md, decoded as YAML, and a line of history.jsonl,
// decoded as JSON. The name is written with an escape for its first letter,
// or folded over two lines of a statement, so no one line of the file holds
// it and the decoded value does: one finding at the key's line, said in the
// value's words. The controls, the name on one line, are the line's finding.
// A name in the prose below a block is the prose line's finding and does not
// hide a block's value above it. Planted failure: the check decodes by the
// extension, so a .md and a .jsonl file are read by lines only, and the
// escape and the fold pass, the two gaps of a scan that reads lines only.
func TestANameAGuideBlockOrAHistoryLineHoldsInAValueIsFound(t *testing.T) {
	t.Parallel()
	first, second := "quo"+"r", "ve"+"x" // the stand-in, built at run time
	n, err := ParseNames([]byte(`\b` + first + `\s+` + second + `\b` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	bs := string(rune(92)) // a backslash, built at run time
	const guide = "language/go/guide.md"
	const statement = "    statement: Return the error; do not log and return it."
	const prose = "No rules here."
	cases := []struct {
		name, file string
		edit       func(string) string
		msgs       map[string]string // the line's text to find, by the message
	}{
		{"guide: the control, one line", guide, func(s string) string {
			return strings.Replace(s, statement, "    statement: Ask "+first+" "+second+", then return the error.", 1)
		}, map[string]string{"Ask " + first: NamesMsg}},
		{"guide: a statement with an escape", guide, func(s string) string {
			return strings.Replace(s, statement, `    statement: "Ask `+bs+`x71`+first[1:]+" "+second+`, then return the error."`, 1)
		}, map[string]string{"statement: \"Ask " + bs: NamesValueMsg}},
		{"guide: a statement folded over two lines", guide, func(s string) string {
			return strings.Replace(s, statement, "    statement: >-\n      Ask "+first+"\n      "+second+", then return the error.", 1)
		}, map[string]string{"statement: >-": NamesValueMsg}},
		{"guide: a value above a name in the prose below", guide, func(s string) string {
			s = strings.Replace(s, statement, `    statement: "Ask `+bs+`x71`+first[1:]+" "+second+`, then return the error."`, 1)
			return strings.Replace(s, prose, "Ask "+first+" "+second+" here.", 1)
		}, map[string]string{"statement: \"Ask " + bs: NamesValueMsg, "Ask " + first + " " + second + " here.": NamesMsg}},
		{"history: the control, one line", HistoryFile, func(s string) string {
			return strings.Replace(s, `"who":"team-a"`, `"who":"`+first+" "+second+`"`, 1)
		}, map[string]string{`"who":"` + first: NamesMsg}},
		{"history: a who with an escape", HistoryFile, func(s string) string {
			return strings.Replace(s, `"who":"team-a"`, `"who":"`+bs+`u0071`+first[1:]+" "+second+`"`, 1)
		}, map[string]string{`"who":"` + bs: NamesValueMsg}},
		{"bindings: a record named with no .yaml ending", "bindings/zz-record", func(string) string {
			return "reason: \"" + bs + "x71" + first[1:] + " " + second + "\"\n"
		}, map[string]string{"reason: \"" + bs: NamesValueMsg}},
		{"bindings: a record named guide.md, outside any fence", "bindings/guide.md", func(string) string {
			return "reason: \"" + bs + "x71" + first[1:] + " " + second + "\"\n"
		}, map[string]string{"reason: \"" + bs: NamesValueMsg}},
		{"bindings: a record named history.jsonl, a value over two lines", "bindings/history.jsonl", func(string) string {
			return "reason: \"" + bs + "x71" + first[1:] + "\n  " + second + "\"\n"
		}, map[string]string{"reason: \"" + bs: NamesValueMsg}},
		{"bindings: a record named .YAML", "bindings/zz-record.YAML", func(string) string {
			return "reason: \"" + bs + "x71" + first[1:] + " " + second + "\"\n"
		}, map[string]string{"reason: \"" + bs: NamesValueMsg}},
		{"history: an escape on the last line", HistoryFile, func(s string) string {
			i := strings.LastIndex(strings.TrimRight(s, "\n"), `"who":"`)
			j := i + len(`"who":"`)
			k := strings.Index(s[j:], `"`)
			return s[:j] + bs + `u0071` + first[1:] + " " + second + s[j+k:]
		}, map[string]string{`"who":"` + bs: NamesValueMsg}},
	}
	if got := loadWithNames(t, fullBook(t), n); len(got.Findings) != 0 {
		t.Fatalf("the clean control: findings = %v", got.Findings)
	}
	for _, c := range cases {
		files := fullBook(t)
		if c.file == guide {
			files[guide] = guideMD
		}
		files[c.file] = c.edit(files[c.file])
		if files[c.file] == fullBook(t)[c.file] {
			t.Fatalf("%s: the plant changed nothing in %s", c.name, c.file)
		}
		lines := strings.Split(files[c.file], "\n")
		var want []Finding
		for text, msg := range c.msgs {
			at := 0 // the last line that holds the text
			for i, l := range lines {
				if strings.Contains(l, text) {
					at = i + 1
				}
			}
			if at == 0 {
				t.Fatalf("%s: no line holds %q", c.name, text)
			}
			want = append(want, Finding{File: c.file, Line: at, Msg: msg, Fix: namesFix, Names: true})
		}
		slices.SortFunc(want, func(a, b Finding) int { return a.Line - b.Line })
		got := loadWithNames(t, files, n).Findings
		if !slices.Equal(got, want) {
			t.Errorf("%s: findings = %v; want %v alone", c.name, got, want)
		}
	}
}

// TestAGuideBelowTheBooksBindingsIsReadAsAGuideToo: a layer may be named
// bindings, so <dir>/bindings/<part>/guide.md is a guide the load reads block
// by block and, below the book's bindings/, a record it decodes whole. Both
// are read and the findings merge. Planted: a
// guide with an escaped name in a block, below the book's own bindings/, which
// the record read alone misses (it is no one YAML mapping); the same file in a
// folder merely named bindings, which only the guide read finds.
func TestAGuideBelowTheBooksBindingsIsReadAsAGuideToo(t *testing.T) {
	t.Parallel()
	first, second := "quo"+"r", "ve"+"x"
	n, err := ParseNames([]byte(`\b` + first + `\s+` + second + `\b` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	bs := string(rune(92))
	text := strings.Replace(guideMD, "    statement: Return the error; do not log and return it.",
		`    statement: "Ask `+bs+`x71`+first[1:]+" "+second+`, then return the error."`, 1)
	if text == guideMD {
		t.Fatal("the plant changed nothing")
	}
	for _, c := range []struct{ name, bindings, file string }{
		{"a part of a layer named bindings", "bindings", "bindings/go/guide.md"},
		{"below the book's own bindings/ in a folder", "language/bindings", "language/bindings/go/guide.md"},
		{"in a folder merely named bindings", "bindings", "language/bindings/guide.md"},
	} {
		got := namesInBook(n, nil, c.bindings, c.file, []byte(text))
		if len(got) != 1 || got[0].Msg != NamesValueMsg {
			t.Errorf("%s: findings = %v; want its one value finding", c.name, got)
		}
	}
}

// TestATermsLiteralLineIsReadWithTheLiteralBlanked: the literal of a defined
// term passes on its own line in terms.yaml and wherever else it is written
// bare, and a term reference in braces never reads as a name. Planted failure:
// the full-file read loses the term-literal allowance and the rulebook that
// defines a term for a name it must use goes red on the definition itself.
func TestATermsLiteralLineIsReadWithTheLiteralBlanked(t *testing.T) {
	t.Parallel()
	files := fullBook(t)
	files["terms.yaml"] += "  - slug: person\n    definition: The stand-in.\n    literal: " + plant + "\n"
	files["sources.yaml"] = strings.Replace(files["sources.yaml"], "title: Effective Go", "title: Effective Go by "+plant, 1)
	if got := loadNamed(t, files); len(got.Findings) != 0 {
		t.Errorf("findings = %v; want none, the literal is the rulebook's own", got.Findings)
	}
}

// TestALoadWithNoListReadsNoFileForNames: with no list nothing is held, and
// NamesIn with a nil list finds nothing.
func TestALoadWithNoListReadsNoFileForNames(t *testing.T) {
	t.Parallel()
	if got := NamesIn(nil, nil, plant+".yaml", []byte(plant)); got != nil {
		t.Errorf("NamesIn(nil) = %v", got)
	}
}

// TestMaskHidesEveryNameAndTheWholeOfAHashedOne: a name the patterns find is
// cut wherever it stands; one found by its hash has no text to cut, so the
// whole string goes; no list and no name leave the string as it is.
func TestMaskHidesEveryNameAndTheWholeOfAHashedOne(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		names    *Names
		in, want string
	}{
		{standInNames(t), "rules/" + plant + "/x.yaml: by " + plant, "rules/" + MaskedName + "/x.yaml: by " + MaskedName},
		{standInNames(t), "rules/errors.yaml", "rules/errors.yaml"},
		{hashedNames(t), "rules/" + plant + ".yaml", MaskedName},
		{nil, plant, plant},
	} {
		if got := c.names.Mask(c.in); got != c.want {
			t.Errorf("Mask(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestABadPatternsErrorDoesNotQuoteThePattern: the line of the list that does
// not compile is named by its number, never by its text, which is a piece of
// the private list. Planted failure: the error carries the compiler's
// message, which quotes the pattern.
func TestABadPatternsErrorDoesNotQuoteThePattern(t *testing.T) {
	t.Parallel()
	_, err := ParseNames([]byte("fine\n(" + plant + "\n"))
	bad, ok := errors.AsType[*BadNames](err)
	if !ok || bad.Line != 2 || strings.Contains(err.Error(), plant) || err.Error() != "line 2 of the names file is not a pattern that compiles" {
		t.Errorf("err = %v; want line 2 and no pattern", err)
	}
}
