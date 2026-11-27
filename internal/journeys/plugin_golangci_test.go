package journeys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// golangciBundle is the installed golangci-lint bundle of the version the repository
// carries: the folder plugin add is given, below plugins/ of this tree (gragas#2033).
func golangciBundle(version string) string {
	return filepath.Join(moduleRoot(), "plugins", "golangci-lint", version)
}

// toolsPin is the digest tools.yaml pins for name@version, read from the line
// as a person reads it.
func toolsPin(t *testing.T, key string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(moduleRoot(), "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(key) + `: \{.*digest: "(sha256:[0-9a-f]{64})"\}`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatalf("tools.yaml pins no %s", key)
	}
	return m[1]
}

// TestInstallingTheRealGolangciLintPluginAddsLocksApprovesAndListsIt: the
// install commands the final run gives, run on the bundle this tree carries: plugin add
// (dry run, then --apply) writes the folder and a lock row whose digest is the
// one tools.yaml pins; plugin list says it is not approved; plugin approve
// with that digest records it; plugin list says it is. Each screen is a
// golden. Risk: a bundle that locks to another digest than its pin, or an
// add that approves.
func TestInstallingTheRealGolangciLintPluginAddsLocksApprovesAndListsIt(t *testing.T) {
	t.Parallel()
	const key = "golangci-lint@1.0.1"
	pin := toolsPin(t, key)
	dir := rulebookFrom(t, fixtureCopy(t, "base"))
	env := ruleEnv(t)
	bundle := golangciBundle("1.0.1")
	for _, s := range []struct {
		name string
		args []string
		code int
	}{
		{"plugin-add-golangci-dry-run", []string{"plugin", "add", bundle}, 6},
		{"plugin-add-golangci-apply", []string{"plugin", "add", bundle, "--apply"}, 0},
		{"plugin-list-golangci-before-the-yes", []string{"plugin", "list"}, 0},
		{"plugin-approve-golangci-apply", []string{"plugin", "approve", key, "--digest", pin, "--apply"}, 0},
		{"plugin-list-golangci-after-the-yes", []string{"plugin", "list"}, 0},
	} {
		r, tr := approvalRun(t, dir, env, s.args...)
		if r.code != s.code {
			t.Fatalf("%s: exit %d, want %d\n%s", s.name, r.code, s.code, tr)
		}
		tr = addScrub(tr, dir, filepath.Dir(filepath.Dir(bundle)))
		tr = strings.ReplaceAll(tr, pin, "<pin>")
		uitest.Golden(t, s.name, tr)
	}
	body, err := os.ReadFile(filepath.Join(dir, "plugins", "lock.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "digest: \""+pin+"\"") {
		t.Errorf("the lock does not hold the pin %s:\n%s", pin, body)
	}
	for _, p := range []string{"plugin.yaml", "plugin.wasm", "testdata/recordings/2.15.0/0-0.stdout"} {
		if _, err := os.Stat(filepath.Join(dir, "plugins", "golangci-lint", "1.0.1", filepath.FromSlash(p))); err != nil {
			t.Errorf("the add left no %s: %v", p, err)
		}
	}
}

// TestTheLocksDigestOfEachInstalledPluginIsTheToolsFilesPin: for every
// plugin the lock holds, plugin list --json gives the digest the lock holds
// and tools.yaml pins. Risk: a lock or a pin edited alone.
func TestTheLocksDigestOfEachInstalledPluginIsTheToolsFilesPin(t *testing.T) {
	t.Parallel()
	dir := rulebookFrom(t, fixtureCopy(t, "base"))
	copyTree(t, filepath.Join(moduleRoot(), "plugins"), filepath.Join(dir, "plugins"))
	env, _ := folders(t)
	r := run(t, dir, env, "plugin", "list", "--json")
	var l struct {
		Rows []struct{ Name, Version, Digest string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &l); err != nil || len(l.Rows) == 0 {
		t.Fatalf("plugin list --json: %v\n%s%s", err, r.stdout, r.stderr)
	}
	for _, row := range l.Rows {
		if got := toolsPin(t, row.Name+"@"+row.Version); got != row.Digest {
			t.Errorf("%s@%s: the lock reads %s, tools.yaml pins %s", row.Name, row.Version, row.Digest, got)
		}
	}
}

// TestAFreshCloneWithTheRealPluginsInstalledLoadsNoneAndItsCheckIsNotReady
// (gragas#2033): a rulebook repository that carries the installed golangci-lint
// plugin, locked and pinned, on a machine that approved nothing: plugin list
// says it is not approved, and the check ends not ready (exit 4), naming the
// tool and the approval, never clean. Risk: a plugin that loads from the clone
// without an approval, which would let a clone choose what runs.
func TestAFreshCloneWithTheRealPluginsInstalledLoadsNoneAndItsCheckIsNotReady(t *testing.T) {
	t.Parallel()
	repo := filepath.Join(t.TempDir(), "repo")
	copyTree(t, oldBook(), filepath.Join(repo, "rulebook"))
	copyTree(t, filepath.Join(moduleRoot(), "plugins"), filepath.Join(repo, "plugins"))
	for path, body := range map[string]string{
		".gragas/parts": "go\n",
		".golangci.yml": "version: \"2\"\nlinters:\n  default: none\n",
	} {
		p := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env := ruleEnv(t)
	env["COLUMNS"] = "200"
	r := run(t, filepath.Dir(repo), env, "check", "-C", repo)
	said := strings.Join(strings.Fields(r.stdout+r.stderr), " ")
	for _, want := range []string{"not ready", "golangci-lint rules were not checked", "the plugin is not approved: sha256:", "gragas tools"} {
		if !strings.Contains(said, want) {
			t.Errorf("the check does not say %q\n%s%s", want, r.stdout, r.stderr)
		}
	}
	if r.code != 4 || strings.Contains(said, "  clean") {
		t.Errorf("exit %d, want 4 (not ready), never clean\n%s%s", r.code, r.stdout, r.stderr)
	}
	l := run(t, repo, env, "plugin", "list", "--json")
	var list struct {
		Rows []struct {
			Name     string
			Approved bool
		}
	}
	if err := json.Unmarshal([]byte(l.stdout), &list); err != nil || len(list.Rows) == 0 {
		t.Fatalf("plugin list --json: %v\n%s%s", err, l.stdout, l.stderr)
	}
	for _, row := range list.Rows {
		if row.Approved {
			t.Errorf("%s reads as approved on a machine that approved nothing", row.Name)
		}
	}
}

// TestInstallingTheOtherThreeRealPluginsAddsLocksApprovesAndListsEach
// (gragas#2033): ast-grep 1.1.0, eslint 1.0.0 and tsc 1.0.0, from the folders
// this tree carries, with golangci-lint 1.0.1: each is added by plugin add
// and locked at the digest tools.yaml pins, is listed not approved, and is
// listed approved after plugin approve with that digest; the screen of the
// four approved is a golden. Risk: a bundle that does not install, or one
// version that reads as another's.
func TestInstallingTheOtherThreeRealPluginsAddsLocksApprovesAndListsEach(t *testing.T) {
	t.Parallel()
	keys := []string{"ast-grep@1.1.0", "eslint@1.0.0", "golangci-lint@1.0.1", "tsc@1.0.0"}
	dir := rulebookFrom(t, fixtureCopy(t, "base"))
	env := ruleEnv(t)
	listed := func() map[string]bool {
		r := run(t, dir, env, "plugin", "list", "--json")
		var l struct {
			Rows []struct {
				Name, Version string
				Approved      bool
			}
		}
		if err := json.Unmarshal([]byte(r.stdout), &l); err != nil {
			t.Fatalf("plugin list --json: %v\n%s%s", err, r.stdout, r.stderr)
		}
		m := map[string]bool{}
		for _, row := range l.Rows {
			m[row.Name+"@"+row.Version] = row.Approved
		}
		return m
	}
	for _, key := range keys {
		name, version, _ := strings.Cut(key, "@")
		r := run(t, dir, env, "plugin", "add", filepath.Join(moduleRoot(), "plugins", name, version), "--apply")
		if r.code != 0 {
			t.Fatalf("plugin add %s: exit %d\n%s%s", key, r.code, r.stdout, r.stderr)
		}
	}
	got := listed()
	for _, key := range keys {
		if approved, ok := got[key]; !ok || approved {
			t.Errorf("%s after the add: listed %v, approved %v, want listed and not approved", key, ok, approved)
		}
	}
	body, err := os.ReadFile(filepath.Join(dir, "plugins", "lock.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		pin := toolsPin(t, key)
		if !strings.Contains(string(body), "digest: \""+pin+"\"") {
			t.Errorf("the lock does not hold the pin of %s, %s:\n%s", key, pin, body)
		}
		r := run(t, dir, env, "plugin", "approve", key, "--digest", pin, "--apply")
		if r.code != 0 {
			t.Fatalf("plugin approve %s: exit %d\n%s%s", key, r.code, r.stdout, r.stderr)
		}
	}
	got = listed()
	for _, key := range keys {
		if !got[key] {
			t.Errorf("%s after the yes: not approved", key)
		}
	}
	r, tr := approvalRun(t, dir, env, "plugin", "list")
	if r.code != 0 {
		t.Fatalf("plugin list: exit %d\n%s", r.code, tr)
	}
	uitest.Golden(t, "plugin-list-the-four-after-the-yes", tr)
}
