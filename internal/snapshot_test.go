package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// The checks of this package read one tree, and it does not change while they
// run. The first test that asks for it reads it once (snap): what go list
// says of the module and of what the binaries link, and the Go files parsed.
// The tests read from it and never write to it; a test that plants a breach
// takes its own copy (world.clone), never the snapshot's own.
//
// The read is made from a test's body, never from TestMain before m.Run: Go
// records the files a test opens, the key of its test cache, only inside
// m.Run, so a file read before it is in no key, and a change to it would
// replay the package's checks from the cache (gragas#1222).

// theTree is the one read of the tree the package's tests share: once reads it
// into snap on the first call of snap(), and asked holds the go list runs and
// the file reads asked of the tree, by their key, each made when it is first
// asked.
var theTree = struct { //nolint:gochecknoglobals // the tree read once, from the body of the first test that asks; nothing writes it after
	once  sync.Once
	snap  *snapshot
	asked *answers
}{asked: &answers{held: map[string]*answer{}}}

// snap is the tree, read on the first call, from the body of the test that
// makes it, and the same snapshot for every call after.
func snap() *snapshot {
	theTree.once.Do(func() { theTree.snap = readTree() })
	return theTree.snap
}

type snapshot struct {
	// err is why the tree could not be read; every reader stops on it.
	err error
	// world is go list and the walk of the tree, as tree reads them.
	world world
	// asts are the Go files of the world parsed with their comments, by slash
	// path from the root, over fset.
	asts map[string]*ast.File
	fset *token.FileSet
}

// answers holds each go list run, each make -n and each file read of the tree once, by environment and arguments: the first
// caller runs it, any other waits for that answer.
type answers struct {
	mu   sync.Mutex
	held map[string]*answer
}

type answer struct {
	once sync.Once
	out  []byte
	err  error
	// runs is how many times go list ran for this key.
	runs atomic.Int32
}

// of is the answer to go list args with env added, run once.
func (a *answers) of(env, args []string, run func() ([]byte, error)) ([]byte, error) {
	key := answerKey(env, args)
	a.mu.Lock()
	r, ok := a.held[key]
	if !ok {
		r = &answer{}
		a.held[key] = r
	}
	a.mu.Unlock()
	r.once.Do(func() {
		r.runs.Add(1)
		r.out, r.err = run()
	})
	return r.out, r.err
}

// ran is how many times go list ran for env and args: once when it was asked,
// never when it was not.
func (a *answers) ran(env, args []string) int32 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r, ok := a.held[answerKey(env, args)]; ok {
		return r.runs.Load()
	}
	return 0
}

// answerKey names a go list run by its environment and arguments.
func answerKey(env, args []string) string {
	return strings.Join(env, "\x00") + "\x01" + strings.Join(args, "\x00")
}

// The go list runs of more than one test, named once so the snapshot asks
// what the tests ask.

// shippedArgs is the build the shipped binary is made from, what each package
// imports.
func shippedArgs() []string {
	return []string{"-e", "-deps", "-json=ImportPath,Imports", "./cmd/gragas"}
}

// devArgs is the same for gragas-dev.
func devArgs() []string {
	return []string{"-e", "-deps", "-json=ImportPath,Imports", "./cmd/gragas-dev"}
}

// modulesArgs is the external modules the shipped binary links.
func modulesArgs() []string { return []string{"-deps", "-json=Standard,Module", "./cmd/gragas"} }

// subsetArgs is what the subset's binary links and embeds.
func subsetArgs() []string { return buildArgs("./cmd/gragas-subset") }

// allArgs is every package of the module, by name.
func allArgs() []string { return []string{"./..."} }

// buildArgs is go list -deps of the main package main, with what each package
// imports and embeds.
func buildArgs(main string) []string {
	return []string{"-e", "-deps", "-json=ImportPath,Imports,EmbedFiles", main}
}

// readTree reads the tree once: the go list runs at the same time, then the
// world from them, and the Go files parsed. snap calls it, once.
func readTree() *snapshot {
	s := &snapshot{fset: token.NewFileSet(), asts: map[string]*ast.File{}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ask := func(env []string, args []string) {
		wg.Go(func() {
			if _, err := goOut(env, args...); err != nil {
				mu.Lock()
				s.err = errors.Join(s.err, err)
				mu.Unlock()
			}
		})
	}
	systemsList, err := systems()
	if err != nil {
		s.err = err
		return s
	}
	runs := [][]string{nil}
	for _, sys := range systemsList {
		goos, goarch, _ := strings.Cut(sys, "/")
		runs = append(runs, []string{"GOOS=" + goos, "GOARCH=" + goarch, "CGO_ENABLED=0"})
	}
	for _, env := range runs {
		ask(env, listArgs())
	}
	for _, args := range [][]string{testListArgs(), allArgs(), shippedArgs(), devArgs(), modulesArgs(), subsetArgs(), buildArgs("./cmd/gragas")} {
		ask(nil, args)
	}
	wg.Wait()
	if s.err != nil {
		return s
	}

	tb, err := loadTable()
	if err != nil {
		s.err = err
		return s
	}
	w := world{module: tb.Module, pkgs: map[string]*pkg{}}
	for _, step := range []func(*world) error{(*world).list, (*world).walk, (*world).listTests, (*world).counted} {
		if err := step(&w); err != nil {
			s.err = err
			return s
		}
	}
	s.world = w
	s.parse()
	return s
}

// parse reads each Go file of the world, in parallel, over the one file set.
func (s *snapshot) parse() {
	type parsed struct {
		rel  string
		file *ast.File
		err  error
	}
	var rels []string
	for _, p := range s.world.sorted() {
		for _, name := range append(slices.Clone(p.files), p.tests...) {
			rels = append(rels, path.Join(p.path, name))
		}
	}
	out := make([]parsed, len(rels))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, rel := range rels {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			f, err := parser.ParseFile(s.fset, filepath.Join(root, filepath.FromSlash(rel)), nil, parser.ParseComments)
			out[i] = parsed{rel, f, err}
		})
	}
	wg.Wait()
	for _, p := range out {
		if p.err != nil {
			// A file that does not parse is reported by the test that reads it.
			continue
		}
		s.asts[p.rel] = p.file
	}
}

