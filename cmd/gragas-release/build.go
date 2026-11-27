package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/birdtable/gragas/internal/dev/say"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// snapshotTag is the tag goreleaser's --snapshot builds name the archives with,
// and the one the unpack step holds a dry release to.
const snapshotTag = "v0.0.0-snapshot"

// buildRequest is what build's flags asked for: the repository, the folder the
// clones and the record are made in, goreleaser's path (held to its pin before
// it runs), the systems, and for
// a publish the tag and the folder of its notes.
type buildRequest struct {
	repo, out, goreleaser string
	publish               bool
	tag, notes            string
	systems               []system
}

// goreleaserRun is one start of goreleaser: the program at program, an
// absolute path, with args, in dir, in the person's environment with extra
// over it, its streams on stdout and stderr.
type goreleaserRun struct {
	dir, program   string
	args, extra    []string
	stdout, stderr io.Writer
}

func buildFlags(args []string, diag *say.Speaker) (buildRequest, bool) {
	set := flag.NewFlagSet("gragas-release build", flag.ContinueOnError)
	set.SetOutput(diag.W)
	repo := set.String("C", ".", "the repository whose HEAD is built")
	out := set.String("out", "", "the folder the two clones and unpack.json are made in, emptied first")
	goreleaser := set.String("goreleaser", "", "goreleaser, by its absolute path; refused unless its sha256 is the one tools.yaml pins for this host")
	systems := set.String("systems", "", "every system an archive is built for, os/arch, comma-separated")
	publish := set.Bool("publish", false, "make the first build a draft release")
	tag := set.String("tag", "", "the tag of a publish, vX.Y.Z")
	notes := set.String("notes", "", "the folder of a publish's release notes, <tag>.md in it")
	if err := set.Parse(args); err != nil {
		return buildRequest{}, false
	}
	bad := func(why string) (buildRequest, bool) {
		diag.Text("gragas-release build: " + why + "\n" + usage)
		return buildRequest{}, false
	}
	switch {
	case set.NArg() > 0:
		return bad(fmt.Sprintf("it takes no argument, and was given %q", set.Arg(0)))
	case *out == "":
		return bad("-out names no folder")
	case *goreleaser == "":
		return bad("-goreleaser names no program")
	case *publish && !tagForm.MatchString(*tag):
		return bad(fmt.Sprintf("-tag %q is not a release tag, v and a semantic version", *tag))
	case *publish && *notes == "":
		return bad("-notes names no folder, and a publish takes its notes from it")
	case !*publish && (*tag != "" || *notes != ""):
		return bad("-tag and -notes are a publish's, and a snapshot is " + snapshotTag)
	}
	list, err := parseSystems(*systems)
	if err != nil {
		return bad(err.Error())
	}
	req := buildRequest{repo: *repo, out: *out, goreleaser: *goreleaser, publish: *publish, tag: *tag, notes: *notes, systems: list}
	if !req.publish {
		req.tag = snapshotTag
	}
	return req, true
}

// buildStep is the release's build (supply.release.6, .11),
// the Taskfile's release-build, which was a sh script of rm, mkdir, a clone loop
// and goreleaser twice. goreleaser runs twice, from two clean clones of HEAD
// in out, each clone's sources first held to plain files (sourcesStep), the
// second with a cold build cache; the two checksum files are
// compared, then the first folder is unpacked and what it found recorded in
// out/unpack.json. With publish the first build is the draft release, made with
// GITHUB_TOKEN and the tag's notes, and the second publishes nothing and is
// given no token. Before any of it, the goreleaser named is held to the
// sha256 tools.yaml pins for this host (heldGoreleaser), so only the pinned
// program is started. This step starts goreleaser and is the one that does;
// it still tags, signs and publishes nothing itself.
func buildStep(ctx context.Context, tc toolchain, req buildRequest, out, diag *say.Speaker) error {
	if err := heldGoreleaser(ctx, tc, req); err != nil {
		return err
	}
	dir, err := releaseFolder(req.repo, req.out)
	if err != nil {
		return err
	}
	notes, err := publishNotes(tc, req)
	if err != nil {
		return err
	}
	if err := cleanClones(ctx, tc, req.repo, dir, out); err != nil {
		return err
	}
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	first := goreleaserRun{dir: a, program: req.goreleaser, args: []string{"release", "--clean", "--snapshot"}, stdout: out.W, stderr: diag.W}
	second := first
	second.dir, second.extra = b, []string{"GOCACHE=" + filepath.Join(dir, "cache-b")}
	if req.publish {
		first.args = []string{"release", "--clean", "--release-notes", notes}
		second.args = []string{"release", "--clean", "--skip=publish"}
		second.extra = append(second.extra, "GITHUB_TOKEN=")
	}
	for _, r := range []goreleaserRun{first, second} {
		if err := tc.Release(ctx, r); err != nil {
			return err
		}
	}
	if err := compareStep(filepath.Join(a, "dist", sumsName), filepath.Join(b, "dist", sumsName), out); err != nil {
		return err
	}
	return verified(ctx, tc, request{repo: a, dist: filepath.Join(a, "dist"), tag: req.tag, systems: req.systems, record: filepath.Join(dir, "unpack.json")}, out)
}

