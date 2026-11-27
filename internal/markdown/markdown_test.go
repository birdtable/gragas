package markdown_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/markdown"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// dump is the whole parse of a guide as lines of text, the form the goldens
// hold: the headings, paragraphs and blocks in document order, then the MUST
// lines.
func dump(d *markdown.Doc, err error) string {
	var b strings.Builder
	for _, h := range d.Headings {
		fmt.Fprintf(&b, "heading %d line %d %q #%s\n", h.Level, h.Line, h.Text, h.Anchor)
	}
	for _, p := range d.Paragraphs {
		fmt.Fprintf(&b, "paragraph lines %d-%d %q\n", p.Line, p.EndLine, p.Text)
	}
	for _, k := range d.Blocks {
		fmt.Fprintf(&b, "block lines %d-%d info %q closed %t under #%s bytes %d-%d body %d-%d %q\n",
			k.Line, k.EndLine, k.Info, k.Closed, k.Section, k.Start, k.End, k.BodyStart, k.BodyEnd, k.Body)
	}
	for _, m := range d.Musts {
		fmt.Fprintf(&b, "must line %d under #%s %q\n", m.Line, m.Section, m.Text)
	}
	if err != nil {
		if e, ok := errors.AsType[*markdown.Error](err); ok {
			fmt.Fprintf(&b, "error line %d: %v\n", e.Line, e.Err)
		} else {
			fmt.Fprintf(&b, "error %v\n", err)
		}
	}
	return b.String()
}

// TestTheFiveGuideShapesGiveTheirLiteralParse is the done-when of the
// goldens: each guide in testdata/ parses to the text beside it, and the
// golden is a literal, never made by the code it checks.
func TestTheFiveGuideShapesGiveTheirLiteralParse(t *testing.T) {
	t.Parallel()
	shapes := []string{"several-blocks", "no-blocks", "unterminated", "trailing-text", "prose-musts"}
	for _, name := range shapes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, err := os.ReadFile(filepath.Join("testdata", name+".md"))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", name+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			if got := dump(markdown.Parse(src)); got != string(want) {
				t.Errorf("%s parses to:\n%s\nwant:\n%s", name, got, want)
			}
		})
	}
}

// TestAGoldenThatDisagreesFails plants a golden that is not the parse and
// shows the comparison of the test above sees it.
func TestAGoldenThatDisagreesFails(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("testdata", "no-blocks.md"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "several-blocks.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if dump(markdown.Parse(src)) == string(want) {
		t.Error("two different guides gave one parse")
	}
}

func TestFindReturnsOnlyTheGragasBlocks(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("testdata", "several-blocks.md"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := markdown.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(d.Blocks); got != 3 {
		t.Fatalf("the guide has %d blocks, want 3", got)
	}
	found := d.Find(markdown.GragasInfo)
	if len(found) != 2 {
		t.Fatalf("Find(gragas) found %d blocks, want 2", len(found))
	}
	for _, b := range found {
		if b.Info != "gragas" {
			t.Errorf("Find(gragas) returned a block with info %q", b.Info)
		}
	}
	if got := d.Find("go"); len(got) != 1 || got[0].Info != "go" {
		t.Errorf("Find(go) = %v, want the one go block", got)
	}
	if got := d.Find("nothing"); len(got) != 0 {
		t.Errorf("Find(nothing) = %v, want none", got)
	}
}

// TestABlocksBytesAreTheBlockAndNothingElse is the promise Render relies on:
// the offsets of a block cut the source into the part before it, the block and
// the part after, so a rewrite of the block moves no other byte.
func TestABlocksBytesAreTheBlockAndNothingElse(t *testing.T) {
	t.Parallel()
	src := "intro\n\n```gragas\nid: a\n```\nafter\n"
	d, err := markdown.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	blocks := d.Find(markdown.GragasInfo)
	if len(blocks) != 1 {
		t.Fatalf("found %d blocks, want 1", len(blocks))
	}
	b := blocks[0]
	if got, want := src[b.Start:b.End], "```gragas\nid: a\n```\n"; got != want {
		t.Errorf("the block is %q, want %q", got, want)
	}
	if got, want := src[b.BodyStart:b.BodyEnd], "id: a\n"; got != want {
		t.Errorf("the body is %q, want %q", got, want)
	}
	if got := src[:b.Start] + src[b.End:]; got != "intro\n\nafter\n" {
		t.Errorf("the text outside the block is %q", got)
	}
}

func TestAnUnclosedFenceIsATypedErrorWithItsLine(t *testing.T) {
	t.Parallel()
	d, err := markdown.Parse([]byte("# T\n\n```gragas\nid: a\n"))
	if e, ok := errors.AsType[*markdown.Error](err); !ok || e.Line != 3 {
		t.Fatalf("error = %v, want a markdown.Error on line 3", err)
	}
	if !errors.Is(err, markdown.ErrUnterminatedFence) {
		t.Errorf("error %v does not match ErrUnterminatedFence", err)
	}
	if len(d.Blocks) != 1 || d.Blocks[0].Closed {
		t.Errorf("blocks = %+v, want the one open block", d.Blocks)
	}
}

func TestAClosedFenceIsNoError(t *testing.T) {
	t.Parallel()
	if _, err := markdown.Parse([]byte("```gragas\nid: a\n```\n")); err != nil {
		t.Errorf("a closed fence gave %v", err)
	}
	if _, err := markdown.Parse([]byte("```gragas\nid: a\n``` x\n")); err == nil {
		t.Error("a closing line with text after it closed the fence")
	}
}

func TestTheFencesMarkdownReadsAreThoseTheBlockFinderReads(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, src string
		blocks    int
		closed    bool
	}{
		{"a tilde fence", "~~~gragas\nx\n~~~\n", 1, true},
		{"a longer closing fence", "```gragas\nx\n`````\n", 1, true},
		{"a shorter closing fence", "````gragas\nx\n```\n", 1, false},
		{"a fence indented three spaces", "   ```gragas\n   x\n   ```\n", 1, true},
		{"an indented code block with a fence in it", "    ```gragas\n    x\n    ```\n", 0, false},
		{"a fence in a list item", "- item\n\n  ```gragas\n  x\n  ```\n", 0, false},
		{"a fence in a quotation", "> ```gragas\n> x\n> ```\n", 0, false},
		{"a fence with no info and no body", "```\n```\n", 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, _ := markdown.Parse([]byte(tc.src))
			if len(d.Blocks) != tc.blocks {
				t.Fatalf("%q gave %d blocks, want %d", tc.src, len(d.Blocks), tc.blocks)
			}
			if tc.blocks > 0 && d.Blocks[0].Closed != tc.closed {
				t.Errorf("%q: closed = %t, want %t", tc.src, d.Blocks[0].Closed, tc.closed)
			}
		})
	}
}

