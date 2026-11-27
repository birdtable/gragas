package journeys

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-files/filestest"
)

// builtFile is where TestMain records what it built the binary from, in the
// folder the binary is in (tests.journeys.3).
const builtFile = "gragas.built.json"

// builtRecord is the tree and the toolchain a binary came from: the digest of the
// program's source files, and the Go version that built it. A journey on a
// binary whose record is not the tree it is run against fails, so a golden is
// never held against a binary from other code.
type builtRecord struct {
	Tree      string `json:"tree"`
	Toolchain string `json:"toolchain"`
}

// currentRecord is the record of the tree under root as it is now, with the
// toolchain this suite was compiled by.
func currentRecord(root string) (builtRecord, error) {
	tree, err := sourceTree(root)
	if err != nil {
		return builtRecord{}, err
	}
	return builtRecord{Tree: tree, Toolchain: runtime.Version()}, nil
}

// binaryRecord is the record of the binary at bin: the digest of the tree it was
// built from, which TestMain wrote beside it, and the toolchain the binary itself
// carries in its build information.
func binaryRecord(bin string) (builtRecord, error) {
	b, err := os.ReadFile(filepath.Join(filepath.Dir(bin), builtFile))
	if err != nil {
		return builtRecord{}, fmt.Errorf("the binary has no record of what it was built from: %w", err)
	}
	var rec builtRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return builtRecord{}, fmt.Errorf("the build record is not read: %w", err)
	}
	info, err := buildinfo.ReadFile(bin)
	if err != nil {
		return builtRecord{}, fmt.Errorf("the binary carries no build information: %w", err)
	}
	rec.Toolchain = info.GoVersion
	return rec, nil
}

// sourceTree is the digest of what the binary is built from, the files of
// treeFiles. Each path, in sorted order, is joined with its length and bytes,
// so the digest moves with a file's name and content and with nothing else.
// The files are read here, inside m.Run, so they are this package's test-cache
// key too.
func sourceTree(root string) (string, error) {
	files, err := treeFiles(root)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// treeFiles is every file under root a build of the program can read, by
// slash path in sorted order:
//   - the module files, those that exist;
//   - every Go file under cmd and internal that is not a test, outside
//     testdata;
//   - every file but a test under the folder of a package whose Go files hold a
//     //go:embed line, testdata included, since an embed pattern may name
//     anything below its package.
//
// The embedded files are read from the folders rather than from go list, which
// would cost a go command on every pty start that checks freshness. Taking
// the whole folder may count a file nothing embeds; that only rebuilds more
// often. TestTheTreeHoldsEveryFileTheProgramEmbeds holds this against go list.
func treeFiles(root string) ([]string, error) {
	var files, embedding []string
	for _, f := range []string{"go.mod", "go.sum", "go.work", "go.work.sum"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			files = append(files, f)
		}
	}
	isTest := func(p string) bool { return strings.HasSuffix(p, "_test.go") }
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && p == filepath.Join(root, top) {
				return filepath.SkipDir // a tree without this folder has no files in it
			}
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || isTest(p) {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if embeds(b) {
				embedding = append(embedding, filepath.Dir(p))
			}
			return addRel(&files, root, p)
		})
		if err != nil {
			return nil, err
		}
	}
	for _, dir := range embedding {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || isTest(p) {
				return err
			}
			return addRel(&files, root, p)
		})
		if err != nil {
			return nil, err
		}
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}

// embeds is whether a Go file holds a //go:embed directive: a line that starts
// with it, as the compiler reads one.
func embeds(src []byte) bool {
	for line := range bytes.Lines(src) {
		if bytes.HasPrefix(line, []byte("//go:embed")) {
			return true
		}
	}
	return false
}

// addRel adds the slash path of p from root to files.
func addRel(files *[]string, root, p string) error {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return err
	}
	*files = append(*files, filepath.ToSlash(rel))
	return nil
}

