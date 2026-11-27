package config

// The kinds of thing gragas keeps that `cache` lists and clears, as one small
// registry (architecture 5.4). A kind is what a person can name, where it
// lives and which files of the table of kept files it is. A kind that a later
// milestone fills is registered here with Built false, so the listing says it
// exists and holds nothing yet; the milestone that fills it flips the flag.

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// CacheKind is one kind of kept thing.
type CacheKind struct {
	// Name is what a person types: journal, explain, config, releases.
	Name string
	// Holds says in a few words what the kind is.
	Holds string
	// Place is the folder it lives in, and Names the files of it, as the
	// table of kept files writes them (Persisted).
	Place Place
	Names []string
	// Journal is whether the files are journal files, whose records say how
	// old an entry is.
	Journal bool
	// Built is whether this build fills the kind.
	Built bool
	// Nested is whether a name matches the leading folders of a path, so the
	// files at any depth below a folder it matches are the kind's: path.Match
	// stops at a slash.
	Nested bool
}

// CacheKinds are the kinds, in the order they are listed. Each one's Names
// are the Names of a row of Persisted, which a test holds.
func CacheKinds() []CacheKind {
	return []CacheKind{
		{Name: "journal", Holds: "the copies of what a run replaced, and the closed runs they belong to",
			Place: InState, Names: []string{"*.journal", "*.copies/*/*"}, Journal: true, Built: true},
		{Name: "explain", Holds: "the index that explain reads", Place: InCache, Names: []string{"explain/*"}},
		{Name: "config", Holds: "the settings and tools as last read", Place: InCache, Names: []string{"config/*"}},
		{Name: "releases", Holds: "the rulebook releases fetched", Place: InCache, Names: []string{"releases/*"}},
		{Name: "scratch", Holds: "the folders a proof run prepares and removes, and any a stopped run left", Place: InCache, Names: []string{"scratch/*"}, Built: true, Nested: true},
		// A clear removes every folder of the tools kind, whatever its age,
		// through the step runner: a module cache a tool keeps in it holds
		// read-only folders, which go-files' removal of this account's own
		// tree empties, following no link.
		{Name: "tools", Holds: "the caches a plugin's grant keeps by its cache_vars, one folder for each plugin, version, grant and project", Place: InCache, Names: []string{ToolsFolder + "/*"}, Built: true, Nested: true},
	}
}

// ToolsFolder is the folder of the cache the tools kind lives in: the step
// runner's Cache is ToolsDir of the cache folder.
const ToolsFolder = "tools"

// ToolsDir is where a grant's cache folders are made under the cache folder
// cache, "" when the machine has no cache folder.
func ToolsDir(cache string) string {
	if cache == "" {
		return ""
	}
	return filepath.Join(cache, ToolsFolder)
}

// KeepSeconds is how long the journal's copies and the caches are kept, in
// seconds: the keep setting's count and its d (days) or h (hours).
func (s Settings) KeepSeconds() (int64, error) {
	n, unit := strings.TrimRight(s.Keep, "dh"), strings.TrimPrefix(s.Keep, strings.TrimRight(s.Keep, "dh"))
	count, err := strconv.Atoi(n)
	if err != nil || (unit != "d" && unit != "h") || count < 1 {
		return 0, fmt.Errorf("keep is %q, not a whole number followed by d or h", s.Keep)
	}
	if unit == "d" {
		return int64(count) * 24 * 3600, nil
	}
	return int64(count) * 3600, nil
}

// ScratchFolder is the folder of the cache the scratch folders of a run are
// made in, the scratch kind's place: the Sandbox adapter's Folder is this
// name, which a test holds.
const ScratchFolder = "scratch"

// ScratchDir is where a run's scratch folders are made under the cache
// folder cache, "" when the machine has no cache folder.
func ScratchDir(cache string) string {
	if cache == "" {
		return ""
	}
	return filepath.Join(cache, ScratchFolder)
}
