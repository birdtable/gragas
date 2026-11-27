package contracttest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// LinkFiles are what the folder holds when the Links suite starts, beside
// its links: a folder standing for a repository's own, and a file beside.
func LinkFiles() map[string]string {
	return map[string]string{
		"real/HEAD":   "head\n",
		"real/config": "config\n",
		"deep/a.txt":  "a\n",
		"beside.txt":  "beside\n",
	}
}

// LinkPlants are the links the Links suite plants, each path to its target,
// slash-separated and relative to the link's own folder: inside the root, a
// folder, a file, a target that is missing, and a link below a folder that
// is real; and out, a folder beside the root that BeyondFiles fills.
func LinkPlants() map[string]string {
	return map[string]string{
		"state":         "real",
		"history.jsonl": "real/config",
		"dangling":      "real/gone",
		"deep/via":      "../real",
		"out":           "../" + Beyond,
	}
}

// Beyond is the folder beside the root, the one the link "out" leads to.
const Beyond = "beyond"

// BeyondFiles are what Beyond holds. A maker on disk plants them there, next
// to the folder it answers, so a refusal through the link is of a file that
// is there; the fake world holds no file outside its roots and plants none.
func BeyondFiles() map[string]string {
	return map[string]string{"secret": "outside\n"}
}

// LinksMaker makes the Tree and the Writer under test over a folder holding
// files and links, as LinkFiles and LinkPlants give them: the Tree, which
// opened nothing yet, the Writer that writes in its roots, and the folder.
type LinksMaker func(t *testing.T, files, links map[string]string) (tree port.Tree, w port.Writer, dir string)

// Links runs the suite of links inside a root: a link at any
// component of a path, the last included, is refused for Replace, Append,
// Remove and Read, matching port.ErrOutside and naming the component, and what it
// leads to is left as it was. A link on no path written or read stops
// nothing. Within answers a path whose last part is a link that leads inside
// with port.ErrLinked and places it nowhere; that the error
// names where the link leads is held by the guard's own tests, which read
// the refusal, not by this suite, which reads no error's text (go.errors.8).
func Links(t *testing.T, build LinksMaker) {
	t.Helper()
	open := func(t *testing.T) (port.Tree, port.Writer, model.Root) {
		t.Helper()
		tr, w, dir := build(t, LinkFiles(), LinkPlants())
		root, err := tr.Open(dir)
		if err != nil {
			t.Fatalf("Open(%q) = %v", dir, err)
		}
		return tr, w, root
	}
	cases := []struct{ name, path, link string }{
		{"a link as a middle component", "state/HEAD", "state"},
		{"a link as the last component", "history.jsonl", "history.jsonl"},
		{"a link to a target that is missing", "dangling", "dangling"},
		{"a link below a real folder", "deep/via/config", "deep/via"},
	}
	for _, c := range cases {
		t.Run(c.name+" is refused for Replace, Append, Remove and Read", func(t *testing.T) {
			tr, w, root := open(t)
			refusedAt(t, "Read", c.path, c.link, func() error { _, err := tr.Read(root, c.path); return err })
			refusedAt(t, "Replace", c.path, c.link, func() error { return w.Replace(root, c.path, []byte("x\n")) })
			refusedAt(t, "Append", c.path, c.link, func() error { return w.Append(root, c.path, []byte("x")) })
			refusedAt(t, "Remove", c.path, c.link, func() error { return w.Remove(root, c.path) })
			for p, want := range LinkFiles() {
				holds(t, tr, root, p, want)
			}
			if _, err := tr.Read(root, "real/gone"); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("a refused write made the link's missing target: Read(real/gone) = %v", err)
			}
		})
	}
	t.Run("Within: a path whose last part is a link that leads inside is ErrLinked", func(t *testing.T) { linkedLast(t, build) })
	t.Run("the control: the same files by their real paths, and a link beside, write and read", func(t *testing.T) {
		tr, w, root := open(t)
		if err := w.Replace(root, "real/HEAD", []byte("new\n")); err != nil {
			t.Errorf("Replace(real/HEAD) = %v", err)
		}
		if err := w.Append(root, "real/config", []byte("more")); err != nil {
			t.Errorf("Append(real/config) = %v", err)
		}
		if err := w.Replace(root, "deep/fresh/b.txt", []byte("b\n")); err != nil {
			t.Errorf("Replace(deep/fresh/b.txt) = %v", err)
		}
		if err := w.Append(root, "beside.txt", []byte("more")); err != nil {
			t.Errorf("Append(beside.txt) = %v", err)
		}
		holds(t, tr, root, "real/HEAD", "new\n")
		holds(t, tr, root, "real/config", "config\nmore\n")
		holds(t, tr, root, "deep/fresh/b.txt", "b\n")
		holds(t, tr, root, "deep/a.txt", "a\n")
		holds(t, tr, root, "beside.txt", "beside\nmore\n")
	})
	linksOpened(t, open)
}

