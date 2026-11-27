package main

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/root"
	"github.com/birdtable/gragas/internal/config"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/host/assets"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/host/term"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

// newTree is a Tree that is closed when t ends, before t's temporary folders
// are removed: Windows refuses to remove a folder that is held open, and a
// Tree holds every root it opened. It asks for t's temporary
// folder first, so that the folder's removal is registered before the close
// and so runs after it.
func newTree(t *testing.T) *hostfs.Tree {
	t.Helper()
	t.TempDir()
	tr := &hostfs.Tree{}
	t.Cleanup(func() {
		if err := tr.Close(); err != nil {
			t.Errorf("close the tree: %v", err)
		}
	})
	return tr
}

// TestMainFillsEveryRequiredPort: the adapters main wires leave no required
// port empty (arch.ports.17).
func TestMainFillsEveryRequiredPort(t *testing.T) {
	t.Parallel()
	if missing := porttest.Missing(ports(config.Config{}), true); len(missing) > 0 {
		t.Errorf("main leaves %v empty", missing)
	}
}

// TestTheStateFolderWalksTrustTheConfiguredMounts: main hands the lock, the journal and the old lock the configured
// mounts, as it hands the sandbox and git, so their walks of the state folder
// trust a nosuid disk GRAGAS_MOUNTS names, as every other walk does. Risk: a
// state folder on such a disk refused, with a fix line that names the
// setting the person already set. Planted: one of the three left without
// them (red here).
func TestTheStateFolderWalksTrustTheConfiguredMounts(t *testing.T) {
	t.Parallel()
	cfg := config.Config{StateDir: t.TempDir(), Mounts: []string{"/work/disk"}}
	w := world(nil, cfg)
	l, lok := w.Lock.(*hostfs.Lock)
	j, jok := w.Journal.(hostfs.Journal)
	old, ook := ports(cfg).Locks.(hostfs.LegacyLocks)
	if !lok || !jok || !ook || !slices.Equal(l.Mounts, cfg.Mounts) || !slices.Equal(j.Mounts, cfg.Mounts) || !slices.Equal(old.Mounts, cfg.Mounts) {
		t.Errorf("the lock, the journal and the old lock trust %v; want each to trust the configured %v", [][]string{l.Mounts, j.Mounts, old.Mounts}, cfg.Mounts)
	}
}

// TestMainFindsTheTableOfEndingsWhole: the check main makes before anything
// runs passes on the table internal/exit builds, so a built gragas never stops
// on it. Risk: a table go-run/exit refuses, found by a person
// running gragas and not by a test.
func TestMainFindsTheTableOfEndingsWhole(t *testing.T) {
	t.Parallel()
	if err := endings(); err != nil {
		t.Errorf("main's check of the table: %v", err)
	}
}

// TestTheLineCapHoldsTheHooksInput: the terminal main wires reads a line as
// long as the guard's hook input, the largest real input it is handed, so the
// cap on a line never refuses what guard edit would take.
// Risk: the hook input's cap is raised and the terminal refuses it first.
func TestTheLineCapHoldsTheHooksInput(t *testing.T) {
	t.Parallel()
	if term.MaxLineBytes < coreapi.MaxHookInput {
		t.Errorf("term.MaxLineBytes = %d, below coreapi.MaxHookInput = %d", term.MaxLineBytes, coreapi.MaxHookInput)
	}
}

// TestMainOpensThePlaceholderPageThroughAssets: the Assets port main fills
// answers the explorer's page, and a development build's page is the
// placeholder with its marker, not a page that pretends to work.
func TestMainOpensThePlaceholderPageThroughAssets(t *testing.T) {
	t.Parallel()
	page, err := world(nil, config.Config{}).Assets.Read("explorer/index.html")
	if err != nil {
		t.Fatalf("Read(explorer/index.html) = %v", err)
	}
	if !strings.Contains(string(page), assets.PlaceholderMarker) {
		t.Errorf("a development build's explorer page lacks the marker %q", assets.PlaceholderMarker)
	}
}

// TestTheStepsPortLooksWhereTheSettingsSayFirst: the Steps adapter main
// fills takes the settings' tool-paths as the first step of its locate order
// and the tools file's tools as its last, so a tool both place where nothing
// is says it looked at each, in that order, and is not ready; and a manifest
// that grants no program for its tool matches port.ErrGrant. Nothing is started: there
// is no file.
func TestTheStepsPortLooksWhereTheSettingsSayFirst(t *testing.T) {
	t.Parallel()
	// A root no folder is at: absolute, and nonexistent. A root and the paths under it are in the
	// slash form the model holds them in.
	root := filepath.ToSlash(filepath.Join(t.TempDir(), "nowhere"))
	at, machine := path.Join(root, "lint-a"), path.Join(root, "machine", "lint-a")
	cfg := config.Config{Settings: config.Settings{ToolPaths: map[string]map[string]string{root: {"lint-a@1.0.0": at}}},
		Machine: config.Machine{Tools: map[string]string{"lint-a@1.0.0": machine}}}
	grant := plugin.Manifest{Tool: "lint-a", Programs: []plugin.Program{{Found: &plugin.ProgramFound{ID: "lint-a", Versions: ">=1.0.0 <2.0.0", VersionArgv: []string{"--version"}}}}}
	scope := func(m plugin.Manifest) plugin.Scope {
		return plugin.Scope{Manifest: m, Root: model.Root(root), OwnPlace: true, Floor: plugin.LevelIsolated,
			Note:    func(context.Context, plugin.Noted) error { return nil },
			Scratch: model.Scratch{Dir: filepath.ToSlash(t.TempDir()), Level: model.Isolated, Seconds: 60}}
	}
	_, err := tools(cfg, 0).Bind(t.Context(), scope(grant))
	if tried := "tried settings " + at + ", tools-file " + machine; !errors.Is(err, port.ErrStepNotReady) || !strings.Contains(err.Error(), tried) {
		t.Errorf("Bind of a tool the settings and the tools file place at nothing: %v, want not ready, naming %q", err, tried)
	}
	none := plugin.Manifest{Tool: "lint-a"}
	if _, err := tools(cfg, 0).Bind(t.Context(), scope(none)); !errors.Is(err, port.ErrGrant) {
		t.Errorf("Bind of a manifest that grants no program for its tool: %v, want port.ErrGrant", err)
	}
}

