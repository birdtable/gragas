package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/fstest"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The cause of a row that ends the run: a word of
// model.GateCauses the caller switches on, never read out of the sentence.

// causesOf is the cause of each row of p that ends the run, in order. A row
// that does not end the run and carries a cause, and a row that ends it with
// a cause outside model.GateCauses, are failures of t.
func causesOf(t *testing.T, p model.ProjectCheck) []model.GateCause {
	t.Helper()
	var out []model.GateCause
	for _, n := range p.NotChecked {
		switch {
		case n.Tool == "" && n.Cause != "":
			t.Errorf("the row %q does not end the run and carries the cause %q", n.Msg, n.Cause)
		case n.Tool != "" && !slices.Contains(model.GateCauses(), n.Cause):
			t.Errorf("the row of %s ends the run with the cause %q, which is none of %v", n.Tool, n.Cause, model.GateCauses())
		case n.Tool != "":
			out = append(out, n.Cause)
		}
	}
	return out
}

// noBase is a repository with one tracked file and no merge base with the
// base ref.
func noBase(t *testing.T) coreapi.RepoAudit {
	t.Helper()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", map[string]string{"x": "y"})
	w.SeedRepo(root, porttest.Repo{Index: map[string]string{"a.go": "package p\n"}, Bases: map[[2]string]string{{"origin/main", "HEAD"}: ""}})
	return coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: rules(), Names: listOf(t, "zorbquux"),
		Src: fstest.MapFS{"a.go": &fstest.MapFile{Data: []byte("package p\n")}}}
}

// TestEachRowThatEndsTheRunCarriesItsCause: every way a repository check
// whose rules bind is left unmade says its cause on the row, one of
// model.GateCauses: the history for a build with no git, a repository that
// cannot be opened and a range whose messages are not read whole; the base
// for a merge base not found, on the counts and on the messages of a range;
// the names list for a list the owned read refuses; the text for a given
// text not read. With no rule bound the same run has no row that ends it,
// and no row carries a cause.
// Planted failure: the row says its cause only in its sentence.
func TestEachRowThatEndsTheRunCarriesItsCause(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		want  []model.GateCause
		found func(*testing.T, map[string]string) coreapi.RepoFindings
	}{
		{"no git is the history, on each check", []model.GateCause{model.CauseHistory, model.CauseHistory, model.CauseHistory}, func(_ *testing.T, r map[string]string) coreapi.RepoFindings {
			return coreapi.CheckRepo(context.Background(), coreapi.RepoAudit{NoGit: true, Rules: r})
		}},
		{"a repository not opened is the history, on each check", []model.GateCause{model.CauseHistory, model.CauseHistory, model.CauseHistory}, func(t *testing.T, r map[string]string) coreapi.RepoFindings {
			t.Helper()
			return coreapi.UnopenedRepo(listOf(t, "zorbquux"), r, errors.New("git is not trusted"))
		}},
		{"no merge base is the base, on the counts", []model.GateCause{model.CauseBase}, func(t *testing.T, r map[string]string) coreapi.RepoFindings {
			t.Helper()
			a := noBase(t)
			a.Rules = r
			return coreapi.CheckRepo(context.Background(), a)
		}},
		{"no merge base is the base, on the messages of a range too", []model.GateCause{model.CauseBase, model.CauseBase}, func(t *testing.T, r map[string]string) coreapi.RepoFindings {
			t.Helper()
			a := noBase(t)
			a.Rules, a.Range = r, true
			return coreapi.CheckRepo(context.Background(), a)
		}},
		{"messages of a range not read whole are the history", []model.GateCause{model.CauseHistory}, func(t *testing.T, r map[string]string) coreapi.RepoFindings {
			t.Helper()
			const base, head = "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"
			files := map[string]string{"a.go": "package p\n"}
			w := porttest.NewWorld(t)
			root := w.Seed("repo", map[string]string{"x": "y"})
			w.SeedRepo(root, porttest.Repo{Index: files,
				Bases: map[[2]string]string{{"origin/main", "HEAD"}: base, {"HEAD", "HEAD"}: head},
				Heads: map[string]porttest.Rev{"base": {Commit: base, Files: files}, "work": {Commit: head, Files: files}},
				Logs:  map[[2]string][]model.Message{{base, head}: {{Commit: head, Text: "a message\x00cut"}}}})
			return coreapi.CheckRepo(context.Background(), coreapi.RepoAudit{History: w.Ports().History, FileNames: w.Ports().FileNames, Root: root, Rules: r, Names: listOf(t, "zorbquux"),
				Range: true, Src: fstest.MapFS{"a.go": &fstest.MapFile{Data: []byte("package p\n")}}})
		}},
		{"a refused names list is the names list", []model.GateCause{model.CauseNamesList}, func(t *testing.T, r map[string]string) coreapi.RepoFindings {
			t.Helper()
			a := trackedAudit(t, map[string]string{"a.md": "a"}, nil)
			a.Rules, a.NamesErr = r, fmt.Errorf("read the names file: %w", port.ErrNotOwned)
			return coreapi.CheckRepo(context.Background(), a)
		}},
		{"a given text not read is the text", []model.GateCause{model.CauseText}, func(t *testing.T, r map[string]string) coreapi.RepoFindings {
			t.Helper()
			a := trackedAudit(t, map[string]string{"a.md": "a"}, nil)
			w := porttest.NewWorld(t)
			a.Rules, a.Names, a.Owned, a.Texts = r, listOf(t, "zorbquux"), w.Ports().Owned, []string{"/data/x/missing.txt"}
			return coreapi.CheckRepo(context.Background(), a)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := causesOf(t, repoCheck(c.found(t, rules()))); !slices.Equal(got, c.want) {
				t.Errorf("the rows that end the run say %v; want %v", got, c.want)
			}
			if p := repoCheck(c.found(t, nil)); len(causesOf(t, p)) != 0 || p.Word() != model.Done {
				t.Errorf("with no rule bound: ended %q with %+v; want done and no row with a cause", p.Word(), p.NotChecked)
			}
		})
	}
}

