// Package config is the one place gragas reads its settings: the environment,
// through the getenv main hands it, and the settings file, through the reader
// main hands it, which opens the file and checks who may write it. This
// package opens no file. It also holds the one helper that says where
// gragas's own files go, and the table of every file gragas keeps.
//
// The sources, first to last, each over the ones after it: flags (the
// command tree's), the environment, the settings file, the defaults
// (cli.config.1).
package config

import (
	_ "embed"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/birdtable/go-files/appdirs"
)

// The environment variables config reads, besides the XDG folders and HOME.
const (
	// NamesFileVar names the private forbidden-names file. It is the names-file
	// key's environment source, and wins over the file.
	NamesFileVar = "GRAGAS_NAMES_FILE"
	// BaseRefVar names the base ref check in a rulebook holds HEAD's history
	// against. It is the base-ref key's environment source, and wins over
	// the file, so a CI job names its target branch without a settings file.
	BaseRefVar = "GRAGAS_BASE_REF"
	// ShellVar names the shell the lines to paste are written for. It is the
	// shell key's environment source, and wins over the file, so a journey
	// or a script fixes the dialect whatever shell it was started from.
	ShellVar = "GRAGAS_SHELL"
	// MountsVar names the folders root mounted a disk on for this machine,
	// in the system's own list form (: on Unix), each an absolute, clean
	// path. It is the mounts key's environment source, and wins over the
	// file; the settings and tools files are read with it alone, since the
	// file that holds the key may itself be on such a disk.
	MountsVar = "GRAGAS_MOUNTS"
	// ConfigVar names the settings file, in place of the one in the config
	// folder: the one variable that moves it (cli.config.5).
	ConfigVar = "GRAGAS_CONFIG"
	// ConfigFlag names the settings file on the command line, over
	// ConfigVar. The root reads it from the arguments before the rest.
	ConfigFlag = "--config"
)

// Config is the settings of one run, from every source.
type Config struct {
	// NamesFile is the path of the forbidden-names file, or "" for none: the
	// environment's, else the settings file's.
	NamesFile string
	// BaseRef is the base ref check in a rulebook holds HEAD's history
	// against, "" for none: the environment's, else the settings file's.
	BaseRef string
	// Mounts are the folders, in the system's own form, on whose disks a
	// name mounted nosuid, or noexec without nosuid, is judged by its owners:
	// the environment's, else the settings file's. Each
	// adapter that judges a name hands them to host/trust, which holds each
	// entry first.
	Mounts []string
	// CI is whether a CI system runs this: plain output, no banner, no
	// question (CI set to anything).
	CI bool
	// ConfigDir, StateDir and CacheDir are gragas's own folders, as go-files'
	// appdirs lays them out for the system: each from its XDG
	// variable, else from HOME, on Unix and macOS (a relative XDG variable is
	// ignored, as the XDG spec says), under LOCALAPPDATA on Windows. They are
	// "" when the home is not usable, and Load refuses that. The config
	// folder holds the settings file and the tools file, the state folder the
	// lock and the run log, and the cache folder what can be rebuilt.
	ConfigDir, StateDir, CacheDir string
	// SettingsFile is the settings file's path, "" when there is no folder
	// to look in. Named is whether --config or GRAGAS_CONFIG named it.
	SettingsFile string
	Named        bool
	// Settings is what the settings file says, or the defaults.
	Settings Settings
	// Machine is what the tools file says, the zero Machine with no file.
	Machine Machine
	// colourFromEnv is whether NO_COLOR or CLICOLOR_FORCE is set, which
	// decides colour over the file.
	colourFromEnv bool
	// Columns is COLUMNS as a width, 0 when unset or not a number.
	Columns int
	// NoColor is NO_COLOR set to anything, DumbTerm is TERM=dumb, TrueColor is
	// COLORTERM naming 24-bit colour, Accessible is ACCESSIBLE set to anything.
	NoColor, DumbTerm, TrueColor, Accessible bool
	// mountsVar is GRAGAS_MOUNTS as it is set, which Load reads.
	mountsVar string
	// ForceColor is CLICOLOR_FORCE set to anything: colour even into a pipe,
	// over the colour key. NO_COLOR and the rest that take colour away win.
	ForceColor bool
	// home is why HOME was not joined for a folder that fell back to it:
	// not an absolute, clean path free of control characters, as git's
	// HomeOK holds it. Load refuses the run on it, by name.
	home, homeKey string
	// ChildPath and ChildLang are PATH and LANG, the whole environment an
	// enforcer tool is run with (cli.process.2): nothing else of this
	// process's environment reaches it.
	ChildPath, ChildLang string
	// shellEnv is GRAGAS_SHELL as it is set, "" when unset: Dialect reads it
	// over the shell key. It is kept apart from Settings so a run that falls
	// back to the defaults still carries it, as it carries NamesFile.
	shellEnv string
}

