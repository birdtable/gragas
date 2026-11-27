package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/go-proc/gitenv/gittest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/dev/newcommand"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/dev/templates/command"
)

// theRepository is the module this package sits in.
const theRepository = "../.."

// inCopy runs c in the copy and gives its output, with the fix of a cold
// module cache after it when a go stopped on one (configtest.GoSaid), and
// whether it succeeded.
func inCopy(dir string, c *exec.Cmd) (string, bool) {
	c.Dir = dir
	var out bytes.Buffer
	c.Stdout, c.Stderr = &out, &out
	err := c.Run()
	return configtest.GoSaid(out.String()), err == nil
}

// linters are the ones the repository's golangci config turns on, its default
// set (standard: errcheck, govet, ineffassign, staticcheck, unused) first and
// then its enable list: the lint of
// the copy runs these and only these. TestTheCopysLintersAreTheConfigsOwn holds
// the list to the config. A constant, so the call below has no variable
// argument (gosec G204).
const linters = "errcheck,govet,ineffassign,staticcheck,unused,bodyclose,contextcheck,depguard,dupl,errorlint,exhaustive,exptostd,forbidigo,gochecknoglobals,gochecknoinits,gocognit,gocritic,gocyclo,godoclint,gomoddirectives,gomodguard_v2,gosec,iface,interfacebloat,modernize,noctx,nolintlint,paralleltest,revive,sloglint,thelper,tparallel,usestdlibvars"

// standard is the default set golangci-lint names "standard".
const standard = "errcheck,govet,ineffassign,staticcheck,unused"

// configured are the linters the config under dir runs: the default set when
// its "  default:" is standard, then the list items between "  enable:" and the
// next line that is not one.
func configured(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	in := false
	if strings.Contains(string(b), "\n  default: standard") {
		names = strings.Split(standard, ",")
	}
	for line := range strings.Lines(string(b)) {
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "  enable:":
			in = true
		case in && strings.HasPrefix(line, "    - "):
			name, _, _ := strings.Cut(strings.TrimPrefix(line, "    - "), " ")
			// The generated list names the standard five too, each for its rule
			// (gragas#1744); a linter named twice is enabled once.
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		case in && strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#"):
			return strings.Join(names, ",")
		}
	}
	return strings.Join(names, ",")
}

// lintOfTheCopy runs the linters over the three packages the generator writes
// into and gives what they said and whether they passed.
func lintOfTheCopy(ctx context.Context, dir, verb string) (string, bool) {
	cmd := exec.CommandContext(ctx, ".tools/bin/golangci-lint", "run", "--allow-parallel-runners", "--enable-only")
	cmd.Args = append(cmd.Args, linters, "./internal/cli/cmd/"+verb, "./internal/cli/root", "./internal/core/api", "./internal/journeys")
	return inCopy(dir, cmd)
}

