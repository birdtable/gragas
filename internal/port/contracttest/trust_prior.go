package contracttest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// heldOf is the grants of the i-th planted module, by id and sorted.
func heldOf(i int) []model.GrantHeld {
	return []model.GrantHeld{{ID: "alpha", Digest: DigestOf(2000 + i)}, {ID: "node", Digest: DigestOf(3000 + i)}}
}

// trustPrior: what Prior answers with no record, none for the name, one
// approval, two of a name, a record of grants not kept by id, a name that is
// the start of another, and what it refuses.
func trustPrior(t *testing.T, build TrustMaker) {
	t.Helper()
	priorRows(t, build)
	priorHeld(t, build)
	priorRefused(t, build)
}

// priorRows: no record, none for the name, and the approval of a name by either view.
func priorRows(t *testing.T, build TrustMaker) {
	t.Helper()
	t.Run("no record and no approval of the name are a first approval", func(t *testing.T) {
		s := build(t)
		if got, err := s.A.Prior(t.Context(), "standin"); !errors.Is(err, fs.ErrNotExist) || got.Digest != "" {
			t.Errorf("Prior over no record = %+v, %v, want no approval and fs.ErrNotExist", got, err)
		}
		approve(t, s.A, approvalOf(1))
		if got, err := s.B.Prior(t.Context(), "other"); !errors.Is(err, fs.ErrNotExist) || got.Digest != "" {
			t.Errorf("Prior of a name never approved = %+v, %v, want no approval and fs.ErrNotExist", got, err)
		}
	})
	t.Run("the approval of the name answers, with the grants it held, by either view", func(t *testing.T) {
		s := build(t)
		a := approvalOf(1)
		a.Held = heldOf(1)
		approve(t, s.A, a)
		got, err := s.B.Prior(t.Context(), "standin")
		if err != nil || got.Digest != a.Digest || got.Ref != a.Ref || got.Grants != a.Grants || !sameHeld(got.Held, a.Held) {
			t.Errorf("Prior(standin) = %+v, %v, want %+v", got, err, a)
		}
	})
}

// priorHeld: what Prior answers of the grants by id, and the order of two.
func priorHeld(t *testing.T, build TrustMaker) {
	t.Helper()
	t.Run("a plugin that asks for no grant is held empty, not unrecorded", func(t *testing.T) {
		s := build(t)
		none, unknown := approvalOf(1), approvalOf(2)
		none.Ref, unknown.Ref = "none@1", "unknown@1"
		none.Held = []model.GrantHeld{}
		approve(t, s.A, none)
		approve(t, s.A, unknown)
		if got, err := s.A.Prior(t.Context(), "none"); err != nil || got.Held == nil || len(got.Held) != 0 {
			t.Errorf("Prior(none) = %+v, %v, want Held empty and not nil", got, err)
		}
		if got, err := s.A.Prior(t.Context(), "unknown"); err != nil || got.Held != nil {
			t.Errorf("Prior(unknown) = %+v, %v, want Held nil: its grants were not kept by id", got, err)
		}
	})
	t.Run("of two versions of a name, the later one answers", func(t *testing.T) {
		s := build(t)
		first, second := approvalOf(1), approvalOf(2)
		first.Ref, second.Ref = "standin@1", "standin@2"
		first.Held, second.Held = heldOf(1), heldOf(2)
		approve(t, s.A, first)
		approve(t, s.B, second)
		if got, err := s.A.Prior(t.Context(), "standin"); err != nil || got.Digest != second.Digest {
			t.Errorf("Prior(standin) = %+v, %v, want the approval of %s, the later", got, err, second.Digest)
		}
	})
}

// priorRefused: a name that is the start of another, a list of grants not
// formed, and a record Prior refuses.
func priorRefused(t *testing.T, build TrustMaker) {
	t.Helper()
	t.Run("a name is the whole of the reference before its @, not the start of it", func(t *testing.T) {
		s := build(t)
		a := approvalOf(1)
		a.Ref = "standin-two@1"
		approve(t, s.A, a)
		if got, err := s.A.Prior(t.Context(), "standin"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Prior(standin) = %+v, %v, want fs.ErrNotExist: standin-two is another plugin", got, err)
		}
		if _, err := s.A.Prior(t.Context(), "standin-two"); err != nil {
			t.Errorf("Prior(standin-two) = %v, want its approval", err)
		}
	})
	t.Run("a grant list not in order or with a grant twice is refused and writes nothing", func(t *testing.T) {
		s := build(t)
		for name, h := range map[string][]model.GrantHeld{
			"unsorted":  {{ID: "node", Digest: DigestOf(1)}, {ID: "alpha", Digest: DigestOf(2)}},
			"twice":     {{ID: "alpha", Digest: DigestOf(1)}, {ID: "alpha", Digest: DigestOf(2)}},
			"no id":     {{ID: "", Digest: DigestOf(1)}},
			"a bad sum": {{ID: "alpha", Digest: "sha256:abc"}},
		} {
			a := approvalOf(1)
			a.Held = h
			if err := s.A.Approve(t.Context(), a); !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("Approve with grants %s = %v, want fs.ErrInvalid", name, err)
			}
		}
		if _, err := s.A.Prior(t.Context(), "standin"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Prior after the refusals = %v, want fs.ErrNotExist: nothing was written", err)
		}
	})
	t.Run("a refused record and a carried one are refused by Prior", func(t *testing.T) {
		s := build(t)
		want := s.Refuse(t)
		if got, err := s.A.Prior(t.Context(), "standin"); !errors.Is(err, want) || got.Digest != "" {
			t.Errorf("Prior over a refused record = %+v, %v, want no approval and %v", got, err, want)
		}
		if got, err := build(t).Carried.Prior(t.Context(), "standin"); !errors.Is(err, port.ErrCarried) || got.Digest != "" {
			t.Errorf("Prior of a record in a clone = %+v, %v, want no approval and ErrCarried", got, err)
		}
	})
}

// sameHeld is whether a and b are the same grants, nil and empty apart.
func sameHeld(a, b []model.GrantHeld) bool {
	if (a == nil) != (b == nil) || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
