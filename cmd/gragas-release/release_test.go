package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/birdtable/go-proc/proc"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/exit"
)

// TestMain is the blocked child first: the unpack step starts this program
// again as it, and under go test this program is the test binary.
func TestMain(m *testing.M) {
	if code, ok := blockedChild(); ok {
		os.Exit(code)
	}
	os.Exit(configtest.NoLeaks(configtest.Main(m)))
}

// The blocked child takes "--" and a program by its absolute path, and
// anything else is a usage fault that names the word and starts nothing:
// go-proc's sandbox is never asked to run a line gragas refuses
// (gragas#1455).
func TestTheBlockedChildRefusesALineItDoesNotTake(t *testing.T) {
	t.Parallel()
	for claim, args := range map[string][]string{
		"nothing":          nil,
		"no --":            {"/work/true"},
		"no program":       {"--"},
		"a relative path":  {"--", "true"},
		"a word, not a --": {"run", "/work/true"},
	} {
		var stderr bytes.Buffer
		if code := runBlocked(args, nil, &stderr); code != exit.Usage || !strings.Contains(stderr.String(), "usage: "+exit.BlockedWord+" -- <program by its absolute path>") {
			t.Errorf("%s: exit %d, said %q; want usage, naming %s", claim, code, stderr.String(), exit.BlockedWord)
		}
	}
}

// fakeChain is the toolchain with no process behind it: the host is
// linux/amd64, and the unpacked binary answers version with says, and
// contract fixture writes one file into the fixture folder unless noFixture.
type fakeChain struct {
	mu        sync.Mutex
	says      string
	failOn    startTest
	fail      bool
	noFixture bool
	// failSays is what a failing start test printed.
	failSays string
	ran      []string
	tests    []startTest
	tree     string
}

// Tree lists the tree of the fake: the three sources .goreleaser.yaml names,
// each a plain file, unless tree is set.
func (f *fakeChain) Tree(context.Context, string) (string, error) {
	if f.tree != "" {
		return f.tree, nil
	}
	return plainTree(), nil
}

func (*fakeChain) Host(context.Context) (system, error) {
	return system{OS: "linux", Arch: "amd64"}, nil
}

func (*fakeChain) Network() string { return "behind the fake's block" }

// Git is no git: build's tests run the real one over repositories of their
// own (build_test.go).
func (*fakeChain) Git(context.Context, string, ...string) (string, int, error) {
	return "", 0, errors.New("the fake toolchain runs no git")
}

// Release is no goreleaser: build's tests run their own (build_test.go).
func (*fakeChain) Release(context.Context, goreleaserRun) error {
	return errors.New("the fake toolchain runs no goreleaser")
}

// Getenv is an empty environment.
func (*fakeChain) Getenv(string) string { return "" }

func (f *fakeChain) Start(_ context.Context, dir, name string, test startTest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		return "", err
	}
	for _, kv := range startEnv(dir) {
		if _, v, _ := strings.Cut(kv, "="); beneath(dir, v) {
			if fi, err := os.Stat(v); err != nil || !fi.IsDir() {
				return "", fmt.Errorf("the start test's folder %s is not a folder: %w", kv, err)
			}
		}
	}
	f.ran = append(f.ran, name)
	f.tests = append(f.tests, test)
	if f.fail && test == f.failOn {
		return f.failSays, errors.New("exit status 1")
	}
	switch test {
	case startVersion:
		return f.says, nil
	case startFixture:
		if !f.noFixture {
			if err := os.MkdirAll(filepath.Join(dir, startFixtureDir), 0o700); err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(dir, startFixtureDir, "rulebook.yaml"), []byte("x\n"), 0o600); err != nil {
				return "", err
			}
		}
		return "done: the fixture is written\n", nil
	}
	return "usage: gragas <command>\n", nil
}

const testTag = "v1.2.3"

func testSystems() []system {
	return []system{{"darwin", "arm64"}, {"linux", "amd64"}, {"windows", "amd64"}}
}

// file is one entry of a planted archive: a name, a type and bytes.
type file struct {
	name string
	kind byte
	data string
	when time.Time
}

