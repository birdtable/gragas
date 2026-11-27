package contracttest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// TrustSetup is the Trust under test as two views over one record, as two
// runs of gragas on one machine are, with no approval in it yet.
type TrustSetup struct {
	A, B port.Trust
	// Refuse plants a record each method must refuse, one Approved could not
	// read whole and trust, and answers the error the refusal matches.
	Refuse func(t *testing.T) error
	// Carried is a Trust whose record would lie in a folder a clone carries.
	Carried port.Trust
}

// TrustMaker makes the setup for one subtest.
type TrustMaker func(t *testing.T) TrustSetup

// Trust runs the Trust suite against the port build answers.
func Trust(t *testing.T, build TrustMaker) {
	t.Helper()
	trustAnswers(t, build)
	trustDigests(t, build)
	trustRevokes(t, build)
	trustRefusesForm(t, build)
	trustRefusesRecord(t, build)
	trustRefusesCarried(t, build)
	trustRefusesEnded(t, build)
	trustPrior(t, build)
	trustTogether(t, build)
}

// DigestOf is the i-th planted digest, in the digest's form: "sha256:" and
// i written as sixty-four hexadecimal digits.
func DigestOf(i int) model.Digest { return model.Digest(fmt.Sprintf("sha256:%064x", i)) }

// approvalOf is the approval of the i-th planted module, the standin.
func approvalOf(i int) model.Approval {
	return model.Approval{Digest: DigestOf(i), Ref: fmt.Sprintf("standin@%d", i), Grants: DigestOf(1000 + i)}
}

// approved is Approved's answer, failing the test on an error.
func approved(t *testing.T, x port.Trust, d model.Digest) bool {
	t.Helper()
	ok, err := x.Approved(t.Context(), d)
	if err != nil {
		t.Fatalf("Approved(%s) = %v, %v", d, ok, err)
	}
	return ok
}

// approve is Approve, failing the test on an error.
func approve(t *testing.T, x port.Trust, a model.Approval) {
	t.Helper()
	if err := x.Approve(t.Context(), a); err != nil {
		t.Fatalf("Approve(%s) = %v", a.Digest, err)
	}
}

// trustAnswers: what Approved answers with no record, one approval, and the
// approval made again.
func trustAnswers(t *testing.T, build TrustMaker) {
	t.Helper()
	// No record is the first state of every machine, so it is
	// the empty record, not a refusal, and the host's Open refuses it as not
	// approved in the words of that alone.
	t.Run("no record is not approved, and no error", func(t *testing.T) {
		ok, err := build(t).A.Approved(t.Context(), DigestOf(1))
		if ok || err != nil {
			t.Errorf("Approved with no record = %v, %v, want false and no error", ok, err)
		}
	})
	t.Run("an approval holds for the other view", func(t *testing.T) {
		s := build(t)
		approve(t, s.A, approvalOf(1))
		if !approved(t, s.B, DigestOf(1)) {
			t.Error("the digest A approved is not approved for B")
		}
	})
	t.Run("approving again keeps it approved", func(t *testing.T) {
		s := build(t)
		approve(t, s.A, approvalOf(1))
		again := approvalOf(1)
		again.Ref = "standin@again"
		approve(t, s.B, again)
		if !approved(t, s.A, DigestOf(1)) {
			t.Error("a digest approved twice is not approved")
		}
	})
}

