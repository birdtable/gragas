// Package contracttest holds one suite per port, each a function a test calls
// with a way to build the port: the fake of internal/port/testing in its own
// tests, and the real adapter in its own package, so the fake is pinned by
// the thing it stands in for (tests.doubles.4, architecture section 4). A
// suite states what the port's doc comment promises, and nothing of how an
// adapter does it.
//
// The suites that exist are those of the ports whose adapters are built or
// next: Tree, Writer, Schema, Clock, Assets, Terminal, Trust, Bundles, Steps
// and the identity method of History. The others arrive with their adapters.
// It is a stand-in: only _test.go files import it.
package contracttest

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"sort"
	"testing"
	"testing/fstest"
	"time"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ClockSlack is how far from the instant it is told to expect a clock may be.
const ClockSlack = 5 * time.Second

// Clock runs the Clock suite against the port build answers. expected says the
// instant the clock must be near, within ClockSlack: the host's time for a real
// adapter, so one frozen at 1970 fails, and the seeded instant for the fake.
func Clock(t *testing.T, build func(t *testing.T) port.Clock, expected func() time.Time) {
	t.Helper()
	t.Run("the instant is near the one expected", func(t *testing.T) {
		want := expected()
		if got := build(t).Now(); got.Before(want.Add(-ClockSlack)) || got.After(want.Add(ClockSlack)) {
			t.Errorf("Now is %v, want within %v of %v", got, ClockSlack, want)
		}
	})
	t.Run("the instant is in UTC and is no zero time", func(t *testing.T) {
		now := build(t).Now()
		if now.IsZero() {
			t.Error("Now answered the zero time")
		}
		if now.Location() != time.UTC {
			t.Errorf("Now is in %v, want UTC", now.Location())
		}
	})
	t.Run("the instant never goes back", func(t *testing.T) {
		c := build(t)
		first := c.Now()
		if second := c.Now(); second.Before(first) {
			t.Errorf("Now went from %v back to %v", first, second)
		}
	})
}

// Assets runs the Assets suite against the port build answers. known is the
// files the port holds, by slash path with their content, and build answers
// the port over exactly those: the real adapter passes what its embedded
// folder holds.
func Assets(t *testing.T, known map[string]string, build func(t *testing.T) port.Assets) {
	t.Helper()
	if len(known) == 0 {
		t.Fatal("the suite was given no files to read")
	}
	var paths []string
	for p := range known {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	t.Run("a file reads whole", func(t *testing.T) {
		a := build(t)
		for _, p := range paths {
			got, err := a.Read(p)
			if err != nil || string(got) != known[p] {
				t.Errorf("Read(%q) = %q, %v, want %q", p, got, err, known[p])
			}
		}
	})
	t.Run("a path that is not embedded is not found", func(t *testing.T) {
		if _, err := build(t).Read("no/such/file"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Read of a missing path = %v, want fs.ErrNotExist", err)
		}
	})
	t.Run("the whole folder lists sorted", func(t *testing.T) {
		got, err := build(t).List("")
		if err != nil || !slices.Equal(got, paths) {
			t.Errorf("List(\"\") = %v, %v, want %v", got, err, paths)
		}
	})
	t.Run("a folder with nothing is an empty list", func(t *testing.T) {
		got, err := build(t).List("no-such-folder")
		if err != nil || len(got) != 0 {
			t.Errorf("List of an empty folder = %v, %v, want none", got, err)
		}
	})
	t.Run("a folder that leaves the embedded one is an error", func(t *testing.T) {
		if _, err := build(t).List("../outside"); err == nil {
			t.Error("List(\"../outside\") was taken")
		}
	})
}

// SchemaCase is the one schema the Schema suite checks documents against: its
// name, its text, a document that is valid and one that is not.
type SchemaCase struct {
	Name string
	Text string
	Good []byte
	Bad  []byte
}

// SchemaCases is the schema the suite uses: an object that must hold id.
func SchemaCases() SchemaCase {
	return SchemaCase{
		Name: "doc.schema.json",
		Text: `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["id"]}`,
		Good: []byte(`{"id":1}`),
		Bad:  []byte(`{}`),
	}
}

// Schema runs the Schema suite against the port build answers over c.
func Schema(t *testing.T, build func(t *testing.T, c SchemaCase) port.Schema) {
	t.Helper()
	c := SchemaCases()
	t.Run("a valid document has no problem", func(t *testing.T) {
		got, err := build(t, c).Check(c.Name, c.Good)
		if err != nil || len(got) != 0 {
			t.Errorf("Check(good) = %v, %v, want nil", got, err)
		}
	})
	t.Run("an invalid document has up to ten problems sorted by place", func(t *testing.T) {
		got, err := build(t, c).Check(c.Name, c.Bad)
		if err != nil || len(got) == 0 || len(got) > 10 {
			t.Fatalf("Check(bad) = %v, %v, want one to ten problems", got, err)
		}
		if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i].Line < got[j].Line }) {
			t.Errorf("the problems are not sorted by place: %v", got)
		}
		for _, p := range got {
			if p.Msg == "" {
				t.Errorf("a problem says nothing: %v", p)
			}
		}
	})
	t.Run("a schema that was not compiled is one problem", func(t *testing.T) {
		got, err := build(t, c).Check("nope.schema.json", c.Good)
		if err != nil || len(got) != 1 {
			t.Errorf("Check(unknown name) = %v, %v, want one problem and no error", got, err)
		}
	})
	t.Run("a document that is not JSON is an error", func(t *testing.T) {
		if _, err := build(t, c).Check(c.Name, []byte(`{"id":`)); err == nil {
			t.Error("a document that is not JSON was checked")
		}
	})
}

