package journeys

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// bundleFolder writes the bundle of name at version below a new folder that is
// not the rulebook and answers the version's folder, the one plugin add is
// given, and the folder it lies below, so a transcript can name it.
func bundleFolder(t *testing.T, name, version string, extra map[string]string) (folder, root string) {
	t.Helper()
	files := map[string]string{}
	for p, body := range bundleOf(t, name, version) {
		files[name+"/"+version+"/"+p] = body
	}
	for p, body := range extra {
		files[name+"/"+version+"/"+p] = body
	}
	root = rulebookFrom(t, files)
	return filepath.Join(root, name, version), root
}

var addRunID = regexp.MustCompile(`files written, run [0-9a-f]+`)

// addScrub is a transcript with the rulebook, and the bundle's folder as the
// host may spell it with its links resolved, named by a fixed word.
func addScrub(tr, dir string, bundleRoots ...string) string {
	for _, r := range bundleRoots {
		tr = strings.ReplaceAll(tr, r, "<bundles>")
		if real, err := filepath.EvalSymlinks(r); err == nil {
			tr = strings.ReplaceAll(tr, real, "<bundles>")
		}
	}
	// "3 files written, run 041a7adf5a570f33": the run's id is new each time.
	tr = addRunID.ReplaceAllString(scrub(tr, dir), "files written, run <run>")
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		tr = strings.ReplaceAll(tr, real, "<repo>")
	}
	return tr
}

// TestPluginAddDryRunApplyAndASecondRun: the dry run of a bundle from outside
// the project lists each file and the lock entry, ends on the apply line and
// writes nothing; --apply writes the files below plugins/<name>/<version> and
// then the lock, which names the source as local; a second run says there is
// nothing to write; and the plugin is listed as not approved, since adding
// approves nothing. A bundle from a folder inside the project is locked with
// the path relative to it. Each is held by a golden. Risk: a dry run that
// writes, a lock that carries one person's home folder, or an add that
// approves.
func TestPluginAddDryRunApplyAndASecondRun(t *testing.T) {
	t.Parallel()
	dir := rulebookFrom(t, fixtureCopy(t, "base"))
	env := ruleEnv(t)
	out, outRoot := bundleFolder(t, "lint-a", "1.0.0", nil)
	// A bundle below the project, so its source is a path in it.
	in := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(in, 0o700); err != nil {
		t.Fatal(err)
	}
	_, inRoot := bundleFolder(t, "lint-b", "2.0.0", nil)
	if err := os.Rename(filepath.Join(inRoot, "lint-b"), filepath.Join(in, "lint-b")); err != nil {
		t.Fatal(err)
	}
	inFolder := filepath.Join(in, "lint-b", "2.0.0")
	lock := filepath.Join(dir, "plugins", "lock.yaml")
	steps := []struct {
		name string
		args []string
		gone bool // the lock is still absent after the step
	}{
		{"plugin-add-dry-run", []string{"plugin", "add", out}, true},
		{"json-plugin-add-dry-run", []string{"plugin", "add", out, "--json"}, true},
		{"plugin-add-apply", []string{"plugin", "add", out, "--apply"}, false},
		{"plugin-add-again", []string{"plugin", "add", out, "--apply"}, false},
		{"plugin-add-from-the-project", []string{"plugin", "add", inFolder, "--apply"}, false},
		{"plugin-list-after-add", []string{"plugin", "list"}, false},
	}
	for _, s := range steps {
		r, tr := approvalRun(t, dir, env, s.args...)
		// A dry run ends declined, exit 6, since it changed nothing.
		want := 0
		if s.gone {
			want = 6
		}
		if r.code != want {
			t.Fatalf("%s: exit %d, want %d\n%s", s.name, r.code, want, tr)
		}
		_, err := os.Stat(lock)
		if s.gone && err == nil {
			t.Fatalf("%s: the dry run wrote the lock", s.name)
		}
		uitest.Golden(t, s.name, addScrub(tr, dir, outRoot, inRoot))
	}
	body, err := os.ReadFile(lock)
	if err != nil {
		t.Fatal(err)
	}
	uitest.Golden(t, "plugin-add-lock", string(body))
	for _, p := range []string{"plugins/lint-a/1.0.0/plugin.yaml", "plugins/lint-a/1.0.0/plugin.wasm", "plugins/lint-b/2.0.0/plugin.wasm"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			t.Errorf("the add left no %s: %v", p, err)
		}
	}
}

