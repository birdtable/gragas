package main

import (
	"bytes"
	"context"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/newcommand"
	"github.com/birdtable/gragas/internal/dev/templates/command"
	"github.com/birdtable/gragas/internal/exit"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// plannedRow is the planned row of decision as it stood while the command was
// planned: a group with two verbs not yet built. The repository holds no planned
// row of a group (decision, the last, is built), so the test owns this one and
// writes it into its folder, where the generator reads it as a seed.
const plannedRow = `name: decision
order: 60
context: R
ends: plan
summary: "Record a decision or supersede one"
then: [new, supersede]
examples:
  - {line: "gragas decision new", does: "record a decision"}
  - {line: "gragas decision supersede <slug>", does: "supersede a decision by a new one"}
writes: ["decisions.yaml", "history.jsonl"]
exits: ["done", "declined"]
json: plan
`

// tree is the repository's command table and the planned row above, in a folder
// of the test's own.
func tree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "commands.md"))
	if err != nil {
		t.Fatal(err)
	}
	for f, body := range map[string][]byte{"docs/commands.md": b, "internal/cli/planned/decision.yaml": []byte(plannedRow)} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func do(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, diag bytes.Buffer
	code := run(context.Background(), args, &out, &diag)
	return code, out.String(), diag.String()
}

func files(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestADryRunWritesNothingAndApplyWritesTheFiles(t *testing.T) {
	t.Parallel()
	dir := tree(t)
	before := files(t, dir)
	code, out, _ := do(t, "command", "-C", dir, "decision", "restore")
	if code != exit.Done || !strings.Contains(out, "add --apply") || files(t, dir) != before {
		t.Fatalf("a dry run: code %d, wrote %d files, said %q", code, files(t, dir)-before, out)
	}
	code, out, diag := do(t, "command", "-C", dir, "--apply", "decision", "restore")
	if code != exit.Done {
		t.Fatalf("apply: code %d: %s", code, diag)
	}
	for _, g := range []string{"internal/cli/root/testdata/help-commands.golden", "internal/cli/root/testdata/readme-table.golden"} {
		if !strings.Contains(out, g) {
			t.Errorf("the closing text does not name %s:\n%s", g, out)
		}
	}
	if files(t, dir) != before+5 {
		t.Errorf("apply left %d more files, want 5 (6 new, the planned row removed)", files(t, dir)-before)
	}
	if _, err := os.Stat(filepath.Join(dir, "internal/cli/planned/decision.yaml")); err == nil {
		t.Error("the planned row of decision is still there: the command is built")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "internal/cli/cmd/decision/command.go"))
	if !strings.Contains(string(b), "NotYet") || !strings.Contains(string(b), "package decision") {
		t.Errorf("the package of decision does not keep the planned verbs not yet built:\n%s", b)
	}
	if code, _, diag := do(t, "command", "-C", dir, "--apply", "decision", "restore"); code != exit.Failed || !strings.Contains(diag, "already declared") && !strings.Contains(diag, "exists") {
		t.Errorf("a second run was taken: %d %q", code, diag)
	}
}

func TestANameOffTheListOrTakenIsRefusedAndNothingIsWritten(t *testing.T) {
	t.Parallel()
	dir := tree(t)
	before := files(t, dir)
	for _, c := range []struct {
		args []string
		says string
	}{
		{[]string{"get"}, "refused"},
		{[]string{"frobnicate"}, "base list"},
		{[]string{"explain"}, "already declared"},
		{[]string{"rule", "new"}, "already declared"},
	} {
		code, _, diag := do(t, append([]string{"command", "-C", dir, "--apply"}, c.args...)...)
		if code != exit.Failed || !strings.Contains(diag, c.says) {
			t.Errorf("%v: code %d, said %q", c.args, code, diag)
		}
	}
	if files(t, dir) != before {
		t.Error("a refusal wrote a file")
	}
}

func TestUsage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"frob"}, {"command"}, {"command", "a", "b", "c"}, {"check", "extra"}} {
		if code, _, _ := do(t, args...); code != exit.Usage {
			t.Errorf("%v: code %d, want usage", args, code)
		}
	}
}

// TestTheGoldenExampleIsWhatTheTemplatesRender: the files under the golden
// folder are byte for byte what the templates make for the sample name, and
// no other file is there. With -update it writes them.
func TestTheGoldenExampleIsWhatTheTemplatesRender(t *testing.T) {
	t.Parallel()
	sample := newcommand.Name{Group: command.SampleGroup, Verb: command.SampleVerb}
	if flag.Lookup("update").Value.String() == "true" {
		files, err := newcommand.Render(command.Files, sample)
		if err != nil {
			t.Fatal(err)
		}
		golden := filepath.Join("..", "..", "internal", "dev", "templates", "command", "golden")
		if err := os.RemoveAll(golden); err != nil {
			t.Fatal(err)
		}
		for path, text := range files {
			full := filepath.Join(golden, filepath.FromSlash(path)+".golden")
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	drift, err := newcommand.Drift(command.Files, command.Golden, sample)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range drift {
		t.Error(d)
	}
}

// planOf is the plan for decision restore over dir, from the real templates.
func planOf(t *testing.T, dir string) newcommand.Plan {
	t.Helper()
	rows, err := plannedRows(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	seed := seedOf(rows["decision"])
	p, err := newcommand.Make(command.Files, os.DirFS(dir), newcommand.Name{Group: "decision", Verb: "restore"}, &seed)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestWriteNeverOverwritesAFileAndLeavesNoneOfItsPlanBehind: a file planted at
// the last path the plan writes is refused, its bytes stay, and the files made
// before it are removed (O_EXCL and the clean-up, held by hand-breaking each).
func TestWriteNeverOverwritesAFileAndLeavesNoneOfItsPlanBehind(t *testing.T) {
	t.Parallel()
	dir := tree(t)
	p := planOf(t, dir)
	paths := slices.Sorted(maps.Keys(p.Files))
	last := paths[len(paths)-1]
	full := filepath.Join(dir, filepath.FromSlash(last))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := write(dir, p); err == nil {
		t.Fatal("a planted file was overwritten without a word")
	}
	if b, _ := os.ReadFile(full); string(b) != "mine\n" {
		t.Errorf("the planted file now holds %q", b)
	}
	for _, other := range paths[:len(paths)-1] {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(other))); err == nil {
			t.Errorf("%s was made and kept by a run that failed", other)
		}
	}
}

// TestAFailureLaterInTheWriteRemovesEveryFileMade: the planned file is a folder with something in it,
// so its removal, the last step, fails, and each file made before it goes.
func TestAFailureLaterInTheWriteRemovesEveryFileMade(t *testing.T) {
	t.Parallel()
	dir := tree(t)
	p := planOf(t, dir)
	reg := filepath.Join(dir, "internal", "cli", "planned", "decision.yaml")
	if err := os.Remove(reg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(reg, "held"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := write(dir, p); err == nil {
		t.Fatal("a write to a folder succeeded")
	}
	for f := range p.Files {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err == nil {
			t.Errorf("%s was made and kept by a run that failed", f)
		}
	}
}