// terminalShow is the part of the Terminal suite that draws screens.
func terminalShow(t *testing.T, build func(t *testing.T, lines []string) port.Terminal) {
	t.Helper()
	t.Run("a whole screen of every block is shown", func(t *testing.T) {
		if err := build(t, nil).Show(everyBlock(t)); err != nil {
			t.Errorf("Show = %v", err)
		}
	})
	t.Run("a screen with a control sequence in a cell is shown", func(t *testing.T) {
		c, err := model.NewClosing(model.Done, "a \x1b[31mred\x1b[0m word", "")
		if err != nil {
			t.Fatal(err)
		}
		s, err := model.NewScreen(model.Heading{Text: "\x1b]0;title\x07check"},
			model.Table{Columns: []model.Column{{Title: "file"}}, Rows: [][]string{{"a\x1b[2Jb"}}}, c)
		if err != nil {
			t.Fatal(err)
		}
		if err := build(t, nil).Show(s); err != nil {
			t.Errorf("Show = %v", err)
		}
	})
	t.Run("the banner is written without a control sequence or a failure", func(t *testing.T) {
		if err := build(t, nil).Banner("v1.2.3 (abc1234)"); err != nil {
			t.Errorf("Banner = %v", err)
		}
	})
	t.Run("a screen with no block is an error", func(t *testing.T) {
		if err := build(t, nil).Show(model.Screen{}); err == nil {
			t.Error("a screen with no block was shown")
		}
	})
}

