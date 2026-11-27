package root

import (
	"errors"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/legacy"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// inFolder is a tree whose working folder ("" for Open) is a folder of the
// world, so a test can stand below a repository's root.
type inFolder struct {
	port.Tree
	folder string
}

func (f inFolder) Open(dir string) (model.Root, error) {
	if dir == "" {
		return f.Tree.Open(f.folder)
	}
	return f.Tree.Open(dir)
}

// noTOML reads no document: these tests reach no .gitleaks.toml.
type noTOML struct{}

func (noTOML) Read([]byte) (map[string]any, error) { return nil, nil }

const (
	pinFile = ".gragas/pin.yaml"
	pinBody = "rulebook: example\nsource: book\nparts: [go]\nformat: 1\n"
	gitFile = "gitdir: x\n"
)

// joined is the session of a run in folder, over a world that holds each of
// roots (a folder and its files).
func joined(t *testing.T, folder string, roots map[string]map[string]string) kitSession {
	t.Helper()
	w := porttest.NewWorld(t)
	for dir, files := range roots {
		w.Seed(dir, files)
	}
	ports := w.Ports()
	ports.Tree = inFolder{ports.Tree, folder}
	return kitSession{&session{p: Process{World: ports, Ports: legacy.Ports{TOML: noTOML{}}}}}
}

// TestHoldsRulebookIsAStatOfTheWorkingFolderOnly: a rulebook.yaml in the
// folder is the gate; one above it, or none, is not.
func TestHoldsRulebookIsAStatOfTheWorkingFolderOnly(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		roots map[string]map[string]string
		want  bool
	}{
		{"in the folder", map[string]map[string]string{"/w/repo": {"rulebook.yaml": "x\n", ".git": gitFile}}, true},
		{"only above the folder", map[string]map[string]string{"/w/repo": {"rulebook.yaml": "x\n", ".git": gitFile}, "/w/repo/sub": {"a.txt": "a\n"}}, false},
		{"none", map[string]map[string]string{"/w/repo": {"a.txt": "a\n", ".git": gitFile}}, false},
	} {
		folder := "/w/repo"
		if len(c.roots) == 2 {
			folder = "/w/repo/sub"
		}
		got, err := joined(t, folder, c.roots).HoldsRulebook(t.Context())
		if err != nil || got != c.want {
			t.Errorf("%s: HoldsRulebook = %t, %v; want %t", c.name, got, err, c.want)
		}
	}
}

// TestHoldsPinStopsAtTheRepositoryRoot: a pin in the folder or below the
// repository's root up to it is the project's; one above the folder that
// holds .git is another project's; no .git above looks to the top.
func TestHoldsPinStopsAtTheRepositoryRoot(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		folder string
		roots  map[string]map[string]string
		want   bool
	}{
		{"a pin above the repository's root", "/w/repo/sub", map[string]map[string]string{"/w": {pinFile: pinBody}, "/w/repo": {".git": gitFile}, "/w/repo/sub": {"a.txt": "a\n"}}, false},
		{"a pin in the repository's root", "/w/repo/sub", map[string]map[string]string{"/w/repo": {pinFile: pinBody, ".git": gitFile}, "/w/repo/sub": {"a.txt": "a\n"}}, true},
		{"a pin in the folder", "/w/repo/sub", map[string]map[string]string{"/w/repo": {".git": gitFile}, "/w/repo/sub": {pinFile: pinBody}}, true},
		{"a pin and no .git above", "/w/sub", map[string]map[string]string{"/w": {pinFile: pinBody}, "/w/sub": {"a.txt": "a\n"}}, true},
		{"no pin", "/w/repo", map[string]map[string]string{"/w/repo": {".git": gitFile}}, false},
	} {
		got, err := joined(t, c.folder, c.roots).HoldsPin(t.Context())
		if err != nil || got != c.want {
			t.Errorf("%s: HoldsPin = %t, %v; want %t", c.name, got, err, c.want)
		}
	}
}

// TestCheckProjectEndsOnTheProjectsOwnWords: a pin that names a source that
// is not there, and a pin with no repository around it, end the project
// check not ready in the project's words, not on the rulebook's.
func TestCheckProjectEndsOnTheProjectsOwnWords(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		folder string
		roots  map[string]map[string]string
		says   string
	}{
		{"a source not there", "/w/repo", map[string]map[string]string{"/w/repo": {pinFile: pinBody, ".git": gitFile}}, "book"},
		{"no .git above the pin", "/w/sub", map[string]map[string]string{"/w": {pinFile: pinBody}, "/w/sub": {"a.txt": "a\n"}}, "no .git"},
	} {
		_, ref, err := joined(t, c.folder, c.roots).CheckProject(t.Context())
		if ref != nil || !errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: CheckProject = %v, %v; want ErrNotReady naming %q", c.name, ref, err, c.says)
		}
	}
}

// TestTheCheckIsHandedTheWorldsPluginsAndTheFoldersRoot (step 8):
// the session gives the check the plugin port and the ports a read of a config
// is run and journalled through, the folder's root and the run's engine, for the
// command check; a world with no Plugins port gives none, and the check reads by
// the adapter alone.
// Risk: a check that is wired to no plugin, so an installed one is never asked.
func TestTheCheckIsHandedTheWorldsPluginsAndTheFoldersRoot(t *testing.T) {
	t.Parallel()
	k := joined(t, "/w/repo", map[string]map[string]string{"/w/repo": {"a.txt": "a\n"}})
	world := k.s.p.World
	want := func(who string, got coreapi.AuditPlugins) {
		t.Helper()
		if got.Plugins != world.Plugins || got.Steps != world.Steps || got.Sandbox != world.Sandbox || got.Journal != world.Journal || got.Clock != world.Clock {
			t.Errorf("%s: %+v, want the world's own ports", who, got)
		}
		if got.Plugins == nil || got.Steps == nil || got.Sandbox == nil || got.Journal == nil || got.Clock == nil {
			t.Errorf("%s: %+v, want every port the read runs through", who, got)
		}
		if got.Root == "" || got.Command != "check" || got.Engine == "" {
			t.Errorf("%s: %+v, want a root, the command check and the engine's stamp", who, got)
		}
	}
	want("auditPlugins", k.s.auditPlugins("/w/repo"))
	// Each way the check is asked carries it: the project check's ask and the
	// old check's dependencies (the mutant that drops either leaves an
	// installed plugin never asked).
	want("the project check's ask", k.projectAsk("/w/repo").Plugins)
	want("the old check's Audit", k.s.legacyDeps().Audit("/w/repo"))
	// A folder that cannot be opened has no root, and the check then reads no
	// tool through a plugin (the audit's own empty-Root answer).
	k.s.p.World.Tree = unopenable{k.s.p.World.Tree}
	if got := k.s.auditPlugins("/w/repo"); got.Root != "" {
		t.Errorf("for a folder that cannot be opened, auditPlugins = %+v, want no Root", got)
	}
}

// unopenable is a Tree that opens no folder.
type unopenable struct{ port.Tree }

func (unopenable) Open(string) (model.Root, error) { return "", errors.New("cannot be opened") }
