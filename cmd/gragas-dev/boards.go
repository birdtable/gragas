package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/birdtable/gragas/internal/dev/say"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const boardsUsage = `usage: gragas-dev boards check --node path [-C root]
`

// runBoards is the boards command. check rebuilds canvas/ and the parts seed from gen/ in a
// copy of the tree, with the node named, and fails when the copy's files differ from the
// tree's in either direction (a file missing or extra on either side is red). It replaces the
// Taskfile's shell of mktemp, mkdir, cp and diff, none of which is the same program on every
// system (gragas#1228).
func runBoards(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	if len(args) == 0 || args[0] != "check" {
		talk.Text(boardsUsage)
		return exitUsage
	}
	fl := flag.NewFlagSet("boards check", flag.ContinueOnError)
	fl.SetOutput(stderr)
	node := fl.String("node", "", "the node that runs the scripts")
	root := fl.String("C", ".", "the root holding docs/explorer-boards and explorer/core")
	if err := fl.Parse(args[1:]); err != nil || fl.NArg() != 0 || *node == "" {
		talk.Text(boardsUsage)
		return exitUsage
	}
	problems, err := boardsCheck(ctx, *root, *node)
	if err != nil {
		talk.Text("gragas-dev boards: " + err.Error() + "\n")
		return exitFailed
	}
	if len(problems) > 0 {
		out := &say.Speaker{W: stdout}
		for _, p := range problems {
			out.Text(p + "\n")
		}
		return exitFailed
	}
	return exitDone
}

// boardsCheck returns what differs between the tree and a rebuild of it, or an error when the
// rebuild could not be made.
func boardsCheck(ctx context.Context, root, node string) (problems []string, err error) {
	nodePath, err := exec.LookPath(node)
	if err != nil {
		return nil, err
	}
	nodePath, err = filepath.Abs(nodePath)
	if err != nil {
		return nil, err
	}
	b := filepath.Join(root, "docs", "explorer-boards")
	// The rebuild's folder is host/trust's FreshFolder (gragas#2264): fresh at
	// 0700 in the account's own temp folder, kept apart from the tree.
	tmp, remove, err := hosttrust.FreshFolder([]string{root}, "gragas-boards-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if rerr := remove(); rerr != nil && err == nil {
			err = rerr
		}
	}()
	c := filepath.Join(tmp, "docs", "explorer-boards")
	if err := copyBoardInputs(root, tmp); err != nil {
		return nil, err
	}
	for _, script := range []string{"build.mjs", "seed.mjs"} {
		cmd := exec.CommandContext(ctx, "node")
		cmd.Args = append(cmd.Args, script)
		cmd.Path, cmd.Err = nodePath, nil // the node named, a variable the lint cannot see through
		cmd.Dir = filepath.Join(c, "gen")
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("%s: %w: %s", script, err, strings.TrimSpace(string(out)))
		}
	}
	diffs, err := diffTrees(filepath.Join(b, "canvas"), filepath.Join(c, "canvas"))
	if err != nil {
		return nil, err
	}
	problems = append(problems, diffs...)
	if len(problems) > 0 {
		problems = append(problems, "boards: canvas/ differs from a rebuild of gen/")
	}
	seed := filepath.Join("explorer", "core", "parts.seed.yaml")
	have, err1 := os.ReadFile(filepath.Join(root, seed))
	want, err2 := os.ReadFile(filepath.Join(tmp, seed))
	if err1 != nil || err2 != nil || !bytes.Equal(have, want) {
		problems = append(problems, "boards: the parts seed differs from a rebuild of gen/")
	}
	return problems, nil
}

// copyBoardInputs copies what the rebuild reads, from the tree at root into the folder tmp: gen/, the
// Relations pages and canvas.json of canvas/, and explorer/core. No Relations page is an error.
func copyBoardInputs(root, tmp string) error {
	b := filepath.Join(root, "docs", "explorer-boards")
	c := filepath.Join(tmp, "docs", "explorer-boards")
	if err := os.CopyFS(filepath.Join(c, "gen"), os.DirFS(filepath.Join(b, "gen"))); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(c, "canvas"), 0o700); err != nil {
		return err
	}
	pages, err := filepath.Glob(filepath.Join(b, "canvas", "Relations*.dc.html"))
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return errors.New("canvas/ holds no Relations*.dc.html page to rebuild from: an empty tree is red")
	}
	for _, f := range append(pages, filepath.Join(b, "canvas", "canvas.json")) {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(c, "canvas", filepath.Base(f)), data, 0o600); err != nil {
			return err
		}
	}
	return os.CopyFS(filepath.Join(tmp, "explorer", "core"), os.DirFS(filepath.Join(root, "explorer", "core")))
}

// readTree reads every file below dir, by its slash path; a folder that is not there is empty.
func readTree(dir string) (map[string][]byte, error) {
	m := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		m[filepath.ToSlash(rel)] = data
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	return m, err
}

// diffTrees lists, as diff -r does, what is in one folder and not the other and what differs.
func diffTrees(have, want string) ([]string, error) {
	a, err := readTree(have)
	if err != nil {
		return nil, err
	}
	b, err := readTree(want)
	if err != nil {
		return nil, err
	}
	var out []string
	names := map[string]bool{}
	for n := range a {
		names[n] = true
	}
	for n := range b {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	slices.Sort(sorted)
	for _, n := range sorted {
		x, inA := a[n]
		y, inB := b[n]
		switch {
		case inA && !inB:
			out = append(out, "Only in "+have+": "+n)
		case !inA && inB:
			out = append(out, "Only in "+want+": "+n)
		case !bytes.Equal(x, y):
			out = append(out, "Files "+filepath.Join(have, n)+" and "+filepath.Join(want, n)+" differ")
		}
	}
	return out, nil
}
