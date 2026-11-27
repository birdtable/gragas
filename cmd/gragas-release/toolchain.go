package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/birdtable/go-proc/proc"
	"github.com/birdtable/go-proc/sandbox"

	"github.com/birdtable/gragas/internal/exit"
)

// startLimit is how long an unpacked binary has to answer one start test.
const startLimit = 30 * time.Second

// builtName is the name the unpack step gives the binary in the folder it runs
// it from, without the extension a system may run a program by: the file is
// proc.FileName of it for the archive's system (runName).
const builtName = ".release-gragas"

// runName is the file name of the binary in the folder it runs from, for the
// system goos the archive was built for (a name of the go command's GOOS).
func runName(goos string) string {
	return proc.FileName(goos, builtName)
}

// realToolchain runs the go command, git, goreleaser and the unpacked
// program. It is the one place this command starts a process. Every call but
// git's and goreleaser's has a fixed argv written here, and a folder is given
// to the program as its working folder and never as an argument. The build's
// git calls (build.go) are fixed there but the absolute folders of its clones
// and the commit git itself named; goreleaser's are fixed there but the
// absolute path of the tag's notes, and it runs from the absolute path build
// held to tools.yaml's sha256 for this host (heldGoreleaser) before anything
// started.
type realToolchain struct {
	getenv func(string) string
	// environ is the whole environment the build's git and goreleaser run
	// in: the person's.
	environ []string
}

// newToolchain is the toolchain that finds its programs and folders with getenv
// (main passes the process's), and runs git and goreleaser in environ (main
// passes the process's whole environment); nil runs neither.
func newToolchain(getenv func(string) string, environ []string) realToolchain {
	return realToolchain{getenv: getenv, environ: environ}
}