// TestAToolNotAskedIsTheToolCause: a row of the project's own that ends the
// run, a tool with bindings that could not be asked, says the tool cause
// beside the three checks' rows, each with its own; a row that names no tool
// has none, and the project's rows are not written through.
// Planted failure: the tool rows carry no cause, so a caller that switches on
// it takes a tool not asked for nothing.
func TestAToolNotAskedIsTheToolCause(t *testing.T) {
	t.Parallel()
	own := []model.NotChecked{{Tool: "vale", Bindings: 2, Msg: "vale: no reader is held"}, {Msg: "no tier table is held"}}
	r := coreapi.CheckRepo(context.Background(), coreapi.RepoAudit{NoGit: true, Rules: rules()})
	got := r.AddTo(model.ProjectCheck{NotChecked: own})
	want := []model.GateCause{model.CauseTool, model.CauseHistory, model.CauseHistory, model.CauseHistory}
	if c := causesOf(t, got); !slices.Equal(c, want) {
		t.Errorf("the rows that end the run say %v; want %v", c, want)
	}
	if own[0].Cause != "" {
		t.Errorf("AddTo wrote the cause through the project's own rows: %+v", own)
	}

	w, l := projectOf(t, goRules(rule("go.errors.1", "active", "vale", "x"), rule("go.errors.2", "active", "gragas", "names")), map[string]string{"a": "b"}, "go")
	p, err := checkIn(t, w, l, asked())
	if err != nil {
		t.Fatal(err)
	}
	if c := causesOf(t, p); !slices.Equal(c, []model.GateCause{model.CauseTool, model.CauseHistory}) {
		t.Errorf("the project check's rows that end the run say %v of %+v; want the tool for vale and the history for the names", c, p.NotChecked)
	}
}
