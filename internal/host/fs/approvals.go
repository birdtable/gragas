package fs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// ApprovalsVersion is the newest version of the approvals record this gragas
// reads. Version 1 held, for each entry, the digest of the grants alone;
// version 2 may hold, beside it, each grant by id and digest (the held
// field). A version 1 record is read, its entries with no grants by id, and
// is written as version 2 only when an entry being written holds grants by
// id: a write of a record none holds, a Revoke among them, keeps version 1, so
// nothing but a new approval moves a version 1 record to version 2.
const ApprovalsVersion = 2

// approvalsName is the record's name in the state folder.
const approvalsName = "approvals.json"

// errRecord is the refusal of a record that is not whole JSON of its form:
// it is never read as an answer, and never written over.
var errRecord = errors.New("the approvals record is not one this gragas reads")

// errNoRoots is the refusal of an Approvals made with no roots, or whose roots
// answer none: where a clone lies is not known, so the state folder is taken
// to lie in one.
var errNoRoots = errors.New("no root was named to hold the state folder against")

// Approvals is port.Trust on this machine: one record, approvals.json, in the
// state folder, read through the owned read and replaced whole under the
// state folder's own lock. It fails closed: any error
// is the answer no.
//
// Its fields are set by NewApprovals or NewApprovalsOfNoClone alone, so no
// forgotten argument turns the check against the clones off: a zero
// Approvals has no roots and refuses every call.
type Approvals struct {
	writer Writer
	clock  port.Clock
	dir    string
	mounts []string
	roots  func() ([]string, error)
	// noClone is set by NewApprovalsOfNoClone alone: there is no clone to
	// hold the state folder against.
	noClone bool
}

// NewApprovals is the Trust of the record in dir, the state folder, written
// through w, its time from clock, the folders above it judged with mounts.
// roots answers the absolute folders a clone carries for this run, the
// repository around the working folder first: dir must lie in none of them,
// each compared as the system resolves it. A nil roots, an error from it, or
// no root at all refuses every call.
func NewApprovals(w Writer, clock port.Clock, dir string, mounts []string, roots func() ([]string, error)) *Approvals {
	return &Approvals{writer: w, clock: clock, dir: dir, mounts: mounts, roots: roots}
}

// NewApprovalsOfNoClone is NewApprovals for a run that truly has no clone to
// hold the state folder against: nothing is compared. A caller with a
// repository, a project or a rulebook in hand uses NewApprovals.
func NewApprovalsOfNoClone(w Writer, clock port.Clock, dir string, mounts []string) *Approvals {
	return &Approvals{writer: w, clock: clock, dir: dir, mounts: mounts, noClone: true}
}

// Approved answers whether d is approved, false with the refusal on every
// error. No record, and no state folder, is false and no error, as an empty
// record is: it is the first state of every machine.
func (a *Approvals) Approved(ctx context.Context, d model.Digest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !d.Formed() {
		return false, notFormed("the digest", d)
	}
	if _, err := a.where(); err != nil {
		return false, err
	}
	got, err := a.held()
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(got, func(e approval) bool { return e.Digest == d }), nil
}

// Approve records ap with the time of the Clock, replacing an approval of its
// digest.
func (a *Approvals) Approve(ctx context.Context, ap model.Approval) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch {
	case !ap.Digest.Formed():
		return notFormed("the module's digest", ap.Digest)
	case !ap.Grants.Formed():
		return notFormed("the grants' digest", ap.Grants)
	case ap.Ref == "":
		return fmt.Errorf("approve %s: it names no reference: %w", ap.Digest, fs.ErrInvalid)
	}
	if err := model.GrantsHeldFormed(ap.Held); err != nil {
		return fmt.Errorf("approve %s: the grants by id: %w", ap.Digest, err)
	}
	apart, err := a.where()
	if err != nil {
		return err
	}
	return a.change(ctx, apart, func(got []approval) ([]approval, bool) {
		at := a.clock.Now().Unix()
		out := slices.DeleteFunc(got, func(e approval) bool { return e.Digest == ap.Digest })
		return append(out, approval{Approval: ap, at: at}), true
	})
}

// Prior is the newest approval, by the time recorded and then the greater
// digest, whose reference names name: all of Ref before its last "@", or Ref
// whole when it has none. No record, and none for name, match fs.ErrNotExist.
func (a *Approvals) Prior(ctx context.Context, name string) (model.Approval, error) {
	if err := ctx.Err(); err != nil {
		return model.Approval{}, err
	}
	if _, err := a.where(); err != nil {
		return model.Approval{}, err
	}
	got, err := a.held()
	if err != nil {
		return model.Approval{}, err
	}
	var best *approval
	for i, e := range got {
		if refName(e.Ref) != name {
			continue
		}
		if best == nil || e.at > best.at || (e.at == best.at && e.Digest > best.Digest) {
			best = &got[i]
		}
	}
	if best == nil {
		return model.Approval{}, fmt.Errorf("trust: no approval of %q was recorded: %w", name, fs.ErrNotExist)
	}
	return best.Approval, nil
}

