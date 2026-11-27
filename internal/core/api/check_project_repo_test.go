package coreapi_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/project"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The three repository checks inside the project check: what
// checkProject hands them, and what it lists of what they found.

// repoRulesOfGo is the go rules that bind the task gate and the five keys the
// three checks compute, the last as a trial rule.
func repoRulesOfGo() string {
	return goRules(rule("go.errors.1", "active", "task", "gate"),
		rule("go.errors.2", "active", "gragas", "names"), rule("go.errors.3", "active", "gragas", "link"),
		rule("go.errors.4", "active", "gragas", "fold"), rule("go.errors.5", "active", "gragas", "debt"),
		rule("go.errors.6", "trial", "gragas", "directive"))
}

// repoFiles is a project with one thing wrong for each of the three checks: a
// name in a text, two paths that differ only by case, and a directive the
// merge base does not have.
func repoFiles() map[string]string {
	return map[string]string{"Taskfile.yml": taskfileWith, "a.go": "package p\n\n//nolint:errcheck\nvar x = 1\n",
		"B.md": "b", "b.md": "b", "z.md": "about zorbquux\n"}
}

// tracked seeds the project's repository with files as its index, and a.go
// without its directive at the merge base of origin/main and HEAD; the ask it
// answers opens that history and names a list that holds zorbquux.
func tracked(t *testing.T, w projWorld, l project.Loaded, files map[string]string) coreapi.ProjectAsk {
	t.Helper()
	w.SeedRepo(l.Root, porttest.Repo{Index: files,
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: map[string]string{"a.go": "package p\n"}}}})
	w.SeedOutside("/names", "zorbquux\n")
	a := asked()
	a.OpenHistory = func() (port.History, error) { return w.Ports().History, nil }
	return a
}

func kindsOf(p model.ProjectCheck) []model.GateKind {
	out := make([]model.GateKind, len(p.Findings))
	for i, f := range p.Findings {
		out[i] = f.Kind
	}
	return out
}

// TestCheckProjectListsTheThreeRepositoryChecksAfterItsOwn: below a pin, the
// names, the tracked files and the counts are judged over the project's own
// tree and history, under the rules of the pinned parts, and listed after the
// project's own findings in the order of the kinds. The run still ends done,
// and --fail-on ends it on each of the three kinds.
// Planted failure: checkProject returns its own findings without repoChecks'.
func TestCheckProjectListsTheThreeRepositoryChecksAfterItsOwn(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, repoRulesOfGo(), repoFiles(), "go")
	got, err := checkIn(t, w, l, tracked(t, w, l, repoFiles()))
	if err != nil {
		t.Fatal(err)
	}
	want := []model.GateKind{model.GateToolConfig, model.GateNames, model.GateTrackedFiles, model.GateCounts}
	if kinds := kindsOf(got); !slices.Equal(kinds, want) {
		t.Fatalf("kinds %q, want %q\n%+v\n%+v", kinds, want, got.Findings, got.NotChecked)
	}
	for i, at := range map[int]string{1: "z.md:1", 2: "B.md", 3: "a.go:3"} {
		if f := got.Findings[i]; f.Entity != at || f.Fix == "" || !strings.Contains(f.Msg, "go.errors.") {
			t.Errorf("finding %d is %+v; want it at %s, under its rule, with its fix", i, f, at)
		}
	}
	if got.Word() != model.Done {
		t.Errorf("ended %q, want done: a finding ends the run only under --fail-on\n%+v", got.Word(), got.NotChecked)
	}
	for _, k := range want {
		if !got.FailOn([]model.GateKind{k}) {
			t.Errorf("--fail-on %s does not end the run on findings", k)
		}
	}
	doc := coreapi.ProjectDocOf(got, got.Word())
	if !slices.Equal(doc.Findings, got.Findings) {
		t.Errorf("the document's findings are %+v; want the four", doc.Findings)
	}
}