// Terminal runs the Terminal suite against the port build answers over a
// standard input holding lines, one per Ask or Line.
func Terminal(t *testing.T, build func(t *testing.T, lines []string) port.Terminal) {
	t.Helper()
	lines := []string{"yes", "", "last"}
	t.Run("the size and colour are never negative", func(t *testing.T) {
		term := build(t, nil)
		if w, c := term.Width(), term.Colors(); w < 0 || c < 0 {
			t.Errorf("Width = %d and Colors = %d, want neither negative", w, c)
		}
	})
	t.Run("a question reads one line at a time, an empty one included", func(t *testing.T) {
		term := build(t, lines)
		saysEach(t, "Ask", lines, func() (string, error) { return term.Ask(t.Context(), "ok?") })
	})
	t.Run("a question at the end of input is io.EOF", func(t *testing.T) {
		if _, err := build(t, nil).Ask(t.Context(), "ok?"); !errors.Is(err, io.EOF) {
			t.Errorf("Ask at the end = %v, want io.EOF", err)
		}
	})
	t.Run("a line of input reads one line at a time, an empty one included", func(t *testing.T) {
		term := build(t, lines)
		saysEach(t, "Line", lines, func() (string, error) { return term.Line(t.Context()) })
	})
	t.Run("a pick with nothing to choose from picks nothing", func(t *testing.T) {
		if v, ok := build(t, nil).Pick("What now?", nil, "gragas <command>"); ok || v != "" {
			t.Errorf("Pick with no choice = %q, %v, want nothing picked", v, ok)
		}
		_ = build(t, nil).CanAsk()
	})
	t.Run("a line at the end of input is io.EOF", func(t *testing.T) {
		if _, err := build(t, nil).Line(t.Context()); !errors.Is(err, io.EOF) {
			t.Errorf("Line at the end = %v, want io.EOF", err)
		}
	})
	terminalShow(t, build)
	t.Run("a line out takes text with a control sequence in it", func(t *testing.T) {
		if err := build(t, nil).Say("a \x1b[31mred\x1b[0m word"); err != nil {
			t.Errorf("Say = %v", err)
		}
	})
	t.Run("a question whose context ended is its error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := build(t, lines).Ask(ctx, "ok?"); !errors.Is(err, context.Canceled) {
			t.Errorf("Ask on an ended context = %v, want context.Canceled", err)
		}
	})
}

// TreeFiles is the files the Tree suite seeds, by slash path.
func TreeFiles() map[string]string {
	return map[string]string{
		"README.md":    "hello\n",
		"empty":        "",
		"rules/a.yaml": "a: 1\n",
		"deep/x/y.txt": "y",
	}
}

// TreeMaker makes the Tree under test: the port, the folder to Open, and the
// files outside every root. files is what the folder holds.
type TreeMaker func(t *testing.T, files map[string]string) (tree port.Tree, dir string, outside Outside)

// Outside is the files a TreeMaker plants outside every root, each by its
// absolute path in this system's form.
type Outside struct {
	// File holds "outside\n", and this account owns it and no other can
	// write it, so ReadOwned reads it.
	File string
	// Refused is a file the side planted as one another account owns or can
	// write, so ReadOwned refuses it: the adapter's test loosens it through
	// go-files' trusttest, the fake plants its refusal.
	Refused string
}

// Tree runs the Tree suite against the port build answers.
func Tree(t *testing.T, build TreeMaker) {
	t.Helper()
	files := TreeFiles()
	open := opener(func(t *testing.T) (port.Tree, model.Root, string, Outside) {
		t.Helper()
		tr, dir, outside := build(t, files)
		root, err := tr.Open(dir)
		if err != nil {
			t.Fatalf("Open(%q) = %v", dir, err)
		}
		return tr, root, dir, outside
	})
	treeReading(t, open, files)
	treeStat(t, open)
	treeListing(t, open)
	treeWithin(t, open)
	treeWritten(t, open)
	treeOpenIn(t, open)
	treeView(t, open, files)
}

// rowsOf runs each row as a subtest of its name, on a tree of its own.
func rowsOf(t *testing.T, open opener, rows []treeRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			tr, root, _, _ := open(t)
			r.run(t, tr, root)
		})
	}
}

// treeRow is one row of a Tree method: its name and what it holds of a tree
// just opened at root.
type treeRow struct {
	name string
	run  func(t *testing.T, tr port.Tree, root model.Root)
}

// treeOpenIn is OpenIn's rows: a folder in a root opens as a
// root of its own, the same each time; an empty dir is the root; a dir that
// leaves the root by .. is ErrOutside, from a root OpenIn opened too; a
// missing one is fs.ErrNotExist and a file is refused. A link that leads out
// is the Links suite's row.
func treeOpenIn(t *testing.T, open opener) {
	t.Helper()
	rowsOf(t, open, []treeRow{
		{"a folder in a root opens as a root of its own, the same each time", openInOwnRoot},
		{"an empty dir is the root itself", openInEmpty},
		{"a dir that leaves the root is ErrOutside", openInOutside},
		{"a missing folder is fs.ErrNotExist, and a file is no folder", openInMissingOrFile},
	})
}