// refName is the plugin name of a reference: all before its last "@", or the
// whole when it has none.
func refName(ref string) string {
	if name, _, ok := strings.CutLast(ref, "@"); ok {
		return name
	}
	return ref
}

// Revoke removes the approval of d. One that is not there, and no record,
// writes nothing: the record is read first, and only a digest it holds takes
// the lock.
func (a *Approvals) Revoke(ctx context.Context, d model.Digest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !d.Formed() {
		return notFormed("the digest", d)
	}
	apart, err := a.where()
	if err != nil {
		return err
	}
	got, err := a.held()
	switch {
	case err != nil:
		return err
	case !slices.ContainsFunc(got, func(e approval) bool { return e.Digest == d }):
		return nil
	}
	return a.change(ctx, apart, func(got []approval) ([]approval, bool) {
		n := len(got)
		out := slices.DeleteFunc(got, func(e approval) bool { return e.Digest == d })
		return out, len(out) != n
	})
}

// where refuses a state folder that lies in a folder a clone carries, with
// ErrCarried, an Approvals that names no root to hold it against, and a state
// folder that is there but not trusted. It answers the roots, each clean, for
// change to hold the state folder apart from when it makes it; NoClone's are
// none.
func (a *Approvals) where() ([]string, error) {
	if a.dir == "" {
		return nil, errors.New("trust: the approvals have no state folder")
	}
	var roots []string
	if !a.noClone {
		if a.roots == nil {
			return nil, fmt.Errorf("trust: %w: %w", errNoRoots, port.ErrCarried)
		}
		var err error
		roots, err = a.roots()
		if err != nil {
			return nil, fmt.Errorf("trust: the folders a clone carries could not be found: %w", err)
		}
		if len(roots) == 0 {
			return nil, fmt.Errorf("trust: %w: %w", errNoRoots, port.ErrCarried)
		}
	}
	apart, err := apartOf(a.dir, roots)
	if err != nil {
		return nil, fmt.Errorf("trust: %w", err)
	}
	return apart, a.trusted()
}

// trusted refuses a state folder that is there and that privateDirApart would
// refuse to take: one another account owns or can write, or whose folders
// above another account can write, matching port.ErrNotOwned. On this base
// the owned read judges the record and its mounts alone (which
// makes it judge the folders above too, is on main only), so
// without this a record in an open folder would be read as an answer while
// Approve refused to write there. A state folder that is not there passes:
// there is no record in it, and Approve makes it private.
func (a *Approvals) trusted() error {
	if _, err := os.Lstat(a.dir); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("trust: the state folder %s: %w", a.dir, err)
	}
	if err := stateFolder(a.dir, a.mounts); err != nil {
		return fmt.Errorf("trust: %w: %w", port.ErrNotOwned, err)
	}
	return nil
}

// read is every approval of the record, read through the owned read and
// judged whole. No record is an error matching fs.ErrNotExist, which held,
// the readers' way in, answers as the empty record.
//
// A record with more than one name is refused, matching port.ErrManyNames: a
// second name, a hard link from a clone's file, would let what writes that
// file in place write this machine's approvals, and the port keeps them
// outside every repository. The owned
// read judges the record and opens no handle a caller holds, so the record is
// read again by Tree.ReadSoleWithin, which counts its names (confine.NamesFile) on
// the handle it opened and reads through that same handle. What is decoded is
// what that handle read, and it must be what the owned read judged. A count
// the system will not give is a refusal, never one name. A volume that cannot
// hold a second name, as FAT, reports one for every file, so there this
// refuses nothing.
func (a *Approvals) read() ([]approval, error) {
	judged, err := a.writer.Tree.ReadOwned(filepath.Join(a.dir, approvalsName))
	if err != nil {
		return nil, fmt.Errorf("trust: %w", err)
	}
	state, err := a.writer.Tree.Open(a.dir)
	if err != nil {
		return nil, fmt.Errorf("trust: the state folder could not be opened: %w", err)
	}
	b, err := a.writer.Tree.ReadSoleWithin(state, approvalsName, hosttrust.MaxOwned)
	if err != nil {
		return nil, fmt.Errorf("trust: %w", err)
	}
	if !bytes.Equal(b, judged) {
		return nil, fmt.Errorf("trust: the record %s changed while it was read", filepath.Join(a.dir, approvalsName))
	}
	return decodeApprovals(b)
}

