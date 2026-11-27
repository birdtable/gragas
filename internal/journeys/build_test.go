package journeys

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// installedInto is gragas built by a go install of its own, stamped as the
// goldens hold it, into dir alone, and the path of the binary there. The kit
// builds once and keeps the binary in one place, so the build a test compares
// with another, made in another folder, is made here; the arguments are
// constants and the folder goes by GOBIN.
func installedInto(t *testing.T, dir string) string {
	t.Helper()
	c := exec.CommandContext(t.Context(), "go", "install", "-trimpath", "-ldflags", "-X main.version="+stampedVersion+" -X main.commit="+stampedCommit, "./cmd/gragas")
	c.Dir = moduleRoot()
	c.Env = append(configtest.Inherited(), "GOBIN="+dir)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("could not build gragas into %s: %v\n%s", dir, err, configtest.GoSaid(string(out)))
	}
	return filepath.Join(dir, "gragas"+filepath.Ext(stamped().Path()))
}

// TestHelpAndVersionJSONAtThreeWidths: help and version under --json are one
// document that names the contract in one top-level field and no schema number,
// the same at 80, 60 and 40 columns because a document is not wrapped, each
// compared with its golden and never rewritten by the gate (cli.output.1,
// cli.output.3). Risk: a field renamed or a width leaking into a document.
func TestHelpAndVersionJSONAtThreeWidths(t *testing.T) {
	t.Parallel()
	repo := fixture(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"help", []string{"help", "--json"}},
		{"version", []string{"version", "--json"}},
		{"help-check", []string{"check", "--help", "--json"}},
	} {
		var first string
		for _, cols := range []string{"80", "60", "40"} {
			t.Run(tc.name+"-"+cols, func(t *testing.T) {
				r := run(t, repo, map[string]string{"COLUMNS": cols}, tc.args...)
				if r.code != 0 {
					t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
				}
				var doc map[string]any
				if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
					t.Fatalf("stdout is not one document: %v\n%s", err, r.stdout)
				}
				if doc["contract"] != "1.3" || doc["schema"] != nil {
					t.Errorf("contract = %v and schema = %v, want contract 1.3 and no schema", doc["contract"], doc["schema"])
				}
				got := scrub(r.stdout, "")
				if first == "" {
					first = got
				} else if got != first {
					t.Errorf("the document moves with the width")
				}
				uitest.Golden(t, "json-"+tc.name+"-"+cols, got)
			})
		}
	}
}

// TestTwoBuildsFromTwoFoldersPrintTheSameBytes: the same code built twice, into two
// folders prints the same version, so the build folder is in no answer
// (go.layout.4, cli.exit.3). Risk: a path or a timestamp in the version.
func TestTwoBuildsFromTwoFoldersPrintTheSameBytes(t *testing.T) {
	t.Parallel()
	da, db := t.TempDir(), t.TempDir()
	a, b := printed(t, installedInto(t, da), da, "version"), printed(t, installedInto(t, db), db, "version")
	if a != b {
		t.Errorf("two builds print different bytes:\n%s\n%s", a, b)
	}
	if !strings.Contains(a, "tag       v1.2.3") {
		t.Errorf("a stamped build does not print its tag:\n%s", a)
	}
}

// TestABuildWithNoTagSaysSo: a build nobody stamped prints that no release
// tag reaches it, and a --json answer holds a null tag with the reason
// (cli.exit.3, cli.output.9). Risk: a build that passes for a release.
func TestABuildWithNoTagSaysSo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := buildInto(t, dir)
	if out := printed(t, bin, dir, "version"); !strings.Contains(out, "tag       none, no release tag reaches this build") {
		t.Errorf("an unstamped build does not say it has no tag:\n%s", out)
	}
	out := printed(t, bin, dir, "version", "--json")
	var doc struct {
		Tag   *string           `json:"tag"`
		Notes map[string]string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Tag != nil || doc.Notes["tag"] == "" {
		t.Errorf("want a null tag with a reason, got %v and %v", doc.Tag, doc.Notes)
	}
}
