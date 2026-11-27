package porttest

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// trust is the world's record of approvals: nil until the first
// Approve or SeedApproval, so a world nobody seeded has no record, as a
// machine that never approved anything has no file.
type trust struct {
	approvals map[model.Digest]model.Approval
	// seq is the order each approval was recorded in, the later the larger: the
	// fake's own clock, for Prior.
	seq  map[model.Digest]int
	next int
	// refused, when set, is what every method answers instead: the record
	// is one the adapter would refuse to read or write, or it is kept in a
	// folder a clone carries.
	refused error
}

// trustPort is the fake of port.Trust over the same world.
type trustPort struct{ w *World }

// SeedApproval records a as approved, as an earlier Approve would have.
func (w *World) SeedApproval(a model.Approval) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := w.record()
	t.approvals[a.Digest] = a
	t.next++
	t.seq[a.Digest] = t.next
}

// SeedTrustRefused makes every method of Trust answer err, matched as the
// adapter's would be: port.ErrNotOwned and its reason for a record the owned
// read refuses, port.ErrCarried for one kept in a folder a clone carries.
// Approved answers false with it, and Approve and Revoke write nothing.
func (w *World) SeedTrustRefused(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.record().refused = err
}

// Approvals is every approval Trust holds now, sorted by digest.
func (w *World) Approvals() []model.Approval {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.trust == nil {
		return nil
	}
	return slices.SortedFunc(maps.Values(w.trust.approvals), func(a, b model.Approval) int { return cmp.Compare(a.Digest, b.Digest) })
}

// record is the world's record of approvals, made empty on first use. The
// caller holds w.mu.
func (w *World) record() *trust {
	if w.trust == nil {
		w.trust = &trust{approvals: map[model.Digest]model.Approval{}, seq: map[model.Digest]int{}}
	}
	return w.trust
}

// Approved answers as the adapter does: no record is false and no error, as
// an empty one is, a refused record false with its refusal, and
// otherwise whether d itself is approved.
func (x trustPort) Approved(ctx context.Context, d model.Digest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !d.Formed() {
		return false, fmt.Errorf("trust: %q is not a digest of the form sha256:<64 hex>: %w", d, fs.ErrInvalid)
	}
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	switch {
	case x.w.trust == nil:
		return false, nil
	case x.w.trust.refused != nil:
		return false, x.w.trust.refused
	}
	_, ok := x.w.trust.approvals[d]
	return ok, nil
}

// Approve records a, replacing an approval of the same digest.
func (x trustPort) Approve(ctx context.Context, a model.Approval) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := formed(a); err != nil {
		return err
	}
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t := x.w.record()
	if t.refused != nil {
		return t.refused
	}
	t.approvals[a.Digest] = a
	t.next++
	t.seq[a.Digest] = t.next
	return nil
}

// Revoke removes the approval of d; one that is not there is not an error.
func (x trustPort) Revoke(ctx context.Context, d model.Digest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !d.Formed() {
		return fmt.Errorf("trust: %q is not a digest of the form sha256:<64 hex>: %w", d, fs.ErrInvalid)
	}
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	if x.w.trust == nil {
		return nil
	}
	if x.w.trust.refused != nil {
		return x.w.trust.refused
	}
	delete(x.w.trust.approvals, d)
	delete(x.w.trust.seq, d)
	return nil
}

// formed refuses an approval the adapter would not record: a digest that is
// not in the digest's form, or an empty reference.
func formed(a model.Approval) error {
	switch {
	case !a.Digest.Formed():
		return fmt.Errorf("trust: the module's digest %q is not of the form sha256:<64 hex>: %w", a.Digest, fs.ErrInvalid)
	case !a.Grants.Formed():
		return fmt.Errorf("trust: the grants' digest %q is not of the form sha256:<64 hex>: %w", a.Grants, fs.ErrInvalid)
	case a.Ref == "":
		return fmt.Errorf("trust: the approval of %s names no reference: %w", a.Digest, fs.ErrInvalid)
	}
	if err := model.GrantsHeldFormed(a.Held); err != nil {
		return fmt.Errorf("trust: the grants by id of %s: %w", a.Digest, err)
	}
	return nil
}

// Prior answers as the adapter does: the approval of name recorded last, no
// record or none for name matching fs.ErrNotExist, a refused record its
// refusal.
func (x trustPort) Prior(ctx context.Context, name string) (model.Approval, error) {
	if err := ctx.Err(); err != nil {
		return model.Approval{}, err
	}
	x.w.mu.Lock()
	defer x.w.mu.Unlock()
	t := x.w.trust
	switch {
	case t == nil:
		return model.Approval{}, fmt.Errorf("trust: no approvals were ever recorded: %w", fs.ErrNotExist)
	case t.refused != nil:
		return model.Approval{}, t.refused
	}
	var best model.Approval
	at := 0
	for d, a := range t.approvals {
		if refName(a.Ref) == name && t.seq[d] > at {
			best, at = a, t.seq[d]
		}
	}
	if at == 0 {
		return model.Approval{}, fmt.Errorf("trust: no approval of %q was recorded: %w", name, fs.ErrNotExist)
	}
	return best, nil
}

// refName is the plugin name of a reference: all before its last "@", or the
// whole when it has none.
func refName(ref string) string {
	if name, _, ok := strings.CutLast(ref, "@"); ok {
		return name
	}
	return ref
}
