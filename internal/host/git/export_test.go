package git

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/model"
)

// Shown is runs as a failure prints them: each verb, argv, folder and code,
// and the variables of its environment by name alone, so no failure prints
// a value the environment holds.
func Shown(runs []Ran) string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = fmt.Sprintf("{Verb:%s Argv:%q EnvNames:%q Dir:%q Code:%d}", r.Verb, r.Argv, envNames(r.Env), r.Dir, r.Code)
	}
	return "[" + strings.Join(out, " ") + "]"
}

// OpenCache opens the releases cache dir as Fetch does, and closes it.
func OpenCache(g *Git, dir string) error {
	cache, err := g.openCache(dir)
	if err != nil {
		return err
	}
	return cache.Close()
}

// CommitOn is Commit of paths in root as though HEAD were head when it was
// read, so a test can move HEAD between the read and the update.
func CommitOn(ctx context.Context, g *Git, root model.Root, head string, paths []string, message string) (_ string, err error) {
	dir := string(root)
	h, err := parseOID(head)
	if err != nil {
		return "", err
	}
	if err := g.ready(ctx, dir); err != nil {
		return "", err
	}
	r, err := parseRev(head)
	if err != nil {
		return "", err
	}
	before, err := g.tree(ctx, dir, r)
	if err != nil {
		return "", err
	}
	before = regular(before)
	w, err := newWork("")
	if err != nil {
		return "", err
	}
	defer g.done(w, &err)
	changes, err := g.changes(ctx, dir, w, paths, before, len(head))
	if err != nil {
		return "", err
	}
	return g.commitOn(ctx, dir, w, h, changes, message)
}

// ErrNoChange is a commit whose named paths hold what HEAD holds.
var ErrNoChange = errNoChange

// ErrPastCap is a file or a print past its cap.
var ErrPastCap = errPastCap

// MaxListing is the cap of a listing and of a file read whole.
const MaxListing = maxListing

// Opening sets what runs in a commit after the last check of a named path
// and before its open, with the name of the path in the root.
func Opening(g *Git, f func(name string)) { g.opening = f }

// WithCA is g handing an https fetch the CA file at path, so a test can
// fetch from its own loopback server (testCA): the one place the field is
// written.
func WithCA(g *Git, path string) *Git {
	g.ca = path
	return g
}

// ErrDeadline is a run that did not end before the deadline.
var ErrDeadline = errDeadline

// WithWait is g with its deadline at wait, so a test can end a run at it
// without waiting the ten seconds every run is given.
func WithWait(g *Git, wait time.Duration) *Git {
	g.wait = wait
	return g
}

// Walked is what the trust walk saw of path, which a test loosened, as the
// adapter walks it as, git or a fetch's source or the source's git dir:
// the words a refusal must hand on as they are, since go-files's detail is
// this system's own, mode bits on Unix and an access list on Windows.
// A walk that passes path fails the test.
func Walked(t testing.TB, path string, as Walk) string {
	t.Helper()
	return walked(t, map[Walk]subject{WalkGit: gitProgram(), WalkSource: fetchSource(), WalkGitDir: fetchGitDir()}[as], path).Observed
}

// Walk is the subject Walked names a path as.
type Walk int

// The subjects of Walked.
const (
	WalkGit Walk = iota
	WalkSource
	WalkGitDir
)

// WantGlobal is wantGlobal, for the external tests.
func WantGlobal(t *testing.T) string {
	t.Helper()
	return wantGlobal(t)
}