func tarGz(t *testing.T, files []file) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		kind := f.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		h := &tar.Header{Typeflag: kind, Name: f.name, Size: int64(len(f.data)), ModTime: f.when, Format: tar.FormatPAX}
		if kind != tar.TypeReg {
			h.Size, h.Linkname = 0, f.data
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if kind == tar.TypeReg {
			if _, err := tw.Write([]byte(f.data)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fixture is a repository with its licence and notices, and a dist folder of
// one good archive per test system with its checksum file.
type fixture struct {
	t          *testing.T
	repo, dist string
	archives   map[string][]byte
}

func carriedText(f string) string { return "the tree's " + f + "\n" }

func goodFiles(s system) []file {
	folder := folderName(testTag, s) + "/"
	bin := proc.FileName(s.OS, "gragas")
	files := []file{{name: folder, kind: tar.TypeDir}, {name: folder + bin, data: "binary for " + s.String()}}
	for _, f := range carried() {
		files = append(files, file{name: folder + f, data: carriedText(f)})
	}
	return files
}

// newFixture is the repository with its release folder dist inside it, as
// goreleaser leaves it: the archives, their checksum file, each binary in the
// folder it was built to and artifacts.json naming each.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	repo := t.TempDir()
	fx := &fixture{t: t, repo: repo, dist: filepath.Join(repo, "dist"), archives: map[string][]byte{}}
	for _, f := range carried() {
		write(t, filepath.Join(fx.repo, filepath.FromSlash(f)), carriedText(f))
	}
	var built []artifact
	for _, s := range testSystems() {
		fx.archives[archiveName(testTag, s)] = tarGz(t, goodFiles(s))
		write(t, filepath.Join(fx.repo, filepath.FromSlash(builtPath(s))), "binary for "+s.String())
		built = append(built, artifact{Name: "gragas", Path: builtPath(s), GOOS: s.OS, GOARCH: s.Arch, Type: "Binary"},
			artifact{Name: archiveName(testTag, s), Path: "dist/" + archiveName(testTag, s), GOOS: s.OS, GOARCH: s.Arch, Type: "Archive"})
	}
	fx.writeArtifacts(built)
	fx.writeAll(t)
	return fx
}

// builtPath is where goreleaser builds the binary of s, from the project
// folder, as artifacts.json names it.
func builtPath(s system) string {
	return "dist/gragas_" + s.OS + "_" + s.Arch + "/" + proc.FileName(s.OS, "gragas")
}

// writeArtifacts writes artifacts.json listing as.
func (fx *fixture) writeArtifacts(as []artifact) {
	fx.t.Helper()
	// written by hand, as goreleaser writes it: a test of cmd/ writes no JSON
	// through encoding/json (cli.output.2), and %q quotes as JSON does for
	// these plain names
	var rows []string
	for _, a := range as {
		rows = append(rows, fmt.Sprintf(`{"name":%q,"path":%q,"goos":%q,"goarch":%q,"type":%q}`, a.Name, a.Path, a.GOOS, a.GOARCH, a.Type))
	}
	write(fx.t, filepath.Join(fx.dist, artifactsName), "["+strings.Join(rows, ",")+"]")
}

// writeAll writes every archive and a checksum file that lists each one.
func (fx *fixture) writeAll(t *testing.T) {
	t.Helper()
	var sums strings.Builder
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for n := range fx.archives {
			if !yield(n) {
				return
			}
		}
	}) {
		write(t, filepath.Join(fx.dist, name), string(fx.archives[name]))
		sums.WriteString(sum(fx.archives[name]) + "  " + name + "\n")
	}
	write(t, filepath.Join(fx.dist, sumsName), sums.String())
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func systemsFlag() string {
	var s []string
	for _, sys := range testSystems() {
		s = append(s, sys.String())
	}
	return strings.Join(s, ",")
}

func runVerify(t *testing.T, fx *fixture, tc *fakeChain) (int, string, string) {
	t.Helper()
	var out, errs bytes.Buffer
	code := run(t.Context(), []string{"verify", "-C", fx.repo, "-dist", fx.dist, "-tag", testTag, "-systems", systemsFlag()}, &out, &errs, tc)
	return code, out.String(), errs.String()
}

func goodChain() *fakeChain { return &fakeChain{says: "gragas v1.2.3 (abc1234)\n  tag  v1.2.3\n"} }

// The control: goreleaser's folder as the config makes it passes, and only the
// host's binary runs, once per start test.
func TestTheUnpackStepPassesAGoodReleaseAndRunsOnlyTheHostsBinary(t *testing.T) {
	t.Parallel()
	tc := goodChain()
	code, out, errs := runVerify(t, newFixture(t), tc)
	if code != exit.Done {
		t.Fatalf("verify = %d, want %d\n%s", code, exit.Done, errs)
	}
	if want := "verify: 3 archives hold to checksums.txt, each with its licence and notices, and each binary is the one built; the linux/amd64 binary printed v1.2.3, started and wrote the fixture, behind the fake's block\n"; out != want {
		t.Errorf("verify said %q, want %q", out, want)
	}
	if !slices.Equal(tc.ran, []string{builtName, builtName, builtName}) || !slices.Equal(tc.tests, []startTest{startVersion, startHelp, startFixture}) {
		t.Errorf("the unpack step ran %q for %v, want the host's binary for version, help and contract fixture", tc.ran, tc.tests)
	}
}

// Each way a release folder can be wrong is refused, and the refusal names the
// archive or the file it is about (supply.release.6, supply.licences.6).
func TestTheUnpackStepRefusesEachPlantByName(t *testing.T) {
	t.Parallel()
	linux := archiveName(testTag, system{"linux", "amd64"})
	darwin := archiveName(testTag, system{"darwin", "arm64"})
	lf := folderName(testTag, system{"linux", "amd64"}) + "/"
	replace := func(fx *fixture, name string, files []file) {
		fx.archives[name] = tarGz(fx.t, files)
		fx.writeAll(fx.t)
	}
	withLinux := func(edit func([]file) []file) func(*fixture) {
		return func(fx *fixture) { replace(fx, linux, edit(goodFiles(system{"linux", "amd64"}))) }
	}
	for name, c := range map[string]struct {
		plant func(*fixture)
		says  string
	}{
		"bytes that differ from the checksum line": {func(fx *fixture) {
			t := fx.t
			write(t, filepath.Join(fx.dist, darwin), string(tarGz(t, []file{{name: "x"}})))
		}, darwin + ": the bytes differ from the checksum line"},
		"an archive missing from the checksum file": {func(fx *fixture) {
			t := fx.t
			raw, _ := os.ReadFile(filepath.Join(fx.dist, sumsName))
			var keep []string
			for l := range strings.Lines(string(raw)) {
				if !strings.HasSuffix(l, "  "+darwin+"\n") {
					keep = append(keep, l)
				}
			}
			write(t, filepath.Join(fx.dist, sumsName), strings.Join(keep, ""))
		}, "checksums.txt has no line for " + darwin + ", the archive of darwin/arm64"},
		"a .tar.gz the checksum file does not list": {func(fx *fixture) {
			t := fx.t
			write(t, filepath.Join(fx.dist, "gragas_1.2.3_plan9_amd64.tar.gz"), "x")
		}, "gragas_1.2.3_plan9_amd64.tar.gz is in the folder and checksums.txt does not list it"},
		"a line for an archive of no system": {func(fx *fixture) {
			t := fx.t
			fx.archives["gragas_1.2.3_plan9_amd64.tar.gz"] = tarGz(t, nil)
			fx.writeAll(t)
		}, "checksums.txt lists gragas_1.2.3_plan9_amd64.tar.gz, which is the archive of no system of the list"},
		"a name that leaves its folder": {withLinux(func(f []file) []file {
			return append(f, file{name: lf + "../../etc/x", data: "x"})
		}), linux + ": the archive holds " + lf + "../../etc/x, which leaves its folder"},
		"an absolute name": {withLinux(func(f []file) []file {
			return append(f, file{name: "/work/x", data: "x"})
		}), linux + ": the archive holds /work/x, which leaves its folder"},
		"a folder entry that leaves its folder": {withLinux(func(f []file) []file {
			return append(f, file{name: "../up/", kind: tar.TypeDir})
		}), linux + ": the archive holds ../up/, which leaves its folder"},
		"a link": {withLinux(func(f []file) []file {
			return append(f, file{name: lf + "gragas-link", kind: tar.TypeSymlink, data: "/work/sh"})
		}), linux + ": the archive holds " + lf + "gragas-link, which is not a plain file"},
		"a file outside the archive's folder": {withLinux(func(f []file) []file {
			return append(f, file{name: "elsewhere/x", data: "x"})
		}), linux + ": the archive holds elsewhere/x, outside its folder " + lf},
		"no NOTICE": {withLinux(func(f []file) []file {
			return slices.DeleteFunc(f, func(e file) bool { return e.name == lf+"NOTICE" })
		}), linux + ": the archive holds no NOTICE"},
		"no notice per linked module": {withLinux(func(f []file) []file {
			return slices.DeleteFunc(f, func(e file) bool { return e.name == lf+"notices/gragas.txt" })
		}), linux + ": the archive holds no notices/gragas.txt"},
		"a licence that is not the tree's": {withLinux(func(f []file) []file {
			for i := range f {
				if f[i].name == lf+"LICENSE" {
					f[i].data = "another licence"
				}
			}
			return f
		}), linux + ": the archive's LICENSE is not the tree's"},
		"no binary": {withLinux(func(f []file) []file {
			return slices.DeleteFunc(f, func(e file) bool { return e.name == lf+"gragas" })
		}), linux + ": the archive holds no gragas binary"},
		"a file a release does not carry": {withLinux(func(f []file) []file {
			return append(f, file{name: lf + "sbom.json", data: "{}"})
		}), linux + ": the archive holds sbom.json, which a release does not carry"},
		"a Windows archive whose binary is named as on other systems": {func(fx *fixture) {
			w := system{"windows", "amd64"}
			files := goodFiles(w)
			files[1].name = folderName(testTag, w) + "/gragas"
			replace(fx, archiveName(testTag, w), files)
		}, archiveName(testTag, system{"windows", "amd64"}) + ": the archive holds no " + proc.FileName("windows", "gragas") + " binary"},
		"a checksum line that is no digest": {func(fx *fixture) {
			t := fx.t
			write(t, filepath.Join(fx.dist, sumsName), "abc  "+linux+"\n")
		}, `checksums.txt: a line that is not a digest and a name: "abc  ` + linux + `"`},
		"a checksum line whose name leaves the folder": {func(fx *fixture) {
			t := fx.t
			write(t, filepath.Join(fx.dist, sumsName), sum(nil)+"  ../"+linux+"\n")
		}, "checksums.txt: a line that is not a digest and a name"},
		"an archive listed twice": {func(fx *fixture) {
			t := fx.t
			raw, _ := os.ReadFile(filepath.Join(fx.dist, sumsName))
			write(t, filepath.Join(fx.dist, sumsName), string(raw)+sum(fx.archives[linux])+"  "+linux+"\n")
		}, "checksums.txt: it lists " + linux + " twice"},
		"an empty checksum file": {func(fx *fixture) {
			t := fx.t
			write(t, filepath.Join(fx.dist, sumsName), "")
		}, "checksums.txt: it lists no archive"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			c.plant(fx)
			code, out, errs := runVerify(t, fx, goodChain())
			if code != exit.Failed || out != "" || !strings.Contains(errs, "gragas-release verify: "+c.says) {
				t.Errorf("verify = %d, %q, %q; want it refused with %q", code, out, errs, c.says)
			}
		})
	}
}

