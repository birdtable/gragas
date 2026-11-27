package fs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/birdtable/go-run/runlog"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// defaultAge is how long a closed record and its copies are kept when the
// settings file names no age: thirty days.
const defaultAge = 30 * 24 * time.Hour

// runName is what a run id may be. It names a folder of copies, so it is never
// a path.
var runName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Journal is the adapter of port.Journal: the record of a run that writes, in
// the state folder, one file per root and the copies of what the run replaced
// beside it. It cannot be rebuilt: nothing else knows what a run replaced, so a
// closed record and its copies are kept for Age and then swept, and an open
// one never is. The file is go-run's runlog: one run per id, its
// record line written when the run begins and again when it closes, and each
// of gragas's records an entry line of its own under the run's id (Log.Entry,
// go-run v0.8.0), with its version and time, written in one write and flushed
// (Log.Sync) before the effect the record announces. A record costs its own
// line, never the run's list again, and a torn line costs that one record.
// Files are named by the digest of the root and of a path, never by a
// project's name.
//
// runlog writes and reads the file through the state folder's handle, the
// *os.Root the Tree holds for it (Log.Root, go-run v0.7.0), as Writer writes
// the copies: under the Lock that is the folder the lock was taken in, so a
// name changed on the way to it after the lock moves no write.
// runlog takes no lock: the root lock serialises the runs of a root, and the
// records of one run are written by that run alone. So what a read under a
// hold of the lock found is kept in the hold until Release (seen): a run reads
// its journal once, and its intent and close
// under the same hold read nothing more.
//
// One form is read. A line of an older gragas's form, one record a line
// under its run, as release 1 wrote it before runlog, is refused by name
// (model.OlderJournalError), never read as torn: a run of it that crashed
// would otherwise never be rolled back.
//
// A journal holds argv, paths and digests, never a token, a key or a
// credential, so no secret goes in a store.
type Journal struct {
	// Tree opens the state folder as a root, and reads it. Writer writes in
	// it, and is the Writer over the same Tree.
	Tree   port.Tree
	Writer port.Writer
	// Clock stamps a record.
	Clock port.Clock
	// Dir is the engine's state folder, made when it is missing, and Mounts
	// the configured mounts its walk trusts.
	Dir    string
	Mounts []string
	// Age is how long Sweep keeps a closed run; zero is thirty days.
	Age time.Duration
	// Lock, when set, is the Lock whose Tree is this Tree. While it holds
	// the root lock of a root, or its scratch lock, every read and write of
	// that root's journal and copies goes through the state folder the lock
	// was taken in, held open, and never by Dir's path again;
	// with neither held, the state folder is opened by Dir.
	Lock *Lock
	// Roots, when set, answers the folders a clone carries for this run,
	// as Lock.Roots does: Dir must lie outside each of them
	// and outside the root each call names, judged before any record or copy
	// is read or written, held or not, and it is made with them held apart.
	// A call there matches port.ErrCarried. nil judges nothing (runApart).
	Roots func() ([]string, error)

	// judged, when a test of this package sets it, is called after the
	// state folder is judged against the roots and before it is made or
	// opened by Dir, so a test can make a link at a missing name there. It is
	// never set outside a test binary.
	judged func()
}

// entry is a record as its entry line keeps it, the line's value: one JSON
// object, the same fixed fields whoever did the work, and what a tool or the
// platform adds under detail. The run is the id the entry line names.
type entry struct {
	V       int             `json:"v"`
	Kind    string          `json:"kind"`
	At      int64           `json:"at"`
	Command string          `json:"command,omitempty"`
	Plan    string          `json:"plan,omitempty"`
	Root    string          `json:"root"`
	Engine  string          `json:"engine,omitempty"`
	Detail  json.RawMessage `json:"detail,omitempty"`
}

