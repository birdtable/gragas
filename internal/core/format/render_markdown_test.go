package format

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/markdown"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const guideFile = "language/go/guide.md"

// proseGuide has what an author writes around two blocks: headings, a table,
// a nested list, HTML, a link reference, and a comment inside a block.
const proseGuide = "# Go\n\nIntro with *emphasis* and a [link][ref].\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
	"- one\n  - nested\n    - deeper\n\n<div class=\"x\">\n  raw html\n</div>\n\n[ref]: https://example.com\n\n" +
	"## Errors\n\nBefore the block.\n\n```gragas\nanchor: errors\nrules:\n" + guideRules + "```\n\nBetween the blocks.\n\n" +
	"## Naming\n\n```gragas\n# the naming rules\nanchor: naming\nrules:\n" +
	"  - id: go.layout.9\n    statement: Name a package for what it holds.   # short\n    strength: should\n    status: proposed\n    origin: authored\n" +
	"```\n\nAfter the last block.\n"

func guideRender(t *testing.T, guide string, d model.Delta) ([]byte, error) {
	t.Helper()
	files := fullBook(t)
	files[guideFile] = guide
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	root := w.Seed("repo", files)
	got, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	ts, err := Render(p.Tree, p.Schema, root, got, d)
	if err != nil {
		return nil, err
	}
	if len(ts) != 1 || ts[0].Path != guideFile {
		t.Fatalf("targets = %+v", ts)
	}
	return ts[0].Data, nil
}

func asCRLF(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

func TestEachKindOfDeltaTouchesOnlyTheBlockItNamesInAGuide(t *testing.T) {
	t.Parallel()
	newRule := "id: go.layout.10\nstatement: Keep a package small.\nstrength: may\nstatus: proposed\norigin: authored"
	cases := []struct {
		name string
		d    model.Delta
		want string
	}{
		{"set a field of a rule in the first block", setRule("go.errors.9", "status", "active", "retired"),
			"-    status: active\n+    status: retired\n"},
		{"set a field of a rule in the second block", setRule("go.layout.9", "owner", "", "team-a"),
			"+    owner: team-a\n"},
		{"set a field and keep the comment on it", setRule("go.layout.9", "status", "proposed", "active"),
			"-    status: proposed\n+    status: active\n"},
		{"add a list item", setRule("go.layout.9", "aliases[]", "", "go.names.9"),
			"+    aliases:\n+      - go.names.9\n"},
		{"add a rule to a block", model.Delta{Changes: []model.Change{{Entity: "block:" + guideFile + "#naming", Field: "rules[]", After: newRule}}},
			"+  - id: go.layout.10\n+    statement: Keep a package small.\n+    strength: may\n+    status: proposed\n+    origin: authored\n"},
		{"add a block under a new anchor", model.Delta{Changes: []model.Change{{Entity: "block:" + guideFile + "#tests", After: "anchor: tests\nrules:\n  - id: go.tests.9\n    statement: Test a unit alone.\n    strength: should\n    status: proposed\n    origin: authored\n"}}},
			"+\n+```gragas\n+anchor: tests\n+rules:\n+  - id: go.tests.9\n+    statement: Test a unit alone.\n+    strength: should\n+    status: proposed\n+    origin: authored\n+```\n"},
	}
	ran := 0
	for _, c := range cases {
		out, err := guideRender(t, proseGuide, c.d)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got := diff(proseGuide, string(out)); got != c.want {
			t.Errorf("%s: diff\n%s\nwant\n%s", c.name, got, c.want)
		}
		ran++
	}
	if ran != len(cases) {
		t.Fatalf("ran %d of %d", ran, len(cases))
	}
}

// TestEverythingOutsideTheBlockIsByteIdentical edits each block of a guide and
// holds the bytes before and after the block's body, whatever the file's line
// ending, BOM or last line.
func TestEverythingOutsideTheBlockIsByteIdentical(t *testing.T) {
	t.Parallel()
	variants := map[string]func(string) string{
		"lf":                  func(s string) string { return s },
		"crlf":                asCRLF,
		"bom":                 func(s string) string { return "\ufeff" + s },
		"crlf and bom":        func(s string) string { return "\ufeff" + asCRLF(s) },
		"no trailing newline": func(s string) string { return strings.TrimSuffix(s, "\n") },
		"ends on a fence":     func(s string) string { return strings.TrimSuffix(s, "\nAfter the last block.\n") },
	}
	edits := map[string]model.Delta{
		"go.errors.9": setRule("go.errors.9", "status", "active", "retired"),
		"go.layout.9": setRule("go.layout.9", "owner", "", "team-a"),
	}
	ran := 0
	for name, vary := range variants {
		guide := vary(proseGuide)
		for id, d := range edits {
			out, err := guideRender(t, guide, d)
			if err != nil {
				t.Errorf("%s %s: %v", name, id, err)
				continue
			}
			ran++
			// the edited block is the only place the bytes differ: the common
			// prefix and suffix must cover the file but for that block's body
			lo := 0
			for lo < len(guide) && lo < len(out) && guide[lo] == out[lo] {
				lo++
			}
			hi := 0
			for hi < len(guide)-lo && hi < len(out)-lo && guide[len(guide)-1-hi] == out[len(out)-1-hi] {
				hi++
			}
			changed := guide[lo : len(guide)-hi]
			if strings.Count(changed, "```") != 0 || strings.Contains(changed, "Between") || strings.Contains(changed, "After the last") {
				t.Errorf("%s %s: the change reaches outside a block body: %q", name, id, changed)
			}
			eol := "\n"
			if strings.Contains(name, "crlf") {
				eol = "\r\n"
			}
			if (eol == "\r\n" && bytes.Count(out, []byte("\n")) != bytes.Count(out, []byte("\r\n"))) || (eol == "\n" && bytes.Contains(out, []byte("\r"))) {
				t.Errorf("%s %s: the line endings changed", name, id)
			}
			if strings.HasPrefix(guide, "\ufeff") != strings.HasPrefix(string(out), "\ufeff") || strings.HasSuffix(guide, "\n") != strings.HasSuffix(string(out), "\n") {
				t.Errorf("%s %s: the BOM or the last newline changed", name, id)
			}
		}
	}
	if ran != len(variants)*len(edits) {
		t.Fatalf("ran %d of %d", ran, len(variants)*len(edits))
	}
}

func TestACRLFGuideGetsTheSameEditInItsOwnLineEnding(t *testing.T) {
	t.Parallel()
	d := setRule("go.layout.9", "owner", "", "team-a")
	lf, err1 := guideRender(t, proseGuide, d)
	crlf, err2 := guideRender(t, asCRLF(proseGuide), d)
	if err1 != nil || err2 != nil || string(crlf) != asCRLF(string(lf)) {
		t.Errorf("errors %v %v; the CRLF render is not the LF render in CRLF", err1, err2)
	}
}

func TestADeltaThatNeedsProseOrARemovalIsRefusedWithTheSectionNamed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		d    model.Delta
		want string
	}{
		{"prose of a section", model.Delta{Changes: []model.Change{{Entity: "prose:" + guideFile + "#errors", Field: "text", Before: "a", After: "b"}}}, `the section "errors"`},
		{"removing a rule", model.Delta{Changes: []model.Change{{Entity: "rule:go.layout.9", Before: "x"}}}, `the section "naming"`},
		{"a new rule with no block named", model.Delta{Changes: []model.Change{{Entity: "rule:go.errors.9", After: "id: go.errors.9"}}}, "block:<file>#<anchor>"},
	}
	for _, c := range cases {
		_, err := guideRender(t, proseGuide, c.d)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want one holding %q", c.name, err, c.want)
		}
	}
}

