package porttest

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// ErrUnseeded is what a call of the World answers, matching this, when the
// test did not seed what the call needs. The World also fails the test, naming
// the port and the method, so a call nobody planned for never passes quietly.
var ErrUnseeded = errors.New("the fake was not seeded for that call")

// Reporter is the part of *testing.T the World fails a test through: a test
// passes its own t, and a test of the fake passes a recorder.
type Reporter interface {
	Helper()
	Errorf(format string, args ...any)
}

// World is one in-memory world that fills all seventeen ports of internal/port
// (arch.ports.17). It serves only what a test seeded, answers every call
// it was not seeded for with ErrUnseeded after failing the test with the port
// and method named, and returns no constant: a commit's name comes from its
// content, a lock's time from the seeded clock. Every method is safe to call
// from several goroutines; the exported fields are read after the run.
//
// The five ports of the engine's first form are filled by Fake, until M6.
type World struct {
	t  Reporter
	mu sync.Mutex

	roots    map[model.Root]*tree
	outside  map[string]string
	refused  map[string]Refused
	keepIn   string
	writeErr map[string]error
	locks    map[lockKey]int64
	taken    map[lockKey]int64
	// freed is closed, and replaced, whenever another process's hold ends, to
	// wake a Wait.
	freed    chan struct{}
	journal  map[model.Root]*journal
	repos    map[model.Root]Repo
	releases map[string]map[string]map[string]string
	commits  map[model.Root]*commitLog
	tools    map[string]model.Found
	reach    map[model.Level]model.Level
	missing  map[model.Level]bool
	scratch  int
	schemas  map[string]bool
	plants   []plant
	clock    *clock
	assets   map[string]string
	term     *term
	trust    *trust
	steps    map[string]plugin.Result
	// caches is how many folders of a grant's caches the Steps keeps, which
	// KeepCaches sets and SweepCaches removes.
	caches  int
	bundles map[string]map[model.PluginRef]model.PluginBundle
	// plugins is the World's Plugins: a test installs into it through
	// PluginsOf.
	plugins Standin

	// Writes is every Replace, Append and Remove the Writer took, in order.
	Writes []Write
	// Commits is every commit Remote made, and Tags every tag.
	Commits []Commit
	Tags    []TagMade
	// Prepared is every Sandbox.Prepare, in order.
	Prepared []model.Prepare
	// Asked is every question Terminal.Ask wrote; Said is every line Say wrote.
	Asked, Said []string
	// Shown is every Screen Terminal.Show took, in order.
	Shown []model.Screen
	// Picked is the title and flag of every Terminal.Pick that was offered.
	Picked []string
	// Banners is the version of every Terminal.Banner, in order.
	Banners []string
}

// NewWorld is an empty World that fails t on a call it was not seeded for.
func NewWorld(t Reporter) *World {
	return &World{
		t: t, roots: map[model.Root]*tree{}, outside: map[string]string{}, refused: map[string]Refused{}, writeErr: map[string]error{},
		locks: map[lockKey]int64{}, taken: map[lockKey]int64{}, journal: map[model.Root]*journal{},
		repos: map[model.Root]Repo{}, releases: map[string]map[string]map[string]string{},
		commits: map[model.Root]*commitLog{}, tools: map[string]model.Found{},
		reach: map[model.Level]model.Level{}, missing: map[model.Level]bool{},
		schemas: map[string]bool{}, assets: map[string]string{},
	}
}

// Ports is w as every port, each one filled. The optional ports are filled
// too, as the port package says.
func (w *World) Ports() port.Ports {
	return port.Ports{
		Tree: treePort{w}, Writer: writerPort{w}, Lock: lockPort{w}, Journal: journalPort{w},
		History: historyPort{w}, Remote: remotePort{w}, Sandbox: sandboxPort{w},
		Schema: schemaPort{w}, Clock: clockPort{w}, Assets: assetsPort{w}, Terminal: terminalPort{w},
		Owned: ownedPort{w}, FileNames: fileNamesPort{}, Trust: trustPort{w}, Steps: stepsPort{w}, Bundles: bundlesPort{w},
		Plugins: &w.plugins,
	}
}

// unseeded fails the test, naming the port and method and what was missing,
// and answers the error to return. The caller holds w.mu.
func (w *World) unseeded(call, format string, args ...any) error {
	w.t.Helper()
	what := fmt.Sprintf(format, args...)
	w.t.Errorf("%s was not seeded: %s", call, what)
	return fmt.Errorf("%s: %s: %w", call, what, ErrUnseeded)
}

// tree is one seeded root: its files by slash path, and which are tracked.
type tree struct {
	files   map[string]string
	exec    map[string]bool
	tracked map[string]bool // nil when the test never said
	// links are the links in the root, each path to its target, which
	// Read, Replace and Append refuse at any component of their path.
	links map[string]string
	// names are the files with more than one name, by hard links: each
	// name to the one set of names its file has, which ReadSoleWithin refuses.
	names map[string]map[string]bool
}

// Seed makes dir a root holding files, by slash path relative to it, and
// answers the root. Seeding a dir again replaces it.
func (w *World) Seed(dir string, files map[string]string) model.Root {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seed(dir, files)
}

func (w *World) seed(dir string, files map[string]string) model.Root {
	r := model.Root(path.Clean(dir))
	t := &tree{files: map[string]string{}, exec: map[string]bool{}, links: map[string]string{},
		names: map[string]map[string]bool{}}
	maps.Copy(t.files, files)
	w.roots[r] = t
	return r
}

