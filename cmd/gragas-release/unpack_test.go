package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/proc"
	"github.com/birdtable/go-proc/sandbox"

	"github.com/birdtable/gragas/internal/exit"
)

// Each archive's binary is held to the binary goreleaser built for its system,
// read from the build's folder as artifacts.json names it and never from the
// archive (gragas#347, supply.release.6). Each way that record or that binary
// can be wrong is refused, naming the archive or the system.
func TestEachBinaryIsHeldToTheOneBuilt(t *testing.T) {
	t.Parallel()
	darwin := system{"darwin", "arm64"}
	linux := system{"linux", "amd64"}
	binaries := func(fx *fixture, edit func([]artifact) []artifact) {
		var as []artifact
		for _, s := range testSystems() {
			as = append(as, artifact{Name: "gragas", Path: builtPath(s), GOOS: s.OS, GOARCH: s.Arch, Type: "Binary"})
		}
		fx.writeArtifacts(edit(as))
	}
	for name, c := range map[string]struct {
		plant func(*fixture)
		says  string
	}{
		"a built binary that is not the archive's": {func(fx *fixture) {
			write(fx.t, filepath.Join(fx.repo, filepath.FromSlash(builtPath(darwin))), "another build")
		}, archiveName(testTag, darwin) + ": the binary it unpacks to is not the one built: unpacked " + sum([]byte("binary for darwin/arm64"))},
		"the built binary gone": {func(fx *fixture) {
			if err := os.Remove(filepath.Join(fx.repo, filepath.FromSlash(builtPath(linux)))); err != nil {
				fx.t.Fatal(err)
			}
		}, archiveName(testTag, linux) + ": the built binary "},
		"no artifacts.json": {func(fx *fixture) {
			if err := os.Remove(filepath.Join(fx.dist, artifactsName)); err != nil {
				fx.t.Fatal(err)
			}
		}, "the build's record of what it built could not be read"},
		"artifacts.json that is not JSON": {func(fx *fixture) {
			write(fx.t, filepath.Join(fx.dist, artifactsName), "[{")
		}, "artifacts.json: unexpected end of JSON input"},
		"no binary for a system": {func(fx *fixture) {
			binaries(fx, func(as []artifact) []artifact {
				return slices.DeleteFunc(as, func(a artifact) bool { return a.GOOS == "darwin" })
			})
		}, "artifacts.json names no binary built for darwin/arm64"},
		"two binaries for a system": {func(fx *fixture) {
			binaries(fx, func(as []artifact) []artifact { return append(as, as[0]) })
		}, "artifacts.json names two binaries built for darwin/arm64"},
		"a binary path that climbs out of the release folder": {func(fx *fixture) {
			write(fx.t, filepath.Join(fx.repo, "elsewhere", "gragas"), "binary for darwin/arm64")
			binaries(fx, func(as []artifact) []artifact { as[0].Path = "dist/../elsewhere/gragas"; return as })
		}, "artifacts.json: the binary built for darwin/arm64: dist/../elsewhere/gragas is not inside the release folder"},
		"the binary path of the release folder itself": {func(fx *fixture) {
			binaries(fx, func(as []artifact) []artifact { as[0].Path = "dist"; return as })
		}, "artifacts.json: the binary built for darwin/arm64: dist is not inside the release folder"},
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
	// an absolute binary path, made in the test's own folder so it is absolute
	// on every system: a rooted string is relative on Windows (gragas#1689)
	t.Run("an absolute binary path", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		abs := filepath.Join(t.TempDir(), "gragas")
		binaries(fx, func(as []artifact) []artifact { as[0].Path = abs; return as })
		says := fmt.Sprintf("artifacts.json: the binary built for darwin/arm64: the path %q is not a relative path", abs)
		code, out, errs := runVerify(t, fx, goodChain())
		if code != exit.Failed || out != "" || !strings.Contains(errs, "gragas-release verify: "+says) {
			t.Errorf("verify = %d, %q, %q; want it refused with %q", code, out, errs, says)
		}
	})
}