// older is the error of the first line of data in an older gragas's form, a
// JSON object with a version, a run and a kind and no runlog id, or nil. A
// line with an id is runlog's, an entry line keeps its fields under value,
// and a line torn short is no object, so none is taken for one.
func older(data []byte) error {
	for i, line := range bytes.Split(data, []byte("\n")) {
		var old struct {
			ID   *json.RawMessage `json:"id"`
			V    int              `json:"v"`
			Run  string           `json:"run"`
			Kind string           `json:"kind"`
		}
		if json.Unmarshal(line, &old) == nil && old.ID == nil && old.V >= 1 && runName.MatchString(old.Run) && knownKind(old.Kind) {
			return &model.OlderJournalError{Line: i + 1, Run: old.Run}
		}
	}
	return nil
}

// olderIn is the OlderJournalError of root's journal in state, or nil. runlog skips
// a line with no id, and counts it in skipped, so the bytes are read again
// only when it skipped one: a journal with no line skipped is read once.
func (j Journal) olderIn(state, root model.Root, skipped int) error {
	if skipped == 0 {
		return nil
	}
	data, err := j.Tree.Read(state, journalName(root))
	if err != nil {
		return fmt.Errorf("the journal could not be read: %w", err)
	}
	err = older(data)
	if oe, ok := errors.AsType[*model.OlderJournalError](err); ok {
		// The path is for a person to read: the state folder's by Dir, as
		// KeptAt answers it.
		if dir, derr := j.byPath(root); derr == nil {
			oe.Path = filepath.Join(string(dir), filepath.FromSlash(journalName(root)))
		}
	}
	return err
}

// NewRunID answers a run id: sixteen bytes from the cryptographic generator,
// in hex. A run id is not guessable, and a repository test fails on math/rand
// in this package.
func NewRunID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("make a run id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// digestOf is the hex SHA-256 of s, which names a file in the state folder in
// place of a root or a path.
func digestOf(s string) string {
	return digest.Hex([]byte(s))
}

// Append writes r as an entry line of its run in the journal of root, with
// its version, the time and the root's digest set, before the caller makes
// the effect it announces (Log.Entry, go-run v0.8.0): one line, whatever the
// run already holds. A copy or a step comes after its run's intent, written
// by the same run, so nothing of the journal is read for it. An intent or a
// close reads the journal once at most, in the one pass find makes, and not
// at all under a hold that has already read it: a run it does not hold yet
// is begun, its runlog record named by r.Run, after the entry line, which
// runlog folds into its run wherever it falls, so an entry the library
// refuses (a line over MaxLine) leaves the journal as it was. A close also
// finishes the runlog record, through the Handle its intent was given under
// the same hold when there is one; a record after a close is written as any
// other, and the last close wins. A record that is no record, a cancelled
// ctx, a journal with a line of an older form met by the first write of a
// run (a model.OlderJournalError, as Records answers), or a write that
// fails, is an error, and then the caller makes no effect. A journal another
// process holds past journalHold is a write that fails.
func (j Journal) Append(ctx context.Context, root model.Root, r model.Record) error {
	// The callers hand a ctx no interrupt ends, so a record of an effect is
	// not cut short; go-files' hold on the journal waits until ctx ends, so
	// the writes get a deadline of their own.
	ctx, cancel := context.WithTimeout(ctx, journalHold)
	defer cancel()
	one, err := j.line(root, r)
	if err != nil {
		return err
	}
	// A record is refused on a cancelled ctx even when nothing is read for
	// it: go-files' append tries its hold once whatever ctx.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("journal append: %w", err)
	}
	state, s, err := j.stateOf(root)
	if err != nil {
		return fmt.Errorf("journal append: %w", err)
	}
	log, err := j.log(state, root, r.Run, nil)
	if err != nil {
		return fmt.Errorf("journal append: %w", err)
	}
	if r.Kind == model.KindCopy || r.Kind == model.KindStep || r.Kind == model.KindStart {
		if err := log.Entry(ctx, r.Run, json.RawMessage(one)); err != nil {
			return fmt.Errorf("journal append: the record could not be written: %w", err)
		}
		return nil
	}
	if s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	// A record added to a journal Records refuses would be lost with the
	// journal when the refusal's fix removes it, so an intent and a close
	// meet the refusal. A copy or a step reads
	// nothing; it follows its run's intent, which met it.
	held, h, err := j.find(ctx, state, root, r, s)
	if err == nil {
		err = log.Entry(ctx, r.Run, json.RawMessage(one))
		if err == nil {
			h, err = mark(ctx, log, r, held, h)
		}
		if err != nil {
			err = fmt.Errorf("the record could not be written: %w", err)
		}
	}
	if err != nil {
		// What was seen may no longer be what the journal holds, and a
		// Handle keeps the first write that failed, so the next append reads
		// again.
		s.forget()
		return fmt.Errorf("journal append: %w", err)
	}
	s.know(r.Run, h)
	return nil
}