// Track says which files of root its history holds; the rest are untracked.
// Until a test says, a listing by scope is a call it did not seed.
func (w *World) Track(root model.Root, paths ...string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t := w.roots[root]; t != nil {
		t.tracked = map[string]bool{}
		for _, p := range paths {
			t.tracked[p] = true
		}
	}
}

// Executable marks files of root as executable by their owner.
func (w *World) Executable(root model.Root, paths ...string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t := w.roots[root]; t != nil {
		for _, p := range paths {
			t.exec[p] = true
		}
	}
}

// Link plants a link at p in root, leading to target, slash-separated and
// relative to the link's folder, as a repository can commit one. Read,
// Replace and Append refuse a path with a link at any component, as
// host/fs does; the fake never follows one.
func (w *World) Link(root model.Root, p, target string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t := w.roots[root]; t != nil {
		t.links[p] = target
	}
}

// Name gives the file at p in root a second name, as a hard link does,
// slash-separated and relative to root: second holds what p holds, and the
// two are one file. Read reads either; ReadSoleWithin refuses both, as host/fs
// does. An append through either name is refused, matching
// port.ErrOutside, as host/fs's open to append refuses a file with a second
// name, and a replace or a remove takes that one name out of the set, as
// host/fs's replace by rename and its remove do.
func (w *World) Name(root model.Root, p, second string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := w.roots[root]
	if t == nil {
		return
	}
	set := t.names[p]
	if set == nil {
		set = map[string]bool{p: true}
		t.names[p] = set
	}
	set[second] = true
	t.names[second] = set
	t.files[second] = t.files[p]
}

// unname takes p out of the set of names its file shares, as a file put in
// its place by rename, or its removal, does.
func (t *tree) unname(p string) {
	if set := t.names[p]; set != nil {
		delete(set, p)
		delete(t.names, p)
		if len(set) == 1 {
			for other := range set {
				delete(t.names, other)
			}
		}
	}
}

// linked refuses p, for op, when a component of it, the last included, is a
// link, with the error host/fs gives. It matches case exactly, so a fake
// run is no proof of a refusal on a file system that folds case; host/fs's
// own tests hold that.
func (t *tree) linked(op, p string) error {
	if p == "" {
		return nil
	}
	parts := strings.Split(p, "/")
	for i := range parts {
		at := strings.Join(parts[:i+1], "/")
		if _, ok := t.links[at]; ok {
			return fmt.Errorf("%s %s: %w", op, p, &fs.PathError{Op: "walk", Path: at,
				Err: fmt.Errorf("it is a link, and no link is followed on a path that is written or checked: %w", port.ErrOutside)})
		}
	}
	return nil
}

// SeedOutside makes the absolute path hold content for ReadOwned: a slash
// path, as the world spells its own, or this system's absolute path, as a
// test that reads it from the working folder needs.
func (w *World) SeedOutside(abs, content string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.outside[abs] = content
}

// Refused is a refusal SeedRefused plants on a file outside the roots: why
// it is refused and the line that fixes it, as the owned read says them, and
// the port's ErrOwned error of its reason, nil for a refusal of none.
type Refused struct {
	Why, Fix string
	Reason   error
}

// SeedRefused makes ReadOwned of the absolute path refuse it as a file
// another account owns or can write: the error matches port.ErrNotOwned,
// names the path as the adapter's does, and answers r through Refusal().
func (w *World) SeedRefused(abs string, r Refused) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.refused[abs] = r
}

// KeepCopiesIn makes KeptAt answer paths in dir, this system's absolute path,
// in place of the world's own /state: for a suite that holds the answer to
// this system's idea of absolute.
func (w *World) KeepCopiesIn(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.keepIn = dir
}

// File is the content of the file at p in root now, as the Writer left it,
// and whether it is there. A test reads it to see what a write did.
func (w *World) File(root model.Root, p string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t := w.roots[root]; t != nil {
		c, ok := t.files[p]
		return c, ok
	}
	return "", false
}

// rootOf is the seeded root, or fails the test for call.
func (w *World) rootOf(call string, r model.Root) (*tree, error) {
	if t := w.roots[r]; t != nil {
		return t, nil
	}
	return nil, w.unseeded(call, "the root %q was never seeded", r)
}

// inside is whether p is a path a root may name: clean, relative, never
// climbing, and with no backslash or colon, which Windows reads as a
// separator or a drive, as host/fs refuses them. "" is the root itself.
func inside(p string) error {
	if p == "" || p == "." {
		return nil
	}
	if strings.HasPrefix(p, "/") || slices.Contains(strings.Split(p, "/"), "..") || path.Clean(p) != p || strings.ContainsAny(p, `\:`) {
		return fmt.Errorf("%w: %q", port.ErrOutside, p)
	}
	return nil
}

func notExist(op, p string) error {
	return &fs.PathError{Op: op, Path: p, Err: fs.ErrNotExist}
}

// isDir is whether some file of t lies below p.
func (t *tree) isDir(p string) bool {
	if p == "" {
		return true
	}
	for f := range t.files {
		if strings.HasPrefix(f, p+"/") {
			return true
		}
	}
	return false
}

// MissingPorts is the fields of p that hold no port, by name.
func MissingPorts(p port.Ports) []string {
	var out []string
	v := reflect.ValueOf(p)
	for f := range reflect.TypeFor[port.Ports]().Fields() {
		if v.FieldByIndex(f.Index).IsNil() {
			out = append(out, f.Name)
		}
	}
	return out
}

// CountPorts is how many fields Ports has.
func CountPorts(p port.Ports) int { return reflect.TypeOf(p).NumField() }

// PluginsOf is the Plugins of w's ports, which a test installs plugins into.
func (w *World) PluginsOf() *Standin { return &w.plugins }