// trustDigests: an approval holds for its own digest and for no other.
func trustDigests(t *testing.T, build TrustMaker) {
	t.Helper()
	// Item 4: an approval is of the bundle's digest, as the lock names it.
	// Trust is handed digests and computes none, so each row hands it the
	// approved one and one that differs, standing for the bundle changed.
	// That a changed grant changes the bundle's digest is held by the
	// host's own row.
	t.Run("a bundle whose module changed by one byte is not approved", func(t *testing.T) {
		s := build(t)
		approve(t, s.A, approvalOf(1))
		module := []byte(DigestOf(1))
		module[len(module)-1] = '0'
		if approved(t, s.B, model.Digest(module)) {
			t.Errorf("%s, the bundle with its module changed, is approved", module)
		}
	})
	t.Run("a bundle whose grant changed and module did not is not approved", func(t *testing.T) {
		s := build(t)
		approve(t, s.A, approvalOf(1))
		grant := DigestOf(1 << 32)
		if approved(t, s.B, grant) {
			t.Errorf("%s, the bundle with its grant changed, is approved", grant)
		}
		if !approved(t, s.B, DigestOf(1)) {
			t.Error("the bundle as approved is no longer approved")
		}
	})
}

// trustRevokes: Revoke takes the one approval named, and naming one that is
// not there is no error.
func trustRevokes(t *testing.T, build TrustMaker) {
	t.Helper()
	t.Run("a revoked digest is not approved, and the emptied record is no error", func(t *testing.T) {
		s := build(t)
		approve(t, s.A, approvalOf(1))
		if err := s.A.Revoke(t.Context(), DigestOf(1)); err != nil {
			t.Fatalf("Revoke = %v", err)
		}
		if approved(t, s.B, DigestOf(1)) {
			t.Error("a revoked digest is approved")
		}
	})
	t.Run("revoking one of two removes the one named and keeps the other", func(t *testing.T) {
		s := build(t)
		approve(t, s.A, approvalOf(1))
		approve(t, s.A, approvalOf(2))
		if err := s.A.Revoke(t.Context(), DigestOf(1)); err != nil {
			t.Fatalf("Revoke = %v", err)
		}
		if approved(t, s.B, DigestOf(1)) {
			t.Error("the revoked digest is approved")
		}
		if !approved(t, s.B, DigestOf(2)) {
			t.Error("revoking one of two took the other's approval")
		}
	})
	t.Run("revoking what is not approved is not an error", func(t *testing.T) {
		s := build(t)
		if err := s.A.Revoke(t.Context(), DigestOf(1)); err != nil {
			t.Errorf("Revoke with no record = %v", err)
		}
		approve(t, s.A, approvalOf(2))
		if err := s.B.Revoke(t.Context(), DigestOf(1)); err != nil {
			t.Errorf("Revoke of a digest not approved = %v", err)
		}
		if !approved(t, s.A, DigestOf(2)) {
			t.Error("revoking another digest took this one's approval")
		}
	})
}

// trustRefusesForm: a digest or an approval not in its form is refused by
// each method, and writes nothing.
func trustRefusesForm(t *testing.T, build TrustMaker) {
	t.Helper()
	bad := model.Digest("sha256:ABC")
	t.Run("a digest not in the digest's form is refused by each method", func(t *testing.T) {
		s := build(t)
		if ok, err := s.A.Approved(t.Context(), bad); ok || err == nil {
			t.Errorf("Approved(%s) = %v, %v, want false and an error", bad, ok, err)
		}
		if err := s.A.Revoke(t.Context(), bad); err == nil {
			t.Errorf("Revoke(%s) is no error", bad)
		}
		for name, a := range map[string]model.Approval{
			"a bad digest":         {Digest: bad, Ref: "standin", Grants: DigestOf(1)},
			"a bad grants' digest": {Digest: DigestOf(1), Ref: "standin", Grants: bad},
			"no reference":         {Digest: DigestOf(1), Grants: DigestOf(2)},
		} {
			if err := s.A.Approve(t.Context(), a); err == nil {
				t.Errorf("Approve with %s is no error", name)
			}
		}
		if ok, err := s.B.Approved(t.Context(), DigestOf(1)); ok {
			t.Errorf("a refused Approve wrote an approval: %v, %v", ok, err)
		}
	})
}