// find is whether r's run is in the journal of root in state, and the Handle
// of its runlog record when one is known or a close needs one, after the
// older-form check: from what s has seen under the hold when that settles
// it, and otherwise from one read, runlog's Resume with Skipped counted, so
// the held check and the older-form check come from one pass. A line
// skipped is the one case read again, for the older form, as olderIn says.
// The caller holds s.mu when s is not nil.
func (j Journal) find(ctx context.Context, state, root model.Root, r model.Record, s *seen) (bool, *runlog.Handle, error) {
	if s != nil && s.clean {
		h, held := s.runs[r.Run]
		switch {
		case held && (h != nil || r.Kind != model.KindClose):
			return true, h, nil
		case !held && s.all:
			return false, nil, nil
		}
	}
	var skipped int
	log, err := j.log(state, root, r.Run, &skipped)
	if err != nil {
		return false, nil, err
	}
	h, err := log.Resume(ctx, r.Run)
	held := err == nil
	if errors.Is(err, runlog.ErrNotFound) {
		err = nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("the journal could not be read: %w", err)
	}
	if err := j.olderIn(state, root, skipped); err != nil {
		return false, nil, err
	}
	if s != nil {
		s.clean = true
	}
	return held, h, nil
}

// seen is what this process has read and written of a root's journal under
// one hold of the root's lock, kept in the Lock's holding and gone with it at
// Release. While the hold is held no other gragas takes the
// lock, so nothing but this process writes the record line of a run, and the
// one rewrite, Sweep, takes the lock and forgets what was seen; a lockless
// writer elsewhere appends whole lines in the current form under run ids of
// its own. So what a read under the hold found stays true, each of this
// process's own writes keeps it true, and an intent or a close needs no read
// of its own. Once the lock is put back, another process may write, and
// nothing here is used again. The lock is advisory (internal/apply, the
// residual): a process that takes none could add a line of an older form,
// which no release wrote.
type seen struct {
	mu sync.Mutex
	// clean says a whole read under this hold found no line of an older form.
	clean bool
	// all says runs holds every run of the journal, as Records read them,
	// and not only the runs this process began, resumed or found.
	all bool
	// runs is the runs the journal holds that this process knows of under
	// the hold, each with the Handle Begin or Resume gave it here, or nil
	// when a read found it.
	runs map[string]*runlog.Handle
}

// forget drops everything s has seen; a nil s has seen nothing.
func (s *seen) forget() {
	if s != nil {
		s.clean, s.all, s.runs = false, false, nil
	}
}

// know records that the journal holds run, with h its Handle when there is
// one; a Handle known already stays when h is nil. A nil s records nothing.
func (s *seen) know(run string, h *runlog.Handle) {
	if s == nil {
		return
	}
	if s.runs == nil {
		s.runs = map[string]*runlog.Handle{}
	}
	if h != nil || s.runs[run] == nil {
		s.runs[run] = h
	}
}