// contract fixture must leave the fixture it says it wrote, and each start
// test that fails is named by its command line.
func TestTheFixtureStartTestMustWriteTheFixture(t *testing.T) {
	t.Parallel()
	linux := archiveName(testTag, system{"linux", "amd64"})
	for name, c := range map[string]struct {
		tc   *fakeChain
		says string
	}{
		"no fixture written": {&fakeChain{says: "gragas v1.2.3 (abc)\n", noFixture: true},
			linux + ": the linux/amd64 binary's gragas contract fixture --out fixture --apply ended well and wrote no fixture into fixture"},
		"contract fixture fails": {&fakeChain{says: "gragas v1.2.3 (abc)\n", fail: true, failOn: startFixture},
			linux + ": the linux/amd64 binary did not start with gragas contract fixture --out fixture --apply: exit status 1"},
		"a failure names what the binary said, its end kept": {&fakeChain{says: "gragas v1.2.3 (abc)\n", fail: true, failOn: startHelp,
			failSays: strings.Repeat("a long banner ", 50) + "\n  failed     the line it ended on\n"},
			"did not start with gragas help: exit status 1; it said: ..."},
		"the line it ended on is kept": {&fakeChain{says: "gragas v1.2.3 (abc)\n", fail: true, failOn: startHelp,
			failSays: strings.Repeat("a long banner ", 50) + "\n  failed     the line it ended on\n"},
			"failed     the line it ended on"},
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

// runRecorded is runVerify with -record naming a file in a folder of its own,
// and answers the record it wrote.
func runRecorded(t *testing.T, fx *fixture, tc *fakeChain) (int, record) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "unpack.json")
	var out, errs bytes.Buffer
	code := run(t.Context(), []string{"verify", "-C", fx.repo, "-dist", fx.dist, "-tag", testTag, "-systems", systemsFlag(), "-record", path}, &out, &errs, tc)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("verify = %d wrote no record: %v\n%s", code, err, errs.String())
	}
	var r record
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return code, r
}

// The step's result is recorded, pass or fail (gragas#347): a release that
// passes records each archive with its digest and its binary's, the host, how
// the start tests were kept from the network and each start test run; one
// that fails records the failure the step ended on.
func TestTheRecordSaysWhatTheStepFound(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	code, r := runRecorded(t, fx, goodChain())
	if code != exit.Done {
		t.Fatalf("verify = %d, want done", code)
	}
	raw, err := os.ReadFile(filepath.Join(fx.dist, sumsName))
	if err != nil {
		t.Fatal(err)
	}
	var want []recordArchive
	for _, s := range testSystems() {
		name := archiveName(testTag, s)
		want = append(want, recordArchive{Name: name, System: s.String(), SHA256: sum(fx.archives[name]), Binary: sum([]byte("binary for " + s.String()))})
	}
	if r.Tag != testTag || r.Result != recordPass || r.Failure != "" || r.Sums != sum(raw) || r.Host != "linux/amd64" ||
		r.Network != "behind the fake's block" || !slices.Equal(r.Checked, want) ||
		!slices.Equal(r.Started, []string{"gragas version", "gragas help", "gragas contract fixture --out fixture --apply"}) {
		t.Errorf("the record of a release that passed is %+v", r)
	}

	fx = newFixture(t)
	write(t, filepath.Join(fx.repo, filepath.FromSlash(builtPath(system{"linux", "amd64"}))), "another build")
	code, r = runRecorded(t, fx, goodChain())
	if code != exit.Failed || r.Result != recordFail || !strings.Contains(r.Failure, "is not the one built") {
		t.Errorf("the record of a release that failed is %+v, after exit %d", r, code)
	}
}

