package main

import (
	"errors"
	"io"
	"path/filepath"

	"github.com/birdtable/go-proc/sandbox"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/exit"
)

// errBlockedUsage is the blocked child started without --, without a
// program, or with a program that is not named by its absolute path.
var errBlockedUsage = errors.New("usage: " + exit.BlockedWord + " -- <program by its absolute path> [arguments]")

// runBlocked is gragas-release started as the blocked child (main's
// blockedChild): args are what follows exit.BlockedWord, "--", the program's
// absolute path and its arguments. It sets go-proc's block on this process
// and execs the program (sandbox.Run), so it returns only when the
// program was not run.
func runBlocked(args, environ []string, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	if len(args) < 2 || args[0] != "--" || !filepath.IsAbs(args[1]) {
		talk.Text("gragas-release " + errBlockedUsage.Error() + "\n")
		return exit.Usage
	}
	err := sandbox.Run(args[1], args[2:], environ)
	talk.Text("gragas-release " + exit.BlockedWord + ": " + err.Error() + "\n")
	if errors.Is(err, sandbox.ErrUsage) {
		return exit.Usage
	}
	return exit.Failed
}
