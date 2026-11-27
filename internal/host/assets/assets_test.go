package assets_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/assets"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// dataFolders are the engine's data folders the adapter embeds, beside it.
func dataFolders() []string {
	return []string{"handbook", "audit", "profiles", "fixture", "contract/schema", "contract/fixture", "format/schema", "project/schema"}
}

// diskFiles are the files below dir on disk, by slash path from dir, hidden
// folder markers left out. Inputs read: this package's own folder.
func diskFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == ".placeholder" || d.Name() == ".gitkeep" {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestTheAdapterKeepsTheAssetsContractOnAMapOfFiles: the suite that runs on the
// fake runs on the adapter over a folder of known files, one of them in the
// explorer's path.
func TestTheAdapterKeepsTheAssetsContractOnAMapOfFiles(t *testing.T) {
	t.Parallel()
	known := map[string]string{
		"handbook/intro.md":      "# Intro\n",
		"fixture/base/rules.yml": "rules: []\n",
		"explorer/index.html":    "<p>page</p>",
		"adapters/empty.yaml":    "",
	}
	contracttest.Assets(t, known, func(*testing.T) port.Assets {
		return assets.NewFrom(
			fstest.MapFS{
				"handbook/intro.md":      {Data: []byte(known["handbook/intro.md"])},
				"handbook/.placeholder":  {Data: []byte("hidden")},
				"fixture/base/rules.yml": {Data: []byte(known["fixture/base/rules.yml"])},
				"adapters/empty.yaml":    {},
			},
			fstest.MapFS{"index.html": {Data: []byte(known["explorer/index.html"])}},
			fstest.MapFS{".gitkeep": {}},
		)
	})
}

// TestTheAdapterKeepsTheAssetsContractOnTheEmbeddedFiles: the same suite on the
// files this program embeds, over what is on disk beside the adapter.
func TestTheAdapterKeepsTheAssetsContractOnTheEmbeddedFiles(t *testing.T) {
	t.Parallel()
	known := map[string]string{}
	for _, f := range dataFolders() {
		for p, c := range diskFiles(t, f) {
			known[f+"/"+p] = c
		}
	}
	explorer := diskFiles(t, "explorer-copy")
	if len(explorer) == 0 {
		explorer = diskFiles(t, "explorer-placeholder")
	}
	for p, c := range explorer {
		known["explorer/"+p] = c
	}
	contracttest.Assets(t, known, func(*testing.T) port.Assets { return assets.New() })
}

// placeholderAdapter is the adapter as a development build is: the real
// placeholder page and no built copy.
func placeholderAdapter(t *testing.T) assets.Assets {
	t.Helper()
	return assets.NewFrom(fstest.MapFS{}, os.DirFS("explorer-placeholder"), fstest.MapFS{".gitkeep": {}})
}

// TestThePlaceholderIsADataPageWithAMarkerTheReleaseCanRefuse: the page the
// explorer answers without a built copy is a file of HTML, says so in words
// and carries the marker; nothing in its folder is Go.
func TestThePlaceholderIsADataPageWithAMarkerTheReleaseCanRefuse(t *testing.T) {
	t.Parallel()
	page, err := placeholderAdapter(t).Read("explorer/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{assets.PlaceholderMarker, "explorer not built", "<!doctype html>"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("the placeholder lacks %q", want)
		}
	}
	err = filepath.WalkDir("explorer-placeholder", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".go") {
			t.Errorf("%s: code in the placeholder's folder; it is a data page", p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestABuiltCopyReplacesThePlaceholderAndCarriesNoMarker: with a file in the
// marked copy the explorer answers it, the placeholder is gone from the list,
// and no embedded file carries the marker.
func TestABuiltCopyReplacesThePlaceholderAndCarriesNoMarker(t *testing.T) {
	t.Parallel()
	built := fstest.MapFS{".gitkeep": {}, "index.html": {Data: []byte("<p>app</p>")}, "app/main.js": {Data: []byte("1")}}
	a := assets.NewFrom(fstest.MapFS{}, os.DirFS("explorer-placeholder"), built)
	got, err := a.List("explorer")
	if want := []string{"explorer/app/main.js", "explorer/index.html"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("List(explorer) = %v, %v, want %v", got, err, want)
	}
	page, _ := a.Read("explorer/index.html")
	if string(page) != "<p>app</p>" || strings.Contains(string(page), assets.PlaceholderMarker) {
		t.Errorf("the explorer answered %q, want the built copy", page)
	}
}

// TestAFolderWithNothingYetIsEmbeddedWithItsReason: each data folder exists in
// the tree and holds either a real file or a .placeholder that names the
// milestone that fills it; the adapter hides that file.
func TestAFolderWithNothingYetIsEmbeddedWithItsReason(t *testing.T) {
	t.Parallel()
	milestone := regexp.MustCompile(`\bM[0-9]+[ab]?\b`)
	embedded := assets.New()
	for _, f := range dataFolders() {
		if len(diskFiles(t, f)) > 0 {
			continue
		}
		reason, err := os.ReadFile(filepath.Join(f, ".placeholder"))
		if err != nil {
			t.Errorf("%s: no file and no .placeholder to say why: %v", f, err)
			continue
		}
		if !milestone.Match(reason) {
			t.Errorf("%s/.placeholder names no milestone: %q", f, reason)
		}
		if _, err := embedded.Read(f + "/.placeholder"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Read(%s/.placeholder) = %v, want it hidden", f, err)
		}
		if got, err := embedded.List(f); err != nil || len(got) != 0 {
			t.Errorf("List(%s) = %v, %v, want none", f, got, err)
		}
	}
}

// TestAPathThatLeavesTheFolderOrNamesAMarkerIsRefused covers the paths a caller
// builds from outside: they never read or list outside the embedded files.
func TestAPathThatLeavesTheFolderOrNamesAMarkerIsRefused(t *testing.T) {
	t.Parallel()
	a := assets.New()
	for _, p := range []string{"", ".", "../x", "/repo/x", "a//b", "explorer/../go.mod", "explorer/.gitkeep", "explorer", "handbook/.placeholder", "explorer-placeholder/index.html", "explorer-copy/.gitkeep"} {
		if b, err := a.Read(p); !errors.Is(err, fs.ErrNotExist) || b != nil {
			t.Errorf("Read(%q) = %q, %v, want fs.ErrNotExist", p, b, err)
		}
	}
	for _, d := range []string{"..", "../x", "/", "a/../..", "a//b"} {
		if got, err := a.List(d); err == nil {
			t.Errorf("List(%q) = %v, want an error", d, got)
		}
	}
}

// FuzzReadAndListNeverEscape: any path a caller hands is answered or refused,
// and a path that is answered is a valid slash path.
func FuzzReadAndListNeverEscape(f *testing.F) {
	for _, s := range []string{"", ".", "..", "explorer/index.html", "explorer", "../x", "handbook/.placeholder", "a\x00b", "a//b"} {
		f.Add(s)
	}
	a := assets.New()
	f.Fuzz(func(t *testing.T, p string) {
		if b, err := a.Read(p); err == nil && !fs.ValidPath(p) {
			t.Errorf("Read(%q) answered %q for a path that is not valid", p, b)
		}
		if got, err := a.List(p); err == nil {
			for _, g := range got {
				if !fs.ValidPath(g) {
					t.Errorf("List(%q) answered the invalid path %q", p, g)
				}
			}
		}
	})
}

// TestTheFixtureRulebooksAreEmbeddedAndListed: the conformance suite is in the
// program, below fixture/, with the files that start with a dot (a project's
// pin, lock and tool configuration), and a read answers the bytes on disk. The
// control is a file the fixture does not hold.
func TestTheFixtureRulebooksAreEmbeddedAndListed(t *testing.T) {
	t.Parallel()
	embedded := assets.New()
	listed, err := embedded.List("fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"fixture/README.md", "fixture/base/rulebook.yaml", "fixture/base/history.jsonl", "fixture/other/rulebook.yaml",
		"fixture/projects/clean/.gragas/pin.yaml", "fixture/projects/weakened/.gragas/lock.yaml", "fixture/projects/clean/.golangci.yml",
	} {
		if !slices.Contains(listed, want) {
			t.Errorf("List(fixture) lacks %s", want)
			continue
		}
		got, err := embedded.Read(want)
		disk, derr := os.ReadFile(want)
		if derr != nil || err != nil || string(got) != string(disk) {
			t.Errorf("Read(%s) = %d bytes, %v; on disk %d bytes, %v", want, len(got), err, len(disk), derr)
		}
	}
	if slices.Contains(listed, "fixture/.placeholder") {
		t.Error("the folder has files, so its marker is not listed")
	}
	if _, err := embedded.Read("fixture/base/no-such-file.yaml"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("control: Read of a file the fixture does not hold = %v", err)
	}
}
