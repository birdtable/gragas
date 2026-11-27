package tool

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/birdtable/go-proc/proc"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// launch is one start: the tool's name and the shape for the record, the
// checked path, the argv, the folder, the deadline and the memory limit in
// MiB, 0 for none.
type launch struct {
	name, shape, path string
	argv              []string
	dir, root         string
	// env is the PATH the run is given: the config's, or a pinned find's
	// with the project's folders left out (pinnedPath).
	env  string
	wait time.Duration
	mib  int
	// stdin is what the run reads, nil for nothing; max is the bytes kept
	// of each stream, 0 for the adapter's cap. A step's run sets both.
	stdin []byte
	max   int
	// home is the Home of this run alone, "" for the config's.
	home string
	// cache is NAME=folder of each cache variable the step's grant names,
	// nil for none.
	cache []string
	// homeCache is the folder the Home's cache is kept in between steps of
	// the grant (proc.HomeEnvCache), "" for one fresh with the Home.
	homeCache string
}

// capOf is the bytes kept of each stream of l's run.
func (t *Tool) capOf(l launch) int { return cmp.Or(l.max, t.max) }

// drain is how long a run reads an output pipe a process that left its tree
// still holds once the tree is killed (go-proc's Cmd.Drain): the 10 s go-proc
// gave a zero Drain before v0.4.0, which honours a zero as none.
const drain = 10 * time.Second

// start is the one place a tool is started, through go-proc's proc.Run:
// path with argv, in dir, with the environment env answers, in a tree of
// its own that is killed whole when the run ends, at the cap, at the
// deadline, and when the tool exits, so a grandchild it left running ends
// with it. A run held to a memory limit is held as memoryHold says, and is
// not counted past its limit. Every start is recorded. A
// tool that exits non-zero is not an error; one the adapter ended or did
// not count is an *Ended with what it printed up to the cap.
func (t *Tool) start(ctx context.Context, l launch) (model.Output, error) {
	h := cmp.Or(l.home, t.cfg.Home)
	if err := t.checkHome(h, l); err != nil {
		return model.Output{}, err
	}
	if err := makeHome(h); err != nil {
		return model.Output{}, err
	}
	env, err := t.env(l.env, h, l.cache, l.homeCache)
	if err != nil {
		return model.Output{}, err
	}
	// gosec's G204 sees no exec here; proc.Run starts the path the
	// grant's find checked, with the words of the plan's step.
	cmd := proc.Cmd{
		Path: filepath.FromSlash(l.path), Args: l.argv, Dir: filepath.FromSlash(l.dir), Env: env,
		Timeout: l.wait, MaxOutput: t.capOf(l), Drain: drain,
	}
	if l.stdin != nil {
		cmd.Stdin = bytes.NewReader(l.stdin)
	}
	c, judgedAfter, err := memoryHold(l, cmd)
	if err != nil {
		return model.Output{}, err
	}
	res, err := proc.Run(ctx, c)
	return t.outcome(ctx, l, judgedAfter, res, err)
}

// outcome is what a run of l comes to once go-proc's Run answered res and
// err under the caller's ctx: refused before it started, the caller's
// cancel, a start that failed, ended at its cap, its deadline or the bound
// its tree was held to, not counted on its peak or on a process it left, a
// process outside its tree named, or counted. Every run is recorded. It
// reads nothing of the system, so the mapping holds the same everywhere,
// and a test plants go-proc's answers to hold it (outcome_test.go), where
// go-proc's own tests hold that a tree is ended and a process left is seen.
func (t *Tool) outcome(ctx context.Context, l launch, judgedAfter bool, res proc.Result, err error) (model.Output, error) {
	rec := Ran{Tool: l.name, Shape: l.shape, Argv: append([]string{l.path}, l.argv...), Dir: l.dir, Code: res.Code, PeakMiB: -1, JudgedAfter: judgedAfter, Left: res.Left}
	if peak, kind, read := peakMiB(res); read {
		rec.PeakMiB, rec.PeakKind = peak, kind
	}
	if errors.Is(err, proc.ErrRefused) {
		t.record(rec)
		return model.Output{}, startRefused(l, err)
	}
	cut, escaped := endedBy(err, proc.WhyCap), endedBy(err, proc.WhyEscaped)
	if !cut && ctx.Err() != nil {
		t.record(rec)
		return model.Output{}, ctx.Err()
	}
	if neverRan(err) {
		t.record(rec)
		return model.Output{}, err
	}
	deadline := endedBy(err, proc.WhyDeadline)
	out := model.Output{Code: res.Code, Stdout: res.Stdout, Stderr: res.Stderr, JudgedAfter: judgedAfter}
	if cut || deadline {
		out.Cut, out.Code = cut, -1
	}
	limit := endedBy(err, proc.WhyLimit)
	if limit {
		out.Code = -1
	}
	if ended := t.endOf(l, cut, deadline, limit, judgedAfter, res); ended != nil {
		if escaped {
			ended.Says += ", and a process outside its tree still holds its output"
		}
		rec.Ended = ended.Why
		t.record(rec)
		return out, ended
	}
	t.record(rec)
	if escaped {
		return model.Output{}, fmt.Errorf("%s ended, and a process outside its tree still holds its output: %w", l.name, err)
	}
	if _, ok := errors.AsType[*exec.ExitError](err); ok || err == nil {
		return out, nil
	}
	return model.Output{}, err
}

