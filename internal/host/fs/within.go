package fs

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/birdtable/go-files/confine"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Within answers where the file at abs lies in root, slash-separated. Every
// link in abs is followed as far as the path exists, which is how the host
// itself would reach the file; the parts that do not exist yet are kept as
// written. So a link from outside to a folder inside answers the file below
// it, and a link inside that leads out is outside, a link to something
// missing included. Where it lies is go-files' confine.Below, which follows a
// junction too and compares folders by file ID, never by spelling.
//
// abs must be clean as the host spells a path, either separator the host
// takes counted alike: a . or .. part, or a separator doubled or at the end,
// matches port.ErrNotClean. The kernel follows a link before the .. after it
// and a tool may apply the .. to the spelling first, so link/.. could name
// one file here and another to the tool that writes it.
//
// abs's last part must not be a link either: one that leads inside matches
// port.ErrLinked, the error naming where it leads. A tool that writes in
// place writes that file, and one that writes a new file and renames it over
// replaces the link with a file at abs, which nothing placed.
func (t *Tree) Within(root model.Root, abs string) (string, error) {
	if abs == "" || !filepath.IsAbs(abs) {
		return "", fmt.Errorf("within %q: not an absolute path", abs)
	}
	full := filepath.FromSlash(abs)
	if filepath.Clean(full) != full {
		return "", fmt.Errorf("within %s: %w", abs, port.ErrNotClean)
	}
	t.mu.Lock()
	_, ok := t.roots[root]
	t.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("within %s: the root %s was not opened", abs, root)
	}
	rel, err := confine.Below(filepath.FromSlash(string(root)), full)
	switch {
	case errors.Is(err, confine.ErrOutside):
		return "", fmt.Errorf("within %s: %w: %w", abs, port.ErrOutside, err)
	case err != nil:
		return "", fmt.Errorf("within %s: %w", abs, err)
	case rel == ".":
		return "", fmt.Errorf("within %s: the root itself is no file", abs)
	}
	linked, err := lastLinked(full)
	switch {
	case err != nil:
		return "", fmt.Errorf("within %s: %w", abs, err)
	case linked:
		return "", fmt.Errorf("within %s: %w to %s", abs, port.ErrLinked, filepath.ToSlash(rel))
	}
	return filepath.ToSlash(rel), nil
}

// lastLinked is whether the last part of full, absolute and clean, is a link,
// a junction included: go-files' confine.Resolve follows every link and keeps
// every other name as written, so full resolves to its folder's answer and its
// own name unless that name is followed somewhere else. No mode is read, so a
// link is a link alike on every system.
func lastLinked(full string) (bool, error) {
	whole, err := confine.Resolve(full)
	if err != nil {
		return false, err
	}
	dir, err := confine.Resolve(filepath.Dir(full))
	if err != nil {
		return false, err
	}
	return whole != filepath.Join(dir, filepath.Base(full)), nil
}

// Written answers where the file at abs lies in root as abs spells it,
// slash-separated. The shortest part of abs that the host reads as root is
// root, whatever its spelling, an 8.3 short name or a link above the root
// among them, as go-files' confine.Below finds it by file ID; every part below
// it keeps the spelling abs gives it, since nothing below the root is
// resolved. abs must be clean, as for Within, or it matches port.ErrNotClean.
// A path no part of which is root matches ErrOutside.
func (t *Tree) Written(root model.Root, abs string) (string, error) {
	if abs == "" || !filepath.IsAbs(abs) {
		return "", fmt.Errorf("written %q: not an absolute path", abs)
	}
	full := filepath.FromSlash(abs)
	if filepath.Clean(full) != full {
		return "", fmt.Errorf("written %s: %w", abs, port.ErrNotClean)
	}
	t.mu.Lock()
	_, ok := t.roots[root]
	t.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("written %s: the root %s was not opened", abs, root)
	}
	base := filepath.FromSlash(string(root))
	var parts []string
	for p := full; ; p = filepath.Dir(p) {
		parts = append(parts, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	for _, p := range slices.Backward(parts) {
		if name, err := confine.Below(base, p); err != nil || name != "." {
			continue
		}
		rel, err := filepath.Rel(p, full)
		if err != nil || rel == "." {
			return "", fmt.Errorf("written %s: the root itself is no file", abs)
		}
		return filepath.ToSlash(rel), nil
	}
	return "", fmt.Errorf("written %s: %w", abs, port.ErrOutside)
}