// recordBuild writes the digest of the tree the binary in dir was just built
// from.
func recordBuild(dir string) error {
	tree, err := sourceTree(moduleRoot())
	if err != nil {
		return err
	}
	b, err := json.Marshal(builtRecord{Tree: tree})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, builtFile), append(b, '\n'), 0o644)
}

// staleness is why the binary recorded as built is not the one for the tree as
// it is now, or "" when it is.
func staleness(built, now builtRecord) string {
	switch {
	case built.Tree != now.Tree:
		return "the binary was built from another tree (" + short(built.Tree) + ", now " + short(now.Tree) + ")"
	case built.Toolchain != now.Toolchain:
		return "the binary was built with " + built.Toolchain + ", the toolchain is now " + now.Toolchain
	}
	return ""
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// freshBinary is why the binary the suite runs is not the tree it is run
// against, as an error, or nil when it is.
func freshBinary(t *testing.T) error {
	t.Helper()
	return freshAt(gragasBin(t))
}

// freshAt is freshBinary for the binary at bin, so a test can judge a plant
// without changing the binary the other tests run.
func freshAt(bin string) error {
	built, err := binaryRecord(bin)
	if err != nil {
		return err
	}
	now, err := currentRecord(moduleRoot())
	if err != nil {
		return err
	}
	if why := staleness(built, now); why != "" {
		return errors.New("a journey on a stale binary: " + why)
	}
	return nil
}

// TestTheBuildRecordNamesTheTreeAndTheToolchain: the binary the suite built has
// a record of the tree and the toolchain it came from, and the record is the
// tree as it is now. Risk: a golden held against a binary from other code
// (tests.journeys.3).
func TestTheBuildRecordNamesTheTreeAndTheToolchain(t *testing.T) {
	t.Parallel()
	built, err := binaryRecord(gragasBin(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Tree) != 64 || !strings.HasPrefix(built.Toolchain, "go") {
		t.Fatalf("the record names no tree or no toolchain: %+v", built)
	}
	now, err := currentRecord(moduleRoot())
	if err != nil {
		t.Fatal(err)
	}
	if why := staleness(built, now); why != "" {
		t.Errorf("control: the binary just built should be fresh: %s", why)
	}
}

// TestAJourneyOnAStaleBinaryFails: a record of another tree, or of another
// toolchain, is stale and a record of the tree as it is is not. The plant is a
// record with one digit changed; the control is the real one.
func TestAJourneyOnAStaleBinaryFails(t *testing.T) {
	t.Parallel()
	now, err := currentRecord(moduleRoot())
	if err != nil {
		t.Fatal(err)
	}
	if why := staleness(now, now); why != "" {
		t.Errorf("control: the same record is stale: %s", why)
	}
	other := now
	other.Tree = "0" + now.Tree[1:]
	if now.Tree[0] == '0' {
		other.Tree = "1" + now.Tree[1:]
	}
	if staleness(other, now) == "" {
		t.Error("a record of another tree passed")
	}
	other = now
	other.Toolchain = "go0.0.0"
	if staleness(other, now) == "" {
		t.Error("a record of another toolchain passed")
	}
	// The tree moves with a file's content: one source file, one byte.
	dir := t.TempDir()
	copyFile(t, filepath.Join(moduleRoot(), "go.mod"), filepath.Join(dir, "go.mod"))
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "cmd", "x", "main.go")
	if err := os.WriteFile(src, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := sourceTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("package main \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := sourceTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Error("a changed byte left the tree digest as it was")
	}
}

// TestAnEmbeddedFileMovesTheTree: in a module of the test's own, a package
// embeds a file beside it and a folder below it. A changed byte in either moves
// the tree's digest (the plants), and a changed test file of that package, or
// a text file of a package that embeds nothing, leaves it where it was (the
// controls). Risk: journeys replayed, or run on a binary built before the
// change, after an edit to an embedded asset alone.
func TestAnEmbeddedFileMovesTheTree(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		file  string
		moves bool
	}{
		"a file the package embeds":                   {"internal/x/a.txt", true},
		"a file in a folder the package embeds":       {"internal/x/sub/b.yaml", true},
		"a test of the embedding package":             {"internal/x/x_test.go", false},
		"a file beside a package that embeds nothing": {"internal/y/notes.txt", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for rel, body := range map[string]string{
				"go.mod":                "module example.com/m\n\ngo 1.24\n",
				"internal/x/x.go":       "package x\n\nimport \"embed\"\n\n//go:embed a.txt all:sub\nvar files embed.FS\n",
				"internal/x/a.txt":      "a\n",
				"internal/x/sub/b.yaml": "b: 1\n",
				"internal/x/x_test.go":  "package x\n",
				"internal/y/y.go":       "package y\n",
				"internal/y/notes.txt":  "notes\n",
			} {
				p := filepath.Join(dir, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before, err := sourceTree(dir)
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(dir, filepath.FromSlash(c.file))
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, append(b, ' '), 0o644); err != nil {
				t.Fatal(err)
			}
			after, err := sourceTree(dir)
			if err != nil {
				t.Fatal(err)
			}
			if moved := before != after; moved != c.moves {
				t.Errorf("a changed byte in %s: the tree moved %v, want %v", c.file, moved, c.moves)
			}
		})
	}
}

// TestTheTreeHoldsEveryFileTheProgramEmbeds: every file go list says a package
// of this module that a command links embeds is one treeFiles names, so the
// walk's reading of //go:embed lines misses none of the compiler's. Risk: an
// embed the walk does not see, as in a file whose directive it misreads.
func TestTheTreeHoldsEveryFileTheProgramEmbeds(t *testing.T) {
	t.Parallel()
	root := moduleRoot()
	tmpl := `{{if .Module}}{{if .Module.Main}}{{$d := .Dir}}{{range .EmbedFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}{{end}}{{end}}`
	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", "-f", tmpl, "./cmd/...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	files, err := treeFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for line := range strings.Lines(string(out)) {
		rel, err := filepath.Rel(root, strings.TrimSpace(line))
		if err != nil {
			t.Fatal(err)
		}
		n++
		if _, found := slices.BinarySearch(files, filepath.ToSlash(rel)); !found {
			t.Errorf("%s is embedded in the program but not in the tree's digest", filepath.ToSlash(rel))
		}
	}
	if n < 100 {
		t.Errorf("go list named %d embedded files, and the program embeds well over a hundred", n)
	}
}

// plantStale is the path of a copy of the binary whose record names another
// tree: the real record with one digit changed. It sets nothing, so tests that
// plant run together.
func plantStale(t *testing.T) string {
	t.Helper()
	real := gragasBin(t)
	built, err := binaryRecord(real)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	plant := filepath.Join(dir, filepath.Base(real))
	filestest.WriteProgram(t, plant, data)
	tree := "0" + built.Tree[1:]
	if built.Tree[0] == '0' {
		tree = "1" + built.Tree[1:]
	}
	b, err := json.Marshal(builtRecord{Tree: tree})
	if err != nil {
		t.Fatal(err)
	}
	filestest.WriteFile(t, filepath.Join(dir, builtFile), b)
	return plant
}

// TestTheFreshnessCheckFailsOnAStaleRecord: the check is clean on the binary
// the suite built (the control) and fails, naming a stale binary and another
// tree, on a copy whose record has one digit changed (the plant). This holds
// the check itself; that the pty launcher is wired to it, and calls it before
// it starts anything, is TestThePtyLauncherChecksFreshnessBeforeItStarts
// (tests.journeys.3).
func TestTheFreshnessCheckFailsOnAStaleRecord(t *testing.T) {
	t.Parallel()
	if err := freshBinary(t); err != nil {
		t.Fatalf("control: the binary just built failed the check: %v", err)
	}
	plant := plantStale(t)
	err := freshAt(plant)
	if err == nil || !strings.Contains(err.Error(), "stale binary") || !strings.Contains(err.Error(), "another tree") {
		t.Errorf("a binary with a record of another tree was told %v, want a stale-binary failure", err)
	}
}