func openInOwnRoot(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	for dir, file := range map[string]string{"rules": "a.yaml", "deep/x": "y.txt"} {
		sub, err := tr.OpenIn(root, dir)
		if err != nil || sub == root {
			t.Errorf("OpenIn(%q) = %q, %v, want a root of its own", dir, sub, err)
			continue
		}
		holds(t, tr, sub, file, TreeFiles()[dir+"/"+file])
		if again, err := tr.OpenIn(root, dir); err != nil || again != sub {
			t.Errorf("OpenIn(%q) again = %q, %v, want %q", dir, again, err, sub)
		}
	}
}

func openInEmpty(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	if got, err := tr.OpenIn(root, ""); err != nil || got != root {
		t.Errorf("OpenIn(\"\") = %q, %v, want %q", got, err, root)
	}
}

func openInOutside(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	openInRefused(t, tr, root, table(t, "read-outside"))
	sub, err := tr.OpenIn(root, "deep/x")
	if err != nil {
		t.Fatalf("OpenIn(deep/x) = %v", err)
	}
	openInRefused(t, tr, sub, []string{"..", "../../rules", "../x"})
}

// openInRefused fails the test unless OpenIn of each dir in root matches
// ErrOutside.
func openInRefused(t *testing.T, tr port.Tree, root model.Root, dirs []string) {
	t.Helper()
	for _, p := range dirs {
		if got, err := tr.OpenIn(root, p); !errors.Is(err, port.ErrOutside) {
			t.Errorf("OpenIn(%q) in %s = %q, %v, want ErrOutside", p, root, got, err)
		}
	}
}

func openInMissingOrFile(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	for _, p := range []string{"nope", "rules/nope/deeper"} {
		if got, err := tr.OpenIn(root, p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("OpenIn(%q) = %q, %v, want fs.ErrNotExist", p, got, err)
		}
	}
	for _, p := range []string{"README.md", "empty", "rules/a.yaml"} {
		if got, err := tr.OpenIn(root, p); err == nil || errors.Is(err, fs.ErrNotExist) {
			t.Errorf("OpenIn(%q), a file, = %q, %v, want refused as no folder", p, got, err)
		}
	}
}

// treeView is FS's rows: the view holds the root's files,
// passes testing/fstest's own suite, shows what List shows, and refuses a
// path that leaves the root, from a root OpenIn opened too. A link that
// leads out is the Links suite's row; a root that was not opened is the
// adapter's own test, since the fake fails a test on it.
func treeView(t *testing.T, open opener, files map[string]string) {
	t.Helper()
	rowsOf(t, open, []treeRow{
		{"the view holds the root's files and passes fstest.TestFS", func(t *testing.T, tr port.Tree, root model.Root) {
			t.Helper()
			viewHolds(t, viewOf(t, tr, root), files)
		}},
		{"the view shows what List shows", viewLists},
		{"a path in the view that leaves the root is refused", viewOutside},
		{"a file opened in the view reads no more than it is asked, and says its size first", viewReadsAsked},
		{"a name in the view that is not a regular file opens, and its handle says so", viewNotRegular},
	})
}

// viewReadsAsked is what the audit's capped read leans on: the
// handle's Stat answers the file's size before any read, and a Read of n
// bytes answers at most n, so a reader that asks for no more than a cap is
// never handed more.
func viewReadsAsked(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	f, err := viewOf(t, tr, root).Open("README.md")
	if err != nil {
		t.Fatalf("Open(README.md) in the view = %v", err)
	}
	closed(t, f, "README.md")
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len("hello\n")) {
		t.Fatalf("Stat of README.md before a read = %v, %v, want a regular file of %d bytes", info, err, len("hello\n"))
	}
	buf := make([]byte, 8)
	n, err := f.Read(buf[:2])
	if n > 2 || err != nil || string(buf[:n]) != "he"[:n] {
		t.Errorf("Read of 2 bytes = %d %q, %v, want at most 2 of %q", n, buf[:n], err, "he")
	}
	rest, err := io.ReadAll(f)
	if err != nil || string(buf[:n])+string(rest) != "hello\n" {
		t.Errorf("the rest after a Read of %d = %q, %v, want the file", n, rest, err)
	}
}

