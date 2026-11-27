package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// applied runs the write path over a root seeded with files, writing each of
// data's files whole and adding the record at record (a line appended when
// appendRecord, a file replaced whole otherwise), and answers the paths the
// Writer took.
func applied(t *testing.T, files map[string]string, data []string, record string, appendRecord bool) []string {
	t.Helper()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("/work/root", files)
	before := func(p string) model.Digest {
		if c, ok := w.File(root, p); ok {
			return apply.DigestBytes([]byte(c))
		}
		return ""
	}
	o := apply.Run(t.Context(), w.Ports(), root, apply.Spec{
		Command: "gragas test --apply", Engine: "v0.0.0-test", Run: "run-1", Scope: append(slices.Clone(data), record),
		Plan: func(context.Context) (model.Plan, error) {
			return model.Plan{
				Delta: model.Delta{Changes: []model.Change{{Entity: "x"}}},
				Yes:   &model.Yes{Who: "A Person <a@example.test>", At: 1794132000, Command: "gragas test --apply"},
			}, nil
		},
		Render: func(model.Delta) ([]model.Target, error) {
			var out []model.Target
			for _, p := range data {
				out = append(out, model.Target{Path: p, Before: before(p), Data: []byte("{}\n")})
			}
			return out, nil
		},
		Record: func(model.Yes) (model.Target, error) {
			return model.Target{Path: record, Before: before(record), Data: []byte("{}"), Append: appendRecord}, nil
		},
	})
	if o.Word != model.Done {
		t.Fatalf("ended %q %+v, cause %v", o.Word, o.Failure, o.Cause)
	}
	var got []string
	for _, wr := range w.Writes {
		got = append(got, wr.Path)
	}
	return got
}

// unheldBy is the written paths of place that no row of table holds.
func unheldBy(table []config.Kept, place config.Place, written []string) []string {
	var missing []string
	for _, p := range written {
		if _, ok := config.Holds(table, place, p); !ok {
			missing = append(missing, p)
		}
	}
	return missing
}

// TestEveryPathTheWritePathWritesIsInThePersistedFilesTable: a rulebook's plan
// (generated data, then a history line) and a project's plan (waivers and
// baselines, then the lock rewritten whole) are run through apply, and each
// path the Writer took is a row of config.Persisted in the place it was
// written (architecture 5.5, store.seams.4). A table without the history row
// misses the history line, and a path nobody listed is missed. Risk: a file
// gragas writes that docs/settings.md does not say it keeps.
func TestEveryPathTheWritePathWritesIsInThePersistedFilesTable(t *testing.T) {
	t.Parallel()
	table := config.Persisted()
	rulebook := applied(t, map[string]string{"history.jsonl": "{}\n", "rulebook/rules.json": "{}\n"},
		[]string{"rulebook/rules.json", "rulebook/bindings.json"}, "history.jsonl", true)
	project := applied(t, map[string]string{".gragas/lock.yaml": "x: 1\n"},
		[]string{".gragas/waivers.yaml", ".gragas/baselines.yaml"}, ".gragas/lock.yaml", false)

	if len(rulebook) != 3 || len(project) != 3 {
		t.Fatalf("wrote %v and %v: the walk below checks too little", rulebook, project)
	}
	if m := unheldBy(table, config.InRepository, rulebook); len(m) != 0 {
		t.Errorf("a rulebook's writes not in the table: %v", m)
	}
	if m := unheldBy(table, config.InProject, project); len(m) != 0 {
		t.Errorf("a project's writes not in the table: %v", m)
	}
	var short []config.Kept
	for _, k := range table {
		if !slices.Contains(k.Names, "history.jsonl") {
			short = append(short, k)
		}
	}
	if m := unheldBy(short, config.InRepository, rulebook); !slices.Equal(m, []string{"history.jsonl"}) {
		t.Errorf("a table without the history row missed %v, want the history line alone", m)
	}
	if m := unheldBy(table, config.InRepository, []string{"rulebook/surprise.txt"}); len(m) != 1 {
		t.Errorf("a file nobody listed was held: %v", m)
	}
}