// withABreak is the templates with tail added to the command file's template.
func withABreak(t *testing.T, tail string) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	err := fs.WalkDir(command.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(command.Files, p)
		if err != nil {
			return err
		}
		if path.Base(p) == "command.go.tmpl" {
			b = append(b, tail...)
		}
		out[p] = &fstest.MapFile{Data: b}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestTheGeneratedCopyBuildsVetsLintsAndFailsItsStubs: the generator runs into
// a copy of the tree, built once; the copy vets, its generated files pass the
// linters the config enables, the registry rules hold in it and its three stubs
// fail naming the command (arch.templates.2, tests.runners.11). Planted: a
// template that gives a global variable is refused by the lint of the copy.
func TestTheGeneratedCopyBuildsVetsLintsAndFailsItsStubs(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dst := t.TempDir()
	if err := check(t.Context(), theRepository, dst, &say.Speaker{W: discard{}}); err != nil {
		t.Fatal(err)
	}
	taken, err := declared(os.DirFS(theRepository))
	if err != nil {
		t.Fatal(err)
	}
	name, ok := freeName(taken)
	if !ok {
		t.Fatal("no free verb of the base list")
	}
	clean, err := newcommand.Make(command.Files, os.DirFS(theRepository), name, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out, ok := inCopy(dst, exec.CommandContext(ctx, "go", "run", "./cmd/gragas-dev", "gen", "commands")); !ok {
		t.Fatalf("the join does not run in the copy:\n%s", out)
	}
	if out, ok := inCopy(dst, exec.CommandContext(ctx, "go", "build", "./...")); !ok {
		t.Fatalf("the copy does not build:\n%s", out)
	}

	t.Run("the copy", func(t *testing.T) {
		t.Run("vets", func(t *testing.T) {
			t.Parallel()
			vet := exec.CommandContext(ctx, "go", "vet")
			vet.Args = append(vet.Args, "./internal/cli/cmd/"+name.Verb, "./internal/cli/root", "./internal/core/api", "./internal/journeys")
			if out, ok := inCopy(dst, vet); !ok {
				t.Errorf("the copy does not vet:\n%s", out)
			}
		})
		t.Run("lints", func(t *testing.T) {
			t.Parallel()
			if out, ok := lintOfTheCopy(ctx, dst, name.Verb); !ok {
				t.Errorf("the generated files do not lint:\n%s", out)
			}
		})
		t.Run("holds the registry rules", func(t *testing.T) {
			t.Parallel()
			run := "TestEveryWordOfTheRegistryIsOnTheBaseListOrAGroup|TestTheTreeIsTheRegistrysBuiltAndLegacyRowsAndNothingElse|TestTheRegistryRefusesGetMakeDestroyAndAWordOffTheBaseList"
			if out, ok := inCopy(dst, exec.CommandContext(ctx, "go", "test", "./internal/cli/root", "-run", run)); !ok {
				t.Errorf("a registry rule fails in the copy:\n%s", out)
			}
		})
		t.Run("fails its three stubs naming the command", func(t *testing.T) {
			t.Parallel()
			stubs := exec.CommandContext(ctx, "go", "test")
			stubs.Args = append(stubs.Args, "./internal/cli/cmd/"+name.Verb, "./internal/core/api", "./internal/journeys", "-run", "^Test[A-Z][a-z]+(Transcript|Journey)?$")
			out, ok := inCopy(dst, stubs)
			if ok {
				t.Fatalf("the stubs pass:\n%s", out)
			}
			if n := strings.Count(out, name.Verb+": write "); n != 3 {
				t.Errorf("%d stubs fail naming %q, want 3:\n%s", n, name.Verb, out)
			}
		})
	})

	// Each planted template runs after the checks above, which share the copy,
	// and is put back before the next: a global variable that gochecknoglobals
	// refuses, and an error left unchecked that errcheck, one of the default
	// set, refuses.
	for _, plant := range []struct{ linter, tail string }{
		{"gochecknoglobals", "\nvar plantedBreak = 1\n"},
		{"errcheck", "\nfunc plantedUnchecked() { f := func() error { return nil }; f() }\n"},
	} {
		planted, err := newcommand.Make(withABreak(t, plant.tail), os.DirFS(theRepository), name, nil)
		if err != nil {
			t.Fatal(err)
		}
		var broken string
		for p, text := range planted.Files {
			if text != clean.Files[p] {
				broken = p
			}
		}
		if broken == "" {
			t.Fatalf("the template planted for %s changed no generated file", plant.linter)
		}
		full := filepath.Join(dst, filepath.FromSlash(broken))
		if err := os.WriteFile(full, []byte(planted.Files[broken]), 0o644); err != nil {
			t.Fatal(err)
		}
		out, passed := lintOfTheCopy(ctx, dst, name.Verb)
		if passed || !strings.Contains(out, plant.linter) || !strings.Contains(out, path.Base(broken)) {
			t.Errorf("the lint of the copy did not refuse the planted %s (passed %v):\n%s", plant.linter, passed, out)
		}
		if err := os.WriteFile(full, []byte(clean.Files[broken]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestTheCopysLintersAreTheConfigsOwn: the list the lint of the copy runs is
// the list the config enables. Planted: a list with one linter short is not it.
func TestTheCopysLintersAreTheConfigsOwn(t *testing.T) {
	t.Parallel()
	want := configured(t, theRepository)
	if linters != want {
		t.Errorf("the copy lints with\n  %s\nthe config enables\n  %s", linters, want)
	}
	if short := linters[:strings.LastIndex(linters, ",")]; short == want {
		t.Error("a list one linter short is judged the config's own")
	}
}

// generated are the files make gen writes from a new command's folder, the
// only ones of the tree a new command may change.
const joined = "internal/cli/root/join_gen.go"

// treeDiff is how the copy differs from the repository: the files it adds, the
// files it changes and the files it removes, by path from the root. Both
// sides leave out what leftOut names.
func treeDiff(t *testing.T, src, dst string) (added, changed, removed []string) {
	t.Helper()
	read := func(root string) map[string]string {
		out := map[string]string{}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil || rel == "." {
				return err
			}
			if leftOut(filepath.ToSlash(rel)) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			b, err := os.ReadFile(p)
			out[filepath.ToSlash(rel)] = string(b)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	before, after := read(src), read(dst)
	for p, text := range after {
		old, ok := before[p]
		switch {
		case !ok:
			added = append(added, p)
		case old != text:
			changed = append(changed, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			removed = append(removed, p)
		}
	}
	slices.Sort(added)
	slices.Sort(changed)
	slices.Sort(removed)
	return added, changed, removed
}

// strays are the paths of a diff that are not the command's own or generated:
// a hand-written file outside the new package, which a new command never
// touches. Added files are held to the plan's own rule; a changed or removed
// one is a stray unless gen wrote it.
func strays(n newcommand.Name, added, changed, removed []string) []string {
	files := map[string]string{}
	for _, p := range added {
		files[p] = ""
	}
	var out []string
	if err := newcommand.Outside(n, newcommand.Plan{Files: files}); err != nil {
		out = append(out, err.Error())
	}
	for _, p := range slices.Concat(changed, removed) {
		if p != joined {
			out = append(out, p)
		}
	}
	return out
}

// TestANewCommandChangesNoHandWrittenFileOutsideItsPackage: the generator runs
// into a copy and make gen's join runs over it; the diff of the copy against
// the repository holds the new package, the core's files of the same name and
// the generated join, and nothing else (#444, done when 1). Planted: a hand
// edit of a file of the root, and a file added beside it, are each told.
func TestANewCommandChangesNoHandWrittenFileOutsideItsPackage(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dst := t.TempDir()
	snap := snapshotOf(t, theRepository)
	if err := check(t.Context(), snap, dst, &say.Speaker{W: discard{}}); err != nil {
		t.Fatal(err)
	}
	taken, err := declared(os.DirFS(theRepository))
	if err != nil {
		t.Fatal(err)
	}
	name, ok := freeName(taken)
	if !ok {
		t.Fatal("no free verb of the base list")
	}
	if out, ok := inCopy(dst, exec.CommandContext(ctx, "go", "run", "./cmd/gragas-dev", "gen", "commands")); !ok {
		t.Fatalf("the join does not run in the copy:\n%s", out)
	}
	added, changed, removed := treeDiff(t, snap, dst)
	if got := strays(name, added, changed, removed); len(got) > 0 {
		t.Errorf("the new command touches files that are not its own: %v", got)
	}
	if !slices.Equal(changed, []string{joined}) {
		t.Errorf("the join changed %v, want [%s]", changed, joined)
	}
	if !slices.Contains(added, "internal/cli/cmd/"+name.Verb+"/command.go") {
		t.Errorf("the package of %s is not in the diff: %v", name.Verb, added)
	}

	hand := filepath.Join(dst, "internal", "cli", "root", "registry_data.go")
	b, err := os.ReadFile(hand)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hand, append(b, "\n// a hand edit\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(dst, "internal", "cli", "root", "beside.go")
	if err := os.WriteFile(stray, []byte("package root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, changed, removed = treeDiff(t, snap, dst)
	got := strings.Join(strays(name, added, changed, removed), " ")
	if !strings.Contains(got, "internal/cli/root/registry_data.go") || !strings.Contains(got, "internal/cli/root/beside.go") {
		t.Errorf("a hand edit and a file beside it were not told: %q", got)
	}
}

// gitIn runs c, a git command, in dir with no config of the account or the
// system, and fails the test with its output if it does not succeed.
func gitIn(t *testing.T, dir string, c *exec.Cmd) {
	t.Helper()
	c.Env = gittest.Git(dir)
	if out, ok := inCopy(dir, c); !ok {
		t.Fatalf("%s in %s:\n%s", strings.Join(c.Args, " "), dir, out)
	}
}

// snapshotOf copies the files git tracks in repo into a temp folder and makes
// that folder a git work tree of its own, so what the live tree gains while a
// test runs is not in it: another package's dist, a .plant-* folder, pnpm's
// run state (gragas#1324).
func snapshotOf(t *testing.T, repo string) string {
	t.Helper()
	snap := t.TempDir()
	if err := copyTree(t.Context(), repo, snap); err != nil {
		t.Fatal(err)
	}
	gitIn(t, snap, exec.CommandContext(t.Context(), "git", "init", "--quiet"))
	// The pinned tools' link is left out of the index, as the copy leaves it
	// out: git reads a junction as a folder and would walk the whole of
	// .tools, past git's longest path (gragas#1589).
	gitIn(t, snap, exec.CommandContext(t.Context(), "git", "add", "--force", "--", ".", ":(exclude).tools"))
	return snap
}

// TestTheCopyIsOfTheTrackedFilesAndNothingThatComesLater: planted, a file
// written into the tree before the copy outside the index (a node_modules
// folder, a dist, a .plant-* folder) is not copied; a file written after it is
// not in the snapshot or its diff, so no writer beside the test turns it red;
// a tracked file removed after it is still told as removed (gragas#1324).
func TestTheCopyIsOfTheTrackedFilesAndNothingThatComesLater(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	put := func(rel, body string) {
		t.Helper()
		p := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("a.go", "package a\n")
	put("pkg/b.go", "package b\n")
	gitIn(t, repo, exec.CommandContext(t.Context(), "git", "init", "--quiet"))
	gitIn(t, repo, exec.CommandContext(t.Context(), "git", "add", "."))
	put("explorer/node_modules/.pnpm-task-run-state-v1/run.jsonl", "{}\n")
	put("dist/out.js", "x\n")
	put(".plant-1/f", "x\n")

	snap, dst := snapshotOf(t, repo), t.TempDir()
	if err := copyTree(t.Context(), snap, dst); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"explorer", "dist", ".plant-1"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s was copied: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "pkg", "b.go")); err != nil {
		t.Fatalf("pkg/b.go was not copied, so the copy proves nothing: %v", err)
	}

	put("dist/later.js", "x\n")
	put("explorer/node_modules/.pnpm-task-run-state-v1/run.finished", "")
	if err := os.Remove(filepath.Join(repo, "a.go")); err != nil {
		t.Fatal(err)
	}
	if added, changed, removed := treeDiff(t, snap, dst); len(added)+len(changed)+len(removed) != 0 {
		t.Errorf("what came to the live tree after the snapshot is in the diff: added %v, changed %v, removed %v", added, changed, removed)
	}
	if err := os.Remove(filepath.Join(dst, "pkg", "b.go")); err != nil {
		t.Fatal(err)
	}
	if _, _, removed := treeDiff(t, snap, dst); !slices.Equal(removed, []string{"pkg/b.go"}) {
		t.Errorf("a file of the snapshot that is not in the copy is told as %v, want [pkg/b.go]", removed)
	}
}
