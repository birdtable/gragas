package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/go-proc/gitenv"
	"github.com/birdtable/go-proc/proc"
	"github.com/birdtable/gragas/internal/dev/say"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// hookSources are the files that make up the hooks, from the top of the
// tree: the two one-line hooks git runs; this file, the code they run;
// main.go and support.go, which go run builds it with and which dispatch to
// it; and go.mod and go.sum, which pin what it is built from. A pushed commit
// is judged only when it carries these as they are here (gragas#1297): the
// hook that runs is the checkout's, so an older checkout's hook, with fewer
// checks, would otherwise judge a newer commit.
func hookSources() []string {
	return []string{"hooks/pre-commit", "hooks/pre-push", "cmd/gragas-dev/hook.go", "cmd/gragas-dev/main.go", "cmd/gragas-dev/support.go", "go.mod", "go.sum"}
}

// runHook is the hook command. git's pre-commit and pre-push hooks are each
// one line that runs it (gragas#1655), so the hooks run the same Go on every
// system. pre-commit runs the clone's own task pre-commit. pre-push reads
// git's ref lines on stdin and runs task pre-push on the tree each pushed ref
// carries (docs/checks.md, The Taskfile). Neither changes a tracked file or
// writes git config. environ is the environment git gave the hook, which main
// reads (cli.config.3).
func runHook(ctx context.Context, environ, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	if len(args) == 0 {
		talk.Text(usage)
		return exitUsage
	}
	// git passes the pre-push hook the remote's name and URL, which it does
	// not read; pre-commit gets nothing.
	switch {
	case args[0] == "pre-commit" && len(args) == 1:
	case args[0] == "pre-push" && len(args) <= 3:
	default:
		talk.Text(usage)
		return exitUsage
	}
	h, err := newHook(ctx, ".", environ, stdout, stderr)
	if err != nil {
		talk.Text(args[0] + ": " + err.Error() + "\n")
		return exitFailed
	}
	if args[0] == "pre-commit" {
		return h.preCommit(ctx)
	}
	return h.prePush(ctx, stdin)
}

// hook is a run of a hook in the clone at root, with the clone's own task and
// the environment git gave the hook, cut to the allowlist (allowed). index is
// the GIT_INDEX_FILE of that environment, which only preCommit hands on.
type hook struct {
	root, task     string
	env            []string
	index          string
	stdout, stderr io.Writer
	say            *say.Speaker
}

// newHook finds git's top level from dir and the clone's .tools/bin/task
// there. exec.LookPath of the path with no suffix finds the program with
// the system's own suffix where there is one. env is cut to the allowlist
// first (allowed), so no git, task or member the hook starts is given a
// variable the gate has not named. GIT_INDEX_FILE is not on it: it is kept
// aside, for preCommit alone (indexOf).
func newHook(ctx context.Context, dir string, env []string, stdout, stderr io.Writer) (*hook, error) {
	index := indexOf(env)
	env, err := allowed(ctx, env)
	if err != nil {
		return nil, err
	}
	out, err := hookGit(ctx, dir, env, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	root := filepath.Clean(filepath.FromSlash(strings.TrimSpace(out)))
	task, err := exec.LookPath(filepath.Join(root, ".tools", "bin", "task"))
	if err != nil {
		return nil, fmt.Errorf("%s is not there: run  CGO_ENABLED=0 go build -C tools -trimpath -o ../.tools/bin/task github.com/go-task/task/v3/cmd/task  then  .tools/bin/task tools (%w)",
			filepath.Join(root, ".tools", "bin", "task"), err)
	}
	return &hook{root: root, task: task, env: env, index: index, stdout: stdout, stderr: stderr, say: &say.Speaker{W: stderr}}, nil
}

// preCommit runs task pre-commit in the clone, with the environment git gave
// the hook and the index it names: the staged checks read the stage the commit
// is built from, a temporary index under git commit -a or with paths.
func (h *hook) preCommit(ctx context.Context) int {
	env := h.env
	if h.index != "" {
		env = append(slices.Clone(env), "GIT_INDEX_FILE="+h.index)
	}
	return h.runTask(ctx, h.root, env, hookTaskTimeout, "pre-commit")
}

// indexOf is the value of env's GIT_INDEX_FILE, found in any case, or "". git
// names the index to a pre-commit hook alone; anywhere else the name is the
// caller's, and an index of theirs (one at the base, or an empty one) would make
// a changed input read as unchanged and empty a member's list of files, so the
// pre-push hook and landing run hand no git and no task one (gragas#2181).
func indexOf(env []string) string {
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); strings.EqualFold(k, "GIT_INDEX_FILE") {
			return v
		}
	}
	return ""
}