func TestAStaleBeforeInAGuideBlockIsRefusedWithTheLineOfTheFile(t *testing.T) {
	t.Parallel()
	_, err := guideRender(t, proseGuide, setRule("go.errors.9", "status", "retired", "active"))
	line := 1 + strings.Count(proseGuide[:strings.Index(proseGuide, "    status: active")], "\n")
	if !errors.Is(err, ErrMoved) || !strings.Contains(err.Error(), guideFile+":"+itoa(line)+":") {
		t.Errorf("err = %v; want ErrMoved at %s:%d", err, guideFile, line)
	}
}

func TestABlockUnderAnAnchorThatExistsIsRefused(t *testing.T) {
	t.Parallel()
	_, err := guideRender(t, proseGuide, model.Delta{Changes: []model.Change{{Entity: "block:" + guideFile + "#errors", After: "anchor: errors\nrules: []\n"}}})
	if !errors.Is(err, ErrMoved) {
		t.Errorf("err = %v", err)
	}
}

func itoa(n int) string {
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}

// nopSchema accepts every document.
type nopSchema struct{}

func (nopSchema) Check(string, []byte) ([]model.Problem, error) { return nil, nil }

var _ port.Schema = nopSchema{}

func FuzzRenderGuide(f *testing.F) {
	f.Add(proseGuide, "status", "active", "retired")
	f.Add(asCRLF(proseGuide), "owner", "", "x")
	f.Add("```gragas\nanchor: a\nrules:\n  - id: a.b.1\n    owner: &x y\n```", "owner", "y", "z")
	f.Add("```gragas\nanchor: a\nrules:\n  - id: a.b.1\n    pros:\n      - a\n      # c\n      - b\n```\n", "pros[]", "", "z")
	f.Fuzz(func(t *testing.T, guide, field, before, after string) {
		c := model.Change{Entity: "rule:a.b.1", Field: field, Before: before, After: after}
		out, err := guideChange(nopSchema{}, guideFile, []byte(guide), c)
		if err != nil {
			return
		}
		if outsideBlocks(guide) != outsideBlocks(string(out)) {
			t.Fatalf("bytes outside the blocks changed:\n%q\n%q", guide, out)
		}
	})
}

// outsideBlocks is the guide with the body of every closed gragas block taken out.
func outsideBlocks(src string) string {
	doc, _ := markdown.Parse([]byte(src))
	blocks := gragasBlocks(doc)
	for _, b := range slices.Backward(blocks) {
		src = src[:b.BodyStart] + src[b.BodyEnd:]
	}
	return src
}
