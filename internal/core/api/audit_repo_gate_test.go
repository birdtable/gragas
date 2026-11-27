package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The three repository checks as the project check lists them:
// each finding under its kind, and each row that says a check was not made,
// with whether it ends the run not ready.

// listOf is a names list that holds the one name.
func listOf(t *testing.T, name string) *format.Names {
	t.Helper()
	l, err := format.ParseNames([]byte(name + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// repoCheck is the project check that holds only what the three checks found.
func repoCheck(r coreapi.RepoFindings) model.ProjectCheck { return r.AddTo(model.ProjectCheck{}) }

// rowSaying is the rows whose message holds s.
func rowSaying(p model.ProjectCheck, s string) []model.NotChecked {
	var out []model.NotChecked
	for _, n := range p.NotChecked {
		if strings.Contains(n.Msg, s) {
			out = append(out, n)
		}
	}
	return out
}

// TestARefusedNamesListEndsNotReady: a list the owned read refuses is a row
// with the names check behind it, which ends the run not ready, and says the
// closed reason, never where the list lies.
func TestARefusedNamesListEndsNotReady(t *testing.T) {
	t.Parallel()
	a := trackedAudit(t, map[string]string{"a.md": "a"}, nil)
	a.NamesErr = fmt.Errorf("read the names file /data/alice/names.txt: %w", port.ErrNotOwned)
	p := repoCheck(coreapi.CheckRepo(context.Background(), a))
	if p.Word() != model.NotReady {
		t.Errorf("a refused list ends %q, want not ready", p.Word())
	}
	got := p.Unjudged()
	if len(got) != 1 || got[0].Tool != "gragas names" || got[0].Bindings != 1 || !strings.Contains(got[0].Msg, "not owned by this account") {
		t.Fatalf("rows with a tool %+v, want the names check's, with its one binding and the reason", got)
	}
	if strings.Contains(got[0].Msg, "alice") || strings.Contains(got[0].Msg, "/data") {
		t.Errorf("the row says where the list lies: %q", got[0].Msg)
	}
}

// TestARowTheProjectChoseOrCannotCureLeavesTheEndingDone: no list named, an
// empty list, a check no rule binds, an empty tracked list and the NUL-byte
// row are each said, and none of them ends the run other than done.
func TestARowTheProjectChoseOrCannotCureLeavesTheEndingDone(t *testing.T) {
	t.Parallel()
	empty, err := format.ParseNames([]byte("# none\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, says string
		audit      func(*testing.T) coreapi.RepoAudit
	}{
		{"no list named", "no names list was given", func(t *testing.T) coreapi.RepoAudit {
			t.Helper()
			return trackedAudit(t, map[string]string{"a.md": "a"}, nil)
		}},
		{"an empty list", "the names list is empty", func(t *testing.T) coreapi.RepoAudit {
			t.Helper()
			a := trackedAudit(t, map[string]string{"a.md": "a"}, nil)
			a.Names = empty
			return a
		}},
		{"no rule binds", "no rule of the rulebook binds", func(t *testing.T) coreapi.RepoAudit {
			t.Helper()
			a := trackedAudit(t, map[string]string{"a.md": "a"}, nil)
			a.Rules = nil
			return a
		}},
		{"an empty tracked list", "empty list of tracked files", func(t *testing.T) coreapi.RepoAudit {
			t.Helper()
			return trackedAudit(t, map[string]string{}, nil)
		}},
		{"a file with a NUL byte", "hold a NUL byte", func(t *testing.T) coreapi.RepoAudit {
			t.Helper()
			a := trackedAudit(t, map[string]string{"a.png": "\x89PNG\x00"}, nil)
			a.Names = listOf(t, "zorbquux")
			return a
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			p := repoCheck(coreapi.CheckRepo(context.Background(), c.audit(t)))
			if len(rowSaying(p, c.says)) == 0 {
				t.Errorf("no row says %q: %+v", c.says, p.NotChecked)
			}
			if p.Word() != model.Done {
				t.Errorf("ends %q with %+v; want done", p.Word(), p.Unjudged())
			}
		})
	}
}

// TestAWholeCheckNotMadeEndsNotReady: git not set up, and a merge base not
// found, leave a check whose rules bind unjudged for a cause the project can
// cure: the run ends not ready, with the count of the rules not judged. With
// no rule bound the same cause is a row.
func TestAWholeCheckNotMadeEndsNotReady(t *testing.T) {
	t.Parallel()
	noGit := coreapi.RepoAudit{NoGit: true, Rules: rules()}
	p := repoCheck(coreapi.CheckRepo(context.Background(), noGit))
	want := map[string]int{"gragas names": 1, "gragas tracked": 2, "gragas counts": 2}
	got := map[string]int{}
	for _, n := range p.Unjudged() {
		got[n.Tool] = n.Bindings
	}
	if p.Word() != model.NotReady || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("no git ends %q with %v; want not ready with %v", p.Word(), got, want)
	}
	if unbound := repoCheck(coreapi.CheckRepo(context.Background(), coreapi.RepoAudit{NoGit: true})); unbound.Word() != model.Done {
		t.Errorf("no git and no rule bound ends %q; want done", unbound.Word())
	}

	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: map[string]string{"a.go": "package p\n"}, Bases: map[[2]string]string{{"origin/main", "HEAD"}: ""}})
	a := coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Names: listOf(t, "zorbquux"),
		Src: fstest.MapFS{"a.go": &fstest.MapFile{Data: []byte("package p\n")}}}
	p = repoCheck(coreapi.CheckRepo(context.Background(), a))
	if u := p.Unjudged(); p.Word() != model.NotReady || len(u) != 1 || u[0].Tool != "gragas counts" || !strings.Contains(u[0].Msg, "merge base") {
		t.Errorf("no merge base ends %q with %+v; want not ready on the counts", p.Word(), u)
	}
}

