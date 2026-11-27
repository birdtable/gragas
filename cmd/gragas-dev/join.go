package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/birdtable/gragas/internal/dev/join"
	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/dev/say"
)

// runGen is the gen command: its one generator so far is commands, the join of
// the packages below internal/cli/cmd.
func runGen(args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	if len(args) == 0 || args[0] != "commands" {
		talk.Text(usage)
		return exitUsage
	}
	fl := flag.NewFlagSet("gen commands", flag.ContinueOnError)
	fl.SetOutput(stderr)
	check := fl.Bool("check", false, "write nothing; fail when "+join.File+" is stale")
	dir := fl.String("C", ".", "the repository holding the command packages")
	if err := fl.Parse(args[1:]); err != nil || fl.NArg() != 0 {
		talk.Text(usage)
		return exitUsage
	}
	if err := joinCommand(*dir, *check, &say.Speaker{W: stdout}); err != nil {
		talk.Text("gragas-dev gen commands: " + err.Error() + "\n")
		return exitFailed
	}
	return exitDone
}

// joinCommand writes the join of the command packages in dir, or under check
// says whether the one on disk is current and writes none. A check that finds
// it current ends on its count line (gragas#695): the one file it compared.
func joinCommand(dir string, check bool, out *say.Speaker) (err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	table, err := root.ReadFile(layers.File)
	if err != nil {
		return err
	}
	t, err := layers.Parse(table)
	if err != nil {
		return fmt.Errorf("%s: %w", layers.File, err)
	}
	names, err := join.Names(root.FS())
	if err != nil {
		return err
	}
	want, err := join.Render(t.Module, names)
	if err != nil {
		return err
	}
	have, err := root.ReadFile(join.File)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil && bytes.Equal(have, want) {
		out.Text("the join of the commands is current\n")
		if check {
			out.Text(countLine(1, "files") + "\n")
		}
		return out.Err
	}
	if check {
		out.Text("stale " + join.File + "\n")
		return errors.Join(errStale, out.Err)
	}
	if err := replaceIn(root, join.File, want); err != nil {
		return err
	}
	out.Text("wrote " + join.File + "\n")
	return out.Err
}