// pushed is what the pre-push hook makes of git's ref lines.
type pushed struct {
	here    bool     // a pushed tree is the clean checkout's
	away    []string // the commits whose tree is not, one a tree
	seen    int      // the trees pushed
	deleted []string // the refs that are only deleted
	skip    string   // SKIP_UNCHANGED: "0" when refs/heads/main is pushed
}

// prePush runs task pre-push on each tree the push carries: where it is the
// clean checkout's, here; otherwise in a throwaway worktree at that commit,
// removed after, with this clone's .tools linked in. A push that only
// deletes refs runs no tier. A member whose inputs are the base's is skipped
// only when no pushed ref is refs/heads/main: the push that lands runs every
// member whole.
func (h *hook) prePush(ctx context.Context, stdin io.Reader) int {
	p, err := h.read(ctx, stdin)
	if err != nil {
		h.say.Text("pre-push: refused: " + err.Error() + "\n")
		return exitFailed
	}
	if p.seen == 0 && len(p.deleted) > 0 {
		(&say.Speaker{W: h.stdout}).Text("pre-push: only deletes " + strings.Join(p.deleted, " ") + "; no code to check\n")
		return exitDone
	}
	if p.here || p.seen == 0 {
		(&say.Speaker{W: h.stdout}).Text("pre-push: running task pre-push (git push --no-verify skips it)\n")
		if code := h.runTask(ctx, h.root, h.env, hookTaskTimeout, "pre-push", "SKIP_UNCHANGED="+p.skip); code != 0 {
			return code
		}
	}
	for _, sha := range p.away {
		if code := h.away(ctx, sha, p.skip); code != 0 {
			return code
		}
	}
	return exitDone
}

// read reads git's ref lines, one a ref: <local ref> <local sha> <remote ref>
// <remote sha>. A commit whose hook sources differ from the ones that run
// here is refused, whatever else is pushed; so is one that does not merge
// into origin/main (mergeable.go), before any member runs.
func (h *hook) read(ctx context.Context, stdin io.Reader) (pushed, error) {
	p := pushed{skip: "1"}
	head, err := h.git(ctx, "rev-parse", "HEAD^{tree}")
	if err != nil {
		head = "none"
	}
	status, err := h.git(ctx, "status", "--porcelain")
	if err != nil {
		return p, err
	}
	clean := strings.TrimSpace(status) == ""
	seen := map[string]bool{}
	lines := bufio.NewScanner(stdin)
	for lines.Scan() {
		f := strings.Fields(lines.Text())
		if len(f) != 4 {
			continue
		}
		sha, ref := f[1], f[2]
		if strings.Trim(sha, "0") == "" {
			p.deleted = append(p.deleted, ref)
			continue
		}
		if err := h.refused(ctx, sha); err != nil {
			return p, err
		}
		tree, err := h.git(ctx, "rev-parse", sha+"^{tree}")
		if err != nil {
			return p, err
		}
		tree = strings.TrimSpace(tree)
		if ref == "refs/heads/main" {
			p.skip = "0"
		}
		if seen[tree] {
			continue
		}
		seen[tree] = true
		p.seen++
		if tree == strings.TrimSpace(head) && clean {
			p.here = true
		} else {
			p.away = append(p.away, sha)
		}
	}
	return p, lines.Err()
}

