package contracttest

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/port"
)

// FileNames runs the FileNames suite against the port build answers: a name no system takes, or
// .git in any case, is refused with an error that matches fs.ErrInvalid, and no code matches on its words; a name
// that passes answers a key, and two names a disk that folds case or a canonical normal form reads as one answer
// one key. The refused names are go-files' own list, filestest.HostileNames, each refused in at least one of its
// names; the keys are written out: the fake folds as the adapter does only because both are confine.Portable, and
// a fake that lower-cased would answer these three wrong.
func FileNames(t *testing.T, build func(t *testing.T) port.FileNames) {
	t.Helper()
	tr := build(t)
	t.Run("Portable: each hostile name is refused, and the error matches fs.ErrInvalid", func(t *testing.T) {
		t.Parallel()
		portableRefusesTheHostileNames(t, tr)
	})
	t.Run("Portable: names a folder that folds reads as one answer one key", func(t *testing.T) {
		t.Parallel()
		portableKeysOneForNamesThatFold(t, tr)
	})
	t.Run("Portable: two names a disk keeps apart answer two keys, and a plain name passes", func(t *testing.T) {
		t.Parallel()
		portableKeysTwoForNamesApart(t, tr)
	})
}

// portableRefusesTheHostileNames holds each name of go-files' list to a refusal in at least one of its names.
func portableRefusesTheHostileNames(t *testing.T, tr port.FileNames) {
	t.Helper()
	for _, h := range filestest.HostileNames() {
		if strings.Contains(h.Why, "fork") {
			continue // a path rule of go-files' confine.Local, which Portable does not hold
		}
		if !oneNameRefused(t, tr, h.Path) {
			t.Errorf("no name of %+q is refused by Portable, which go-files' list refuses as %q", h.Path, h.Why)
		}
	}
}

// oneNameRefused is whether Portable refuses a name of the slash path p, each refusal matching fs.ErrInvalid.
func oneNameRefused(t *testing.T, tr port.FileNames, p string) bool {
	t.Helper()
	refused := false
	for name := range strings.SplitSeq(p, "/") {
		if _, err := tr.Portable(name); err != nil {
			refused = true
			if !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("Portable(%+q): %v does not match fs.ErrInvalid", name, err)
			}
		}
	}
	return refused
}

// portableKeysOneForNamesThatFold holds the names a disk that folds reads as one to one key each.
func portableKeysOneForNamesThatFold(t *testing.T, tr port.FileNames) {
	t.Helper()
	for _, names := range [][]string{
		{"\u00e9", "e\u0301"},  // a composed and a decomposed e acute
		{"k", "K", "\u212a"},   // k, K and the Kelvin sign
		{"\u00df", "ss", "SS"}, // sharp s and ss
		{"History.jsonl", "history.jsonl"},
	} {
		want, err := tr.Portable(names[0])
		if err != nil || want == "" {
			t.Errorf("Portable(%+q) = %q, %v, want a key", names[0], want, err)
			continue
		}
		for _, n := range names[1:] {
			if got, err := tr.Portable(n); err != nil || got != want {
				t.Errorf("Portable(%+q) = %q, %v, want the key %q of %+q", n, got, err, want, names[0])
			}
		}
	}
}

// portableKeysTwoForNamesApart holds two names a disk keeps apart to two keys, and an empty name to a refusal.
func portableKeysTwoForNamesApart(t *testing.T, tr port.FileNames) {
	t.Helper()
	a, errA := tr.Portable("keep.md")
	b, errB := tr.Portable("keeps.md")
	if errA != nil || errB != nil || a == "" || b == "" || a == b {
		t.Errorf("keep.md and keeps.md: %q %v and %q %v, want two keys", a, errA, b, errB)
	}
	if _, err := tr.Portable(""); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("an empty name: %v, want an error that matches fs.ErrInvalid", err)
	}
}
