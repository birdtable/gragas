package internal

import (
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A new tool is data only. A change
// that adds a file under the adapters folder, a new tool's folder or a new
// version of an adapter, changes adapter data and the rulebook and no Go. This
// file holds that line (gragas#1993). A test cannot read a purpose, so it reads
// the change: what the tree differs from the references by.
//
// There is no exception list, no flag and no comment that lets one through. A
// tool that cannot be added without Go is a finding against the engine or the
// dev tools, filed as a ticket of its own, and that Go is generic.

// adaptersFolder is the folder of the adapters, where the tree states it: the
// embed line of the one package that embeds a folder named adapters names the
// folder beside that package's own file. The path is never written a second
// time here.
func adaptersFolder(t *testing.T) string {
	t.Helper()
	s := snap()
	if s.err != nil {
		t.Fatal(s.err)
	}
	var found []string
	for _, src := range sources(t, s.world) {
		if src.test {
			continue
		}
		for _, g := range src.file.Comments {
			for _, c := range g.List {
				if embedsFolder(c, "adapters") {
					found = append(found, path.Join(path.Dir(src.path), "adapters"))
				}
			}
		}
	}
	slices.Sort(found)
	if len(found) == 0 {
		// The folder left gragas with the four tools (gragas#2031): no package
		// embeds it, so the planted cases below hold the rule on this path alone.
		return "internal/host/assets/adapters"
	}
	if len(slices.Compact(found)) != 1 {
		t.Fatalf("the adapters folder is stated by the go:embed line of one package, and %d were found: %q", len(found), found)
	}
	return found[0]
}

// embedsFolder is whether the comment is a go:embed line that names the folder
// (with or without the all: prefix).
func embedsFolder(c *ast.Comment, folder string) bool {
	rest, ok := strings.CutPrefix(c.Text, "//go:embed ")
	if !ok {
		return false
	}
	for pattern := range strings.FieldsSeq(rest) {
		if strings.TrimPrefix(pattern, "all:") == folder {
			return true
		}
	}
	return false
}

// toolChange is one path the change touches, as git diff --name-status says it.
type toolChange struct{ status, path string }

// toolIsData is what the change breaks of the data-only rule: for each file added under
// adapters, the Go files the same change touches, test files included, added,
// changed or deleted. A change that adds no adapter file is not held to
// anything, and an adapter file the base already holds, edited, is no trigger.
func toolIsData(changes []toolChange, adapters string) []string {
	var added, goFiles []string
	for _, c := range changes {
		if c.status == "A" && strings.HasPrefix(c.path, adapters+"/") {
			added = append(added, c.path)
		}
		if strings.HasSuffix(c.path, ".go") {
			goFiles = append(goFiles, c.path)
		}
	}
	if len(added) == 0 || len(goFiles) == 0 {
		return nil
	}
	slices.Sort(added)
	slices.Sort(goFiles)
	var out []string
	for _, a := range added {
		out = append(out, a+" is added, and the same change touches Go: "+strings.Join(goFiles, ", ")+
			"; a new tool or a new adapter version is data only (gragas#1993), so the Go change is generic and goes in its own pull request, proves itself with a fixture and lands first")
	}
	return out
}

// toolRefs are the commits the change is judged against, or why there are
// none: the merge base of each head, HEAD and, in a merge, MERGE_HEAD, with
// origin/main and with origin/release-1 where the clone has it, a reference
// that is an ancestor of another dropped (a piece is read from its nearest base, and
// only two tips that do not contain each other are both read). A path is part
// of the change only where it differs from every one of them (toolChanged), so
// for a piece on release-1 it is the piece, for a piece on main it is the
// piece, and for a merge of the two long-lived branches, whose references are
// the two tips, it is what the hand merge wrote, never what either branch
// already held. No reference is picked as the newest. Nothing is read from a
// tree that is no checkout, a shallow clone or a clone with no origin/main.
func toolRefs(t *testing.T, dir string) ([]string, string) {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, "the merge base cannot be read: " + err.Error()
	}
	ceiling := []string{"GIT_CEILING_DIRECTORIES=" + filepath.Dir(abs)}
	shallow, err := gitTry(t, abs, ceiling, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, "the tree is not a git checkout, so the change has no base to be held from: " + err.Error()
	}
	if shallow != "false" {
		return nil, "the clone is shallow, so its merge base is not the one main has: fetch the full history"
	}
	heads := []string{"HEAD"}
	if _, err := gitTry(t, abs, ceiling, "rev-parse", "-q", "--verify", "MERGE_HEAD"); err == nil {
		heads = append(heads, "MERGE_HEAD")
	}
	var refs []string
	for _, ref := range []string{"refs/remotes/origin/main", "refs/remotes/origin/release-1"} {
		if _, err := gitTry(t, abs, ceiling, "rev-parse", "-q", "--verify", ref); err != nil {
			if ref == "refs/remotes/origin/main" {
				return nil, "there is no origin/main, so the change has no base to be held from: " + err.Error()
			}
			continue
		}
		for _, head := range heads {
			base, err := gitTry(t, abs, ceiling, "merge-base", head, ref)
			if err != nil {
				if ref == "refs/remotes/origin/main" {
					return nil, head + " has no merge base with origin/main (no history in common), so the change has no base to be held from: " + err.Error()
				}
				continue
			}
			if !slices.Contains(refs, base) {
				refs = append(refs, base)
			}
		}
	}
	// A reference that is an ancestor of another says nothing the other does not: a path equal to the
	// fork's copy would be dropped from the change though the piece's own branch holds another (gragas#1996).
	// Only references that do not contain each other stay, so a piece is read from its nearest base and a
	// merge of the two tips from both.
	if len(refs) > 1 {
		out, err := gitTry(t, abs, ceiling, append([]string{"merge-base", "--independent"}, refs...)...)
		if err != nil {
			return nil, "the references of the change cannot be reduced to those that do not contain each other: " + err.Error()
		}
		refs = strings.Fields(out)
	}
	return refs, ""
}

