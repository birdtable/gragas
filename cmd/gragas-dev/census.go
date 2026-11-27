package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/birdtable/gragas/internal/dev/census"
	"github.com/birdtable/gragas/internal/dev/say"
)

// runCensus is the census command: the standard library's identifiers that
// differ by system (gragas#1750), one a line with why, from the toolchain
// the go on PATH is. internal/osapi_test.go holds gate/os-api.txt to the same
// census; the command is how a person reads it.
func runCensus(ctx context.Context, environ, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("census", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "the folder the go tool runs in")
	if err := fl.Parse(args); err != nil || fl.NArg() != 0 {
		talk.Text(usage)
		return exitUsage
	}
	goTool := censusGo(environ, *dir)
	root, err := goTool(ctx, nil, "env", "GOROOT")
	if err != nil {
		talk.Text("gragas-dev census: " + err.Error() + "\n")
		return exitFailed
	}
	c, err := census.Take(ctx, goTool, os.DirFS(strings.TrimSpace(string(root))))
	if err != nil {
		talk.Text("gragas-dev census: " + err.Error() + "\n")
		return exitFailed
	}
	(&say.Speaker{W: stdout}).Text(c.TSV())
	talk.Text(fmt.Sprintf("%s%d packages and %d identifiers read for %d ports; %d differ by system\n", reportMark, c.Packages, c.Read, len(c.Ports), len(c.Differ)))
	return exitDone
}

// censusGo runs the go tool in dir under environ with goEnv, the census's own
// variables after it.
func censusGo(environ []string, dir string) census.Go {
	return func(ctx context.Context, env []string, args ...string) ([]byte, error) {
		return output(ctx, environ, dir, append(goEnv(), env...), "go", args...)
	}
}
