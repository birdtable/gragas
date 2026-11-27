package main

import (
	"slices"
	"strings"
	"testing"
)

// mainEdit puts a commit on origin/main, on top of base, that writes body to file, and leaves
// the clone's checkout as it was.
func mainEdit(t *testing.T, dir, base, file, body string) {
	t.Helper()
	gitIn(t, dir, "checkout", "-q", "-b", "side", base)
	put(t, dir, file, body)
	gitIn(t, dir, "add", file)
	gitIn(t, dir, "commit", "-q", "-m", "main moved")
	sha := gitIn(t, dir, "rev-parse", "HEAD")
	gitIn(t, dir, "checkout", "-q", "topic")
	gitIn(t, dir, "update-ref", mainRef, sha)
}

// TestMergeableHeadIsHeldBeforeAnyMemberRuns holds gragas#1733, class 3: the
// pre-push hook asks git whether the head merges into origin/main, and a head that conflicts is
// refused naming the files, with no member run. The control is a head main has not touched, which
// runs the tier; and a clone with no origin/main is refused and says so, never passed because git
// answered nothing.
func TestMergeableHeadIsHeldBeforeAnyMemberRuns(t *testing.T) {
	t.Parallel()
	t.Run("a clean control", func(t *testing.T) {
		t.Parallel()
		dir, log, first, second := hookClone(t, fakeTaskConfig{}, "")
		mainEdit(t, dir, first, "c.txt", "main's change elsewhere\n")
		if code, out := prePushIn(t, dir, second, "refs/heads/topic"); code != 0 {
			t.Fatalf("a head main has not touched failed with %d:\n%s", code, out)
		}
		if got := taskCalls(t, log); len(got) == 0 {
			t.Error("the clean head ran no member")
		}
	})
	t.Run("a conflict", func(t *testing.T) {
		t.Parallel()
		dir, log, first, second := hookClone(t, fakeTaskConfig{}, "")
		mainEdit(t, dir, first, "a.txt", "three\n")
		code, out := prePushIn(t, dir, second, "refs/heads/topic")
		if code == 0 || !strings.Contains(out, "does not merge into origin/main") || !strings.Contains(out, "a.txt") {
			t.Errorf("a head that conflicts gave %d, want it refused naming a.txt:\n%s", code, out)
		}
		if got := taskCalls(t, log); len(got) != 0 {
			t.Errorf("a refused head ran %q", got)
		}
	})
	t.Run("origin/main not fetched", func(t *testing.T) {
		t.Parallel()
		dir, log, _, second := hookClone(t, fakeTaskConfig{}, "")
		gitIn(t, dir, "update-ref", "-d", mainRef)
		code, out := prePushIn(t, dir, second, "refs/heads/topic")
		if code == 0 || !strings.Contains(out, "origin/main is not fetched") || !strings.Contains(out, "git fetch origin main") {
			t.Errorf("a clone with no origin/main gave %d, want it refused saying so:\n%s", code, out)
		}
		if got := taskCalls(t, log); len(got) != 0 {
			t.Errorf("a refused head ran %q", got)
		}
	})
}

