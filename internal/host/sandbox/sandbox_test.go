package sandbox_test

import (
	"context"
	"errors"
	"github.com/birdtable/go-files/filestest"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/sandbox"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// cache is a cache folder of the test's own, its links resolved, so a path
// the adapter answers compares with one the test builds. It is
// filestest.Private, which the trust walk passes on every system, as the
// adapter walks the cache folder before it holds the base.
func cache(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(filestest.Private(t))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// runs is the scratch folders left in the cache folder dir.
func runs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, sandbox.Folder))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// files is every file below dir, relative and slash-separated, links not
// followed, and whether dir is there.
func files(dir string) ([]string, bool) {
	if _, err := os.Lstat(dir); err != nil {
		return nil, false
	}
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return err
	})
	return out, true
}

// TestTheSandboxKeepsItsContract: the Sandbox suite against the adapter.
func TestTheSandboxKeepsItsContract(t *testing.T) {
	t.Parallel()
	contracttest.Sandbox(t, func(t *testing.T) contracttest.SandboxSetup {
		t.Helper()
		return contracttest.SandboxSetup{
			Sandbox: sandbox.New(sandbox.Config{Dir: cache(t)}),
			Files:   func(s model.Scratch) ([]string, bool) { return files(filepath.FromSlash(s.Dir)) },
			Read: func(s model.Scratch, rel string) ([]byte, error) {
				return os.ReadFile(filepath.Join(filepath.FromSlash(s.Dir), filepath.FromSlash(rel)))
			},
		}
	})
}

// TestALinkPlantedWhereTheFoldersAreMadeIsRefused: the cache's scratch
// folder replaced by a link elsewhere, or by a file, refuses Prepare and
// Release, and nothing is written or removed where it points. Risk: every
// run's files, and every removal, put in another folder.
func TestALinkPlantedWhereTheFoldersAreMadeIsRefused(t *testing.T) {
	t.Parallel()
	dir, elsewhere := cache(t), cache(t)
	if err := os.WriteFile(filepath.Join(elsewhere, "keep"), []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, elsewhere, filepath.Join(dir, sandbox.Folder))
	s := sandbox.New(sandbox.Config{Dir: dir})
	if sc, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "a.go"}}}); !errors.Is(err, port.ErrRefused) || !strings.Contains(err.Error(), "is a link") {
		t.Errorf("Prepare through a planted link = %+v, %v; want refused", sc, err)
	}
	if err := s.Release(model.Scratch{Dir: filepath.ToSlash(filepath.Join(dir, sandbox.Folder, "run-1"))}); !errors.Is(err, port.ErrRefused) {
		t.Errorf("Release through a planted link = %v; want refused", err)
	}
	if got, _ := files(elsewhere); !slices.Equal(got, []string{"keep"}) {
		t.Errorf("where the link points holds %v, want keep alone", got)
	}
	file := cache(t)
	if err := os.WriteFile(filepath.Join(file, sandbox.Folder), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := sandbox.New(sandbox.Config{Dir: file}).Prepare(t.Context(), model.Prepare{Level: model.Isolated}); err == nil {
		t.Error("Prepare over a file where the folders are made = nil, want refused")
	}
}