// TestEachFindingIsListedUnderItsKindInTheOrderOfTheKinds: a name, a link, a
// fold and a new directive are each a finding of their kind, names first,
// then the tracked files, then the counts, each with its place, its rule and
// its fix.
func TestEachFindingIsListedUnderItsKindInTheOrderOfTheKinds(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	head := map[string]string{"a.go": "package p\n\n//nolint:errcheck\nvar x = 1\n", "B.md": "b", "b.md": "b", "z.md": "about zorbquux\n"}
	w.SeedRepo(root, porttest.Repo{Index: head, IndexLinks: map[string]string{"l.lnk": "z.md"},
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: map[string]string{"a.go": "package p\n"}}}})
	work := fstest.MapFS{}
	for p, s := range head {
		work[p] = &fstest.MapFile{Data: []byte(s)}
	}
	a := coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Src: work, Rules: rules(), Names: listOf(t, "zorbquux"), Counting: linterCounting()}
	p := repoCheck(coreapi.CheckRepo(context.Background(), a))
	var kinds []model.GateKind
	for _, f := range p.Findings {
		kinds = append(kinds, f.Kind)
		if f.Entity == "" || f.Msg == "" || f.Fix == "" || !strings.Contains(f.Msg, "(gragas ") {
			t.Errorf("finding %+v lacks its place, its rule or its fix", f)
		}
	}
	want := []model.GateKind{model.GateNames, model.GateTrackedFiles, model.GateTrackedFiles, model.GateCounts}
	if !slices.Equal(kinds, want) {
		t.Fatalf("kinds %q, want %q", kinds, want)
	}
	if f := p.Findings[3]; f.File != "a.go" || f.Line != 3 || f.Entity != "a.go:3" {
		t.Errorf("the directive's finding is at %q line %d (%q); want a.go:3", f.File, f.Line, f.Entity)
	}
	if !p.FailOn([]model.GateKind{model.GateCounts}) || p.FailOn([]model.GateKind{model.GateToolConfig}) {
		t.Errorf("--fail-on counts does not end on the counts, or tool-config does")
	}
}

// TestNoGateFindingOrRowSaysAName: a path, a text and a reason that hold a
// name are masked in the project check's shapes too, and a finding at a path
// that holds a name carries no file for the screen to print.
func TestNoGateFindingOrRowSaysAName(t *testing.T) {
	t.Parallel()
	a := trackedAudit(t, map[string]string{"docs/zorbquux.md": "about zorbquux\n", "docs/clean.md": "ok"}, map[string]string{"docs/zorbquux.lnk": "docs/clean.md"})
	a.Names = listOf(t, "zorbquux")
	p := repoCheck(coreapi.CheckRepo(context.Background(), a))
	if len(p.Findings) == 0 {
		t.Fatal("no finding")
	}
	for _, f := range p.Findings {
		for _, s := range []string{f.Entity, f.Msg, f.Fix, f.File} {
			if strings.Contains(strings.ToLower(s), "zorbquux") {
				t.Errorf("a finding says the name: %q", s)
			}
		}
	}
	for _, n := range p.NotChecked {
		if strings.Contains(strings.ToLower(n.Tool+n.Msg), "zorbquux") {
			t.Errorf("a row says the name: %+v", n)
		}
	}
}