// linksOpened is the rows of OpenIn and FS over links: a folder
// reached through a link that stays inside opens as the folder it leads to,
// and one through a link that leads out is ErrOutside. The view opens a file
// as Read does, so a link at any step, inside or out, is refused there.
func linksOpened(t *testing.T, open func(t *testing.T) (port.Tree, port.Writer, model.Root)) {
	t.Helper()
	rows := []treeRow{
		{"a folder through a link inside opens as the folder it leads to", openInThroughLink},
		{"a folder through a link that leads out is ErrOutside", func(t *testing.T, tr port.Tree, root model.Root) {
			t.Helper()
			openInRefused(t, tr, root, []string{"out", "out/deeper"})
		}},
		{"a file through a link at any step, inside or out, is refused in the view", viewThroughLink},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			tr, _, root := open(t)
			r.run(t, tr, root)
		})
	}
}

func openInThroughLink(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	real, err := tr.OpenIn(root, "real")
	if err != nil {
		t.Fatalf("OpenIn(real) = %v", err)
	}
	for _, dir := range []string{"state", "deep/via"} {
		if got, err := tr.OpenIn(root, dir); err != nil || got != real {
			t.Errorf("OpenIn(%q) = %q, %v, want %q, the root of real", dir, got, err, real)
		}
	}
	holds(t, tr, real, "HEAD", "head\n")
}

func viewThroughLink(t *testing.T, tr port.Tree, root model.Root) {
	t.Helper()
	view := viewOf(t, tr, root)
	through := []string{"state/HEAD", "history.jsonl", "deep/via/config"}
	for p := range BeyondFiles() {
		through = append(through, "out/"+p)
	}
	viewRefuses(t, view, through)
	for p, want := range LinkFiles() {
		if got, err := fs.ReadFile(view, p); err != nil || string(got) != want {
			t.Errorf("the control: ReadFile(%q) in the view = %q, %v, want %q", p, got, err, want)
		}
	}
}

// refusedAt fails the test unless do's error matches port.ErrOutside and
// holds an *fs.PathError naming link as the component that is a link.
func refusedAt(t *testing.T, method, path, link string, do func() error) {
	t.Helper()
	err := do()
	if !errors.Is(err, port.ErrOutside) {
		t.Errorf("%s(%q) = %v, want ErrOutside", method, path, err)
		return
	}
	if at, ok := errors.AsType[*fs.PathError](err); !ok || at.Path != link {
		t.Errorf("%s(%q) = %v, want an *fs.PathError naming %s as the link", method, path, err, link)
	}
}

// linkedLast: Within answers a path whose last part is a link that leads
// inside with port.ErrLinked and no path, and places the real path.
func linkedLast(t *testing.T, build LinksMaker) {
	t.Helper()
	tr, _, dir := build(t, LinkFiles(), LinkPlants())
	root, err := tr.Open(dir)
	if err != nil {
		t.Fatalf("Open(%q) = %v", dir, err)
	}
	for _, below := range []string{"/history.jsonl", "/state"} {
		got, err := tr.Within(root, dir+below)
		if !errors.Is(err, port.ErrLinked) || got != "" {
			t.Errorf("Within(%q) = %q, %v, want ErrLinked and no path", dir+below, got, err)
		}
	}
	if got, err := tr.Within(root, dir+"/real/config"); err != nil || got != "real/config" {
		t.Errorf("the control: Within(%q) = %q, %v, want real/config", dir+"/real/config", got, err)
	}
}
