package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// lapRun runs one lap word in dir at the instant t0+at, and returns its exit code and what it printed.
func lapRun(t *testing.T, dir string, at time.Duration, words ...string) (int, string, string) {
	t.Helper()
	t0 := time.Date(2026, 11, 22, 3, 0, 0, 0, time.UTC)
	var out, errb bytes.Buffer
	code := runLap(context.Background(), append([]string{"--dir", dir}, words...), func() time.Time { return t0.Add(at) }, &out, &errb)
	return code, out.String(), errb.String()
}

// TestLapReportsEachMemberSlowestFirstAndMarksOneThatNeverEnded: members that begin and end side by side
// are each timed from their own two lines, the report lists them slowest first, a member that began and
// never ended (it failed, or the tier was killed) is marked with the time it had run, and a member that
// ran twice is the later run. Planted failure: a report ordered as the members began, or one that drops
// the member with no end, goes red.
func TestLapReportsEachMemberSlowestFirstAndMarksOneThatNeverEnded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	steps := []struct {
		at    time.Duration
		words []string
	}{
		{0, []string{"reset"}},
		{time.Second, []string{"begin", "format"}},
		{time.Second, []string{"begin", "vet-system"}},
		{2 * time.Second, []string{"begin", "lint-diff-system"}},
		{3 * time.Second, []string{"end", "format"}},
		{9 * time.Second, []string{"end", "vet-system"}},
		{12 * time.Second, []string{"begin", "tidy"}},
		{12 * time.Second, []string{"begin", "vet-system-linux/amd64"}},
	}
	for _, s := range steps {
		if code, _, errb := lapRun(t, dir, s.at, s.words...); code != 0 || errb != "" {
			t.Fatalf("lap %v = %d, %q; want 0 and nothing said", s.words, code, errb)
		}
	}
	code, out, _ := lapRun(t, dir, 14*time.Second, "report")
	if code != 0 {
		t.Fatalf("report = %d", code)
	}
	var names []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n")[1:] {
		f := strings.Fields(l)
		if strings.Contains(l, "never ended") {
			f[2] = "unfinished:" + f[2]
		}
		names = append(names, f[2])
	}
	want := []string{"unfinished:lint-diff-system", "vet-system", "format", "unfinished:tidy", "unfinished:vet-system-linux/amd64"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("the report lists %v, want %v (the member with no end ran 12 s, vet 8 s, format 2 s, the other 2 s):\n%s", names, want, out)
	}
	if !strings.Contains(out, "5 members") || !strings.Contains(out, "12s") {
		t.Errorf("the head names the count and the span the tier ran:\n%s", out)
	}
}

// TestLapNeverFailsATierAndSaysWhatItCannotTime: a lap with no tools folder, a name with a space and a
// report with nothing to report all end 0 (a lap is a note, never a check); a missing or unknown word
// and a begin with no name are the one usage failure, which the tests of the Taskfile keep out of a tier.
func TestLapNeverFailsATierAndSaysWhatItCannotTime(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if code, _, errb := lapRun(t, filepath.Join(dir, "missing"), 0, "begin", "x"); code != 0 || !strings.Contains(errb, "not timed") {
		t.Errorf("a folder that is not there: %d, %q; want 0 and a note", code, errb)
	}
	if code, _, errb := lapRun(t, dir, 0, "begin", "a b"); code != 0 || !strings.Contains(errb, "not timed") {
		t.Errorf("a name with a space: %d, %q; want 0 and a note", code, errb)
	}
	if code, out, _ := lapRun(t, dir, 0, "report"); code != 0 || out != "" {
		t.Errorf("a report of no log: %d, %q; want 0 and nothing", code, out)
	}
	for _, words := range [][]string{nil, {"stop"}, {"begin"}, {"end", "a", "b"}, {"reset", "x"}, {"report", "x"}} {
		if code, _, _ := lapRun(t, dir, 0, words...); code != exitUsage {
			t.Errorf("lap %v = %d, want the usage code %d", words, code, exitUsage)
		}
	}
}

// TestLapReadsPastAGarbledLine: a line the report cannot read (a torn write, a word it does not know) is
// skipped, and the members on the lines around it are still timed.
func TestLapReadsPastAGarbledLine(t *testing.T) {
	t.Parallel()
	log := "begin a 1000000000\nbegi\nend a 3000000000\nsurprise b notanumber\nbegin b 4000000000\nend b 5000000000\n"
	out := lapReport(log, time.Unix(10, 0))
	if !strings.Contains(out, "2 members") || !strings.Contains(out, "2s") || !strings.Contains(out, "1s") {
		t.Errorf("the two members are timed past the garbled lines:\n%s", out)
	}
	if lapReport("", time.Unix(10, 0)) != "" {
		t.Error("a log with no member reports nothing")
	}
}