// refused is the error for a pushed sha that carries another hook than the one
// that runs, or does not merge into origin/main; nil for one that is neither.
func (h *hook) refused(ctx context.Context, sha string) error {
	if err := h.sameHook(ctx, sha); err != nil {
		return err
	}
	return h.mergeable(ctx, sha)
}

// sameHook refuses sha unless it carries the hook sources as the working
// tree holds them, which is what runs: git diff --quiet exits 1 on a
// difference, and anything else but 0 is an error, refused too.
func (h *hook) sameHook(ctx context.Context, sha string) error {
	if _, err := runRead(ctx, "git", "", h.env, gitenv.ReadArgs(h.root, append([]string{"diff", "--quiet", sha, "--"}, hookSources()...)...)); err != nil {
		return fmt.Errorf("%s carries a hook (%s) that differs from the one that runs here, so push from a checkout of the branch you push (its hook differs)",
			h.short(ctx, sha), strings.Join(hookSources(), ", "))
	}
	return nil
}

// short is sha's short name, or sha itself when git gives none.
func (h *hook) short(ctx context.Context, sha string) string {
	out, err := h.git(ctx, "rev-parse", "--short", sha)
	if err != nil {
		return sha
	}
	return strings.TrimSpace(out)
}

// away runs task pre-push on sha in a throwaway worktree. No node_modules is
// linked in: pnpm refuses a task-run state folder that is a link, and a
// node_modules whose hoist folder is one (gragas#1033). So the tree gets its
// own, explorer's and tools/.node's, from task tools-pnpm, offline, from the
// store this clone's install recorded (gragas#1522): with no store named,
// pnpm picks one by a probe on the tree's file system, which need not be the
// store that holds the locked packages. The worktree's folder is
// host/trust's TreeFolder (gragas#2264): fresh at 0700 in the account's own
// temp folder for work trees, kept apart from the clone, never swept by
// another run, and not inside the folder a member's FreshFolder is made in.
func (h *hook) away(ctx context.Context, sha, skip string) (code int) {
	short := h.short(ctx, sha)
	dir, removeDir, err := hosttrust.TreeFolder([]string{h.root}, "gragas-pre-push-")
	if err != nil {
		h.say.Text("pre-push: refused: no folder for a worktree of " + short + ": " + err.Error() + "\n")
		return exitFailed
	}
	tree := filepath.Join(dir, "tree")
	defer func() {
		if err := h.remove(ctx, dir, tree, removeDir); err != nil {
			h.say.Text("pre-push: " + err.Error() + "\n")
			if code == 0 {
				code = exitFailed
			}
		}
	}()
	if _, err := h.git(ctx, "worktree", "add", "--quiet", "--detach", tree, sha); err != nil {
		h.say.Text("pre-push: refused: " + short + " is pushed and is not the clean HEAD, and no worktree could be made to check it. Push from a checkout of " + short + ". (" + err.Error() + ")\n")
		return exitFailed
	}
	if err := linkTools(h.root, tree); err != nil {
		h.say.Text("pre-push: refused: " + err.Error() + "\n")
		return exitFailed
	}
	(&say.Speaker{W: h.stdout}).Text("pre-push: " + short + " is pushed and is not the clean HEAD, so task pre-push runs on it in " + tree + "\n")
	modules := filepath.Join(h.root, "explorer", "node_modules", ".modules.yaml")
	store, ok := pnpmStore(modules)
	if !ok {
		h.say.Text("pre-push: refused: " + modules + " names no pnpm store, so " + short + "'s tree cannot be installed offline: run  task tools  in this clone\n")
		return exitFailed
	}
	// gitenv.Repo drops every name that points a git at a repository, so no
	// git the tier runs in the worktree is pointed back at this clone. h.env,
	// the allowlist (allowed), holds none today; this holds if it ever does.
	env := gitenv.Repo(h.env)
	install := append(slices.Clone(env), "pnpm_config_store_dir="+store, "pnpm_config_offline=true")
	if h.runTask(ctx, tree, install, hookTaskTimeout, "tools-pnpm") != 0 {
		h.say.Text("pre-push: refused: the offline install of " + short + "'s explorer and tools/.node packages failed in " + tree + ": run  task tools  in this clone, so its store holds every locked package\n")
		return exitFailed
	}
	if code := h.runTask(ctx, tree, env, hookTaskTimeout, "pre-push", "SKIP_UNCHANGED="+skip); code != 0 {
		h.say.Text("pre-push: task pre-push failed on " + short + "\n")
		return code
	}
	return exitDone
}

