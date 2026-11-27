package porttest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing/fstest"
	"time"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// Write is one change the Writer took.
type Write struct {
	// Op is "replace", "append" or "remove".
	Op   string
	Root model.Root
	Path string
	// Data is the bytes given: the whole file for a replace, the line for an
	// append, nothing for a remove.
	Data []byte
}

// WriteFails makes every later write of path, "" for any path, answer err,
// leaving the files as they were.
func (w *World) WriteFails(path string, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writeErr[path] = err
}

type treePort struct{ w *World }

// ownedPort is the Owned port over the same world: the files SeedOutside put
// outside every root and the refusals SeedRefused planted.
type ownedPort struct{ w *World }

func (x treePort) Open(dir string) (model.Root, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	r := model.Root(path.Clean(dir))
	if _, err := x.w.rootOf("Tree.Open", r); err != nil {
		return "", err
	}
	return r, nil
}

func (x treePort) Locate(start, marker string) (model.Root, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	for d := path.Clean(start); ; d = path.Dir(d) {
		if t := x.w.roots[model.Root(d)]; t != nil {
			if _, ok := t.files[marker]; ok {
				return model.Root(d), nil
			}
		}
		if d == "/" || d == "." {
			return "", notExist("locate", marker)
		}
	}
}

func (x treePort) Read(root model.Root, p string) ([]byte, error) {
	return x.read("Tree.Read", root, p, false)
}

// ReadSoleWithin is Read for a file with the one name p, a file given a second
// name by World.Name matching port.ErrManyNames, of at most max bytes: a file with more
// matches pluginbundle.ErrOverBound and answers no bytes, as host/fs answers. A max
// outside [0, MaxInt64) is refused as host/fs refuses it.
func (x treePort) ReadSoleWithin(root model.Root, p string, max int64) ([]byte, error) {
	if max < 0 || max == math.MaxInt64 {
		return nil, fmt.Errorf("read %s: the bound %d is outside [0, %d)", p, max, int64(math.MaxInt64))
	}
	data, err := x.read("Tree.ReadSoleWithin", root, p, true)
	if err == nil && int64(len(data)) > max {
		return nil, fmt.Errorf("read %s: it is over the %d bytes allowed: %w", p, max, pluginbundle.ErrOverBound)
	}
	return data, err
}

// read is Read, called as call, and ReadSoleWithin when sole is set.
func (x treePort) read(call string, root model.Root, p string, sole bool) ([]byte, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf(call, root)
	if err != nil {
		return nil, err
	}
	if err := inside(p); err != nil {
		return nil, err
	}
	if err := t.linked("read", p); err != nil {
		return nil, err
	}
	if c, ok := t.files[p]; ok {
		if n := len(t.names[p]); sole && n > 1 {
			return nil, fmt.Errorf("read %s: %w: %d names", p, port.ErrManyNames, n)
		}
		return []byte(c), nil
	}
	if t.isDir(p) {
		return nil, fmt.Errorf("read %s: is a folder", p)
	}
	return nil, notExist("read", p)
}

func (x treePort) Stat(root model.Root, p string) (model.Entry, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf("Tree.Stat", root)
	if err != nil {
		return model.Entry{}, err
	}
	if err := inside(p); err != nil {
		return model.Entry{}, err
	}
	if c, ok := t.files[p]; ok {
		return model.Entry{Path: p, Size: int64(len(c)), Exec: t.exec[p]}, nil
	}
	if t.isDir(p) {
		return model.Entry{Path: p, Dir: true}, nil
	}
	return model.Entry{}, notExist("stat", p)
}