func TestAnchorsAreUniqueInADocument(t *testing.T) {
	t.Parallel()
	d, err := markdown.Parse([]byte("# Intro\n\n## Intro\n\n## Intro\n\n# Intro 1\n\n# `Code` and **bold**!\n"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range d.Headings {
		got = append(got, h.Anchor)
	}
	want := []string{"intro", "intro-1", "intro-2", "intro-1-1", "code-and-bold"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("anchors = %v, want %v", got, want)
	}
}

func TestEveryLineOfABlockBelongsToOneSection(t *testing.T) {
	t.Parallel()
	d, err := markdown.Parse([]byte("```gragas\nbefore: any heading\n```\n\n# A\n\n```gragas\nin: a\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 2 {
		t.Fatalf("found %d blocks, want 2", len(d.Blocks))
	}
	if d.Blocks[0].Section != "" || d.Blocks[1].Section != "a" {
		t.Errorf("sections = %q and %q, want \"\" and \"a\"", d.Blocks[0].Section, d.Blocks[1].Section)
	}
}

// FuzzParse holds the parser to its promises on any input: it never panics;
// the only error is an unclosed fence; and every offset and line it reports
// stands inside the source, in order.
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"",
		"# T\n\n```gragas\nid: a\n```\n",
		"```gragas\nunclosed",
		"~~~\n~~~",
		"```\n```\n```\n",
		"Title\n===\n\n- a MUST\n  ```gragas\n  x\n  ```\n",
		"> ```gragas\n> x\n",
		"#\n##\n",
		"\r\n\r\n```gragas\r\nx\r\n```\r\n",
		"\x00\xff\xfe ```\n",
		"    ```gragas\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := markdown.Parse(data)
		if d == nil {
			t.Fatal("Parse returned no document")
		}
		if err != nil && !errors.Is(err, markdown.ErrUnterminatedFence) {
			t.Fatalf("error %v is not an unclosed fence", err)
		}
		lines := strings.Count(string(data), "\n") + 1
		for _, b := range d.Blocks {
			if b.Start < 0 || b.Start > b.BodyStart || b.BodyStart > b.BodyEnd || b.BodyEnd > b.End || b.End > len(data) {
				t.Fatalf("offsets %+v are not in order inside %d bytes", b, len(data))
			}
			if b.Line < 1 || b.Line > lines || b.EndLine < b.Line || b.EndLine > lines {
				t.Fatalf("lines %+v are not inside %d lines", b, lines)
			}
			if b.Body != string(data[b.BodyStart:b.BodyEnd]) {
				t.Fatalf("the body of %+v is not the bytes it names", b)
			}
		}
		last := 0
		for _, h := range d.Headings {
			if h.Line < last || h.Line > lines {
				t.Fatalf("heading %+v is out of order or past line %d", h, lines)
			}
			last = h.Line
		}
		for _, m := range d.Musts {
			if m.Line < 1 || m.Line > lines {
				t.Fatalf("must %+v is past line %d", m, lines)
			}
		}
	})
}

// TestStatementsListEveryStrengthLineAndItsWordsAndMustsStayTheMustLines: SHOULD
// and MAY are listed beside MUST, a line with two words lists both in the order
// they stand, a word in lower case or inside code is none, and Musts is the
// statements that carry MUST.
func TestStatementsListEveryStrengthLineAndItsWordsAndMustsStayTheMustLines(t *testing.T) {
	t.Parallel()
	src := "# G\n\nA MUST be so.\nB SHOULD be so and C MAY not.\nd must not count.\nE MAY be so.\n\n    F MUST in code\n\n## H\n\nG SHOULD and MUST both.\n"
	d, err := markdown.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range d.Statements {
		got = append(got, fmt.Sprintf("%d #%s %v", s.Line, s.Section, s.Words))
	}
	want := []string{"3 #g [MUST]", "4 #g [SHOULD MAY]", "6 #g [MAY]", "12 #h [SHOULD MUST]"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("statements = %q, want %q", got, want)
	}
	if len(d.Musts) != 2 || d.Musts[0].Line != 3 || d.Musts[1].Line != 12 {
		t.Errorf("musts = %+v, want the lines 3 and 12", d.Musts)
	}
}