// held is read, with no record read as the empty one: every reader of the
// record answers through it, so none takes a missing record for a refusal,
// and Prior's first approval is none recorded for the name, not
// the missing file. Only a record that is not there is empty,
// the state folder not there yet among it. What is there and cannot be read or
// trusted stays the error it is, and none of those matches fs.ErrNotExist:
// the owned read refuses a link at the record's name, a dangling one among
// them, and the state folder's judge one that is a file
// (TestOnlyAMissingRecordIsEmpty).
func (a *Approvals) held() ([]approval, error) {
	got, err := a.read()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return got, err
}

// make is privateDirApart of the state folder, its walk also refusing a folder
// that is, or lies below, any folder of apart (Who.Apart). carried has
// passed the names that were there and kept the missing ones as written; a
// missing one made a link into a clone between that check and this make is
// followed by the judge and refused, so nothing is made or written in a clone
// whichever comes first.
func (a *Approvals) make(apart []string) error {
	if err := privateDirApart(a.dir, a.mounts, apart); err != nil {
		return fmt.Errorf("trust: the state folder %s: %w", a.dir, err)
	}
	return nil
}

// change takes the state folder's own root lock, reads the record again
// under it, and replaces it whole with what edit makes of it, unless edit
// says nothing changed. The lock is a Lock of its own over the state folder,
// which this call alone holds, so no lock of a project is held while it
// waits. The state folder is made, or taken, held apart from the roots where
// answered (make).
func (a *Approvals) change(ctx context.Context, apart []string, edit func([]approval) ([]approval, bool)) (err error) {
	if err := a.make(apart); err != nil {
		return err
	}
	state, err := a.writer.Tree.Open(a.dir)
	if err != nil {
		return fmt.Errorf("trust: the state folder could not be opened: %w", err)
	}
	l := &Lock{Dir: a.dir, Mounts: a.mounts, Clock: a.clock}
	h, err := l.Wait(ctx, state, model.RootLock)
	if err != nil {
		return fmt.Errorf("trust: %w", err)
	}
	defer func() { err = errors.Join(err, l.Release(h)) }()
	got, err := a.held()
	if err != nil {
		return err
	}
	out, changed := edit(got)
	if !changed {
		return nil
	}
	b, err := encodeApprovals(out)
	if err != nil {
		return err
	}
	if err := a.writer.Replace(state, approvalsName, b); err != nil {
		return fmt.Errorf("trust: %w", err)
	}
	return nil
}

// approval is one entry of the record: what was approved, and when, in Unix
// seconds by the Clock.
type approval struct {
	model.Approval
	at int64
}

// recordKeys is the record's members, sorted.
func recordKeys() []string { return []string{"approvals", "version"} }

// entryKeys is each entry's members, sorted.
func entryKeys() []string { return []string{"at", "digest", "grants", "ref"} }

// entryKeysHeld is an entry of version 2 that keeps its grants by id, sorted.
func entryKeysHeld() []string { return []string{"at", "digest", "grants", "held", "ref"} }

// decodeApprovals reads a record: one object of exactly a version this gragas
// reads and a list of entries, each of exactly a digest, a reference, a
// grants' digest and a time, no digest twice. Anything else is refused whole,
// matching errRecord; a newer version is refused naming both.
func decodeApprovals(b []byte) ([]approval, error) {
	v, err := canon.Decode(b)
	if err != nil {
		return nil, refuseRecord("it is not JSON one way: %v", err)
	}
	top, ok := v.(canon.Object)
	if !ok || !exactly(top, recordKeys()) {
		return nil, refuseRecord("it is not an object of exactly %v", recordKeys())
	}
	version := any(top["version"])
	if i, ok := version.(canon.Int); ok {
		version = int64(i)
	}
	switch n, is := ordered.Version(version, ApprovalsVersion); is {
	case ordered.VersionRead:
		// The version this gragas reads: the list is judged next.
	case ordered.VersionNewer:
		return nil, refuseRecord("its version is %d, and this gragas reads %d", n, ApprovalsVersion)
	case ordered.VersionNotWhole, ordered.VersionBelowOne:
		return nil, refuseRecord("its version is not a whole number from 1")
	}
	at2 := version == int64(2)
	list, ok := top["approvals"].(canon.Array)
	if !ok {
		return nil, refuseRecord("approvals is not a list")
	}
	out := make([]approval, 0, len(list))
	for i, item := range list {
		e, err := decodeEntry(item, at2)
		if err != nil {
			return nil, refuseRecord("entry %d: %v", i, err)
		}
		if slices.ContainsFunc(out, func(o approval) bool { return o.Digest == e.Digest }) {
			return nil, refuseRecord("entry %d: %s is approved twice", i, e.Digest)
		}
		out = append(out, e)
	}
	return out, nil
}

