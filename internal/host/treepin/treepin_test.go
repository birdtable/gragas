package treepin_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/treepin"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

func plant(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAFolderHeldToItsDigestPassesAndAChangedFileIsRefused(t *testing.T) {
	t.Parallel()
	dir := plant(t, map[string]string{"x/a/b": "one", "y/c": "two"})
	pin, err := treepin.Sum(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := treepin.Check(dir, pin); err != nil {
		t.Fatalf("the clean control is refused: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x", "a", "b"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := treepin.Check(dir, pin); err == nil || !strings.Contains(err.Error(), pin) {
		t.Fatalf("a changed file passed, or the refusal does not name the pin: %v", err)
	}
}

func TestAnAddedFileIsRefused(t *testing.T) {
	t.Parallel()
	dir := plant(t, map[string]string{"x/a": "one"})
	pin, err := treepin.Sum(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x", "planted"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := treepin.Check(dir, pin); err == nil {
		t.Fatal("a planted extra file passed")
	}
}

func TestALinkInsideTheFolderIsTakenAndOneLeavingItIsRefused(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	dir := plant(t, map[string]string{"x/lib/real": "x"})
	testenv.Link(t, "real", filepath.Join(dir, "x", "lib", "alias"))
	if _, err := treepin.Sum(dir); err != nil {
		t.Fatalf("a link whose target stays inside is refused: %v", err)
	}
	testenv.Link(t, "../../outside", filepath.Join(dir, "x", "lib", "escape"))
	if _, err := treepin.Sum(dir); err == nil {
		t.Fatal("a link leaving the folder passed")
	}
}

// boundary: MaxEntries
func TestAFolderAtItsEntryCapPassesAndOneOverIsRefused(t *testing.T) {
	t.Parallel()
	dir := plant(t, map[string]string{"x/seed": ""})
	for i := range treepin.MaxEntries - 1 {
		if err := os.WriteFile(filepath.Join(dir, "x", "f"+strconv.Itoa(i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := treepin.Sum(dir); err != nil {
		t.Fatalf("a folder at the cap is refused: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x", "over"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := treepin.Sum(dir); err == nil {
		t.Fatal("a folder one entry over the cap passed")
	}
}

// boundary: MaxBytes
func TestAFolderAtItsByteCapPassesAndOneOverIsRefused(t *testing.T) {
	t.Parallel()
	dir := plant(t, map[string]string{"x/seed": ""})
	set := func(size int64) {
		t.Helper()
		f, err := os.OpenFile(filepath.Join(dir, "x", "big"), os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	set(treepin.MaxBytes)
	if _, err := treepin.Sum(dir); err != nil {
		t.Fatalf("a folder at the byte cap is refused: %v", err)
	}
	set(treepin.MaxBytes + 1)
	if _, err := treepin.Sum(dir); err == nil {
		t.Fatal("a folder one byte over the cap passed")
	}
}

// The metadata folder holds the checkout's absolute path, which differs between
// two clones of one tree: it is no part of the pin, and nothing else beside the
// browsers' folders is allowed.
func TestTheMetadataFolderIsNotPinnedAndALooseFileBesideTheFoldersIsRefused(t *testing.T) {
	t.Parallel()
	dir := plant(t, map[string]string{"x/a": "one", ".links/h": "/clone/one/playwright-core"})
	pin, err := treepin.Sum(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".links", "h"), []byte("/another/clone/playwright-core"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".links", "other"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := treepin.Check(dir, pin); err != nil {
		t.Fatalf("a different metadata folder changed the digest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "loose"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := treepin.Sum(dir); err == nil || !strings.Contains(err.Error(), "not a folder of browsers") {
		t.Fatalf("a loose file beside the folders passed, or the refusal is not the adapter's: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "loose")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "y"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := treepin.Check(dir, pin); err == nil {
		t.Fatal("an added folder passed")
	}
}

// boundary: MaxFolders
func TestAFolderOfFoldersAtItsCapPassesAndOneOverIsRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for i := range treepin.MaxFolders {
		if err := os.MkdirAll(filepath.Join(dir, "f"+strconv.Itoa(i)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := treepin.Sum(dir); err != nil {
		t.Fatalf("browsers at the folder cap are refused: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "over"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := treepin.Sum(dir); err == nil {
		t.Fatal("one folder over the cap passed")
	}
}

// Two folder sets must not give one digest: a name holding the separator or a
// newline could make the outer listing read as another set's.
func TestAFolderNamedWithASpaceOrANewlineIsRefused(t *testing.T) {
	t.Parallel()
	dir := plant(t, map[string]string{"x/a": "one"})
	if _, err := treepin.Sum(dir); err != nil {
		t.Fatalf("the clean control is refused: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "A b"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := treepin.Sum(dir); err == nil {
		t.Fatal("a folder named with a space passed")
	}
	if err := os.Remove(filepath.Join(dir, "A b")); err != nil {
		t.Fatal(err)
	}
	// A system that cannot hold a newline in a name cannot hold the plant.
	if err := os.Mkdir(filepath.Join(dir, "A\nB"), 0o750); err == nil {
		if _, err := treepin.Sum(dir); err == nil {
			t.Fatal("a folder named with a newline passed")
		}
	}
}

// TestAPinOfTheOlderFormatIsRefusedByNameBeforeTheWalk: a
// browsers1: pin, of the treesum t1: lines go-files v0.4.0 no longer gives,
// is refused as an older format, to be measured again, and not as a plain
// mismatch; it is refused before the walk, so a folder the walk would refuse
// says the same. The control: the folder's own browsers2: pin passes.
// Planted: Check walking first, or the old prefix taken.
func TestAPinOfTheOlderFormatIsRefusedByNameBeforeTheWalk(t *testing.T) {
	t.Parallel()
	dir := plant(t, map[string]string{"x/a": "one"})
	pin, err := treepin.Sum(dir)
	if err != nil || !strings.HasPrefix(pin, "browsers2:") {
		t.Fatalf("the digest is %q, %v, want browsers2:", pin, err)
	}
	if err := treepin.Check(dir, pin); err != nil {
		t.Fatalf("the clean control is refused: %v", err)
	}
	old := "browsers1:" + strings.TrimPrefix(pin, "browsers2:")
	for name, at := range map[string]string{"the folder": dir, "no folder": filepath.Join(dir, "gone")} {
		if err := treepin.Check(at, old); !errors.Is(err, treepin.ErrOlder) || !strings.Contains(err.Error(), "older format") {
			t.Errorf("%s: a browsers1: pin answered %v, want ErrOlder", name, err)
		}
	}
}
