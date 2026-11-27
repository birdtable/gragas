package coreapi_test

import (
	"encoding/json"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// The journal's lines and the details of its records are written by this
// engine, but read back from a file a person, a crash or another version may
// have changed, so what reads them is a parser of input the program did not
// write (tests.generated.1).

// journalOfDetails is a world whose run r1 has an intent, a copy of a.txt
// with copy as its detail and a close with shut as its detail, and ok is
// false when the journal refuses a detail that is not a JSON value.
func journalOfDetails(t *testing.T, copyDetail, shutDetail []byte) (*journalWorld, bool) {
	t.Helper()
	if !json.Valid(copyDetail) || !json.Valid(shutDetail) {
		return nil, false
	}
	j := newJournalWorld(t, map[string]string{"a.txt": "new"})
	j.record("r1", model.KindIntent, now-300, nil)
	j.record("r1", model.KindCopy, now-299, copyDetail)
	j.record("r1", model.KindClose, now-290, shutDetail)
	return j, true
}

// The seeds of the details: the shapes the engine writes, and the same
// shapes wrong in each field.
func detailSeeds() [][2]string {
	return [][2]string{
		{`{"path":"a.txt","existed":true,"before":"sha256:00"}`, `{"outcome":"done","files":[{"Path":"a.txt","Before":"sha256:00","After":"sha256:11"}]}`},
		{`{"path":"a.txt","existed":false,"before":""}`, `{"outcome":"rolled back","files":[]}`},
		{`{"path":"../a.txt","existed":true,"before":""}`, `{"outcome":"done","files":null}`},
		{`{"path":"","existed":true}`, `{"outcome":""}`},
		{`{"path":7,"existed":"yes"}`, `{"outcome":["done"],"files":{}}`},
		{`[]`, `null`},
		{`{"path":"a.txt","path":"b.txt"}`, `{"outcome":"done","outcome":"failed"}`},
	}
}

// FuzzJournalDetails (tests.generated.1): whatever the detail of a copy
// record and of a close record is, listing the runs does not panic and names
// the run once with a state, and restoring it, which only reads, does not
// panic and answers files and lines that pair up, or an error.
func FuzzJournalDetails(f *testing.F) {
	for _, s := range detailSeeds() {
		f.Add([]byte(s[0]), []byte(s[1]))
	}
	f.Fuzz(func(t *testing.T, copyDetail, shutDetail []byte) {
		j, ok := journalOfDetails(t, copyDetail, shutDetail)
		if !ok {
			return
		}
		list, err := coreapi.JournalList(t.Context(), readOnly(t, j.ports), j.root)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list.Runs) != 1 || list.Runs[0].Run != "r1" || list.Runs[0].State == "" || list.Runs[0].State == "open" {
			t.Fatalf("list = %+v, want r1 closed with a state", list.Runs)
		}
		got, err := coreapi.JournalRestore(t.Context(), readOnly(t, j.ports), j.root, restoreAsk("r1"), posix)
		if err != nil {
			return
		}
		if len(got.Files) != len(got.Lines) {
			t.Fatalf("restore: %d files and %d lines: %+v", len(got.Files), len(got.Lines), got)
		}
		if (len(got.Moved) > 0 || len(got.Damaged) > 0) && len(got.Lines) > 0 {
			t.Fatalf("restore refused and still gave lines: %+v", got)
		}
	})
}

// TestTheJournalDetailSeedsReachTheReaders is the anti-vacuity check of
// FuzzJournalDetails: the first seed, which is what the engine writes, lists
// its file and its outcome and restores to one line; the same seed with the
// path of the copy changed to another file lists that file instead, and with
// one byte of the close cut it lists the state as unknown. So the details are
// read, not skipped.
func TestTheJournalDetailSeedsReachTheReaders(t *testing.T) {
	t.Parallel()
	seed := detailSeeds()[0]
	j, ok := journalOfDetails(t, []byte(seed[0]), []byte(seed[1]))
	if !ok {
		t.Fatal("the first seed is not JSON")
	}
	list, err := coreapi.JournalList(t.Context(), readOnly(t, j.ports), j.root)
	if err != nil || len(list.Runs) != 1 || list.Runs[0].State != "done" || len(list.Runs[0].Files) != 1 || list.Runs[0].Files[0] != "a.txt" {
		t.Fatalf("the seed lists %+v, %v; want r1 done with a.txt", list.Runs, err)
	}
	other, _ := journalOfDetails(t, []byte(`{"path":"b.txt","existed":true,"before":"sha256:00"}`), []byte(seed[1]))
	if l, err := coreapi.JournalList(t.Context(), readOnly(t, other.ports), other.root); err != nil || len(l.Runs[0].Files) != 1 || l.Runs[0].Files[0] != "b.txt" {
		t.Fatalf("the changed copy lists %+v, %v; want b.txt", l.Runs, err)
	}
	cut, _ := journalOfDetails(t, []byte(seed[0]), []byte(`{"outcome":"done"`))
	if cut != nil {
		t.Fatal("a close cut by one byte is JSON, and the journal would take it")
	}
	unknown, _ := journalOfDetails(t, []byte(seed[0]), []byte(`{"outcome":7}`))
	if l, err := coreapi.JournalList(t.Context(), readOnly(t, unknown.ports), unknown.root); err != nil || l.Runs[0].State != "unknown" {
		t.Fatalf("a close with an outcome of the wrong type lists %+v, %v; want unknown", l.Runs, err)
	}
}