func (x treePort) List(root model.Root, dir string, scope model.Scope) ([]model.Entry, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf("Tree.List", root)
	if err != nil {
		return nil, err
	}
	if err := inside(dir); err != nil {
		return nil, err
	}
	if scope != model.Every && t.tracked == nil {
		return nil, x.w.unseeded("Tree.List", "the files of %q were not said to be tracked or not; call Track", root)
	}
	prefix := ""
	if dir != "" && dir != "." {
		prefix = dir + "/"
	}
	found := map[string]model.Entry{}
	for p, c := range t.files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		if scope == model.Tracked && !t.tracked[p] || scope == model.Untracked && t.tracked[p] {
			continue
		}
		found[p] = model.Entry{Path: p, Size: int64(len(c))}
		for d := path.Dir(p); d != "." && len(d)+1 > len(prefix); d = path.Dir(d) {
			found[d] = model.Entry{Path: d, Dir: true}
		}
	}
	out := make([]model.Entry, 0, len(found))
	for _, e := range found {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// ReadOwned reads what SeedOutside put at the absolute path, unless
// SeedRefused refused it, which wins.
func (x ownedPort) ReadOwned(p string) ([]byte, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if !absolute(p) {
		return nil, fmt.Errorf("the path %q is not absolute", p)
	}
	if r, ok := x.w.refused[p]; ok {
		return nil, &refusal{path: p, r: r}
	}
	if c, ok := x.w.outside[p]; ok {
		return []byte(c), nil
	}
	return nil, notExist("read owned", p)
}

// refusal is ReadOwned's planted refusal: it matches port.ErrNotOwned and
// names the path, as the adapter's does.
type refusal struct {
	path string
	r    Refused
}

func (e *refusal) Error() string { return e.path + " " + e.r.Why }

func (e *refusal) Is(target error) bool {
	return target == port.ErrNotOwned || (e.r.Reason != nil && target == e.r.Reason)
}

// Refusal is why and the fix, as trust.Owned answers them.
func (e *refusal) Refusal() (why, fix string) { return e.r.Why, e.r.Fix }

type writerPort struct{ w *World }

// failing is the error a write of p was made to answer, or nil.
func (w *World) failing(p string) error {
	if err, ok := w.writeErr[p]; ok {
		return err
	}
	return w.writeErr[""]
}

func (x writerPort) Replace(root model.Root, p string, data []byte) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf("Writer.Replace", root)
	if err != nil {
		return err
	}
	if err := inside(p); err != nil || p == "" {
		return fmt.Errorf("replace %q: %w", p, firstErr(err, fmt.Errorf("a file is named by a path")))
	}
	if err := t.linked("replace", p); err != nil {
		return err
	}
	if err := x.w.failing(p); err != nil {
		return err
	}
	t.unname(p)
	t.files[p] = string(data)
	x.w.Writes = append(x.w.Writes, Write{Op: "replace", Root: root, Path: p, Data: slices.Clone(data)})
	return nil
}

func (x writerPort) Append(root model.Root, p string, line []byte) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf("Writer.Append", root)
	if err != nil {
		return err
	}
	if err := inside(p); err != nil || p == "" {
		return fmt.Errorf("append %q: %w", p, firstErr(err, fmt.Errorf("a file is named by a path")))
	}
	if s := string(line); strings.HasPrefix(s, "\n") || strings.HasSuffix(s, "\n") || strings.Contains(s, "\n\n") {
		return fmt.Errorf("append %q: the lines hold a newline at an end or an empty line between two", p)
	}
	if err := t.linked("append", p); err != nil {
		return err
	}
	if len(t.names[p]) > 1 {
		return fmt.Errorf("append %s: %w", p, &fs.PathError{Op: "open", Path: p,
			Err: fmt.Errorf("it has a second name, a hard link: %w", port.ErrOutside)})
	}
	if err := x.w.failing(p); err != nil {
		return err
	}
	old := t.files[p]
	if old != "" && !strings.HasSuffix(old, "\n") {
		old += "\n" // a torn last line is ended first
	}
	t.files[p] = old + string(line) + "\n"
	x.w.Writes = append(x.w.Writes, Write{Op: "append", Root: root, Path: p, Data: slices.Clone(line)})
	return nil
}

// Remove takes the file at p away. The fake holds no links, so a folder, a
// path some file lies below, is the one thing it refuses.
func (x writerPort) Remove(root model.Root, p string) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf("Writer.Remove", root)
	if err != nil {
		return err
	}
	if err := inside(p); err != nil || p == "" {
		return fmt.Errorf("remove %q: %w", p, firstErr(err, fmt.Errorf("a file is named by a path")))
	}
	if err := t.linked("remove", p); err != nil {
		return err
	}
	if t.isDir(p) {
		return fmt.Errorf("remove %q: it is a folder, and only a file is removed", p)
	}
	if err := x.w.failing(p); err != nil {
		return err
	}
	if _, ok := t.files[p]; !ok {
		return nil
	}
	delete(t.files, p)
	delete(t.exec, p)
	t.unname(p)
	x.w.Writes = append(x.w.Writes, Write{Op: "remove", Root: root, Path: p})
	return nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// lockKey names one lock of one root.
