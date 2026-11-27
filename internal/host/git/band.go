package git

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// band is what a git's version allows the History methods that read objects
// (history-version-bands). Who reads no object and runs in every band.
type band int

const (
	// bandUnread is a git whose version is not read yet: nothing that reads
	// objects runs.
	bandUnread band = iota
	// bandOld is a git before 2.38, which has no ls-files --format: the
	// methods that read objects are not ready.
	bandOld
	// bandPartial is a git from 2.38 to 2.43, which has no --no-lazy-fetch:
	// the methods run only in a repository that names no promisor remote.
	bandPartial
	// bandNoLazy is a git from 2.44 on: every verb that reads objects runs
	// with --no-lazy-fetch.
	bandNoLazy
)

// version is a git's version as it prints it.
type version struct {
	major, minor, patch int
	text                string
}

func (v version) atLeast(major, minor int) bool {
	return v.major > major || v.major == major && v.minor >= minor
}

// bandOf is the band of v.
func bandOf(v version) band {
	switch {
	case !v.atLeast(2, 38):
		return bandOld
	case !v.atLeast(2, 44):
		return bandPartial
	}
	return bandNoLazy
}

// versionLine is the whole of what `git version` prints: three numbers, none
// with a leading zero, and only the suffixes of a release candidate, a
// Windows build or Apple's.
var versionLine = regexp.MustCompile(`^git version (0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})((?:\.rc[0-9]{1,3})?(?:\.windows\.[0-9]{1,4})?(?: \(Apple Git-[0-9]{1,6}\))?)\n$`)

// parseVersion reads what `git version` prints, strictly: anything else is
// an error.
func parseVersion(out []byte) (version, error) {
	m := versionLine.FindSubmatch(out)
	if m == nil {
		return version{}, fmt.Errorf("git version printed %q, which is no version", clip(out))
	}
	var n [3]int
	for i := range n {
		v, err := strconv.Atoi(string(m[i+1]))
		if err != nil {
			return version{}, err
		}
		n[i] = v
	}
	return version{major: n[0], minor: n[1], patch: n[2], text: fmt.Sprintf("%d.%d.%d%s", n[0], n[1], n[2], m[4])}, nil
}

// ready reads the git's band once, then answers whether a method that reads
// objects may run in dir: not on a git before 2.38, and on one before 2.44
// not in a repository that names a promisor remote, since such a git would
// fetch a missing object from it (history-version-bands).
func (g *Git) ready(ctx context.Context, dir string) error {
	if g.band == bandUnread {
		out, err := g.run(ctx, verbVersion, dir)
		if err != nil {
			return g.answer("git version could not be read", err)
		}
		v, err := parseVersion(out)
		if err != nil {
			return g.failed("git version printed what it does not print", err)
		}
		g.found, g.band = v, bandOf(v)
	}
	switch g.band {
	case bandOld:
		return tooOld(g.found.text)
	case bandPartial:
		return g.noPromisor(ctx, dir)
	case bandUnread, bandNoLazy:
	}
	return nil
}

// noPromisor is nil when the repository's config and the account's name no
// promisor remote: neither extensions.partialclone nor any
// remote.<name>.promisor, whatever its value. git config exits 1, saying
// nothing, when no key matches.
func (g *Git) noPromisor(ctx context.Context, dir string) error {
	out, err := g.run(ctx, verbPromisor, dir)
	if ex, ok := errors.AsType[*exited](err); ok && ex.code == 1 && len(ex.stderr) == 0 {
		return nil
	}
	if err != nil {
		return g.answer("git config could not be read", err)
	}
	if len(out) == 0 {
		return g.failed("git config matched and printed nothing", errors.New("an empty answer with exit 0"))
	}
	return partialClone(g.found.text)
}

// answer is err as a method answers it: the check's not ready or refused as
// it is, and anything else a failure of what, with the run id.
func (g *Git) answer(what string, err error) error {
	if nr, ok := errors.AsType[*NotReady](err); ok {
		return nr
	}
	if rf, ok := errors.AsType[*Refused](err); ok {
		return rf
	}
	return g.failed(what, err)
}
