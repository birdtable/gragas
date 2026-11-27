package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/birdtable/go-files/confine"
	whole "github.com/birdtable/go-files/write"
	"github.com/birdtable/go-proc/gitenv"

	"github.com/birdtable/gragas/internal/dev/newcommand"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/dev/templates/command"
)

// check holds the golden example to the templates, then lays the repository
// into dst, an empty folder the caller removes, and runs the generator there
// for a name the registry does not declare. The caller builds, vets, lints and
// tests the copy; TestTheGeneratedCopyBuildsVetsLintsAndFailsItsStubs does
// (arch.templates.2, tests.runners.11).
func check(ctx context.Context, dir, dst string, out *say.Speaker) error {
	sample := newcommand.Name{Group: command.SampleGroup, Verb: command.SampleVerb}
	drift, err := newcommand.Drift(command.Files, command.Golden, sample)
	if err != nil {
		return err
	}
	if len(drift) > 0 {
		return fmt.Errorf("the golden example and the templates disagree:\n  %s\nchange them together", strings.Join(drift, "\n  "))
	}
	out.Text("golden example: current\n")
	if dst == "" {
		return nil
	}
	taken, err := declared(os.DirFS(dir))
	if err != nil {
		return err
	}
	n, ok := freeName(taken)
	if !ok {
		return errors.New("no free verb of the base list to try the generator with")
	}
	if err := copyTree(ctx, dir, dst); err != nil {
		return err
	}
	if err := makeCommand(dst, []string{n.Verb}, true, &say.Speaker{W: discard{}}); err != nil {
		return fmt.Errorf("the generator into a copy: %w", err)
	}
	out.Text("generator into a copy: " + n.Verb + "\n")
	return nil
}

// freeName is the first verb of the base list no row declares, a flat command.
func freeName(taken map[string][]string) (newcommand.Name, bool) {
	w := words()
	for _, v := range w.Base {
		if _, ok := taken[v]; !ok && !slices.Contains(w.Groups, v) && !slices.Contains(w.Refused, v) {
			return newcommand.Name{Verb: v}, true
		}
	}
	return newcommand.Name{}, false
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// copyTree copies the repository to dst: the files git tracks in src (its
// index, so a new file is in the copy once it is added), not its history, not
// what a build or a test wrote beside them, and the pinned tools by a link.
// The live tree is read for the list and the bytes at this moment and never
// again, so what another package writes into it afterwards is not in the copy
// (gragas#1324). What leftOut names is not copied.
func copyTree(ctx context.Context, src, dst string) error {
	if err := linkTools(src, dst); err != nil {
		return err
	}
	// go-proc's gitenv: no GIT_ name a hook or a caller set points the list at
	// another repository or index, and no system or global config, optional
	// lock, hook or fsmonitor runs in a read (gragas#2262). Its -C src names
	// the folder, so the command sets no Dir: a relative src would be read twice.
	cmd := exec.CommandContext(ctx, "git")
	cmd.Args = append(cmd.Args, gitenv.ReadArgs(src, "ls-files", "-z", "--cached")...)
	cmd.Env = gitenv.Env(cmd.Environ(), gitenv.NoGlobalConfig()...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git ls-files in %s: %w: %s", src, err, strings.TrimSpace(stderr.String()))
	}
	for rel := range strings.SplitSeq(stdout.String(), "\x00") {
		if rel == "" || leftOut(rel) {
			continue
		}
		from := filepath.Join(src, filepath.FromSlash(rel))
		info, err := os.Lstat(from)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
			continue // gone from disk since it was added, or a link or a folder
		}
		if err != nil {
			return err
		}
		b, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		// the copy is private, its folders and files the owner's alone (go-files'
		// write.File's own mode, an owner-only list on Windows), and scratch the
		// caller removes, so no file of it is flushed
		to := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			return err
		}
		if err := whole.File(to, b, whole.NoSync()); err != nil {
			return err
		}
	}
	return nil
}

// linkTools links the pinned tools of src, its .tools folder, at dst's
// .tools, through go-files' confine.LinkFolder: a symbolic link, or on
// Windows a junction, made in dst opened as a root. A src with no .tools
// has none to link, and the copy gets none: a tool run in it fails as it
// would through a link to nothing. A .tools that is itself a link, as in a
// copy of a copy, is linked at the folder it leads to, as go-files'
// confine.Resolve follows it: one hop at a time, a junction among them on
// Windows, which filepath.EvalSymlinks has not followed since Go 1.23
// (gragas#1589). Resolve answers a missing name as written, so a src with no
// .tools is told by the os.Stat after it.
func linkTools(src, dst string) (err error) {
	tools, err := confine.Resolve(filepath.Join(mustAbs(src), ".tools"))
	if err != nil {
		return err
	}
	if fi, err := os.Stat(tools); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	} else if !fi.IsDir() {
		return fmt.Errorf("%s is not a folder, so the pinned tools are not linked", tools)
	}
	root, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err := confine.LinkFolder(root, tools, ".tools"); err != nil {
		return fmt.Errorf("the pinned tools %s are not linked into the copy: %w", tools, err)
	}
	return nil
}

// leftOut is whether the copy leaves out rel, a path from the repository's
// root as git writes one, / between its names on every system: the history,
// the pinned tools and the times at the top. Untracked folders such as
// node_modules never reach it, for copyTree reads the index.
func leftOut(rel string) bool {
	top, _, _ := strings.Cut(rel, "/")
	return slices.Contains([]string{".git", ".tools", "times"}, top)
}

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}