// TestPluginAddJSONApply: --apply --json is one document on stdout. Risk: an
// add that prints a second document or its words on stdout.
func TestPluginAddJSONApply(t *testing.T) {
	t.Parallel()
	dir := rulebookFrom(t, fixtureCopy(t, "base"))
	env := ruleEnv(t)
	out, outRoot := bundleFolder(t, "lint-a", "1.0.0", nil)
	args := []string{"plugin", "add", out, "--apply", "--json"}
	r, tr := approvalRun(t, dir, env, args...)
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, tr)
	}
	uitest.Golden(t, "json-plugin-add-apply", addScrub(tr, dir, outRoot))
}

// TestPluginAddRefusals: each refusal of an add, at 80 and 40 columns, writes
// nothing: a bundle that is not the plugin its folder names, a file over the
// size a bundle may hold, a version locked at other bytes, and a folder the lock
// does not name. Each is held by a golden and leaves the lock and the folders
// as they were. Risk: a refusal that leaves half a bundle, or one that says
// nothing of where to go on.
func TestPluginAddRefusals(t *testing.T) {
	t.Parallel()
	// A file one byte over what a bundle's file may be (8 MiB).
	big := strings.Repeat("x", 8<<20+1)
	// The folder names one plugin, lint-z, and its manifest another, lint-a.
	misfiled := map[string]string{}
	for p, body := range bundleOf(t, "lint-a", "1.0.0") {
		misfiled["lint-z/1.0.0/"+p] = body
	}
	wrongNameRoot := rulebookFrom(t, misfiled)
	wrongManifest := filepath.Join(wrongNameRoot, "lint-z", "1.0.0")
	tooBig, tooBigRoot := bundleFolder(t, "lint-a", "1.0.0", map[string]string{"notes.txt": big})
	changed, changedRoot := bundleFolder(t, "lint-a", "1.0.0", map[string]string{"notes.txt": "another file"})
	plain, plainRoot := bundleFolder(t, "lint-a", "1.0.0", nil)

	dir := rulebookFrom(t, fixtureCopy(t, "base"))
	env := ruleEnv(t)
	// lint-a@1.0.0 is added first, so a second bundle of it holds other bytes.
	if r := run(t, dir, env, "plugin", "add", plain, "--apply"); r.code != 0 {
		t.Fatalf("the first add: exit %d\n%s", r.code, transcript(nil, r))
	}
	// A folder of lint-c the lock does not name.
	other := rulebookFrom(t, fixtureCopy(t, "base"))
	stray := filepath.Join(other, "plugins", "lint-a", "1.0.0")
	if err := os.MkdirAll(stray, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stray, "plugin.wasm"), []byte("left behind"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		dir  string
		args []string
	}{
		{"refused-wrong-manifest", dir, []string{"plugin", "add", wrongManifest, "--apply"}},
		{"refused-too-big", dir, []string{"plugin", "add", tooBig, "--apply"}},
		{"refused-locked-at-other-bytes", dir, []string{"plugin", "add", changed, "--apply"}},
		{"refused-a-folder-the-lock-does-not-name", other, []string{"plugin", "add", plain, "--apply"}},
	}
	lockBefore, err := os.ReadFile(filepath.Join(dir, "plugins", "lock.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, cols := range []string{"80", "40"} {
		for _, c := range cases {
			env["COLUMNS"] = cols
			r := run(t, c.dir, env, c.args...)
			if r.code == 0 {
				t.Fatalf("%s at %s: exit 0, want a refusal\n%s", c.name, cols, transcript(c.args, r))
			}
			tr := addScrub(transcript(c.args, r), c.dir, wrongNameRoot, tooBigRoot, changedRoot, plainRoot)
			uitest.Golden(t, "plugin-add-"+c.name+"-"+cols, tr)
		}
	}
	lockAfter, err := os.ReadFile(filepath.Join(dir, "plugins", "lock.yaml"))
	if err != nil || string(lockAfter) != string(lockBefore) {
		t.Errorf("a refused add changed the lock: %v\n%s", err, lockAfter)
	}
	for _, p := range []string{"plugins/lint-z", "plugins/lint-a/1.0.0/notes.txt"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err == nil {
			t.Errorf("a refused add left %s", p)
		}
	}
}