// remove takes the throwaway worktree away: the link to .tools first, by
// itself, so nothing that removes the tree walks into the clone's tools;
// then the worktree, by git; then its folder, by removeDir, FreshFolder's
// removal of dir.
func (h *hook) remove(ctx context.Context, dir, tree string, removeDir func() error) error {
	var errs []error
	if err := os.Remove(filepath.Join(tree, ".tools")); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("the link to .tools in %s is left: %w", tree, err))
	} else if _, err := os.Stat(tree); err == nil {
		if _, err := h.git(ctx, "worktree", "remove", "--force", tree); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		if err := removeDir(); err != nil {
			errs = append(errs, fmt.Errorf("the folder %s is left: %w", dir, err))
		}
	}
	return errors.Join(errs...)
}

// linkTools links the clone's .tools at the tree's, by go-files'
// confine.LinkFolder: a symbolic link, or on Windows a junction. A .tools
// that is itself a link is linked at the folder it leads to, as go-files'
// confine.Resolve follows it: one hop at a time, a junction among them on
// Windows, which filepath.EvalSymlinks has not followed since Go 1.23
// (gragas#1589). Resolve answers a missing name as written, so the folder's
// absence is the os.Stat after it.
func linkTools(root, tree string) (err error) {
	tools, err := filepath.Abs(filepath.Join(root, ".tools"))
	if err != nil {
		return err
	}
	if tools, err = confine.Resolve(tools); err != nil {
		return fmt.Errorf("the clone's .tools: %w", err)
	}
	if _, err := os.Stat(tools); err != nil {
		return fmt.Errorf("the clone's .tools: %w", err)
	}
	r, err := os.OpenRoot(tree)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	if err := confine.LinkFolder(r, tools, ".tools"); err != nil {
		return fmt.Errorf("the pinned tools %s are not linked into %s: %w", tools, tree, err)
	}
	return nil
}

// pnpmStore is the store pnpm's install recorded in the clone's
// .modules.yaml, which pnpm writes as JSON: its storeDir, when that is text
// and not empty.
func pnpmStore(modules string) (string, bool) {
	b, err := os.ReadFile(modules)
	if err != nil {
		return "", false
	}
	return parseStore(b)
}

// parseStore is the storeDir of a .modules.yaml's bytes, when that is text
// and not empty.
func parseStore(b []byte) (string, bool) {
	var m struct {
		StoreDir *string `json:"storeDir"`
	}
	if json.Unmarshal(b, &m) != nil || m.StoreDir == nil || *m.StoreDir == "" {
		return "", false
	}
	return *m.StoreDir, true
}

// pathFirst is env with dir first on its PATH, joined to the list the environment already had with the
// system's own list separator: the one place the dev entry reads that separator (hook's task and pnpm
// both start a program from the pinned folder). The PATH is found whatever the case of its name, as a
// system that folds environment names reads it, and set once.
func pathFirst(env []string, dir string) []string {
	list := []string{dir}
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); strings.EqualFold(k, "PATH") && v != "" {
			list = append(list, v)
		}
	}
	return append(withoutVars(env, []string{"PATH"}), "PATH="+strings.Join(list, string(os.PathListSeparator)))
}

// hookTaskTimeout ends a task a hook runs (pre-commit, pre-push, tools-pnpm)
// that hangs (gragas#2263). It is a backstop and no budget: the tier's budget
// is hatch's check after the run (gate/budgets.yaml), and a push slowed by the
// machine's load is reported over it, never killed by this.
const hookTaskTimeout = 30 * time.Minute

// runOutput is the most of each stream of a program the dev entry runs that is
// kept (gragas#2263): a program that writes more is ended, and says so.
const runOutput = 64 << 20

