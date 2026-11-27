package fs_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

func testLimits() hostfs.Limits {
	return hostfs.Limits{Module: 1024, File: 256, Total: 4096, Lock: 4096}
}

func theStandin() model.PluginRef { return model.PluginRef{Name: "standin", Version: "1.0.0"} }

func lockOf(d model.Digest) string {
	return "version: 1\nplugins:\n  - name: standin\n    version: 1.0.0\n    source: file:standin\n    digest: " + string(d) + "\n"
}

const aDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

// folderAt is the folder of ref under dir.
func folderAt(dir string, ref model.PluginRef) string {
	return filepath.Join(dir, "plugins", ref.Name, ref.Version)
}

// put installs a bundle of these files under ref at dir and locks it at its
// digest, as the lock's writer takes it.
func put(t *testing.T, dir string, ref model.PluginRef, manifest, module []byte, other ...model.File) {
	t.Helper()
	files := append([]model.File{{Path: pluginbundle.ManifestFile, Data: manifest}, {Path: pluginbundle.ModuleFile, Data: module}}, other...)
	for _, f := range files {
		write(t, folderAt(dir, ref), map[string]string{f.Path: string(f.Data)})
	}
	lock := filepath.Join(dir, "plugins", "lock.yaml")
	var entries []hostfs.LockEntry
	if raw, err := os.ReadFile(lock); err == nil {
		if entries, err = hostfs.DecodePluginLock(raw); err != nil {
			t.Fatal(err)
		}
	}
	entries = append(entries, hostfs.LockEntry{Ref: ref, Source: "file:" + ref.Name, Digest: pluginbundle.Digest(files)})
	write(t, dir, map[string]string{"plugins/lock.yaml": string(hostfs.EncodePluginLock(entries))})
}

// bundlesIn is the Bundles over a new tree, within limits.
func bundlesIn(t *testing.T, limits hostfs.Limits) hostfs.Bundles {
	t.Helper()
	return hostfs.NewBundles(newTree(t), limits)
}

func TestBundlesAreAPort(t *testing.T) {
	t.Parallel()
	contracttest.Bundles(t, func(t *testing.T) contracttest.BundlesSetup {
		t.Helper()
		dir := t.TempDir()
		return contracttest.BundlesSetup{
			Bundles: bundlesIn(t, testLimits()),
			Root:    dir,
			Put: func(t *testing.T, ref model.PluginRef, manifest, module []byte, other ...model.File) {
				t.Helper()
				put(t, dir, ref, manifest, module, other...)
			},
			Change: func(t *testing.T, ref model.PluginRef, module []byte) {
				t.Helper()
				write(t, folderAt(dir, ref), map[string]string{"plugin.wasm": string(module)})
			},
			Add: func(t *testing.T, ref model.PluginRef, f model.File) {
				t.Helper()
				write(t, folderAt(dir, ref), map[string]string{f.Path: string(f.Data)})
			},
		}
	})
}

