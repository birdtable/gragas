package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/gitenv/gittest"
	testenv "github.com/birdtable/go-testkit"
)

// put writes body to name in dir, making the folders on the way.
func put(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitIn runs git in dir as the stand-in person and returns what it printed.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	// The identity and its dates are fixed, so one base makes one commit id in
	// every repository a test makes; they come last over gittest.Git's own, and
	// win. gittest.Git strips every GIT_ the process carries and names no home of
	// the person's.
	identity := []string{"GIT_AUTHOR_NAME=Alex", "GIT_AUTHOR_EMAIL=alex@example.com", "GIT_COMMITTER_NAME=Alex", "GIT_COMMITTER_EMAIL=alex@example.com",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z"}
	out, err := outputIn(t.Context(), dir, overVars(gittest.Git(dir), identity), "git", args...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// git runs git in dir with an identity, failing the test on error.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false")
	cmd.Args = append(cmd.Args, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// countRE is the count line a check prints when it passes (gragas#695), with
// the unit the test names.
var countRE = regexp.MustCompile(`^count: (0|[1-9][0-9]{0,8}) ([a-z][a-z-]{0,31})$`)

// counted splits out's last line off as the count, fails the test unless it is
// a count line of unit, and answers the rest of out and the number.
func counted(t *testing.T, out, unit string) (rest string, n int) {
	t.Helper()
	body := strings.TrimSuffix(out, "\n")
	i := strings.LastIndexByte(body, '\n')
	last := body[i+1:]
	m := countRE.FindStringSubmatch(last)
	if m == nil || m[2] != unit {
		t.Errorf("the output does not end on a count of %s:\n%s", unit, out)
		return out, -1
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	return body[:i+1], n
}

// relink puts a symbolic link to to where at was.
func relink(t *testing.T, at, to string) {
	t.Helper()
	if err := os.RemoveAll(at); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, to, at)
}