// decodeEntry reads one entry of the record.
func decodeEntry(v canon.Value, v2 bool) (approval, error) {
	o, ok := v.(canon.Object)
	_, has := o["held"]
	switch {
	case !ok:
		return approval{}, errors.New("it is not an object")
	case has && v2 && !exactly(o, entryKeysHeld()), !has && !exactly(o, entryKeys()), has && !v2:
		return approval{}, fmt.Errorf("it is not an object of exactly %v, or of %v in version 2", entryKeys(), entryKeysHeld())
	}
	d, dok := o["digest"].(canon.String)
	g, gok := o["grants"].(canon.String)
	r, rok := o["ref"].(canon.String)
	at, aok := o["at"].(canon.Int)
	switch {
	case !dok || !model.Digest(d).Formed():
		return approval{}, errors.New("digest is not of the form sha256:<64 hex>")
	case !gok || !model.Digest(g).Formed():
		return approval{}, errors.New("grants is not of the form sha256:<64 hex>")
	case !rok || r == "":
		return approval{}, errors.New("ref is not text, or empty")
	case !aok || at < 0:
		return approval{}, errors.New("at is not a whole number of seconds from 0")
	}
	out := approval{at: int64(at)}
	out.Digest, out.Ref, out.Grants = model.Digest(d), string(r), model.Digest(g)
	if has {
		held, err := decodeHeld(o["held"])
		if err != nil {
			return approval{}, err
		}
		out.Held = held
	}
	return out, nil
}

// encodeApprovals is the record of entries, sorted by digest, in canon's
// document layout: the same entries are the same bytes on every run.
func encodeApprovals(entries []approval) ([]byte, error) {
	sorted := slices.SortedFunc(slices.Values(entries), func(x, y approval) int { return canon.Compare(string(x.Digest), string(y.Digest)) })
	list := make(canon.Array, 0, len(sorted))
	for _, e := range sorted {
		o := canon.Object{
			"digest": canon.String(e.Digest), "ref": canon.String(e.Ref),
			"grants": canon.String(e.Grants), "at": canon.Int(e.at),
		}
		if e.Held != nil {
			o["held"] = encodeHeld(e.Held)
		}
		list = append(list, o)
	}
	version := 1
	if slices.ContainsFunc(sorted, func(e approval) bool { return e.Held != nil }) {
		version = ApprovalsVersion
	}
	b, err := canon.Encode(canon.Object{"version": canon.Int(version), "approvals": list}, canon.Document)
	if err != nil {
		return nil, fmt.Errorf("trust: the record could not be written: %w", err)
	}
	return b, nil
}

// exactly is whether o has the members keys and no other.
func exactly(o canon.Object, keys []string) bool {
	return slices.Equal(canon.SortedKeys(o), keys)
}

// refuseRecord is a refusal of the record, matching errRecord.
func refuseRecord(format string, args ...any) error {
	return fmt.Errorf("trust: %s: %s: %w", approvalsName, fmt.Sprintf(format, args...), errRecord)
}

// notFormed is the refusal of a digest that is not in the digest's form.
func notFormed(what string, d model.Digest) error {
	return fmt.Errorf("trust: %s %q is not of the form sha256:<64 hex>: %w", what, d, fs.ErrInvalid)
}

// decodeHeld reads an entry's grants by id: a list of objects of exactly an id
// and a digest, sorted by id, each once (model.GrantsHeldFormed). An empty list is a
// plugin that asks for no grant, not nil.
func decodeHeld(v canon.Value) ([]model.GrantHeld, error) {
	list, ok := v.(canon.Array)
	if !ok {
		return nil, errors.New("held is not a list")
	}
	out := make([]model.GrantHeld, 0, len(list))
	for i, item := range list {
		o, ok := item.(canon.Object)
		if !ok || !exactly(o, []string{"digest", "id"}) {
			return nil, fmt.Errorf("held %d is not an object of exactly an id and a digest", i)
		}
		id, iok := o["id"].(canon.String)
		d, dok := o["digest"].(canon.String)
		if !iok || !dok {
			return nil, fmt.Errorf("held %d: the id and the digest are not text", i)
		}
		out = append(out, model.GrantHeld{ID: string(id), Digest: model.Digest(d)})
	}
	if err := model.GrantsHeldFormed(out); err != nil {
		return nil, fmt.Errorf("held: %w", err)
	}
	return out, nil
}

// encodeHeld is an entry's grants by id.
func encodeHeld(h []model.GrantHeld) canon.Array {
	out := make(canon.Array, 0, len(h))
	for _, g := range h {
		out = append(out, canon.Object{"id": canon.String(g.ID), "digest": canon.String(g.Digest)})
	}
	return out
}
