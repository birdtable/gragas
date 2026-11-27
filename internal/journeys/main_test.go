// Package journeys runs the real gragas binary the way a person and a
// script run it: its exit codes, its streams, its goldens, its JSON, the
// terminal rows, a run that writes and one stopped by a signal. Only this
// package runs the binary (tests.journeys.2).
package journeys

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/birdtable/go-files/appdirs"
	testenv "github.com/birdtable/go-testkit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// The build the binary is stamped with, so a golden does not move with the
// commit.
const (
	stampedVersion = "v1.2.3"
	stampedCommit  = "abc1234"
)

// TestMain runs the tests, and removes the binary the first of them built.
func TestMain(m *testing.M) {
	os.Exit(configtest.MainCrash(&wrapped{m}))
}

// wrapped removes the built binary's folder after the tests. It reads
// nothing before m.Run: Go keys the test cache on what a test opens inside
// m.Run only, so the build is made from a test's body.
type wrapped struct{ m *testing.M }

func (w *wrapped) Run() int {
	code := w.m.Run()
	if b.dir != "" {
		_ = os.RemoveAll(b.dir)
	}
	if pluginBuilt.dir != "" {
		_ = os.RemoveAll(pluginBuilt.dir)
	}
	return code
}

// b is the binary of this run, built once, on the first call of gragas.
var b struct { //nolint:gochecknoglobals // one build for the whole run, made by the first test that asks
	once sync.Once
	dir  string
	bin  string
	err  error
}

// gragas is the binary built from the tree for this run, the path to it: built
// on the first call, from the body of the test that makes it, so the tree's
// files the build record reads are in this package's test-cache key.
func gragas() (string, error) {
	b.once.Do(func() { b.dir, b.bin, b.err = build() })
	return b.bin, b.err
}

// gragasBin is gragas, the test failing when it could not be built.
func gragasBin(t *testing.T) string {
	t.Helper()
	bin, err := gragas()
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

// stamped is gragas as the goldens hold it: built by the kit with the version
// and commit stamped, and without a file system path of the build.
func stamped() *testenv.Binary {
	return testenv.Build("cmd/gragas").X("main.version", stampedVersion).X("main.commit", stampedCommit).Trimpath()
}

// build puts the stamped gragas in a new folder of its own, with the record of
// the tree it came from beside it, and answers the folder and the binary. The
// kit builds once and keeps the binary where it likes; the copy is here so the
// record has a folder that is ours.
func build() (dir, bin string, err error) {
	dir, err = os.MkdirTemp("", "gragas-acceptance")
	if err != nil {
		return "", "", err
	}
	bin, err = copiedInto(dir, stamped())
	if err != nil {
		return dir, "", err
	}
	if err := recordBuild(dir); err != nil {
		return dir, "", errors.New("could not record the build: " + err.Error())
	}
	return dir, bin, nil
}

// copiedInto copies the binary the kit built into dir and answers the copy's
// path, named gragas, with the extension the kit's file has, so a script finds it
// on its PATH by that name and the host starts it.
func copiedInto(dir string, kit *testenv.Binary) (string, error) {
	src := kit.Path()
	if src == "" {
		return "", errors.New("could not build gragas:\n" + configtest.GoSaid(kit.Run().Stderr))
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "gragas"+filepath.Ext(src))
	return dst, os.WriteFile(dst, data, 0o755)
}

// moduleRoot is the folder holding go.mod, from this package's folder.
func moduleRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// result is what one run left behind.
type result struct {
	stdout, stderr string
	code           int
}

// run runs the binary in dir with the isolated environment and env on top.
// Its stdout is a pipe, never a terminal.
func run(t *testing.T, dir string, env map[string]string, args ...string) result {
	t.Helper()
	c := command(t, dir, env, args...)
	var out, errb strings.Builder
	c.Stdout, c.Stderr = &out, &errb
	code := 0
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("could not run gragas: %v", err)
		}
		code = ee.ExitCode()
	}
	return toolchainMasked(t, result{stdout: out.String(), stderr: errb.String(), code: code})
}

// toolchainMasked is r with the Go release put back as a word. The version
// line names the toolchain (go-run's version.Build), which moves with every
// Go release and says nothing of gragas, so no golden moves with it; scrub
// does the same for the goldens it reads.
func toolchainMasked(tb testing.TB, r result) result {
	tb.Helper()
	r.stdout = stateRowMasked(tb, goVersion.ReplaceAllString(r.stdout, "go<version>"))
	r.stderr = stateRowMasked(tb, goVersion.ReplaceAllString(r.stderr, "go<version>"))
	return r
}

// command is the binary ready to run in dir, its environment the isolated
// one with env on top.
func command(t *testing.T, dir string, env map[string]string, args ...string) *exec.Cmd {
	t.Helper()
	return commandOf(t, gragasBin(t), dir, env, args...)
}

// commandOf is the binary at bin ready to run in dir, its environment the
// isolated one with env on top. GRAGAS_SHELL is posix, so every line to paste
// a golden or a journey compares is the same whatever shell started the
// suite: on Windows the shell setting's auto reads the parent processes.
// A journey of another dialect sets it in env.
func commandOf(t *testing.T, bin, dir string, env map[string]string, args ...string) *exec.Cmd {
	t.Helper()
	c := exec.CommandContext(t.Context(), bin, args...) //nolint:gosec // the binary this suite built
	c.Dir = dir
	set := map[string]string{"COLUMNS": "80", stateKey: stateHome(t), "GRAGAS_SHELL": "posix"}
	maps.Copy(set, env)
	c.Env = configtest.Inherited()
	for _, k := range slices.Sorted(maps.Keys(placed(set))) {
		c.Env = append(c.Env, k+"="+placed(set)[k])
	}
	return c
}