// runTask runs the clone's task with args in dir, with env, its output on the
// hook's own streams, and the clone's .tools/bin first on the PATH, through
// proc.Run: its tree is killed whole when it ends, at timeout, or when ctx
// ends, as Ctrl+C ends it (term.Main). The task reads no stdin: a terminal
// would be refused, as proc gives the task none. It answers taskEnd's code,
// and says why when the task gave no exit code of its own.
func (h *hook) runTask(ctx context.Context, dir string, env []string, timeout time.Duration, args ...string) int {
	res, err := proc.Run(ctx, proc.Cmd{
		Path:      h.task,
		Args:      args,
		Dir:       dir,
		Env:       append(pathFirst(env, filepath.Join(h.root, ".tools", "bin")), "GIT_OPTIONAL_LOCKS=0"),
		Stdout:    h.stdout,
		Stderr:    h.stderr,
		Timeout:   timeout,
		MaxOutput: runOutput,
		Drain:     proc.DefaultDrain,
	})
	code, why := taskEnd(res.Code, err)
	if why != "" {
		h.say.Text("task " + args[0] + " " + why + "\n")
	}
	return code
}

// taskEnd is the exit code a hook answers for a task's run, and what it says
// of it: the task's own code above 0, exitDone at 0; and exitFailed, with why,
// for a run that did not reach its end (refused, its timeout, its output cap,
// ctx) or a task that gave no code of its own, as one ended by a signal does,
// which said nothing before (gragas#2408).
func taskEnd(code int, err error) (int, string) {
	switch {
	case err != nil:
		return exitFailed, "did not run to its end: " + err.Error()
	case code > 0:
		return code, ""
	case code == 0:
		return exitDone, ""
	}
	return exitFailed, "was ended by a signal, and gave no exit code of its own"
}

// git is hookGit in the clone.
func (h *hook) git(ctx context.Context, args ...string) (string, error) {
	return hookGit(ctx, h.root, h.env, args...)
}

// hookGit is the output of git with args in dir, run as the hook's git:
// with env, the environment git gave the hook cut to the allowlist, and
// gitenv.ReadArgs's arguments, so it takes no optional lock and runs no hook
// and no file system monitor the clone's own configuration names. Its -C dir
// names the folder, so the command sets no Dir: a relative dir would be read
// twice.
func hookGit(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	out, err := runRead(ctx, "git", "", env, gitenv.ReadArgs(dir, args...))
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// readTimeout ends a short read of git or go the dev entry makes that hangs
// (gragas#2263): none takes a second on a healthy clone.
const readTimeout = 2 * time.Minute

// progRun is one run of a program through proc.Run (gragas#2263): name, a
// bare name found on this process's PATH as exec.Command found it, or a path;
// its args; the folder it runs in; env, its whole environment, never nil,
// which proc reads as a bare one; the timeout at which its tree is killed
// whole, as it is when ctx ends; and the writers, when not nil, that are given
// its streams as it writes them. Each stream is kept up to runOutput.
type progRun struct {
	name, dir      string
	args, env      []string
	timeout        time.Duration
	stdout, stderr io.Writer
}

// do runs r. Its error is a program not found or a run that did not reach its
// end; the program's own exit is Result.Code, -1 when a signal ended it.
func (r progRun) do(ctx context.Context) (proc.Result, error) {
	path, err := exec.LookPath(r.name)
	if err != nil {
		return proc.Result{Code: -1}, err
	}
	return proc.Run(ctx, proc.Cmd{
		Path: path, Args: r.args, Dir: r.dir, Env: r.env,
		Stdout: r.stdout, Stderr: r.stderr,
		Timeout: r.timeout, MaxOutput: runOutput, Drain: proc.DefaultDrain,
	})
}

// exitWords is how a run that reached its end with code failed: its exit
// status, or the signal that ended it, which gives none.
func exitWords(code int) string {
	if code < 0 {
		return "ended by a signal"
	}
	return fmt.Sprintf("exit status %d", code)
}

// runRead runs the program name with args in dir, env its whole environment,
// and ends it at readTimeout. It answers the program's stdout when it exits 0;
// else an error holding its stderr, how it ended when it printed none, or why
// the run did not reach its end.
func runRead(ctx context.Context, name, dir string, env, args []string) ([]byte, error) {
	res, err := progRun{name: name, dir: dir, args: args, env: env, timeout: readTimeout}.do(ctx)
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		if msg := strings.TrimSpace(string(res.Stderr)); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, errors.New(exitWords(res.Code))
	}
	return res.Stdout, nil
}

