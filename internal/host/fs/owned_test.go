package fs_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	testenv "github.com/birdtable/go-testkit"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/port"
)

// refusal is the owned read's refusal, as a caller reads it without
// importing host/trust.
type refusal interface {
	error
	Refusal() (why, fix string)
}

// TestReadOwnedRefusesAFileItsGroupCanWrite: a file outside the roots that
// its group can write is refused, matching port.ErrNotOwned, with why and the
// line that fixes it; no second account is needed to plant it. The control is
// the same file before its group may write it, which is read. The group's
// write is planted through go-files' trusttest, never a mode written here.
func TestReadOwnedRefusesAFileItsGroupCanWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"list": "a\n"})
	path := filepath.Join(dir, "list")
	tr := newTree(t)
	if got, err := tr.ReadOwned(path); err != nil || string(got) != "a\n" {
		t.Fatalf("the control: ReadOwned of a file only this account writes = %q, %v", got, err)
	}
	trusttest.Loosen(t, path, trusttest.GroupWrites)
	got, err := tr.ReadOwned(path)
	if !errors.Is(err, port.ErrNotOwned) || got != nil {
		t.Fatalf("ReadOwned of a file its group can write = %q, %v; want nothing and port.ErrNotOwned", got, err)
	}
	r, ok := errors.AsType[refusal](err)
	if !ok {
		t.Fatalf("the refusal %v says no why and fix through Refusal()", err)
	}
	why, fix := r.Refusal()
	// The words are the system's own: a group on Unix, an access list's entry on
	// Windows. What both give is a why, and a fix that names the file.
	if why == "" || !strings.Contains(fix, filepath.Base(path)) {
		t.Errorf("Refusal() = %q, %q; want a why and a fix naming the file", why, fix)
	}
}

// TestReadOwnedRefusesAPathThatIsNotClean: the path the settings name is
// read as given, so one that climbs or is relative is refused before any
// file is opened, even when it names a file that would be read. The control
// is the clean path to the same file.
func TestReadOwnedRefusesAPathThatIsNotClean(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"list": "a\n"})
	tr := newTree(t)
	if _, err := tr.ReadOwned(filepath.Join(dir, "list")); err != nil {
		t.Fatalf("the control: ReadOwned of the clean path = %v", err)
	}
	for _, p := range []string{dir + "/x/../list", "list", ""} {
		if got, err := tr.ReadOwned(p); err == nil {
			t.Errorf("ReadOwned(%q) = %q, nil; want refused", p, got)
		}
	}
}

// TestTheLegacyNamesFileIsReadOwned: the old engine's names read, Files.Open,
// is the same owned read: a file its group can write is refused, matching
// port.ErrNotOwned. The control is the same file before its group may write
// it, which is read whole.
func TestTheLegacyNamesFileIsReadOwned(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, map[string]string{"list": "a\n"})
	path := filepath.Join(dir, "list")
	f, err := (hostfs.Files{}).Open(path)
	if err != nil {
		t.Fatalf("the control: Open = %v", err)
	}
	got, err := io.ReadAll(f)
	if cerr := f.Close(); err != nil || cerr != nil || string(got) != "a\n" {
		t.Fatalf("the control: read %q, %v, %v", got, err, cerr)
	}
	trusttest.Loosen(t, path, trusttest.GroupWrites)
	if _, err := (hostfs.Files{}).Open(path); !errors.Is(err, port.ErrNotOwned) {
		t.Errorf("Open of a file its group can write = %v; want port.ErrNotOwned", err)
	}
}

// TestAnOwnedReadsRefusalMatchesItsReason: the owned
// read's refusal matches the port's error of its reason, beside
// port.ErrNotOwned, through the adapter's ReadOwned (the Owned port) and the old engine's
// Files.Open alike, and no other reason's, so the core says each refusal's
// own fix without reading its words, which name the path. The reasons
// planted here are the ones a file of this account's can show: its group may
// write it, a link at the name, a folder there, and a file over 1 MiB. The
// control is each path before its plant, a file read whole. Planted: the
// adapter's refusal with no reason (every row red here).
func TestAnOwnedReadsRefusalMatchesItsReason(t *testing.T) {
	t.Parallel()
	reasons := []error{port.ErrOwnedByOther, port.ErrOwnedWritable, port.ErrOwnedLink, port.ErrOwnedNotRegular, port.ErrOwnedTooLong, port.ErrOwnedMount, port.ErrOwnedMountEntry}
	for _, c := range []struct {
		name  string
		plant func(t *testing.T, path string)
		is    error
	}{
		{"its group can write it", func(t *testing.T, path string) {
			t.Helper()
			trusttest.Loosen(t, path, trusttest.GroupWrites)
		}, port.ErrOwnedWritable},
		{"a link at the name", func(t *testing.T, path string) {
			t.Helper()
			if err := os.Rename(path, path+".real"); err != nil {
				t.Fatal(err)
			}
			testenv.Link(t, path+".real", path)
		}, port.ErrOwnedLink},
		{"a folder at the name", func(t *testing.T, path string) {
			t.Helper()
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}, port.ErrOwnedNotRegular},
		{"over 1 MiB", func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte(strings.Repeat("a", 1<<20+1)), 0o600); err != nil {
				t.Fatal(err)
			}
		}, port.ErrOwnedTooLong},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			write(t, dir, map[string]string{"list": "a\n"})
			path := filepath.Join(dir, "list")
			tr := newTree(t)
			if got, err := tr.ReadOwned(path); err != nil || string(got) != "a\n" {
				t.Fatalf("the control: ReadOwned = %q, %v", got, err)
			}
			c.plant(t, path)
			_, tree := tr.ReadOwned(path)
			_, legacy := (hostfs.Files{}).Open(path)
			for _, got := range []struct {
				by  string
				err error
			}{{"ReadOwned", tree}, {"Files.Open", legacy}} {
				if !errors.Is(got.err, port.ErrNotOwned) || !errors.Is(got.err, c.is) {
					t.Errorf("%s: %s = %v; want port.ErrNotOwned and %v", c.name, got.by, got.err, c.is)
				}
				for _, other := range reasons {
					if !errors.Is(other, c.is) && errors.Is(got.err, other) {
						t.Errorf("%s: %s = %v; it matches %v too", c.name, got.by, got.err, other)
					}
				}
			}
		})
	}
}
