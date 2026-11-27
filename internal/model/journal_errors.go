package model

import "fmt"

// The two refusals of a journal's reader, here so that a caller past the
// port names each by its type: the journal adapter answers
// them from Records and RecordsOf, and the cache listing says which a
// journal holds without ending on it.

// NewerRecordError is the error of a journal that holds a record written by
// a newer engine: the run, the record's place in the run's list from 1, its
// version, and the newest this engine reads. A record of an older version is
// read as it is.
type NewerRecordError struct {
	Run   string
	Entry int
	Found int
	Reads int
}

func (e *NewerRecordError) Error() string {
	return fmt.Sprintf("record %d of run %s in the journal is version %d, and this engine reads up to version %d", e.Entry, e.Run, e.Found, e.Reads)
}

// OlderJournalError is the error of a journal that holds a line in the form
// an older gragas wrote, one record a line under its run, before the journal
// was go-run's runlog: the line, from 1, its run, and the
// journal's path when the reader knows it. This engine reads no such line, so
// a run of it is neither shown nor rolled back from it; it is refused by
// name, never read as torn.
type OlderJournalError struct {
	Line int
	Run  string
	Path string
}

func (e *OlderJournalError) Error() string {
	journal := "the journal"
	if e.Path != "" {
		journal += " " + e.Path
	}
	return fmt.Sprintf("line %d of %s is a record of run %s from an older release of gragas, which this engine does not read: recover its runs with that release (gragas journal recover), then remove the journal", e.Line, journal, e.Run)
}