// TestTheResolveFromFolderIsLinkedAndReleaseNeverFollowsIt: the folder is
// reached through the link, and Release removes the link and leaves the
// folder and its files. Risk: a release that empties a project's
// dependency folder.
func TestTheResolveFromFolderIsLinkedAndReleaseNeverFollowsIt(t *testing.T) {
	t.Parallel()
	dir, deps := cache(t), cache(t)
	if err := os.MkdirAll(filepath.Join(deps, "plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deps, "plugin", "index.js"), []byte("p"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := sandbox.New(sandbox.Config{Dir: dir})
	sc, err := s.Prepare(t.Context(), model.Prepare{
		Level: model.Isolated, Config: []model.File{{Path: "eslint.config.js"}},
		Resolve: model.Link{Path: "node_modules", Dir: filepath.ToSlash(deps)},
	})
	if err != nil {
		t.Fatal(err)
	}
	at := filepath.Join(filepath.FromSlash(sc.Dir), "node_modules")
	if target, err := os.Readlink(at); err != nil || target != deps {
		t.Errorf("the link at %s is %q, %v; want %s", at, target, err, deps)
	}
	if b, err := os.ReadFile(filepath.Join(at, "plugin", "index.js")); err != nil || string(b) != "p" {
		t.Errorf("the plugin through the link: %q, %v", b, err)
	}
	if err := s.Release(sc); err != nil {
		t.Fatal(err)
	}
	if got, _ := files(deps); !slices.Equal(got, []string{"plugin/index.js"}) {
		t.Errorf("after Release the linked folder holds %v, want its plugin", got)
	}
}

// TestALinkedFolderOutsideItsShapeIsRefusedAndNoFolderIsLeft: the folder to
// link must be a folder that is not a link, placed at a path inside, where
// no file of the run is; each refusal leaves no scratch folder and writes
// nothing into the folder named. A folder that is not there is told by
// fs.ErrNotExist, never by the system's words, which differ on Windows.
// Risk: a fixture written through the link into a project's own files.
func TestALinkedFolderOutsideItsShapeIsRefusedAndNoFolderIsLeft(t *testing.T) {
	t.Parallel()
	dir, deps := cache(t), cache(t)
	link := filepath.Join(cache(t), "deps-link")
	testenv.Link(t, deps, link)
	plain := filepath.Join(deps, "plain")
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := sandbox.New(sandbox.Config{Dir: dir})
	for name, c := range map[string]struct {
		p    model.Prepare
		says string
		is   error
	}{
		"a link":          {model.Prepare{Resolve: model.Link{Path: "node_modules", Dir: filepath.ToSlash(link)}}, "is itself a link", nil},
		"a file":          {model.Prepare{Resolve: model.Link{Path: "node_modules", Dir: filepath.ToSlash(plain)}}, "is not a folder", nil},
		"a relative path": {model.Prepare{Resolve: model.Link{Path: "node_modules", Dir: "deps"}}, "not an absolute, clean path", nil},
		"no folder":       {model.Prepare{Resolve: model.Link{Path: "node_modules", Dir: filepath.ToSlash(filepath.Join(deps, "gone"))}}, "to link", fs.ErrNotExist},
		"placed outside":  {model.Prepare{Resolve: model.Link{Path: "../node_modules", Dir: filepath.ToSlash(deps)}}, "leaves the scratch folder", nil},
		"under a fixture": {model.Prepare{Fixtures: []model.File{{Path: "node_modules/evil.js", Data: []byte("x")}}, Resolve: model.Link{Path: "node_modules", Dir: filepath.ToSlash(deps)}}, "could not be linked", nil},
	} {
		c.p.Level = model.Isolated
		sc, err := s.Prepare(t.Context(), c.p)
		if err == nil || !strings.Contains(err.Error(), c.says) || (c.is != nil && !errors.Is(err, c.is)) {
			t.Errorf("%s: %+v, %v; want refused saying %q, matching %v", name, sc, err, c.says, c.is)
		}
	}
	if left := runs(t, dir); len(left) != 0 {
		t.Errorf("the refusals left %v", left)
	}
	if got, _ := files(deps); !slices.Equal(got, []string{"plain"}) {
		t.Errorf("the folder to link holds %v, want plain alone", got)
	}
}

// TestReleaseRemovesOnlyAFolderPrepareCouldHaveMade: a folder outside the
// cache's scratch folder, the scratch folder itself, one not named as a run,
// one that is not clean, and a folder named as a run outside the scratch
// folder are each refused and left. Risk: a Scratch from anywhere removing
// a project.
func TestReleaseRemovesOnlyAFolderPrepareCouldHaveMade(t *testing.T) {
	t.Parallel()
	dir, other := cache(t), cache(t)
	s := sandbox.New(sandbox.Config{Dir: dir})
	sc, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated})
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, sandbox.Folder)
	named := filepath.Join(base, "mine")
	if err := os.Mkdir(named, 0o700); err != nil {
		t.Fatal(err)
	}
	// A run's name outside the scratch folder: only the parent half of the
	// shape check refuses it, since its name has the prefix.
	stray := filepath.Join(other, filepath.Base(filepath.FromSlash(sc.Dir)))
	if err := os.Mkdir(stray, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{other, base, named, sc.Dir + "/../" + filepath.Base(named), dir, stray} {
		if err := s.Release(model.Scratch{Dir: filepath.ToSlash(d), Level: model.Isolated}); !errors.Is(err, port.ErrRefused) {
			t.Errorf("Release(%s) = %v, want refused", d, err)
		}
	}
	for _, d := range []string{other, named, filepath.FromSlash(sc.Dir), stray} {
		if _, err := os.Lstat(d); err != nil {
			t.Errorf("%s was removed: %v", d, err)
		}
	}
	if err := s.Release(sc); err != nil {
		t.Errorf("the control, the folder Prepare made: %v", err)
	}
}

