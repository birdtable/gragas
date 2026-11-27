package internal

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The two flags, built from parts so that this file is no use of either.
const (
	serialRunners   = "--allow-" + "serial-runners"
	parallelRunners = "--allow-" + "parallel-runners"
)

// lintRunnerFindings is what is wrong with how files, each a Go or Taskfile
// source by its path, start golangci-lint (gates.command.22): a use of the
// flag that waits on a host-wide lock with no deadline, and a Taskfile line
// that runs the linter without the flag that takes no lock. A Go call that
// lacks the flag is held at run time, by the lint helper of contexts_test.go.
func lintRunnerFindings(files map[string]string) []string {
	var found []string
	for _, name := range slices.Sorted(mapKeys(files)) {
		for i, line := range strings.Split(files[name], "\n") {
			at := name + ":" + itoa(i+1)
			if strings.Contains(line, serialRunners) {
				found = append(found, at+": "+serialRunners+" waits on a host-wide lock with no deadline; use "+parallelRunners)
			}
			if name == "Taskfile.yml" && strings.Contains(line, "golangci-lint run") && !strings.Contains(line, parallelRunners) {
				found = append(found, at+": golangci-lint runs without "+parallelRunners)
			}
		}
	}
	return found
}

func mapKeys(m map[string]string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

func itoa(n int) string {
	var b [20]byte
	i := len(b)
	for ; n >= 10; n /= 10 {
		i--
		b[i] = byte('0' + n%10)
	}
	i--
	b[i] = byte('0' + n)
	return string(b[i:])
}

// lintSources is every tracked Go file and Taskfile of the tree, by path.
func lintSources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, rel := range trackedIn(t, root) {
		if !strings.HasSuffix(rel, ".go") && path.Base(rel) != "Taskfile.yml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, rel))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		out[rel] = string(data)
	}
	return out
}

func TestNoLintStartsWithTheSerialRunnersFlag(t *testing.T) {
	t.Parallel()
	src := lintSources(t)
	if _, ok := src["Taskfile.yml"]; !ok {
		t.Fatal("the tree holds no Taskfile to read, so the test would pass on nothing")
	}
	if found := lintRunnerFindings(src); len(found) != 0 {
		t.Errorf("a lint starts in a way gates.command.22 refuses:\n%s", strings.Join(found, "\n"))
	}
}

func TestAPlantedSerialRunnersFlagOrABareMakeLintIsSeen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"a Go call with the serial flag", map[string]string{"a_test.go": "exec.Command(\"golangci-lint\", \"run\", \"" + serialRunners + "\")\n"}, "a_test.go:1: " + serialRunners},
		{"a Taskfile line with the serial flag", map[string]string{"Taskfile.yml": "lint:\n\tgolangci-lint run " + serialRunners + " ./...\n"}, "Taskfile.yml:2: " + serialRunners},
		{"a Taskfile lint with neither flag", map[string]string{"Taskfile.yml": "lint:\n\tgolangci-lint run ./...\n"}, "Taskfile.yml:2: golangci-lint runs without " + parallelRunners},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			found := lintRunnerFindings(tc.files)
			if len(found) == 0 || !strings.HasPrefix(found[0], tc.want) {
				t.Errorf("findings = %q; want the first to start %q", found, tc.want)
			}
		})
	}
	control := map[string]string{
		"Taskfile.yml": "lint:\n\tgolangci-lint run " + parallelRunners + " ./...\n",
		"a_test.go":    "exec.Command(\"golangci-lint\", \"run\", \"" + parallelRunners + "\")\n",
	}
	if found := lintRunnerFindings(control); len(found) != 0 {
		t.Errorf("the control, both with the parallel flag, has findings: %q", found)
	}
}
