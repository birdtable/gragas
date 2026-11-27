// Package tool is the Steps port's adapter: the one place an enforcer tool is
// located and run (arch.ports.6, cli.process.1). It names no tool. Where a
// tool may live and how it prints its version are the grant's, found as
// step_grant.go finds it; a new tool is a new plugin and no change here.
//
// A grant's program is located in a fixed order (step_grant.go): the path the
// settings give it for the project, then the grant's own place below the
// project, then the per-machine tools file's. The first that holds the tool at
// a version the grant serves is the one; another version is never taken in its
// place, and it is never looked up in PATH and never installed.
// What is found is checked before it is asked anything (trust.go): the file
// and every folder above it up to the project or a folder of root's must be
// root's or this account's and not writable by group or others. A
// script is checked with the program its first line starts: an absolute
// interpreter, or the first folder of the PATH a run is given that holds the
// name /usr/bin/env is asked for, each held to the same walk; any other first
// line refuses the tool (interpreter, trust.go).
//
// Each run (run.go) starts the path the grant's find resolved, with the words
// of the plan's step held as step_words.go holds them; no shell is ever
// started. Each run goes through go-proc's proc.Run:
// the environment is PATH and LANG and nothing else, what it
// prints is capped, and a run past its cap or its deadline has its whole
// tree ended (cli.process.2, cli.process.3). The tree ends with every run in
// any case. This is a guard against accidents, not a jail: a tool runs as
// this account, and only the sandboxed level confines it. A run held to a
// scratch folder's limits is also held to its memory limit (run.go,
// memoryHold): where go-proc holds the whole tree to a memory
// limit here (a delegated cgroup on Linux, the Job Object on Windows) it runs
// held to one and the kernel ends it past it; where the system only reads a
// peak (macOS) it runs unheld and is judged after, and says so; where it does
// neither it is refused before it starts, with the fix. Either way its peak
// is judged against the limit after the run (overMemory), and a run that left
// a process of its tree running is not counted, since that process's peak
// was never read (leftUnread, go-proc's Result.Left). A grandchild that leaves the tree
// (setsid), and one whose parent did not wait for it and that ended before
// the tool did, are the named residuals. Every run is recorded.
package tool

import (
	"sync"
	"time"

	"github.com/birdtable/go-console/shell"
	"github.com/birdtable/go-proc/proc"
)

// The bounds of every run (cli.process.2). A run that prints past MaxOutput
// or runs past Deadline is ended, and the run says so.
const (
	Deadline  = 2 * time.Minute
	MaxOutput = 16 << 20
	// versionDeadline bounds the run that asks a tool its version.
	versionDeadline = 10 * time.Second
)