// goOut is what go list prints for args with env added to the environment,
// run once for the whole package.
func goOut(env []string, args ...string) ([]byte, error) {
	return theTree.asked.of(env, args, func() ([]byte, error) { return runGoList(env, args...) })
}

// clone is a copy of w no test's change reaches the snapshot through: the
// packages, their lists, the files and the test closures.
func (w world) clone() world {
	c := w
	c.pkgs = make(map[string]*pkg, len(w.pkgs))
	for k, p := range w.pkgs {
		q := *p
		q.files, q.tests = slices.Clone(p.files), slices.Clone(p.tests)
		q.imports, q.testImports = slices.Clone(p.imports), slices.Clone(p.testImports)
		c.pkgs[k] = &q
	}
	c.files, c.constrained = slices.Clone(w.files), slices.Clone(w.constrained)
	c.testDeps = make(map[string][]string, len(w.testDeps))
	for k, v := range w.testDeps {
		c.testDeps[k] = slices.Clone(v)
	}
	return c
}

// TestTheSnapshotIsTheTreeAsAFreshReadSaysIt: the snapshot's go list and
// its walk are what a read made now says. Risk: a snapshot that serves the
// checks a tree other than the one on disk.
func TestTheSnapshotIsTheTreeAsAFreshReadSaysIt(t *testing.T) {
	t.Parallel()
	s := snap()
	if s.err != nil {
		t.Fatal(s.err)
	}
	held, err := goOut(nil, listArgs()...)
	if err != nil {
		t.Fatal(err)
	}
	live, err := runGoList(nil, listArgs()...)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(held, live) {
		t.Error("the snapshot's go list is not what go list prints now")
	}
	fresh := world{module: s.world.module, pkgs: map[string]*pkg{}}
	if err := fresh.walk(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.world.files, fresh.files) || !slices.Equal(s.world.constrained, fresh.constrained) {
		t.Error("the snapshot's files are not what a walk of the tree finds now")
	}
	for rel, f := range s.asts {
		if f == nil || f.Name == nil {
			t.Errorf("%s: the snapshot holds no parsed file", rel)
		}
	}
	if len(s.asts) != len(s.world.files)-nonGo(s.world.files) {
		t.Errorf("the snapshot parsed %d Go files, and the tree holds %d", len(s.asts), len(s.world.files)-nonGo(s.world.files))
	}
}

// nonGo is how many of files are not Go files.
func nonGo(files []string) int {
	n := 0
	for _, f := range files {
		if !strings.HasSuffix(f, ".go") {
			n++
		}
	}
	return n
}

// TestAChangeToACopyOfTheWorldNeverReachesTheSnapshot: the packages, their
// lists, the files and the closures a test plants a breach in are its own.
// Risk: one test's planted breach read by another.
func TestAChangeToACopyOfTheWorldNeverReachesTheSnapshot(t *testing.T) {
	t.Parallel()
	s := snap()
	if s.err != nil {
		t.Fatal(s.err)
	}
	c := s.world.clone()
	for _, p := range c.pkgs {
		p.imports = append(p.imports, "planted")
		p.files = append(p.files[:0], "planted.go")
	}
	c.files[0], c.constrained = "planted", append(c.constrained, "planted.go")
	for k := range c.testDeps {
		c.testDeps[k] = append(c.testDeps[k], "planted")
		delete(c.testDeps, k)
		break
	}
	for _, p := range s.world.pkgs {
		if slices.Contains(p.imports, "planted") || slices.Contains(p.files, "planted.go") {
			t.Fatalf("%s: a change to a copy reached the snapshot", p.path)
		}
	}
	if s.world.files[0] == "planted" || slices.Contains(s.world.constrained, "planted.go") {
		t.Error("a change to a copy reached the snapshot's files")
	}
	if len(s.world.testDeps) != len(c.testDeps)+1 {
		t.Error("a change to a copy reached the snapshot's closures")
	}
}

// TestAGoListIsRunOnceWhateverHowManyTestsAskIt: the readers of the tree ask
// the snapshot, and each go list the package's tests share ran once, in
// the tree read. Risk: every test that reads the tree running go list again, as
// each did before the snapshot.
func TestAGoListIsRunOnceWhateverHowManyTestsAskIt(t *testing.T) {
	t.Parallel()
	for range 3 {
		tree(t)
		goList(t)
		if err := golist(func(*json.Decoder) error { return io.EOF }, shippedArgs()...); err != nil {
			t.Fatal(err)
		}
		read(t, "Taskfile.yml")
	}
	for name, args := range map[string][]string{"list": listArgs(), "test list": testListArgs(), "every package": allArgs(), "shipped": shippedArgs(), "dev": devArgs(), "modules": modulesArgs(), "subset": subsetArgs()} {
		if got := theTree.asked.ran(nil, args); got != 1 {
			t.Errorf("go list %s ran %d times, want once", name, got)
		}
	}
	if got := theTree.asked.ran(nil, []string{"read", "Taskfile.yml"}); got != 1 {
		t.Errorf("the Taskfile was read %d times, want once", got)
	}
}