// startRefused is the refusal of a run go-proc refused before it started.
// A path go-proc starts no program at is said in this
// adapter's words, with the path and a fix that works: one that holds =, on
// every system, and on Windows a batch file or a file that is not a program,
// which go-proc says by proc.ErrBatch and proc.ErrNotProgram,
// so no system's rules are kept here. The path is the find's, absolute and with
// its links followed, so go-proc's other refusals of a path never come here.
// Any other refusal
// is go-proc's of the run's set-up or of its memory limit, which every
// level holds, so no setting of the tool's fixes it and it is reported.
func startRefused(l launch, err error) error {
	at := l.name + " at " + hosttrust.Quote(filepath.FromSlash(l.path))
	if strings.ContainsRune(l.path, '=') {
		return refused(at+" is not started: its path, links followed, holds =, and go-proc starts no program whose path holds = (an env-style launcher reads it as a setting)",
			"install "+l.name+" in a folder whose path holds no =, and name that path under tool-paths in the settings file or in the tools file")
	}
	const programFix = " itself, an .exe, under tool-paths in the settings file or in the tools file; a tool installed only as a .cmd shim, as npm installs each node tool, has none and cannot be run here"
	switch {
	case errors.Is(err, proc.ErrBatch):
		return refused(at+" is not started: it is a batch file, which Windows runs through cmd.exe, and cmd.exe reads its arguments by rules of its own, by which an argument can run a second command, so go-proc starts no batch file",
			"name the program file of "+l.name+programFix)
	case errors.Is(err, proc.ErrNotProgram):
		return refused(at+" is not started: it is not a program file (.exe or .com), and Windows hands such a file, or the batch file PATHEXT finds for it, to a program of its own that reads the arguments by rules of its own, so go-proc does not start it",
			"name the program file of "+l.name+programFix)
	}
	return refused("go-proc refused to start "+l.name+": "+err.Error(), "report the fault with this line: no setting of "+l.name+"'s fixes it")
}

// memoryHold is c as it runs for l's memory limit, whether that limit is
// judged after the run alone, or the refusal of l before anything starts.
// A launch with no limit, the default, since
// gragas sets none of its own, runs c as it is: no
// TreeMemory and no peak are asked of go-proc, so it runs on every host,
// one that holds no run's memory among them. A launch with a limit set
// asks go-proc, by proc.HoldsTreeMemory (go-proc#417), what this system can
// hold here, with no branch on the system of its own:
//
//   - c held to treeMemory and reading its peak: where go-proc holds it (a
//     delegated cgroup on Linux, the Job Object on Windows), the run goes so,
//     the kernel ends it past that bound (proc.WhyLimit, EndedMemory), and its
//     peak is judged against the limit after, as ever;
//   - where go-proc cannot hold the bound here but reads a peak
//     (proc.TreeMemoryJudgedAfter), c reading its peak alone: a system that
//     reads one (macOS's resident peak) runs it unheld and judges the peak
//     after, and says so (judged after); one that reads none
//     (proc.TreeMemoryRefused, Linux with no delegated cgroup) is refused by
//     name, with the fix, since no limit would be seen to hold;
//   - any other refusal is c's own fault, and is reported as go-proc's
//     refusal of the run (startRefused).
//
// go-proc's Run stays the authority: a Holds that answers nil does not
// promise a run, and a refusal Run makes after it is startRefused's as ever.
func memoryHold(l launch, c proc.Cmd) (proc.Cmd, bool, error) {
	if l.mib < 1 {
		return c, false, nil
	}
	c.ReadPeak = true
	capped := c
	capped.Limits.TreeMemory = treeMemory(l.mib)
	hold, _, err := proc.HoldsTreeMemory(capped)
	switch {
	case err != nil:
		return c, false, startRefused(l, err)
	case hold == proc.TreeMemoryHeld:
		return capped, false, nil
	case hold == proc.TreeMemoryJudgedAfter:
		return c, true, nil
	}
	return c, false, unheld(l)
}

