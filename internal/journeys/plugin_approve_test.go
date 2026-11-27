package journeys

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
)

// approvalRun runs the binary in dir at 200 columns, so a digest is on one line,
// and answers the transcript of it scrubbed of the folder.
func approvalRun(t *testing.T, dir string, env map[string]string, args ...string) (result, string) {
	t.Helper()
	e := map[string]string{"COLUMNS": "200"}
	maps.Copy(e, env)
	r := run(t, dir, e, args...)
	return r, scrub(transcript(args, r), dir)
}

// onePlugin is a rulebook that holds one locked plugin, lint-a at 1.0.0, and
// the state folders of a machine that approved nothing.
func onePlugin(t *testing.T) (dir string, env map[string]string, d string) {
	t.Helper()
	dir, ds := pluginRulebook(t, lockRow{name: "lint-a", version: "1.0.0", source: "../lint-a"})
	env, _ = folders(t)
	return dir, env, ds["lint-a@1.0.0"]
}

// TestPluginApproveShowsWhatItCoversAndWritesOnlyWithApply: the dry run shows
// the name and version, the digest, the tool and its range, every grant, the
// preset path, resolve_from and the install words, and ends on the line to
// type; --json is one document on stdout; --apply with the digest shown
// records it and a second dry run says it is recorded. Each is held by a
// golden. Risk: a screen that hides a grant, or a dry run that records.
func TestPluginApproveShowsWhatItCoversAndWritesOnlyWithApply(t *testing.T) {
	t.Parallel()
	dir, env, d := onePlugin(t)
	steps := []struct {
		name string
		args []string
		code int
	}{
		{"json-plugin-approve-dry-run", []string{"plugin", "approve", "lint-a", "--json"}, 0},
		{"plugin-list-before-the-yes", []string{"plugin", "list"}, 0},
		{"plugin-approve-apply", []string{"plugin", "approve", "lint-a@1.0.0", "--digest", d, "--apply"}, 0},
		{"plugin-list-after-the-yes", []string{"plugin", "list"}, 0},
		{"json-plugin-approve-apply", []string{"plugin", "approve", "lint-a", "--digest", d, "--apply", "--json"}, 0},
		{"json-plugin-list-after-the-yes", []string{"plugin", "list", "--json"}, 0},
	}
	for _, s := range steps {
		r, tr := approvalRun(t, dir, env, s.args...)
		if r.code != s.code {
			t.Fatalf("%s: exit %d, want %d\n%s", s.name, r.code, s.code, tr)
		}
		uitest.Golden(t, s.name, strings.ReplaceAll(tr, d, "<digest>"))
	}
}

// TestPluginApproveOfOneVersionLeavesTheNextUnapproved: a new version of an
// approved plugin is unapproved until it is approved itself, a bare name that
// two versions hold is refused with the versions named, and a bundle changed
// since it was locked, its grants alone or its module alone, is listed with a
// note, never shown for a yes. Risk: an approval that carries to bytes it did
// not show.
func TestPluginApproveOfOneVersionLeavesTheNextUnapproved(t *testing.T) {
	t.Parallel()
	dir, ds := pluginRulebook(t,
		lockRow{name: "lint-a", version: "1.0.0", source: "../lint-a"},
		lockRow{name: "lint-a", version: "1.1.0", source: "../lint-a"},
		lockRow{name: "lint-b", version: "1.0.0", source: "../lint-b"},
		lockRow{name: "lint-c", version: "1.0.0", source: "../lint-c"})
	env, _ := folders(t)
	d := ds["lint-a@1.0.0"]
	// lint-b's grants change and its module does not; lint-c's module changes.
	b := filepath.Join(dir, "plugins", "lint-b", "1.0.0", "plugin.yaml")
	body, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(strings.Replace(string(body), `"warning": true`, `"warning": false`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins", "lint-c", "1.0.0", "plugin.wasm"), []byte("another module"), 0o600); err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		name string
		args []string
		fail bool
	}{
		{"plugin-approve-one-version", []string{"plugin", "approve", "lint-a@1.0.0", "--digest", d, "--apply"}, false},
		{"plugin-list-a-new-version-is-unapproved", []string{"plugin", "list"}, false},
		{"json-plugin-list-a-new-version-is-unapproved", []string{"plugin", "list", "--json"}, false},
	}
	for _, s := range steps {
		r, tr := approvalRun(t, dir, env, s.args...)
		if (r.code != 0) != s.fail {
			t.Fatalf("%s: exit %d, want failing %v\n%s", s.name, r.code, s.fail, tr)
		}
		for k, v := range ds {
			tr = strings.ReplaceAll(tr, v, "<"+k+">")
		}
		uitest.Golden(t, s.name, tr)
	}
}

// TestPluginListInAFolderWithNoLockAndInAClone: a rulebook with no plugin lock
// lists none and ends done, and a plugin a clone carries, locked and not
// approved on this machine, is listed no, as is any other. Risk: a clone's
// plugin that reads as approved, or an absent lock read as an error. The
// clone's golden equals plugin-list-before-the-yes.golden on purpose: both are
// a machine with no approval record at all, and a clone is exactly that, so
// the two pin that one screen from two stories, not a duplicate by accident.
func TestPluginListInAFolderWithNoLockAndInAClone(t *testing.T) {
	t.Parallel()
	env, _ := folders(t)
	bare := rulebookFrom(t, fixtureCopy(t, "base"))
	clone, _, d := onePlugin(t)
	for _, s := range []struct {
		name, dir string
		args      []string
	}{
		{"plugin-list-none", bare, []string{"plugin", "list"}},
		{"json-plugin-list-none", bare, []string{"plugin", "list", "--json"}},
		{"plugin-list-a-clone", clone, []string{"plugin", "list"}},
	} {
		r, tr := approvalRun(t, s.dir, env, s.args...)
		if r.code != 0 {
			t.Fatalf("%s: exit %d\n%s", s.name, r.code, tr)
		}
		uitest.Golden(t, s.name, strings.ReplaceAll(tr, d, "<digest>"))
	}
}