// line is r as the entry line Append writes for root: its version, the time
// from the Clock and the root's digest set, its detail compacted. A record
// that is no record (a run not of runName, a kind not known, a detail not one
// JSON value) is an error.
func (j Journal) line(root model.Root, r model.Record) ([]byte, error) {
	if !runName.MatchString(r.Run) || !knownKind(r.Kind) {
		return nil, fmt.Errorf("journal append: a record needs a run of 1 to 64 letters, digits, dashes or underscores and a kind of intent, copy, step, start or close, got %q and %q", r.Run, r.Kind)
	}
	if len(r.Detail) > 0 && !json.Valid(r.Detail) {
		return nil, errors.New("journal append: the detail of a record is one JSON value")
	}
	e := entry{
		V: model.RecordVersion, Kind: r.Kind, At: j.Clock.Now().Unix(), Command: r.Command,
		Plan: r.Plan, Root: digestOf(string(root)), Engine: r.Engine,
	}
	if len(r.Detail) > 0 {
		var compact bytes.Buffer
		if err := json.Compact(&compact, r.Detail); err != nil {
			return nil, fmt.Errorf("journal append: %w", err)
		}
		e.Detail = compact.Bytes()
	}
	one, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("journal append: %w", err)
	}
	return one, nil
}

// mark is the runlog record of an intent or a close r, after its entry line
// is written, and the run's Handle: a run the log does not hold yet (held
// false) is begun, and a close finishes the run's record through h, the
// Handle find answered, or one Resume gives when it answered none. An entry
// line is not part of the record line, so a Handle given before it is the
// record as it stands.
func mark(ctx context.Context, log runlog.Log, r model.Record, held bool, h *runlog.Handle) (*runlog.Handle, error) {
	var err error
	switch {
	case !held:
		h, err = log.Begin(nil, nil)
	case r.Kind == model.KindClose && h == nil:
		h, err = log.Resume(ctx, r.Run)
	}
	if err == nil && r.Kind == model.KindClose {
		err = h.Finish(0, "", "")
	}
	return h, err
}

// log is the runlog of root's journal in state, the state folder j.state
// answered, which begins a run under the id run and counts the lines a read
// skips in skipped when it is not nil. Log.Root is the folder this journal's
// Tree holds open for state (go-run v0.7.0): every open of the
// journal, to write, read or resume, goes through it, so a write under the
// Lock goes to the folder the lock was taken in and never by Dir's path,
// and runlog makes no folder. Perm is left at runlog's own,
// 0600, since a set one is refused on Windows; the state folder is made
// private before any write. runlog appends through go-files' write.Append,
// which ends a torn last line before the new one in the same write, so a run
// begun after a crash keeps its first line.
func (j Journal) log(state, root model.Root, run string, skipped *int) (runlog.Log, error) {
	tree, ok := j.Tree.(*Tree)
	if !ok {
		return runlog.Log{}, errors.New("the journal's Tree is not this package's, so its state folder cannot be held")
	}
	dir, err := tree.folder(state)
	if err != nil {
		return runlog.Log{}, err
	}
	return runlog.Log{
		Root: dir, Path: journalName(root), Sync: true, Now: j.Clock.Now,
		NewID: func() string { return run }, Skipped: skipped,
	}, nil
}

// runs is every run of the journal of root in state, as runlog reads them
// through the folder held, and how many lines it skipped as torn. A root
// with no journal is no runs.
func (j Journal) runs(ctx context.Context, state, root model.Root) ([]runlog.Record, int, error) {
	var skipped int
	log, err := j.log(state, root, "", &skipped)
	if err != nil {
		return nil, 0, err
	}
	runs, err := log.Read(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("the journal could not be read: %w", err)
	}
	return runs, skipped, nil
}

