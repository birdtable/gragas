package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// program answers the path git is run at: the setting, with its links
// resolved, when every name on the way passes the check. It never looks in
// PATH. The check is made on every run, just before it starts. A resolved
// path that holds = is refused here in this adapter's words, with the path
// and the fix: go-proc starts no program whose path holds =,
// and its own refusal names neither. The folder git runs in may hold =.
func (g *Git) program(context.Context) (string, error) {
	s := gitProgram()
	path, err := g.trusted(s, g.cfg.Git)
	if err != nil {
		return "", err
	}
	if strings.ContainsRune(path, '=') {
		return "", s.refused(s.setting+" resolves to "+strconv.Quote(path)+", whose path holds =, and go-proc, which starts git, starts no program whose path holds = (an env-style launcher reads it as a setting)",
			"install git in a folder whose path holds no =, links followed, and set "+s.setting+" in the settings file to it")
	}
	return path, nil
}

// trusted answers set, the path the setting of s names, with its links
// resolved, when every name on the way passes the check for s, as
// internal/host/trust walks it. A setting may hold the model's slash form,
// so set is made this system's own form once, here at the edge, and
// that native path is the one judged and walked.
func (g *Git) trusted(s subject, set string) (string, error) {
	native := filepath.FromSlash(set)
	switch {
	case set == "":
		return "", s.missing("names no path")
	case !nativeClean(native):
		return "", s.missing("is " + strconv.Quote(set) + ", which is not an absolute, clean path")
	}
	return g.walkFor(s, native)
}

// nativeClean is whether p, a path in this system's own form, is absolute and
// clean: no climb, no . and no doubled or trailing separator.
func nativeClean(p string) bool { return filepath.IsAbs(p) && filepath.Clean(p) == p }

// walkFor walks set, the path of s, as this account, and answers what the
// walk answers in this adapter's words.
func (g *Git) walkFor(s subject, set string) (string, error) {
	path, err := hosttrust.Walk(s.trust(), set, hosttrust.Who{Mounts: g.cfg.Mounts})
	return g.answered(s, set, path, err)
}

// answered is what the walk of set, the path of s, answered, path and err,
// turned into this adapter's own: a name that is not there is not ready,
// one it does not trust refused, and one it cannot read failed. A name
// another account owns is refused with the fix of s for it, when s has one,
// and with the walk's chown line when it has none. A failure
// names set, which the walk's sentence does not: where the walk opens each
// folder to judge it, a folder this account may not read fails there, and
// not at a later read of the adapter's that names it.
func (g *Git) answered(s subject, set, path string, err error) (string, error) {
	var unsafe *hosttrust.Unsafe
	var failed *hosttrust.Failed
	switch {
	case err == nil:
		return path, nil
	case errors.As(err, &unsafe):
		fix := unsafe.Fix
		if unsafe.Owned && s.owned != "" {
			fix = s.owned
		}
		return "", s.refused(unsafe.Observed, fix)
	case errors.As(err, &failed):
		what := failed.What
		if !strings.Contains(what, set) {
			what += ", at " + set
		}
		return "", g.failed(what, failed.Err)
	case errors.Is(err, os.ErrNotExist):
		return "", s.missing("is " + set + ", where there is no file")
	}
	return "", g.failed("the path of "+s.name+" at "+set+" could not be read", err)
}