// The host's binary must print the tag on version's first line, and start.
func TestTheHostsBinaryMustPrintTheTagAndStart(t *testing.T) {
	t.Parallel()
	linux := archiveName(testTag, system{"linux", "amd64"})
	for name, c := range map[string]struct {
		tc   *fakeChain
		says string
	}{
		"another tag":   {&fakeChain{says: "gragas v1.2.4 (abc1234)\n"}, linux + `: the linux/amd64 binary's version says "gragas v1.2.4 (abc1234)", not the tag v1.2.3`},
		"a prefix only": {&fakeChain{says: "gragas v1.2.30 (abc1234)\n"}, "not the tag v1.2.3"},
		"nothing":       {&fakeChain{says: "  \n"}, linux + ": the linux/amd64 binary printed nothing for gragas version"},
		"help fails": {&fakeChain{says: "gragas v1.2.3 (abc)\n", fail: true, failOn: startHelp},
			linux + ": the linux/amd64 binary did not start with gragas help: exit status 1"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, _, errs := runVerify(t, newFixture(t), c.tc)
			if code != exit.Failed || !strings.Contains(errs, c.says) {
				t.Errorf("verify = %d, %q; want %q", code, errs, c.says)
			}
		})
	}
}

// writeSums writes a checksum file of the archives by name, into a folder of
// its own, and answers its path.
func writeSums(t *testing.T, archives map[string][]byte) string {
	t.Helper()
	var b strings.Builder
	for _, n := range slices.Sorted(func(yield func(string) bool) {
		for n := range archives {
			if !yield(n) {
				return
			}
		}
	}) {
		b.WriteString(sum(archives[n]) + "  " + n + "\n")
	}
	path := filepath.Join(t.TempDir(), sumsName)
	write(t, path, b.String())
	return path
}