// Keep stores old, the content of the file at path before the run replaces
// it, under the run, in a file named by the digest of the path. A second copy
// for the same run and path is refused, so a retry never stores the replaced
// bytes as the original. A copy that cannot be stored is an error, and then the
// caller does not replace the file.
func (j Journal) Keep(root model.Root, run, path string, old []byte) error {
	if !runName.MatchString(run) {
		return fmt.Errorf("journal keep: a run is 1 to 64 letters, digits, dashes or underscores, got %q", run)
	}
	state, err := j.state(root)
	if err != nil {
		return fmt.Errorf("journal keep: %w", err)
	}
	switch _, err := j.Tree.Read(state, copyName(root, run, path)); {
	case err == nil:
		return fmt.Errorf("journal keep: a copy of %s is already kept under run %s, and a second would store the replaced bytes as the original", path, run)
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("journal keep %s: %w", path, err)
	}
	if err := j.Writer.Replace(state, copyName(root, run, path), old); err != nil {
		return fmt.Errorf("journal keep %s: the copy could not be stored: %w", path, err)
	}
	return nil
}

// Kept answers the copy Keep stored for path under run. A copy nobody kept
// matches fs.ErrNotExist.
func (j Journal) Kept(root model.Root, run, path string) ([]byte, error) {
	if !runName.MatchString(run) {
		return nil, fmt.Errorf("journal kept: a run is 1 to 64 letters, digits, dashes or underscores, got %q", run)
	}
	state, err := j.state(root)
	if err != nil {
		return nil, fmt.Errorf("journal kept: %w", err)
	}
	data, err := j.Tree.Read(state, copyName(root, run, path))
	if err != nil {
		return nil, fmt.Errorf("journal kept %s: %w", path, err)
	}
	return data, nil
}

// KeptAt answers the absolute path of the file Kept reads, in the state
// folder, when the copy is there. A copy nobody kept matches
// fs.ErrNotExist.
func (j Journal) KeptAt(root model.Root, run, path string) (string, error) {
	if !runName.MatchString(run) {
		return "", fmt.Errorf("journal kept at: a run is 1 to 64 letters, digits, dashes or underscores, got %q", run)
	}
	state, err := j.state(root)
	if err != nil {
		return "", fmt.Errorf("journal kept at: %w", err)
	}
	if _, err := j.Tree.Stat(state, copyName(root, run, path)); err != nil {
		return "", fmt.Errorf("journal kept at %s: %w", path, err)
	}
	// The answer is a path, for a person to read: the state folder's by
	// Dir, as Tree.Open names it, never the name a held folder has among
	// the roots.
	dir, err := j.byPath(root)
	if err != nil {
		return "", fmt.Errorf("journal kept at: %w", err)
	}
	return filepath.Join(string(dir), filepath.FromSlash(copyName(root, run, path))), nil
}

// Records answers every record of the journal of root: the runs in the order
// they began, and each run's records in the order they were written. It also
// answers how many were torn and skipped: a line runlog skips (an entry line
// of a run the journal has no record line for among them), the entries of a
// run whose id is no run's name, and an entry that is no JSON object, has no
// version, or is no record. A record of a newer version is an error, a
// model.NewerRecordError, and no records.
//
// Under a hold of the root's lock, what it read is seen (seen): every run the
// journal holds, and that no line is of an older form, so an intent or a
// close under the same hold reads nothing.
func (j Journal) Records(ctx context.Context, root model.Root) ([]model.Record, int, error) {
	state, s, err := j.stateOf(root)
	if err != nil {
		return nil, 0, fmt.Errorf("journal records: %w", err)
	}
	if s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	runs, torn, err := j.runs(ctx, state, root)
	if err == nil {
		err = j.olderIn(state, root, torn)
	}
	if err != nil {
		// A read that fails, or finds a line of an older form, leaves
		// nothing seen to trust.
		s.forget()
		return nil, 0, fmt.Errorf("journal records: %w", err)
	}
	if s != nil {
		s.clean, s.all = true, true
		for _, run := range runs {
			s.know(run.ID, nil)
		}
	}
	return records(runs, torn)
}

