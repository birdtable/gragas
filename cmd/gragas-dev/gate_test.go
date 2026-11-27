package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/gate"
	"github.com/birdtable/gragas/internal/repo/gatetest"
)

// gateFiles are what the gate check reads from the repository, from the
// module root two folders up; a folder is copied whole.
func gateFiles() []string {
	return []string{"gate", "docs/reasons", "layers.yaml", "suppressions.txt", ".gitleaks.toml", "cmd/gragas-docs/main.go", "cmd/gragas/main.go", "internal/config/settings.go", "internal/host/fs/legacywrite.go", "internal/cli/root/output.go", "cmd/gragas-subset/terminal.go", "internal/host/fs/legacylock.go", "internal/host/jsonschema/embedded.go", "internal/cli/root/host.go", "internal/cli/root/registry_data.go", "internal/host/git/git.go", "internal/core/api/cache.go", "internal/core/api/book_load.go", "internal/core/rulebook/names.go", "internal/core/rulebook/parts.go", "internal/core/api/change.go", "internal/core/api/journal_recover.go", "internal/core/api/journal_restore.go", "internal/core/api/history.go", "internal/host/git/program.go", "internal/host/tool/trust.go", "internal/core/api/sarif.go", "cmd/gragas-release/verify.go", "cmd/gragas-new/main.go", "internal/apply/digest.go", "internal/canon/digest.go", "internal/core/format/digest.go", "internal/dev/surface/surface.go", "internal/port/testing/world_git.go", "internal/model/contract/codec.gen.go", "internal/core/api/exemptions.go", "internal/core/api/conformance.go", "cmd/gragas-subset/main.go", "cmd/gragas-new/check.go", "cmd/gragas-release/toolchain.go", "internal/host/sandbox/sandbox.go", "cmd/gragas-dev/gate_exemptions.go", "internal/cli/root/join_gen.go", "internal/core/contract/fieldref.go", "internal/dev/generate/generate.go", "cmd/gragas-dev/tokens.go", "internal/host/term/palette.go"}
}

// copyGate is the registry and the files it counts, in a folder of the test's
// own.
func copyGate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range gateFiles() {
		src := filepath.Join("..", "..", f)
		err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(filepath.Join("..", ".."), p)
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, rel), data, 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// The rulebook the check reads is the one in the format: the folder itself, or before the switch a converted copy.
	if err := os.CopyFS(filepath.Join(dir, "rulebook"), gatetest.Of(t, filepath.Join("..", "..")).Rulebook); err != nil {
		t.Fatal(err)
	}
	return dir
}

// fakeMachine replaces what the check reads from the go tool and from git:
// the closure is the list the registry itself holds, and there is no base.
func fakeMachine(t *testing.T, dir string) machine {
	t.Helper()
	return machine{
		modules: func(_ context.Context, _ string) ([]string, error) {
			data, err := os.ReadFile(filepath.Join(dir, "gate", "closure.txt"))
			if err != nil {
				return nil, err
			}
			return listLines(string(data)), nil
		},
		today: func() string { return fakeToday },
	}
}

// listLines is the non-blank, non-comment lines of a list.
func listLines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}

func doGate(m machine, dir string) (code int, out, errs string) {
	var o, e bytes.Buffer
	code = runWith(context.Background(), m, []string{"gate", "check", "-C", dir}, &o, &e)
	return code, o.String(), e.String()
}