// TestAnUnopenedRepositoryIsMaskedAndEndsNotReady: the reason the repository
// could not be opened is masked as CheckRepo's rows are (the path can hold a
// login), each bound check halts the run, and no other row is said.
func TestAnUnopenedRepositoryIsMaskedAndEndsNotReady(t *testing.T) {
	t.Parallel()
	err := errors.New("open /data/zorbquux/repo: permission denied")
	r := coreapi.UnopenedRepo(listOf(t, "zorbquux"), rules(), err)
	p := repoCheck(r)
	if p.Word() != model.NotReady || len(p.Unjudged()) != 3 || len(p.NotChecked) != 3 {
		t.Errorf("ends %q with %+v; want not ready on the three checks, and no other row", p.Word(), p.NotChecked)
	}
	for _, n := range r.NotChecked {
		if strings.Contains(n.Why, "zorbquux") {
			t.Errorf("a row says the name: %q", n.Why)
		}
	}
	for _, n := range p.NotChecked {
		if strings.Contains(n.Msg, "zorbquux") {
			t.Errorf("a row says the name: %q", n.Msg)
		}
	}
	var none *format.Names
	if got := coreapi.UnopenedRepo(none, nil, err); repoCheck(got).Word() != model.Done {
		t.Errorf("with no list and no rule bound it ends %q; want done", repoCheck(got).Word())
	}
}

// TestAddToKeepsTheProjectsOwnFindingsFirst: the project's findings come
// before the three checks', as model.ProjectKinds orders them, and the
// project's slices are not written through.
func TestAddToKeepsTheProjectsOwnFindingsFirst(t *testing.T) {
	t.Parallel()
	own := []model.GateFinding{{Kind: model.GateToolConfig, Entity: "rule:x", Msg: "weaker"}}
	p := model.ProjectCheck{Findings: own[:1:1]}
	r := coreapi.RepoFindings{Gate: []model.GateFinding{{Kind: model.GateNames}}, Unjudged: []model.NotChecked{{Msg: "a row"}}}
	got := r.AddTo(p)
	if len(got.Findings) != 2 || got.Findings[0].Kind != model.GateToolConfig || got.Findings[1].Kind != model.GateNames || len(got.NotChecked) != 1 {
		t.Errorf("AddTo gave %+v", got)
	}
	if len(p.Findings) != 1 {
		t.Errorf("AddTo wrote through the project's findings: %+v", p.Findings)
	}
}

// TestARefusedGivenTextEndsNotReady: a text the person named that cannot be
// read is the check they asked for not made, so with names bound the run ends
// not ready on the names, naming the text by its place; with no rule binding
// names it leaves the ending done.
func TestARefusedGivenTextEndsNotReady(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: map[string]string{"a.md": "a"},
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: "c0"},
		Heads: map[string]porttest.Rev{"base": {Commit: "c0", Files: map[string]string{"a.md": "a"}}}})
	a := coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Names: listOf(t, "zorbquux"),
		Src: fstest.MapFS{"a.md": &fstest.MapFile{Data: []byte("a")}}, Owned: w.Ports().Owned,
		Texts: []string{"/data/x/missing.txt"}}
	p := repoCheck(coreapi.CheckRepo(context.Background(), a))
	u := p.Unjudged()
	if p.Word() != model.NotReady || len(u) != 1 || u[0].Tool != "gragas names" || u[0].Bindings != 1 || !strings.Contains(u[0].Msg, "given text 1") {
		t.Errorf("a refused given text ends %q with %+v; want not ready on the names, at given text 1", p.Word(), u)
	}
	a.Rules = map[string]string{"link": "t.link"}
	if p := repoCheck(coreapi.CheckRepo(context.Background(), a)); p.Word() != model.Done {
		t.Errorf("a refused given text with names unbound ends %q; want done", p.Word())
	}
}

// TestUnreadRangeMessagesEndNotReady: --range names the commit messages, so a
// range whose messages cannot be read whole ends not ready on the names, while
// the counts, whose merge base was found, are judged.
func TestUnreadRangeMessagesEndNotReady(t *testing.T) {
	t.Parallel()
	const base, head = "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"
	files := map[string]string{"a.go": "package p\n"}
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: files,
		Bases: map[[2]string]string{{"origin/main", "HEAD"}: base, {"HEAD", "HEAD"}: head},
		Heads: map[string]porttest.Rev{"base": {Commit: base, Files: files}, "work": {Commit: head, Files: files}},
		Logs:  map[[2]string][]model.Message{{base, head}: {{Commit: head, Text: "a message\x00cut"}}}})
	a := coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Names: listOf(t, "zorbquux"), Range: true,
		Src: fstest.MapFS{"a.go": &fstest.MapFile{Data: []byte("package p\n")}}}
	p := repoCheck(coreapi.CheckRepo(context.Background(), a))
	u := p.Unjudged()
	if p.Word() != model.NotReady || len(u) != 1 || u[0].Tool != "gragas names" || !strings.Contains(u[0].Msg, "commit messages of the range") {
		t.Errorf("unread range messages end %q with %+v; want not ready on the names alone", p.Word(), u)
	}
}
