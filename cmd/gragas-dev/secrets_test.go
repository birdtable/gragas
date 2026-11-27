package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"
)

// The count of gragas-dev secrets is the sh script's awk (gragas#1655, X171): a commit counts once, when a file it
// does not delete holds a hunk. The plants of internal/secrets_test.go run it against gitleaks; these hold its
// reading of the log's lines.
func TestTheSecretCountIsCommitsWithAHunkInAFileTheyDoNotDelete(t *testing.T) {
	t.Parallel()
	commit := func(h string) string { return "\x01" + h + "\n" }
	file := func(name string, deleted bool, hunks int) string {
		s := "diff --git a/" + name + " b/" + name + "\n"
		if deleted {
			s += "deleted file mode 100644\n"
		}
		s += "index 1..2\n--- a/" + name + "\n+++ b/" + name + "\n"
		for range hunks {
			// A hunk's own lines start with + or -, so one that reads as a header is not one.
			s += "@@ -1 +1 @@\n-diff --git a/x b/x\n+@@ not a header\n"
		}
		return s
	}
	for _, c := range []struct {
		name string
		log  string
		want int
	}{
		{"nothing", "", 0},
		{"an empty commit", commit("a"), 0},
		{"one hunk", commit("a") + file("f", false, 1), 1},
		{"two files and three hunks count once", commit("a") + file("f", false, 2) + file("g", false, 1), 1},
		{"a delete alone", commit("a") + file("f", true, 1), 0},
		{"a delete and an edit", commit("a") + file("f", true, 1) + file("g", false, 1), 1},
		{"an empty file or a mode alone", commit("a") + "diff --git a/e b/e\nnew file mode 100644\nindex 0..1\n", 0},
		{"three commits, one with nothing", commit("a") + file("f", false, 1) + commit("b") + commit("c") + file("g", false, 1), 2},
		{"no newline at the end", commit("a") + strings.TrimSuffix(file("f", false, 1), "\n"), 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := countHunkCommits(strings.NewReader(c.log))
			if err != nil || got != c.want {
				t.Errorf("got %d, %v; want %d", got, err, c.want)
			}
			// Read a byte at a time, the count is the same.
			got, err = countHunkCommits(iotest.OneByteReader(strings.NewReader(c.log)))
			if err != nil || got != c.want {
				t.Errorf("a byte at a time: got %d, %v; want %d", got, err, c.want)
			}
		})
	}
}

// A line longer than the reader's buffer is read through: what follows it in the same line is not a line's start.
func TestTheSecretCountReadsALineLongerThanItsBuffer(t *testing.T) {
	t.Parallel()
	long := "+" + strings.Repeat("x", 200*1024) + "@@ \x01 diff --git a/x b/x\n"
	log := "\x01a\ndiff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n" + long + "\x01b\n"
	got, err := countHunkCommits(strings.NewReader(log))
	if err != nil || got != 1 {
		t.Errorf("got %d, %v; want 1", got, err)
	}
}

func TestTheSecretCountSaysAReadThatFails(t *testing.T) {
	t.Parallel()
	r := io.MultiReader(bytes.NewReader([]byte("\x01a\n")), iotest.ErrReader(io.ErrUnexpectedEOF))
	if _, err := countHunkCommits(r); err == nil {
		t.Error("a failed read: no error")
	}
}

func TestSecretsRefusesAPassWithoutItsSettings(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{},
		{"--gitleaks", "g", "--config", "/repo/c"},
		{"--scan", "tree", "--config", "/repo/c"},
		{"--scan", "tree", "--gitleaks", "g"},
		{"--scan", " ", "--gitleaks", "g", "--config", "/repo/c"},
		{"--scan", "tree", "--gitleaks", "g", "--config", "/repo/c", "extra"},
	} {
		var out, errb bytes.Buffer
		if code := runSecrets(t.Context(), os.Environ(), args, &out, &errb); code != exitUsage {
			t.Errorf("%q: exit %d, want %d", args, code, exitUsage)
		}
	}
}
