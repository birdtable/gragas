// Package commondir answers where a git checkout keeps what its worktrees
// share: the folder that holds config, hooks and the object store.
//
// It is generic code a platform library lacks, kept here until the library has
// it (go-proc#416, a gitenv helper): the swap is one import. Nothing in it
// names a project.
package commondir

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/birdtable/go-proc/gitenv"
	"github.com/birdtable/go-proc/proc"
)

const (
	deadline = 10 * time.Second
	// drain is how long a run reads a pipe a process that left git's tree still
	// holds once the tree is killed (go-proc's Cmd.Drain).
	drain = 10 * time.Second
)

// CommonDir is the git dir shared by the checkout at dir and its worktrees, as
// git itself names it (git rev-parse --git-common-dir), made absolute. In a
// clone it is dir's own .git; in a linked worktree, where .git is a file, it is
// the main checkout's. A dir that is no checkout, or a git that cannot answer,
// is an error.
func CommonDir(ctx context.Context, dir string) (string, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("find the common git dir of %s: %w", dir, err)
	}
	res, err := proc.Run(ctx, proc.Cmd{
		Path:    git,
		Args:    []string{"rev-parse", "--git-common-dir"},
		Dir:     dir,
		Env:     gitenv.Env(proc.SystemEnv(), gitenv.NoGlobalConfig()...),
		Timeout: deadline,
		// One path is the whole answer, and a path is under the system's own limit
		// (1 KiB to 4 KiB), so 4 KiB is never reached: a bound go-proc needs, no
		// figure a test can hold at its edge, so it is not a named cap.
		MaxOutput: 4 << 10,
		Drain:     drain,
	})
	if err != nil {
		return "", fmt.Errorf("find the common git dir of %s: %w", dir, err)
	}
	if res.Code != 0 {
		return "", fmt.Errorf("find the common git dir of %s: git exited %d: %s", dir, res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	p := filepath.FromSlash(strings.TrimSpace(string(res.Stdout)))
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return p, nil
}