func runCompare(t *testing.T, a, b string) (int, string, string) {
	t.Helper()
	var out, errs bytes.Buffer
	code := run(t.Context(), []string{"compare", a, b}, &out, &errs, goodChain())
	return code, out.String(), errs.String()
}

// Two builds that made the same bytes pass the compare; a time stamp planted
// in one build's archive makes it fail, and the message names the archive that
// differs (supply.release.11).
func TestAPlantedTimeStampFailsTheCompareAndNamesTheArchive(t *testing.T) {
	t.Parallel()
	commit := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	build := func(when time.Time) map[string][]byte {
		out := map[string][]byte{}
		for _, s := range testSystems() {
			files := goodFiles(s)
			for i := range files {
				files[i].when = commit
			}
			if s.OS == "linux" {
				files[1].when = when
			}
			out[archiveName(testTag, s)] = tarGz(t, files)
		}
		return out
	}
	first := writeSums(t, build(commit))
	if code, out, errs := runCompare(t, first, writeSums(t, build(commit))); code != exit.Done || out != "compare: 3 archives, byte for byte the same from two clean clones\n" {
		t.Fatalf("two equal builds: compare = %d, %q, %q", code, out, errs)
	}
	code, out, errs := runCompare(t, first, writeSums(t, build(commit.Add(time.Second))))
	linux := archiveName(testTag, system{"linux", "amd64"})
	if code != exit.Failed || out != "" || !strings.Contains(errs, "gragas-release compare: not reproducible: "+linux+" differs between the two builds") {
		t.Errorf("a planted time stamp: compare = %d, %q, %q; want it to name %s", code, out, errs, linux)
	}
}

