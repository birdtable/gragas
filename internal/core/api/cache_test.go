package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const now = 1_800_000_000

// ask is the four kinds, the journal built, in the state and cache folders the
// test seeds.
func ask(state, cache string) coreapi.CacheAsk {
	return coreapi.CacheAsk{Keep: "30d", KeepSeconds: 30 * 86400, Kinds: []coreapi.CacheKind{
		{Name: "journal", Holds: "copies and closed runs", Dir: state, Names: []string{"*.journal", "*.copies/*/*"}, Journal: true, Built: true},
		{Name: "explain", Holds: "the index", Dir: cache, Names: []string{"explain/*"}},
	}}
}

func lines(l ...string) string { return strings.Join(l, "\n") + "\n" }

// rec is a record of run in the world's journal form, of kind at at.
func rec(version int, run, kind string, at int64) string {
	return porttest.JournalLine(model.Record{Version: version, Run: run, Kind: kind, At: at})
}

// TestListingTellsClosedRunsFromOpenOnes: the oldest entry is the oldest
// closed run, an open run is counted and never dates the journal, and a file
// the kind does not name is not counted. The records are the journal's
// reader's, RecordsOf over the file's bytes, so a torn line is
// skipped as the journal skips it. Risk: clear later removing evidence.
func TestListingTellsClosedRunsFromOpenOnes(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Unix(now, 0))
	journal := lines(
		rec(1, "r1", model.KindIntent, 1000),
		rec(1, "r1", model.KindClose, 1100),
		rec(1, "r2", model.KindIntent, 1200),
		rec(1, "r3", model.KindIntent, 1300),
		rec(1, "r3", model.KindClose, 1400),
		`{"Version":1,"Run":"r4","Kind":"intent`) // a torn last line
	journalBytes := int64(len(journal))
	w.Seed("/state", map[string]string{
		"aa.journal":      journal,
		"aa.copies/r1/dd": "old",
		"runs.jsonl":      "not the journal's",
		"gragas.lock":     "",
	})
	w.Seed("/cache", map[string]string{})
	got, err := coreapi.CacheList(t.Context(), w.Ports(), ask("/state", "/cache"))
	if err != nil {
		t.Fatal(err)
	}
	j := got.Kinds[0]
	if j.Files != 2 || j.Size == nil || *j.Size != journalBytes+3 {
		t.Errorf("journal files %d, size %v: want the journal and one copy, not the run log or the lock", j.Files, j.Size)
	}
	if j.Oldest == nil || *j.Oldest != 1100 || j.Age == nil || *j.Age != now-1100 {
		t.Errorf("oldest %v age %v, want the first close, 1100", j.Oldest, j.Age)
	}
	if j.Open == nil || *j.Open != 1 {
		t.Errorf("open %v, want r2 alone", j.Open)
	}
	if j.ClearedAfter != "30d" || got.Keep != "30d" {
		t.Errorf("cleared after %q", j.ClearedAfter)
	}
	ex := got.Kinds[1]
	if ex.Files != 0 || ex.Size == nil || *ex.Size != 0 || ex.Oldest != nil || ex.OldestNote == "" {
		t.Errorf("explain: %+v, want empty with a reason for the null", ex)
	}
}

// olderAt is a Journal whose reader refuses the bytes of a journal that hold
// marker as one of an older release's form, as the adapter's reader refuses
// release-1's lines: the fake's own form has no older one, so the refusal is
// a double.
type olderAt struct {
	port.Journal
	marker string
}

func (o olderAt) RecordsOf(ctx context.Context, data []byte) ([]model.Record, int, error) {
	if strings.Contains(string(data), o.marker) {
		return nil, 0, &model.OlderJournalError{Line: 1, Run: "r0"}
	}
	return o.Journal.RecordsOf(ctx, data)
}

// TestAJournalTheReaderRefusesIsARowThatSaysSo: a journal the
// journal's reader refuses, of a newer version or of an older release's form,
// dates nothing and is named in the row's note instead of a wrong age; the
// listing never ends on it. The other journal is still read, its open run
// counted, and the other kinds are still listed. The control is the same
// state folder without the refused journal, which dates the journal by its
// close. Risk: one journal a person cannot read hides the whole cache.
func TestAJournalTheReaderRefusesIsARowThatSaysSo(t *testing.T) {
	t.Parallel()
	readable := lines(rec(1, "r1", model.KindIntent, 1000), rec(1, "r1", model.KindClose, 1100), rec(1, "r2", model.KindIntent, 1200))
	for _, c := range []struct {
		name, refused string
		journal       func(port.Journal) port.Journal
		note          []string
	}{
		{"none", "", nil, nil},
		{"newer", lines(rec(9, "r9", model.KindClose, 1)), nil, []string{"version 9", "reads up to version 1"}},
		{"older", "the older form\n", func(j port.Journal) port.Journal { return olderAt{j, "the older form"} }, []string{"bb.journal", "older release"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			w.Instant(time.Unix(now, 0))
			state := map[string]string{"aa.journal": readable}
			if c.refused != "" {
				state["bb.journal"] = c.refused
			}
			w.Seed("/state", state)
			w.Seed("/cache", map[string]string{"explain/index": "x"})
			ports := w.Ports()
			if c.journal != nil {
				ports.Journal = c.journal(ports.Journal)
			}
			got, err := coreapi.CacheList(t.Context(), ports, ask("/state", "/cache"))
			if err != nil {
				t.Fatalf("CacheList = %v, want a listing", err)
			}
			j := got.Kinds[0]
			if j.Open == nil || *j.Open != 1 || j.Files != len(state) {
				t.Errorf("open %v files %d, want r2 of the journal read and %d files", j.Open, j.Files, len(state))
			}
			if len(got.Kinds) != 2 || got.Kinds[1].Files != 1 {
				t.Errorf("kinds %+v, want the explain kind listed after the journal's", got.Kinds)
			}
			if c.note == nil {
				if j.Oldest == nil || *j.Oldest != 1100 || j.OldestNote != "" {
					t.Errorf("control: oldest %v note %q, want 1100 and no note", j.Oldest, j.OldestNote)
				}
				return
			}
			if j.Oldest != nil || j.Age != nil {
				t.Errorf("oldest %v age %v, want null beside a refused journal", j.Oldest, j.Age)
			}
			for _, want := range c.note {
				if !strings.Contains(j.OldestNote, want) {
					t.Errorf("note %q, want it to name %q", j.OldestNote, want)
				}
			}
		})
	}
}