// toolDiff is what differs between ref and the work tree of dir: the commits
// past it, what is staged and what is changed in a tracked file, so one read
// serves a builder's branch and the lander's squash, staged on a detached HEAD
// at the base with nothing committed. A file git does not track is no part of
// a change, and a rename is a deletion and an addition.
func toolDiff(t *testing.T, dir, ref string) map[string]string {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	out, err := gitTry(t, abs, []string{"GIT_CEILING_DIRECTORIES=" + filepath.Dir(abs)}, "diff", "--name-status", "--no-renames", "-z", ref)
	if err != nil {
		t.Fatal(err)
	}
	var tokens []string
	for tok := range strings.SplitSeq(out, "\x00") {
		if tok != "" {
			tokens = append(tokens, tok)
		}
	}
	if len(tokens)%2 != 0 {
		t.Fatalf("git diff --name-status -z wrote %d fields, want pairs: %q", len(tokens), tokens)
	}
	diff := map[string]string{}
	for i := 0; i < len(tokens); i += 2 {
		diff[tokens[i+1]] = tokens[i]
	}
	return diff
}

// toolChanged is the paths that differ from every one of refs, each with its
// status: A only where no reference holds the path.
func toolChanged(t *testing.T, dir string, refs []string) []toolChange {
	t.Helper()
	var changed map[string]string
	for _, ref := range refs {
		diff := toolDiff(t, dir, ref)
		if changed == nil {
			changed = diff
			continue
		}
		for p, s := range changed {
			d, ok := diff[p]
			switch {
			case !ok:
				delete(changed, p)
			case d != "A" && s == "A":
				changed[p] = d
			}
		}
	}
	var out []toolChange
	for p, s := range changed {
		out = append(out, toolChange{status: s, path: p})
	}
	slices.SortFunc(out, func(a, b toolChange) int { return strings.Compare(a.path, b.path) })
	return out
}

// toolIsDataIn is the whole check of the tree at dir: the problems of its
// change, or the one problem that it could not run, with its reason. The real
// test and the planted ones call this alone, so what the real one says with no
// base is what the planted case reads.
func toolIsDataIn(t *testing.T, dir, adapters string) []string {
	t.Helper()
	refs, why := toolRefs(t, dir)
	if why != "" {
		return []string{"could not run: " + why}
	}
	return toolIsData(toolChanged(t, dir, refs), adapters)
}

// TestAChangeThatAddsAnAdapterFileTouchesNoGo holds this tree's change to
// (gragas#1993): where it adds a file under the adapters folder it
// touches no Go file. With no base to judge it by the test could not run, and
// is red and says so, never green for want of one. It is a member of the
// push's os-code, run with -count=1, so a cached answer never outlives a file
// written after it.
//
// What it costs when this fails: a tool arrives with Go that names it, and the
// next tool needs Go again, so the engine grows a case for each (A6).
func TestAChangeThatAddsAnAdapterFileTouchesNoGo(t *testing.T) {
	t.Parallel()
	for _, p := range toolIsDataIn(t, root, adaptersFolder(t)) {
		t.Error(p)
	}
}