// TestCheckProjectACleanRepositoryHasNoRepositoryFindingAndNoRow is the
// control: with nothing wrong, the three checks add no finding and no row.
func TestCheckProjectACleanRepositoryHasNoRepositoryFindingAndNoRow(t *testing.T) {
	t.Parallel()
	files := map[string]string{"Taskfile.yml": "version: '3'\ntasks:\n  gate: {cmds: [echo]}\n", "a.go": "package p\n"}
	w, l := projectOf(t, repoRulesOfGo(), files, "go")
	got, err := checkIn(t, w, l, tracked(t, w, l, files))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 0 || got.Word() != model.Done {
		t.Errorf("a clean project has findings %+v and ends %q", got.Findings, got.Word())
	}
	for _, n := range got.NotChecked {
		if strings.Contains(n.Msg, "gragas ") {
			t.Errorf("a clean project has the row %+v", n)
		}
	}
}

// TestCheckProjectJudgesTheRepositoryUnderThePinnedPartsOnly: a rule of a part
// the pin does not name, and a proposed rule, bind none of the three checks,
// so the names check and the counts say no rule binds them, the same tree has
// no finding of theirs, and the history is never opened: a project that asks
// for none of the three has none of its files read for them.
// Planted failure: repoRules reads every rule of the book.
func TestCheckProjectJudgesTheRepositoryUnderThePinnedPartsOnly(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		rules string
		parts []string
	}{
		"another part":  {repoRulesOfGo(), []string{"ts"}},
		"proposed only": {goRules(rule("go.errors.1", "active", "task", "lint"), rule("go.errors.2", "proposed", "gragas", "names"), rule("go.errors.3", "proposed", "gragas", "debt")), []string{"go"}},
	} {
		w, l := projectOf(t, c.rules, repoFiles(), c.parts...)
		a := tracked(t, w, l, repoFiles())
		a.OpenHistory = func() (port.History, error) {
			t.Errorf("%s: the history was opened with no rule to judge under", name)
			return nil, errors.New("opened")
		}
		got, err := checkIn(t, w, l, a)
		if err != nil {
			t.Fatal(name, err)
		}
		if len(got.Findings) != 0 || got.Word() != model.Done || len(got.NotChecked) == 0 {
			t.Errorf("%s: findings %+v, ended %q; want none and done", name, got.Findings, got.Word())
		}
		if len(rowSaying(got, "no rule of the rulebook binds the names key")) != 1 || len(rowSaying(got, "binds a count key")) != 1 {
			t.Errorf("%s: the rows are %+v; want one each for the names and the counts", name, got.NotChecked)
		}
	}
}

// TestCheckProjectSaysAKeyNoCheckJudgesNotChecked: a rule of the pinned parts
// that binds a key of the program none of the three checks judges, replace
// here, is one row that names the key and the rule, and the ending is as it
// is: with the names bound beside it, and with nothing else bound, where the
// history is still never opened. A part the pin does not name gives no row.
// Planted failure: the rule is passed in silence.
func TestCheckProjectSaysAKeyNoCheckJudgesNotChecked(t *testing.T) {
	t.Parallel()
	files := map[string]string{"Taskfile.yml": "version: '3'\ntasks:\n  gate: {cmds: [echo]}\n", "a.go": "package p\n"}
	const row = "gragas replace rules were not checked: go.errors.7 binds the replace key, which no check below a pin judges"
	for _, c := range []struct {
		name, rules, part string
		opens             bool
		want              int
	}{
		{"beside a bound check, one row", goRules(rule("go.errors.1", "active", "task", "gate"), rule("go.errors.2", "active", "gragas", "names"), rule("go.errors.7", "active", "gragas", "replace")), "go", true, 1},
		{"bound alone, one row and no history", goRules(rule("go.errors.1", "active", "task", "gate"), rule("go.errors.7", "trial", "gragas", "replace")), "go", false, 1},
		{"a proposed rule, no row", goRules(rule("go.errors.1", "active", "task", "gate"), rule("go.errors.7", "proposed", "gragas", "replace")), "go", false, 0},
		{"a part the pin does not name, no row", goRules(rule("go.errors.1", "active", "task", "gate"), rule("go.errors.7", "active", "gragas", "replace")), "ts", false, 0},
	} {
		w, l := projectOf(t, c.rules, files, c.part)
		a := tracked(t, w, l, files)
		if !c.opens {
			a.OpenHistory = func() (port.History, error) {
				t.Errorf("%s: the history was opened with none of the three checks bound", c.name)
				return nil, errors.New("opened")
			}
		}
		got, err := checkIn(t, w, l, a)
		if err != nil {
			t.Fatal(c.name, err)
		}
		rows := rowSaying(got, "replace")
		if len(rows) != c.want || (c.want == 1 && (rows[0].Msg != row || rows[0].Tool != "")) {
			t.Errorf("%s: the rows about replace are %+v; want %d, saying %q with no check behind it", c.name, rows, c.want, row)
		}
		if len(got.Findings) != 0 || got.Word() != model.Done {
			t.Errorf("%s: findings %+v, ended %q; want none and done", c.name, got.Findings, got.Word())
		}
	}
}