// viewNotRegular holds the other half: a name that is a folder opens through
// the view at once, and its handle's Stat says it is no regular file, so the
// audit refuses it from the handle before it reads. A FIFO or a device is the
// same through the adapter, whose Open is go-files confine.Open, following no
// link and never waiting on a FIFO (go-files'
// TestOpenNeverWaitsOnAPipe plants the FIFO, which needs a call of one
// system); the fake's view holds files and the folders above them only.
func viewNotRegular(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	f, err := viewOf(t, tr, root).Open("rules")
	if err != nil {
		t.Fatalf("Open(rules), a folder, in the view = %v", err)
	}
	closed(t, f, "rules")
	info, err := f.Stat()
	if err != nil || info.Mode().IsRegular() || !info.IsDir() {
		t.Errorf("Stat of the folder rules = %v, %v, want a folder and no regular file", info, err)
	}
}

// closed closes f, opened in a view at name, when the test ends, and fails
// the test when the close does.
func closed(t *testing.T, f fs.File, name string) {
	t.Helper()
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("Close of %s in the view = %v", name, err)
		}
	})
}

// viewOf is the view of root, or the test stops.
func viewOf(t *testing.T, tr port.Tree, root model.Root) fs.FS {
	t.Helper()
	view, err := tr.FS(root)
	if err != nil {
		t.Fatalf("FS(%s) = %v", root, err)
	}
	return view
}

func viewHolds(t *testing.T, view fs.FS, files map[string]string) {
	t.Helper()
	names := slices.Sorted(maps.Keys(files))
	for _, p := range names {
		if got, err := fs.ReadFile(view, p); err != nil || string(got) != files[p] {
			t.Errorf("ReadFile(%q) in the view = %q, %v, want %q", p, got, err, files[p])
		}
	}
	if err := fstest.TestFS(view, names...); err != nil {
		t.Errorf("the view fails fstest.TestFS: %v", err)
	}
}

func viewLists(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	view := viewOf(t, tr, root)
	listed, err := tr.List(root, "", model.Every)
	if err != nil {
		t.Fatalf("List = %v", err)
	}
	var want, got []string
	for _, e := range listed {
		want = append(want, e.Path)
	}
	err = fs.WalkDir(view, ".", func(p string, _ fs.DirEntry, err error) error {
		if p != "." {
			got = append(got, p)
		}
		return err
	})
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("the view walks %v, %v; List answers %v", got, err, want)
	}
}

func viewOutside(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	viewRefuses(t, viewOf(t, tr, root), table(t, "read-outside"))
	sub, err := tr.OpenIn(root, "rules")
	if err != nil {
		t.Fatalf("OpenIn(rules) = %v", err)
	}
	inner := viewOf(t, tr, sub)
	viewRefuses(t, inner, []string{"../README.md"})
	if _, err := fs.ReadFile(inner, "a.yaml"); err != nil {
		t.Errorf("the control: ReadFile(a.yaml) in the view of rules = %v", err)
	}
}

// viewRefuses fails the test unless each path reads as an error in view.
func viewRefuses(t *testing.T, view fs.FS, paths []string) {
	t.Helper()
	for _, p := range paths {
		if got, err := fs.ReadFile(view, p); err == nil {
			t.Errorf("ReadFile(%q) in the view = %q, want refused", p, got)
		}
	}
}

// opener makes the tree of one subtest and answers it, its root, its folder and
// the files outside every root.
type opener func(t *testing.T) (port.Tree, model.Root, string, Outside)