// TestAChangeIsHeldToGD139OnPlantedChanges: toolIsData on each planted change,
// by name, each with the words it must say.
func TestAChangeIsHeldToGD139OnPlantedChanges(t *testing.T) {
	t.Parallel()
	const dir = "internal/host/assets/adapters"
	for _, tc := range []struct {
		name    string
		changes []toolChange
		want    []string
	}{
		{"an adapter version and a Go file", []toolChange{{"A", dir + "/eslint/1.2.0.yaml"}, {"M", "cmd/gragas-new/main.go"}}, []string{dir + "/eslint/1.2.0.yaml is added", "cmd/gragas-new/main.go", "gragas#1993"}},
		{"an adapter version and a test file", []toolChange{{"A", dir + "/eslint/1.2.0.yaml"}, {"A", "internal/x_test.go"}}, []string{dir + "/eslint/1.2.0.yaml is added", "internal/x_test.go"}},
		{"an adapter version and a Go file deleted", []toolChange{{"A", dir + "/eslint/1.2.0.yaml"}, {"D", "internal/old.go"}}, []string{"internal/old.go"}},
		{"a version file renamed to another, with Go changed", []toolChange{{"D", dir + "/eslint/1.1.0.yaml"}, {"A", dir + "/eslint/1.2.0.yaml"}, {"M", "internal/core/link/generate.go"}}, []string{dir + "/eslint/1.2.0.yaml is added", "internal/core/link/generate.go"}},
		{"two adapter files and two Go files", []toolChange{{"A", dir + "/a/1.0.0.yaml"}, {"A", dir + "/b/1.0.0.yaml"}, {"M", "x/b.go"}, {"M", "x/a.go"}}, []string{dir + "/a/1.0.0.yaml is added, and the same change touches Go: x/a.go, x/b.go", dir + "/b/1.0.0.yaml is added"}},
		{"a new tool: folder, rulebook and preset, no Go", []toolChange{{"A", dir + "/newtool/1.0.0.yaml"}, {"A", "rulebook/program/x/rules/y.yaml"}, {"M", "presets/gragas/settings.yml"}}, nil},
		{"Go changed, no adapter file added", []toolChange{{"M", "internal/core/link/generate.go"}, {"A", "internal/core/link/x_test.go"}}, nil},
		{"an adapter file the base holds, edited, with Go changed", []toolChange{{"M", dir + "/eslint/1.1.0.yaml"}, {"M", "internal/core/link/generate.go"}}, nil},
		{"an adapter file deleted with Go changed", []toolChange{{"D", dir + "/eslint/1.0.0.yaml"}, {"M", "internal/core/link/generate.go"}}, nil},
		{"a file named like the folder, not under it", []toolChange{{"A", dir + "-notes/x.yaml"}, {"M", "x.go"}}, nil},
		{"no change", nil, nil},
	} {
		got := toolIsData(tc.changes, dir)
		if len(tc.want) == 0 {
			if len(got) != 0 {
				t.Errorf("%s: %q, want none", tc.name, got)
			}
			continue
		}
		joined := strings.Join(got, "\n")
		for _, w := range tc.want {
			if !strings.Contains(joined, w) {
				t.Errorf("%s: %q, want it to say %q", tc.name, joined, w)
			}
		}
	}
}

// putFile writes one file below dir, its folders made.
func putFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commitAll stages everything of dir and commits it as msg.
func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	wideGit(t, dir, "add", ".")
	wideGit(t, dir, "commit", "-q", "-m", msg)
}

// plantRepo is a repository with the adapters folder and one Go file at its
// origin/main, and, with release, release-1 one commit past it that holds an
// adapter version and a Go file of its own (a piece already landed there).
func plantRepo(t *testing.T, adapters string, release bool) string {
	t.Helper()
	dir := writeModule(t, map[string]string{
		adapters + "/eslint/1.1.0.yaml": "tool: eslint\n",
		"x/x.go":                        "package x\n",
	})
	wideGit(t, dir, "init", "-q", "-b", "main")
	commitAll(t, dir, "main")
	wideGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	if release {
		wideGit(t, dir, "checkout", "-q", "-b", "release-1")
		putFile(t, dir, adapters+"/eslint/1.0.0.yaml", "tool: eslint\n")
		putFile(t, dir, "x/earlier.go", "package x\n")
		commitAll(t, dir, "an earlier piece of release-1")
		wideGit(t, dir, "update-ref", "refs/remotes/origin/release-1", "HEAD")
	}
	return dir
}