// An archive one build made and the other did not fails the compare, named.
func TestAnArchiveOnlyOneBuildMadeFailsTheCompare(t *testing.T) {
	t.Parallel()
	both := map[string][]byte{"gragas_1.2.3_linux_amd64.tar.gz": []byte("a")}
	more := map[string][]byte{"gragas_1.2.3_linux_amd64.tar.gz": []byte("a"), "gragas_1.2.3_plan9_amd64.tar.gz": []byte("b")}
	a, b := writeSums(t, both), writeSums(t, more)
	if code, _, errs := runCompare(t, a, b); code != exit.Failed || !strings.Contains(errs, "the second build made gragas_1.2.3_plan9_amd64.tar.gz and the first did not") {
		t.Errorf("compare = %d, %q", code, errs)
	}
	if code, _, errs := runCompare(t, b, a); code != exit.Failed || !strings.Contains(errs, "the first build made gragas_1.2.3_plan9_amd64.tar.gz and the second did not") {
		t.Errorf("compare = %d, %q", code, errs)
	}
	bad := filepath.Join(t.TempDir(), sumsName)
	write(t, bad, "not a line\n")
	if code, _, errs := runCompare(t, a, bad); code != exit.Failed || !strings.Contains(errs, bad+": a line that is not a digest and a name") {
		t.Errorf("compare of a bad file = %d, %q", code, errs)
	}
}

func TestUsageAndExitCodes(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][]string{
		"no step":                      nil,
		"an unknown step":              {"build"},
		"compare of one file":          {"compare", "a"},
		"draft of no checksum":         {"draft", "r.json", "v1.0.0"},
		"draft of no tag":              {"draft", "r.json", "1.0.0", "checksums.txt"},
		"no dist":                      {"verify", "-tag", "v1.0.0", "-systems", "linux/amd64"},
		"a tag with no v":              {"verify", "-dist", "d", "-tag", "1.0.0", "-systems", "linux/amd64"},
		"a tag that is a path":         {"verify", "-dist", "d", "-tag", "v1.0.0/../x", "-systems", "linux/amd64"},
		"no systems":                   {"verify", "-dist", "d", "-tag", "v1.0.0"},
		"a system twice":               {"verify", "-dist", "d", "-tag", "v1.0.0", "-systems", "linux/amd64,linux/amd64"},
		"a system of a path":           {"verify", "-dist", "d", "-tag", "v1.0.0", "-systems", "linux/../amd64"},
		"an argument":                  {"verify", "-dist", "d", "-tag", "v1.0.0", "-systems", "linux/amd64", "extra"},
		"an unknown flag":              {"verify", "-out", "d"},
		"sources with an argument":     {"sources", "extra"},
		"sources with an unknown flag": {"sources", "-dist", "d"},
	} {
		var out, errs bytes.Buffer
		if code := run(t.Context(), args, &out, &errs, goodChain()); code != exit.Usage || out.Len() != 0 || errs.Len() == 0 {
			t.Errorf("%s: run = %d, %q, %q; want usage on stderr", name, code, out.String(), errs.String())
		}
	}
}