func treeReading(t *testing.T, open opener, files map[string]string) {
	t.Helper()
	t.Run("a folder opens as the same root each time", func(t *testing.T) {
		tr, root, dir, _ := open(t)
		if again, err := tr.Open(dir); err != nil || again != root {
			t.Errorf("Open again = %q, %v, want %q", again, err, root)
		}
	})
	t.Run("a file reads whole, an empty one as no bytes", func(t *testing.T) {
		tr, root, _, _ := open(t)
		for p, want := range files {
			if got, err := tr.Read(root, p); err != nil || string(got) != want {
				t.Errorf("Read(%q) = %q, %v, want %q", p, got, err, want)
			}
		}
	})
	t.Run("a missing file is fs.ErrNotExist", func(t *testing.T) {
		tr, root, _, _ := open(t)
		if _, err := tr.Read(root, "nope.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Read of a missing file = %v, want fs.ErrNotExist", err)
		}
	})
	t.Run("a path that leaves the root is ErrOutside", func(t *testing.T) {
		tr, root, _, _ := open(t)
		for _, p := range table(t, "read-outside") {
			if _, err := tr.Read(root, p); !errors.Is(err, port.ErrOutside) {
				t.Errorf("Read(%q) = %v, want ErrOutside", p, err)
			}
		}
	})
	t.Run("a folder is no file to read", func(t *testing.T) {
		tr, root, _, _ := open(t)
		if _, err := tr.Read(root, "rules"); err == nil {
			t.Error("Read of a folder succeeded")
		}
	})
}

func treeStat(t *testing.T, open opener) {
	t.Helper()
	t.Run("a stat tells a file's size and a folder", func(t *testing.T) {
		tr, root, _, _ := open(t)
		if e, err := tr.Stat(root, "README.md"); err != nil || e.Dir || e.Size != 6 || e.Path != "README.md" {
			t.Errorf("Stat(file) = %+v, %v", e, err)
		}
		if e, err := tr.Stat(root, "rules"); err != nil || !e.Dir {
			t.Errorf("Stat(folder) = %+v, %v", e, err)
		}
		if e, err := tr.Stat(root, ""); err != nil || !e.Dir {
			t.Errorf("Stat(root) = %+v, %v", e, err)
		}
		if _, err := tr.Stat(root, "nope.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat of a missing path = %v, want fs.ErrNotExist", err)
		}
	})
}

func treeListing(t *testing.T, open opener) {
	t.Helper()
	t.Run("a listing is every file and folder below, sorted by path", func(t *testing.T) {
		tr, root, _, _ := open(t)
		got, err := tr.List(root, "", model.Every)
		if err != nil {
			t.Fatal(err)
		}
		var paths []string
		for _, e := range got {
			paths = append(paths, e.Path)
		}
		want := []string{"README.md", "deep", "deep/x", "deep/x/y.txt", "empty", "rules", "rules/a.yaml"}
		if !slices.Equal(paths, want) {
			t.Errorf("List = %v, want %v", paths, want)
		}
	})
	t.Run("a listing of a folder is what is below it", func(t *testing.T) {
		tr, root, _, _ := open(t)
		got, err := tr.List(root, "rules", model.Every)
		if err != nil || len(got) != 1 || got[0].Path != "rules/a.yaml" {
			t.Errorf("List(rules) = %+v, %v, want rules/a.yaml", got, err)
		}
	})
	t.Run("a folder that is not there is an empty list, never an error", func(t *testing.T) {
		tr, root, _, _ := open(t)
		for _, dir := range []string{"no-such", "no-such/deeper", "rules/no-such"} {
			if got, err := tr.List(root, dir, model.Every); err != nil || len(got) != 0 {
				t.Errorf("List(%q) = %+v, %v, want an empty list and no error", dir, got, err)
			}
		}
	})
	t.Run("the nearest folder above with the marker is the root", func(t *testing.T) {
		tr, root, dir, _ := open(t)
		if got, err := tr.Locate(dir+"/deep/x", "README.md"); err != nil || got != root {
			t.Errorf("Locate = %q, %v, want %q", got, err, root)
		}
		if _, err := tr.Locate(dir+"/deep/x", "no-such-marker"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Locate of no marker = %v, want fs.ErrNotExist", err)
		}
	})
}

// OwnedMaker makes the Owned under test and plants the files outside every
// root the suite reads: File, which this account owns and no other can write,
// and Refused, which another account owns or can write.
type OwnedMaker func(t *testing.T) (owned port.Owned, outside Outside)

