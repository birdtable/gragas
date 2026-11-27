package coreapi_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
)

// cleanWith is the tier contract of a copy of the clean fixture in which the
// member `{cmd: go vet ./..., timeout: 5m}` of pre-push is replaced by with.
func cleanWith(t *testing.T, with string) coreapi.Conformance {
	t.Helper()
	src := filepath.Join("testdata", "tiers", "clean")
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(src, "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	const member = "{cmd: go vet ./..., timeout: 5m}"
	if !strings.Contains(string(b), member) {
		t.Fatalf("the clean fixture has no member %s to replace", member)
	}
	text := strings.Replace(string(b), member, with, 1)
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	w := embedded(t)
	tt := coreapi.LoadTiers(w.Ports().Assets)
	rd := coreapi.LoadReaders(w.Ports().Assets)
	if tt.Refused != "" || rd.Refused != "" {
		t.Fatalf("the embedded data is refused: %q %q", tt.Refused, rd.Refused)
	}
	return coreapi.TierContract(os.DirFS(dir).(fs.ReadFileFS), rd.Rows, tt, "2026-11-15")
}

// TestAMemberThatCannotFailIsARunsFindingNamingTheTaskfileTheTaskAndTheMember:
// each form of a member that cannot fail in the pre-push task of the clean
// fixture gives one runs finding, with the file, the task and the member, and the plain member of the
// clean fixture gives none. Each row is its own plant: take the form out of the
// check and the row turns red.
func TestAMemberThatCannotFailIsARunsFindingNamingTheTaskfileTheTaskAndTheMember(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, member, form string }{
		{"|| true", "{cmd: go vet ./... || true, timeout: 5m}", "|| true"},
		{"|| :", "{cmd: go vet ./... || :, timeout: 5m}", "|| :"},
		{"set -n", `{cmd: "set -n; go vet ./...", timeout: 5m}`, "set -n"},
		{"|| echo", `{cmd: "go vet ./... || echo failed", timeout: 5m}`, "|| echo"},
		{"a pipe to tee, with no pipefail", `{cmd: "go vet ./... | tee vet.log", timeout: 5m}`, "| tee"},
		{"set +e and a last statement that cannot fail", `{cmd: "set +e; go vet ./...; true", timeout: 5m}`, "set +e"},
	} {
		r := cleanWith(t, c.member)
		lines := findingLines(r)
		if len(r.Findings) != 1 || !strings.HasPrefix(lines[0], "Taskfile.yml:") || !strings.Contains(lines[0], "gates.command.1") ||
			!strings.Contains(lines[0], "pre-push") || !strings.Contains(lines[0], "cannot fail ("+c.form+")") || !strings.Contains(lines[0], "go vet ./...") {
			t.Errorf("%s: findings %q, want one runs finding naming Taskfile.yml, pre-push and the form %q", c.name, lines, c.form)
		}
	}
	if r := cleanWith(t, "{cmd: go vet ./..., timeout: 5m}"); len(r.Findings) != 0 {
		t.Errorf("the plain member: findings %q, want none", findingLines(r))
	}
}

// TestAMemberTheCheckCannotJudgeIsSaidNotCheckedAndNeverPassed: a form that would
// end the member green inside a condition is no finding and no pass: the check
// says it did not read it.
func TestAMemberTheCheckCannotJudgeIsSaidNotCheckedAndNeverPassed(t *testing.T) {
	t.Parallel()
	r := cleanWith(t, `{cmd: "if go vet ./...; then :; else exit 0; fi", timeout: 5m}`)
	said := false
	for _, n := range r.NotChecked {
		if strings.Contains(n.String(), "was not read for a failure it cannot make") && strings.Contains(n.String(), "pre-push") {
			said = true
		}
	}
	if len(r.Findings) != 0 || !said {
		t.Errorf("findings %q, not checked %v; want no finding and the member said not read", findingLines(r), r.NotChecked)
	}
}

// TestAMemberThatGoTaskEndsRedIsNoFinding: the members the first reading took
// for unable to fail, which go-task 3.54.0 runs with errexit on and ends red
// (`false; exit 0` exits 201), and a pipe to tee in a task that sets pipefail.
func TestAMemberThatGoTaskEndsRedIsNoFinding(t *testing.T) {
	t.Parallel()
	for name, member := range map[string]string{
		"exit 0 after a statement that can fail": `{cmd: "go vet ./...; exit 0", timeout: 5m}`,
		"|| true then a statement that fails":    `{cmd: "go vet ./... || true; go build ./...", timeout: 5m}`,
		"set +e and a last statement that fails": `{cmd: "set +e; go vet ./...", timeout: 5m}`,
		"a pipe to tee with pipefail":            `{cmd: "go vet ./... | tee vet.log", timeout: 5m, set: [pipefail]}`,
	} {
		if r := cleanWith(t, member); len(r.Findings) != 0 {
			t.Errorf("%s: findings %q, want none", name, findingLines(r))
		}
	}
}