// A start test's whole environment is the system's own variables and its own
// folders beneath the one it runs in, under each name this system reads, and
// no network, proxy or path variable of the person's (gragas#347,
// tests.isolation.1). madeFolders makes every one of those folders, and
// nothing outside the folder.
func TestTheStartTestsEnvironmentIsItsOwnFoldersAlone(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "run")
	got := startEnv(dir)
	if want := append(proc.SystemEnv(), proc.HomeEnv(filepath.Join(dir, startHome))...); !slices.Equal(got, want) {
		t.Fatalf("startEnv = %q, want the system's own and go-proc's home list beneath %s: %q", got, startHome, want)
	}
	system := proc.SystemEnv()
	folders := 0
	for _, kv := range got {
		k, v, _ := strings.Cut(kv, "=")
		if strings.Contains(strings.ToLower(k), "proxy") || k == "PATH" || strings.HasPrefix(k, "GIT_") || strings.HasPrefix(k, "SSH_") {
			t.Errorf("a start test is given %s", k)
		}
		switch {
		case beneath(filepath.Join(dir, startHome), v):
			folders++
		case filepath.IsAbs(v) && !slices.Contains(system, kv):
			t.Errorf("a start test is given %s, a folder outside its own %s", kv, startHome)
		}
	}
	if folders == 0 {
		t.Fatalf("startEnv = %q names no folder of the start test's own", got)
	}
	if err := madeFolders(dir, append(got, "OUTSIDE="+filepath.Join(filepath.Dir(dir), "outside"))); err != nil {
		t.Fatal(err)
	}
	for _, kv := range got {
		if _, v, _ := strings.Cut(kv, "="); beneath(dir, v) {
			if fi, err := os.Stat(v); err != nil || !fi.IsDir() {
				t.Errorf("madeFolders did not make %s: %v", kv, err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "outside")); err == nil {
		t.Error("madeFolders made a folder outside the one it was given")
	}
}

// A start test is this program started again as the blocked child, which
// sets go-proc's block and execs the binary by its path with the test's
// words, on every system: there is no argv without
// the block. Its folder and this program must be absolute paths, since the
// block runs a program only by one.
func TestTheStartCommandRunsTheBinaryBehindTheBlock(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "run")
	bin := filepath.Join(dir, builtName)
	self := filepath.Join(t.TempDir(), "gragas-release")
	for _, c := range []struct {
		claim string
		test  startTest
		want  []string
	}{
		{"version behind the block", startVersion, []string{self, exit.BlockedWord, "--", bin, "version"}},
		{"help behind the block", startHelp, []string{self, exit.BlockedWord, "--", bin, "help"}},
		{"contract fixture behind the block", startFixture, []string{self, exit.BlockedWord, "--", bin, "contract", "fixture", "--out", "fixture", "--apply"}},
	} {
		got, err := startCommand(self, dir, builtName, c.test)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s: startCommand(%q, %s) = %q, %v; want %q", c.claim, self, c.test, got, err, c.want)
		}
	}
	if got, err := startCommand(self, dir, "gragas-other", startHelp); err != nil || len(got) < 4 || got[3] != filepath.Join(dir, "gragas-other") {
		t.Errorf("the start command of the binary named gragas-other is %q, %v; want %s behind the block", got, err, filepath.Join(dir, "gragas-other"))
	}
	for claim, name := range map[string]string{"no name": "", "a name in a folder": filepath.Join("sub", builtName), "a name above the folder": filepath.Join("..", builtName), "the folder itself": "."} {
		if got, err := startCommand(self, dir, name, startHelp); err == nil {
			t.Errorf("a binary given as %s was given a start command, %q", claim, got)
		}
	}
	for claim, s := range map[string]string{"no path": "", "a relative path": "gragas-release"} {
		if got, err := startCommand(s, dir, builtName, startHelp); err == nil {
			t.Errorf("this program at %s was given a start command, %q", claim, got)
		}
	}
	if _, err := startCommand(self, "run", builtName, startHelp); err == nil {
		t.Error("a relative folder was given a start command")
	}
	if _, err := startCommand(self, dir, builtName, startTest(99)); err == nil {
		t.Error("a start test that is none was given a start command")
	}
}

// Network says what sandbox.Check found: on Linux every way out closed, on
// macOS signals and the service manager left open, and with no block that
// the start tests are refused, with why. The Blocks are fixed,
// never read from this host.
func TestTheNetworkTextSaysWhatTheBlockCloses(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		claim string
		block sandbox.Block
		err   error
		want  string
	}{
		{"Linux", sandbox.Block{Network: true, Daemons: true, Terminal: true, Signals: true, ServiceManager: true},
			nil, "behind go-proc's block (go-proc/sandbox), which closes the network, other daemons, the terminal, signals out, the service manager"},
		{"macOS", sandbox.Block{Network: true, Daemons: true, Terminal: true},
			nil, "behind go-proc's block (go-proc/sandbox), which closes the network, other daemons, the terminal and leaves open signals out, the service manager"},
		{"no block", sandbox.Block{}, fmt.Errorf("no sandbox here: %w", sandbox.ErrNoBlock),
			"with no network block on this system, so its start tests are refused: no sandbox here: "},
	} {
		if got := networkText(c.block, c.err); !strings.HasPrefix(got, c.want) {
			t.Errorf("%s: %q, want %q", c.claim, got, c.want)
		}
	}
}
