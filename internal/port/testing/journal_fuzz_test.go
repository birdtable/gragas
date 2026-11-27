package porttest_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// FuzzTheWorldsJournalReader: the world's reader of a journal's bytes
// (journalPort.RecordsOf) takes any bytes without a panic.
// It answers records or the newer version's error and nothing else; a
// record it answers is of a version it reads, a run name and one of the
// four kinds; no line is answered twice, so the records and the torn lines
// are at most the lines; and a record it answers reads back as itself from
// its own line. The seeds are the form JournalLine writes, a torn line, a
// newer record, a record of no kind and of a bad run.
func FuzzTheWorldsJournalReader(f *testing.F) {
	for _, seed := range []string{
		"",
		porttest.JournalLine(model.Record{Version: 1, Run: "run-1", Kind: model.KindIntent, At: 1}) + "\n",
		porttest.JournalLine(model.Record{Version: 1, Run: "run-1", Kind: model.KindClose, At: 2, Detail: []byte(`{"a":1}`)}) + "\n" + `{"Run":` + "\n",
		porttest.JournalLine(model.Record{Version: 9, Run: "run-1", Kind: model.KindStep}),
		`{"Version":1,"Run":"run-1","Kind":"later"}`,
		`{"Version":1,"Run":"../x","Kind":"intent"}`,
		"\r\n\n  \n{",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		j := porttest.NewWorld(t).Ports().Journal
		recs, torn, err := j.RecordsOf(t.Context(), data)
		if err != nil {
			newer, ok := errors.AsType[*model.NewerRecordError](err)
			if !ok || newer.Found <= model.RecordVersion || recs != nil || torn != 0 {
				t.Fatalf("RecordsOf = %v, %d, %v: want only the newer version's error, and nothing with it", recs, torn, err)
			}
			return
		}
		lines := len(bytes.FieldsFunc(data, func(r rune) bool { return r == '\n' }))
		if torn < 0 || len(recs)+torn > lines {
			t.Fatalf("%d records and %d torn of %d lines", len(recs), torn, lines)
		}
		for _, r := range recs {
			if r.Version < 1 || r.Version > model.RecordVersion || r.Run == "" {
				t.Fatalf("record %+v: want a version it reads and a run", r)
			}
			again, n, err := j.RecordsOf(t.Context(), []byte(porttest.JournalLine(r)))
			if err != nil || n != 0 || len(again) != 1 || porttest.JournalLine(again[0]) != porttest.JournalLine(r) {
				t.Fatalf("record %+v reads back as %v, %d, %v", r, again, n, err)
			}
		}
	})
}