// TestACancelledListingEndsOnTheJournal: a cancel the journal's reader meets
// is no refused journal: the listing ends on it.
func TestACancelledListingEndsOnTheJournal(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Unix(now, 0))
	w.Seed("/state", map[string]string{"aa.journal": lines(rec(1, "r1", model.KindIntent, 1000))})
	w.Seed("/cache", map[string]string{})
	ports := w.Ports()
	ports.Journal = cancelledReader{ports.Journal}
	if _, err := coreapi.CacheList(t.Context(), ports, ask("/state", "/cache")); !errors.Is(err, context.Canceled) {
		t.Errorf("CacheList = %v, want the cancel", err)
	}
}

// cancelledReader is a Journal whose reader meets a cancel.
type cancelledReader struct{ port.Journal }

func (cancelledReader) RecordsOf(context.Context, []byte) ([]model.Record, int, error) {
	return nil, 0, fmt.Errorf("journal records: %w", context.Canceled)
}

// TestAKindWithNoFolderIsNullWithAReason: a machine with no state folder has
// an unknown size, not a zero.
func TestAKindWithNoFolderIsNullWithAReason(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Unix(now, 0))
	got, err := coreapi.CacheList(t.Context(), w.Ports(), ask("", ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range got.Kinds {
		if k.Size != nil || k.SizeNote == "" || k.Oldest != nil {
			t.Errorf("%s: %+v, want null with a reason", k.Kind, k)
		}
	}
}

// listFails is a Tree whose List fails, and readFails one whose Read fails,
// as a folder the account cannot read would: doubles, not permission bits.
type listFails struct {
	port.Tree
	err error
}

func (l listFails) List(model.Root, string, model.Scope) ([]model.Entry, error) { return nil, l.err }

type readFails struct {
	port.Tree
	err error
}

func (r readFails) Read(model.Root, string) ([]byte, error) { return nil, r.err }

// TestAnUnreadableFolderFailsTheListing: a List or a Read that fails is an
// error, never a folder that holds nothing. Risk: a cache that cannot be read
// listed as empty, so a person trusts a size of 0.
func TestAnUnreadableFolderFailsTheListing(t *testing.T) {
	t.Parallel()
	cause := errors.New("the disk said no")
	for name, wrap := range map[string]func(port.Tree) port.Tree{
		"list": func(t port.Tree) port.Tree { return listFails{t, cause} },
		"read": func(t port.Tree) port.Tree { return readFails{t, cause} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			w.Instant(time.Unix(now, 0))
			w.Seed("/state", map[string]string{"aa.journal": lines(rec(1, "r1", model.KindIntent, 1))})
			w.Seed("/cache", map[string]string{})
			ports := w.Ports()
			ports.Tree = wrap(ports.Tree)
			if _, err := coreapi.CacheList(t.Context(), ports, ask("/state", "/cache")); !errors.Is(err, cause) {
				t.Errorf("error %v, want the disk's", err)
			}
		})
	}
}

// TestAScratchFolderCountsEveryFileBelowIt: the scratch kind's name stops at
// a slash, so the files below a folder a run made are counted by the kind's
// Nested form, and a file the name does not reach is not. Risk: a listing
// that says "nothing is kept" over a cache full of fixtures.
func TestAScratchFolderCountsEveryFileBelowIt(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Unix(now, 0))
	w.Seed("/cache", map[string]string{
		"scratch/run-1/a.go":       "xx",
		"scratch/run-1/src/b.go":   "yyy",
		"scratch/run-2/src/d/e.go": "z",
		"scratch/stray":            "w",
		"explain/index":            "not scratch",
	})
	scratch := func(nested bool) coreapi.CacheAsk {
		return coreapi.CacheAsk{Keep: "30d", KeepSeconds: 30 * 86400, Kinds: []coreapi.CacheKind{
			{Name: "scratch", Holds: "run folders", Dir: "/cache", Names: []string{"scratch/*"}, Built: true, Nested: nested}}}
	}
	got, err := coreapi.CacheList(t.Context(), w.Ports(), scratch(true))
	if err != nil {
		t.Fatal(err)
	}
	if k := got.Kinds[0]; k.Files != 4 || k.Size == nil || *k.Size != 7 {
		t.Errorf("nested: %d files of %v bytes, want 4 files of 7 bytes: three below a run folder and the stray file", k.Files, k.Size)
	}
	flat, err := coreapi.CacheList(t.Context(), w.Ports(), scratch(false))
	if err != nil {
		t.Fatal(err)
	}
	if k := flat.Kinds[0]; k.Files != 1 {
		t.Errorf("not nested: %d files, want the stray file alone, which is the miss the Nested form fixes", k.Files)
	}
}
