package proof

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// ErrPath is what an error matches when a path that decides where a file is
// placed leaves the scratch folder: absolute, climbing, empty, spelled with a
// backslash, a drive or a control character, or not in its cleanest form.
var ErrPath = errors.New("the path does not stay inside the scratch folder")

// Placement is the two folders a pair is run in: the good example and the bad
// one, each at the path the binding declares, with the config set mirrored
// around it. A tool's path-scoped config applies to a file as it would to real
// code only if the file stands where the config expects it (S44).
type Placement struct {
	// NotRun is why nothing was placed, ReasonNoPair for a binding that
	// declares no path, and empty when the files are placed.
	NotRun model.Reason
	// Good and Bad are the files of each folder, sorted by path.
	Good, Bad []model.File
}

// Place puts the pair at the representative path binding declares, inside a
// scratch folder the caller makes: the good example in Good, the bad one in
// Bad, and every config file at its own path beside it, so a path-scoped
// config reaches the example. A binding with no declared path places nothing
// and is not run, no pair path. It is a pure function: it makes no folder and
// reads no file, and the pair's bytes are placed and never executed here.
//
// An error matches ErrPath for a declared path or a config path that is not a
// plain relative path of slash-separated names, and for a config file that
// stands where the example is placed. It is an error, too, for a binding that
// names another pair than the one given.
func Place(p Pair, b model.Binding, config []model.File) (Placement, error) {
	if b.PairPath == "" {
		return Placement{NotRun: model.ReasonNoPair}, nil
	}
	if b.Pair != p.Number {
		return Placement{}, fmt.Errorf("the binding names pair %d, not pair %d", b.Pair, p.Number)
	}
	if err := Confine(b.PairPath); err != nil {
		return Placement{}, fmt.Errorf("the declared path %s: %w", clip(b.PairPath), err)
	}
	mirror := make([]model.File, 0, len(config)+1)
	seen := map[string]bool{b.PairPath: true}
	for _, f := range config {
		if err := Confine(f.Path); err != nil {
			return Placement{}, fmt.Errorf("the config file %s: %w", clip(f.Path), err)
		}
		if seen[f.Path] {
			return Placement{}, fmt.Errorf("the config file %s stands where the example is placed, or twice", clip(f.Path))
		}
		seen[f.Path] = true
		mirror = append(mirror, f)
	}
	with := func(e Example) []model.File {
		files := append(slices.Clone(mirror), model.File{Path: b.PairPath, Data: e.Data})
		slices.SortFunc(files, func(a, b model.File) int { return strings.Compare(a.Path, b.Path) })
		return files
	}
	return Placement{Good: with(p.Good), Bad: with(p.Bad)}, nil
}

// Confine is nil for a path that names a place inside a folder: relative,
// slash-separated, in its cleanest form, with no part empty, "." or "..", no
// backslash, colon or control character, and at most 4096 bytes. Anything
// else matches ErrPath.
func Confine(p string) error {
	if p == "" || len(p) > 4096 || path.IsAbs(p) || path.Clean(p) != p {
		return ErrPath
	}
	for _, r := range p {
		if r < ' ' || r == 0x7f || r == '\\' || r == ':' {
			return ErrPath
		}
	}
	for part := range strings.SplitSeq(p, "/") {
		if part == "" || part == "." || part == ".." {
			return ErrPath
		}
	}
	return nil
}
