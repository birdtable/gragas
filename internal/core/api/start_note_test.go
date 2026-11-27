package coreapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestAStartIsJournalledBeforeItIsMadeAsAStartRecord: the Note writes one KindStart record of the run, its Detail the Noted,
// and each start is its own record, in the order they were noted.
// Risk: a program started with no record, or a start journalled as a step that
// recovery would refuse.
func TestAStartIsJournalledBeforeItIsMadeAsAStartRecord(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Seed("/repo", map[string]string{})
	ports := w.Ports()
	root, err := ports.Tree.Open("/repo")
	if err != nil {
		t.Fatal(err)
	}
	begun(t, ports.Journal, root, "run-1")
	note := coreapi.StartNote(ports.Journal, root, "run-1", "gragas check", "1.0.0")
	first := plugin.Noted{Round: 0, Step: 0, Grant: "tool", VersionAsk: true, Words: []string{"--version"}}
	second := plugin.Noted{Round: 1, Step: 2, Grant: "tool", Program: "/tools/x", Version: "2.15.0", Words: []string{"config", "verify"}, Level: plugin.LevelIsolated}
	for _, n := range []plugin.Noted{first, second} {
		if err := note(t.Context(), n); err != nil {
			t.Fatalf("Note(%+v) = %v", n, err)
		}
	}
	recs, torn, err := ports.Journal.Records(t.Context(), root)
	if err != nil || torn != 0 || len(recs) != 3 {
		t.Fatalf("Records = %d records, %d torn, %v; want the intent and two starts", len(recs), torn, err)
	}
	for i, want := range []plugin.Noted{first, second} {
		r := recs[i+1]
		var got plugin.Noted
		if r.Kind != model.KindStart || r.Run != "run-1" || r.Command != "gragas check" || json.Unmarshal(r.Detail, &got) != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("record %d = %s run %q detail %s, want a start of run-1 holding %+v", i, r.Kind, r.Run, r.Detail, want)
		}
	}
}

// TestAStartTheJournalRefusesIsNotStarted: a Note whose record the journal
// cannot take answers an error naming the grant, so the step is not made.
// Risk: a program started that the journal never heard of.
func TestAStartTheJournalRefusesIsNotStarted(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Seed("/repo", map[string]string{})
	ports := w.Ports()
	root, err := ports.Tree.Open("/repo")
	if err != nil {
		t.Fatal(err)
	}
	note := coreapi.StartNote(ports.Journal, root, "no good run id!", "gragas check", "1.0.0")
	err = note(t.Context(), plugin.Noted{Grant: "tool"})
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("Note on a run id the journal refuses = %v, want an error", err)
	}
	if recs, _, _ := ports.Journal.Records(t.Context(), root); len(recs) != 0 {
		t.Errorf("the journal holds %d records after a refused start, want none", len(recs))
	}
}

// begun writes the intent of run, which begins it: on the host's journal a
// start of a run not yet begun is read as torn, so a row that writes starts
// writes this first.
func begun(t *testing.T, j port.Journal, root model.Root, run string) {
	t.Helper()
	if err := j.Append(t.Context(), root, model.Record{Run: run, Kind: model.KindIntent, Command: "gragas check", Detail: []byte("{}")}); err != nil {
		t.Fatalf("the intent of %s: %v", run, err)
	}
}