// unheld is the refusal of a run held to a memory limit that this system
// neither holds while it runs nor reads after it: Linux with no cgroup
// systemd delegated to gragas, where go-proc refuses Limits.TreeMemory and
// reads no peak. It matches port.ErrNotHeld, so a caller that cannot vouch
// for a tool it did not run ends red on it; its fix names the
// delegated cgroup docs/checks.md sets up. Its word is not ready, not
// refused: the step fits its grant, and it is the host that is not set up
// (spec S40, as a missing input is).
func unheld(l launch) error {
	r, err := model.NewRefusal(model.NotReady, l.name+" is held to the memory limit of "+itoa(l.mib)+" MiB, and this system can neither hold the run's tree to it nor read its peak memory after it (go-proc holds and reads a tree's memory on Linux only in a cgroup systemd delegated to it), so it is not run",
		nil, "run gragas as a user service with Delegate=yes and DelegateSubgroup=app (docs/checks.md, The delegated cgroup), or run it at the sandboxed level")
	if err != nil {
		return errors.Join(port.ErrRefused, port.ErrNotHeld, err)
	}
	return &Refused{Refusal: r, notHeld: true}
}

// treeMemory is the bound, in bytes, go-proc holds the whole tree of a run
// with a limit of mib to while it runs: twice mib, the bound the data
// segment's backstop was before go-proc v0.4.0. It is a guard against a
// runaway, not the limit: the limit itself is judged on the peak after the
// run (overMemory). On Linux it is the run's cgroup's memory.max, which
// counts the page cache and every process of the tree, and a tool may write
// its own memory.max: it is no security boundary. The factor waits for a
// measured table.
func treeMemory(mib int) uint64 {
	if mib < 1 {
		return 0 // never: memoryHold holds no run with no limit; the bound is what gosec G115 reads
	}
	return uint64(mib) * 2 << 20
}

// endedBy is whether err holds a proc.Ended for why, through every error it
// joins or wraps: a run ended at its cap may also be one a process outside
// its tree held the output of, so each Why is asked for on its own.
func endedBy(err error, why string) bool { return errors.Is(err, proc.Ended{Why: why}) }

// neverRan is whether err says the program never ran: go-proc's Run
// returns no error for the program's own exit, whatever its code, and an
// Ended for a run it ended, so any other error is a start that failed (a
// file that cannot be exec'd, a wrapper that cannot start, a system with
// no way to kill a tree). Such a run read no peak, and the start's own
// error is what it says, never a memory limit that was not seen to hold.
func neverRan(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[proc.Ended](err); ok {
		return false
	}
	_, exited := errors.AsType[*exec.ExitError](err)
	return !exited
}

// checkHome holds the project to the Home variables' check, when they are
// set: the project is judged by the folder it is, not by how it is spelled.
func (t *Tool) checkHome(h string, l launch) error {
	if h == "" {
		return nil
	}
	return t.homeOK(h, l.root, l.dir)
}

// endOf is why a run is ended or not counted, nil for one counted as it is:
// it printed past its cap, ran past its deadline, held more memory at its
// peak than its limit, passed the bound go-proc held its tree to (limit,
// proc.WhyLimit), or, held to a limit, may have left a member of its tree
// running. judgedAfter is a run whose limit nothing held while it ran.
func (t *Tool) endOf(l launch, cut, deadline, limit, judgedAfter bool, res proc.Result) *Ended {
	switch {
	case cut:
		return &Ended{Tool: l.name, Why: EndedCap, Says: "printed more than its cap of " + itoa(t.capOf(l)) + " bytes"}
	case deadline:
		return &Ended{Tool: l.name, Why: EndedDeadline, Says: "ran past its deadline of " + l.wait.String(), Limit: "the deadline of " + l.wait.String()}
	case l.mib < 1:
		return nil
	case limit:
		bound := "the memory limit of " + itoa(l.mib) + " MiB"
		return &Ended{Tool: l.name, Why: EndedMemory, Limit: bound,
			Says: "passed the bound of " + itoa(int(treeMemory(l.mib)>>20)) + " MiB its tree was held to for " + bound + ", and the system ended the tree"}
	}
	peak, kind, read := peakMiB(res)
	if e := overMemory(l.name, peak, kind, read, judgedAfter, l.mib); e != nil {
		return e
	}
	if res.Left {
		return leftUnread(l.name, l.mib)
	}
	return nil
}

// leftUnread is the *Ended of a run held to a memory limit of mib whose tree
// held a process when the tool exited, by go-proc's Result.Left.
// The peak go-proc reads is the tool's and that of each descendant it waited
// for, so that process's peak was never read: the limit was not seen to
// hold, and the result is not counted. proc.Run kills the tree all the same.
// Left is true when go-proc could not list the tree, so the run fails
// closed.
func leftUnread(name string, mib int) *Ended {
	limit := "the memory limit of " + itoa(mib) + " MiB"
	return &Ended{Tool: name, Why: EndedMemory, Limit: limit,
		Says: "left a process of its tree running when it exited, whose peak memory was never read, so " + limit + " was not seen to hold; the tree was ended"}
}