func rewrite(t *testing.T, dir, file string, change func(string) string) {
	t.Helper()
	was := read(t, dir, file)
	now := change(was)
	if now == was {
		t.Fatalf("the plant changed nothing in %s", file)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(now), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTheRepositorysGateHolds: the registry as committed passes the whole
// check, with the real go tool reading the shipped binary's modules. It reads the
// rulebook folder itself, so it is green only on a tree in the rule-file format
// (after the switch, g-r7): before it, the exemptions check says the rule files
// are not read. Risk:
// task gate-registry passing on a check that reads nothing.
func TestTheRepositorysGateHolds(t *testing.T) {
	t.Parallel()
	m := realMachine(os.Environ())
	m.baseMarks = nil // the base's marks are marks_test.go's
	code, out, errs := doGate(m, filepath.Join("..", ".."))
	if code != exitDone || !strings.Contains(out, "holds") {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
}

// TestAMalformedRowInEachRegistryFileFailsTheGate: a planted malformed row in
// each file is named under the file, after a clean control passes
// (gates.plants.1).
func TestAMalformedRowInEachRegistryFileFailsTheGate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		file, from, to, says string
	}{
		{gate.Exemptions, "    exit: rule-source/amendments#34\n", "", "exit"},
		{gate.Exemptions, `expires: "2027-03-31"`, `expires: soon`, "expires"},
		{gate.Standins, "example.org  # the second domain, for a library or a mirror", "example.org  # short", "reason"},
		{gate.ClosureList, "go.yaml.in/yaml/v3", "NOT A MODULE", "value"},
		{gate.Crash, `"exit": "birdtable/gragas#1420"`, `"exit": "soon"`, "crash/7"},
		{gate.Crash, `"points": 6`, `"points": 6, "outside": "a command run outside the write path"`, "crash/0"},
		{gate.Subsets, `"decision": "explorer-iframe-build-entry",`, `"decision": "explorer-iframe-build-entry", "no_embeds": true,`, "subsets/1"},
		{gate.Subsets, `"decision": "embedded-data-folders",`, `"decision": "A999",`, "docs/reasons holds no file named A999.md"},
	}
	for _, tc := range tests {
		t.Run(tc.file+" "+tc.says, func(t *testing.T) {
			t.Parallel() // its own copy of the gate files
			dir := copyGate(t)
			m := fakeMachine(t, dir)
			if code, out, errs := doGate(m, dir); code != exitDone {
				t.Fatalf("the clean control exit %d\n%s%s", code, out, errs)
			}
			rewrite(t, dir, tc.file, func(s string) string { return strings.Replace(s, tc.from, tc.to, 1) })
			code, out, _ := doGate(m, dir)
			if code != exitFailed || !strings.Contains(out, tc.file+": ") || !strings.Contains(out, tc.says) {
				t.Errorf("exit %d, want %d naming %s and %q\n%s", code, exitFailed, tc.file, tc.says, out)
			}
		})
	}
}

// TestAMissingRegistryFileOrSchemaFailsTheGateClosed: each file, gone, fails
// the gate; so do the schemas, gone (gates.plants.5).
func TestAMissingRegistryFileOrSchemaFailsTheGateClosed(t *testing.T) {
	t.Parallel()
	for _, f := range []string{gate.Exemptions, gate.Standins, gate.ClosureList} {
		t.Run(f, func(t *testing.T) {
			dir := copyGate(t)
			m := fakeMachine(t, dir)
			closure := read(t, dir, gate.ClosureList)
			if err := os.Remove(filepath.Join(dir, f)); err != nil {
				t.Fatal(err)
			}
			m.modules = func(context.Context, string) ([]string, error) {
				return strings.Fields(strings.ReplaceAll(closure, "#", " ")), nil
			}
			code, out, _ := doGate(m, dir)
			if code != exitFailed || !strings.Contains(out, f+": is missing") {
				t.Errorf("exit %d\n%s", code, out)
			}
		})
	}
	t.Run("the schemas", func(t *testing.T) {
		dir := copyGate(t)
		m := fakeMachine(t, dir)
		if err := os.RemoveAll(filepath.Join(dir, "gate", "schema")); err != nil {
			t.Fatal(err)
		}
		code, _, errs := doGate(m, dir)
		if code != exitFailed || !strings.Contains(errs, "schemas") {
			t.Errorf("exit %d\n%s", code, errs)
		}
	})
}

// TestTheGateCommandRefusesWhatItDoesNotUnderstand: the landing flags and
// gate judge went with the landing check (gragas#912), and are refused as any
// other word is.
func TestTheGateCommandRefusesWhatItDoesNotUnderstand(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"gate"}, {"gate", "run"}, {"gate", "judge"}, {"gate", "check", "extra"}, {"gate", "check", "--apply"},
		{"gate", "check", "--land"}, {"gate", "check", "--staged"}} {
		if code, _, errs := do(args...); code != exitUsage || errs == "" {
			t.Errorf("%v: exit %d, stderr %q; want exit %d with a message", args, code, errs, exitUsage)
		}
	}
	t.Run("a base that cannot be read", func(t *testing.T) {
		dir := copyGate(t)
		m := fakeMachine(t, dir)
		m.baseMarks = realMachine(os.Environ()).baseMarks
		code, _, errs := doGate(m, dir)
		if code != exitFailed || errs == "" {
			t.Errorf("exit %d, stderr %q: a folder with no git must fail closed", code, errs)
		}
	})
}