// gitSettings names git's own settings for a test's home, for a test that runs
// git itself (the binary's git is handed its environment by host/git, which
// starts it empty and sets HOME alone): its .gitconfig as the global file, since Inherited keeps testenv's GIT_CONFIG_GLOBAL, which would hide
// it, and no system file. These are git's variables, the same on every system, so
// git finds the identity wherever the system keeps the home.
func gitSettings(env map[string]string) []string {
	home := env[homeKey]
	if home == "" {
		return nil
	}
	return []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + filepath.Join(home, ".gitconfig")}
}

// fixture is a rulebook repository in a folder of the test's own: the core's
// valid repository, which holds the generated files its tree makes so gen
// finds them current, and the repository's schemas.
func fixture(t *testing.T) string {
	t.Helper()
	root := moduleRoot()
	dir := t.TempDir()
	copyTree(t, filepath.Join(root, "internal/core/api/testdata/valid"), dir)
	copyTree(t, filepath.Join(root, "rulebook/schema"), filepath.Join(dir, "rulebook/schema"))
	return dir
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	n := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		n++
		copyFile(t, p, filepath.Join(dst, rel))
		return nil
	})
	if err != nil || n == 0 {
		t.Fatalf("copy %s: %v (%d files)", src, err, n)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// stateHome is the XDG state folder of t: a folder named for the test under the
// one isolate set, so tests that run together never share a lock or a run
// record. Every command of t runs with it.
func stateHome(t *testing.T) string {
	t.Helper()
	s := os.Getenv(envName(stateKey))
	if s == "" {
		t.Fatalf("%s is not set: isolate sets it", envName(stateKey))
	}
	return filepath.Join(s, "tests", strings.NewReplacer("/", "_", `\`, "_").Replace(t.Name()))
}

// placed is env as the system the binary is built for reads it: each name by
// envName. A system that moves all its folders by one variable (Windows) gives
// the state and the cache one name, and the state's value stands, so a test's
// two folders are one root there; appFolders says where each lands.
func placed(env map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range env {
		if k != stateKey {
			out[envName(k)] = v
		}
	}
	if v, ok := env[stateKey]; ok {
		out[envName(stateKey)] = v
	}
	return out
}

// appFolders is gragas's folders under the env a test hands the binary, as
// go-files' appdirs places them on this system: where the binary will keep its
// state and its cache, which is the test's state and cache folder under gragas
// where the layout is XDG's and another place where it is not.
func appFolders(t *testing.T, env map[string]string) appdirs.Folders {
	t.Helper()
	got := placed(env)
	if got[appdirs.HomeVar] == "" {
		got[appdirs.HomeVar] = os.TempDir()
	}
	if src, err := appdirs.Sources(appdirs.XDG); err == nil && src.Run != "" && got[src.Run] == "" {
		got[src.Run] = os.TempDir() // the run folder is no folder of the test's; the system wants it set
	}
	f, err := appdirs.HereWith("gragas", appdirs.XDG, func(k string) string { return got[k] })
	if err != nil {
		t.Fatalf("the binary's folders under the test's environment: %v", err)
	}
	return f
}

// envName is the variable the system the binary is built for reads what a test
// sets by its XDG name or by HOME: the state folder by the one go-files'
// appdirs.Sources names for it (nothing where only the home moves it), the home
// by appdirs.HomeVar, and any other name as it is. The tests spell no system's
// variable of their own.
func envName(k string) string {
	switch k {
	case stateKey:
		if f, err := appdirs.Sources(appdirs.XDG); err == nil && f.State != "" {
			return f.State
		}
	case cacheKey:
		if f, err := appdirs.Sources(appdirs.XDG); err == nil && f.Cache != "" {
			return f.Cache
		}
	case configKey:
		if f, err := appdirs.Sources(appdirs.XDG); err == nil && f.Config != "" {
			return f.Config
		}
	}
	return k
}

// stateDir is where the binary keeps its lock and run record in a test: the
// state folder of t, under gragas.
func stateDir(t *testing.T) string {
	t.Helper()
	return appFolders(t, map[string]string{stateKey: stateHome(t)}).State
}

// buildInto puts gragas in dir for the tests that compare builds, and answers
// the binary's path: the unstamped build the kit builds beside the stamped one
// (installedInto builds the stamped one into a folder of its own). dir is a new
// folder of the test's own.
func buildInto(t *testing.T, dir string) string {
	t.Helper()
	bin, err := copiedInto(dir, testenv.Build("cmd/gragas").Trimpath())
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

// printed is what the binary at bin writes on standard output for args, run
// in dir.
func printed(t *testing.T, bin, dir string, args ...string) string {
	t.Helper()
	out, err := commandOf(t, bin, dir, nil, args...).Output()
	if err != nil {
		t.Fatalf("%s %v failed: %v", bin, args, err)
	}
	return string(out)
}

// The names a test's environment map is keyed by. They are no system's
// variables: envName turns each into the one the system the binary is built
// for reads (appdirs.HomeVar, and the variable appdirs.Sources names for the
// folder), so the tests spell no system's variable of their own.
const (
	homeKey   = appdirs.HomeVar
	stateKey  = "state"
	cacheKey  = "cache"
	configKey = "config"
)