// Config is what the adapter is made from. Every field comes from config,
// never from a literal.
type Config struct {
	// FileName is the file name a go program of a grant's path has on this system. Nil
	// is this package's FileName; a test hands one that marks the name.
	FileName func(id string) string
	// Paths is the settings' tool-paths: per project root, absolute and
	// slash-separated, each tool@version to the tool's absolute path.
	Paths map[string]map[string]string
	// Machine is the tools file's tools: each tool@version to the tool's
	// absolute path on this machine, the locate order's last step.
	Machine map[string]string
	// Path and Lang are the values of PATH and LANG every run is given: the
	// whole of its environment. One that is empty is left out, and nothing
	// of this process's environment takes its place. A pinned find's runs,
	// and a run of what it answered, are given Path with every folder that
	// is relative, has a .. segment, lies at or below the root, cannot be
	// compared with it, or holds the list separator or a double quote left out. When that leaves no folder, an empty Path
	// among them, every pinned find answers not found and starts nothing,
	// and a step's run refuses what one answered: a run is never given
	// no PATH, which reads the folder it runs in.
	Path, Lang string
	// Home is a folder the caller made and owns, for a program that keeps a
	// cache: a run then also gets HOME and TMPDIR set to it, XDG_CACHE_HOME
	// below it, and GOENV=off and GOTOOLCHAIN=local so the go command reads no env file
	// and fetches no toolchain, and no other variable. It is empty by default.
	// Every start, a find's version check included, is refused unless Home is
	// an absolute, clean folder other than the root with no control
	// character, outside the project root and the folder of the call (by
	// spelling, and as folders, by identity), with no link on its way, not
	// writable by group or others, and every name on its way passing the
	// trust walk (home.go; go-files trust.Path). The caller
	// can set no other variable, PATH, LD_PRELOAD and the like among them.
	Home string
	// Cache is the folder a grant's cache variables point below:
	// config.ToolsDir of the cache folder, absolute and clean. Each variable a
	// step's grant names by cache_vars is given a folder of its own there, one
	// for each plugin, version, grant and project, made private once every
	// name of it that is there is judged (cache.go), and the Home's cache of
	// such a step is kept there too, in one more folder of that grant's
	// (go-proc's proc.HomeEnvCache), apart from the Home that goes with the
	// step. A grant with no cache_vars keeps nothing there: its step's Home
	// cache is fresh with its Home. Empty, a grant's variables are left unset,
	// and its program keeps its cache in the step's Home, which goes with the
	// step.
	Cache string
	// Report is told of a fault that fails no run: a step's Home that could not be
	// removed or swept. Nil is not told.
	Report func(msg string)
	// Mounts are config.Config's Mounts: folders on whose disks a name
	// mounted nosuid, or noexec without nosuid, is judged by its owners in
	// place of refused, in every trust walk the adapter makes.
	Mounts []string
	// VersionWait is how long the run that asks a tool its version may take. Zero
	// is versionDeadline, ten seconds, which every program a person runs is held
	// to; a caller that runs a tool on a loaded host and wants no verdict from a
	// slow start, as the recorder, sets it to Deadline.
	VersionWait time.Duration
	// Dialect is the shell a fix line to paste is written for: main's dialect
	// of the shell setting, the one the run's other lines are quoted for. The
	// zero Dialect, as when that shell has no safe quoting, says the step in
	// words, and so does an argv shell.Quote refuses.
	Dialect shell.Dialect
}

// Tool locates and runs enforcer tools for one run. Make it with New.
type Tool struct {
	cfg Config
	// wait and max are the deadline and the cap of a run: Deadline and
	// MaxOutput, unless a test of this package set them lower.
	wait time.Duration
	max  int

	mu  sync.Mutex
	ran []Ran
}

// New makes the adapter. It reads no file and runs nothing.
func New(cfg Config) *Tool {
	if cfg.FileName == nil {
		cfg.FileName = FileName
	}
	return &Tool{cfg: cfg, wait: Deadline, max: MaxOutput}
}

// Ran is the record of one run: the tool, the shape ("version" for the run
// that asks a tool its version), the argv as it was started, the folder,
// the exit code (-1 when it did not exit), why it was ended, "" when it
// ended itself, the peak memory in MiB of a run held to a memory limit
// (peakMiB), -1 when none was read, what that peak counts (go-proc's
// PeakKind, "" when none was read), whether the limit was judged after the
// run alone (memoryHold), and whether its tree held a process when the tool
// exited (go-proc's Result.Left).
type Ran struct {
	Tool, Shape string
	Argv        []string
	Dir         string
	Code        int
	Ended       string
	PeakMiB     int
	PeakKind    string
	JudgedAfter bool
	Left        bool
}

// Ran answers every run this adapter started, in order, a copy.
func (t *Tool) Ran() []Ran {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Ran, len(t.ran))
	for i, r := range t.ran {
		r.Argv = append([]string(nil), r.Argv...)
		out[i] = r
	}
	return out
}

// FileName is the file name a program named id has on this system,
// go-proc's, so no other package imports go-proc for it.
func FileName(id string) string { return proc.LocalFileName(id) }

// report tells Config.Report of a fault that fails no run.
func (t *Tool) report(msg string) {
	if t.cfg.Report != nil {
		t.cfg.Report(msg)
	}
}
