package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/go-files/filestest"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/digest"
)

// tarGz is an archive of one regular file.
func tarGz(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pinnedRepo is a folder holding a tools.yaml that pins program, in archive,
// for the platform plat.
func pinnedRepo(t *testing.T, plat string, program, archive []byte) string {
	t.Helper()
	dir := t.TempDir()
	yml := "tools:\n  - id: lint\n    from: release\n    platforms:\n      " + plat + ":\n        url: https://example.invalid/lint.tar.gz\n" +
		"        archive: " + digest.Hex(archive) + "\n        member: lint-1/lint\n        sha256: " + digest.Hex(program) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "tools.yaml"), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func fetches(archive []byte, n *int) func(context.Context, string) ([]byte, error) {
	return func(context.Context, string) ([]byte, error) { *n++; return archive, nil }
}

// TestAFetchPlacesTheProgramThePinNamesAndRefusesAnyOther (gragas#911): the
// archive and the program are each held to tools.yaml's sha256 before a byte
// is placed in .tools/bin. A program of another sha256, or an archive of
// another, is refused and never placed; a program already pinned is not
// fetched again; --check fetches nothing.
func TestAFetchPlacesTheProgramThePinNamesAndRefusesAnyOther(t *testing.T) {
	t.Parallel()
	program := []byte("the pinned program")
	archive := tarGz(t, "lint-1/lint", program)
	talk := &say.Speaker{W: &bytes.Buffer{}}
	dst := func(dir string) string { return filepath.Join(dir, ".tools", "bin", "lint") }

	dir := pinnedRepo(t, "linux/amd64", program, archive)
	if err := fetchTool(t.Context(), fetchJob{dir: dir, id: "lint", platform: "linux/amd64", check: true, get: nil}, talk); err == nil {
		t.Error("--check with nothing in .tools/bin passed")
	}
	var calls int
	if err := fetchTool(t.Context(), fetchJob{dir: dir, id: "lint", platform: "linux/amd64", check: false, get: fetches(archive, &calls)}, talk); err != nil {
		t.Fatalf("a fetch of the pinned archive: %v", err)
	}
	got, err := os.ReadFile(dst(dir))
	if err != nil || !bytes.Equal(got, program) {
		t.Fatalf("placed %q, %v; want the pinned program", got, err)
	}
	if err := fetchTool(t.Context(), fetchJob{dir: dir, id: "lint", platform: "linux/amd64", check: false, get: fetches(archive, &calls)}, talk); err != nil || calls != 1 {
		t.Errorf("a second fetch: %v after %d fetches, want none more", err, calls)
	}
	if err := fetchTool(t.Context(), fetchJob{dir: dir, id: "lint", platform: "linux/amd64", check: true, get: nil}, talk); err != nil {
		t.Errorf("--check on the pinned program: %v", err)
	}

	// A planted program of the wrong sha256 inside an archive that is the pinned one's.
	wrong := tarGz(t, "lint-1/lint", []byte("a program of another build"))
	plant := pinnedRepo(t, "linux/amd64", program, archive)
	err = fetchTool(t.Context(), fetchJob{dir: plant, id: "lint", platform: "linux/amd64", get: fetches(wrong, &calls)}, talk)
	if err == nil || !strings.Contains(err.Error(), "the archive has sha256") {
		t.Errorf("an archive of the wrong sha256: %v, want it refused", err)
	}
	// A pinned archive whose program is of the wrong sha256.
	badProgram := pinnedRepo(t, "linux/amd64", []byte("another program"), archive)
	err = fetchTool(t.Context(), fetchJob{dir: badProgram, id: "lint", platform: "linux/amd64", check: false, get: fetches(archive, &calls)}, talk)
	if err == nil || !strings.Contains(err.Error(), "has sha256") {
		t.Errorf("a program of the wrong sha256: %v, want it refused", err)
	}
	for _, d := range []string{plant, badProgram} {
		if _, err := os.Stat(dst(d)); err == nil {
			t.Errorf("a refused program was placed in %s", d)
		}
	}
	// A stale program in .tools/bin is red under --check, and a fetch replaces it.
	filestest.WriteProgram(t, dst(dir), []byte("stale"))
	if err := fetchTool(t.Context(), fetchJob{dir: dir, id: "lint", platform: "linux/amd64", check: true, get: nil}, talk); err == nil {
		t.Error("--check on a stale program passed")
	}
	if err := fetchTool(t.Context(), fetchJob{dir: dir, id: "lint", platform: "linux/amd64", check: false, get: fetches(archive, &calls)}, talk); err != nil {
		t.Errorf("a fetch over a stale program: %v", err)
	}
	if got, _ := os.ReadFile(dst(dir)); !bytes.Equal(got, program) {
		t.Errorf("the stale program was not replaced: %q", got)
	}
	// A platform the file does not pin, a tool it does not hold.
	for _, c := range []struct{ id, plat string }{{"lint", "plan9/arm"}, {"nope", "linux/amd64"}} {
		if err := fetchTool(t.Context(), fetchJob{dir: dir, id: c.id, platform: c.plat, check: true}, talk); err == nil {
			t.Errorf("%s on %s passed", c.id, c.plat)
		}
	}
}

// TestAToolThatIsNotAReleaseHasNoPinToFetch: a tool of tools.yaml that is built
// from the lock (from: go) is refused, not fetched.
func TestAToolThatIsNotAReleaseHasNoPinToFetch(t *testing.T) {
	t.Parallel()
	yml := "tools:\n  - id: gt\n    from: go\n    platforms:\n      linux/amd64:\n        url: https://example.invalid/gt.tar.gz\n        archive: a\n        member: gt\n        sha256: b\n"
	if _, err := readToolPin([]byte(yml), "gt", "linux/amd64"); err == nil || !strings.Contains(err.Error(), "not a release tool") {
		t.Errorf("a go tool: err %v, want not a release tool", err)
	}
}

// TestTheRealToolsFilePinsGolangciLintForThisPlatform: tools.yaml, the one
// pin, names a complete release program of golangci-lint for each platform
// the fetch runs on, and of ast-grep, which ships as a zip, of node and
// pnpm, windows/amd64 among them, and of vale.
func TestTheRealToolsFilePinsGolangciLintForThisPlatform(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, plat := range []string{"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64", "windows/amd64"} {
		for _, id := range []string{"golangci-lint", "ast-grep", "node", "pnpm", "vale"} {
			if _, err := readToolPin(data, id, plat); err != nil {
				t.Errorf("%s %s: %v", id, plat, err)
			}
		}
	}
}

// zipOf is a zip archive of one program, named name, of mode 0755.
func zipOf(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	h.SetMode(0o755)
	return zipWith(t, h, body)
}

// zipLink is a zip archive of one link, named name, whose target is body.
func zipLink(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	h.SetMode(fs.ModeSymlink)
	return zipWith(t, h, body)
}

// zipWith is a zip archive of the one entry h, holding body.
func zipWith(t *testing.T, h *zip.FileHeader, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// zipClaiming is a zip of one stored file whose header claims size bytes.
func zipClaiming(t *testing.T, name string, size uint64) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: name, Method: zip.Store, UncompressedSize64: size, CompressedSize64: 1}
	h.SetMode(0o755)
	w, err := zw.CreateRaw(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// windowsRepo is a folder holding a tools.yaml that pins program, as member
// of archive fetched from url, for windows/amd64.
func windowsRepo(t *testing.T, url, member string, program, archive []byte) string {
	t.Helper()
	dir := t.TempDir()
	yml := "tools:\n  - id: lint\n    from: release\n    platforms:\n      windows/amd64:\n        url: " + url + "\n" +
		"        archive: " + digest.Hex(archive) + "\n        member: " + member + "\n        sha256: " + digest.Hex(program) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "tools.yaml"), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestAWindowsZipIsFetchedFromItsFolderAndAnyOtherIsRefused (gragas#1088): a
// windows pin is a zip whose program is an .exe, at its top or in a folder; it
// is held to both sums like a .tar.gz and placed as the id and .exe. A planted
// zip of the wrong sum, a program of the wrong sum, a member that is missing,
// is not a regular file or claims more than maxProgram bytes, and an archive
// that is neither form are each refused, and nothing is placed.
func TestAWindowsZipIsFetchedFromItsFolderAndAnyOtherIsRefused(t *testing.T) {
	t.Parallel()
	const url = "https://example.invalid/lint-windows-amd64.zip"
	program := []byte("the pinned windows program")
	talk := &say.Speaker{W: &bytes.Buffer{}}
	name := programName("lint", "windows/amd64")
	for _, member := range []string{"lint-1-windows-amd64/" + name, name} {
		archive := zipOf(t, member, program)
		dir := windowsRepo(t, url, member, program, archive)
		var calls int
		if err := fetchTool(t.Context(), fetchJob{dir: dir, id: "lint", platform: "windows/amd64", check: false, get: fetches(archive, &calls)}, talk); err != nil {
			t.Fatalf("%s: a fetch of the pinned zip: %v", member, err)
		}
		got, err := os.ReadFile(filepath.Join(dir, ".tools", "bin", name))
		if err != nil || !bytes.Equal(got, program) {
			t.Errorf("%s: placed %q, %v; want the pinned program as %s", member, got, err, name)
		}
		if _, err := os.Stat(filepath.Join(dir, ".tools", "bin", "lint")); err == nil {
			t.Errorf("%s: a windows program was placed as lint, not as %s", member, name)
		}
		if err := fetchTool(t.Context(), fetchJob{dir: dir, id: "lint", platform: "windows/amd64", check: true, get: nil}, talk); err != nil {
			t.Errorf("%s: --check on the pinned program: %v", member, err)
		}
	}

	member := "lint-1-windows-amd64/" + name
	pinned := zipOf(t, member, program)
	for claim, c := range map[string]struct {
		url             string
		pinArchive, got []byte
		pinProgram      []byte
		says            string
	}{
		"a planted zip of the wrong sha256 is refused":                 {url, pinned, zipOf(t, member, []byte("another build")), program, "the archive has sha256"},
		"a pinned zip whose program is of the wrong sha256 is refused": {url, pinned, pinned, []byte("another program"), member + " has sha256"},
		"a zip without the member is refused":                          {url, zipOf(t, name, program), nil, program, "no such file"},
		"a member that is a link is refused":                           {url, zipLink(t, member, program), nil, program, "not a regular file"},
		"a member whose header claims too many bytes is refused":       {url, zipClaiming(t, member, maxProgram+1), nil, program, "not a regular file"},
		"an archive that is neither a .tar.gz nor a .zip is refused":   {"https://example.invalid/lint.7z", pinned, nil, program, "neither"},
		"a .tar.gz served for a .zip pin is refused":                   {url, tarGz(t, member, program), nil, program, "zip"},
	} {
		served := c.got
		if served == nil {
			served = c.pinArchive
		}
		dir := windowsRepo(t, c.url, member, c.pinProgram, c.pinArchive)
		var calls int
		err := fetchTool(t.Context(), fetchJob{dir: dir, id: "lint", platform: "windows/amd64", check: false, get: fetches(served, &calls)}, talk)
		if err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want it refused saying %q", claim, err, c.says)
		}
		if _, err := os.Stat(filepath.Join(dir, ".tools", "bin", name)); err == nil {
			t.Errorf("%s: a refused program was placed", claim)
		}
	}
}

// windowsGaps are the tools of a tools.yaml text with no windows/amd64 pin and
// no no-windows reason, and the release tools whose windows/amd64 pin is not a
// complete .zip pin of an .exe.
func windowsGaps(data []byte) []string {
	var gaps []string
	why := regexp.MustCompile(`\n    no-windows: "[^"\n]+"`)
	for block := range strings.SplitSeq(string(data), "\n  - id: ") {
		id, rest, ok := strings.Cut(block, "\n")
		if !ok || strings.HasPrefix(block, "#") || strings.Contains(id, ":") {
			continue
		}
		pinned := strings.Contains(rest, "\n      windows/amd64:")
		reason := why.MatchString(rest)
		switch {
		case !pinned && !reason:
			gaps = append(gaps, id+": no windows/amd64 pin and no no-windows reason")
		case pinned && reason:
			gaps = append(gaps, id+": a windows/amd64 pin and a no-windows reason")
		case pinned && strings.Contains(rest, "\n    from: release\n"):
			pin, err := readToolPin(data, id, "windows/amd64")
			if err != nil || !strings.HasSuffix(pin.URL, ".zip") || !strings.HasSuffix(pin.Member, programName(id, "windows/amd64")) || !strings.HasPrefix(pin.URL, "https://") {
				gaps = append(gaps, id+": the windows/amd64 pin is not a complete https .zip pin of "+programName(id, "windows/amd64"))
			}
		}
	}
	return gaps
}

// TestEveryToolPinsWindowsOrSaysWhyNot: node, pnpm and the
// release shelf, goreleaser, changie and vale among it, each pin a complete windows/amd64 zip, and every other tool
// says in no-windows why it has none. A copy of tools.yaml with a windows pin
// cut and no reason put in, or with a reason beside a pin, is red.
func TestEveryToolPinsWindowsOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if gaps := windowsGaps(data); len(gaps) > 0 {
		t.Errorf("tools.yaml: %s", strings.Join(gaps, "; "))
	}
	for _, id := range []string{"node", "pnpm", "golangci-lint", "ast-grep", "goreleaser", "changie", "vale", "actionlint", "zizmor"} {
		if _, err := readToolPin(data, id, "windows/amd64"); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	cut := regexp.MustCompile(`(?m)^      windows/amd64:\n(?:        [^\n]*\n){4}`)
	if len(cut.FindAll(data, -1)) != 9 {
		t.Fatalf("tools.yaml holds %d windows/amd64 release pins, want 9", len(cut.FindAll(data, -1)))
	}
	if gaps := windowsGaps(cut.ReplaceAll(data, nil)); len(gaps) != 9 {
		t.Errorf("a copy with every windows release pin cut: %v, want the nine tools red", gaps)
	}
	reason := []byte("    no-windows: \"a reason\"\n    platforms:\n")
	planted := bytes.Replace(data, []byte("    platforms:\n"), reason, 1)
	if gaps := windowsGaps(planted); len(gaps) != 1 {
		t.Errorf("a copy with a reason beside golangci-lint's pin: %v, want it red", gaps)
	}
	bare := regexp.MustCompile(`(?m)^    no-windows: [^\n]*\n`).ReplaceAll(data, nil)
	if gaps := windowsGaps(bare); len(gaps) == 0 {
		t.Error("a copy with every no-windows reason cut is not red")
	}
}

// TestValeIsFetchedOnlyAsThePublishersArchive (gragas#1119): the real
// tools.yaml pins vale 3.26.0 on darwin and linux, amd64 and arm64, and on
// windows/amd64 as the release's zip, each from errata-ai's release of the
// tag. On each platform the fetch asks for the pinned url alone, and an
// archive that is not the pinned one, a planted .tar.gz or .zip holding a vale
// of another sum, is refused before it is opened and nothing is placed.
// Nothing here reaches the network: the planted archive is the test's own.
func TestValeIsFetchedOnlyAsThePublishersArchive(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	const release = "https://github.com/errata-ai/vale/releases/download/v3.26.0/vale_3.26.0_"
	for plat, asset := range map[string]string{
		"darwin/amd64":  "macOS_64-bit.tar.gz",
		"darwin/arm64":  "macOS_arm64.tar.gz",
		"linux/amd64":   "Linux_64-bit.tar.gz",
		"linux/arm64":   "Linux_arm64.tar.gz",
		"windows/amd64": "Windows_64-bit.zip",
	} {
		t.Run(plat, func(t *testing.T) {
			t.Parallel()
			pin, err := readToolPin(data, "vale", plat)
			if err != nil {
				t.Fatal(err)
			}
			if pin.URL != release+asset || pin.Member != programName("vale", plat) {
				t.Fatalf("vale on %s is pinned as %s, member %s; want %s%s, member %s", plat, pin.URL, pin.Member, release, asset, programName("vale", plat))
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "tools.yaml"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			planted := tarGz(t, pin.Member, []byte("a vale of another build"))
			if strings.HasSuffix(asset, ".zip") {
				planted = zipOf(t, pin.Member, []byte("a vale of another build"))
			}
			var asked []string
			get := func(_ context.Context, url string) ([]byte, error) { asked = append(asked, url); return planted, nil }
			err = fetchTool(t.Context(), fetchJob{dir: dir, id: "vale", platform: plat, get: get}, &say.Speaker{W: &bytes.Buffer{}})
			if err == nil || !strings.Contains(err.Error(), "the archive has sha256") {
				t.Errorf("a planted archive: %v, want it refused by the archive's sum", err)
			}
			if len(asked) != 1 || asked[0] != pin.URL {
				t.Errorf("the fetch asked for %q, want the pinned url alone", asked)
			}
			if _, err := os.Stat(filepath.Join(dir, ".tools", "bin", programName("vale", plat))); err == nil {
				t.Error("a refused vale was placed")
			}
		})
	}
}