// Owned runs the Owned suite against the port build answers: the whole read
// of a file outside every root, and the refusals of it (taken
// out of the Tree suite, which it was one subject too many for).
func Owned(t *testing.T, build OwnedMaker) {
	t.Helper()
	t.Run("a file outside every root is read owned only when no other account owns or can write it", func(t *testing.T) {
		tr, outside := build(t)
		if outside.Refused == "" {
			t.Fatal("the side planted no refused file, so the owned read's refusal is not judged")
		}
		if got, err := tr.ReadOwned(outside.File); err != nil || string(got) != "outside\n" {
			t.Errorf("ReadOwned = %q, %v", got, err)
		}
		rel := workingFile(t)
		if _, err := tr.ReadOwned(rel); err == nil {
			t.Errorf("the relative path %q to a file that exists was read owned", rel)
		}
		if _, err := tr.ReadOwned(""); err == nil {
			t.Error("the empty path was read owned")
		}
		if _, err := tr.ReadOwned(outside.File + ".missing"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ReadOwned of a missing file = %v, want fs.ErrNotExist", err)
		}
		got, err := tr.ReadOwned(outside.Refused)
		if !errors.Is(err, port.ErrNotOwned) || got != nil {
			t.Fatalf("ReadOwned of the refused file = %q, %v; want nothing and port.ErrNotOwned", got, err)
		}
		r, ok := errors.AsType[refuser](err)
		if !ok {
			t.Fatalf("the refusal %v says no why and fix through Refusal()", err)
		}
		if why, _ := r.Refusal(); why == "" {
			t.Errorf("the refusal %v says no why", err)
		}
	})
}

// workingFile is the name of a regular file of the working folder, the first
// the folder lists: a relative path to a file that exists, made with no path
// from one folder to another, so no drive or folder of the system is read.
func workingFile(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Type().IsRegular() {
			return e.Name()
		}
	}
	t.Fatal("the working folder holds no regular file to name by a relative path")
	return ""
}

// saysEach calls next once per line and fails the test for each answer that is
// not the line, or that errs.
func saysEach(t *testing.T, name string, lines []string, next func() (string, error)) {
	t.Helper()
	for _, want := range lines {
		if got, err := next(); err != nil || got != want {
			t.Errorf("%s = %q, %v, want %q", name, got, err, want)
		}
	}
}

// EveryBlock is a screen with one of each block and a closing word, for a
// suite or a golden to draw.
func EveryBlock(t *testing.T) model.Screen {
	t.Helper()
	return everyBlock(t)
}

