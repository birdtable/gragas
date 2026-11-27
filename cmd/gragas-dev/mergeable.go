package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/birdtable/go-proc/gitenv"
)

// mainRef is the ref a pushed head is merged against, unless it is a release's: the clone's copy of main.
const mainRef = "refs/remotes/origin/main"

// releaseRef is the clone's copy of the release branch: a head that grew from it merges into it.
const releaseRef = "refs/remotes/origin/release-1"

// mergeBase is the ref a pushed head must merge into: origin/release-1 when the head descends from a
// commit of origin/main..origin/release-1 (its merge base with the release is not already in main),
// else origin/main. A clone with no origin/release-1, or a head with no base in common with it, is
// checked against main.
func (h *hook) mergeBase(ctx context.Context, sha string) string {
	if _, err := h.git(ctx, "rev-parse", "--verify", "--quiet", releaseRef+"^{commit}"); err != nil {
		return mainRef
	}
	base, err := h.git(ctx, "merge-base", sha, releaseRef)
	if err != nil || strings.TrimSpace(base) == "" {
		return mainRef
	}
	cmd := withArgs(exec.CommandContext(ctx, "git"), gitenv.ReadArgs(h.root, "merge-base", "--is-ancestor", strings.TrimSpace(base), mainRef))
	cmd.Env = h.env
	if exitErr, ok := errors.AsType[*exec.ExitError](cmd.Run()); ok && exitErr.ExitCode() == 1 {
		return releaseRef
	}
	return mainRef
}

// mergeable refuses a pushed head that does not merge into its base (mergeBase;
// gragas#1733, class 3): it names the files that conflict. It reads git's
// own answer, merge-tree --write-tree, which writes tree and blob objects into
// the clone and changes no ref and no working file, started as every git of
// the hook is. A clone with no origin/main, or a git that answers nothing, is
// a refusal too, never a pass.
func (h *hook) mergeable(ctx context.Context, sha string) error {
	if _, err := h.git(ctx, "rev-parse", "--verify", "--quiet", mainRef+"^{commit}"); err != nil {
		return fmt.Errorf("origin/main is not fetched here, so %s cannot be checked to merge into it: run git fetch origin main and push again", sha)
	}
	ref := h.mergeBase(ctx, sha)
	into := strings.TrimPrefix(ref, "refs/remotes/")
	cmd := withArgs(exec.CommandContext(ctx, "git"), gitenv.ReadArgs(h.root, "merge-tree", "--write-tree", "--name-only", "--no-messages", ref, sha))
	cmd.Env = h.env
	out, err := cmd.Output()
	if err == nil {
		return nil
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 {
		files := conflictFiles(string(out))
		if len(files) == 0 {
			return fmt.Errorf("%s does not merge into %s, and git named no file: rebase onto %s and push again", sha, into, into)
		}
		return fmt.Errorf("%s does not merge into %s; these files conflict: %s: rebase onto %s, resolve them and push again", sha, into, strings.Join(files, ", "), into)
	}
	msg := err.Error()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && len(exitErr.Stderr) > 0 {
		msg = strings.TrimSpace(string(exitErr.Stderr))
	}
	return fmt.Errorf("git could not check that %s merges into %s: %s", sha, into, msg)
}

// conflictFiles are the paths git merge-tree --name-only prints after the
// tree it wrote on its first line, up to the blank line that ends them,
// each once.
func conflictFiles(out string) []string {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	var files []string
	for _, l := range lines[min(1, len(lines)):] {
		if l == "" {
			break
		}
		if len(files) == 0 || files[len(files)-1] != l {
			files = append(files, l)
		}
	}
	return files
}
