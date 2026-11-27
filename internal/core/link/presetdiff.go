package link

import (
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
)

// The three ways a preset file changes.
const (
	FileAdded   = "added"
	FileRemoved = "removed"
	FileChanged = "changed"
)

// FileChange is one preset file of a tool that differs between two
// generations: the lines only the new file holds and the lines only the old one
// held, each in file order. A file whose bytes are equal is no change.
type FileChange struct {
	Tool    string
	Path    string
	Kind    string
	Added   []string
	Removed []string
}

// DiffPresets is what a change to the book does to the presets: every file
// that is added, removed or changed from before to after, in the order of the
// tool and path. A file is the tool's and its path together, so two tools that
// name one file make two files. It is the diff a plan shows (S42); it reads
// nothing and holds no date.
func DiffPresets(before, after Generated) []FileChange {
	type held struct {
		tool, file string
		data       []byte
	}
	index := func(g Generated) map[string]held {
		m := map[string]held{}
		for _, f := range g.Files {
			m[path.Join(f.Tool, f.File.Path)] = held{f.Tool, f.File.Path, f.File.Data}
		}
		return m
	}
	old, now := index(before), index(after)
	var out []FileChange
	keys := append(canon.SortedKeys(old), canon.SortedKeys(now)...)
	slices.SortFunc(keys, canon.Compare)
	for _, k := range slices.Compact(keys) {
		o, hadOld := old[k]
		n, hasNew := now[k]
		c := FileChange{Tool: o.tool, Path: o.file, Kind: FileChanged}
		if hasNew {
			c.Tool, c.Path = n.tool, n.file
		}
		switch {
		case !hadOld:
			c.Kind = FileAdded
		case !hasNew:
			c.Kind = FileRemoved
		case string(o.data) == string(n.data):
			continue
		}
		c.Added, c.Removed = lineDiff(string(o.data), string(n.data))
		out = append(out, c)
	}
	return out
}

// lineDiff is the lines of next that old lacks and the lines of old that next
// lacks, counting repeats, each in the order of its file.
func lineDiff(old, next string) (added, removed []string) {
	count := func(s string) map[string]int {
		m := map[string]int{}
		for _, l := range lines(s) {
			m[l]++
		}
		return m
	}
	had, has := count(old), count(next)
	for _, l := range lines(next) {
		if had[l] > 0 {
			had[l]--
			continue
		}
		added = append(added, l)
	}
	for _, l := range lines(old) {
		if has[l] > 0 {
			has[l]--
			continue
		}
		removed = append(removed, l)
	}
	return added, removed
}

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
