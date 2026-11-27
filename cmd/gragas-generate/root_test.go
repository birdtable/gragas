package main

import (
	"os"
	"path/filepath"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/dev/generate"
)

// spellings are the two ways a committed link leaves the repository: written as
// an absolute path, and climbing out with "..".
func spellings() []struct {
	name  string
	climb bool
} {
	return []struct {
		name  string
		climb bool
	}{
		{"a link written as an absolute path", false},
		{"a link that climbs out with ..", true},
	}
}

// linkOut makes at (under dir) a link to target, which is outside dir. With
// climb the target is written relative to the link's folder.
func linkOut(t *testing.T, target, at string, climb bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		t.Fatal(err)
	}
	if climb {
		rel, err := filepath.Rel(filepath.Dir(at), target)
		if err != nil {
			t.Fatal(err)
		}
		target = rel
	}
	testenv.Link(t, target, at)
}

// outsideHolds is the entries of a folder outside the repository.
func outsideHolds(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestGenerateReadsTheSchemasThroughTheRoot: the schema folder is a link to a
// copy of the schemas outside the repository. A plain read lists and parses the
// copy and goes on to write; through the Root the read is refused and the run
// fails, writing nothing. Planted failure: both the listing
// (fs.ReadDir(root.FS(), ...)) and the per-file read (root.ReadFile) in
// readGenerateInput as os calls on the joined path; either one alone stays
// green, because the other still refuses.
func TestGenerateReadsTheSchemasThroughTheRoot(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	for _, tc := range spellings() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := copySchemas(t)
			outside := filepath.Join(t.TempDir(), "schemas")
			at := filepath.Join(dir, filepath.FromSlash(generate.SchemaDir[:len(generate.SchemaDir)-1]))
			if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(at, outside); err != nil {
				t.Fatal(err)
			}
			linkOut(t, outside, at, tc.climb)
			code, _, errs := do("-C", dir)
			if code != exitFailed {
				t.Fatalf("exit %d, want %d\n%s", code, exitFailed, errs)
			}
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(generate.GoDir))); err == nil {
				t.Error("a refused read still wrote the generated files")
			}
		})
	}
}

// TestGenerateWritesThroughTheRoot: the folder of the Go types is a link to an
// empty folder outside the repository. A plain write makes the files there;
// through the Root the write is refused and the run fails, leaving the folder
// outside empty. Planted failure: the stale read and the listing in
// generateCommand, root.MkdirAll and replaceIn in writeGenerated, all as os
// calls on the joined path; any one alone stays green, because the others
// still refuse.
func TestGenerateWritesThroughTheRoot(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	for _, tc := range spellings() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := copySchemas(t)
			outside := t.TempDir()
			linkOut(t, outside, filepath.Join(dir, filepath.FromSlash(generate.GoDir[:len(generate.GoDir)-1])), tc.climb)
			code, _, errs := do("-C", dir)
			if code != exitFailed {
				t.Fatalf("exit %d, want %d\n%s", code, exitFailed, errs)
			}
			if got := outsideHolds(t, outside); len(got) != 0 {
				t.Errorf("the folder outside holds %v", got)
			}
		})
	}
}

// TestGenerateRemovesNothingThroughALinkedFolder: the folder of the TypeScript
// files is a link to a folder outside the repository that holds a leftover. A
// plain run lists the leftover and removes it; through the Root the listing is
// refused, the run fails and the file stays. Planted failure: the listing in
// leftovers, the stale read and the removal in writeGenerated, all as os calls
// on the joined path; any one alone stays green.
func TestGenerateRemovesNothingThroughALinkedFolder(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	for _, tc := range spellings() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := copySchemas(t)
			if code, _, errs := do("-C", dir); code != exitDone {
				t.Fatalf("the control: exit %d\n%s", code, errs)
			}
			at := filepath.Join(dir, filepath.FromSlash(generate.TSDir[:len(generate.TSDir)-1]))
			outside := filepath.Join(t.TempDir(), "ts")
			if err := os.Rename(at, outside); err != nil {
				t.Fatal(err)
			}
			left := filepath.Join(outside, "gone.gen.ts")
			if err := os.WriteFile(left, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			linkOut(t, outside, at, tc.climb)
			code, _, errs := do("-C", dir)
			if code != exitFailed {
				t.Fatalf("exit %d, want %d\n%s", code, exitFailed, errs)
			}
			if _, err := os.Stat(left); err != nil {
				t.Errorf("the leftover outside the repository is gone: %v", err)
			}
		})
	}
}

// TestGenerateReadsEachSchemaFileThroughTheRoot: the schema folder is real and
// one file in it, common.schema.json, is a link to a copy outside the
// repository. The listing sees an ordinary entry; only the read of that file
// can refuse it. A plain read parses the outside copy and writes types made
// from it into the tree; through the Root the run fails. Planted failure: the
// per-file root.ReadFile in readGenerateInput as os.ReadFile on the joined
// path, with the listing left on the Root.
func TestGenerateReadsEachSchemaFileThroughTheRoot(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	for _, tc := range spellings() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := copySchemas(t)
			name := filepath.ToSlash(filepath.Join(generate.SchemaDir, "common.schema.json"))
			outside := moveOut(t, dir, name)
			linkOut(t, outside, filepath.Join(dir, filepath.FromSlash(name)), tc.climb)
			code, _, errs := do("-C", dir)
			if code != exitFailed {
				t.Fatalf("exit %d, want %d\n%s", code, exitFailed, errs)
			}
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(generate.GoDir))); err == nil {
				t.Error("a refused read still wrote the generated files")
			}
		})
	}
}

// moveOut moves the file at name in dir to a place outside the repository and
// returns where it went, leaving nothing at its old place.
func moveOut(t *testing.T, dir, name string) string {
	t.Helper()
	outside := filepath.Join(t.TempDir(), filepath.Base(name))
	if err := os.Rename(filepath.Join(dir, filepath.FromSlash(name)), outside); err != nil {
		t.Fatal(err)
	}
	return outside
}
