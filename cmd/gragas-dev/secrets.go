package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/birdtable/go-proc/gitenv"
	"github.com/birdtable/gragas/internal/dev/say"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// runSecrets is `secrets --scan staged|tree|<revisions> --gitleaks file --config file...`: one pass of the
// secret scan, the Taskfile's secrets-scan, in one process (gragas#1655, X171). It was a sh script of
// mktemp, awk, grep and cat, which Windows ran through go-task's own copies of them; this is the same
// judgement in Go, and it reads git's log as a stream, so no temp file is made.
//
// It reads only what git tracks. gitleaks git reads git log -p -U0 with the --log-opts given (gitleaks
// 8.30.1, sources/git.go:75): commits, never the folder. staged and tree are first made a commit that no
// ref names: git write-tree is the stage, git stash create the tracked files as they are, and git
// commit-tree makes the commit, on HEAD for staged, as a root for tree. A file git does not track is never
// read, so build output needs no allow-list. Every pass names its revisions: with no --log-opts gitleaks
// reads git log --all (sources/git.go:94), every ref of the clone.
//
// The options: --text and --no-textconv, so a NUL byte, a binary or -diff attribute or a textconv driver
// cannot hide a file's bytes; --root; --diff-merges=first-parent, so a merge's own lines are read;
// --no-color and --no-ext-diff, which gitleaks does not pass itself (sources/git.go:76), so the
// repository's own config cannot colour or replace the patch; --ignore-gitleaks-allow, so an allow comment
// hides nothing. A .gitleaksignore at the root, which gitleaks always reads (cmd/root.go:317), is refused,
// and so is any path the revisions add or change whose name holds gitleaks.toml, but the root .gitleaks.toml
// (skippedPath, gragas#2334), and so are a shallow clone and a grafts file, whose history is not the one a push sends. git runs with
// no system or global config and no replace objects (gitScanEnv). gitleaks logs each line its git writes
// to stderr at ERR and then stops judging, still exiting 0 (sources/git.go:262), so such a line fails the
// pass. A finding names its rule, file and line; --redact keeps the value out of the report.
//
// The count (main's internal/dev/scan, ErrUnscanned): gitleaks must say it scanned as many commits as
// git's own log of the same range holds with a hunk in a file the commit does not delete, the commits
// gitleaks counts. The count runs git log with its own --format, so a repository's config that changes
// the log gitleaks reads, as format.pretty does (gitleaks then reads no commit, says nothing and exits 0),
// fails the pass.
func runSecrets(ctx context.Context, environ, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("secrets", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "the repository the pass reads")
	scanFlag := fl.String("scan", "", "staged, tree, or a revision range of git log")
	gitleaks := fl.String("gitleaks", "", "the pinned gitleaks")
	var configs []string
	fl.Func("config", "a gitleaks config, one pass each, named by absolute path (repeatable)", func(s string) error {
		configs = append(configs, s)
		return nil
	})
	words, err := parseInterspersed(fl, args)
	if err != nil || len(words) != 0 || strings.TrimSpace(*scanFlag) == "" || *gitleaks == "" || len(configs) == 0 {
		talk.Text(usage)
		return exitUsage
	}
	j := secretsJob{dir: *dir, scan: *scanFlag, gitleaks: *gitleaks, environ: environ, configs: configs, stdout: stdout, stderr: stderr}
	problems := secretsPass(ctx, j)
	for _, p := range problems {
		talk.Text("secrets: " + p + "\n")
	}
	if len(problems) > 0 {
		return exitFailed
	}
	return exitDone
}

// secretsJob is one pass: the repository, the scan, the pinned gitleaks, its configs, and where gitleaks's
// own output goes (its stdout to stdout, its stderr to stderr after it is read).
type secretsJob struct {
	dir, scan, gitleaks string
	environ, configs    []string
	stdout, stderr      io.Writer
}

