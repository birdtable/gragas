package configtest

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-files/appdirs"
)

func TestMain(m *testing.M) { os.Exit(Main(m)) }

// TestAnIsolatedRunHasAHomeOfItsOwn: go-testkit's Main (imported as testenv)
// gave this package a HOME beneath a folder of its own, with git's
// configuration inside that folder.
func TestAnIsolatedRunHasAHomeOfItsOwn(t *testing.T) {
	t.Parallel()
	home := os.Getenv(appdirs.HomeVar)
	if !strings.HasPrefix(filepath.Base(filepath.Dir(home)), "testkit-") {
		t.Fatalf("%s = %q, want a folder of go-testkit's", appdirs.HomeVar, home)
	}
	if got := os.Getenv("GIT_CONFIG_NOSYSTEM"); got != "1" {
		t.Errorf("GIT_CONFIG_NOSYSTEM = %q, want 1", got)
	}
	if got := os.Getenv("GIT_CONFIG_GLOBAL"); !strings.HasPrefix(got, filepath.Dir(home)) {
		t.Errorf("GIT_CONFIG_GLOBAL = %q, want a file beside HOME", got)
	}
}

// TestAChildInheritsNoLeakButWhatIsolateSet: a name set after isolation began
// and not kept is dropped from a child's environment; the folders isolation
// chose stay.
func TestAChildInheritsNoLeakButWhatIsolateSet(t *testing.T) {
	// serial: it sets variables of the process, which Clean reads
	t.Setenv("CI", "1")
	t.Setenv("GRAGAS_NAMES_FILE", "/somewhere/names")
	t.Setenv("LANG", "C")
	env := Inherited()
	keys := make([]string, len(env))
	for i, kv := range env {
		keys[i], _, _ = strings.Cut(kv, "=")
	}
	for _, gone := range []string{"CI", "GRAGAS_NAMES_FILE", "LANG"} {
		if slices.Contains(keys, gone) {
			t.Errorf("a child inherits %s", gone)
		}
	}
	for _, kept := range []string{appdirs.HomeVar, stateVar(t), "GIT_CONFIG_NOSYSTEM", "PATH"} {
		if !slices.Contains(keys, kept) {
			t.Errorf("a child lacks %s", kept)
		}
	}
}

// TestATestsGoIsOfflineAndKeepsNoCredential: a go a
// test starts runs with no workspace and no module proxy, whatever the run
// had, reads no go env file, and is handed no credential or go setting the
// run was started with: GOAUTH, NETRC, GOFLAGS and GOPRIVATE, set here after
// the isolation began, never reach it. Risk: a test's go that fetches, with
// the person's credentials, or builds against a go.work beside the clone.
func TestATestsGoIsOfflineAndKeepsNoCredential(t *testing.T) {
	// serial: it sets variables of the process, which Clean reads
	t.Setenv("GOAUTH", "planted")
	t.Setenv("NETRC", "planted")
	t.Setenv("GOFLAGS", "planted")
	t.Setenv("GOPRIVATE", "planted")
	env := Inherited()
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if slices.Contains([]string{"GOAUTH", "NETRC", "GOFLAGS", "GOPRIVATE"}, name) {
			t.Errorf("a child inherits %s", kv)
		}
	}
	for _, kv := range []string{"GOENV=off", "GOWORK=off", "GOPROXY=off"} {
		if !slices.Contains(env, kv) {
			t.Errorf("a child lacks %s", kv)
		}
	}
	// go env answers GOENV off as empty, so the file it reads is held above,
	// by the environment the child is handed.
	cmd := exec.CommandContext(t.Context(), "go", "env", "-json", "GOWORK", "GOPROXY", "GOPRIVATE", "GOFLAGS")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("go env -json printed %q: %v", out, err)
	}
	want := map[string]string{"GOWORK": "off", "GOPROXY": "off", "GOPRIVATE": "", "GOFLAGS": ""}
	for name, v := range want {
		if got[name] != v {
			t.Errorf("go env %s in the isolation = %q, want %q", name, got[name], v)
		}
	}
}

// TestAColdModuleCacheIsRedAndNamesTaskTools: a go a test starts
// that needs a module the cache lacks stops, since it never fetches, and
// GoSaid names task tools as the fix. A go that stopped on anything else is
// said as it was, with no fix of the cache. The cache here is an empty one
// of the test's, and the module one no proxy holds. Risk: a cold cache read
// as a fault of the test, or fetched over the network from inside one.
func TestAColdModuleCacheIsRedAndNamesTaskTools(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "go", "mod", "download", "example.com/absent@v1.0.0")
	cmd.Dir = t.TempDir()
	cmd.Env = append(Inherited(), "GOMODCACHE="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("go mod download of a module the cache lacks passed:\n%s", out)
	}
	if said := GoSaid(string(out)); !strings.Contains(said, "GOPROXY=off") || !strings.HasSuffix(said, "run task tools, which fills the cache, and run the tests again") {
		t.Errorf("a cold cache says %q, want go's refusal and the fix naming task tools", said)
	}
	if said := GoSaid("main.go:3:2: undefined: x"); said != "main.go:3:2: undefined: x" {
		t.Errorf("a go that stopped on its build says %q, want it as it was", said)
	}
}

// stateVar is the variable the system's folders read their state folder from:
// appdirs says which, so no name is written here.
func stateVar(t testing.TB) string {
	t.Helper()
	f, err := appdirs.Sources(appdirs.XDG)
	if err != nil {
		t.Fatal(err)
	}
	return f.State
}
