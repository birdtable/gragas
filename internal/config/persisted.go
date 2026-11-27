package config

// Every file gragas keeps, made in one place (store.state-files.1,
// cli.config.5), and the table that says who writes each (architecture 5.5,
// store.seams.4). A test in internal/cli/root runs every command that writes
// over the in-memory ports and fails on a written path the table does not
// hold; M0b-05 completes it for apply's writes.

import (
	"path"
	"path/filepath"
	"strings"
)

// File is one of gragas's own files in its own folders.
type File int

// gragas's own files.
const (
	FileSettings File = iota
	FileTools
	FileLock
	FileRunLog
)

// Path is where f is, "" when its folder is unknown. Every path of
// gragas's own files is made here.
func (c Config) Path(f File) string {
	switch f {
	case FileSettings:
		if c.Named {
			return c.SettingsFile
		}
		return under(c.ConfigDir, "settings.yaml")
	case FileTools:
		if c.Settings.Tools != "" {
			return c.Settings.Tools
		}
		return under(c.ConfigDir, "tools.yaml")
	case FileLock:
		return under(c.StateDir, "gragas.lock")
	case FileRunLog:
		return under(c.StateDir, "runs.jsonl")
	}
	return ""
}

func under(dir, name string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, name)
}

// Place is the folder a kept file is in.
type Place string

// The places gragas keeps files.
const (
	// InRepository: a rulebook repository's tree, the folder -C names.
	InRepository Place = "the repository"
	// InProject: a project's root, in .gragas/ or beside it.
	InProject Place = "the project"
	// InState, InConfig and InCache: gragas's own folders.
	InState  Place = "the state folder"
	InConfig Place = "the config folder"
	InCache  Place = "the cache folder"
)

// StateFile is the project's state file, relative to the project root: the one
// place its path is written, for the table and for the line init prints to keep
// it out of git (store.state-files.1).
const StateFile = ".gragas/state.yaml"

// PinFile is the project's pin, relative to the project root: the file in
// which a repository names its rulebook. internal/core/project
// reads it, from the name its caller hands it.
const PinFile = ".gragas/pin.yaml"

// Kept is one row of the table of kept files.
type Kept struct {
	// Names are the files, as path.Match patterns relative to Place.
	Names []string
	Place Place
	// WrittenBy is the command that writes it, "nothing" for a file only
	// a person writes.
	WrittenBy string
	// Rebuilt says whether it can be made again, and how.
	Rebuilt string
	// Durability is how a write lands.
	Durability string
	// Built is whether this build writes it: a row that is not built is the
	// design's, for the milestone that builds it.
	Built bool
}

// Persisted is the table of every file gragas keeps.
func Persisted() []Kept {
	return []Kept{
		{Names: []string{"rulebook/rules.json", "rulebook/bindings.json", "rulebook/coverage.json", "rulebook/ids.lock"},
			Place: InRepository, WrittenBy: "gen --apply", Rebuilt: "yes, by gen --apply from the rulebook's YAML",
			Durability: "replaced whole: a reader sees the old file or the new one", Built: true},
		{Names: []string{"gragas.lock"}, Place: InState, WrittenBy: "gen --apply; apply and proof.Run (M0b)",
			Rebuilt: "not state", Durability: "a kernel advisory lock, no content that matters", Built: true},
		{Names: []string{"runs.jsonl"}, Place: InState, WrittenBy: "gen --apply, and every run that writes",
			Rebuilt: "no, but it is a record, not a source", Durability: "append, one JSON line per run", Built: true},
		{Names: []string{"settings.yaml", "tools.yaml"}, Place: InConfig, WrittenBy: "nothing: a person writes them",
			Rebuilt: "no", Durability: "not written by gragas"},
		{Names: []string{"history.jsonl"}, Place: InRepository, WrittenBy: "every plan that ends in a rulebook yes",
			Rebuilt: "no, and it lives in the owner's git repository", Durability: "append, one line, flushed"},
		{Names: []string{".gragas/lock.yaml", PinFile, ".gragas/waivers.yaml", ".gragas/baselines.yaml"}, Place: InProject,
			WrittenBy: "the project plans", Rebuilt: "no, and they live in the owner's git repository", Durability: "replaced whole, flushed"},
		{Names: []string{StateFile}, Place: InProject, WrittenBy: "status, check, prove, upgrade",
			Rebuilt: "yes, by the next fetch", Durability: "replaced whole"},
		{Names: []string{"*.journal", "*.copies/*/*"}, Place: InState, WrittenBy: "apply; cache clear removes closed runs",
			Rebuilt: "no, but it is transient: closed records are kept for the age keep says", Durability: "append, one line, flushed; plain, versioned and append-only"},
		{Names: []string{"explain/*", "config/*", "releases/*"}, Place: InCache, WrittenBy: "explain, check and fetch, managed by cache",
			Rebuilt: "yes", Durability: "replaced whole"},
		{Names: []string{"scratch/*"}, Place: InCache, WrittenBy: "prove, one folder a run, removed when the run ends, a cancel included",
			Rebuilt: "yes: each run makes its own", Durability: "made and removed by the run that prepared it; never read by another", Built: true},
		{Names: []string{ToolsFolder + "/*"}, Place: InCache, WrittenBy: "a plugin's step whose grant names cache_vars: gragas makes the folders, and the grant's program writes in them",
			Rebuilt: "yes: a tool's cache, which its next step fills again", Durability: "the program's own; gragas never reads them", Built: true},
	}
}

// Holds is the row of table that holds the file name in place, and whether
// one does.
func Holds(table []Kept, place Place, name string) (Kept, bool) {
	name = strings.TrimPrefix(path.Clean(filepath.ToSlash(name)), "./")
	for _, k := range table {
		if k.Place != place {
			continue
		}
		for _, pattern := range k.Names {
			if ok, err := path.Match(pattern, name); err == nil && ok {
				return k, true
			}
		}
	}
	return Kept{}, false
}

// Where is the place of the absolute path p among c's own folders, and its
// name there, or false when p is in none of them.
func (c Config) Where(p string) (Place, string, bool) {
	for _, d := range []struct {
		place Place
		dir   string
	}{{InState, c.StateDir}, {InConfig, c.ConfigDir}, {InCache, c.CacheDir}} {
		if d.dir == "" {
			continue
		}
		if rel, err := filepath.Rel(d.dir, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return d.place, filepath.ToSlash(rel), true
		}
	}
	return "", "", false
}