// Git runs git with args in dir, in the environment the toolchain was given,
// with no prompt for a password and no optional lock taken on the index. It
// answers git's standard output and its exit code; err is a git that did not
// run at all. Its standard error is dropped: the build names each failure
// itself.
func (t realToolchain) Git(ctx context.Context, dir string, args ...string) (string, int, error) {
	if t.environ == nil {
		return "", 0, errors.New("git: no environment was given, and none is inherited")
	}
	res, err := progRun{path: "git", dir: dir, args: args, env: over(t.environ, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0"), limit: readLimit}.do(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(res.Stdout), res.Code, nil
}

// Release runs the goreleaser at r.program with r.args in r.dir, in the
// environment the toolchain was given with r.extra over it (a later setting
// wins), its streams r's. r.program must be an absolute path, so no PATH and
// no working folder chooses which goreleaser runs.
func (t realToolchain) Release(ctx context.Context, r goreleaserRun) error {
	if t.environ == nil {
		return errors.New("goreleaser: no environment was given, and none is inherited")
	}
	if !filepath.IsAbs(r.program) {
		return fmt.Errorf("goreleaser %q is not an absolute path", r.program)
	}
	// run from r.program, as the dev tools run theirs: the path that runs is
	// the absolute one build held to its pin, never PATH's
	res, err := progRun{path: r.program, dir: r.dir, args: r.args, env: over(t.environ, r.extra...), limit: releaseLimit, stdout: r.stdout, stderr: r.stderr}.do(ctx)
	if err = ended(res, err); err != nil {
		return fmt.Errorf("goreleaser %s in %s: %w", strings.Join(r.args, " "), r.dir, err)
	}
	return nil
}

// Getenv reads key from the environment the toolchain was given.
func (t realToolchain) Getenv(key string) string { return t.getenv(key) }

// Host asks the go command which system it runs on, so no platform fact is read
// from the runtime here (arch.ports.1).
func (t realToolchain) Host(ctx context.Context) (system, error) {
	return hostIn(ctx, baseEnv(t.getenv))
}

// hostIn is Host with its whole environment given, so a test can hand it none.
func hostIn(ctx context.Context, env []string) (system, error) {
	out, err := capture(ctx, "", env, "go", "env", "GOHOSTOS", "GOHOSTARCH")
	f := strings.Fields(out)
	if err != nil || len(f) != 2 {
		return system{}, fmt.Errorf("go env did not name the host: %v %w", f, err)
	}
	return system{OS: f[0], Arch: f[1]}, nil
}

// where are the variables the go command needs to find things, and nothing
// that changes what it reads: the ones go-proc names a child on this system
// needs (proc.Needed: the program path, the language, the home names and the
// system's own), and Go's module folders. No temp folder: go env, go
// telemetry off and git ls-tree write none.
func where() []string {
	return append(proc.Needed(), "GOPATH", "GOMODCACHE")
}

// baseEnv is the whole environment of every go command, not a layer on the
// person's: the variables in where, the installed toolchain and no other, and
// no workspace. A GOFLAGS or GOOS of the shell it runs in never reaches it.
func baseEnv(getenv func(string) string) []string {
	env := []string{"GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=-mod=readonly"}
	for _, k := range where() {
		if v := getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// Tree runs git ls-tree -r -z --full-tree HEAD in the clone repo, with the
// variables of where and no system config, so no setting of the machine
// changes what it lists.
func (t realToolchain) Tree(ctx context.Context, repo string) (string, error) {
	env := []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0"}
	for _, k := range where() {
		if v := t.getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return capture(ctx, repo, env, "git", "ls-tree", "-r", "-z", "--full-tree", "HEAD")
}

// Start runs the binary dir/name (runName's, for the archive's system) with one of
// the start tests, in dir, with startEnv's environment and nothing of the
// person running it. It runs behind go-proc's block on every system that
// gives one: this program is started again as the blocked child, which sets
// the block on itself and execs the binary (startCommand). Where sandbox.Check
// finds no block it is refused, by name, and nothing starts: no start test
// runs unblocked.
func (realToolchain) Start(ctx context.Context, dir, name string, test startTest) (string, error) {
	// With no folders, Check is Run's and reads no program path.
	if _, err := sandbox.Check(sandbox.Folders{}, ""); err != nil {
		return "", fmt.Errorf("release: the start test %q is refused, since this system gives it no network block: %w", test, err)
	}
	// This program, as the system names it to os.Executable (never a /proc path of a system's own).
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("release: this program's own path is not known: %w", err)
	}
	argv, err := startCommand(self, dir, name, test)
	if err != nil {
		return "", err
	}
	// startCommand's argv: this program, the block's word and the binary the
	// step wrote, each by its path; both streams in one, in the order said
	var said bytes.Buffer
	res, err := progRun{path: argv[0], dir: dir, args: argv[1:], env: startEnv(dir), limit: startLimit, stdout: &said, stderr: &said}.do(ctx)
	return said.String(), ended(res, err)
}

// Network says how a start test is kept from the network on this system, as
// sandbox.Check finds it: the block and what it closes, or why there is none.
func (realToolchain) Network() string {
	return networkText(sandbox.Check(sandbox.Folders{}, ""))
}

// networkText is what Network says of the Block sandbox.Check found, or of
// its error.
func networkText(b sandbox.Block, err error) string {
	if err != nil {
		return "with no network block on this system, so its start tests are refused: " + err.Error()
	}
	var closed, open []string
	for _, f := range []struct {
		name string
		on   bool
	}{
		{"the network", b.Network},
		{"other daemons", b.Daemons},
		{"the terminal", b.Terminal},
		{"signals out", b.Signals},
		{"the service manager", b.ServiceManager},
	} {
		if f.on {
			closed = append(closed, f.name)
		} else {
			open = append(open, f.name)
		}
	}
	text := "behind go-proc's block (go-proc/sandbox), which closes " + strings.Join(closed, ", ")
	if len(open) > 0 {
		text += " and leaves open " + strings.Join(open, ", ")
	}
	return text
}

// startCommand is the argv of one start test of the binary in dir: self,
// this program by its absolute path, as the blocked child, which sets the
// block and execs the binary dir/name by its path with the test's words.
// Nothing in it comes from the person's environment, and there is no argv
// without the block. name is one file name in dir, never a path.
func startCommand(self, dir, name string, test startTest) ([]string, error) {
	words := startArgs(test)
	if words == nil {
		return nil, fmt.Errorf("no start test %d", test)
	}
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("the start test's folder %s is not an absolute path", dir)
	}
	if !filepath.IsAbs(self) {
		return nil, fmt.Errorf("this program's path %q is not an absolute path, so it cannot start the block", self)
	}
	if name == "" || name == "." || name != filepath.Base(name) || !filepath.IsLocal(name) {
		return nil, fmt.Errorf("the start test's binary %q is not one file name in its folder", name)
	}
	argv := append([]string{filepath.Join(dir, name)}, words...)
	return append([]string{self, exit.BlockedWord, "--"}, argv...), nil
}

// startEnv is the whole environment of a start test: the system's own
// variables a program here needs to start (proc.SystemEnv, none off Windows),
// and a home, configuration, cache, state and temp folder of its own beneath
// dir/startHome under each name this system reads (proc.HomeEnv); no PATH,
// no proxy and no variable of the person's. madeFolders makes its folders.
func startEnv(dir string) []string {
	return append(proc.SystemEnv(), proc.HomeEnv(filepath.Join(dir, startHome))...)
}

// capture runs the program name, found in this process's PATH, with args in
// dir, with the environment env and no other, for no longer than readLimit, and
// answers its stdout. A failure carries what the program said on stderr.
// A nil env is refused: a caller that gives none has lost the one it built,
// and go-proc would run the program in a bare one instead.
func capture(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	what := strings.Join(append([]string{name}, args...), " ")
	if env == nil {
		return "", fmt.Errorf("%s: no environment was given, and none is inherited", what)
	}
	res, err := progRun{path: name, dir: dir, args: args, env: env, limit: readLimit}.do(ctx)
	if err = ended(res, err); err != nil {
		return string(res.Stdout), fmt.Errorf("%s: %w: %s", what, err, strings.TrimSpace(string(res.Stderr)))
	}
	return string(res.Stdout), nil
}

// readLimit is how long a go env, a git ls-tree or one of the guard's git
// calls may take: each reads a little and answers at once.
const readLimit = 2 * time.Minute

// releaseLimit is how long goreleaser may take to build, pack and sum every
// archive of a release.
const releaseLimit = 30 * time.Minute

// outputCap is the most of each stream a run keeps; a program that writes more
// is ended.
const outputCap = 64 << 20

// progRun is one run of a program: path is an absolute path, or a name with
// no separator that run finds in this process's PATH, as os/exec found it.
// env is the program's whole environment, limit its bound, and stdout and
// stderr, when not nil, are given its streams as it writes them.
type progRun struct {
	path, dir      string
	args, env      []string
	limit          time.Duration
	stdout, stderr io.Writer
}

// do is the one start of a process in this command: go-proc's, so the
// program's whole tree ends with the run, at its limit or when ctx ends.
func (r progRun) do(ctx context.Context) (proc.Result, error) {
	path := r.path
	if filepath.Base(path) == path {
		found, err := exec.LookPath(path)
		if err != nil {
			return proc.Result{Code: -1}, err
		}
		path = found
	}
	return proc.Run(ctx, proc.Cmd{Path: path, Args: r.args, Dir: r.dir, Env: r.env, Stdout: r.stdout, Stderr: r.stderr, Timeout: r.limit, MaxOutput: outputCap, Drain: proc.DefaultDrain})
}

// ended is the error of a run that did not exit 0: err when the run did not
// end on its own, else the exit status in os/exec's words.
func ended(res proc.Result, err error) error {
	switch {
	case err != nil:
		return err
	case res.Code < 0:
		return errors.New("ended by a signal")
	case res.Code != 0:
		return fmt.Errorf("exit status %d", res.Code)
	}
	return nil
}

// over is environ with each variable of extra set as extra sets it: environ's
// own settings of those names are dropped, so the list holds one setting of
// each and the child's value does not rest on which of two settings a layer
// below keeps (os/exec, which go-proc runs through, keeps the last).
func over(environ []string, extra ...string) []string {
	names := make(map[string]bool, len(extra))
	for _, kv := range extra {
		name, _, _ := strings.Cut(kv, "=")
		names[name] = true
	}
	out := make([]string, 0, len(environ)+len(extra))
	for _, kv := range environ {
		if name, _, _ := strings.Cut(kv, "="); !names[name] {
			out = append(out, kv)
		}
	}
	return append(out, extra...)
}