// switchPrefixes begin the names of the variables a tool of the tiers reads
// as a switch, a patch or a program to start, any one of which a caller could
// set for a push to green a red member (gragas#2161):
//   - TASK_: go-task's own flags. TASK_DRY prints the tier and runs none of
//     it, and exits 0 (go-task v3.54.0 internal/flags/flags.go:150).
//   - GOLANGCI_: golangci-lint's diff patch, which keeps no finding outside
//     its hunks (pkg/result/processors/diff.go:43), and its cache program
//     (internal/go/cache/default.go:55).
//   - PNPM_CONFIG_ and NPM_CONFIG_: pnpm's settings, script-shell among them,
//     which picks the program a script runs under.
//   - NODE_OPTIONS: node's, which preloads a file.
//   - GOCACHEPROG: go's, which answers for the build and test cache.
//   - GOENV: the file go reads every variable from that the environment does
//     not set, GOCACHEPROG among them (go1.27 cmd/go/internal/cfg/cfg.go:437).
//
// The hook and landing run hand on none of them, as allowed names none; landing
// begin, check and merge refuse a lander's environment that holds one, as
// they judge that environment and build none.
func switchPrefixes() []string {
	return []string{"TASK_", "GOLANGCI_", "PNPM_CONFIG_", "NPM_CONFIG_", "NODE_OPTIONS", "GOCACHEPROG", "GOENV"}
}

// isSwitch is whether the name of kv begins with one of switchPrefixes,
// whatever its case, except exactly GOENV=off: that is the one value that makes go read no file of
// variables, which allowed adds to the environment of every member (gragas#2181), so the landing's
// own environment holds it and is not refused for it (gragas#2389). Any other GOENV, a path or an
// empty value, is still one.
func isSwitch(kv string) bool {
	if kv == "GOENV=off" {
		return false
	}
	k, _, _ := strings.Cut(kv, "=")
	return slices.ContainsFunc(switchPrefixes(), func(p string) bool {
		return len(k) >= len(p) && strings.EqualFold(k[:len(p)], p)
	})
}

// kept is a variable of the caller's that the hook hands on, and why a git or a
// member of the tiers needs it.
type kept struct{ name, why string }

// hookKept is the gate's own part of the allowlist (gragas#2181): the names the
// hook and landing run hand on of the environment they are given besides the
// system's, which go-proc names (allowedNames). Every other name is dropped, in
// any case, so a route nobody listed is closed, not open: git's GIT_CONFIG_COUNT
// and GIT_CONFIG_PARAMETERS, go's GOMODCACHE, GOCACHE, GOEXPERIMENT, GODEBUG and
// CGO_ENABLED, and any later tool's variable. A name a member needs that this
// table leaves out turns its tier red, never green. Not here, as they are set,
// not kept: GOFLAGS, GOTOOLCHAIN, GOWORK and GOPROXY, which the Taskfile sets for
// every member; GIT_OPTIONAL_LOCKS and GOENV, which the hook sets; the pnpm store
// pair, which away sets. No member of these tiers runs gh: names and
// self-check-issues are release members. Nor is GIT_INDEX_FILE, which git names
// to pre-commit alone and preCommit alone hands on (indexOf).
func hookKept() []kept {
	const fetch = "a fetch by go, git or govulncheck: "
	return []kept{
		{"GOPRIVATE", "the modules go fetches from their own repositories and asks no proxy or sum database of, so a private module's path never reaches the public ones; read from the person's go env file too (goPrivate)"},
		{"GONOPROXY", "as GOPRIVATE, for the proxy alone"},
		{"GONOSUMDB", "as GOPRIVATE, for the sum database alone; go.sum and -mod=readonly still verify every module"},
		{"SSH_AUTH_SOCK", fetch + "the agent git signs in to a repository with over ssh, a private module's among them"},
		{"SSL_CERT_FILE", fetch + "the certificates it trusts, where the system's are not the ones"},
		{"SSL_CERT_DIR", fetch + "as SSL_CERT_FILE, a folder of them"},
		{"HTTPS_PROXY", fetch + "the proxy it reaches the network by"},
		{"HTTP_PROXY", fetch + "as HTTPS_PROXY, for plain http"},
		{"NO_PROXY", fetch + "the hosts it reaches without the proxy"},
		{"TERM", "the terminal, which says whether a tool's output may carry colour"},
		{"NO_COLOR", "the person's word that no tool's output carries colour"},
		{"GATE_WEIGHT", "the concurrency the Taskfile's explorer-lint gives eslint, auto when it is unset"},
	}
}

