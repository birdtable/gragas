package main

import (
	"context"
	"errors"
	"flag"
	"github.com/birdtable/gragas/internal/dev/say"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// runPnpm is `pnpm [-C dir] [--tools dir] [--program pnpm|node] [--env K=V]... -- args`: the Taskfile's
// explorer members start the pinned pnpm (or node) through it, so the Taskfile joins no PATH of its own.
// The pinned tools folder goes first on the child's PATH, so a program pnpm starts finds the pinned node
// and never another; pnpm is told not to manage its own version, which would fetch one. The program is
// found in the tools folder alone, by the path with no suffix, which exec.LookPath completes with the
// system's own (hook.go does the same for task), and a folder with no such program is red, never a
// search of PATH. The child's streams are this process's, and its exit code is the command's.
func runPnpm(ctx context.Context, environ []string, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("pnpm", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "the folder the program runs in")
	tools := fl.String("tools", ".tools/bin", "the folder of the pinned programs")
	prog := fl.String("program", "pnpm", "pnpm or node")
	var extra []string
	fl.Func("env", "a variable the child is given, as K=V (repeatable)", func(s string) error {
		if k, _, ok := strings.Cut(s, "="); !ok || k == "" {
			return errors.New("want K=V")
		}
		extra = append(extra, s)
		return nil
	})
	// Flags end at the first "--": every word after it is the program's own, whatever it looks like.
	cut := slices.Index(args, "--")
	if cut < 0 || cut == len(args)-1 {
		talk.Text(usage)
		return exitUsage
	}
	if err := fl.Parse(args[:cut]); err != nil || fl.NArg() != 0 || (*prog != "pnpm" && *prog != "node") {
		talk.Text(usage)
		return exitUsage
	}
	rest := args[cut+1:]
	bin, err := filepath.Abs(*tools)
	if err != nil {
		talk.Text("gragas-dev pnpm: " + err.Error() + "\n")
		return exitFailed
	}
	path, err := exec.LookPath(filepath.Join(bin, *prog))
	if err != nil {
		talk.Text("gragas-dev pnpm: " + err.Error() + ": run task tools\n")
		return exitFailed
	}
	res, err := progRun{name: path, dir: *dir, args: rest, env: pnpmEnv(environ, bin, extra), timeout: pnpmTimeout, stdout: stdout, stderr: stderr}.do(ctx)
	code, why := taskEnd(res.Code, err)
	if why != "" {
		talk.Text("gragas-dev pnpm: " + *prog + " " + why + "\n")
	}
	return code
}

// pnpmTimeout ends a pnpm or node run that hangs (gragas#2263). It is a
// backstop past the longest step the Taskfile gives one, 20 minutes, whose own
// timeout ends it first.
const pnpmTimeout = 30 * time.Minute

// pnpmEnv is environ with bin first on PATH (pathFirst) and pnpm told not to manage its own version,
// and then the variables the caller gave, which win.
func pnpmEnv(environ []string, bin string, extra []string) []string {
	out := withoutVars(pathFirst(environ, bin), []string{"npm_config_manage_package_manager_versions"})
	out = append(out, "npm_config_manage_package_manager_versions=false")
	return append(out, extra...)
}