// TestCheckProjectWithNoGitEndsNotReadyWhenARuleBindsACheck: a build with no
// git cannot read the tracked files, so each of the three checks a rule binds
// is a row with the check behind it and the run ends not ready; with no rule
// bound nothing was to be read, so no row says git and the run ends done.
// Planted failure: a check not made is a pass.
func TestCheckProjectWithNoGitEndsNotReadyWhenARuleBindsACheck(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, repoRulesOfGo(), repoFiles(), "go")
	got, err := checkIn(t, w, l, asked())
	if err != nil {
		t.Fatal(err)
	}
	if got.Word() != model.NotReady {
		t.Fatalf("ended %q, want not ready\n%+v", got.Word(), got.NotChecked)
	}
	for tool, n := range map[string]int{"gragas names": 1, "gragas tracked": 2, "gragas counts": 2} {
		if row, ok := rowOf(got, tool); !ok || row.Bindings != n || !strings.Contains(row.Msg, "git is not set up here") {
			t.Errorf("the row of %s is %+v (%t); want %d bindings and the reason", tool, row, ok, n)
		}
	}
	w, l = projectOf(t, goRules(rule("go.errors.1", "active", "task", "lint")), map[string]string{"Taskfile.yml": taskfileWith}, "go")
	if got, _ = checkIn(t, w, l, asked()); got.Word() != model.Done || len(rowSaying(got, "git is not set up here")) != 0 {
		t.Errorf("with no rule bound: ended %q with rows %+v; want done and no row about git", got.Word(), got.NotChecked)
	}
}

// TestCheckProjectAHistoryThatCannotBeOpenedIsSaidMaskedAndEndsNotReady: the
// opener's reason is said by each of the three rows, with a name of the list
// hidden in it, and the run ends not ready.
// Planted failure: the reason is printed as the adapter gave it.
// Risk: a login on the list, in git's path, printed by a check for names.
func TestCheckProjectAHistoryThatCannotBeOpenedIsSaidMaskedAndEndsNotReady(t *testing.T) {
	t.Parallel()
	w, l := projectOf(t, repoRulesOfGo(), repoFiles(), "go")
	a := tracked(t, w, l, repoFiles())
	a.OpenHistory = func() (port.History, error) { return nil, errors.New("git at /data/zorbquux/git is not trusted") }
	got, err := checkIn(t, w, l, a)
	if err != nil {
		t.Fatal(err)
	}
	if rows := rowSaying(got, "the repository could not be opened: git at /data/<name>/git is not trusted"); len(rows) != 3 || got.Word() != model.NotReady {
		t.Errorf("ended %q with %d rows that say the masked reason; want not ready and three\n%+v", got.Word(), len(rows), got.NotChecked)
	}
	for _, n := range got.NotChecked {
		if strings.Contains(n.Msg, "zorbquux") {
			t.Errorf("the row %q says the name", n.Msg)
		}
	}
}
