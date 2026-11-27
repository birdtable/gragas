// Command gragas-docs writes the documents made from the command registry:
// docs/commands.md, docs/command-tree.json and the command table in the
// README. With --check it writes nothing and fails when one differs. It is
// built and run from this repository's Taskfile and is not shipped. It is a
// command of its own, and not part of gragas-dev, because it links the
// command tree, which the landing check's closure must not.
//
// main is the only place that exits (go.errors.4).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/birdtable/gragas/internal/cli/root"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/host/term"
	"github.com/birdtable/gragas/internal/rendered"
)

func main() {
	// Ctrl+C and SIGTERM are the kit's: its Main cancels the context the run is
	// given, on every system.
	os.Exit(term.Main(context.Background(), "gragas-docs", os.Stderr, func(ctx context.Context, args []string) int {
		return run(ctx, args, os.Stdout, os.Stderr)
	}))
}

// run is the program with its streams, so a test drives it without a process.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("gragas-docs", flag.ContinueOnError)
	set.SetOutput(stderr)
	check := set.Bool("check", false, "write nothing and fail when a document differs")
	dir := set.String("C", ".", "the repository")
	if err := set.Parse(args); err != nil || set.NArg() > 0 {
		return exit.Usage
	}
	out, diag := &say.Speaker{W: stdout}, &say.Speaker{W: stderr}
	stale, compared, err := generate(ctx, *dir, *check)
	if err != nil {
		diag.Text("gragas-docs: " + err.Error() + "\n")
		return exit.Failed
	}
	for _, p := range stale {
		if *check {
			diag.Text("gragas-docs: " + p + " differs from the command registry: run task gen\n")
		} else {
			out.Text("wrote " + p + "\n")
		}
	}
	switch {
	case out.Err != nil || diag.Err != nil:
		return exit.Failed
	case *check && len(stale) > 0:
		return exit.Failed
	}
	if *check {
		// The count line: the documents it compared, which the
		// gate runner holds to the generated-files member's floor.
		out.Text(fmt.Sprintf("count: %d files\n", compared))
		if out.Err != nil {
			return exit.Failed
		}
	}
	return exit.Done
}

// generate makes the documents of the repository in dir. It answers the paths
// that differ and how many it compared, and writes them unless check is set.
// It reads and writes through dir opened as a root, so no link in the tree
// takes a read or a write outside it (supply.scans.3, gosec G304).
func generate(ctx context.Context, dir string, check bool) (stale []string, compared int, err error) {
	tree, err := os.OpenRoot(dir)
	if err != nil {
		return nil, 0, err
	}
	defer func() { err = errors.Join(err, tree.Close()) }()
	readme, err := tree.ReadFile(root.ReadmeFile)
	if err != nil {
		return nil, 0, err
	}
	docs, err := root.Documents(ctx, readme)
	if err != nil {
		return nil, 0, err
	}
	made, err := plannedDocuments(tree)
	if err != nil {
		return nil, 0, err
	}
	where, err := whereDocuments(tree)
	if err != nil {
		return nil, 0, err
	}
	maps.Copy(made, where)
	type document struct {
		Path string
		Data []byte
	}
	var all []document
	for _, d := range docs {
		all = append(all, document{d.Path, d.Data})
	}
	for _, p := range slices.Sorted(maps.Keys(made)) {
		all = append(all, document{p, made[p]})
	}
	files := make([]rendered.File, 0, len(all))
	for _, d := range all {
		files = append(files, rendered.File{Path: d.Path, Data: d.Data})
	}
	stale, err = rendered.Compare(files, func(p string) ([]byte, error) { return tree.ReadFile(filepath.FromSlash(p)) })
	if err != nil {
		return nil, 0, err
	}
	if check {
		return stale, len(files), nil
	}
	for _, d := range all {
		if !slices.Contains(stale, d.Path) {
			continue
		}
		if err := tree.WriteFile(filepath.FromSlash(d.Path), d.Data, 0o644); err != nil {
			return nil, 0, err
		}
	}
	return stale, len(files), nil
}