// TestTheSubsetListIsHeldToTheBase: with a base that holds the committed
// subset list, the list as it is passes, and an element row that no longer
// forbids the standalone package, on the decision the base already holds,
// fails the gate (gragas#341). The base is the copy as it was before the
// plant, read as git would show it.
func TestTheSubsetListIsHeldToTheBase(t *testing.T) {
	t.Parallel()
	dir := copyGate(t)
	baseList := read(t, dir, gate.Subsets)
	m := fakeMachine(t, dir)
	m.baseFile = func(_ context.Context, _, _, name string) ([]byte, error) {
		if name == gate.Subsets {
			return []byte(baseList), nil
		}
		data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(name)))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return data, err
	}
	if code, out, errs := doGate(m, dir); code != exitDone {
		t.Fatalf("the list as committed, held to itself: exit %d\n%s%s", code, out, errs)
	}
	rewrite(t, dir, gate.Subsets, func(s string) string {
		return strings.Replace(s, `"forbidden": ["@gragas/standalone"]`, `"forbidden": ["@gragas/tokens"]`, 1)
	})
	code, out, _ := doGate(m, dir)
	if code != exitFailed || !strings.Contains(out, "the subset element no longer forbids @gragas/standalone, on the decision explorer-iframe-build-entry, which the base already holds") {
		t.Errorf("exit %d, want %d naming the widened row\n%s", code, exitFailed, out)
	}

	// The files list of an allowed package is held too (config-embeds-system-variables): the key dropped,
	// or a name added, on the decision the base already holds, is red.
	write(t, dir, gate.Subsets, baseList)
	rewrite(t, dir, gate.Subsets, func(s string) string {
		return strings.Replace(s, `,
          "files": ["vars_system.txt"]`, "", 1)
	})
	code, out, _ = doGate(m, dir)
	if code != exitFailed || !strings.Contains(out, "allows internal/config with no files list") {
		t.Errorf("the files key dropped: exit %d, want %d naming it\n%s", code, exitFailed, out)
	}
	write(t, dir, gate.Subsets, baseList)
	rewrite(t, dir, gate.Subsets, func(s string) string {
		return strings.Replace(s, `"files": ["vars_system.txt"]`, `"files": ["vars_system.txt", "second.txt"]`, 1)
	})
	code, out, _ = doGate(m, dir)
	if code != exitFailed || !strings.Contains(out, "lets internal/config embed second.txt") {
		t.Errorf("a file name added: exit %d, want %d naming it\n%s", code, exitFailed, out)
	}
}

// TestTheSubsetListIsHeldToARealGitBase runs gitBaseFile against git
// for real: a copy of the registry committed as a repository
// of the test's own, its remote main the commit. The list as committed
// passes; an element row that no longer forbids the standalone package, on
// explorer-iframe-build-entry, which the base holds, fails. So does Explorer-iframe-build-entry, its name in another case,
// which a file system that folds case would open as explorer-iframe-build-entry.md and git does not
// list. With no list at the base, the gate says so on a report line.
func TestTheSubsetListIsHeldToARealGitBase(t *testing.T) {
	t.Parallel()
	commit := func(t *testing.T, dir string) {
		t.Helper()
		git(t, dir, "init", "-q", "-b", "main")
		git(t, dir, "add", "-A")
		git(t, dir, "commit", "-q", "-m", "the base")
		git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	}
	real := func(t *testing.T, dir string) machine {
		t.Helper()
		m := fakeMachine(t, dir)
		m.baseFile = realMachine(os.Environ()).baseFile
		return m
	}
	widen := func(decision string) func(string) string {
		return func(s string) string {
			return strings.Replace(s, `"decision": "explorer-iframe-build-entry",
      "why": "the web component is built without the standalone package, so no here view and no shell travel with it",
      "forbidden": ["@gragas/standalone"]`, `"decision": "`+decision+`",
      "why": "the web component is built without the standalone package, so no here view and no shell travel with it",
      "forbidden": ["@gragas/tokens"]`, 1)
		}
	}
	cases := []struct {
		claim, decision, says string
	}{
		{"a widening on a record the base holds", "explorer-iframe-build-entry", "the subset element no longer forbids @gragas/standalone, on the decision explorer-iframe-build-entry, which the base already holds"},
		{"a widening on the record's name in another case", "Explorer-iframe-build-entry", "holds no file named Explorer-iframe-build-entry.md"},
	}
	for _, c := range cases {
		t.Run(c.claim, func(t *testing.T) {
			t.Parallel()
			dir := copyGate(t)
			commit(t, dir)
			m := real(t, dir)
			if code, out, errs := doGate(m, dir); code != exitDone || strings.Contains(out, "the base holds no list") {
				t.Fatalf("the list as committed, held to its own git base: exit %d\n%s%s", code, out, errs)
			}
			rewrite(t, dir, gate.Subsets, widen(c.decision))
			code, out, _ := doGate(m, dir)
			if code != exitFailed || !strings.Contains(out, c.says) {
				t.Errorf("exit %d, want %d saying %q\n%s", code, exitFailed, c.says, out)
			}
		})
	}
	t.Run("a base with no list is a report line", func(t *testing.T) {
		t.Parallel()
		dir := copyGate(t)
		list := read(t, dir, gate.Subsets)
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(gate.Subsets))); err != nil {
			t.Fatal(err)
		}
		commit(t, dir)
		write(t, dir, gate.Subsets, list)
		code, out, errs := doGate(real(t, dir), dir)
		if code != exitDone || !strings.Contains(out, reportMark+"subsets: the base holds no list, so none was held to it") {
			t.Errorf("exit %d, want %d and the report line\n%s%s", code, exitDone, out, errs)
		}
	})
}

