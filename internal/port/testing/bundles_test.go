package porttest_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// bundlesOf is the setup of the suite over b, the fake or a plant that
// wraps it.
func bundlesOf(fake *porttest.Bundles, b port.Bundles) contracttest.BundlesSetup {
	const root = "/rulebook"
	return contracttest.BundlesSetup{
		Bundles: b,
		Root:    root,
		Put: func(t *testing.T, ref model.PluginRef, manifest, module []byte, other ...model.File) {
			t.Helper()
			fake.Put(root, ref, manifest, module, other...)
		},
		Change: func(t *testing.T, ref model.PluginRef, module []byte) {
			t.Helper()
			if err := fake.Change(root, ref, module); err != nil {
				t.Fatal(err)
			}
		},
		Add: func(t *testing.T, ref model.PluginRef, f model.File) {
			t.Helper()
			if err := fake.Add(root, ref, f); err != nil {
				t.Fatal(err)
			}
		},
	}
}

// TestTheInMemoryBundlesAnswerTheSuite: the fake every opener test uses
// answers the Bundles suite that the host/fs adapter will.
func TestTheInMemoryBundlesAnswerTheSuite(t *testing.T) {
	t.Parallel()
	contracttest.Bundles(t, func(t *testing.T) contracttest.BundlesSetup {
		t.Helper()
		var b porttest.Bundles
		return bundlesOf(&b, &b)
	})
}

// unsorted lists in the order of the version alone.
type unsorted struct{ *porttest.Bundles }

func (u unsorted) List(ctx context.Context, root string) ([]model.PluginRef, error) {
	out, err := u.Bundles.List(ctx, root)
	slices.SortFunc(out, func(a, b model.PluginRef) int { return -strings.Compare(a.Version, b.Version) })
	return out, err
}

// emptyList answers an empty list, not a nil one, for none.
type emptyList struct{ *porttest.Bundles }

func (e emptyList) List(ctx context.Context, root string) ([]model.PluginRef, error) {
	out, err := e.Bundles.List(ctx, root)
	if out == nil && err == nil {
		out = []model.PluginRef{}
	}
	return out, err
}

// judging refuses a bundle whose two digests differ, which is the opener's
// to refuse.
type judging struct{ *porttest.Bundles }

func (j judging) Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error) {
	b, err := j.Bundles.Read(ctx, root, ref)
	if err == nil && b.Read != b.Locked {
		return model.PluginBundle{}, fmt.Errorf("the bundle changed since it was locked")
	}
	return b, err
}

// relocking takes the lock's digest from the folder as read.
type relocking struct{ *porttest.Bundles }

func (r relocking) Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error) {
	b, err := r.Bundles.Read(ctx, root, ref)
	b.Locked = b.Read
	return b, err
}

// blank answers a missing ref as an empty bundle and no error.
type blank struct{ *porttest.Bundles }

func (k blank) Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error) {
	b, err := k.Bundles.Read(ctx, root, ref)
	if err != nil && ctx.Err() == nil {
		return model.PluginBundle{}, nil
	}
	return b, err
}

// sharing hands out the one slice it holds, so a caller's change is the
// next reader's.
type sharing struct {
	*porttest.Bundles
	held map[model.PluginRef]model.PluginBundle
}

func (s *sharing) Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error) {
	if b, ok := s.held[ref]; ok {
		return b, nil
	}
	b, err := s.Bundles.Read(ctx, root, ref)
	if err == nil {
		s.held[ref] = b
	}
	return b, err
}

// twoFiles answers the two files alone and drops every other.
type twoFiles struct{ *porttest.Bundles }

func (o twoFiles) Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error) {
	b, err := o.Bundles.Read(ctx, root, ref)
	b.Other = nil
	return b, err
}

// unordered answers the other files in the order opposite to their paths'.
type unordered struct{ *porttest.Bundles }

func (u unordered) Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error) {
	b, err := u.Bundles.Read(ctx, root, ref)
	slices.Reverse(b.Other)
	return b, err
}

// ownSum answers a digest of its own spelling, in the digest's form, as both
// the lock's and the read's: the sum of the module alone.
type ownSum struct{ *porttest.Bundles }

func (o ownSum) Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error) {
	b, err := o.Bundles.Read(ctx, root, ref)
	b.Locked = model.Digest(digest.Prefixed(b.Module))
	b.Read = b.Locked
	return b, err
}

// heedless reads on after its context ended, under a context that does not.
type heedless struct{ *porttest.Bundles }

func (h heedless) List(ctx context.Context, root string) ([]model.PluginRef, error) {
	return h.Bundles.List(context.WithoutCancel(ctx), root)
}

func (h heedless) Read(ctx context.Context, root string, ref model.PluginRef) (model.PluginBundle, error) {
	return h.Bundles.Read(context.WithoutCancel(ctx), root, ref)
}

// TestABundlesThatBreaksItsContractFailsTheSuite: each plant breaks one
// promise of port.Bundles and fails the suite for that reason; the control,
// the fake itself, passes.
func TestABundlesThatBreaksItsContractFailsTheSuite(t *testing.T) {
	t.Parallel()
	plants := []struct {
		label, want string
		wrap        func(*porttest.Bundles) port.Bundles
	}{
		{"a list in another order", "want [{a 1.10.0} {a 1.9.0} {b 1.0.0}]", func(b *porttest.Bundles) port.Bundles { return unsorted{b} }},
		{"none as an empty list", "want a nil list and no error", func(b *porttest.Bundles) port.Bundles { return emptyList{b} }},
		{"a changed bundle refused", "Read({standin 1.0.0}) = the bundle changed since it was locked", func(b *porttest.Bundles) port.Bundles { return judging{b} }},
		{"the lock's digest taken from the folder", "the lock's digest moved from", func(b *porttest.Bundles) port.Bundles { return relocking{b} }},
		{"a missing ref as an empty bundle", "= <nil>, want port.ErrNotInstalled", func(b *porttest.Bundles) port.Bundles { return blank{b} }},
		{"the bytes shared between reads", "a change to what Read gave changed the next Read", func(b *porttest.Bundles) port.Bundles {
			return &sharing{Bundles: b, held: map[model.PluginRef]model.PluginBundle{}}
		}},
		{"the other files dropped", "want the files as put", func(b *porttest.Bundles) port.Bundles { return twoFiles{b} }},
		{"the other files out of path order", "in the order of their paths", func(b *porttest.Bundles) port.Bundles { return unordered{b} }},
		{"a digest of the reader's own spelling", "want pluginbundle.Digest of its files", func(b *porttest.Bundles) port.Bundles { return ownSum{b} }},
		{"an ended context not heeded", "with an ended context = <nil>, want context.Canceled", func(b *porttest.Bundles) port.Bundles { return heedless{b} }},
	}
	for _, p := range plants {
		if passes(t, p.label, p.want, func(t *testing.T) {
			t.Helper()
			contracttest.Bundles(t, func(t *testing.T) contracttest.BundlesSetup {
				t.Helper()
				var b porttest.Bundles
				return bundlesOf(&b, p.wrap(&b))
			})
		}) {
			t.Errorf("the plant %q passed the suite", p.label)
		}
	}
	if !passes(t, control, "", func(t *testing.T) {
		t.Helper()
		contracttest.Bundles(t, func(t *testing.T) contracttest.BundlesSetup {
			t.Helper()
			var b porttest.Bundles
			return bundlesOf(&b, &b)
		})
	}) {
		t.Error("the fake failed the suite")
	}
}
