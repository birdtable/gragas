package contracttest

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// BundlesSetup is the Bundles under test over one root with no lock in it
// yet, and the three ways a test changes what lies there.
type BundlesSetup struct {
	Bundles port.Bundles
	// Root is the root the suite lists and reads.
	Root string
	// Put installs a bundle of these two files and the other files, each by
	// its path below the bundle's folder, under ref at Root, and locks it at
	// its digest as the lock's writer would take it.
	Put func(t *testing.T, ref model.PluginRef, manifest, module []byte, other ...model.File)
	// Change rewrites the module of the bundle Put under ref and leaves the
	// lock as it was.
	Change func(t *testing.T, ref model.PluginRef, module []byte)
	// Add writes one more regular file into the folder of the bundle Put
	// under ref and leaves the lock as it was.
	Add func(t *testing.T, ref model.PluginRef, f model.File)
}

// BundlesMaker makes the setup for one subtest.
type BundlesMaker func(t *testing.T) BundlesSetup

// Bundles runs the Bundles suite against the port build answers.
func Bundles(t *testing.T, build BundlesMaker) {
	t.Helper()
	bundlesList(t, build)
	bundlesRead(t, build)
	bundlesOther(t, build)
	bundlesChanged(t, build)
	bundlesMissing(t, build)
	bundlesEnded(t, build)
}

// digestForm is the digest's form: "sha256:" and sixty-four lower-case
// hexadecimal digits.
var digestForm = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// sumOf is the one digest of b's files (pluginbundle.Digest).
func sumOf(b model.PluginBundle) model.Digest {
	return pluginbundle.Digest(append([]model.File{{Path: pluginbundle.ManifestFile, Data: b.Manifest}, {Path: pluginbundle.ModuleFile, Data: b.Module}}, b.Other...))
}

// fixtures is the other files of a bundle that carries its fixtures, out of
// the order of their paths.
func fixtures() []model.File {
	return []model.File{{Path: "testdata/b/want.json", Data: []byte("{}\n")}, {Path: "README", Data: []byte("readme\n")}, {Path: "testdata/a/want.json", Data: []byte("[]\n")}}
}

// refOf is the ref of name at version.
func refOf(name, version string) model.PluginRef {
	return model.PluginRef{Name: name, Version: version}
}

// readBundle is Read's answer, failing the test on an error.
func readBundle(t *testing.T, s BundlesSetup, ref model.PluginRef) model.PluginBundle {
	t.Helper()
	b, err := s.Bundles.Read(t.Context(), s.Root, ref)
	if err != nil {
		t.Fatalf("Read(%v) = %v", ref, err)
	}
	return b
}

// bundlesList: List is the lock's refs by name and then version as bytes,
// and none is a nil list.
func bundlesList(t *testing.T, build BundlesMaker) {
	t.Helper()
	t.Run("a root with no bundle lists none, with no error", func(t *testing.T) {
		s := build(t)
		got, err := s.Bundles.List(t.Context(), s.Root)
		if got != nil || err != nil {
			t.Errorf("List = %v, %v, want a nil list and no error", got, err)
		}
	})
	t.Run("the list is by name and then version, as bytes", func(t *testing.T) {
		s := build(t)
		for _, r := range []model.PluginRef{refOf("b", "1.0.0"), refOf("a", "1.9.0"), refOf("a", "1.10.0")} {
			s.Put(t, r, []byte("manifest of "+r.Name+r.Version), []byte("module of "+r.Name+r.Version))
		}
		got, err := s.Bundles.List(t.Context(), s.Root)
		want := []model.PluginRef{refOf("a", "1.10.0"), refOf("a", "1.9.0"), refOf("b", "1.0.0")}
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("List = %v, %v, want %v", got, err, want)
		}
	})
}