// RecordsOf answers what Records answers for a root whose journal file holds
// data: the same reader, runlog's, over the bytes the caller read, so it
// opens nothing. A line of an older form is looked for only when a line was
// skipped, as Records looks, and its error names no path, which the caller
// knows. runlog's ReadFrom counts no skipped line, so the lines are counted
// here by runlog's own judgement of each alone: a line that reads as a run
// alone is a record line, and any other is an entry line runlog folded into
// a run or one it skipped.
//
// Its state folder is judged first, as every other call judges it, with no
// root of the call's own: bytes a caller read from a state
// folder a clone carries are refused, matching port.ErrCarried, whatever they
// hold, and so is no data, so a caller asks with none before it reads a file.
func (j Journal) RecordsOf(ctx context.Context, data []byte) ([]model.Record, int, error) {
	if j.Dir != "" {
		if _, err := runApart(j.Dir, j.Roots); err != nil {
			return nil, 0, fmt.Errorf("journal records: the journal's state folder %s: %w", j.Dir, err)
		}
	}
	runs, err := runlog.ReadFrom(ctx, bytes.NewReader(data), 0)
	if err != nil {
		return nil, 0, fmt.Errorf("journal records: the journal could not be read: %w", err)
	}
	skipped, err := skippedOf(ctx, data, runs)
	if err != nil {
		return nil, 0, fmt.Errorf("journal records: the journal could not be read: %w", err)
	}
	if skipped > 0 {
		if err := older(data); err != nil {
			return nil, 0, fmt.Errorf("journal records: %w", err)
		}
	}
	return records(runs, skipped)
}

// skippedOf is how many lines of data runlog skipped when it read runs from
// it: every line that is not blank, less the record lines, each of which
// reads as a run alone, and the entry lines runs hold. A line is split and
// trimmed as runlog splits it.
func skippedOf(ctx context.Context, data []byte, runs []runlog.Record) (int, error) {
	lines := 0
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if len(line) == 0 {
			continue
		}
		alone, err := runlog.ReadFrom(ctx, bytes.NewReader(line), 0)
		if err != nil {
			return 0, err
		}
		if len(alone) == 0 {
			lines++
		}
	}
	for _, run := range runs {
		lines -= len(run.Entries)
	}
	return lines, nil
}

// records is the records of runs, after torn already skipped, the torn ones
// counted, or the error of the first record of a newer version. Each run's
// records are its entry lines, in the order of the file.
func records(runs []runlog.Record, torn int) ([]model.Record, int, error) {
	var out []model.Record
	for _, run := range runs {
		if !runName.MatchString(run.ID) {
			torn += len(run.Entries)
			continue
		}
		for i, line := range run.Entries {
			raw := line.Value
			var head struct {
				V int `json:"v"`
			}
			var e entry
			switch {
			case json.Unmarshal(raw, &head) != nil || head.V < 1:
				torn++
			case head.V > model.RecordVersion:
				return nil, 0, &model.NewerRecordError{Run: run.ID, Entry: i + 1, Found: head.V, Reads: model.RecordVersion}
			case json.Unmarshal(raw, &e) == nil && knownKind(e.Kind):
				out = append(out, model.Record{
					Version: e.V, Run: run.ID, Kind: e.Kind, At: e.At, Command: e.Command, Plan: e.Plan,
					Root: e.Root, Engine: e.Engine, Detail: slices.Clone([]byte(e.Detail)),
				})
			default:
				torn++
			}
		}
	}
	return out, torn, nil
}

