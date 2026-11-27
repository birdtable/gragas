// Command gragas-new makes the files of a new command from the golden
// example: gragas-new command [<group>] <verb> shows the files it would
// write, and with --apply writes them; gragas-new check holds the golden
// example to the templates and runs the generator into a copy of the tree.
// It is built from this repository's Taskfile and is not shipped. It is a
// command of its own, and not part of gragas-dev, because it links the command
// tree, which the landing check's closure must not (A60).
//
// main is the only place that exits (go.errors.4).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/cli/planned"
	"github.com/birdtable/gragas/internal/dev/newcommand"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/dev/templates/command"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/host/term"
)

func main() {
	// Ctrl+C and SIGTERM are the kit's: its Main cancels the context the run is
	// given, on every system (gragas#1614).
	os.Exit(term.Main(context.Background(), "gragas-new", os.Stderr, func(ctx context.Context, args []string) int {
		return run(ctx, args, os.Stdout, os.Stderr)
	}))
}

const usage = "usage: gragas-new command [<group>] <verb> [--apply] [-C dir]\n       gragas-new check [-C dir] [--into folder]\n"

// run is the program with its streams, so a test drives it without a process.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	out, diag := &say.Speaker{W: stdout}, &say.Speaker{W: stderr}
	if len(args) == 0 || (args[0] != "command" && args[0] != "check") {
		diag.Text(usage)
		return exit.Usage
	}
	set := flag.NewFlagSet("gragas-new "+args[0], flag.ContinueOnError)
	set.SetOutput(stderr)
	apply := set.Bool("apply", false, "write the files; without it, only list them")
	dir := set.String("C", ".", "the repository")
	into := set.String("into", "", "check: an empty folder to run the generator in, a copy of the tree")
	if err := set.Parse(args[1:]); err != nil {
		return exit.Usage
	}
	var err error
	switch args[0] {
	case "command":
		if set.NArg() < 1 || set.NArg() > 2 {
			diag.Text(usage)
			return exit.Usage
		}
		err = makeCommand(*dir, set.Args(), *apply, out)
	default:
		if set.NArg() != 0 {
			diag.Text(usage)
			return exit.Usage
		}
		err = check(ctx, *dir, *into, out)
	}
	if err != nil {
		diag.Text("gragas-new: " + err.Error() + "\n")
		return exit.Failed
	}
	if out.Err != nil || diag.Err != nil {
		return exit.Failed
	}
	return exit.Done
}

// words are the lists a name is held to: the registry's own.
func words() newcommand.Words {
	return newcommand.Words{Base: kit.BaseVerbs(), Groups: kit.Groups(), Refused: kit.Refused()}
}

var commandRow = regexp.MustCompile(`(?m)^\| ([a-z][a-z-]*)(?: \(([^)]*)\))? \|`)

// declared are the names docs/commands.md lists, with the verbs of each.
func declared(tree fs.FS) (map[string][]string, error) {
	b, err := fs.ReadFile(tree, "docs/commands.md")
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, m := range commandRow.FindAllStringSubmatch(string(b), -1) {
		var verbs []string
		for v := range strings.SplitSeq(m[2], ",") {
			if v = strings.TrimSpace(v); v != "" {
				verbs = append(verbs, v)
			}
		}
		out[m[1]] = verbs
	}
	return out, nil
}

// plannedRows are the commands decided and not built, by name: the files of
// internal/cli/planned in the tree, read into rows.
func plannedRows(tree fs.FS) (map[string]kit.Command, error) {
	sub, err := fs.Sub(tree, "internal/cli/planned")
	if err != nil {
		return nil, err
	}
	rows, err := planned.Load(sub)
	if err != nil {
		return nil, err
	}
	out := map[string]kit.Command{}
	for _, r := range rows {
		out[r.Name] = r
	}
	return out, nil
}

// seedOf is a planned row as the seed of the package that builds it.
func seedOf(r kit.Command) newcommand.Seed {
	s := newcommand.Seed{Order: r.Order, Context: string(r.Context), Ends: string(r.Ends), OptIn: r.OptIn, Summary: r.Summary,
		JSON: r.JSONKind, Writes: r.Writes, Then: r.Then}
	for i, e := range r.Examples {
		s.Examples[i] = [2]string{e.Line, e.Does}
	}
	for _, w := range r.Exits {
		s.Exits = append(s.Exits, string(w))
	}
	return s
}

// takenIn are the names docs/commands.md declares, less the planned ones: a
// planned command is built by this run, which removes its file.
func takenIn(tree fs.FS, rows map[string]kit.Command) (map[string][]string, error) {
	taken, err := declared(tree)
	if err != nil {
		return nil, err
	}
	for name := range rows {
		delete(taken, name)
	}
	return taken, nil
}

func makeCommand(dir string, args []string, apply bool, out *say.Speaker) error {
	n := newcommand.Name{Verb: args[len(args)-1]}
	if len(args) == 2 {
		n.Group = args[0]
	}
	tree := os.DirFS(dir)
	rows, err := plannedRows(tree)
	if err != nil {
		return err
	}
	taken, err := takenIn(tree, rows)
	if err != nil {
		return err
	}
	if err := newcommand.Refuse(words(), n, taken); err != nil {
		return err
	}
	var seed *newcommand.Seed
	if r, ok := rows[n.Family()]; ok {
		s := seedOf(r)
		seed = &s
	}
	plan, err := newcommand.Make(command.Files, tree, n, seed)
	if err != nil {
		return err
	}
	out.Text("gragas-new command " + n.String() + "\n")
	for _, p := range plan.Paths() {
		out.Text("  " + p + "\n")
	}
	if !apply {
		out.Text("nothing written: add --apply to write these files\n")
		return nil
	}
	if err := write(dir, plan); err != nil {
		return err
	}
	out.Text(written)
	if plan.Note != "" {
		out.Text(plan.Note + "\n")
	}
	return nil
}

// write lays the plan down: new files that must not exist, then the removal of
// the planned file the command replaces. A file already made is removed when a
// later step fails; a removal that fails leaves the planned file where it was.
func write(dir string, plan newcommand.Plan) (err error) {
	var made []string
	defer func() {
		if err != nil {
			for _, p := range made {
				err = errors.Join(err, os.Remove(p))
			}
		}
	}()
	for _, path := range slices.Sorted(maps.Keys(plan.Files)) {
		text := plan.Files[path]
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		made = append(made, full)
		_, werr := io.WriteString(f, text)
		if err := errors.Join(werr, f.Close()); err != nil {
			return err
		}
	}
	for _, path := range plan.Removes {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

// written closes a run that wrote. The registry's two goldens change when the
// stub row joins the registry, and they are protected: say so before the test
// goes red.
const written = "written; task gen joins it; the stubs fail until they are filled in\n" +
	"the registry changed: regenerate internal/cli/root/testdata/help-commands.golden and\n" +
	"internal/cli/root/testdata/readme-table.golden with go test ./internal/cli/root -run TestHelpTheMenuAndTheReadmeTableAreTheRegistrysAsGoldens -update,\n" +
	"read the diff, and list both as covers: they are protected\n"
