package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// gh api --paginate prints GitHub's listing of a repository's releases, one
// JSON array a page, and GitHub is a service this program did not write, so
// what reads it is a parser of foreign input (tests.generated.1): whatever
// the bytes, it does not panic, and answers releases or an error, never
// both; and the draft it names from them is a draft of the tag.

const pages = `[{"id":1,"tag_name":"v1.2.3","draft":false,"assets":[]},{"id":42,"tag_name":"v1.2.3","draft":true,"assets":[{"name":"checksums.txt","digest":"sha256:00"}]}]
[{"id":2,"tag_name":"v1.2.2","draft":true,"assets":null}]
`

// FuzzReadReleases: see above.
func FuzzReadReleases(f *testing.F) {
	for _, s := range []string{pages, ``, `[]`, `{}`, `null`, `[null]`, pages[:len(pages)-4], `[{"id":"7"}]`, `[][]`, "\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		all, err := readReleases([]byte(raw))
		if err != nil && all != nil {
			t.Fatalf("releases %+v beside the error %v", all, err)
		}
		if d, err := theDraft(all, testTag); err == nil && (!d.Draft || d.TagName != testTag) {
			t.Fatalf("named %+v, which is no draft of %s", d, testTag)
		}
	})
}

// TestTheReleaseSeedReachesItsReader is the anti-vacuity check: the seed's two
// pages give three releases and the one draft of the tag, id 42; cut by four
// bytes it is an error, so what the target mutates is read.
func TestTheReleaseSeedReachesItsReader(t *testing.T) {
	all, err := readReleases([]byte(pages))
	if err != nil || len(all) != 3 {
		t.Fatalf("the seed reads %+v, %v; want three releases", all, err)
	}
	if d, err := theDraft(all, testTag); err != nil || d.ID != 42 || len(d.Assets) != 1 || d.Assets[0].Digest != "sha256:00" {
		t.Errorf("the seed's draft of %s is %+v, %v; want id 42 and its one asset", testTag, d, err)
	}
	if all, err := readReleases([]byte(pages[:len(pages)-4])); err == nil {
		t.Errorf("the seed cut reads %+v, want an error", all)
	}
}

// goreleaser writes artifacts.json, and the unpack step reads the path of
// each built binary from it, so what reads it is a parser of input this
// program did not write (tests.generated.1): whatever the bytes, it does not
// panic, answers paths or an error, never both, and every path it answers is
// one of a system asked, inside the release folder.

const builtSeed = `[{"name":"metadata.json","path":"dist/metadata.json","type":"Metadata"},{"name":"gragas","path":"dist/gragas_linux_amd64_v1/gragas","goos":"linux","goarch":"amd64","type":"Binary"},{"name":"gragas","path":"dist/gragas_darwin_arm64_v8.0/gragas","goos":"darwin","goarch":"arm64","type":"Binary"}]`

// builtFolders are the project folder and the release folder inside it the
// fuzz target reads paths against. Both are absolute on every system, as
// builtBinaries makes them, and nothing is written there (a rooted
// "/build/a" has no volume on Windows, so no absolute path was ever inside it).
func builtFolders(tb testing.TB) (repo, dist string) {
	tb.Helper()
	repo, err := filepath.Abs(filepath.Join("build", "a"))
	if err != nil {
		tb.Fatal(err)
	}
	return repo, filepath.Join(repo, "dist")
}

// FuzzReadBuilt: see above.
func FuzzReadBuilt(f *testing.F) {
	for _, s := range []string{builtSeed, ``, `[]`, `{}`, `null`, `[null]`, builtSeed[:len(builtSeed)-4],
		`[{"path":"dist/../x","goos":"linux","goarch":"amd64","type":"Binary"}]`, `[{"path":"/x","goos":"linux","goarch":"amd64","type":"Binary"}]`, "\x00"} {
		f.Add(s)
	}
	repo, dist := builtFolders(f)
	systems := []system{{"linux", "amd64"}, {"darwin", "arm64"}}
	f.Fuzz(func(t *testing.T, raw string) {
		paths, err := readBuilt([]byte(raw), dist, repo, systems)
		if err != nil && paths != nil {
			t.Fatalf("paths %v beside the error %v", paths, err)
		}
		for s, p := range paths {
			r, rerr := filepath.Rel(dist, p)
			if !slices.Contains(systems, s) || rerr != nil || r == "." || strings.HasPrefix(r, "..") {
				t.Fatalf("answered %s for %s, outside %s or for a system not asked", p, s, dist)
			}
		}
	})
}

// TestTheBuiltSeedReachesItsReader is the anti-vacuity check: the seed names
// the two systems' binaries inside the release folder; cut by four bytes it is
// an error, so what the target mutates is read.
func TestTheBuiltSeedReachesItsReader(t *testing.T) {
	repo, dist := builtFolders(t)
	paths, err := readBuilt([]byte(builtSeed), dist, repo, []system{{"linux", "amd64"}, {"darwin", "arm64"}})
	if err != nil || paths[system{"linux", "amd64"}] != filepath.Join(dist, "gragas_linux_amd64_v1", "gragas") || len(paths) != 2 {
		t.Fatalf("the seed reads %v, %v; want the two systems' binaries", paths, err)
	}
	if paths, err := readBuilt([]byte(builtSeed[:len(builtSeed)-4]), dist, repo, []system{{"linux", "amd64"}}); err == nil {
		t.Errorf("the seed cut reads %v, want an error", paths)
	}
}