// plantDiverged is two long-lived branches that each hold what the other
// lacks: main one commit past the fork with an adapter folder and a Go file of
// its own, release-1 one commit past it with an adapter version and a Go file
// of its own, and each tip as the origin's. It leaves release-1 checked out.
func plantDiverged(t *testing.T, adapters string) string {
	t.Helper()
	dir := plantRepo(t, adapters, false)
	wideGit(t, dir, "checkout", "-q", "-b", "release-1")
	putFile(t, dir, adapters+"/eslint/1.0.0.yaml", "tool: eslint\n")
	putFile(t, dir, "x/earlier.go", "package x\n")
	commitAll(t, dir, "an earlier piece of release-1")
	wideGit(t, dir, "update-ref", "refs/remotes/origin/release-1", "HEAD")
	wideGit(t, dir, "checkout", "-q", "main")
	putFile(t, dir, adapters+"/newtool/1.0.0.yaml", "tool: newtool\n")
	putFile(t, dir, "y/y.go", "package y\n")
	commitAll(t, dir, "a landed piece of main")
	wideGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	wideGit(t, dir, "checkout", "-q", "release-1")
	return dir
}

// TestAChangeIsHeldToGD139OnPlantedRepositories: the whole of it on planted
// repositories, each with its words: a builder's branch, the lander's squash
// staged on a detached HEAD at the base with nothing committed, a release-1
// piece, a rename, and a tree with no base.
func TestAChangeIsHeldToGD139OnPlantedRepositories(t *testing.T) {
	t.Parallel()
	adapters := adaptersFolder(t)
	version := adapters + "/eslint/1.2.0.yaml"
	for _, tc := range []struct {
		name    string
		release bool
		change  func(t *testing.T, dir string)
		want    []string // what the problems must say; none means green
	}{
		{"1: a version and a Go file, committed on a branch", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "-b", "feature")
			putFile(t, dir, version, "tool: eslint\n")
			putFile(t, dir, "x/x.go", "package x\n\nvar a = 1\n")
			commitAll(t, dir, "a tool with Go")
		}, []string{version + " is added", "x/x.go"}},
		{"2: a version and a test file", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "-b", "feature")
			putFile(t, dir, version, "tool: eslint\n")
			putFile(t, dir, "x/x_test.go", "package x\n")
			commitAll(t, dir, "a tool with a test")
		}, []string{version + " is added", "x/x_test.go"}},
		{"3: a new tool folder with rulebook and preset files, no Go", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "-b", "feature")
			putFile(t, dir, adapters+"/newtool/1.0.0.yaml", "tool: newtool\n")
			putFile(t, dir, "rulebook/program/x/rules/y.yaml", "id: x\n")
			putFile(t, dir, "presets/gragas/settings.yml", "a: b\n")
			commitAll(t, dir, "a tool as data")
		}, nil},
		{"4: Go changed, no adapter file added", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "-b", "feature")
			putFile(t, dir, "x/x.go", "package x\n\nvar a = 1\n")
			commitAll(t, dir, "ordinary work")
		}, nil},
		{"5: an adapter file the base holds, edited, with Go changed", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "-b", "feature")
			putFile(t, dir, adapters+"/eslint/1.1.0.yaml", "tool: eslint\nextends: true\n")
			putFile(t, dir, "x/x.go", "package x\n\nvar a = 1\n")
			commitAll(t, dir, "a generic change and its adapter edited")
		}, nil},
		{"6: the case of 1, staged on a detached HEAD at the base, nothing committed", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "--detach")
			putFile(t, dir, version, "tool: eslint\n")
			putFile(t, dir, "x/x.go", "package x\n\nvar a = 1\n")
			wideGit(t, dir, "add", ".")
		}, []string{version + " is added", "x/x.go"}},
		{"a version staged alone on a detached HEAD at the base: green", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "--detach")
			putFile(t, dir, version, "tool: eslint\n")
			wideGit(t, dir, "add", ".")
		}, nil},
		{"a file git does not track is no part of the change", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "--detach")
			putFile(t, dir, version, "tool: eslint\n")
			putFile(t, dir, "x/untracked.go", "package x\n")
			wideGit(t, dir, "add", version)
		}, nil},
		{"a version file renamed to another, with Go changed, committed: red", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "-b", "feature")
			wideGit(t, dir, "mv", adapters+"/eslint/1.1.0.yaml", version)
			putFile(t, dir, "x/x.go", "package x\n\nvar a = 1\n")
			commitAll(t, dir, "a renamed version with Go")
		}, []string{version + " is added", "x/x.go"}},
		{"a release-1 piece: release-1's own adapter and Go are not the change", true, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "-b", "piece")
			putFile(t, dir, "x/x.go", "package x\n\nvar a = 1\n")
			commitAll(t, dir, "ordinary work on release-1")
		}, nil},
		{"a release-1 piece that adds a version with Go: red, and only its own Go is named", true, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "-b", "piece")
			putFile(t, dir, version, "tool: eslint\n")
			putFile(t, dir, "x/x.go", "package x\n\nvar a = 1\n")
			commitAll(t, dir, "a tool with Go")
		}, []string{version + " is added, and the same change touches Go: x/x.go;"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := plantRepo(t, adapters, tc.release)
			tc.change(t, dir)
			assertProblems(t, toolIsDataIn(t, dir, adapters), tc.want, "x/earlier.go")
		})
	}
	t.Run("7: no origin/main is no base, and the check says so as the real test does", func(t *testing.T) {
		dir := plantRepo(t, adapters, false)
		wideGit(t, dir, "update-ref", "-d", "refs/remotes/origin/main")
		problems := toolIsDataIn(t, dir, adapters)
		if len(problems) != 1 || !strings.HasPrefix(problems[0], "could not run: ") || !strings.Contains(problems[0], "no origin/main") {
			t.Errorf("problems = %q, want the one that says could not run and why", problems)
		}
	})
	t.Run("a folder that is no checkout is no base", func(t *testing.T) {
		problems := toolIsDataIn(t, t.TempDir(), adapters)
		if len(problems) != 1 || !strings.HasPrefix(problems[0], "could not run: ") {
			t.Errorf("problems = %q, want the one that says could not run", problems)
		}
	})
}