// bundlesRead: Read gives the files as they were put and the two digests
// equal, each pluginbundle.Digest of those files, and two bundles two
// digests.
func bundlesRead(t *testing.T, build BundlesMaker) {
	t.Helper()
	t.Run("a bundle reads as it was put, its digest as read the lock's", func(t *testing.T) {
		s := build(t)
		r := refOf("standin", "1.0.0")
		s.Put(t, r, []byte("contract: 1\n"), []byte("\x00asm\x01\x00\x00\x00"))
		b := readBundle(t, s, r)
		if !bytes.Equal(b.Manifest, []byte("contract: 1\n")) || !bytes.Equal(b.Module, []byte("\x00asm\x01\x00\x00\x00")) || b.Other != nil {
			t.Errorf("Read = manifest %q, module %q, other %v, want the files as put", b.Manifest, b.Module, b.Other)
		}
		if !digestForm.MatchString(string(b.Locked)) || b.Read != b.Locked {
			t.Errorf("Read's digests = locked %q, read %q, want one digest in its form", b.Locked, b.Read)
		}
		if want := sumOf(b); b.Read != want {
			t.Errorf("Read's digest = %s, want pluginbundle.Digest of its files, %s", b.Read, want)
		}
	})
	t.Run("a bundle's bytes are the caller's to change", func(t *testing.T) {
		s := build(t)
		r := refOf("standin", "1.0.0")
		s.Put(t, r, []byte("contract: 1\n"), []byte("module"), model.File{Path: "README", Data: []byte("readme")})
		b := readBundle(t, s, r)
		if len(b.Other) != 1 || len(b.Other[0].Data) == 0 {
			t.Fatalf("Read's other files = %v, want the files as put", b.Other)
		}
		b.Manifest[0], b.Module[0], b.Other[0].Data[0] = 'X', 'X', 'X'
		if again := readBundle(t, s, r); again.Manifest[0] != 'c' || again.Module[0] != 'm' || len(again.Other) != 1 || again.Other[0].Data[0] != 'r' {
			t.Errorf("a change to what Read gave changed the next Read: %q, %q, %q", again.Manifest, again.Module, again.Other[0].Data)
		}
	})
}

// bundlesOther: a bundle's other files read as put, in the order of their
// paths, under its digest, and a bundle with one more file has another.
func bundlesOther(t *testing.T, build BundlesMaker) {
	t.Helper()
	t.Run("a bundle's other files read as put, by path, under its digest", func(t *testing.T) {
		s := build(t)
		r := refOf("standin", "1.0.0")
		s.Put(t, r, []byte("contract: 1\n"), []byte("module"), fixtures()...)
		b := readBundle(t, s, r)
		want := slices.Clone(fixtures())
		slices.SortFunc(want, func(a, b model.File) int { return strings.Compare(a.Path, b.Path) })
		if !slices.EqualFunc(b.Other, want, func(a, b model.File) bool { return a.Path == b.Path && bytes.Equal(a.Data, b.Data) }) {
			t.Errorf("Read's other files = %v, want %v, in the order of their paths", b.Other, want)
		}
		if sum := sumOf(b); b.Read != sum || b.Locked != sum {
			t.Errorf("Read's digests = locked %s, read %s, want pluginbundle.Digest of every file, %s", b.Locked, b.Read, sum)
		}
	})
	t.Run("two bundles that differ by one other file have two digests", func(t *testing.T) {
		s := build(t)
		s.Put(t, refOf("standin", "1.0.0"), []byte("contract: 1\n"), []byte("module"))
		s.Put(t, refOf("standin", "1.0.1"), []byte("contract: 1\n"), []byte("module"), model.File{Path: "testdata/x", Data: nil})
		one, two := readBundle(t, s, refOf("standin", "1.0.0")), readBundle(t, s, refOf("standin", "1.0.1"))
		if one.Locked == two.Locked {
			t.Errorf("a bundle and the bundle with one more file have the one digest %s", one.Locked)
		}
	})
	t.Run("two bundles that differ by one byte of their module have two digests", func(t *testing.T) {
		s := build(t)
		s.Put(t, refOf("standin", "1.0.0"), []byte("contract: 1\n"), []byte("module 1"))
		s.Put(t, refOf("standin", "1.0.1"), []byte("contract: 1\n"), []byte("module 2"))
		one, two := readBundle(t, s, refOf("standin", "1.0.0")), readBundle(t, s, refOf("standin", "1.0.1"))
		if one.Locked == two.Locked {
			t.Errorf("two bundles have the one digest %s", one.Locked)
		}
	})
}

