// Command gragas-release is what gragas keeps of its release: goreleaser
// builds the archives and their checksum file from .goreleaser.yaml, and this
// command checks what it reads and what it built (#914). It has five steps,
// which Taskfile.yml runs, and build, which runs three of them around
// goreleaser:
//
//   - build is the release's build, the Taskfile's release-build: it empties
//     holds the goreleaser it is given to the sha256 tools.yaml pins for this
//     host and starts nothing else (pin.go), empties its folder, makes two
//     clean clones of HEAD in it, runs sources in each, starts that
//     goreleaser in each, the second with a cold build
//     cache, then compare and verify (build.go). It was a sh script of rm,
//     mkdir, a clone loop and goreleaser twice (gragas#1655, X211).
//   - sources runs before goreleaser, in each clean clone: it reads every
//     source .goreleaser.yaml names under files: and its like, and holds each
//     to be a plain file in the commit checked out, never a symbolic link,
//     which goreleaser would follow (gragas#1206).
//   - compare holds two checksum files, of two builds from two clean clones,
//     to each other: the same archives, each the same bytes
//     (supply.release.11).
//   - verify is the unpack step (supply.release.6): it reads the checksum file
//     back, holds each archive to its line and the folder to the file both
//     ways, unpacks each archive, refuses a name that leaves its folder, holds
//     the licence and notices in it to the tree's, holds the binary to the one
//     goreleaser built (artifacts.json), and runs the host's binary behind
//     go-proc's block, refused where the system gives none: version, which
//     must print the tag, help, and contract fixture. With -record it writes
//     what it found.
//   - draft reads GitHub's listing of the releases and names the one draft of
//     the tag whose assets are the checksum file verify read and its
//     archives, by name and sha256 both ways, so the release is published by
//     that draft's id and holds the bytes that were checked.
//
// It builds only by starting goreleaser, once its sha256 is the one tools.yaml
// pins, which makes the draft release when build is given -publish; it never tags, signs or publishes
// itself. The release guard, which holds the tree to be the commit the release
// names, clean, and the remote's (supply.release.1), is hatch tag, which the
// Taskfile runs before the tag, in the dry release and at the publish; it was
// this command's guard step, and before that scripts/release-guard
// (gragas#1655, gragas#2260). The command is its own program, apart from
// gragas-dev, so the landing check's closure does not grow (A60).
//
// main is the only place that exits (go.errors.4).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/host/term"
)

func main() {
	if code, ok := blockedChild(); ok {
		os.Exit(code)
	}
	// Ctrl+C and SIGTERM are the kit's: its Main cancels the context the run is
	// given, on every system (gragas#1614).
	os.Exit(term.Main(context.Background(), "gragas-release", os.Stderr, func(ctx context.Context, args []string) int {
		return run(ctx, args, os.Stdout, os.Stderr, newToolchain(os.Getenv, os.Environ()))
	}))
}

// system is one operating system and architecture.
type system struct{ OS, Arch string }

func (s system) String() string { return s.OS + "/" + s.Arch }

// toolchain is every effect the unpack step asks of the machine: main fills it
// with the real one, a test with a fake, so no test starts a process
// (tests.isolation.7).
type toolchain interface {
	// Host answers the system this machine runs, as the go command reports it.
	Host(ctx context.Context) (system, error)
	// Start runs the binary the unpack step wrote in dir, with one of the
	// start tests, and answers what it printed; name is its file name there. Its home,
	// configuration, cache, state and temp folders are startEnv's, beneath
	// dir/startHome, which the step makes.
	Start(ctx context.Context, dir, name string, test startTest) (string, error)
	// Network says how a start test is kept from the network, for the
	// listing and the record.
	Network() string
	// Tree answers git ls-tree -r -z of the commit the clone repo has checked
	// out: every tracked path with its mode.
	Tree(ctx context.Context, repo string) (string, error)
	// Git runs git with args in dir for the build's clones, and answers its
	// standard output and exit code; err is a git that did not run.
	Git(ctx context.Context, dir string, args ...string) (string, int, error)
	// Release runs goreleaser as r asks.
	Release(ctx context.Context, r goreleaserRun) error
	// Getenv reads one variable of the person's environment.
	Getenv(key string) string
}