func everyBlock(t *testing.T) model.Screen {
	t.Helper()
	c, err := model.NewClosing(model.Declined, "nothing was changed", "gragas gen --apply")
	if err != nil {
		t.Fatal(err)
	}
	s, err := model.NewScreen(
		model.Heading{Text: "gen"}, model.Reads{What: "rulebook/ in the project"},
		model.Table{Columns: []model.Column{{Title: "rule", Priority: 3}, {Title: "status", Priority: 1}},
			Rows: [][]string{{"cli.look.5", "active"}, {"arch.ports.6", "active"}}},
		model.List{Items: []string{"one", "two"}}, model.Note{Text: "a note"},
		model.DryRunFooter{Flag: "--apply"}, c)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Identifier is the identity method of History, the part of the port M1
// builds; the rest of History has its suite when its adapter does (M2-01).
type Identifier interface {
	Who(ctx context.Context, root model.Root) (model.Identity, error)
}

// WhoMaker answers an Identifier and the root of a repository that commits
// as who, or that has no identity configured when who is nil.
type WhoMaker func(t *testing.T, who *model.Identity) (Identifier, model.Root)

// TheIdentity is the identity the Who suite configures: a literal, so a
// suite never compares an adapter with itself.
func TheIdentity() model.Identity {
	return model.Identity{Name: "Ada Example", Email: "ada@example.com"}
}

// Who runs the Who suite: the identity a repository commits as, and none
// configured, which is an error and never an empty identity.
func Who(t *testing.T, build WhoMaker) {
	t.Helper()
	t.Run("the configured identity is answered as it is", func(t *testing.T) {
		want := TheIdentity()
		h, root := build(t, &want)
		got, err := h.Who(t.Context(), root)
		if err != nil || got != (model.Identity{Name: "Ada Example", Email: "ada@example.com"}) {
			t.Errorf("Who = %+v, %v; want Ada Example <ada@example.com>", got, err)
		}
	})
	t.Run("no identity configured is an error, never an empty identity", func(t *testing.T) {
		h, root := build(t, nil)
		got, err := h.Who(t.Context(), root)
		if err == nil {
			t.Errorf("Who with none configured = %+v and no error", got)
		}
		if got != (model.Identity{}) {
			t.Errorf("Who with none configured answered %+v beside its error", got)
		}
	})
}

// treeWithin holds Within: a host path below the folder answers its path in
// the root, a new file's too; a path that is not clean is ErrNotClean,
// one that climbs out of the root included; a path that names the file
// outside every root is ErrOutside; a relative one is refused.
func treeWithin(t *testing.T, open opener) {
	t.Helper()
	treePlacing(t, open, "Within", port.Tree.Within, map[string]string{
		"/rules/a.yaml":   "rules/a.yaml",
		"/rules/new.yaml": "rules/new.yaml",
		"/deep/new/z.txt": "deep/new/z.txt",
	})
}

// treeWritten holds Written: a host path below the folder answers its path
// in the root as written, each part in the spelling the path gives it, a
// spelling the folder does not hold included; a path that is not clean is
// ErrNotClean, as for Within, one that climbs out of the root included; a
// path that names the file outside every root is ErrOutside; a relative one
// is refused.
func treeWritten(t *testing.T, open opener) {
	t.Helper()
	treePlacing(t, open, "Written", port.Tree.Written, map[string]string{
		"/rules/a.yaml":   "rules/a.yaml",
		"/Rules/A.yaml":   "Rules/A.yaml",
		"/deep/new/z.txt": "deep/new/z.txt",
	})
}

// treeNotClean holds that a method that places a host path refuses one that
// is not clean with ErrNotClean, one that climbs out of the root included.
func treeNotClean(t *testing.T, open opener, name string, place func(port.Tree, model.Root, string) (string, error)) {
	t.Helper()
	t.Run(name+": a path that is not clean is ErrNotClean", func(t *testing.T) {
		tr, root, dir, _ := open(t)
		for _, below := range []string{"/deep/x/../../rules/a.yaml", "/rules/./a.yaml", "/rules//a.yaml", "/rules/", "/../x", "/rules/../../x"} {
			if got, err := place(tr, root, dir+below); !errors.Is(err, port.ErrNotClean) {
				t.Errorf("%s(%q) = %q, %v, want ErrNotClean", name, dir+below, got, err)
			}
		}
	})
}

// treePlacing holds a method that places a host path in a root: each path
// below the folder, by its part after the folder, answers want's path; a path
// that is not clean is ErrNotClean, one that climbs out of the root included;
// the file outside every root is ErrOutside; a
// relative or empty path is refused.
func treePlacing(t *testing.T, open opener, name string, place func(port.Tree, model.Root, string) (string, error), want map[string]string) {
	t.Helper()
	t.Run(name+": a path in the folder is its path in the root", func(t *testing.T) {
		tr, root, dir, _ := open(t)
		for below, rel := range want {
			if got, err := place(tr, root, dir+below); err != nil || got != rel {
				t.Errorf("%s(%q) = %q, %v, want %q", name, dir+below, got, err, rel)
			}
		}
	})
	treeNotClean(t, open, name, place)
	t.Run(name+": the file outside every root is ErrOutside", func(t *testing.T) {
		tr, root, _, outside := open(t)
		for _, abs := range []string{outside.File} {
			if got, err := place(tr, root, abs); !errors.Is(err, port.ErrOutside) {
				t.Errorf("%s(%q) = %q, %v, want ErrOutside", name, abs, got, err)
			}
		}
	})
	t.Run(name+": a relative or empty path is refused", func(t *testing.T) {
		tr, root, _, _ := open(t)
		for _, abs := range []string{"", "rules/a.yaml", "./rules/a.yaml"} {
			if got, err := place(tr, root, abs); err == nil {
				t.Errorf("%s(%q) = %q, want an error", name, abs, got)
			}
		}
	})
}

// refuser is the refusal of an owned read: why it is refused and the line
// that fixes it.
type refuser interface {
	error
	Refusal() (why, fix string)
}
