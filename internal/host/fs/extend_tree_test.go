package fs

import (
	"os"
	"path/filepath"
	"testing"

	testenv "github.com/birdtable/go-testkit"
)

// repoOf is a real folder opened as the program opens it (Open), and the
// folder beside it that a link may lead to.
func repoOf(t *testing.T) (repo Repo, dir, outside string) {
	t.Helper()
	base := t.TempDir()
	dir, outside = filepath.Join(base, "repo"), filepath.Join(base, "outside")
	for _, d := range []string{dir, outside} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo, dir, outside
}

// TestARepoReadsNoFileAnExtendLinksOutTo: a file of the repository that is a
// link leading out (the config, or the file a gitleaks [extend] path names,
// which the checker reads whole through this reader) is an error, and what the
// outside file holds is never returned. The checker reports any read error of
// a file that is there as the tool not checked (core/audit's tests hold that
// half). Risk: a file outside the repository judged as the config's.
func TestARepoReadsNoFileAnExtendLinksOutTo(t *testing.T) {
	t.Parallel()
	testenv.Links(t)
	for _, row := range []struct {
		name   string
		target func(outside string) string
	}{
		{"an absolute link", func(o string) string { return filepath.Join(o, "evil.toml") }},
		{"a relative link", func(string) string { return filepath.Join("..", "outside", "evil.toml") }},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			repo, dir, outside := repoOf(t)
			if err := os.WriteFile(filepath.Join(outside, "evil.toml"), []byte("[extend]\nuseDefault = true\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			testenv.Link(t, row.target(outside), filepath.Join(dir, "link.toml"))
			if b, err := repo.ReadFile("link.toml"); err == nil {
				t.Errorf("ReadFile(link.toml) = %q, want an error", b)
			}
		})
	}
}
