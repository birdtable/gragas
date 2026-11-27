// Package assets is the Assets port's adapter: the files embedded in the
// program, read by path and listed by prefix.
//
// The engine's own data folders sit beside this file, because go:embed reads
// only below its package: handbook/, profiles/, fixture/,
// contract/schema/, contract/fixture/ (a generated copy of the repository's
// contract/fixture/), format/schema/ and project/schema/ (the pin's). A folder with nothing to embed yet
// holds a .placeholder file that says which milestone fills it, so the build
// never skips it silently; the adapter hides that file, and .gitkeep, from
// Read and List.
//
// The explorer is the marked copy that `make explorer-copy` writes into
// explorer-copy/ and nobody commits. Without it, the adapter answers the
// placeholder page of explorer-placeholder/, which is data and carries
// PlaceholderMarker for the release recipe to refuse. Both answer under the
// path explorer/.
package assets

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/port"
)

//go:embed all:audit all:handbook all:profiles all:fixture all:contract/schema all:contract/fixture all:format/schema all:project/schema all:explorer-placeholder all:explorer-copy
var tree embed.FS

// PlaceholderMarker is the text the placeholder page carries and a built
// explorer copy never does: the release recipe refuses a binary holding it.
const PlaceholderMarker = "gragas:explorer-placeholder"

// ExplorerDir is the folder of the port's paths the explorer answers under.
const ExplorerDir = "explorer"

// Assets is the port over three folders of files.
type Assets struct {
	data     fs.FS
	explorer fs.FS
}

var _ port.Assets = Assets{}

// New is the adapter over the files embedded in this program.
func New() Assets {
	return NewFrom(tree, sub(tree, "explorer-placeholder"), sub(tree, "explorer-copy"))
}

// none is a folder that holds nothing.
type none struct{}

func (none) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// sub is the folder dir of f, which an embedded folder always has.
func sub(f fs.FS, dir string) fs.FS {
	s, err := fs.Sub(f, dir)
	if err != nil {
		return none{}
	}
	return s
}

// NewFrom is the adapter over given folders: data holds the engine's data
// folders, page the placeholder page, built the marked copy. The explorer
// answers the built copy when it holds a file, the page otherwise.
func NewFrom(data, page, built fs.FS) Assets {
	explorer := page
	if len(files(built)) > 0 {
		explorer = built
	}
	return Assets{data: data, explorer: explorer}
}

// Read answers the whole content of the file at p.
func (a Assets) Read(p string) ([]byte, error) {
	src, rel, ok := a.resolve(p)
	if !ok || isHidden(rel) {
		return nil, &fs.PathError{Op: "read", Path: p, Err: fs.ErrNotExist}
	}
	b, err := fs.ReadFile(src, rel)
	if err != nil {
		return nil, &fs.PathError{Op: "read", Path: p, Err: fs.ErrNotExist}
	}
	return b, nil
}

// List answers the paths of the files below dir, sorted.
func (a Assets) List(dir string) ([]string, error) {
	if dir == "." {
		dir = ""
	}
	if dir != "" && !fs.ValidPath(dir) {
		return nil, fmt.Errorf("list %q: %w", dir, fs.ErrInvalid)
	}
	var all []string
	for _, f := range files(a.data) {
		if !explorerFolder(f) {
			all = append(all, f)
		}
	}
	for _, f := range files(a.explorer) {
		all = append(all, path.Join(ExplorerDir, f))
	}
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	out := []string{}
	for _, f := range all {
		if strings.HasPrefix(f, prefix) {
			out = append(out, f)
		}
	}
	slices.Sort(out)
	return out, nil
}

// resolve is the folder that holds p and p inside it.
func (a Assets) resolve(p string) (fs.FS, string, bool) {
	if !fs.ValidPath(p) || p == "." {
		return nil, "", false
	}
	if rel, ok := strings.CutPrefix(p, ExplorerDir+"/"); ok {
		return a.explorer, rel, true
	}
	if explorerFolder(p) {
		return nil, "", false
	}
	return a.data, p, true
}

// isHidden is whether p names a file that only holds a folder in the tree.
func isHidden(p string) bool {
	switch path.Base(p) {
	case ".placeholder", ".gitkeep":
		return true
	}
	return false
}

// files are the paths of the regular files of root that are not hidden. A
// folder that cannot be walked holds none.
func files(root fs.FS) []string {
	var out []string
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && !isHidden(p) {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil
	}
	return out
}

// explorerFolder is whether p is below a folder of the embedded tree that is
// the explorer's, not data: those answer under explorer/ and never by their
// own names.
func explorerFolder(p string) bool {
	first, _, _ := strings.Cut(p, "/")
	return first == "explorer-placeholder" || first == "explorer-copy"
}