// stopAfter is a context whose Err answers Canceled from its n-th call on,
// so a test cancels a Prepare between two files.
type stopAfter struct {
	context.Context
	n *int
}

func (c stopAfter) Err() error {
	if *c.n--; *c.n < 0 {
		return context.Canceled
	}
	return nil
}

// TestACancelOrARefusalPastTheFolderLeavesNoFolder: a Prepare cancelled
// before it starts, cancelled between two files, or refused at its second
// file removes what it made. Risk: a stopped run's fixtures left in the
// cache.
func TestACancelOrARefusalPastTheFolderLeavesNoFolder(t *testing.T) {
	t.Parallel()
	dir := cache(t)
	s := sandbox.New(sandbox.Config{Dir: dir})
	many := model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "a.go"}, {Path: "b.go"}, {Path: "c.go"}}}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Prepare(cancelled, many); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled Prepare = %v", err)
	}
	n := 2
	if _, err := s.Prepare(stopAfter{t.Context(), &n}, many); !errors.Is(err, context.Canceled) {
		t.Errorf("a Prepare cancelled between files = %v", err)
	}
	if _, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "A.go"}, {Path: "A.go"}}}); !errors.Is(err, port.ErrRefused) {
		t.Errorf("two files at one path = %v, want refused", err)
	}
	if left := runs(t, dir); len(left) != 0 {
		t.Errorf("left %v", left)
	}
	if sc, err := s.Prepare(t.Context(), many); err != nil || len(runs(t, dir)) != 1 {
		t.Errorf("the control: %+v, %v, %v", sc, err, runs(t, dir))
	}
}

// TestTheSandboxedLevelIsUnavailableAndMakesNoFolder: this build has no
// container or OS sandbox, so a sandboxed Prepare matches ErrUnavailable and
// makes nothing. Risk: a cell recorded as sandboxed that ran isolated.
func TestTheSandboxedLevelIsUnavailableAndMakesNoFolder(t *testing.T) {
	t.Parallel()
	dir := cache(t)
	sc, err := sandbox.New(sandbox.Config{Dir: dir}).Prepare(t.Context(), model.Prepare{Level: model.Sandboxed})
	if !errors.Is(err, port.ErrUnavailable) || sc != (model.Scratch{}) {
		t.Errorf("sandboxed = %+v, %v; want unavailable", sc, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, sandbox.Folder)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a folder was made: %v", err)
	}
}

