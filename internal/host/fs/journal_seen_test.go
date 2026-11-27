package fs_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-run/runlog"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
)

// heldJournal is a journal whose Lock holds its state folder, as the wiring
// makes it, over a state folder in a fresh temporary folder, and a root it
// keeps a journal of.
func heldJournal(t *testing.T) (hostfs.Journal, *hostfs.Lock, model.Root, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	tree := newTree(t)
	l := &hostfs.Lock{Dir: dir, Clock: lockInstant(), Tree: tree}
	j := hostfs.Journal{Tree: tree, Writer: hostfs.Writer{Tree: tree}, Clock: lockInstant(), Dir: dir, Lock: l}
	return j, l, openRoot(t, tree, "r"), dir
}

// olderLine is a line of an older gragas's form, which the journal refuses.
const olderLine = `{"v":1,"run":"run-0","kind":"intent","at":1,"root":"x"}` + "\n"

// behindTheLock adds line to the end of the one journal in the state folder
// dir, as a process that takes no lock would, and answers the file's name.
func behindTheLock(t *testing.T, dir, line string) string {
	t.Helper()
	name := journalFile(t, dir)
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, append(data, line...), 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}

// TestAReadUnderAHoldServesTheCloseAndGoesWithTheHold: the
// journal read under a hold of the root lock, as a run reads it before its
// intent, serves the run's intent and close, which read nothing more: a line
// of an older form planted behind the lock after the intent does not refuse
// the close. The close is the run's record finished, through the Handle its
// intent was given. Once the lock is put back, nothing read under it is used:
// the next hold reads again, and refuses the planted line. Without the read
// kept in the hold the close is refused; with it outliving the hold the
// second intent is taken.
func TestAReadUnderAHoldServesTheCloseAndGoesWithTheHold(t *testing.T) {
	t.Parallel()
	j, l, root, dir := heldJournal(t)
	ctx := t.Context()
	h, err := l.Take(ctx, root, model.RootLock)
	if err != nil {
		t.Fatal(err)
	}
	if recs, _, err := j.Records(ctx, root); err != nil || len(recs) != 0 {
		t.Fatalf("control: Records of a new root = %+v, %v, want none", recs, err)
	}
	for _, kind := range []string{model.KindIntent, model.KindCopy, model.KindStep} {
		if err := j.Append(ctx, root, model.Record{Run: "run-1", Kind: kind}); err != nil {
			t.Fatalf("append the %s: %v", kind, err)
		}
	}
	name := behindTheLock(t, dir, olderLine)
	if err := j.Append(ctx, root, model.Record{Run: "run-1", Kind: model.KindClose}); err != nil {
		t.Fatalf("the close under the hold its intent was written in = %v, want it written with no read", err)
	}
	if err := l.Release(h); err != nil {
		t.Fatal(err)
	}

	h, err = l.Take(ctx, root, model.RootLock)
	if err != nil {
		t.Fatal(err)
	}
	var oe *model.OlderJournalError
	if err := j.Append(ctx, root, model.Record{Run: "run-2", Kind: model.KindIntent}); !errors.As(err, &oe) || oe.Run != "run-0" {
		t.Errorf("an intent under the next hold = %v, want the OlderJournalError of the planted line", err)
	}
	if err := l.Release(h); err != nil {
		t.Fatal(err)
	}

	// The close was written whole: with the planted line taken out, the run
	// holds its four records and its record line is finished.
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(olderLine), nil, 1)
	runs, err := runlog.ReadFrom(ctx, bytes.NewReader(data), 0)
	if err != nil || len(runs) != 1 || runs[0].ID != "run-1" || len(runs[0].Entries) != 4 || runs[0].Interrupted() {
		t.Errorf("the journal less the planted line holds %+v, %v, want run-1 finished with its four records", runs, err)
	}
}

// TestASweepUnderTheHoldForgetsWhatTheHoldRead: Sweep, the one
// rewrite of the journal, forgets what the hold read before it, so an intent
// after it under the same hold reads again: a line of an older form planted
// after the sweep refuses it. Without the forgetting, the intent is taken on
// the read from before the sweep.
func TestASweepUnderTheHoldForgetsWhatTheHoldRead(t *testing.T) {
	t.Parallel()
	j, l, root, dir := heldJournal(t)
	ctx := t.Context()
	h, err := l.Take(ctx, root, model.RootLock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := l.Release(h); err != nil {
			t.Error(err)
		}
	}()
	if err := j.Append(ctx, root, model.Record{Run: "run-1", Kind: model.KindIntent}); err != nil {
		t.Fatal(err)
	}
	if recs, _, err := j.Records(ctx, root); err != nil || len(recs) != 1 {
		t.Fatalf("control: Records = %+v, %v, want the intent", recs, err)
	}
	if n, err := j.Sweep(ctx, root); err != nil || n != 0 {
		t.Fatalf("control: Sweep = %d, %v, want nothing swept", n, err)
	}
	behindTheLock(t, dir, olderLine)
	var oe *model.OlderJournalError
	if err := j.Append(ctx, root, model.Record{Run: "run-2", Kind: model.KindIntent}); !errors.As(err, &oe) {
		t.Errorf("an intent after the sweep = %v, want the OlderJournalError of the planted line", err)
	}
}

