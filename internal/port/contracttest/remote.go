package contracttest

import (
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// RemoteSetup is the Remote under test and what the suite runs it on.
type RemoteSetup struct {
	R port.Remote
	// Root is a repository with a commit, whose file "rules.yaml" holds
	// "one\n" at that commit and in the work tree.
	Root model.Root
	// Write sets the file at the slash path p of Root's work tree to c.
	Write func(p, c string)
	// From is a source that holds the tag v1, whose files are TheRelease.
	From string
	// Read answers the file at the slash path p of a fetched root.
	Read func(root model.Root, p string) (string, error)
}

// RemoteMaker makes the setup for one subtest.
type RemoteMaker func(t *testing.T) RemoteSetup

// TheRelease is the files of the tag v1 of the suite's source.
func TheRelease() map[string]string {
	return map[string]string{"rules.yaml": "r: 1\n", "presets/a.json": "{}\n"}
}

// Remote runs the Remote suite against the port build answers: a fetch
// answers the tag's files, again the same, and refuses what it cannot
// fetch; a commit of a change is made once and refuses what is not one;
// a tag is made once on a commit and refuses what it lacks.
func Remote(t *testing.T, build RemoteMaker) {
	t.Helper()
	t.Run("a fetched tag is a root holding its files", func(t *testing.T) { remoteFetch(t, build(t)) })
	t.Run("a fetch with no source, no tag or a tag not there is an error", func(t *testing.T) { remoteFetchRefuses(t, build(t)) })
	t.Run("a commit of a change answers its name, and the same again is an error", func(t *testing.T) { remoteCommit(t, build(t)) })
	t.Run("a commit with no path or no message is an error", func(t *testing.T) { remoteCommitRefuses(t, build(t)) })
	t.Run("a tag is made once, and one with no message is an error", func(t *testing.T) { remoteTag(t, build(t)) })
}

func remoteFetch(t *testing.T, s RemoteSetup) {
	t.Helper()
	root, err := s.R.Fetch(t.Context(), s.From, "v1")
	if err != nil {
		t.Fatalf("Fetch(v1): %v", err)
	}
	for p, want := range TheRelease() {
		if got, err := s.Read(root, p); err != nil || got != want {
			t.Errorf("the fetched %s = %q, %v; want %q", p, got, err, want)
		}
	}
	again, err := s.R.Fetch(t.Context(), s.From, "v1")
	if err != nil || again != root {
		t.Errorf("Fetch(v1) again = %q, %v; want %q", again, err, root)
	}
}

func remoteFetchRefuses(t *testing.T, s RemoteSetup) {
	t.Helper()
	for _, c := range [][2]string{{s.From, "v9"}, {s.From, ""}, {"", "v1"}} {
		if got, err := s.R.Fetch(t.Context(), c[0], c[1]); err == nil || got != "" {
			t.Errorf("Fetch(%q, %q) = %q, %v; want an error and no root", c[0], c[1], got, err)
		}
	}
}

// isName holds a commit's name to 40 or 64 lower-case hexadecimal digits.
func isName(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func remoteCommit(t *testing.T, s RemoteSetup) {
	t.Helper()
	s.Write("rules.yaml", "two\n")
	first, err := s.R.Commit(t.Context(), s.Root, []string{"rules.yaml"}, "two")
	if err != nil || !isName(first) {
		t.Fatalf("Commit(rules.yaml) = %q, %v; want a commit's full name", first, err)
	}
	if got, err := s.R.Commit(t.Context(), s.Root, []string{"rules.yaml"}, "two again"); err == nil || got != "" {
		t.Errorf("Commit of no change = %q, %v; want an error and no commit", got, err)
	}
	s.Write("rules.yaml", "three\n")
	second, err := s.R.Commit(t.Context(), s.Root, []string{"rules.yaml"}, "three")
	if err != nil || !isName(second) || second == first {
		t.Errorf("Commit(rules.yaml) again = %q, %v; want a commit's full name other than %s", second, err, first)
	}
}

func remoteCommitRefuses(t *testing.T, s RemoteSetup) {
	t.Helper()
	s.Write("rules.yaml", "two\n")
	cases := []struct {
		paths   []string
		message string
	}{{nil, "m"}, {[]string{}, "m"}, {[]string{"rules.yaml"}, ""}}
	for _, c := range cases {
		if got, err := s.R.Commit(t.Context(), s.Root, c.paths, c.message); err == nil || got != "" {
			t.Errorf("Commit(%q, %q) = %q, %v; want an error and no commit", c.paths, c.message, got, err)
		}
	}
	if _, err := s.R.Commit(t.Context(), s.Root, []string{"rules.yaml"}, "two"); err != nil {
		t.Errorf("Commit after the refusals: %v; want the change still there to commit", err)
	}
}

func remoteTag(t *testing.T, s RemoteSetup) {
	t.Helper()
	s.Write("rules.yaml", "two\n")
	commit, err := s.R.Commit(t.Context(), s.Root, []string{"rules.yaml"}, "two")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.R.Tag(t.Context(), s.Root, "v2", commit, ""); err == nil {
		t.Error("Tag with no message made a tag")
	}
	if err := s.R.Tag(t.Context(), s.Root, "v2", commit, "release v2"); err != nil {
		t.Fatalf("Tag(v2): %v", err)
	}
	if err := s.R.Tag(t.Context(), s.Root, "v2", commit, "release v2"); err == nil {
		t.Error("Tag(v2) again made a tag that exists")
	}
}