func TestTheLockIsStrict(t *testing.T) {
	t.Parallel()
	ok := lockOf(aDigest)
	bad := map[string]string{
		"an unknown key":          strings.Replace(ok, "version: 1\n", "version: 1\nextra: 1\n", 1),
		"a version not whole":     strings.Replace(ok, "version: 1\n", "version: one\n", 1),
		"a newer version":         strings.Replace(ok, "version: 1\n", "version: 2\n", 1),
		"a digest of a bad form":  strings.Replace(ok, aDigest, "sha256:abc", 1),
		"a name twice":            ok + strings.SplitN(ok, "plugins:\n", 2)[1],
		"an entry key unknown":    strings.Replace(ok, "    source", "    other: 1\n    source", 1),
		"a name of a bad form":    strings.Replace(ok, "name: standin", "name: Stand_In", 1),
		"a version of a bad form": strings.Replace(ok, "version: 1.0.0", "version: 1.0", 1),
		"an unsorted list":        strings.Replace(ok, "plugins:\n", "plugins:\n  - name: zeta\n    version: 1.0.0\n    source: s\n    digest: "+aDigest+"\n", 1),
		"an anchor":               strings.Replace(ok, "version: 1\n", "version: &a 1\n", 1),
	}
	for name, text := range bad {
		if _, err := hostfs.DecodePluginLock([]byte(text)); err == nil {
			t.Errorf("%s was read", name)
		}
	}
	// The two refusals that read nothing again name the entry and the line.
	for name, want := range map[string][]string{
		"a name twice":     {"standin@1.0.0 is listed twice", "line"},
		"an unsorted list": {"standin@1.0.0 is out of order", "line"},
	} {
		_, err := hostfs.DecodePluginLock([]byte(bad[name]))
		for _, w := range want {
			if err == nil || !strings.Contains(err.Error(), w) {
				t.Errorf("%s: the refusal is %v, want it to hold %q", name, err, w)
			}
		}
	}
	got, err := hostfs.DecodePluginLock([]byte(ok))
	if err != nil || len(got) != 1 || got[0].Ref != theStandin() {
		t.Fatalf("the good lock: %v, %v", got, err)
	}
	written := hostfs.EncodePluginLock(got)
	back, err := hostfs.DecodePluginLock(written)
	if err != nil || len(back) != 1 || back[0] != got[0] {
		t.Errorf("the lock written is not read back: %v, %v\n%s", back, err, written)
	}
	if string(hostfs.EncodePluginLock(back)) != string(written) {
		t.Error("the lock is written two ways")
	}
}

func TestListNamesWhatTheLockHolds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	put(t, dir, theStandin(), []byte("m"), []byte("w"))
	// A folder the lock does not name is no plugin.
	write(t, dir, map[string]string{"plugins/other/1.0.0/plugin.yaml": "name: other\n"})
	b := bundlesIn(t, testLimits())
	got, err := b.List(context.Background(), dir)
	if err != nil || len(got) != 1 || got[0] != theStandin() {
		t.Fatalf("List = %v, %v", got, err)
	}
	// An entry with no folder is an error that names it.
	lone := t.TempDir()
	write(t, lone, map[string]string{"plugins/lock.yaml": lockOf(aDigest)})
	_, err = b.List(context.Background(), lone)
	if err == nil || !strings.Contains(err.Error(), "standin@1.0.0") || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("List of an entry with no folder = %v", err)
	}
}

func TestAnAmbiguousNameIsRefusedAndNoVersionIsPicked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	two := lockOf(aDigest) + "  - name: standin\n    version: 1.0.1\n    source: file:standin\n    digest: " + aDigest + "\n"
	write(t, dir, map[string]string{"plugins/lock.yaml": two})
	b := bundlesIn(t, testLimits())
	ref, err := b.Find(t.Context(), dir, "standin")
	if err == nil || ref != (model.PluginRef{}) || !strings.Contains(err.Error(), "standin@1.0.0 and standin@1.0.1") {
		t.Errorf("Find = %v, %v: want a refusal naming both and no pick", ref, err)
	}
	if _, err := b.Find(t.Context(), dir, "absent"); !errors.Is(err, port.ErrNotInstalled) {
		t.Errorf("Find of a name not held = %v", err)
	}
	one := t.TempDir()
	write(t, one, map[string]string{"plugins/lock.yaml": lockOf(aDigest)})
	if ref, err := b.Find(t.Context(), one, "standin"); err != nil || ref != theStandin() {
		t.Errorf("Find of one version = %v, %v", ref, err)
	}
}