type lockKey struct {
	root  model.Root
	which model.Lock
}

// HoldLock makes the lock which of root held by another process since the
// given time, in seconds since the Unix epoch.
func (w *World) HoldLock(root model.Root, which model.Lock, since int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.locks[lockKey{root, which}] = since
}

// FreeLock ends the hold of another process that HoldLock made, and wakes the
// Waits for it.
func (w *World) FreeLock(root model.Root, which model.Lock) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.locks, lockKey{root, which})
	if w.freed != nil {
		close(w.freed)
		w.freed = nil
	}
}

type lockPort struct{ w *World }

// Wait is Take that waits for another process's hold until ctx ends, as the
// adapter's does (gragas#1690): ctx ending first answers the HeldError joined
// with ctx's error, and a lock this world's own run holds is never waited for.
func (x lockPort) Wait(ctx context.Context, root model.Root, which model.Lock) (model.Hold, error) {
	x.w.mu.Lock()
	_, err := x.w.rootOf("Lock.Wait", root)
	x.w.mu.Unlock()
	if err != nil {
		return model.Hold{}, err
	}
	for {
		h, err := x.Take(ctx, root, which)
		if !errors.Is(err, port.ErrHeld) {
			return h, err
		}
		x.w.mu.Lock()
		_, other := holder(x.w.locks, lockKey{root, which})
		if x.w.freed == nil {
			x.w.freed = make(chan struct{})
		}
		freed := x.w.freed
		x.w.mu.Unlock()
		if !other {
			return model.Hold{}, err
		}
		select {
		case <-freed:
		case <-ctx.Done():
			return model.Hold{}, errors.Join(err, ctx.Err())
		}
	}
}

// holder is the start time of the hold in held that k contends with: the
// root lock of k's root, or the scratch lock of any root, which is one for
// the machine as the adapter's is (model.ScratchLock).
func holder(held map[lockKey]int64, k lockKey) (int64, bool) {
	if k.which != model.ScratchLock {
		since, ok := held[k]
		return since, ok
	}
	for h, since := range held {
		if h.which == model.ScratchLock {
			return since, true
		}
	}
	return 0, false
}

func (x lockPort) Take(ctx context.Context, root model.Root, which model.Lock) (model.Hold, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return model.Hold{}, err
	}
	if _, err := x.w.rootOf("Lock.Take", root); err != nil {
		return model.Hold{}, err
	}
	k := lockKey{root, which}
	if since, held := holder(x.w.locks, k); held {
		return model.Hold{}, &HeldError{Root: root, Which: which, Since: since}
	}
	if since, mine := holder(x.w.taken, k); mine {
		return model.Hold{}, &HeldError{Root: root, Which: which, Since: since}
	}
	if which == model.RootLock {
		for held := range x.w.taken {
			if held.which == model.ScratchLock {
				return model.Hold{}, fmt.Errorf("the root lock is taken before the scratch lock, and this run holds the scratch lock of %q", held.root)
			}
		}
	}
	now, err := x.w.now("Lock.Take")
	if err != nil {
		return model.Hold{}, err
	}
	x.w.taken[k] = now.Unix()
	return model.Hold{Root: root, Which: which, Since: now.Unix()}, nil
}

func (x lockPort) Release(h model.Hold) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	k := lockKey{h.Root, h.Which}
	if _, mine := x.w.taken[k]; h == (model.Hold{}) || !mine {
		return fmt.Errorf("release: %s lock of %q is not held", h.Which, h.Root)
	}
	delete(x.w.taken, k)
	return nil
}

// journal is one root's journal.
type journal struct {
	records []model.Record
	torn    int
	kept    map[string][]byte
}

// TornLines makes the journal of root report n torn lines skipped.
func (w *World) TornLines(root model.Root, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.journalOf(root).torn = n
}