// TestHistoryForReachesTheRealGitTheSettingsName: the History port main hands
// a run answers who the repository commits as, through the real git named by
// programs.git. Planted failure: historyFor returning a port with no git behind
// it, or ignoring the setting.
//
// serial: it sets the home variable appdirs names (HomeVar: the one config
// reads, and a system's own, so historyFor's git finds the account's identity
// where the system keeps it), which historyFor reads from the process (main.go),
// so it runs before the package's parallel tests resume.
func TestHistoryForReachesTheRealGitTheSettingsName(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[user]\n\tname = A Person\n\temail = a@example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The home variable appdirs names is set, and git is handed its home by
	// value alone, go-proc's gitenv.Home of the Home historyFor gives
	// over gitenv.Env, which reads nothing of this process: no
	// other variable of this process reaches git (the git adapter's
	// TestTheChildsEnvironmentIsBare), so a Home read from another name, or
	// none, hands git no home, and Who answers no name.
	t.Setenv(config.HomeVar, home)
	dir := t.TempDir()
	root, err := newTree(t).Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	cfg.Settings.Programs.Git = configtest.Git(t)
	h, err := historyFor(cfg)("dry-run")
	if err != nil {
		t.Fatalf("historyFor = %v", err)
	}
	who, err := h.Who(t.Context(), root)
	if err != nil || who.Name != "A Person" || who.Email != "a@example.test" {
		t.Errorf("Who = %+v, %v; want A Person <a@example.test>", who, err)
	}
}

// TestHistoryForWithNoGitInTheSettingsIsErrNoGit: the settings naming no git
// give root.ErrNoGit and never a git found on PATH (A48, cli.config.12).
// Planted failure: a LookPath fallback, which would answer a port here.
func TestHistoryForWithNoGitInTheSettingsIsErrNoGit(t *testing.T) {
	t.Parallel()
	if h, err := historyFor(config.Config{})("dry-run"); !errors.Is(err, root.ErrNoGit) || h != nil {
		t.Errorf("historyFor = %v, %v; want root.ErrNoGit", h, err)
	}
}

// TestARelativeHomeGivesGitNoAccountConfig: a HOME that is not absolute makes
// git read the account's config from the folder it runs in, which is the
// repository, so a file the head commits would be read as the account's own
// (the review's probe P1). historyFor hands git no home then, and Who answers
// that none is configured. Planted failure: Home: os.Getenv(config.HomeVar) as it is,
// which answers the planted name.
//
// serial: it sets the home variable appdirs names, which historyFor reads from
// the process (main.go), so it runs before the package's parallel tests resume.
func TestARelativeHomeGivesGitNoAccountConfig(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "planted"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "planted", ".gitconfig"), []byte("[user]\n\tname = Planted Name\n\temail = planted@example.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := newTree(t).Open(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.HomeVar, "planted")
	var cfg config.Config
	cfg.Settings.Programs.Git = configtest.Git(t)
	h, err := historyFor(cfg)("dry-run")
	if err != nil {
		t.Fatalf("historyFor = %v", err)
	}
	if who, err := h.Who(t.Context(), root); err == nil {
		t.Errorf("Who = %+v: a config the repository holds was read as the account's", who)
	}
}

// TestAccountHomeIsAnAbsoluteCleanPathOrNone: the spellings of HOME handed to
// git. Planted failure: returning h as it is.
func TestAccountHomeIsAnAbsoluteCleanPathOrNone(t *testing.T) {
	t.Parallel()
	home := filepath.Join(t.TempDir(), "a")
	const sep = "/"
	for h, want := range map[string]string{home: home, "planted": "", "./a": "", home + sep + ".." + sep + "a": "", home + sep: "", home + "\n": "", home + "\x1b[0m": ""} {
		if got := accountHome(h); got != want {
			t.Errorf("accountHome(%q) = %q, want %q", h, got, want)
		}
	}
}

// TestMainHandsTheWorldAPluginHost (arch.ports.5): main's world
// always carries Plugins, from host, so core and the CLI glue never test it
// for nil. A program with no plugin installed gets the host's own answer
// (an empty list, port.ErrNotInstalled on an Open), not a nil port.
func TestMainHandsTheWorldAPluginHost(t *testing.T) {
	t.Parallel()
	if world(nil, config.Config{StateDir: t.TempDir()}).Plugins == nil {
		t.Error("main's world has no Plugins; want host's")
	}
}
