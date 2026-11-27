package audit

import (
	"slices"
	"strings"
)

// Tracked is one path the repository tracks: its path, slash-separated and
// relative to the root, and whether git's own mode for it is a symbolic link
// (120000), read from the tree and never from a stat of the work tree. Key is
// the path as a disk that folds case reads it, each name of it through the
// port's Portable and joined by "/", a name the port refuses standing for
// itself: two paths with one key are one path on such a disk, and the core holds
// no table of what a disk does with a name. Refused is why a name
// of it is one no system can hold, "" when none is: the path is then a
// not-checked row besides, since the refused name itself is never compared.
type Tracked struct {
	Path    string
	Link    bool
	Key     string
	Refused string
}

// CheckTracked is the tracked-files check: no tracked path is a symbolic link
// (Git for Windows checks one out as a file holding the link's text), and no
// two tracked paths differ only by case (the macOS and Windows file systems
// fold case, so one of the two overwrites the other). Each link is a finding
// of its own; each group of paths that share a key is one finding, at the
// first path of the group and naming the rest, the names a refused name stands
// beside compared as they are. A path with a name no system can hold, or that
// came with no key, is not a finding for that name, since a rule for a name no
// disk holds would be a rule of its own: it is a not-checked row naming the
// path and the reason, never dropped and never a clean answer. The row is the
// case-twin check's, so it is written only when a rule binds that check: with
// none, nobody asked for the comparison, and a row about it is noise on every
// screen.
func CheckTracked(rules RepoRules, files []Tracked) ([]Finding, []NotChecked) {
	var out []Finding
	var unchecked []NotChecked
	groups := map[string][]string{}
	for _, f := range files {
		if f.Link && rules.Has(KeyLink) {
			out = append(out, found(rules, KeyLink, f.Path, 0,
				"is a tracked symbolic link, which Git for Windows checks out as a file holding the link's text"))
		}
		if f.Key != "" {
			groups[f.Key] = append(groups[f.Key], f.Path)
		}
		why := f.Refused
		if why == "" && f.Key == "" {
			why = "it came with no key"
		}
		if why != "" && rules.Has(KeyFold) {
			unchecked = append(unchecked, Unjudged(KeyTracked, f.Path+" was not checked for a twin that differs only by case: "+why))
		}
	}
	for _, paths := range groups {
		if len(paths) < 2 || !rules.Has(KeyFold) {
			continue
		}
		slices.Sort(paths)
		out = append(out, found(rules, KeyFold, paths[0], 0,
			"differs only by case from "+strings.Join(paths[1:], ", ")+", which the macOS and Windows file systems hold as one path"))
	}
	slices.SortFunc(unchecked, func(a, b NotChecked) int { return strings.Compare(a.Why, b.Why) })
	return sorted(out), unchecked
}