// overMemory is the *Ended of a run whose peak memory, peak MiB of kind when
// read, passed mib, nil for one that held less. A peak that was not read is
// over: the limit was not seen to hold. A run judgedAfter says that nothing
// held it while it ran.
func overMemory(name string, peak int, kind string, read, judgedAfter bool, mib int) *Ended {
	limit := "the memory limit of " + itoa(mib) + " MiB"
	switch {
	case !read:
		return &Ended{Tool: name, Why: EndedMemory, Says: "gave no reading of its peak memory against " + limit, Limit: limit}
	case peak > mib:
		says := "held " + itoa(peak) + " MiB at its peak (" + kind + "), over " + limit
		if judgedAfter {
			says += "; this system judges memory after the run, and only a sandboxed run caps it"
		}
		return &Ended{Tool: name, Why: EndedMemory, Says: says, Limit: limit}
	}
	return nil
}

// peakMiB is the run's peak memory in MiB, rounded up, what it counts
// (go-proc's PeakKind), and whether it was read: go-proc's Result.Peak of a
// run that set ReadPeak. On Linux it is the run's cgroup's memory.peak, the
// whole tree's charge, page cache included (tree-charged), and never wait4's
// maxrss, so never this program's resident set carried across an exec;
// on macOS the larger of the tool's own resident peak and
// that of each descendant it waited for (process-resident); on Windows the
// most one process of the job committed (process-committed). A run go-proc
// gave no reading of, or one too large to round up without overflow, is not
// counted (overMemory); where this system reads none, no run held to a limit
// starts (memoryHold).
func peakMiB(res proc.Result) (int, string, bool) {
	if !res.PeakRead || res.PeakKind == proc.PeakNone || res.Peak < 0 || res.Peak > math.MaxInt64-(1<<20-1) {
		return 0, "", false
	}
	return int((res.Peak + 1<<20 - 1) >> 20), string(res.PeakKind), true
}

// record keeps one run's record.
func (t *Tool) record(r Ran) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ran = append(t.ran, r)
}

// env is the whole environment of a run: PATH, the run's own, and LANG,
// each when it is set, and go-proc's proc.HomeEnv of the Home when the config
// has one (the home, config, cache, state and temp folders each in a folder
// of its own below it), with the system's part, go-proc's proc.SystemEnv: the
// machine-wide names a program on this system needs to start or to find the
// system's own files, which go-proc names and gragas never does.
// A step's cache variables, NAME=folder, come first: the
// opener refuses a manifest that names one Reserved holds, and a name of
// env's own given after one would win all the same, as go-proc and os/exec
// keep the last of a name given twice.
// With homeCache, the Home's cache is that folder, kept between steps of the
// grant, and the rest of the Home is fresh (go-proc's proc.HomeEnvCache,
// which names the variable, so gragas names none); a homeCache that is the
// Home, lies beneath it or holds it is refused, and nothing starts.
// It is never nil, since proc.Run gives a nil Env this process's PATH and
// HOME.
func (t *Tool) env(pathEnv, home string, cache []string, homeCache string) ([]string, error) {
	env := make([]string, 0, 2+len(cache))
	env = append(env, cache...)
	if pathEnv != "" {
		env = append(env, envPath+"="+filepath.FromSlash(pathEnv))
	}
	if t.cfg.Lang != "" {
		env = append(env, envLang+"="+t.cfg.Lang)
	}
	env = append(env, proc.SystemEnv()...)
	if home != "" {
		// GOENV=off and GOTOOLCHAIN=local keep the go command from reading
		// an env file below HOME or fetching a toolchain.
		// TMPDIR is the Home too: a tool may keep one lock in the temp
		// folder for the whole machine and refuse to run while another run
		// of it holds that lock, so a run with a Home of its
		// own has a temp folder of its own.
		homeEnv := proc.HomeEnv(filepath.FromSlash(home))
		if homeCache != "" {
			var err error
			if homeEnv, err = proc.HomeEnvCache(filepath.FromSlash(home), homeCache); err != nil {
				return nil, refused("the Home's cache folder "+hosttrust.Quote(filepath.ToSlash(homeCache))+" is not apart from the Home "+hosttrust.Quote(home),
					"point the cache folder and the Home at folders apart, neither beneath the other")
			}
		}
		env = append(env, homeEnv...)
		// GOFLAGS=-modcacherw leaves each module the go command extracts
		// into the Home writable, as it always was; the Home's removal gives
		// a folder left read-only its write bit all the same (go-files'
		// confine.RemoveAllOwned), so this is kept, not relied on.
		env = append(env, envGoEnv+"=off", envGoToolchain+"=local", envGoFlags+"=-modcacherw")
	}
	return env, nil
}
