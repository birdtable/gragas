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
// member, or with a member that is not named by its absolute path.
var errBlockedUsage = errors.New("usage: " + exit.BlockedWord + " -- <program by its absolute path> [arguments]")

// runBlocked is gragas-dev started as the blocked child: args are what
// follows exit.BlockedWord, "--", the member's absolute path and its
// arguments. It sets go-proc's block on this process and execs the member
// (sandbox.Run, gragas#1455), so it returns only when the member was not run.
func runBlocked(args, environ []string, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	if len(args) < 2 || args[0] != "--" || !filepath.IsAbs(args[1]) {
		talk.Text("gragas-dev " + errBlockedUsage.Error() + "\n")
		return exitUsage
	}
	err := sandbox.Run(args[1], args[2:], environ)
	talk.Text("gragas-dev " + exit.BlockedWord + ": " + err.Error() + "\n")
	if errors.Is(err, sandbox.ErrUsage) {
		return exitUsage
	}
	return exitFailed
}