// Sweep removes every run of root that was closed longer ago than Age: its
// record from the journal and its copies. A run with no close is never swept,
// whatever its age, because it may need rolling back. It answers how many runs
// it removed. The caller holds the root lock, so no run is writing. runlog's
// Lines folds each run kept to its one last line, and Writer.Replace puts the
// journal in place whole. The copies go first: a sweep stopped between the two
// leaves a closed run whose copies are gone, which the next sweep removes, and
// never copies nothing names. A link at any component of a copy's path refuses
// the sweep before a copy is removed and before the journal is rewritten.
// The journal is read, the copies removed and the journal
// rewritten in the one state folder, the one the Lock holds when it holds it.
// What the hold has seen of the journal is forgotten first, so nothing after
// the rewrite trusts a read from before it.
func (j Journal) Sweep(ctx context.Context, root model.Root) (int, error) {
	state, s, err := j.stateOf(root)
	if err != nil {
		return 0, fmt.Errorf("journal sweep: %w", err)
	}
	if s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.forget()
	}
	runs, skipped, err := j.runs(ctx, state, root)
	if err != nil {
		return 0, fmt.Errorf("journal sweep: %w", err)
	}
	// A line runlog skipped is not written back by Lines, so a line of an
	// older form is refused here, before the rewrite would drop it.
	if err := j.olderIn(state, root, skipped); err != nil {
		return 0, fmt.Errorf("journal sweep: %w", err)
	}
	if _, _, err := records(runs, 0); err != nil {
		return 0, fmt.Errorf("journal sweep: %w", err)
	}
	age := j.Age
	if age <= 0 {
		age = defaultAge
	}
	cutoff := j.Clock.Now().Add(-age).Unix()
	lines, dropped, err := runlog.Lines(runs, func(run runlog.Record) bool { return !closedBy(run, cutoff) })
	if err != nil {
		return 0, fmt.Errorf("journal sweep: %w", err)
	}
	if len(dropped) == 0 {
		return 0, nil
	}
	old := make([]string, 0, len(dropped))
	for _, run := range dropped {
		old = append(old, run.ID)
	}
	if err := j.removeCopies(state, root, old); err != nil {
		return 0, fmt.Errorf("journal sweep: %w", err)
	}
	if err := j.Writer.Replace(state, journalName(root), lines); err != nil {
		return 0, fmt.Errorf("journal sweep: the journal could not be rewritten: %w", err)
	}
	return len(old), nil
}

// closedBy is whether run is a run a sweep removes: one whose id is a run's
// name, since it names a folder of copies, with a close at or before cutoff.
func closedBy(run runlog.Record, cutoff int64) bool {
	recs, _, err := records([]runlog.Record{run}, 0)
	return err == nil && slices.ContainsFunc(recs, func(r model.Record) bool {
		return r.Kind == model.KindClose && r.At <= cutoff
	})
}

// removeCopies removes the folder of each run's copies. A run with none is not
// an error. Every run's folder is walked first, as Writer walks a path:
// a link or a junction at any component, the folder or a copy
// in it, or a copy that is not a file, refuses the sweep before anything is
// removed. Each copy then goes through Writer.Remove, which walks it again,
// and the emptied folder last.
func (j Journal) removeCopies(state, root model.Root, runs []string) error {
	tree, ok := j.Tree.(*Tree)
	if !ok {
		return errors.New("the journal's Tree is not this package's, so its state folder cannot be walked")
	}
	s, _, err := tree.at(state, copiesName(root))
	if err != nil {
		return err
	}
	found := make([][]string, len(runs))
	for i, run := range runs {
		if found[i], err = j.copiesOf(s, state, root, run); err != nil {
			return err
		}
	}
	for i, run := range runs {
		for _, p := range found[i] {
			if err := j.Writer.Remove(state, p); err != nil {
				return err
			}
		}
		name := filepath.FromSlash(copiesName(root) + "/" + run)
		if err := s.root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return s.fail("journal sweep", copiesName(root)+"/"+run, name, err)
		}
	}
	return nil
}