// assertProblems holds the problems to want: none for green, else each phrase
// said, and never the phrase notWanted (a file of what a branch already held).
func assertProblems(t *testing.T, problems, want []string, notWanted string) {
	t.Helper()
	joined := strings.Join(problems, "\n")
	if len(want) == 0 {
		if len(problems) != 0 {
			t.Errorf("got %q, want green", joined)
		}
		return
	}
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("got %q, want it to say %q", joined, w)
		}
	}
	if notWanted != "" && strings.Contains(joined, notWanted) {
		t.Errorf("got %q: it names %s, which the branch already held", joined, notWanted)
	}
}

// TestAMergeIsHeldToWhatTheMergeItselfChanges: where main and release-1 each
// hold an adapter file and Go the other lacks, merging one into the other is
// no change of either's, so it is green, committed and staged, whichever way;
// and a merge that itself adds an adapter file and edits Go by hand is red,
// committed and staged, naming the merge's own and none of the two branches'.
func TestAMergeIsHeldToWhatTheMergeItselfChanges(t *testing.T) {
	t.Parallel()
	adapters := adaptersFolder(t)
	merged := adapters + "/newtool2/1.0.0.yaml"
	for _, tc := range []struct {
		name     string
		into     string // the branch checked out
		other    string
		commit   bool
		byHand   bool
		wantPlan []string
	}{
		{"main into release-1, staged", "release-1", "main", false, false, nil},
		{"main into release-1, committed", "release-1", "main", true, false, nil},
		{"release-1 into main, staged", "main", "release-1", false, false, nil},
		{"release-1 into main, committed", "main", "release-1", true, false, nil},
		{"main into release-1 with an adapter and Go written by the merge, staged", "release-1", "main", false, true, []string{merged + " is added", "x/x.go"}},
		{"main into release-1 with an adapter and Go written by the merge, committed", "release-1", "main", true, true, []string{merged + " is added", "x/x.go"}},
		{"release-1 into main with an adapter and Go written by the merge, staged", "main", "release-1", false, true, []string{merged + " is added", "x/x.go"}},
		{"release-1 into main with an adapter and Go written by the merge, committed", "main", "release-1", true, true, []string{merged + " is added", "x/x.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := plantDiverged(t, adapters)
			wideGit(t, dir, "checkout", "-q", tc.into)
			wideGit(t, dir, "merge", "-q", "--no-commit", "--no-ff", tc.other)
			if tc.byHand {
				putFile(t, dir, merged, "tool: newtool2\n")
				putFile(t, dir, "x/x.go", "package x\n\nvar a = 1\n")
			}
			wideGit(t, dir, "add", ".")
			if tc.commit {
				wideGit(t, dir, "commit", "-q", "-m", "the merge")
			}
			problems := toolIsDataIn(t, dir, adapters)
			assertProblems(t, problems, tc.wantPlan, "")
			if joined := strings.Join(problems, "\n"); strings.Contains(joined, "x/earlier.go") || strings.Contains(joined, "y/y.go") || strings.Contains(joined, "newtool/1.0.0.yaml") || strings.Contains(joined, "eslint/1.0.0.yaml") {
				t.Errorf("got %q: it names a file one of the branches already held", joined)
			}
		})
	}
}