func (w *World) journalOf(root model.Root) *journal {
	j := w.journal[root]
	if j == nil {
		j = &journal{kept: map[string][]byte{}}
		w.journal[root] = j
	}
	return j
}

type journalPort struct{ w *World }

// runName is what a run id may be: it names a folder beside the journal.
var runName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Append adds r to the journal of root. A record of a run already there
// is refused on a cancelled ctx, as the adapter's read of the run is;
// the first record of a run is not, as runlog's Begin reads
// nothing.
func (x journalPort) Append(ctx context.Context, root model.Root, r model.Record) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.rootOf("Journal.Append", root); err != nil {
		return err
	}
	if slices.ContainsFunc(x.w.journalOf(root).records, func(old model.Record) bool { return old.Run == r.Run }) && ctx.Err() != nil {
		return fmt.Errorf("journal append: %w", ctx.Err())
	}
	if !runName.MatchString(r.Run) || !model.KnownKind(r.Kind) {
		return fmt.Errorf("a record needs a run of 1 to 64 letters, digits, dashes or underscores and a kind of intent, copy, step, start or close, got %q and %q", r.Run, r.Kind)
	}
	if len(r.Detail) > 0 && !json.Valid(r.Detail) {
		return fmt.Errorf("the detail of a record is one JSON value, got %q", r.Detail)
	}
	// The adapter sets At and Root; the fake keeps the ones it was given,
	// and stamps a record given no At by the seeded clock, as the adapter does.
	if r.At == 0 && x.w.clock != nil {
		r.At = x.w.clock.at.Unix()
	}
	r.Version = model.RecordVersion
	r.Detail = slices.Clone(r.Detail)
	j := x.w.journalOf(root)
	j.records = append(j.records, r)
	return nil
}

func (x journalPort) Keep(root model.Root, run, p string, old []byte) error {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.rootOf("Journal.Keep", root); err != nil {
		return err
	}
	if !runName.MatchString(run) {
		return fmt.Errorf("a copy needs a run of 1 to 64 letters, digits, dashes or underscores, got %q", run)
	}
	if _, kept := x.w.journalOf(root).kept[run+"\x00"+p]; kept {
		return fmt.Errorf("a copy of %s is already kept under run %s, and a second would store the replaced bytes as the original", p, run)
	}
	x.w.journalOf(root).kept[run+"\x00"+p] = slices.Clone(old)
	return nil
}

func (x journalPort) Kept(root model.Root, run, p string) ([]byte, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.rootOf("Journal.Kept", root); err != nil {
		return nil, err
	}
	if c, ok := x.w.journalOf(root).kept[run+"\x00"+p]; ok {
		return slices.Clone(c), nil
	}
	return nil, notExist("kept", p)
}

// KeptAt answers where the fake keeps the copy: a path in a state folder of
// its own, /state, named as the host names it, for a message to show.
func (x journalPort) KeptAt(root model.Root, run, p string) (string, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.rootOf("Journal.KeptAt", root); err != nil {
		return "", err
	}
	if !runName.MatchString(run) {
		return "", fmt.Errorf("a copy is kept under a run of 1 to 64 letters, digits, dashes or underscores, got %q", run)
	}
	if _, ok := x.w.journalOf(root).kept[run+"\x00"+p]; !ok {
		return "", notExist("kept at", p)
	}
	if x.w.keepIn != "" {
		return filepath.Join(x.w.keepIn, digestOf(string(root))+".copies", run, digestOf(p)), nil
	}
	return path.Join("/state", digestOf(string(root))+".copies", run, digestOf(p)), nil
}

// absolute is whether p is an absolute path the world takes: a slash path,
// as it spells its own roots, or this system's, for a place outside them. A
// path of this system's is never below a root, so it is outside every one.
func absolute(p string) bool {
	return path.IsAbs(p) || filepath.IsAbs(p)
}

