package fs_test

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// TestAVersionOneRecordIsReadWithNoGrantsByIdAndWrittenAsVersionTwo: a record
// of version 1 answers Approved and Prior, its entry holding no grants by id
// (Held nil, not empty), and an Approve beside it writes version 2 and keeps
// that entry as it was, so no id is made up for it.
func TestAVersionOneRecordIsReadWithNoGrantsByIdAndWrittenAsVersionTwo(t *testing.T) {
	t.Parallel()
	d, g := string(contracttest.DigestOf(1)), string(contracttest.DigestOf(2))
	m := newMachine(t)
	m.plant(t, `{"version":1,"approvals":[{"at":1,"digest":"`+d+`","grants":"`+g+`","ref":"old@1"}]}`)
	x := m.trust(t)
	got, err := x.Prior(t.Context(), "old")
	if err != nil || got.Digest != contracttest.DigestOf(1) || got.Held != nil {
		t.Fatalf("Prior(old) = %+v, %v, want the entry, with Held nil", got, err)
	}
	a := standin(3)
	a.Held = []model.GrantHeld{{ID: "alpha", Digest: contracttest.DigestOf(9)}}
	if err := x.Approve(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(m.record())
	if err != nil {
		t.Fatal(err)
	}
	rec := string(b)
	if !strings.Contains(rec, `"version": 2`) || strings.Count(rec, `"held"`) != 1 {
		t.Errorf("the record after the Approve is\n%s\nwant version 2 and grants by id for the new entry alone", rec)
	}
	if got, err := x.Prior(t.Context(), "old"); err != nil || got.Held != nil {
		t.Errorf("Prior(old) after = %+v, %v, want Held still nil", got, err)
	}
}

// TestPriorIsTheNewestByTimeRecordedThenTheGreaterDigest: of two entries of a
// name, the later time answers whichever digest is greater, and at one time
// the greater digest does.
func TestPriorIsTheNewestByTimeRecordedThenTheGreaterDigest(t *testing.T) {
	t.Parallel()
	d1, d2, g := string(contracttest.DigestOf(1)), string(contracttest.DigestOf(2)), string(contracttest.DigestOf(3))
	entry := func(d string, at string) string {
		return `{"at":` + at + `,"digest":"` + d + `","grants":"` + g + `","ref":"p@1"}`
	}
	for name, c := range map[string]struct {
		a, b string
		want model.Digest
	}{
		"the later time with the lesser digest": {entry(d1, "9"), entry(d2, "5"), contracttest.DigestOf(1)},
		"one time, the greater digest":          {entry(d1, "5"), entry(d2, "5"), contracttest.DigestOf(2)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			m.plant(t, `{"version":2,"approvals":[`+c.a+`,`+c.b+`]}`)
			got, err := m.trust(t).Prior(t.Context(), "p")
			if err != nil || got.Digest != c.want {
				t.Errorf("Prior(p) = %+v, %v, want the approval of %s", got, err, c.want)
			}
		})
	}
	m := newMachine(t)
	if _, err := m.trust(t).Prior(t.Context(), "p"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Prior with no record = %v, want fs.ErrNotExist", err)
	}
}

// TestARevokeKeepsAVersionOneRecordAtVersionOne: a record of version 1 with
// two entries, one revoked, is still version 1 and still holds the other as it
// was, so a gragas that knows only version 1 still reads it: nothing but a new
// approval that holds grants by id moves a version 1 record to version 2.
// Planted: the version written always the newest.
func TestARevokeKeepsAVersionOneRecordAtVersionOne(t *testing.T) {
	t.Parallel()
	d1, d2, g := string(contracttest.DigestOf(1)), string(contracttest.DigestOf(2)), string(contracttest.DigestOf(3))
	m := newMachine(t)
	m.plant(t, `{"version":1,"approvals":[{"at":1,"digest":"`+d1+`","grants":"`+g+`","ref":"one@1"},{"at":2,"digest":"`+d2+`","grants":"`+g+`","ref":"two@1"}]}`)
	x := m.trust(t)
	if err := x.Revoke(t.Context(), contracttest.DigestOf(1)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(m.record())
	if err != nil {
		t.Fatal(err)
	}
	rec := string(b)
	if !strings.Contains(rec, `"version": 1`) || strings.Contains(rec, `"version": 2`) || strings.Contains(rec, `"held"`) || !strings.Contains(rec, d2) || strings.Contains(rec, d1) {
		t.Errorf("the record after the Revoke is\n%s\nwant version 1, the entry two as it was, and entry one gone", rec)
	}
	if ok, err := x.Approved(t.Context(), contracttest.DigestOf(2)); err != nil || !ok {
		t.Errorf("Approved(two) after = %v, %v, want true", ok, err)
	}
}