// TestAPieceIsReadFromItsNearestBase: a reference that is an ancestor of
// another is no reference. A piece's references are
// its branch's tip and the fork with the other long-lived branch; the fork is
// an ancestor of the tip, says nothing of what the piece changed, and a path
// that merely equals the fork's copy must not be dropped from the change. Each
// piece adds a version and, besides, undoes something its own branch holds
// that the fork does not: all three are red, and the controls beside them, which
// the fork's copy never hid, are red as well.
func TestAPieceIsReadFromItsNearestBase(t *testing.T) {
	t.Parallel()
	adapters := adaptersFolder(t)
	version := adapters + "/eslint/1.2.0.yaml"
	for _, tc := range []struct {
		name   string
		onMain bool
		change func(t *testing.T, dir string)
		want   string
	}{
		{"a release-1 piece deletes a Go file born on release-1", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "-b", "piece")
			putFile(t, dir, version, "tool: eslint\n")
			wideGit(t, dir, "rm", "-q", "x/earlier.go")
			commitAll(t, dir, "a tool, and a Go file removed")
		}, "x/earlier.go"},
		{"a release-1 piece restores the fork's copy of a Go file release-1 changed", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "release-1")
			putFile(t, dir, "x/x.go", "package x\n\nvar changed = 1\n")
			commitAll(t, dir, "release-1 changes a Go file")
			wideGit(t, dir, "update-ref", "refs/remotes/origin/release-1", "HEAD")
			wideGit(t, dir, "checkout", "-q", "-b", "piece")
			putFile(t, dir, version, "tool: eslint\n")
			putFile(t, dir, "x/x.go", "package x\n")
			commitAll(t, dir, "a tool, and a Go file put back")
		}, "x/x.go"},
		{"a main piece deletes a Go file born on main since the fork", true, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "main")
			wideGit(t, dir, "checkout", "-q", "-b", "piece")
			putFile(t, dir, version, "tool: eslint\n")
			wideGit(t, dir, "rm", "-q", "y/y.go")
			commitAll(t, dir, "a tool, and a Go file removed")
		}, "y/y.go"},
		{"control: a release-1 piece deletes a Go file both branches hold", false, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "-b", "piece")
			putFile(t, dir, version, "tool: eslint\n")
			wideGit(t, dir, "rm", "-q", "x/x.go")
			commitAll(t, dir, "a tool, and a Go file removed")
		}, "x/x.go"},
		{"control: a main piece edits a Go file born on main since the fork", true, func(t *testing.T, dir string) {
			t.Helper()
			wideGit(t, dir, "checkout", "-q", "main")
			wideGit(t, dir, "checkout", "-q", "-b", "piece")
			putFile(t, dir, version, "tool: eslint\n")
			putFile(t, dir, "y/y.go", "package y\n\nvar a = 1\n")
			commitAll(t, dir, "a tool, and a Go file edited")
		}, "y/y.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dir string
			if tc.onMain {
				dir = plantDiverged(t, adapters)
			} else {
				dir = plantRepo(t, adapters, true)
			}
			tc.change(t, dir)
			assertProblems(t, toolIsDataIn(t, dir, adapters), []string{version + " is added", tc.want}, "")
		})
	}
}
