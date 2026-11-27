package porttest_test

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// follows is the fake tree and writer with each path's links followed first,
// as os.Root follows a link that stays inside a root: the adapter host/fs
// once was. A planted adapter.
type follows struct {
	tree   port.Tree
	writer port.Writer
	links  map[string]string
}

// resolve rewrites p's first linked prefix to its target until none is left.
func (f follows) resolve(p string) string {
	for range 40 {
		parts := strings.Split(p, "/")
		hit := false
		for i := range parts {
			at := strings.Join(parts[:i+1], "/")
			if target, ok := f.links[at]; ok {
				p = path.Join(append([]string{path.Dir(at), target}, parts[i+1:]...)...)
				hit = true
				break
			}
		}
		if !hit {
			return p
		}
	}
	return p
}

func (f follows) Open(dir string) (model.Root, error) { return f.tree.Open(dir) }
func (f follows) Locate(start, marker string) (model.Root, error) {
	return f.tree.Locate(start, marker)
}
func (f follows) Read(r model.Root, p string) ([]byte, error) { return f.tree.Read(r, f.resolve(p)) }
func (f follows) ReadSoleWithin(r model.Root, p string, max int64) ([]byte, error) {
	return f.tree.ReadSoleWithin(r, f.resolve(p), max)
}
func (f follows) Stat(r model.Root, p string) (model.Entry, error) {
	return f.tree.Stat(r, f.resolve(p))
}
func (f follows) List(r model.Root, dir string, s model.Scope) ([]model.Entry, error) {
	return f.tree.List(r, dir, s)
}
func (f follows) Within(r model.Root, a string) (string, error)  { return f.tree.Within(r, a) }
func (f follows) Written(r model.Root, a string) (string, error) { return f.tree.Written(r, a) }
func (f follows) OpenIn(r model.Root, d string) (model.Root, error) {
	return f.tree.OpenIn(r, d)
}
func (f follows) FS(r model.Root) (fs.FS, error) { return f.tree.FS(r) }
func (f follows) Replace(r model.Root, p string, data []byte) error {
	return f.writer.Replace(r, f.resolve(p), data)
}
func (f follows) Append(r model.Root, p string, line []byte) error {
	return f.writer.Append(r, f.resolve(p), line)
}

// TestAnAdapterThatFollowsALinkInsideTheRootFailsTheLinksSuite: the plant
// follows each link to its target and writes there, as host/fs once
// did. The control is the fake, which refuses the link.
func TestAnAdapterThatFollowsALinkInsideTheRootFailsTheLinksSuite(t *testing.T) {
	t.Parallel()
	build := func(plant bool) contracttest.LinksMaker {
		return func(t *testing.T, files, links map[string]string) (port.Tree, port.Writer, string) {
			t.Helper()
			w := porttest.NewWorld(t)
			root := w.Seed("/work/tree", files)
			for p, target := range links {
				w.Link(root, p, target)
			}
			p := w.Ports()
			if plant {
				f := follows{tree: p.Tree, writer: p.Writer, links: links}
				return f, f, "/work/tree"
			}
			return p.Tree, p.Writer, "/work/tree"
		}
	}
	if passes(t, "follows a link", wants()["follows a link"], func(t *testing.T) {
		t.Helper()
		contracttest.Links(t, build(true))
	}) {
		t.Error("an adapter that follows a link inside the root passed the Links suite")
	}
	if !passes(t, control, "", func(t *testing.T) {
		t.Helper()
		contracttest.Links(t, build(false))
	}) {
		t.Error("the control, the fake that refuses a link, failed the Links suite")
	}
}

func (f follows) Remove(r model.Root, p string) error {
	return f.writer.Remove(r, f.resolve(p))
}