// TestADecisionIsAnARecordThatIsAFile: a subset stands on a reason record, so
// a folder named like a record is none, and A99.md as a folder is held to no
// file. The control is the list as committed.
func TestADecisionIsAnARecordThatIsAFile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		claim, decision, says string
		plant                 func(t *testing.T, dir string)
	}{
		{"a folder named like a record", "A99", "docs/reasons holds no file named A99.md", func(t *testing.T, dir string) {
			t.Helper()
			if err := os.MkdirAll(filepath.Join(dir, "docs", "reasons", "A99.md"), 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, dir, "docs/reasons/A99.md/x.md", "# not a record\n")
		}},
	}
	for _, c := range cases {
		t.Run(c.claim, func(t *testing.T) {
			t.Parallel()
			dir := copyGate(t)
			m := fakeMachine(t, dir)
			if code, out, errs := doGate(m, dir); code != exitDone {
				t.Fatalf("the clean control exit %d\n%s%s", code, out, errs)
			}
			if c.plant != nil {
				c.plant(t, dir)
			}
			rewrite(t, dir, gate.Subsets, func(s string) string {
				return strings.Replace(s, `"decision": "explorer-iframe-build-entry",`, `"decision": "`+c.decision+`",`, 1)
			})
			code, out, _ := doGate(m, dir)
			if code != exitFailed || !strings.Contains(out, c.says) {
				t.Errorf("exit %d, want %d saying %q\n%s", code, exitFailed, c.says, out)
			}
		})
	}
}

// TestTheRepositorysBudgetsHoldTheThreeTiers: the --row regexp the Taskfile hands hatch (BUDGET_ROW) reads
// exactly one row, with a limit, for each tier the Taskfile judges, over the real gate/budgets.yaml.
func TestTheRepositorysBudgetsHoldTheThreeTiers(t *testing.T) {
	t.Parallel()
	task, err := os.ReadFile(filepath.Join("..", "..", "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var pattern string
	for l := range strings.SplitSeq(string(task), "\n") {
		if v, ok := strings.CutPrefix(l, "  BUDGET_ROW: '"); ok {
			pattern = strings.TrimSuffix(v, "'")
		}
	}
	if pattern == "" {
		t.Fatal("Taskfile.yml has no BUDGET_ROW")
	}
	row, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "gate", "budgets.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, m := range row.FindAllSubmatch(data, -1) {
		if string(m[row.SubexpIndex("limit")]) == "" {
			t.Errorf("row %s has no limit", m[row.SubexpIndex("id")])
		}
		seen[string(m[row.SubexpIndex("id")])]++
	}
	for _, id := range []string{"tier-pre-commit", "tier-pre-push", "tier-warm", "tier-full", "tier-release"} {
		if seen[id] != 1 {
			t.Errorf("gate/budgets.yaml has %d rows for %s, want 1", seen[id], id)
		}
	}
}