// copiesOf is the slash path, in the state folder, of each copy kept under
// run, after a walk of each that refuses a link at any component and
// anything that is not a file. A run with no folder has none.
func (j Journal) copiesOf(s *store, state, root model.Root, run string) ([]string, error) {
	dir := copiesName(root) + "/" + run
	info, err := s.walk("journal sweep", dir, filepath.FromSlash(dir))
	switch {
	case err != nil:
		return nil, err
	case info == nil:
		return nil, nil
	case !info.IsDir():
		return nil, fmt.Errorf("journal sweep %s: it is a %s, not the folder of a run's copies", dir, kindOf(info.Mode()))
	}
	entries, err := j.Tree.List(state, dir, model.Every)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		got, err := s.walk("journal sweep", e.Path, filepath.FromSlash(e.Path))
		if err != nil {
			return nil, err
		}
		if got == nil || !got.Mode().IsRegular() {
			return nil, fmt.Errorf("journal sweep %s: it is no copy Keep stored, and the sweep removes only those", e.Path)
		}
		out = append(out, e.Path)
	}
	return out, nil
}

// state makes sure root is one this journal's Tree opened, and answers the
// state folder as a root of it: the one Lock holds for root when it holds one,
// and otherwise the folder opened by Dir.
func (j Journal) state(root model.Root) (model.Root, error) {
	state, _, err := j.stateOf(root)
	return state, err
}

// stateOf is state, and what this process has seen of root's journal under
// the hold whose folder it answers; nil when no hold answers it, and then
// nothing seen is used.
func (j Journal) stateOf(root model.Root) (model.Root, *seen, error) {
	if _, err := j.Tree.Stat(root, ""); err != nil {
		return "", nil, fmt.Errorf("the root is not one this journal keeps: %w", err)
	}
	apart, err := j.apart(root)
	if err != nil {
		return "", nil, err
	}
	for _, which := range []model.Lock{model.RootLock, model.ScratchLock} {
		if held, s, ok := j.Lock.folder(j.Tree, root, which); ok {
			return held, s, nil
		}
	}
	state, err := j.open(apart)
	return state, nil, err
}

// apart judges Dir against the folders a clone carries for a call on root
// (runApart), and answers them for the make to hold apart.
func (j Journal) apart(root model.Root) ([]string, error) {
	if j.Dir == "" {
		return nil, errors.New("the journal has no state folder")
	}
	apart, err := runApart(j.Dir, j.Roots, string(root))
	if err != nil {
		return nil, fmt.Errorf("the journal's state folder %s: %w", j.Dir, err)
	}
	return apart, nil
}

// byPath opens the state folder for a call on root by Dir, judged as state
// judges it.
func (j Journal) byPath(root model.Root) (model.Root, error) {
	apart, err := j.apart(root)
	if err != nil {
		return "", err
	}
	return j.open(apart)
}

// open opens the state folder as a root of this journal's Tree by Dir, made
// when it is missing with the folders of apart held apart.
func (j Journal) open(apart []string) (model.Root, error) {
	if j.judged != nil {
		j.judged()
	}
	if err := privateDirApart(j.Dir, j.Mounts, apart); err != nil {
		return "", fmt.Errorf("the journal's state folder %s: %w", j.Dir, err)
	}
	state, err := j.Tree.Open(j.Dir)
	if err != nil {
		return "", fmt.Errorf("the state folder could not be opened: %w", err)
	}
	return state, nil
}

// knownKind is whether kind is one a record has.
func knownKind(kind string) bool { return model.KnownKind(kind) }

// journalHold is how long an Append waits for another process to let go of
// the journal: runlog's own hold on Begin and Finish (defaultHold, go-run
// v0.8.0, unexported), so the entry line and the run's record share one bound.
const journalHold = 10 * time.Second

// journalName is the file of root's journal in the state folder.
func journalName(root model.Root) string { return digestOf(string(root)) + ".journal" }

// copiesName is the folder of root's copies, beside its journal.
func copiesName(root model.Root) string { return digestOf(string(root)) + ".copies" }

// copyName is the file of the copy of path kept under run.
func copyName(root model.Root, run, path string) string {
	return copiesName(root) + "/" + run + "/" + digestOf(path)
}