// FromEnv reads the environment through getenv, so tests pass their own.
// The settings are the defaults: Load reads the file over them.
func FromEnv(getenv func(string) string) Config {
	c := Config{CI: getenv("CI") != "", Settings: Defaults()}
	c.folders(getenv)
	switch named := getenv(ConfigVar); {
	case named != "":
		c.SettingsFile, c.Named = named, true
	case c.ConfigDir != "":
		c.SettingsFile = c.Path(FileSettings)
	}
	c.colourFromEnv = getenv("NO_COLOR") != "" || getenv("CLICOLOR_FORCE") != ""
	c.NamesFile = getenv(NamesFileVar)
	c.BaseRef = getenv(BaseRefVar)
	c.shellEnv = getenv(ShellVar)
	c.mountsVar = getenv(MountsVar)
	c.Columns = columnsOf(getenv("COLUMNS"))
	c.NoColor, c.DumbTerm, c.Accessible = getenv("NO_COLOR") != "", getenv("TERM") == "dumb", getenv("ACCESSIBLE") != ""
	c.ForceColor = getenv("CLICOLOR_FORCE") != ""
	c.TrueColor = getenv("COLORTERM") == "truecolor" || getenv("COLORTERM") == "24bit"
	c.ChildPath, c.ChildLang = getenv("PATH"), getenv("LANG")
	return c
}

// HomeVar is the variable the account's home is read from: the one go-files'
// appdirs reads it from on the system it is built for, which folders reads, so
// no system's variable is spelled here.
const HomeVar = appdirs.HomeVar

// AccountHome is the account's home as getenv gives it, from HomeVar, so the
// home git is handed is the home gragas's folders lie under.
func AccountHome(getenv func(string) string) string { return getenv(HomeVar) }

// folders sets gragas's three folders from go-files' appdirs.HereWith, in its
// XDG layout on macOS as on Linux: the system, and so the layout, come from
// appdirs' build constraints, and the home from getenv alone. A home that is
// not set, or is not an absolute, clean path free of control characters, is
// never joined: the folders are "", and c.home says why, for Load to refuse. A
// relative home would put gragas's own folders under the folder it runs in,
// which may be a repository. No Run folder is no refusal: gragas keeps none.
func (c *Config) folders(getenv func(string) string) {
	// appdirs.HomeVar is the one variable HereWith reads the home from on the
	// system it is built for; its value is kept here so that the home is
	// checked as it is, not as appdirs cleans it. No name of ours says which.
	var key, home string
	// A system appdirs names no folders for answers before it reads the home,
	// so key stays empty: the refusal then says "the home folder".
	f, err := appdirs.HereWith("gragas", appdirs.XDG, func(k string) string {
		v := getenv(k)
		if k == appdirs.HomeVar {
			key, home = k, v
		}
		return v
	})
	switch {
	case key != "" && unclean(home) != "" && home != "":
		c.home, c.homeKey = unclean(home), key
	case errors.Is(err, appdirs.ErrHome):
		c.home, c.homeKey = "is not set", key
	case err != nil && !errors.Is(err, appdirs.ErrRun):
		c.home, c.homeKey = err.Error(), key
	default:
		c.ConfigDir, c.StateDir, c.CacheDir = f.Config, f.State, f.Cache
	}
}

// Load is the settings of a run: the environment through getenv, then the
// settings file flag names (--config, "" for none; over GRAGAS_CONFIG) or the
// one in the config folder, through read, under it; then the tools file the
// tools key names, or the config folder's, through read, as c.Machine. read
// is host/trust's ReadOwned, which cmd/gragas hands it, with GRAGAS_MOUNTS's
// folders alone, never the key's: it opens the file, checks its owner
// and mode, and returns its bytes, or an error that matches fs.ErrNotExist
// or errors.ErrUnsupported, or has a Refusal method (host/trust's Owned).
//
// When something is wrong, Load returns the Config with the defaults in
// place of the file, and a *Problem that says how the run ends.
func Load(getenv func(string) string, flag string, read func(path string, mounts []string) ([]byte, error)) (Config, error) {
	c := FromEnv(getenv)
	envNames, envRef := c.NamesFile, c.BaseRef
	if p := c.fromEnv(); p != nil {
		return c, p
	}
	envMounts, prob := mountsOf(c.mountsVar)
	if prob != nil {
		return c, prob
	}
	c.Mounts = envMounts
	by := ConfigVar
	if flag != "" {
		c.SettingsFile, c.Named, by = flag, true, ConfigFlag
	}
	if why := unclean(c.SettingsFile); c.Named && why != "" {
		return c, refused("%s %s", by, why)
	}
	if c.SettingsFile == "" {
		return c, nil
	}
	if err := c.readSettings(read, by, envNames, envRef, envMounts); err != nil {
		return c, err
	}
	if err := c.readTools(read, envNames, envRef, envMounts); err != nil {
		return c, err
	}
	return c, nil
}