// startTest is one thing the unpack step asks of the binary it unpacked.
type startTest int

const (
	startVersion startTest = iota // gragas version
	startHelp                     // gragas help, which reads no project
	startFixture                  // gragas contract fixture, which writes the fixture rulebook
)

// startHome is the folder inside a start test's folder that its home,
// configuration, cache, state and temp folders are made beneath, and
// startFixtureDir the folder contract fixture writes into there.
const (
	startHome       = "home"
	startFixtureDir = "fixture"
)

// String is the start test as the record and the listing name it: its
// command line.
func (t startTest) String() string {
	return "gragas " + strings.Join(startArgs(t), " ")
}

// startArgs are the words a start test gives the binary after its name; nil
// for no start test.
func startArgs(t startTest) []string {
	switch t {
	case startVersion:
		return []string{"version"}
	case startHelp:
		return []string{"help"}
	case startFixture:
		return []string{"contract", "fixture", "--out", startFixtureDir, "--apply"}
	}
	return nil
}

func sum(b []byte) string { return digest.Hex(b) }

var (
	// tagForm is a release tag: v and a semantic version, a pre-release allowed.
	tagForm = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)
	// systemForm is one system of the list, GOOS/GOARCH.
	systemForm = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9]+$`)
)

const usage = "usage: gragas-release build -out <folder> -goreleaser <program> -systems <os/arch,...> [-C <repository>] [-publish -tag <vX.Y.Z> -notes <folder>]\n" +
	"       gragas-release sources [-C <clone>]\n" +
	"       gragas-release verify -dist <folder> -tag <vX.Y.Z> -systems <os/arch,...> [-C <repository>]\n" +
	"       gragas-release compare <checksums.txt> <checksums.txt>\n" +
	"       gragas-release draft <releases.json> <vX.Y.Z> <checksums.txt>\n"

// run is the program with its streams and its toolchain, so a test drives it
// without a process.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, tc toolchain) int {
	out, diag := &say.Speaker{W: stdout}, &say.Speaker{W: stderr}
	code := dispatch(ctx, args, tc, out, diag)
	if out.Err != nil || diag.Err != nil {
		return exit.Failed
	}
	return code
}

func dispatch(ctx context.Context, args []string, tc toolchain, out, diag *say.Speaker) int {
	if len(args) == 0 {
		diag.Text(usage)
		return exit.Usage
	}
	var err error
	switch args[0] {
	case "build":
		req, ok := buildFlags(args[1:], diag)
		if !ok {
			return exit.Usage
		}
		err = buildStep(ctx, tc, req, out, diag)
	case "verify":
		req, ok := verifyFlags(args[1:], diag)
		if !ok {
			return exit.Usage
		}
		err = verified(ctx, tc, req, out)
	case "sources":
		repo, ok := sourcesFlags(args[1:], diag)
		if !ok {
			return exit.Usage
		}
		err = sourcesStep(ctx, tc, repo, out)
	case "compare":
		if len(args) != 3 {
			diag.Text(usage)
			return exit.Usage
		}
		err = compareStep(args[1], args[2], out)
	case "draft":
		if len(args) != 4 || !tagForm.MatchString(args[2]) {
			diag.Text(usage)
			return exit.Usage
		}
		err = draftStep(args[1], args[2], args[3], out)
	default:
		diag.Text(usage)
		return exit.Usage
	}
	if err != nil {
		diag.Text("gragas-release " + args[0] + ": " + err.Error() + "\n")
		return exit.Failed
	}
	return exit.Done
}

// request is what verify's flags asked for: the repository whose licence and
// notices the archives must carry, the folder goreleaser wrote, the tag and
// the systems.
type request struct {
	repo, dist, tag string
	// record is the file what the step found is written to, "" for none.
	record  string
	systems []system
}

func verifyFlags(args []string, diag *say.Speaker) (request, bool) {
	set := flag.NewFlagSet("gragas-release verify", flag.ContinueOnError)
	set.SetOutput(diag.W)
	repo := set.String("C", ".", "the repository whose LICENSE, NOTICE and notices the archives carry")
	dist := set.String("dist", "", "the folder goreleaser wrote, with checksums.txt")
	tag := set.String("tag", "", "the tag the archives are built for, vX.Y.Z")
	systems := set.String("systems", "", "every system an archive is built for, os/arch, comma-separated")
	rec := set.String("record", "", "the file the step writes what it found to, pass or fail")
	if err := set.Parse(args); err != nil {
		return request{}, false
	}
	bad := func(why string) (request, bool) {
		diag.Text("gragas-release verify: " + why + "\n" + usage)
		return request{}, false
	}
	if set.NArg() > 0 {
		return bad(fmt.Sprintf("it takes no argument, and was given %q", set.Arg(0)))
	}
	if *dist == "" {
		return bad("-dist names no folder")
	}
	if !tagForm.MatchString(*tag) {
		return bad(fmt.Sprintf("-tag %q is not a release tag, v and a semantic version", *tag))
	}
	list, err := parseSystems(*systems)
	if err != nil {
		return bad(err.Error())
	}
	return request{repo: *repo, dist: *dist, tag: *tag, record: *rec, systems: list}, true
}

// parseSystems reads -systems: os/arch, comma-separated, none twice.
func parseSystems(text string) ([]system, error) {
	var list []system
	for s := range strings.SplitSeq(text, ",") {
		if !systemForm.MatchString(s) {
			return nil, fmt.Errorf("-systems holds %q, which is not os/arch", s)
		}
		goos, arch, _ := strings.Cut(s, "/")
		sys := system{OS: goos, Arch: arch}
		if slices.Contains(list, sys) {
			return nil, errors.New("-systems names " + s + " twice")
		}
		list = append(list, sys)
	}
	return list, nil
}

// verified is the unpack step and its record: what it found is written to
// req.record, when one is named, whether it passed or not, and a record that
// cannot be written fails the step.
func verified(ctx context.Context, tc toolchain, req request, out *say.Speaker) error {
	rec := record{Tag: req.tag, Result: recordPass}
	err := verifyStep(ctx, tc, req, out, &rec)
	if req.record == "" {
		return err
	}
	if err != nil {
		rec.Result, rec.Failure = recordFail, err.Error()
	}
	if werr := writeRecord(req.record, rec); werr != nil {
		return errors.Join(err, fmt.Errorf("the record %s could not be written: %w", req.record, werr))
	}
	return err
}

func sourcesFlags(args []string, diag *say.Speaker) (string, bool) {
	set := flag.NewFlagSet("gragas-release sources", flag.ContinueOnError)
	set.SetOutput(diag.W)
	repo := set.String("C", ".", "the clean clone goreleaser builds from")
	if err := set.Parse(args); err != nil {
		return "", false
	}
	if set.NArg() > 0 {
		diag.Text(fmt.Sprintf("gragas-release sources: it takes no argument, and was given %q\n%s", set.Arg(0), usage))
		return "", false
	}
	return *repo, true
}

// errNoLine is a checksum file line that is not a digest and a name.
var errNoLine = errors.New("a line that is not a digest and a name")

// blockedChild is gragas-release started as the blocked child, when its first
// argument is exit.BlockedWord: the unpack step starts the binary it unpacked
// through it, so the start tests run behind go-proc's block
// (supply.install.2, A25, gragas#1455). ok is false when it was started as
// anything else.
func blockedChild() (code int, ok bool) {
	if len(os.Args) < 2 || os.Args[1] != exit.BlockedWord {
		return 0, false
	}
	return runBlocked(os.Args[2:], os.Environ(), os.Stderr), true
}