// trustRefusesRecord: a record the setup plants to be refused is refused by
// each method, and no method writes over it.
func trustRefusesRecord(t *testing.T, build TrustMaker) {
	t.Helper()
	t.Run("a refused record is refused by each method and stays refused", func(t *testing.T) {
		s := build(t)
		want := s.Refuse(t)
		if ok, err := s.A.Approved(t.Context(), DigestOf(1)); ok || !errors.Is(err, want) {
			t.Errorf("Approved over a refused record = %v, %v, want false and %v", ok, err, want)
		}
		if err := s.A.Approve(t.Context(), approvalOf(1)); !errors.Is(err, want) {
			t.Errorf("Approve over a refused record = %v, want %v", err, want)
		}
		if err := s.A.Revoke(t.Context(), DigestOf(1)); !errors.Is(err, want) {
			t.Errorf("Revoke over a refused record = %v, want %v", err, want)
		}
		if ok, err := s.B.Approved(t.Context(), DigestOf(1)); ok || !errors.Is(err, want) {
			t.Errorf("after Approve, the refused record answers %v, %v: it was written over", ok, err)
		}
	})
}

// trustRefusesCarried: a record a clone carries is refused by each method,
// matching port.ErrCarried.
func trustRefusesCarried(t *testing.T, build TrustMaker) {
	t.Helper()
	t.Run("a record a clone carries is refused by each method", func(t *testing.T) {
		c := build(t).Carried
		if ok, err := c.Approved(t.Context(), DigestOf(1)); ok || !errors.Is(err, port.ErrCarried) {
			t.Errorf("Approved = %v, %v, want false and ErrCarried", ok, err)
		}
		if err := c.Approve(t.Context(), approvalOf(1)); !errors.Is(err, port.ErrCarried) {
			t.Errorf("Approve = %v, want ErrCarried", err)
		}
		if err := c.Revoke(t.Context(), DigestOf(1)); !errors.Is(err, port.ErrCarried) {
			t.Errorf("Revoke = %v, want ErrCarried", err)
		}
	})
}

// trustRefusesEnded: a call with an ended ctx is an error, never a yes, and
// changes nothing.
func trustRefusesEnded(t *testing.T, build TrustMaker) {
	t.Helper()
	t.Run("an ended ctx is an error and never approved", func(t *testing.T) {
		s := build(t)
		approve(t, s.A, approvalOf(1))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if ok, err := s.A.Approved(ctx, DigestOf(1)); ok || err == nil {
			t.Errorf("Approved with an ended ctx = %v, %v", ok, err)
		}
		if err := s.A.Approve(ctx, approvalOf(2)); err == nil {
			t.Error("Approve with an ended ctx is no error")
		}
		if err := s.A.Revoke(ctx, DigestOf(1)); err == nil {
			t.Error("Revoke with an ended ctx is no error")
		}
		if _, err := s.A.Prior(ctx, "standin"); err == nil || errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Prior with an ended ctx = %v, want the ctx's own error", err)
		}
		if !approved(t, s.B, DigestOf(1)) || approved(t, s.B, DigestOf(2)) {
			t.Error("a call with an ended ctx changed the record")
		}
	})
}

// together is how many approvals the two views make in turn.
const together = 8

// trustTogether: approvals the two views make in turn all hold, so each
// Approve reads the record afresh and never writes a copy it read before the
// other's back over it. Approvals made at the same time are the adapter's own
// test, beside the lock that orders them (host/fs, approvals_test.go), since
// this file starts no goroutine (go.concurrency.6).
func trustTogether(t *testing.T, build TrustMaker) {
	t.Helper()
	t.Run("approvals the two views make in turn all hold", func(t *testing.T) {
		s := build(t)
		for i := range together {
			x := s.A
			if i%2 == 1 {
				x = s.B
			}
			approve(t, x, approvalOf(i))
		}
		for i := range together {
			if !approved(t, s.A, DigestOf(i)) || !approved(t, s.B, DigestOf(i)) {
				t.Errorf("approval %d, made beside %d others, does not hold for both views", i, together-1)
			}
		}
	})
}