// fromEnv is the Problem of what the environment set, before any file is
// read: a home that is not an absolute, clean path, or a names file, base
// ref or shell of its own that is not one gragas takes; nil when there is
// none. GRAGAS_SHELL's refusal names the values it takes.
func (c *Config) fromEnv() *Problem {
	if c.home != "" {
		if c.homeKey == "" {
			c.homeKey = "the home folder"
		}
		return notReady("set "+c.homeKey+" to an absolute, clean path", "%s %s, so gragas's folders are not put under it", c.homeKey, c.home)
	}
	if why := uncleanNames(c.NamesFile); c.NamesFile != "" && why != "" {
		return refused("%s %s", NamesFileVar, why)
	}
	if why := badRev(c.BaseRef); c.BaseRef != "" && why != "" {
		return refused("%s %s", BaseRefVar, why)
	}
	if why := badShell(c.shellEnv); c.shellEnv != "" && why != "" {
		p := refused("%s %s", ShellVar, why)
		p.Next = unset(ShellVar) + ", or set it to " + either(shellNames())
		return p
	}
	return nil
}

// readSettings reads the settings file through read into c, which by named,
// with envMounts, GRAGAS_MOUNTS's folders, alone: the mounts key cannot let
// its own file be read. envNames, envRef and envMounts are the environment's,
// which decide over the file's. A file by does not name that is not there is
// the defaults.
func (c *Config) readSettings(read func(path string, mounts []string) ([]byte, error), by, envNames, envRef string, envMounts []string) error {
	data, err := read(c.SettingsFile, envMounts)
	switch {
	case errors.Is(err, fs.ErrNotExist) && c.Named:
		return notReady(unset(by)+", or write the file", "%s names %s, which does not exist", by, c.SettingsFile)
	case errors.Is(err, fs.ErrNotExist):
		// The defaults, and the tools file is still checked after.
		return nil
	case err != nil:
		return readProblem("the settings file", c.SettingsFile, err)
	}
	s, err := Parse(c.SettingsFile, data)
	if err != nil {
		return err
	}
	c.Settings = s
	if envNames == "" {
		c.NamesFile = s.NamesFile
	}
	if envRef == "" {
		c.BaseRef = s.BaseRef
	}
	if c.mountsVar == "" {
		c.Mounts = native(s.Mounts)
	}
	return nil
}

// readTools reads the tools file through read into c.Machine: the tools key's,
// or the config folder's, with envMounts alone, as the settings file is. It is
// read with a settings file or without, and who may write it checked, since it
// names programs gragas will run. A missing one is no tools: the tool
// adapter's last step then names no place. A run that ends here carries
// nothing of the settings file: c's settings, names file, base ref and mounts
// go back to the defaults and envNames, envRef and envMounts.
func (c *Config) readTools(read func(path string, mounts []string) ([]byte, error), envNames, envRef string, envMounts []string) error {
	tools := c.Path(FileTools)
	if tools == "" {
		return nil
	}
	data, err := read(tools, envMounts)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		c.Settings, c.NamesFile, c.BaseRef, c.Mounts = Defaults(), envNames, envRef, envMounts
		return readProblem("the tools file", tools, err)
	}
	m, err := ParseTools(tools, data)
	if err != nil {
		c.Settings, c.NamesFile, c.BaseRef, c.Mounts = Defaults(), envNames, envRef, envMounts
		return err
	}
	c.Machine = m
	return nil
}

// Colour is how the settings file asks for colour: "auto", "always" or
// "never". It is "auto" when NO_COLOR or CLICOLOR_FORCE is set, since the
// environment decides over the file; --no-color decides over both.
func (c Config) Colour() string {
	if c.colourFromEnv || c.Settings.Colour == "" {
		return "auto"
	}
	return c.Settings.Colour
}

// LockPath is the lock a run that writes takes, so two never write at once.
func (c Config) LockPath() string { return c.Path(FileLock) }

// RunLog is where every run that wrote is recorded, one JSON line per write.
func (c Config) RunLog() string { return c.Path(FileRunLog) }

// Var is one environment variable config reads, as docs/settings.md lists
// it. FromEnv is the reader of every one.
type Var struct {
	Name, Default, Holds, Refused string
}