// gitScanEnv is what every git the pass runs, gitleaks's too, adds to its environment: no system or global
// config (go-proc's gitenv.NoGlobalConfig), no replace objects, and the scan's own name on the commits staged
// and tree make. It is not gitenv.Env: the staged pass reads the index git handed pre-commit, which
// GIT_INDEX_FILE names, and Env drops every GIT_ name (gragas#2262).
func gitScanEnv() []string {
	return append(gitenv.NoGlobalConfig(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_AUTHOR_NAME=secret scan",
		"GIT_AUTHOR_EMAIL=scan@localhost",
		"GIT_COMMITTER_NAME=secret scan",
		"GIT_COMMITTER_EMAIL=scan@localhost",
	)
}

// secretsGit runs git with the words in the job's repository under environ with gitScanEnv, and returns its
// stdout trimmed.
func secretsGit(ctx context.Context, environ []string, dir string, words ...string) (string, error) {
	out, err := output(ctx, environ, dir, gitScanEnv(), "git", words...)
	return strings.TrimSpace(string(out)), err
}

// secretsPass is one pass of the scan; it returns what is wrong, each without the "secrets: " in front.
func secretsPass(ctx context.Context, j secretsJob) []string {
	if _, err := os.Lstat(filepath.Join(j.dir, ".gitleaksignore")); err == nil {
		return []string{".gitleaksignore is an allow-list, and the scan takes none: remove it"}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return []string{err.Error()}
	}
	if p := historyProblem(ctx, j.environ, j.dir); p != "" {
		return []string{p}
	}
	revs, err := scanRevisions(ctx, j.environ, j.dir, j.scan)
	if err != nil {
		return []string{err.Error()}
	}
	if p, err := skippedPath(ctx, j.environ, j.dir, revs); err != nil {
		return []string{"git cannot list the paths of the pass, so the pass cannot be trusted: " + err.Error()}
	} else if p != "" {
		return []string{p + " is a path the scan would skip (any path holding gitleaks.toml is on the default config's global allow-list, and the private-key pass reads one rule only): rename it"}
	}
	opts := append([]string{"--text", "--no-textconv", "--no-color", "--no-ext-diff", "--root", "--diff-merges=first-parent"}, revs...)
	want, err := commitsWithAHunk(ctx, j.dir, opts)
	if err != nil {
		// A range git cannot read, which gitleaks would log and then exit 0 on.
		return []string{"git cannot read the log of the pass, so the pass cannot be trusted: " + err.Error()}
	}
	var problems []string
	for _, cfg := range j.configs {
		problems = append(problems, gitleaksPass(ctx, j, cfg, opts, want)...)
	}
	return problems
}

// historyProblem is why the history git shows here is not the one a push sends, or "": a shallow clone, or
// a grafts file.
func historyProblem(ctx context.Context, environ []string, dir string) string {
	const msg = "a shallow clone or a grafts file: the history git shows is not the one a push sends"
	shallow, err := secretsGit(ctx, environ, dir, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return err.Error()
	}
	grafts, err := secretsGit(ctx, environ, dir, "rev-parse", "--git-path", "info/grafts")
	if err != nil {
		return err.Error()
	}
	if !filepath.IsAbs(grafts) {
		grafts = filepath.Join(dir, grafts)
	}
	_, err = os.Lstat(grafts)
	if shallow != "false" || err == nil {
		return msg
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err.Error()
	}
	return ""
}

// scanRevisions is the git log revisions of the scan: for staged and tree, the one commit made of the stage
// or of the tracked files as they are; anything else is a revision range, split at its spaces as the sh
// script split it.
func scanRevisions(ctx context.Context, environ []string, dir, scan string) ([]string, error) {
	var tree string
	parent := []string{}
	switch scan {
	case "staged":
		t, err := secretsGit(ctx, environ, dir, "write-tree")
		if err != nil {
			return nil, err
		}
		tree = t
		// A repository with no commit yet stages a root.
		if head, err := secretsGit(ctx, environ, dir, "rev-parse", "-q", "--verify", "HEAD^{commit}"); err == nil && head != "" {
			parent = []string{"-p", head}
		}
	case "tree":
		stash, err := secretsGit(ctx, environ, dir, "stash", "create")
		if err != nil {
			return nil, err
		}
		if stash == "" {
			stash = "HEAD"
		}
		if tree, err = secretsGit(ctx, environ, dir, "rev-parse", "--verify", stash+"^{tree}"); err != nil {
			return nil, err
		}
	default:
		return strings.Fields(scan), nil
	}
	words := append(append([]string{"commit-tree", "--no-gpg-sign"}, parent...), "-m", scan, tree)
	c, err := secretsGit(ctx, environ, dir, words...)
	if err != nil {
		return nil, err
	}
	return []string{"-1", c}, nil
}

// skippedPath is the first path the revisions add or change whose name holds gitleaks.toml in any case, or "".
// The default config's global allow-list skips such a path unanchored (gitleaks 8.30.1, config/config.go:487),
// so the first pass never reads a key put there, and the second pass reads the private-key rule alone: an
// access token in docs/gitleaks.toml was read by neither. The repository's own root .gitleaks.toml is the one
// path let through: it is the scan's config, the gate's own file, and the second pass reads a private key in
// it. A path the revisions only delete is not listed (--diff-filter=d).
func skippedPath(ctx context.Context, environ []string, dir string, revs []string) (string, error) {
	words := append([]string{"-c", "core.quotePath=false", "log", "--format=", "--name-only", "--no-renames", "--root", "--diff-merges=first-parent", "--diff-filter=d"}, revs...)
	out, err := secretsGit(ctx, environ, dir, words...)
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(out) {
		path := strings.TrimSuffix(line, "\n")
		if path != ".gitleaks.toml" && strings.Contains(strings.ToLower(path), "gitleaks.toml") {
			return path, nil
		}
	}
	return "", nil
}

// commitsWithAHunk is the count of the commits git log -p -U0 shows with the options that hold a hunk in a
// file the commit does not delete. The log is read as it is written, never held whole.
func commitsWithAHunk(ctx context.Context, dir string, opts []string) (int, error) {
	cmd := withArgs(exec.CommandContext(ctx, "git"), append([]string{"log", "-p", "-U0", "--format=%x01%H"}, opts...))
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), gitScanEnv()...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	n, readErr := countHunkCommits(pipe)
	if readErr != nil {
		// Let git finish writing, so Wait does not hang on a full pipe.
		if _, err := io.Copy(io.Discard, pipe); err != nil {
			readErr = errors.Join(readErr, err)
		}
	}
	if err := cmd.Wait(); err != nil {
		return 0, fmt.Errorf("%s: %w: %s", strings.Join(cmd.Args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return n, readErr
}

// countHunkCommits counts as the sh script's awk did: a line that starts with \x01 opens a commit; a diff
// --git line opens a file's header, and a deleted file mode line in it marks the file deleted; the first @@
// line of a header ends it, and counts the commit, once, unless the file is deleted. Only the start of a
// line is read, so a line longer than the buffer is no matter.
func countHunkCommits(r io.Reader) (int, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	n := 0
	counted, header, deleted := false, false, false
	start := true
	for {
		line, err := br.ReadSlice('\n')
		if len(line) > 0 && start {
			switch {
			case line[0] == 1:
				counted, header, deleted = false, false, false
			case bytes.HasPrefix(line, []byte("diff --git ")):
				header, deleted = true, false
			case header && bytes.HasPrefix(line, []byte("deleted file mode ")):
				deleted = true
			case header && bytes.HasPrefix(line, []byte("@@")):
				if !deleted && !counted {
					n++
					counted = true
				}
				header = false
			}
		}
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			start = false
		case err == nil:
			start = true
		case errors.Is(err, io.EOF):
			return n, nil
		default:
			return n, err
		}
	}
}

var (
	gitleaksGitErr = regexp.MustCompile(` ERR (\[git\]|error="stderr is not empty")`)
	gitleaksCount  = regexp.MustCompile(`[0-9]+ commits scanned`)
)

// gitleaksPass runs gitleaks over the options with the config cfg, writes its stderr through, and returns
// what is wrong: a finding (its exit), a line its git wrote to stderr, or a count of commits that is not
// want.
func gitleaksPass(ctx context.Context, j secretsJob, cfg string, opts []string, want int) []string {
	name := filepath.Base(cfg)
	cmd := withArgs(exec.CommandContext(ctx, "gitleaks"), []string{
		"git", "--config", cfg, "--ignore-gitleaks-allow", "--no-banner", "--no-color", "--redact", "--verbose",
		"--exit-code", "1", "--log-opts", strings.Join(opts, " "), ".",
	})
	cmd.Path, cmd.Err = j.gitleaks, nil // the pinned one, set after the call: the program stays a constant
	cmd.Dir = j.dir
	cmd.Env = append(cmd.Environ(), gitScanEnv()...)
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = j.stdout, &log
	runErr := cmd.Run()
	var problems []string
	if _, err := j.stderr.Write(log.Bytes()); err != nil {
		problems = append(problems, "gitleaks's report of the "+filepath.Base(cfg)+" pass could not be written: "+err.Error())
	}
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr):
		problems = append(problems, "gitleaks found a secret in the "+name+" pass, or failed: "+runErr.Error())
	default:
		problems = append(problems, "gitleaks did not run in the "+name+" pass: "+runErr.Error())
	}
	if gitleaksGitErr.Match(log.Bytes()) {
		problems = append(problems, "git wrote to its stderr in the "+name+" pass, and gitleaks stops judging when it does: the pass cannot be trusted")
	}
	got := "no count"
	if m := gitleaksCount.FindAll(log.Bytes(), -1); len(m) == 1 {
		got = strings.Fields(string(m[0]))[0]
	}
	if got != strconv.Itoa(want) {
		problems = append(problems, "gitleaks scanned "+got+" of the "+strconv.Itoa(want)+" commits git's log holds in the "+name+" pass: a setting of the repository's own config may change the log gitleaks reads (format.pretty, for one), so the pass cannot be trusted")
	}
	return problems
}
