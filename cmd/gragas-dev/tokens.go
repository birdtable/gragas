package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/dev/scan"
	"github.com/birdtable/gragas/internal/dev/tokens"
)

// tokensFlags reads the flags of the tokens command. ok is false when they are
// not the command's, after the usage has been said.
func tokensFlags(args []string, stderr io.Writer) (check bool, dir string, ok bool) {
	fl := flag.NewFlagSet("tokens", flag.ContinueOnError)
	fl.SetOutput(stderr)
	c := fl.Bool("check", false, "write nothing; fail when a file generated from "+tokens.File+" is stale")
	d := fl.String("C", ".", "the repository holding the token document")
	if err := fl.Parse(args); err != nil || fl.NArg() != 0 {
		(&say.Speaker{W: stderr}).Text(usage)
		return false, "", false
	}
	return *c, *d, true
}

// runTokens is the tokens command: it writes the three files the explorer's
// token document generates, or under --check says which of them is stale and
// writes none.
func runTokens(args []string, stdout, stderr io.Writer) int {
	check, dir, ok := tokensFlags(args, stderr)
	if !ok {
		return exitUsage
	}
	if err := tokensCommand(dir, check, &say.Speaker{W: stdout}); err != nil {
		(&say.Speaker{W: stderr}).Text("gragas-dev tokens: " + err.Error() + "\n")
		return exitFailed
	}
	return exitDone
}

// tokenFiles reads the token document in root, refuses it when a pair is below
// its floor, and renders the three files it generates.
func tokenFiles(root *os.Root, out *say.Speaker) ([]rendered, error) {
	doc, err := root.ReadFile(tokens.File)
	if err != nil {
		return nil, err
	}
	d, err := tokens.ParseBytes(doc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", tokens.File, err)
	}
	if low := d.Low(); len(low) > 0 {
		for _, l := range low {
			out.Text("low " + l + "\n")
		}
		return nil, errors.Join(fmt.Errorf("%s: %d pairs below their floor", tokens.File, len(low)), out.Err)
	}
	palette, err := d.Palette()
	if err != nil {
		return nil, err
	}
	return []rendered{{tokens.CSSFile, d.CSS()}, {tokens.ThemeFile, d.Theme()}, {tokens.PaletteFile, palette}}, nil
}

// staleOf lists the files whose bytes on disk are not the rendered ones, a
// missing file included.
func staleOf(root *os.Root, files []rendered) ([]rendered, error) {
	var stale []rendered
	for _, r := range files {
		have, err := root.ReadFile(r.path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err != nil || !bytes.Equal(have, r.data) {
			stale = append(stale, r)
		}
	}
	return stale, nil
}

// tokensCommand reads the token document in dir, refuses it when a pair is
// below its floor, and writes the generated files that differ, or under check
// lists them and writes none. A check that finds them current ends on its
// count line (gragas#695): the files it compared.
func tokensCommand(dir string, check bool, out *say.Speaker) (err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	files, err := tokenFiles(root, out)
	if err != nil {
		return err
	}
	stale, err := staleOf(root, files)
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		out.Text("the token files are current\n")
		if check {
			out.Text(scan.CountLine(len(files), "files") + "\n")
		}
		return out.Err
	}
	if check {
		for _, r := range stale {
			out.Text("stale " + r.path + "\n")
		}
		return errors.Join(errStale, out.Err)
	}
	for _, r := range stale {
		if err := replaceIn(root, r.path, r.data); err != nil {
			return err
		}
		out.Text("wrote " + r.path + "\n")
	}
	return out.Err
}
