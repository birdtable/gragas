package internal

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// goListOf is what `go list ./...` says of the module of dir under a system: the packages it names, and
// the files it ignores in them, each with its package. A file a one-system name suffix or a build line
// keeps out of the system's build is ignored in a package that is still named; a package whose every
// file is for another system is not named at all, so the package lists of two systems differ
// (gragas#1710, M3). It asks Go, never the file names as text. A list that names no package read
// nothing, which is an error and never an empty answer. An empty platform is the host's.
func goListOf(ctx context.Context, dir, platform string) (packages, ignored []string, err error) {
	const format = `{{.ImportPath}}{{if or .IgnoredGoFiles .IgnoredOtherFiles}} IGNORED {{.IgnoredGoFiles}}{{.IgnoredOtherFiles}}{{end}}`
	cmd := exec.CommandContext(ctx, "go", "list", "-f", format, "./...")
	cmd.Dir = dir
	cmd.Env = append(configtest.Inherited(), "CGO_ENABLED=0")
	if goos, goarch, ok := strings.Cut(platform, "/"); ok {
		cmd.Env = append(cmd.Env, "GOOS="+goos, "GOARCH="+goarch)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("go list under %q: %w", platform, err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		pkg, files, found := strings.Cut(line, " IGNORED ")
		packages = append(packages, pkg)
		if found {
			ignored = append(ignored, pkg+" "+files)
		}
	}
	if len(packages) == 0 {
		return nil, nil, fmt.Errorf("go list under %q named no package: it read nothing", platform)
	}
	slices.Sort(packages)
	return packages, ignored, nil
}

// leftOut is every way one lint run could read less than another: under each platform of the list and
// under the host, a file ignored in a named package, and a package the system does not name that the
// first one did.
func leftOut(ctx context.Context, dir string, platforms []string) ([]string, error) {
	var problems []string
	var base []string
	for i, platform := range append([]string{""}, platforms...) {
		packages, ignored, err := goListOf(ctx, dir, platform)
		if err != nil {
			return nil, err
		}
		for _, f := range ignored {
			problems = append(problems, fmt.Sprintf("under %q go list ignores %s", platform, f))
		}
		if i == 0 {
			base = packages
			continue
		}
		for _, p := range base {
			if !slices.Contains(packages, p) {
				problems = append(problems, fmt.Sprintf("under %q go list names no package %s", platform, p))
			}
		}
		for _, p := range packages {
			if !slices.Contains(base, p) {
				problems = append(problems, fmt.Sprintf("under %q go list names %s, which the host does not", platform, p))
			}
		}
	}
	return problems, nil
}

// TestNoFileIsLeftOut holds that one lint run reads every file (gragas#1710, M3): for the host and for
// each system and architecture pair of CROSS, go list names the same packages and no ignored file in any
// of them, so a lint run of one system reads what a run of any other would. Planted failure: a file with
// a one-system name suffix, a file with a build line, and a whole package of one system or one
// architecture each turn the check red and are named; a folder with no package is refused, never a pass
// on nothing read.
func TestNoFileIsLeftOut(t *testing.T) {
	t.Parallel()
	cross := strings.Split(strings.Trim(taskVar(t, "CROSS"), "[]"), ", ")
	if len(cross) < 2 {
		t.Fatalf("CROSS names %d systems: the check read too few", len(cross))
	}
	problems, err := leftOut(t.Context(), root, cross)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Errorf("%s: one lint run would not read it", p)
	}
	plant := func(files map[string]string) string {
		dir := t.TempDir()
		files["go.mod"] = "module plant\n\ngo 1.24\n"
		for name, body := range files {
			put(t, dir, name, []byte(body))
		}
		return dir
	}
	one := "package p\n"
	for _, tc := range []struct {
		name, file string
		files      map[string]string
	}{
		{"a one-system name suffix", "x_windows.go", map[string]string{"ok.go": one, "x_windows.go": one}},
		{"a build line", "y.go", map[string]string{"ok.go": one, "y.go": "//go:build linux\n\npackage p\n"}},
		{"a whole one-system package", "w", map[string]string{"ok.go": one, "w/w_windows.go": "package w\n"}},
		{"a whole one-architecture package", "a", map[string]string{"ok.go": one, "a/a.go": "//go:build arm64\n\npackage a\n"}},
	} {
		got, err := leftOut(t.Context(), plant(tc.files), []string{"linux/amd64", "darwin/arm64", "windows/amd64"})
		if err != nil || !strings.Contains(strings.Join(got, "\n"), tc.file) {
			t.Errorf("a plant of %s: %v, %v; want it named", tc.name, got, err)
		}
	}
	if got, err := leftOut(t.Context(), plant(map[string]string{"ok.go": one}), []string{"linux/amd64"}); err != nil || len(got) != 0 {
		t.Errorf("a module with nothing left out: %v, %v; want none", got, err)
	}
	if _, err := leftOut(t.Context(), plant(map[string]string{}), []string{"linux/amd64"}); err == nil {
		t.Error("go list answering no package passed: it read nothing")
	}
}
