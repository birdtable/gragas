package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// The goreleaser and changie presets are the base a repository copies, so each
// is run through its own tool, pinned in tools.yaml (gragas#1037): a preset its
// own tool refuses is a broken base. The reader of gragas check holds the same
// two files in internal/core/audit; this is the tool's own word on them.

// pinnedProgram is the .tools/bin copy of a tool task tools places, held to
// tools.yaml's pin: by the sha256 of its program when it is a downloaded release
// program, and by its build info against tools/go.sum, as task tools-check holds
// it, when it is built from source (a source build matches no release sha,
// gragas#1288). It puts .tools/bin first on
// PATH for the test, so a command built by name after it is the pinned program.
// It returns the program's path and what tools.yaml says its version run
// prints.
func pinnedProgram(t *testing.T, id string) (bin, says string) {
	t.Helper()
	says, sums, err := toolPin(lintDoc(t, "tools.yaml"), id)
	if err != nil {
		t.Fatal(err)
	}
	bin = pinnedPath(t, id)
	dir := filepath.Dir(bin)
	hold := func() error { return sumPinned(bin, sums) }
	if len(sums) == 0 { // toolPin gives no sums only for a tool built from source
		hold = func() error { return builtPinned(bin) }
	}
	if err := hold(); err != nil {
		t.Fatal(err)
	}
	path := dir
	for _, kv := range configtest.Inherited() {
		if rest, ok := strings.CutPrefix(kv, "PATH="); ok && rest != "" {
			path += string(filepath.ListSeparator) + rest
		}
	}
	t.Setenv("PATH", path)
	return bin, says
}

// runIn runs cmd in dir with the inherited environment and returns its
// combined output. A program built by name must be the one at want, so the
// pinned tool is never a copy found elsewhere on PATH; want "" holds no one.
func runIn(t *testing.T, want, dir string, cmd *exec.Cmd) (string, error) {
	t.Helper()
	if cmd.Err != nil || (want != "" && cmd.Path != want) {
		t.Fatalf("%v is %s, not the pinned %s: %v", cmd.Args, cmd.Path, want, cmd.Err)
	}
	cmd.Dir = dir
	cmd.Env = configtest.Inherited()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// saysItsPin holds a tool's version run to what tools.yaml says.
func saysItsPin(t *testing.T, bin, says string, version *exec.Cmd) {
	t.Helper()
	out, err := runIn(t, bin, "", version)
	if err != nil {
		t.Fatalf("%s does not run:\n%s", bin, out)
	}
	if !strings.Contains(out, says) {
		t.Fatalf("%s does not say tools.yaml's %q:\n%s", bin, says, out)
	}
}

// presetCopy writes the preset at rel as name in a new folder, after edit has
// changed its text (nil leaves it), and returns the folder.
func presetCopy(t *testing.T, rel, name string, edit func(string) string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if edit != nil {
		text = edit(text)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestTheGoreleaserPresetPassesGoreleaserCheck(t *testing.T) {
	bin, says := pinnedProgram(t, "goreleaser")
	saysItsPin(t, bin, says, exec.CommandContext(t.Context(), "goreleaser", "--version"))
	// goreleaser reads the repository's remote to know where it would release.
	check := func(edit func(string) string) (string, error) {
		dir := presetCopy(t, "presets/goreleaser/base.yaml", ".goreleaser.yaml", edit)
		if out, err := runIn(t, "", dir, exec.CommandContext(t.Context(), "git", "init", "-q", ".")); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
		if out, err := runIn(t, "", dir, exec.CommandContext(t.Context(), "git", "remote", "add", "origin", "https://github.com/example/example.git")); err != nil {
			t.Fatalf("git remote add: %v\n%s", err, out)
		}
		return runIn(t, bin, dir, exec.CommandContext(t.Context(), "goreleaser", "check"))
	}
	if out, err := check(nil); err != nil {
		t.Fatalf("goreleaser check refuses the preset:\n%s", out)
	}
	out, err := check(func(s string) string { return s + "\nunknown_key: 1\n" })
	if err == nil || !strings.Contains(out, "field unknown_key not found") {
		t.Errorf("goreleaser check passes a preset with an unknown key (err %v):\n%s", err, out)
	}
}

// changeFileName is the shape supply.release.5 holds a change file to.
var changeFileName = regexp.MustCompile(`^[0-9]+-[a-z][a-z0-9-]*\.ya?ml$`)

// changieFixture makes two changes the way a person makes them, in a folder
// that copied the preset after edit, and returns the folder and the names
// changie gave the files.
func changieFixture(t *testing.T, bin string, edit func(string) string) (string, []string) {
	t.Helper()
	dir := presetCopy(t, "presets/changie/base.yaml", ".changie.yaml", edit)
	if out, err := runIn(t, bin, dir, exec.CommandContext(t.Context(), "changie", "new", "--kind", "added", "--body", "a thing", "--custom", "Issue=931")); err != nil {
		t.Fatalf("changie new refuses the preset:\n%s", out)
	}
	if out, err := runIn(t, bin, dir, exec.CommandContext(t.Context(), "changie", "new", "--kind", "fixed", "--body", "another", "--custom", "Issue=1037")); err != nil {
		t.Fatalf("changie new refuses the preset:\n%s", out)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".changes", "unreleased"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return dir, names
}

func TestTheChangiePresetLoadsAndBatchesItsFixture(t *testing.T) {
	bin, says := pinnedProgram(t, "changie")
	saysItsPin(t, bin, says, exec.CommandContext(t.Context(), "changie", "--version"))
	dir, names := changieFixture(t, bin, nil)
	if len(names) != 2 {
		t.Fatalf("changie wrote %v, want two change files", names)
	}
	for _, n := range names {
		if !changeFileName.MatchString(n) {
			t.Errorf("changie named a change %s, which is not <issue>-<kind>", n)
		}
	}
	out, err := runIn(t, bin, dir, exec.CommandContext(t.Context(), "changie", "batch", "0.1.0", "--dry-run"))
	if err != nil {
		t.Fatalf("changie batch --dry-run:\n%s", out)
	}
	for _, want := range []string{"## 0.1.0", "### Added", "* a thing (#931)", "### Fixed", "* another (#1037)"} {
		if !strings.Contains(out, want) {
			t.Errorf("the batch lacks %q:\n%s", want, out)
		}
	}
	// A kind the preset does not list is refused, not made.
	if out, err := runIn(t, bin, dir, exec.CommandContext(t.Context(), "changie", "new", "--kind", "Added", "--body", "x", "--custom", "Issue=2")); err == nil {
		t.Errorf("changie makes a change of a kind the preset does not list:\n%s", out)
	}
}

// Without fragmentFileFormat changie names a change from its kind and the
// time, the shape supply.release.5 refuses: the fixture holds the line.
func TestAChangiePresetWithoutItsFragmentFormatNamesChangesAgainstTheRule(t *testing.T) {
	bin, says := pinnedProgram(t, "changie")
	saysItsPin(t, bin, says, exec.CommandContext(t.Context(), "changie", "--version"))
	_, names := changieFixture(t, bin, func(s string) string {
		var keep []string
		for line := range strings.SplitSeq(s, "\n") {
			if !strings.HasPrefix(line, "fragmentFileFormat:") {
				keep = append(keep, line)
			}
		}
		return strings.Join(keep, "\n")
	})
	for _, n := range names {
		if changeFileName.MatchString(n) {
			t.Errorf("changie named a change %s with no fragmentFileFormat, so the fixture does not hold the preset's line", n)
		}
	}
}