// release is the source test: no SBOM, provenance or signing code is left under
// cmd/gragas-release (#914; gragas#345, #346 and #353 hold them for later).
var releaseExtras = regexp.MustCompile(`(?i)sbom|provenance|signer|signature|cyclonedx|spdx|in-toto|slsa|\bsign\(`)

func extrasIn(src map[string]string) []string {
	var out []string
	for name, text := range src {
		for _, m := range releaseExtras.FindAllString(text, -1) {
			out = append(out, name+": "+m)
		}
	}
	slices.Sort(out)
	return out
}

func TestNoSBOMProvenanceOrSigningCodeIsLeft(t *testing.T) {
	t.Parallel()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	src := map[string]string{}
	for _, n := range names {
		if strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		src[n] = string(b)
	}
	if len(src) < 4 {
		t.Fatalf("read %d source files, want the command's", len(src))
	}
	if got := extrasIn(src); len(got) > 0 {
		t.Errorf("cmd/gragas-release holds release extras it no longer makes: %q", got)
	}
	planted := extrasIn(map[string]string{
		"a.go": "func sbom() {}", "b.go": "// the provenance", "c.go": "s port.Signer", "d.go": "x := sign(sums)",
	})
	if len(planted) != 4 {
		t.Errorf("the plants found %q, want all four", planted)
	}
}

// capture refuses a nil environment: os/exec reads nil as "inherit all of it".
func TestCaptureRefusesANilEnvironment(t *testing.T) {
	t.Parallel()
	if _, err := hostIn(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "no environment") {
		t.Fatalf("hostIn with a nil environment = %v, want it refused", err)
	}
}

// Host works in a shell that would break it if the shell's environment reached
// the go command: a toolchain to fetch, bogus flags, a workspace that is not
// there. Risk: an unpack step that fails, or names the wrong host, because of
// the shell it was started from.
func TestTheRealHostIgnoresAHostileShell(t *testing.T) {
	// serial: it sets hostile variables in the process, to prove the toolchain call reads none of them
	for k, v := range map[string]string{"GOTOOLCHAIN": "go1.99.9", "GOFLAGS": "-mod=bogus", "GOWORK": "/nonexistent/go.work", "GOOS": "plan9"} {
		t.Setenv(k, v)
	}
	home, err := os.MkdirTemp("", "gragas-release-home-")
	if err != nil {
		t.Fatal(err)
	}
	// go reads its configuration folder under this home, not the isolated one
	// go-proc v0.1.1 turned telemetry off in (gragas#913). So the home's
	// telemetry is turned off with go's own command, run with the home and the
	// toolchain's environment; without it go's telemetry writes to the home
	// after go exits.
	given := proc.HomeEnv(home)
	if err := madeFolders(home, given); err != nil {
		t.Fatal(err)
	}
	homeEnv := func(k string) string {
		for _, kv := range given {
			if n, v, _ := strings.Cut(kv, "="); n == k {
				return v
			}
		}
		return ""
	}
	t.Cleanup(func() {
		// One removal, held gone: nothing writes to the home after go exits.
		if err := os.RemoveAll(home); err != nil {
			t.Errorf("the home was not removed: %v", err)
		}
		if _, err := os.Lstat(home); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the home is there after one removal: %v", err)
		}
	})
	if err := telemetryOff(t.Context(), homeEnv); err != nil {
		t.Fatal(err)
	}
	tc := newToolchain(homeEnv, nil)
	if s, err := tc.Host(t.Context()); err != nil || s.OS == "plan9" || s.OS == "" {
		t.Errorf("Host = %v, %v", s, err)
	}
}

// telemetryOff turns go's telemetry off for the home getenv names, with go's
// own command and the environment of every go command here. Only the
// hostile-shell test runs it: it hands go a home of its own, which go-proc's
// testenv does not cover, and the release itself never changes a person's go
// settings, so it lives with the tests (gragas#1897).
func telemetryOff(ctx context.Context, getenv func(string) string) error {
	_, err := capture(ctx, "", baseEnv(getenv), "go", "telemetry", "off")
	return err
}
