package main

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/digest"
)

// runPlace is `place SRC DST`: DST is the program SRC holds, and SRC is gone.
// DST is left alone, its modification time included, when it already holds
// SRC's bytes, so a build with no source change leaves bin/gragas as it was
// (gragas#1381): Go's test cache keys the tests that stat bin/gragas on its
// modification time, and a build that wrote the same bytes again made five
// packages miss on every push. A different or missing DST is written through
// writeProgram, executable, beside it and then moved over it.
func runPlace(args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	if len(args) != 2 {
		talk.Text(usage)
		return exitUsage
	}
	replaced, err := place(args[0], args[1])
	if err != nil {
		talk.Text("gragas-dev place: " + err.Error() + "\n")
		return exitFailed
	}
	out := &say.Speaker{W: stdout}
	if replaced {
		out.Text("place: " + args[1] + " is the new program\n")
	} else {
		out.Text("place: " + args[1] + " already holds those bytes, left as it was\n")
	}
	return exitDone
}

// place makes dst hold src's bytes as a program and removes src. replaced is
// false when dst held them already and was not touched. The bytes are compared
// by digest.Hex, the one owner of the sum; the write is writeProgram's.
func place(src, dst string) (replaced bool, err error) {
	program, err := os.ReadFile(src)
	if err != nil {
		return false, err
	}
	have, err := os.ReadFile(dst)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return false, err
	case digest.Hex(have) == digest.Hex(program):
		return false, os.Remove(src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	if err := writeProgram(dst, program); err != nil {
		return false, err
	}
	return true, os.Remove(src)
}
