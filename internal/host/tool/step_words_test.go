package tool_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// TestAStepsWordsAreLiteralButScratchAndNull: every word
// a step gives is passed as written, but {scratch}, which is the scratch
// folder, a word starting {scratch}/, which is the path in it in the system's
// own form, and {null}, which is os.DevNull. Planted: stepWord passing
// {null} as written, and joining {scratch}/ with a slash, not the system's
// form (each red on its row; the second on a system whose separator is not a
// slash, which the lent Windows run of item 11 holds).
func TestAStepsWordsAreLiteralButScratchAndNull(t *testing.T) {
	t.Parallel()
	scratch := filepath.Join(t.TempDir(), "scratch")
	argv := []string{"-e", "lint", "--out=x", "{scratch}", "{scratch}/a/b.json", "{null}", "", "scratch}", "-"}
	got, err := tool.StepWords(argv, scratch)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-e", "lint", "--out=x", scratch, filepath.Join(scratch, "a", "b.json"), os.DevNull, "", "scratch}", "-"}
	if !slices.Equal(got, want) {
		t.Errorf("StepWords(%q) = %q, want %q", argv, got, want)
	}
}

// TestAStepsWordWithAnyOtherBraceIsRefused: a { that is
// not {scratch} or {null} alone, nor {scratch}/ at the start, is refused,
// matching port.ErrRefused, in any place of the word, and a {scratch}/ path
// that leaves the folder is refused as a path. Planted: stepWord passing a
// word that holds a { other than the two (the brace rows red), and taking
// the rest of {scratch}/ as written (the path rows red).
func TestAStepsWordWithAnyOtherBraceIsRefused(t *testing.T) {
	t.Parallel()
	scratch := filepath.Join(t.TempDir(), "scratch")
	for _, w := range []string{
		"{home}", "{", "{scratch", "{null}x", "x{null}", "{Scratch}", "--out={scratch}", "{scratch}x",
		"{scratch}/{null}", "{scratch}/a{b",
		"{scratch}/", "{scratch}/..", "{scratch}/a/../..", "{scratch}//a", "{scratch}/./a", "{scratch}/a\\b", "{scratch}/a:x", "{scratch}/a/",
	} {
		if got, err := tool.StepWords([]string{"ok", w}, scratch); !errors.Is(err, port.ErrRefused) {
			t.Errorf("StepWords(%q) = %q, %v, want port.ErrRefused", w, got, err)
		}
	}
}

// TestAStepsPathIsRelativeSlashedAndStaysBelowTheScratchFolder: a path a step
// places, collects or reads is relative, with slashes,
// no . or .. part, no backslash, colon or NUL, and not empty; each refusal is
// a row, matching port.ErrRefused, and the control is taken in the system's
// own form. Planted: stepPath taking the path as written, unjudged (every
// refusal red).
func TestAStepsPathIsRelativeSlashedAndStaysBelowTheScratchFolder(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, path string }{
		{"empty", ""},
		{"absolute", "/a/b"},
		{"up", ".."},
		{"up inside", "a/../../b"},
		{"dot", "."},
		{"dot part", "a/./b"},
		{"doubled slash", "a//b"},
		{"trailing slash", "a/"},
		{"backslash", "x/a\\b"},
		{"colon", "c:b"},
		{"nul", "a\x00b"},
	} {
		if got, err := tool.StepPath(c.path); !errors.Is(err, port.ErrRefused) {
			t.Errorf("%s: StepPath(%q) = %q, %v, want port.ErrRefused", c.name, c.path, got, err)
		}
	}
	got, err := tool.StepPath("config/eslint.config.js")
	if want := filepath.Join("config", "eslint.config.js"); err != nil || got != want {
		t.Errorf("StepPath(the control) = %q, %v, want %q", got, err, want)
	}
}

// scratchWith is a scratch folder holding files, by slash-separated path, and
// its os.Root.
func scratchWith(t *testing.T, files map[string]string) (string, *os.Root) {
	t.Helper()
	dir := private(t)
	for name, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return dir, root
}