// TestMergeableReadsTheConflictedFilesFromMergeTreeOutput holds the reader of git's answer: the paths after
// the first line, up to the blank line, each once; an answer of one line names none.
func TestMergeableReadsTheConflictedFilesFromMergeTreeOutput(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		in   string
		want []string
	}{
		{"the paths up to the blank line", "abc\na.txt\nb/c.go\n\nAuto-merging", []string{"a.txt", "b/c.go"}},
		{"a path named twice", "abc\na.txt\na.txt\n", []string{"a.txt"}},
		{"a tree line only", "abc\n", nil},
		{"no answer", "", nil},
	} {
		if got := conflictFiles(c.in); !slices.Equal(got, c.want) {
			t.Errorf("%s: conflictFiles(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// growFromRelease puts a commit on origin/release-1 on top of first that writes file, then a head on top
// of that commit that writes headBody to a.txt, and leaves it checked out as HEAD; it answers the head.
func growFromRelease(t *testing.T, dir, first, file, body, headBody string) string {
	t.Helper()
	gitIn(t, dir, "checkout", "-q", "-b", "release-side", first)
	put(t, dir, file, body)
	gitIn(t, dir, "add", file)
	gitIn(t, dir, "commit", "-q", "-m", "release moved")
	gitIn(t, dir, "update-ref", releaseRef, gitIn(t, dir, "rev-parse", "HEAD"))
	gitIn(t, dir, "checkout", "-q", "-b", "grown")
	put(t, dir, "a.txt", headBody)
	gitIn(t, dir, "commit", "-q", "-am", "grown")
	return gitIn(t, dir, "rev-parse", "HEAD")
}

// TestMergeableChecksAReleaseHeadAgainstItsRelease holds the release base: a head that descends from a commit
// of origin/main..origin/release-1 merges into origin/release-1, and any other head into origin/main. Each
// way is planted: a release head that conflicts with main but not with the release passes; one that
// conflicts with the release is refused naming it; a head that grew from main alone is checked against main
// though the release would conflict with it.
func TestMergeableChecksAReleaseHeadAgainstItsRelease(t *testing.T) {
	t.Parallel()
	t.Run("a release head that conflicts only with main passes", func(t *testing.T) {
		t.Parallel()
		dir, log, first, _ := hookClone(t, fakeTaskConfig{}, "")
		head := growFromRelease(t, dir, first, "r.txt", "release work\n", "two\n")
		mainEdit(t, dir, first, "a.txt", "three\n")
		gitIn(t, dir, "checkout", "-q", "grown")
		if code, out := prePushIn(t, dir, head, "refs/heads/grown"); code != 0 {
			t.Fatalf("a head that merges into origin/release-1 was refused with %d:\n%s", code, out)
		}
		if got := taskCalls(t, log); len(got) == 0 {
			t.Error("the release head ran no member")
		}
	})
	t.Run("a release head that conflicts with the release is refused", func(t *testing.T) {
		t.Parallel()
		dir, log, first, _ := hookClone(t, fakeTaskConfig{}, "")
		head := growFromRelease(t, dir, first, "r.txt", "release work\n", "two\n")
		gitIn(t, dir, "checkout", "-q", "release-side")
		put(t, dir, "a.txt", "four\n")
		gitIn(t, dir, "commit", "-q", "-am", "release moved on")
		gitIn(t, dir, "update-ref", releaseRef, gitIn(t, dir, "rev-parse", "HEAD"))
		gitIn(t, dir, "checkout", "-q", "grown")
		code, out := prePushIn(t, dir, head, "refs/heads/grown")
		if code == 0 || !strings.Contains(out, "does not merge into origin/release-1") || !strings.Contains(out, "a.txt") {
			t.Errorf("a head that conflicts with the release gave %d, want it refused naming origin/release-1 and a.txt:\n%s", code, out)
		}
		if got := taskCalls(t, log); len(got) != 0 {
			t.Errorf("a refused head ran %q", got)
		}
	})
	t.Run("a head from main alone is checked against main", func(t *testing.T) {
		t.Parallel()
		dir, log, first, second := hookClone(t, fakeTaskConfig{}, "")
		gitIn(t, dir, "checkout", "-q", "-b", "release-side", first)
		put(t, dir, "a.txt", "five\n")
		gitIn(t, dir, "commit", "-q", "-am", "the release edits a.txt")
		gitIn(t, dir, "update-ref", releaseRef, gitIn(t, dir, "rev-parse", "HEAD"))
		gitIn(t, dir, "checkout", "-q", "topic")
		if code, out := prePushIn(t, dir, second, "refs/heads/topic"); code != 0 {
			t.Fatalf("a head that grew from main was refused against the release with %d:\n%s", code, out)
		}
		if got := taskCalls(t, log); len(got) == 0 {
			t.Error("the head from main ran no member")
		}
	})
}