// Records answers the records of root as the adapter does: the runs in the
// order their intent or close began them, each run's records in the order
// they were written, and the entries of a run not yet begun torn.
func (x journalPort) Records(ctx context.Context, root model.Root) ([]model.Record, int, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.rootOf("Journal.Records", root); err != nil {
		return nil, 0, err
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, fmt.Errorf("journal records: %w", err)
	}
	j := x.w.journalOf(root)
	// A run is begun by its first intent or close; a copy, step or start of a
	// run nothing has begun is torn, as on the adapter's journal, and is read
	// once the run is begun.
	var began []string
	for _, r := range j.records {
		if (r.Kind == model.KindIntent || r.Kind == model.KindClose) && !slices.Contains(began, r.Run) {
			began = append(began, r.Run)
		}
	}
	torn := j.torn
	out := make([]model.Record, 0, len(j.records))
	for _, r := range j.records {
		if slices.Contains(began, r.Run) {
			out = append(out, r)
		} else {
			torn++
		}
	}
	slices.SortStableFunc(out, func(a, b model.Record) int {
		return slices.Index(began, a.Run) - slices.Index(began, b.Run)
	})
	return out, torn, nil
}

// RecordsOf answers the records of data in the world's own form, one
// model.Record a line as encoding/json writes it (JournalLine), in the order
// Records answers them. The fake keeps no file of its journal, so the form is
// the world's alone and no adapter writes it: a test past the port seeds a
// journal file with it and never with the adapter's form. A line that is no
// record is torn, and a record of a newer Version is a
// model.NewerRecordError, as the adapter answers.
func (x journalPort) RecordsOf(ctx context.Context, data []byte) ([]model.Record, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, fmt.Errorf("journal records: %w", err)
	}
	var out []model.Record
	var began []string
	torn, nth := 0, map[string]int{}
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r model.Record
		if json.Unmarshal(line, &r) != nil || r.Version < 1 || !runName.MatchString(r.Run) || !knownKind(r.Kind) {
			torn++
			continue
		}
		nth[r.Run]++
		if r.Version > model.RecordVersion {
			return nil, 0, &model.NewerRecordError{Run: r.Run, Entry: nth[r.Run], Found: r.Version, Reads: model.RecordVersion}
		}
		if (r.Kind == model.KindIntent || r.Kind == model.KindClose) && !slices.Contains(began, r.Run) {
			began = append(began, r.Run)
		}
		out = append(out, r)
	}
	// The entries of a run no intent or close began are torn, as Records
	// answers them.
	out = slices.DeleteFunc(out, func(r model.Record) bool {
		if slices.Contains(began, r.Run) {
			return false
		}
		torn++
		return true
	})
	slices.SortStableFunc(out, func(a, b model.Record) int {
		return slices.Index(began, a.Run) - slices.Index(began, b.Run)
	})
	return out, torn, nil
}

// JournalLine is r as one line of the world's journal form, the form
// RecordsOf reads, for a test that seeds a journal file in a folder.
func JournalLine(r model.Record) string {
	b, err := json.Marshal(r)
	if err != nil {
		// A model.Record always encodes: its fields are strings, numbers
		// and bytes. Were it not to, the empty line is torn, and the test
		// that reads it back fails there.
		return ""
	}
	return string(b)
}

// JournalBytes is the journal of root in the world's form, the bytes
// RecordsOf reads as Records reads root: each record a line, then a torn
// line for each TornLines counted.
func (w *World) JournalBytes(root model.Root) []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	j := w.journalOf(root)
	var b bytes.Buffer
	for _, r := range j.records {
		b.WriteString(JournalLine(r) + "\n")
	}
	for range j.torn {
		b.WriteString(`{"Run":` + "\n")
	}
	return b.Bytes()
}

// knownKind is whether kind is one of the kinds of a record.
func knownKind(kind string) bool { return model.KnownKind(kind) }

// keepClosed is how long the fake keeps a closed run before Sweep removes
// it: the thirty days host/fs keeps one when the settings name no age.
const keepClosed = 30 * 24 * time.Hour