// TestNoTextOfAnIsolatedRunSaysReadOnly: what the adapter says, in every
// refusal and in its own source, never calls the linked folder read-only:
// only the sandboxed level makes it so. Risk: a person told a tool cannot
// write where it can.
func TestNoTextOfAnIsolatedRunSaysReadOnly(t *testing.T) {
	t.Parallel()
	dir, deps := cache(t), cache(t)
	s := sandbox.New(sandbox.Config{Dir: dir})
	var said []string
	for _, p := range []model.Prepare{
		{Level: model.Sandboxed},
		{Level: model.Isolated, Resolve: model.Link{Path: "node_modules", Dir: "deps"}},
		{Level: model.Isolated, Resolve: model.Link{Path: "../x", Dir: filepath.ToSlash(deps)}},
		{Level: model.Isolated, Fixtures: []model.File{{Path: "node_modules/x"}}, Resolve: model.Link{Path: "node_modules", Dir: filepath.ToSlash(deps)}},
		{Level: model.Isolated, Seconds: -1},
	} {
		if _, err := s.Prepare(t.Context(), p); err != nil {
			said = append(said, err.Error())
		}
	}
	if err := s.Release(model.Scratch{}); err != nil {
		said = append(said, err.Error())
	}
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range sources {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		said = append(said, string(b))
	}
	if len(said) < 7 {
		t.Fatalf("only %d texts were read", len(said))
	}
	for _, text := range said {
		if strings.Contains(strings.ToLower(text), "read-only") || strings.Contains(strings.ToLower(text), "read only") {
			t.Errorf("an isolated run says read-only: %q", text)
		}
	}
}

// TestSweepRemovesOnlyWhatPrepareCouldHaveMadeAndFollowsNoLink: a folder a
// stopped run left, one with a linked folder in it and a link named like a
// run's folder are removed as links and folders, never through them; a
// name that is not a run's stays, and so does a link planted where the
// folders are made. Risk: a sweep that empties a project's dependency
// folder, or the folder a planted link points to.
func TestSweepRemovesOnlyWhatPrepareCouldHaveMadeAndFollowsNoLink(t *testing.T) {
	t.Parallel()
	dir, elsewhere := cache(t), cache(t)
	if err := os.WriteFile(filepath.Join(elsewhere, "keep"), []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := sandbox.New(sandbox.Config{Dir: dir})
	if n, err := s.Sweep(); err != nil || n != 0 {
		t.Errorf("Sweep with no scratch folder yet = %d, %v; want 0 and no error", n, err)
	}
	if _, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Fixtures: []model.File{{Path: "src/a.go", Data: []byte("x")}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prepare(t.Context(), model.Prepare{Level: model.Isolated, Resolve: model.Link{Path: "node_modules", Dir: filepath.ToSlash(elsewhere)}}); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, sandbox.Folder)
	testenv.Link(t, elsewhere, filepath.Join(base, "run-linked"))
	named := filepath.Join(base, "mine")
	if err := os.Mkdir(named, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "stray"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Left(); err != nil || n != 3 {
		t.Errorf("Left = %d, %v; want the 3 a Sweep removes (two folders and the link)", n, err)
	}
	if left := runs(t, dir); len(left) != 5 {
		t.Errorf("the scratch folder holds %v after Left, want all five; Left removes nothing", left)
	}
	if n, err := s.Sweep(); err != nil || n != 3 {
		t.Errorf("Sweep = %d, %v; want 3 removed (two folders and the link)", n, err)
	}
	if err := os.Remove(filepath.Join(base, "stray")); err != nil {
		t.Errorf("stray, no name Prepare makes, is gone after a sweep: %v", err)
	}
	if left := runs(t, dir); !slices.Equal(left, []string{"mine"}) {
		t.Errorf("the scratch folder holds %v after a sweep, want mine alone", left)
	}
	if got, _ := files(elsewhere); !slices.Equal(got, []string{"keep"}) {
		t.Errorf("where the links point holds %v, want keep alone", got)
	}

	planted := cache(t)
	testenv.Link(t, elsewhere, filepath.Join(planted, sandbox.Folder))
	if n, err := sandbox.New(sandbox.Config{Dir: planted}).Left(); !errors.Is(err, port.ErrRefused) || n != 0 {
		t.Errorf("Left through a planted link = %d, %v; want 0 and refused, as Sweep is", n, err)
	}
	if n, err := sandbox.New(sandbox.Config{Dir: planted}).Sweep(); !errors.Is(err, port.ErrRefused) || n != 0 {
		t.Errorf("Sweep through a planted link = %d, %v; want 0 and refused", n, err)
	}
	if got, _ := files(elsewhere); !slices.Equal(got, []string{"keep"}) {
		t.Errorf("after a sweep through a planted link, where it points holds %v, want keep alone", got)
	}
}