// publishNotes is the absolute path of a publish's notes, <tag>.md in its
// folder, once the file is there and GITHUB_TOKEN is set; "" for a snapshot.
func publishNotes(tc toolchain, req buildRequest) (string, error) {
	if !req.publish {
		return "", nil
	}
	if tc.Getenv("GITHUB_TOKEN") == "" {
		return "", errors.New("GITHUB_TOKEN is not set, and the draft is made with it")
	}
	notes, err := filepath.Abs(filepath.Join(req.notes, req.tag+".md"))
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(notes); err != nil {
		return "", fmt.Errorf("the notes of %s: %w", req.tag, err)
	}
	return notes, nil
}

// cleanClones empties dir and makes in it a and b, two clones of repo's
// common git folder with no hard links into it, each with repo's HEAD checked
// out detached and its sources held to plain files (sourcesStep).
func cleanClones(ctx context.Context, tc toolchain, repo, dir string, out *say.Speaker) error {
	head, err := gitLine(ctx, tc, repo, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return err
	}
	common, err := gitLine(ctx, tc, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	for _, c := range []string{"a", "b"} {
		clone := filepath.Join(dir, c)
		if err := gitDo(ctx, tc, dir, "clone", "-q", "--no-local", "--no-checkout", common, clone); err != nil {
			return err
		}
		if err := gitDo(ctx, tc, clone, "-c", "advice.detachedHead=false", "checkout", "-q", "--detach", head); err != nil {
			return err
		}
		if err := sourcesStep(ctx, tc, clone, out); err != nil {
			return err
		}
	}
	return nil
}

// releaseFolder is out by its absolute path, refused when emptying it would take
// the repository with it: the repository itself, or a folder that holds it.
func releaseFolder(repo, out string) (string, error) {
	dir, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(repo)
	if err != nil {
		return "", err
	}
	if rel, err := filepath.Rel(dir, root); err == nil && filepath.IsLocal(rel) {
		return "", fmt.Errorf("-out %s is the repository or holds it, and the build empties it first", dir)
	}
	return dir, nil
}

// gitDo runs git with args in dir through the toolchain's git, and a git
// that fails is an error.
func gitDo(ctx context.Context, tc toolchain, dir string, args ...string) error {
	_, code, err := tc.Git(ctx, dir, args...)
	if err == nil && code != 0 {
		err = fmt.Errorf("git %s exited %d", strings.Join(args, " "), code)
	}
	return err
}

// gitLine is the one line git prints for args in dir, through the toolchain's
// git; a git that fails or prints no line is an error.
func gitLine(ctx context.Context, tc toolchain, dir string, args ...string) (string, error) {
	text, code, err := tc.Git(ctx, dir, args...)
	line := strings.TrimSpace(text)
	switch {
	case err != nil:
		return "", err
	case code != 0:
		return "", fmt.Errorf("git %s exited %d", strings.Join(args, " "), code)
	case line == "" || strings.Contains(line, "\n"):
		return "", fmt.Errorf("git %s printed %q, not one line", strings.Join(args, " "), line)
	}
	return line, nil
}
