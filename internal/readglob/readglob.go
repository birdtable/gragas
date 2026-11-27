// Package readglob is the grammar of the glob a plugin's read step names in
// plugin contract 1, and the match of a path against it. A plugin writes a
// read glob in this grammar, the engine's codec refuses what is outside it,
// the host's read step matches the files of the scratch folder with it, and
// the plugin test serves a preset's files by it; one matcher holds for all
// four, and none keeps a copy. It does not stand in contract/plugin/v1, which
// holds the contract's two schemas and nothing else.
//
// The grammar: a glob is a slash-separated path whose parts are names that may
// hold * (any run of characters, a leading dot among them, as gitignore reads
// it) and ? (exactly one character), or ** alone, which is any number of
// folders, none included. No part crosses a slash, and the match is case exact.
// Any other glob character ([, ], {, } and the backslash) and a ** beside other
// characters in its part are outside it.
package readglob

import (
	"errors"
	"fmt"
	"strings"
)

// ErrGlob is what Parts's refusals match: the glob is outside the grammar.
var ErrGlob = errors.New("the glob is outside the contract's grammar")

// Parts is a glob of the contract split at its slashes, ready for Match. It
// refuses, matching ErrGlob, an empty glob, a glob character beyond *, ? and
// **, and a ** with other characters in its part. That a part is a name that
// stays below a folder (not empty, not . or ..) is the reader's to judge, as
// the folder is its own.
func Parts(g string) ([]string, error) {
	if g == "" {
		return nil, fmt.Errorf("the glob of the step is empty: %w", ErrGlob)
	}
	if strings.ContainsAny(g, "[]{}\\") {
		return nil, fmt.Errorf("the glob %q has a glob character beyond *, ? and **: %w", g, ErrGlob)
	}
	parts := strings.Split(g, "/")
	for _, part := range parts {
		if strings.Contains(part, "**") && part != "**" {
			return nil, fmt.Errorf("the glob %q has ** with other characters in its part: %w", g, ErrGlob)
		}
	}
	return parts, nil
}

// Match says whether the slash-separated path parts match the glob parts,
// case exact: ** takes any number of parts, none included. It is wildMatch,
// with ** the star and a part matched by partMatch, so it takes at most
// len(glob)·len(parts) part matches, never a backtrack per star.
func Match(glob, parts []string) bool {
	return wildMatch(len(glob), len(parts),
		func(i int) bool { return glob[i] == "**" },
		func(i, j int) bool { return partMatch([]rune(glob[i]), []rune(parts[j])) })
}

// partMatch says whether name matches the glob part p: * is any run of
// characters, ? is one, and every other character is itself, case exact. As
// gitignore reads it, * takes a name that starts with a dot.
func partMatch(p, name []rune) bool {
	return wildMatch(len(p), len(name),
		func(i int) bool { return p[i] == '*' },
		func(i, j int) bool { return p[i] == '?' || p[i] == name[j] })
}

// wildMatch says whether a pattern of np elements matches a subject of ns,
// where an element i for which star holds takes any run of the subject, none
// included, and every other takes one subject element j for which one(i, j)
// holds. It is the match that goes back only to the last star: an element
// between two stars is matched at its earliest place, and no earlier star is
// tried again, as a later one can take whatever an earlier one would, so it
// makes at most np·ns calls of one, where a backtrack at every star is
// exponential in the stars.
func wildMatch(np, ns int, star func(i int) bool, one func(i, j int) bool) bool {
	i, j := 0, 0
	last, from := -1, 0 // the last star seen, and where its run ends now
	for j < ns {
		switch {
		case i < np && star(i):
			last, from = i, j
			i++
		case i < np && one(i, j):
			i++
			j++
		case last >= 0:
			from++
			i, j = last+1, from
		default:
			return false
		}
	}
	for i < np && star(i) {
		i++
	}
	return i == np
}