// TestReadRefusesWhatItCannotReadAsTheBundlesOwn: each row makes one bundle
// the reader refuses, with the file named; the bound rows are one byte over.
func TestReadRefusesWhatItCannotReadAsTheBundlesOwn(t *testing.T) {
	t.Parallel()
	limits := hostfs.Limits{Module: 100, File: 50, Total: 70000, Lock: 4096}
	tight := hostfs.Limits{Module: 100, File: 50, Total: 160, Lock: 4096}
	ref := theStandin()
	rows := map[string]struct {
		edit func(t *testing.T, dir string)
		want string
	}{
		"a module a byte over its bound": {func(t *testing.T, dir string) {
			t.Helper()
			write(t, folderAt(dir, ref), map[string]string{"plugin.wasm": strings.Repeat("m", 101)})
		}, "plugin.wasm"},
		"a manifest a byte over its bound": {func(t *testing.T, dir string) {
			t.Helper()
			write(t, folderAt(dir, ref), map[string]string{"plugin.yaml": strings.Repeat("y", pluginbundle.MaxManifest+1)})
		}, "plugin.yaml"},
		"another file a byte over its bound": {func(t *testing.T, dir string) {
			t.Helper()
			write(t, folderAt(dir, ref), map[string]string{"testdata/big": strings.Repeat("o", 51)})
		}, "testdata/big"},
		"a bundle a byte over its total": {func(t *testing.T, dir string) {
			t.Helper()
			// 10 + 100 + 50 = 160 holds; one more byte of the last file does not.
			write(t, folderAt(dir, ref), map[string]string{"plugin.yaml": strings.Repeat("y", 10), "plugin.wasm": strings.Repeat("m", 100), "testdata/a": strings.Repeat("o", 50), "testdata/b": "x"})
		}, "a bundle may hold"},
		"a link to a file inside the root": func() struct {
			edit func(t *testing.T, dir string)
			want string
		} {
			return struct {
				edit func(t *testing.T, dir string)
				want string
			}{func(t *testing.T, dir string) {
				t.Helper()
				write(t, dir, map[string]string{"elsewhere.txt": "x"})
				testenv.Link(t, filepath.Join(dir, "elsewhere.txt"), filepath.Join(folderAt(dir, ref), "linked.txt"))
			}, "linked.txt"}
		}(),
		"a hard link to a file out of the root": {func(t *testing.T, dir string) {
			t.Helper()
			outside := filepath.Join(t.TempDir(), "secret.txt")
			if err := os.WriteFile(outside, []byte("the secret"), 0o600); err != nil {
				t.Fatal(err)
			}
			write(t, folderAt(dir, ref), map[string]string{"testdata/keep": "k"})
			if err := os.Link(outside, filepath.Join(folderAt(dir, ref), "testdata", "x.txt")); err != nil {
				t.Fatal(err)
			}
		}, "x.txt"},
		"the lock a hard link to a file out of the root": {func(t *testing.T, dir string) {
			t.Helper()
			lock := filepath.Join(dir, "plugins", "lock.yaml")
			raw, err := os.ReadFile(lock)
			if err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "lock.yaml")
			if err := os.WriteFile(outside, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(lock); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(outside, lock); err != nil {
				t.Fatal(err)
			}
		}, "lock.yaml"},
		"the version folder linked out of the root": {func(t *testing.T, dir string) {
			t.Helper()
			elsewhere := t.TempDir()
			write(t, elsewhere, map[string]string{"plugin.yaml": "m", "plugin.wasm": "w"})
			folder := folderAt(dir, ref)
			if err := os.RemoveAll(folder); err != nil {
				t.Fatal(err)
			}
			testenv.Link(t, elsewhere, folder)
		}, ""},
	}
	for name, row := range rows {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			put(t, dir, ref, []byte("m"), []byte("w"))
			row.edit(t, dir)
			l := limits
			if strings.HasPrefix(name, "a bundle a byte over its total") {
				l = tight
			}
			_, err := bundlesIn(t, l).Read(t.Context(), dir, ref)
			if err == nil || !strings.Contains(err.Error(), row.want) {
				t.Errorf("Read = %v, want a refusal naming %q", err, row.want)
			}
		})
	}
}