// Sweep removes every run of root closed longer ago than keepClosed by the
// seeded clock: its records and its copies. A run with no close is never
// swept. A sweep that removes a run also drops the entries of a run never
// begun, as host/fs's rewrite does. The fake keeps no files, so no link can lie on a copy's path;
// host/fs's own tests hold that refusal.
func (x journalPort) Sweep(ctx context.Context, root model.Root) (int, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.rootOf("Journal.Sweep", root); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("journal sweep: %w", err)
	}
	now, err := x.w.now("Journal.Sweep")
	if err != nil {
		return 0, err
	}
	j := x.w.journalOf(root)
	cutoff := now.Add(-keepClosed).Unix()
	var old []string
	for _, r := range j.records {
		if r.Kind == model.KindClose && r.At <= cutoff && !slices.Contains(old, r.Run) {
			old = append(old, r.Run)
		}
	}
	if len(old) == 0 {
		return 0, nil
	}
	// The journal is rewritten from the runs begun, as host/fs's is, so the
	// entries of a run no intent or close began go with it.
	begun := map[string]bool{}
	for _, r := range j.records {
		if r.Kind == model.KindIntent || r.Kind == model.KindClose {
			begun[r.Run] = true
		}
	}
	j.records = slices.DeleteFunc(j.records, func(r model.Record) bool { return slices.Contains(old, r.Run) || !begun[r.Run] })
	for k := range j.kept {
		if run, _, _ := strings.Cut(k, "\x00"); slices.Contains(old, run) {
			delete(j.kept, k)
		}
	}
	return len(old), nil
}

// HeldError is the error of a Take that found the lock held by another run:
// which lock of which root, and since when, in seconds. It matches port.ErrHeld.
type HeldError struct {
	Root  model.Root
	Which model.Lock
	Since int64
}

func (e *HeldError) Error() string {
	return fmt.Sprintf("the %s lock of %q is held by another run since %d", e.Which, e.Root, e.Since)
}

// Is makes a HeldError match port.ErrHeld.
func (e *HeldError) Is(target error) bool { return target == port.ErrHeld }

// HeldSince is the start time a Take's error names, and whether it names one.
func HeldSince(err error) (int64, bool) {
	if held, ok := errors.AsType[*HeldError](err); ok {
		return held.Since, true
	}
	return 0, false
}

// Written answers abs, a slash path below root, as the path relative to it;
// an abs that is not clean is ErrNotClean, as for Within. The world spells a
// path one way, so it is Within without the links.
func (x treePort) Written(root model.Root, abs string) (string, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if _, err := x.w.rootOf("Tree.Written", root); err != nil {
		return "", err
	}
	if !absolute(abs) {
		return "", fmt.Errorf("written %q: not an absolute path", abs)
	}
	if path.Clean(abs) != abs {
		return "", fmt.Errorf("written %s: %w", abs, port.ErrNotClean)
	}
	rel, ok := strings.CutPrefix(abs, strings.TrimSuffix(string(root), "/")+"/")
	if !ok {
		return "", fmt.Errorf("written %s: %w", abs, port.ErrOutside)
	}
	return rel, nil
}

// Within answers abs, a slash path below root, as the path relative to it; an
// abs that is not clean is ErrNotClean, as host/fs answers. The world follows
// no link: a link at the last part alone, leading inside, is ErrLinked naming
// where it leads, as host/fs answers, and a link at any other part, or one
// that leads out, is outside, the strictest answer host/fs could give, so a
// fake run never passes a path the host might not.
func (x treePort) Within(root model.Root, abs string) (string, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf("Tree.Within", root)
	if err != nil {
		return "", err
	}
	if !absolute(abs) {
		return "", fmt.Errorf("within %q: not an absolute path", abs)
	}
	if path.Clean(abs) != abs {
		return "", fmt.Errorf("within %s: %w", abs, port.ErrNotClean)
	}
	rel, ok := strings.CutPrefix(abs, strings.TrimSuffix(string(root), "/")+"/")
	if !ok {
		return "", fmt.Errorf("within %s: %w", abs, port.ErrOutside)
	}
	if err := inside(rel); err != nil || rel == "." {
		return "", fmt.Errorf("within %s: %w", abs, port.ErrOutside)
	}
	if target, ok := t.links[rel]; ok && t.linked("within", path.Dir(rel)) == nil {
		if to := path.Join(path.Dir(rel), target); inside(to) == nil && to != "." {
			return "", fmt.Errorf("within %s: %w to %s", abs, port.ErrLinked, to)
		}
	}
	if err := t.linked("within", rel); err != nil {
		return "", fmt.Errorf("within %s: a link: %w: %w", abs, port.ErrOutside, err)
	}
	return rel, nil
}