// paths is the paths of files, in order.
func paths(files []plugin.File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

// TestAGlobHasStarQuestionAndDoubleStarOnlyMatchedCaseExact: a glob is
// matched below the scratch folder the config set is mirrored
// in, by path in order, * within a part, ? one character, ** any number of
// folders, none included, and case exact. Planted: partMatch folding case
// (the case row red), ** taking one folder at least (the none row red), and
// * crossing a slash (the part row red).
func TestAGlobHasStarQuestionAndDoubleStarOnlyMatchedCaseExact(t *testing.T) {
	t.Parallel()
	_, root := scratchWith(t, map[string]string{
		"a.json": "{}", "C.JSON": "{}", "ab.json": "{}", "x/a.json": "{}", "x/y/a.json": "{}", "x/b.yaml": "b",
	})
	// One name of each spelling, so a volume that folds case holds the same
	// files: the match is of the names as the walk lists them.
	for _, c := range []struct {
		name, glob string
		want       []string
	}{
		{"case exact", "c.json", nil},
		{"star in a part", "*.json", []string{"a.json", "ab.json"}},
		{"question one character", "a?.json", []string{"ab.json"}},
		{"double star none or more", "**/a.json", []string{"a.json", "x/a.json", "x/y/a.json"}},
		{"star never crosses a slash", "x/*", []string{"x/a.json", "x/b.yaml"}},
		{"nothing matched", "*.toml", nil},
	} {
		got, err := tool.ReadGlob(root, c.glob)
		if err != nil {
			t.Errorf("%s: ReadGlob(%q) = %v", c.name, c.glob, err)
			continue
		}
		if !slices.Equal(paths(got), c.want) {
			t.Errorf("%s: ReadGlob(%q) = %q, want %q", c.name, c.glob, paths(got), c.want)
		}
	}
}

// TestAGlobReadsAsGitignoreDoes holds two rules,
// which the SDK's declarative reader relies on: ** is one whole
// part matching zero or more folders, so <dir>/**/*<ext> matches a file
// directly in <dir>; * and ? match within one part, never across a slash, ?
// exactly one character; and * takes a name that starts with a dot. A ** inside
// a part, as a**b, is not the globstar: the codec refuses it, and so does the
// host, matching port.ErrRefused. Planted: ** taking one folder or more (the
// zero row red), ** taking at most one (the two row red), the whole path
// matched as one part (the slash rows red), ? taking none or one (the one
// character row red), and * refusing a leading dot (the dotfile row red).
func TestAGlobReadsAsGitignoreDoes(t *testing.T) {
	t.Parallel()
	_, root := scratchWith(t, map[string]string{
		"z/a.json": "{}", "o/x/a.json": "{}", "w/x/y/a.json": "{}",
		"s/a.json": "{}", "s/x/b.json": "{}",
		"q/a.json": "{}", "q/ab.json": "{}", "q/abc.json": "{}", "q/a/b.json": "{}",
		"t/.x": "x", "t/a": "a",
	})
	for _, c := range []struct {
		name, glob string
		want       []string
	}{
		{"** with no folder between", "z/**/*.json", []string{"z/a.json"}},
		{"** with one folder between", "o/**/*.json", []string{"o/x/a.json"}},
		{"** with two folders between", "w/**/*.json", []string{"w/x/y/a.json"}},
		{"* never crosses a slash", "s/*.json", []string{"s/a.json"}},
		{"? is exactly one character", "q/a?.json", []string{"q/ab.json"}},
		{"? never stands for a slash", "q/a?b.json", nil},
		{"* takes a dotfile", "t/*", []string{"t/.x", "t/a"}},
	} {
		got, err := tool.ReadGlob(root, c.glob)
		if err != nil {
			t.Errorf("%s: ReadGlob(%q) = %v", c.name, c.glob, err)
			continue
		}
		if !slices.Equal(paths(got), c.want) {
			t.Errorf("%s: ReadGlob(%q) = %q, want %q", c.name, c.glob, paths(got), c.want)
		}
	}
	if got, err := tool.ReadGlob(root, "q/a**b.json"); !errors.Is(err, port.ErrRefused) {
		t.Errorf("a ** inside a part: ReadGlob = %q, %v, want port.ErrRefused, as the codec refuses it", paths(got), err)
	}
}

// TestAGlobBeyondTheContractIsRefused: a glob with [, ],
// {, } or a backslash, a ** beside other characters, and one that is no path
// below the scratch folder are refused, matching port.ErrRefused, before
// anything is walked. Planted: globParts judging nothing (every row red,
// some by a match, some by a walk of no file).
func TestAGlobBeyondTheContractIsRefused(t *testing.T) {
	t.Parallel()
	_, root := scratchWith(t, map[string]string{"a.json": "{}"})
	for _, g := range []string{"", "[ab].json", "{a,b}.json", "x/a\\.json", "**.json", "x/a**", "../a.json", "/a.json", "a//*.json", "./a.json"} {
		if got, err := tool.ReadGlob(root, g); !errors.Is(err, port.ErrRefused) {
			t.Errorf("ReadGlob(%q) = %q, %v, want port.ErrRefused", g, paths(got), err)
		}
	}
}

// TestAReadIsHeldToItsCapsAtTheRead: a read gives at most
// 256 files, 1 MiB each and 8 MiB in all, and one past any cap is refused,
// matching port.ErrRefused; the control at each cap is read whole, text as
// text and other bytes as base64. Planted: the count checked with > not ==
// (the 257 row red), readCapped reading with no limit and judging the size
// after (no row red by count, so its row is the one-byte-over file, which the
// limit cuts to cap+1: red when the cap check is dropped), and the total
// left out of the cap (the in-all row red).
func TestAReadIsHeldToItsCapsAtTheRead(t *testing.T) {
	t.Parallel()
	many := func(n int) map[string]string {
		files := map[string]string{}
		for i := range n {
			files[fmt.Sprintf("f/%03d.txt", i)] = "x"
		}
		return files
	}
	big := strings.Repeat("a", plugin.MaxFile)
	// full is n files of 1 MiB, the last of them short by short bytes.
	full := func(n, short int) map[string]string {
		files := map[string]string{}
		for i := range n {
			files[fmt.Sprintf("b/%d.txt", i)] = big
		}
		files[fmt.Sprintf("b/%d.txt", n-1)] = big[short:]
		return files
	}
	whole := plugin.MaxFiles / plugin.MaxFile
	for _, c := range []struct {
		name   string
		files  map[string]string
		glob   string
		refuse bool
	}{
		{"256 files", many(plugin.MaxReadFiles), "f/*", false},
		{"257 files", many(plugin.MaxReadFiles + 1), "f/*", true},
		{"a file at 1 MiB", map[string]string{"a.txt": big}, "a.txt", false},
		{"a file one byte over", map[string]string{"a.txt": big + "a"}, "a.txt", true},
		{"8 MiB in all", full(whole, 0), "b/*", false},
		{"one byte past 8 MiB in all", full(whole+1, plugin.MaxFile-1), "b/*", true},
	} {
		_, root := scratchWith(t, c.files)
		got, err := tool.ReadGlob(root, c.glob)
		switch {
		case c.refuse && !errors.Is(err, port.ErrRefused):
			t.Errorf("%s: ReadGlob = %d files, %v, want port.ErrRefused", c.name, len(got), err)
		case !c.refuse && (err != nil || len(got) != len(c.files)):
			t.Errorf("%s: ReadGlob = %d files, %v, want %d and no error", c.name, len(got), err, len(c.files))
		}
	}
	_, root := scratchWith(t, map[string]string{"t.txt": "text", "b.bin": "\xff\x00"})
	got, err := tool.ReadGlob(root, "*")
	if err != nil || len(got) != 2 {
		t.Fatalf("ReadGlob(*) = %v, %v", got, err)
	}
	if b := got[0].Data; b.Base64 == nil || *b.Base64 != "/wA=" {
		t.Errorf("the bytes that are not UTF-8 are %+v, want base64 /wA=", b)
	}
	if b := got[1].Data; b.Text == nil || *b.Text != "text" {
		t.Errorf("the text is %+v, want text", b)
	}
}

// TestAReadThroughALinkReadsNothingOutsideTheScratchFolder: a link in the
// scratch folder, to a file or a folder outside it, is
// refused when matched, matching port.ErrRefused, and a link to a folder is
// never walked, so nothing outside is read. Planted: readCapped opening by
// root.Open, which follows a link inside the root (the file row then reads
// the link's own target in the folder, red), and the walk following links
// (the folder row red).
func TestAReadThroughALinkReadsNothingOutsideTheScratchFolder(t *testing.T) {
	t.Parallel()
	outside := private(t)
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, root := scratchWith(t, map[string]string{"in.txt": "in"})
	testenv.Link(t, secret, filepath.Join(dir, "out.txt"))
	testenv.Link(t, filepath.Join(dir, "in.txt"), filepath.Join(dir, "near.txt"))
	testenv.Link(t, outside, filepath.Join(dir, "away"))
	for _, g := range []string{"out.txt", "near.txt"} {
		if got, err := tool.ReadGlob(root, g); !errors.Is(err, port.ErrRefused) {
			t.Errorf("ReadGlob(%q) through a link = %q, %v, want port.ErrRefused", g, paths(got), err)
		}
	}
	if got, err := tool.ReadGlob(root, "**/secret.txt"); err != nil || len(got) != 0 {
		t.Errorf("ReadGlob(**/secret.txt) = %q, %v, want no file: a link to a folder is not walked", paths(got), err)
	}
}