// TestAWriteThatFailsUnderTheHoldForgetsWhatTheHoldRead: an
// intent whose write is refused, here one over runlog's longest line, may
// leave the journal other than what was seen, so the next intent reads again
// and is refused by a line of an older form planted after the refusal. With
// what was seen kept past the refused write, that intent is taken.
func TestAWriteThatFailsUnderTheHoldForgetsWhatTheHoldRead(t *testing.T) {
	t.Parallel()
	j, l, root, dir := heldJournal(t)
	ctx := t.Context()
	h, err := l.Take(ctx, root, model.RootLock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := l.Release(h); err != nil {
			t.Error(err)
		}
	}()
	if err := j.Append(ctx, root, model.Record{Run: "run-1", Kind: model.KindIntent}); err != nil {
		t.Fatal(err)
	}
	if recs, _, err := j.Records(ctx, root); err != nil || len(recs) != 1 {
		t.Fatalf("control: Records = %+v, %v, want the intent", recs, err)
	}
	long := []byte(`{"pad":"` + strings.Repeat("x", runlog.DefaultMaxLine) + `"}`)
	if err := j.Append(ctx, root, model.Record{Run: "run-2", Kind: model.KindIntent, Detail: long}); !errors.Is(err, runlog.ErrLineTooLong) {
		t.Fatalf("control: an intent over the longest line = %v, want runlog's ErrLineTooLong", err)
	}
	behindTheLock(t, dir, olderLine)
	var oe *model.OlderJournalError
	if err := j.Append(ctx, root, model.Record{Run: "run-3", Kind: model.KindIntent}); !errors.As(err, &oe) {
		t.Errorf("an intent after the refused write = %v, want the OlderJournalError of the planted line", err)
	}
}

// TestARecordsThatFindsAnOlderLineForgetsWhatTheHoldRead: a
// Records under the hold that meets a line of an older form leaves nothing
// read before it to trust, so the next intent reads again and is refused.
func TestARecordsThatFindsAnOlderLineForgetsWhatTheHoldRead(t *testing.T) {
	t.Parallel()
	j, l, root, dir := heldJournal(t)
	ctx := t.Context()
	h, err := l.Take(ctx, root, model.RootLock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := l.Release(h); err != nil {
			t.Error(err)
		}
	}()
	if err := j.Append(ctx, root, model.Record{Run: "run-1", Kind: model.KindIntent}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := j.Records(ctx, root); err != nil {
		t.Fatalf("control: Records = %v", err)
	}
	behindTheLock(t, dir, olderLine)
	var oe *model.OlderJournalError
	if _, _, err := j.Records(ctx, root); !errors.As(err, &oe) {
		t.Fatalf("Records after the plant = %v, want the OlderJournalError", err)
	}
	if err := j.Append(ctx, root, model.Record{Run: "run-1", Kind: model.KindClose}); !errors.As(err, &oe) {
		t.Errorf("a close after the refused Records = %v, want the OlderJournalError", err)
	}
}

// TestUnderTheScratchLockAloneACloseReadsTheJournal: the
// scratch lock keeps no other gragas out of a root's journal, so its hold
// keeps nothing seen, and a close under it reads: a line of an older form
// planted after the intent refuses it. With what was seen kept under any
// hold, the close is taken.
func TestUnderTheScratchLockAloneACloseReadsTheJournal(t *testing.T) {
	t.Parallel()
	j, l, root, dir := heldJournal(t)
	ctx := t.Context()
	h, err := l.Take(ctx, root, model.ScratchLock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := l.Release(h); err != nil {
			t.Error(err)
		}
	}()
	if _, _, err := j.Records(ctx, root); err != nil {
		t.Fatalf("control: Records = %v", err)
	}
	if err := j.Append(ctx, root, model.Record{Run: "run-1", Kind: model.KindIntent}); err != nil {
		t.Fatal(err)
	}
	behindTheLock(t, dir, olderLine)
	var oe *model.OlderJournalError
	if err := j.Append(ctx, root, model.Record{Run: "run-1", Kind: model.KindClose}); !errors.As(err, &oe) {
		t.Errorf("a close under the scratch lock alone = %v, want the OlderJournalError of the planted line", err)
	}
}