// goPrivateNames are the variables goPrivate reads.
func goPrivateNames() []string { return []string{"GOPRIVATE", "GONOPROXY", "GONOSUMDB"} }

// allowedNames is the allowlist: the variables a child needs to run on this
// system (proc.Needed: PATH, LANG and the home names), every variable a program
// here reads for the person's home, configuration, cache, state or temp folder
// (proc.HomeEnv's names), which hold go's caches, gragas-tmp's stable folder and
// each tool's own settings where they are, so a landing finds its caches warm,
// and hookKept's. go-proc names the system's variables, so no file of this tree
// does (os-code kind E).
func allowedNames() []string {
	names := proc.Needed()
	for _, kv := range proc.HomeEnv(proc.NeutralDir()) {
		k, _, _ := strings.Cut(kv, "=")
		names = append(names, k)
	}
	for _, k := range hookKept() {
		names = append(names, k.name)
	}
	return names
}

// allowed is env cut to allowedNames, matched in any case, with goPrivate's
// values in place of the caller's and GOENV off: go reads no file of variables
// for a member, the person's or one a caller names (gragas#2181).
func allowed(ctx context.Context, env []string) ([]string, error) {
	names := allowedNames()
	env = slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		return !slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, k) })
	})
	private, err := goPrivate(ctx, env)
	if err != nil {
		return nil, err
	}
	return append(withoutVars(env, goPrivateNames()), append(private, "GOENV=off")...), nil
}

// goPrivate is goPrivateNames as go reads them under env, the allowlist, which
// holds no GOENV: the caller's own exported values, else the person's go env -w
// file at its default place under the home, never a file a caller names. Each
// that is set is NAME=value. go runs in proc.NeutralDir with GOTOOLCHAIN=local,
// so no go.mod there and no line of that file starts another toolchain.
func goPrivate(ctx context.Context, env []string) ([]string, error) {
	out, err := runRead(ctx, "go", proc.NeutralDir(), append(slices.Clone(env), "GOTOOLCHAIN=local"), append([]string{"env", "-json"}, goPrivateNames()...))
	if err != nil {
		return nil, fmt.Errorf("go env %s: %w", strings.Join(goPrivateNames(), " "), err)
	}
	return parseGoPrivate(out)
}

// parseGoPrivate is go env -json's answer for goPrivateNames: each name that is
// set, as NAME=value, in goPrivateNames' order. Anything but a JSON object of
// strings is an error.
func parseGoPrivate(out []byte) ([]string, error) {
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil {
		return nil, fmt.Errorf("go env %s: %w", strings.Join(goPrivateNames(), " "), err)
	}
	var set []string
	for _, k := range goPrivateNames() {
		if v := got[k]; v != "" {
			set = append(set, k+"="+v)
		}
	}
	return set, nil
}

// withoutVars is env less each of names, whatever its case.
func withoutVars(env, names []string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		return slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, k) })
	})
}