// Vars are the environment variables config reads, in the order the doc
// lists them. The terminal's own (TERM, COLUMNS, ACCESSIBLE) are host/term's.
func Vars() []Var {
	return append(append([]Var{
		{ConfigVar, "the config folder's settings.yaml", "the settings file, in place of the one in the config folder",
			"a path that is not absolute and clean; a file it names that does not exist is not ready"},
		{NamesFileVar, "unset: the names-file key, else no names check", "the private names file, over the names-file key",
			"a path that is not absolute and clean; a file that cannot be read or holds a bad pattern is not ready"},
		{BaseRefVar, "unset: the base-ref key, else check in a rulebook is not ready", "the base ref check in a rulebook holds HEAD's history against, over the base-ref key",
			refRefused},
		{ShellVar, "unset: the shell key, else auto", "the shell the lines to paste are written for, over the shell key: " + either(shellNames()) + ", in any case",
			shellRefused},
		{MountsVar, "unset: the mounts key, else none", "the folders root mounted a disk on for this machine, over the mounts key, in the system's list form (: on Unix); the one source the settings and tools files are read with",
			"an empty entry, or an entry that is not an absolute path in its own clean form, named; a folder that is no mount's root, a link, or below a folder an account but root can change refuses the run"},
	}, systemVars()...), []Var{
		{"CI", "unset", "set to anything: plain output, no colour, banner or question", "never"},
		{"NO_COLOR", "unset", "set to anything: no colour, over the colour key", "never"},
		{"CLICOLOR_FORCE", "unset", "set to anything: colour even into a pipe, over the colour key; NO_COLOR wins", "never"},
		{"TERM", "unset", "`dumb`: no colour", "never"},
		{"COLUMNS", "the terminal's own width, else 80", "the width, in digits, over the terminal's own; anything else is ignored", "never"},
		{"ACCESSIBLE", "unset", "set to anything: the accessible form, no colour", "never"},
		{"COLORTERM", "unset", "truecolor or 24bit: 24-bit colour where there is colour", "never"},
		{"PATH", "unset: an enforcer is run with none", "the PATH an enforcer tool is run with; gragas finds no tool by it", "never"},
		{"LANG", "unset: an enforcer is run with none", "the LANG an enforcer tool is run with: with PATH, the whole of its environment", "never"},
	}...)
}

// systemVarsText is the help table's rows that differ by system, kept as
// data so no code here names a system's folder or variable.
//
//go:embed vars_system.txt
var systemVarsText string

// systemVars reads systemVarsText: a row a line, four fields tab apart, `#`
// for a comment.
func systemVars() []Var {
	var out []Var
	for line := range strings.SplitSeq(systemVarsText, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// A row short of a field leaves it empty, which the help table's own
		// test refuses: each needs words.
		f := append(strings.SplitN(line, "\t", 4), "", "", "")
		out = append(out, Var{Name: f[0], Default: f[1], Holds: f[2], Refused: f[3]})
	}
	return out
}

// unset is how a person stops by naming the settings file: the flag is
// left off, the variable unset.
func unset(by string) string {
	if by == ConfigFlag {
		return "leave out " + ConfigFlag
	}
	return "unset " + by
}

// mountsOf is GRAGAS_MOUNTS's folders, split as the system splits a list
// of paths (: on Unix, ; on Windows): each must be absolute and its own
// clean form in the system's spelling, with no control character. An entry
// that is not is refused by name, never passed over, since a mount left out
// in silence would refuse later with no word of why; so is an empty entry,
// as a stray separator leaves. Unset or empty is none.
func mountsOf(v string) ([]string, *Problem) {
	if v == "" {
		return nil, nil
	}
	var out []string
	for _, e := range filepath.SplitList(v) {
		switch {
		case e == "":
			return nil, refused("%s holds an empty entry: write each folder once, with one separator between", MountsVar)
		case strings.IndexFunc(e, unicode.IsControl) >= 0:
			return nil, refused("%s entry %q holds a control character", MountsVar, e)
		case !filepath.IsAbs(e) || strings.HasPrefix(e, "-"):
			return nil, refused("%s entry %q must be an absolute path", MountsVar, e)
		case filepath.Clean(e) != e:
			return nil, refused("%s entry %q must be a clean path: write %q", MountsVar, e, filepath.Clean(e))
		}
		out = append(out, e)
	}
	return out, nil
}

// native is the settings file's mounts, in the slash form, in the system's
// own.
func native(mounts []string) []string {
	out := make([]string, len(mounts))
	for i, m := range mounts {
		out[i] = filepath.FromSlash(m)
	}
	return out
}
