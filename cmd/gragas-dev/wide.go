package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"github.com/birdtable/gragas/internal/dev/say"
	"io"
	"maps"
	"path"
	"slices"
	"strings"
)

const wideUsage = `usage: gragas-dev wide [-C dir] <base>
`

// runWide is the wide command, which tells the push the widely imported packages it changes.
// It reads the Go files changed between <base> and HEAD and, for each package among them that more
// than half of the module's packages depend on, prints one line: the package and how many packages
// depend on it of how many there are. Go's test cache re-runs every one of them, so the push may
// go over its budget, and the tier's footer prints the push's time. A go.mod or go.sum change
// reaches every package; a package few depend on prints nothing. It never fails on what it finds:
// it tells, and the push goes on. It is the Go form of the sh script it replaced, so it runs on a
// system with no sh (gragas#1346).
func runWide(ctx context.Context, environ, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("wide", flag.ContinueOnError)
	fl.SetOutput(stderr)
	root := fl.String("C", ".", "the module's root")
	if err := fl.Parse(args); err != nil || fl.NArg() != 1 {
		talk.Text(wideUsage)
		return exitUsage
	}
	dir, base := *root, fl.Arg(0)
	diff, err := output(ctx, environ, dir, nil, "git", "diff", "--name-only", base+"...HEAD", "--", "*.go", "go.mod", "go.sum")
	if err != nil {
		talk.Text("gragas-dev wide: " + err.Error() + "\n")
		return exitFailed
	}
	modOut, err := goOver(ctx, environ, dir, "list", "-m")
	if err != nil {
		talk.Text("gragas-dev wide: " + err.Error() + "\n")
		return exitFailed
	}
	mod := strings.TrimSpace(string(modOut))
	listed, err := goOver(ctx, environ, dir, "list", "-e", "-f", "{{.ImportPath}} {{join .Deps \" \"}}", "./...")
	if err != nil {
		talk.Text("gragas-dev wide: " + err.Error() + "\n")
		return exitFailed
	}
	total, users := wideDeps(listed)
	out := &say.Speaker{W: stdout}
	for _, line := range wideTell(mod, string(diff), total, users) {
		out.Text(line + "\n")
	}
	return exitDone
}

// wideDeps is how many packages go list printed and, for each package, how many of them depend on it.
func wideDeps(listed []byte) (total int, users map[string]int) {
	users = map[string]int{}
	sc := bufio.NewScanner(bytes.NewReader(listed))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		total++
		for _, d := range slices.Compact(slices.Sorted(slices.Values(f[1:]))) {
			users[d]++
		}
	}
	return total, users
}

// wideTell is the line for each package of the changed files that more than half of the total depend on.
func wideTell(mod, changedFiles string, total int, users map[string]int) []string {
	changed := map[string]bool{}
	for f := range strings.FieldsSeq(changedFiles) {
		switch d := path.Dir(f); {
		case f == "go.mod" || f == "go.sum":
			changed["*"] = true
		case d == ".":
			changed[mod] = true
		default:
			changed[mod+"/"+d] = true
		}
	}
	var lines []string
	for _, pkg := range slices.Sorted(maps.Keys(changed)) {
		n, name := users[pkg], pkg
		if pkg == "*" {
			n, name = total, "go.mod or go.sum"
		}
		if n > 0 && n*2 > total {
			lines = append(lines, fmt.Sprintf("wide: %s is a dependency of %d of %d packages: a change to it re-runs them all, so this push may go over its budget", name, n, total))
		}
	}
	return lines
}