// bundlesChanged: a module changed after its lock, or a file added to its
// folder, is read as it is now, with the lock's digest kept and the digest as
// read another, never refused here: the opener refuses it.
func bundlesChanged(t *testing.T, build BundlesMaker) {
	t.Helper()
	t.Run("a module changed after its lock reads as it is, the two digests apart", func(t *testing.T) {
		s := build(t)
		r := refOf("standin", "1.0.0")
		s.Put(t, r, []byte("contract: 1\n"), []byte("module as locked"))
		before := readBundle(t, s, r)
		s.Change(t, r, []byte("module as changed"))
		after := readBundle(t, s, r)
		if !bytes.Equal(after.Module, []byte("module as changed")) {
			t.Errorf("Read's module = %q, want the module as changed", after.Module)
		}
		if after.Locked != before.Locked {
			t.Errorf("the lock's digest moved from %s to %s with the module", before.Locked, after.Locked)
		}
		if after.Read == after.Locked || !digestForm.MatchString(string(after.Read)) {
			t.Errorf("the digest as read is %q beside the lock's %q, want another digest in its form", after.Read, after.Locked)
		}
	})
	t.Run("a file added after its lock reads as there, the two digests apart", func(t *testing.T) {
		s := build(t)
		r := refOf("standin", "1.0.0")
		s.Put(t, r, []byte("contract: 1\n"), []byte("module"))
		before := readBundle(t, s, r)
		s.Add(t, r, model.File{Path: "testdata/added", Data: []byte("added")})
		after := readBundle(t, s, r)
		if len(after.Other) != 1 || after.Other[0].Path != "testdata/added" {
			t.Errorf("Read's other files = %v, want the file added", after.Other)
		}
		if after.Locked != before.Locked || after.Read == after.Locked || after.Read != sumOf(after) {
			t.Errorf("after a file was added the digests are locked %s, read %s, want the lock's kept and the read the sum of every file, %s", after.Locked, after.Read, sumOf(after))
		}
	})
}

// bundlesMissing: a ref the lock does not name is ErrNotInstalled.
func bundlesMissing(t *testing.T, build BundlesMaker) {
	t.Helper()
	t.Run("a ref the lock does not name matches ErrNotInstalled", func(t *testing.T) {
		s := build(t)
		s.Put(t, refOf("standin", "1.0.0"), []byte("contract: 1\n"), []byte("module"))
		for _, r := range []model.PluginRef{refOf("other", "1.0.0"), refOf("standin", "2.0.0")} {
			if _, err := s.Bundles.Read(t.Context(), s.Root, r); !errors.Is(err, port.ErrNotInstalled) {
				t.Errorf("Read(%v) = %v, want port.ErrNotInstalled", r, err)
			}
		}
	})
}

// bundlesEnded: an ended context ends each method with its own error.
func bundlesEnded(t *testing.T, build BundlesMaker) {
	t.Helper()
	t.Run("an ended context ends List and Read with the context's error", func(t *testing.T) {
		s := build(t)
		r := refOf("standin", "1.0.0")
		s.Put(t, r, []byte("contract: 1\n"), []byte("module"))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := s.Bundles.List(ctx, s.Root); !errors.Is(err, context.Canceled) {
			t.Errorf("List with an ended context = %v, want context.Canceled", err)
		}
		_, err := s.Bundles.Read(ctx, s.Root, r)
		if !errors.Is(err, context.Canceled) || errors.Is(err, port.ErrNotInstalled) {
			t.Errorf("Read with an ended context = %v, want context.Canceled and no sentinel", err)
		}
	})
}
