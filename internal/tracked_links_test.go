package internal

// No tracked file is a link (gragas#872). Git for Windows checks a tracked
// symbolic link out as a text file holding its target, so a CLAUDE.md that is
// a link reads there as the word "AGENTS.md" and an agent finds no
// instructions. The tree holds regular files only; a file that leads to
// another says so in its text.

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// linkMode is the mode git records for a symbolic link.
const linkMode = "120000"

// trackedLinks lists every path of a `git ls-files -s -z` listing whose mode
// is a link. An entry it cannot read is listed as well: the rule only narrows.
func trackedLinks(listing string) []string {
	var out []string
	for entry := range strings.SplitSeq(listing, "\x00") {
		if entry == "" {
			continue
		}
		meta, name, ok := strings.Cut(entry, "\t")
		mode, _, _ := strings.Cut(meta, " ")
		switch {
		case !ok:
			out = append(out, entry+": cannot be read as mode, object, stage and path")
		case mode == linkMode:
			out = append(out, name+": is tracked as a link (mode "+linkMode+"); Git for Windows checks it out as a text file holding its target")
		}
	}
	return out
}

// linksIn is trackedLinks for the tree at dir. A tree with no .git, an
// export, holds only tracked files, so there every link on disk is one, but
// for a node_modules an install wrote, which the walk skips.
func linksIn(t *testing.T, dir string) []string {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
		cmd := exec.CommandContext(t.Context(), "git", "ls-files", "-s", "-z")
		cmd.Dir = dir
		cmd.Env = append(configtest.Inherited(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("list the files git tracks in %s with their modes: %v", dir, err)
		}
		return trackedLinks(string(out))
	}
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "node_modules" {
			return fs.SkipDir // installed, not exported, and Go's test cache must not read it
		}
		if d.Type()&fs.ModeSymlink != 0 {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel)+": is a link in a tree with no .git")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNoTrackedFileIsALink(t *testing.T) {
	t.Parallel()
	for _, p := range linksIn(t, root) {
		t.Error(p)
	}
}

func TestAPlantedTrackedLinkFailsByName(t *testing.T) {
	t.Parallel()
	const sum = "47dc3e3d863cfb5727b87d785d09abf9743c0a72"
	tests := []struct {
		name, listing string
		want          []string
	}{
		{"control", "100644 " + sum + " 0\tAGENTS.md\x00100755 " + sum + " 0\tscripts/release-guard\x00", nil},
		{"a link", "100644 " + sum + " 0\tAGENTS.md\x00" + linkMode + " " + sum + " 0\tCLAUDE.md\x00", []string{"CLAUDE.md: is tracked as a link"}},
		{"a link in a folder", linkMode + " " + sum + " 0\tdocs/a b.md\x00", []string{"docs/a b.md: is tracked as a link"}},
		{"not an entry", "100644 " + sum + " 0 AGENTS.md\x00", []string{"100644 " + sum + " 0 AGENTS.md: cannot be read"}},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := trackedLinks(tt.listing)
			if len(got) != len(tt.want) {
				t.Fatalf("trackedLinks = %q, want %d findings", got, len(tt.want))
			}
			for i, w := range tt.want {
				if !strings.HasPrefix(got[i], w) {
					t.Errorf("finding %d = %q, want it to start %q", i, got[i], w)
				}
			}
			if slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "AGENTS.md: is tracked") }) {
				t.Errorf("a regular file was named: %q", got)
			}
		})
	}
}

// TestAPlantedLinkInATreeWithNoGitFailsByName runs linksIn's walk, the branch
// an export takes: each link on disk, to a file or to a folder, is named by
// its slash path, and a tree of regular files names nothing.
func TestAPlantedLinkInATreeWithNoGitFailsByName(t *testing.T) {
	t.Parallel()
	tree := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		for _, rel := range []string{"AGENTS.md", filepath.Join("docs", "a.md")} {
			p := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("text\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	t.Run("control", func(t *testing.T) {
		t.Parallel()
		if got := linksIn(t, tree(t)); got != nil {
			t.Errorf("linksIn = %q on a tree of regular files, want nothing", got)
		}
	})
	t.Run("a link to a file in a folder and a link to a folder", func(t *testing.T) {
		t.Parallel()
		dir := tree(t)
		testenv.Link(t, filepath.Join("..", "AGENTS.md"), filepath.Join(dir, "docs", "CLAUDE.md"))
		testenv.Link(t, "docs", filepath.Join(dir, "linked"))
		want := []string{
			"docs/CLAUDE.md: is a link in a tree with no .git",
			"linked: is a link in a tree with no .git",
		}
		if got := linksIn(t, dir); !slices.Equal(got, want) {
			t.Errorf("linksIn = %q, want %q", got, want)
		}
	})
}
