package fs_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
)

func TestAGeneratedFileIsWrittenWholeWithItsFolder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := (hostfs.LegacyWriter{}).Write(dir, "rulebook/base/x/README.md", []byte("# x\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p := filepath.Join(dir, "rulebook", "base", "x", "README.md")
	got, err := os.ReadFile(p)
	if err != nil || string(got) != "# x\n" {
		t.Fatalf("read back %q, %v", got, err)
	}
	trusttest.Perm(t, p, 0o644)
	entries, err := os.ReadDir(filepath.Dir(p))
	if err != nil || len(entries) != 1 {
		t.Errorf("folder holds %d entries (%v), want only the file: nothing left aside", len(entries), err)
	}
}

func TestAPathOutOfTheRepositoryIsRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, p := range []string{"../escape", "/repo/x", ""} {
		if err := (hostfs.LegacyWriter{}).Write(dir, p, []byte("x")); err == nil {
			t.Errorf("Write(%q): want a refusal", p)
		}
	}
}

// kitWrite is the shared kit's write helper, which only the old gen's
// writer may import.
const kitWrite = "github.com/birdtable/go-files/write"

// importers are the Go files below dir that import pkg, skipping the
// folders git and the build keep.
func importers(t *testing.T, dir, pkg string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && slices.Contains([]string{".git", ".tools", "dist", "node_modules", "testdata"}, d.Name()) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, "`\"") == pkg {
				rel, err := filepath.Rel(dir, p)
				if err != nil {
					return err
				}
				found = append(found, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// TestOnlyTheTwoWritersImportTheKitsWriteHelper (arch.layers.2): the lint
// lets all of host/fs import go-files/write, so this holds it to the two files
// that write through it: writer.go, whose Writer.Replace holds the root
// around the call, and legacywrite.go, which goes at M6. Outside host/fs the
// block's standing second owners import it too, the six files that
// replace a file whole in the commands (gragas-new's check among them), and the two that write a
// program through Runnable; one more is a copy. The check finds
// a planted second use, aliased, and passes a file that only names the path.
func TestOnlyTheTwoWritersImportTheKitsWriteHelper(t *testing.T) {
	t.Parallel()
	module, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(module, "go.mod")); err != nil {
		t.Fatalf("the module root is not at %s: %v", module, err)
	}
	want := []string{
		"cmd/gragas-dev/main.go",
		"cmd/gragas-generate/main.go",
		"cmd/gragas-new/check.go",
		"cmd/gragas-release/verify.go",
		"internal/host/fs/legacywrite.go",
		"internal/host/fs/writer.go",
	}
	if got := importers(t, module, kitWrite); !slices.Equal(got, want) {
		t.Errorf("%s is imported by %v, want %v", kitWrite, got, want)
	}
	t.Run("a second use is found", func(t *testing.T) {
		plant := t.TempDir()
		write(t, plant, map[string]string{
			"a.go":       "package a\n\nimport \"" + kitWrite + "\"\n",
			"b/c.go":     "package c\n\nimport (\n\t\"os\"\n\tkit \"" + kitWrite + "\"\n)\n",
			"d.go":       "package d\n\n// " + kitWrite + " is named, not imported.\n",
			"e/write.go": "package e\n\nimport \"" + kitWrite + "/other\"\n",
		})
		if got := importers(t, plant, kitWrite); !slices.Equal(got, []string{"a.go", "b/c.go"}) {
			t.Errorf("in the planted module %s is imported by %v, want a.go and b/c.go", kitWrite, got)
		}
	})
}