// OpenIn opens the folder at dir in root as a root of its own, following a
// link whose target stays inside root as host/fs does and refusing one that
// leads out. The new root is named by the folder it reached,
// so a folder reached through a link is the same root as by its own path.
// It holds a copy of the files below that folder, made at its first open:
// a write through one root is not seen through the other, and no test
// writes through both.
func (x treePort) OpenIn(root model.Root, dir string) (model.Root, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf("Tree.OpenIn", root)
	if err != nil {
		return "", err
	}
	if err := inside(dir); err != nil {
		return "", fmt.Errorf("open %q in %s: %w", dir, root, err)
	}
	real, err := t.resolve("open", dir)
	if err != nil {
		return "", err
	}
	if real == "" || real == "." {
		return root, nil
	}
	if _, ok := t.files[real]; ok {
		return "", fmt.Errorf("open %s: it is a file, not a folder", dir)
	}
	if !t.isDir(real) {
		return "", notExist("open", dir)
	}
	sub := model.Root(path.Join(string(root), real))
	if x.w.roots[sub] == nil {
		x.w.roots[sub] = t.below(real)
	}
	return sub, nil
}

// FS answers root's files as a read-only view, a copy made at the call. A
// path with .., or with a link at any step, is not in it, as host/fs
// refuses to open one; host/fs lists the link itself, the world does not.
func (x treePort) FS(root model.Root) (fs.FS, error) {
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t, err := x.w.rootOf("Tree.FS", root)
	if err != nil {
		return nil, err
	}
	view := fstest.MapFS{}
	for p, c := range t.files {
		if t.linked("view", p) != nil {
			continue
		}
		view[p] = &fstest.MapFile{Data: []byte(c)}
	}
	return view, nil
}

// resolve is p, a path inside t, with every link on it followed: one whose
// target stays inside t is followed, one that is absolute or climbs out
// matches port.ErrOutside, and so does a loop, a link reached a second time.
func (t *tree) resolve(op, p string) (string, error) {
	seen := map[string]bool{}
	for {
		at, target, rest, ok := t.firstLink(p)
		if !ok {
			return p, nil
		}
		if seen[at] {
			return "", fmt.Errorf("%s %s: %w", op, p, &fs.PathError{Op: "resolve", Path: at,
				Err: fmt.Errorf("it is a link in a loop: %w", port.ErrOutside)})
		}
		seen[at] = true
		next := path.Join(path.Dir(at), target, rest)
		if strings.HasPrefix(target, "/") || inside(next) != nil {
			return "", fmt.Errorf("%s %s: %w", op, p, &fs.PathError{Op: "resolve", Path: at,
				Err: fmt.Errorf("it is a link that leads out of the root: %w", port.ErrOutside)})
		}
		p = next
	}
}

// firstLink is the first component of p that is a link, its target and the
// rest of p below it.
func (t *tree) firstLink(p string) (at, target, rest string, ok bool) {
	if p == "" || p == "." {
		return "", "", "", false
	}
	parts := strings.Split(p, "/")
	for i := range parts {
		at = strings.Join(parts[:i+1], "/")
		if target, ok = t.links[at]; ok {
			return at, target, strings.Join(parts[i+1:], "/"), true
		}
	}
	return "", "", "", false
}

// below is a new tree holding a copy of what t holds below the folder dir,
// each path made relative to it.
func (t *tree) below(dir string) *tree {
	sub := &tree{files: map[string]string{}, exec: map[string]bool{}, links: map[string]string{}}
	prefix := dir + "/"
	for p, c := range t.files {
		if rel, ok := strings.CutPrefix(p, prefix); ok {
			sub.files[rel] = c
			sub.exec[rel] = t.exec[p]
		}
	}
	for p, target := range t.links {
		if rel, ok := strings.CutPrefix(p, prefix); ok {
			sub.links[rel] = target
		}
	}
	if t.tracked != nil {
		sub.tracked = map[string]bool{}
		for p, yes := range t.tracked {
			if rel, ok := strings.CutPrefix(p, prefix); ok {
				sub.tracked[rel] = yes
			}
		}
	}
	return sub
}
