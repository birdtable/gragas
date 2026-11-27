package internal

// The plants of the explorer's two commit members (gragas#706): task
// explorer-format is red on a planted file Prettier would change and green on
// a formatted one, and task boards is red on a planted edit of one canvas page
// or of the seed and green on a clean copy. Each runs the pinned node of
// .tools/bin, which task tools puts there, so a run without the tools says
// so and fails.

import (
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runTask runs the Taskfile's task in the repository with the variable
// setting (NAME=value), and returns its output and whether it succeeded.
func runTask(t *testing.T, name string, settings ...string) (string, bool) {
	t.Helper()
	return runTaskFrom(t, "", name, settings...)
}

// runTaskFrom is runTask with the Taskfile at taskfile, when it is not empty:
// a copy of the repository's own in a copied root, whose tasks run in the
// copy's folder. The pinned tools stay the repository's, and the variables that
// read the repository's files with sh are given on the command line, so the
// copy needs only the files its task reads.
func runTaskFrom(t *testing.T, taskfile, name string, settings ...string) (string, bool) {
	t.Helper()
	if _, err := os.Stat(pinnedPath(t, "node")); err != nil {
		t.Fatal("the pinned node is not installed: run task tools")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(abs, ".tools", "bin")
	var gateDir []string // the copy has no ./cmd/gragas-dev for GATE's go run, so the gate is the repository's, by the environment (gragas#1655)
	cmd := exec.CommandContext(t.Context(), "task")
	cmd.Path, cmd.Err = pinnedTask(t), nil // the clone's own, set after the call: the constant above is what the rule reads
	if taskfile != "" {
		// TOOLS is read by sh in a member's command, where a backslash is an
		// escape; the real Taskfile's default is slash-separated, so this
		// override is too (gragas#1391). ToSlash is identity on Unix.
		cmd.Args = append(cmd.Args, "--taskfile", taskfile, "TOOLS="+filepath.ToSlash(bin), "SMOKE_RUN=^$", `GATE=go run -C "$GATE_DIR" -trimpath ./cmd/gragas-dev`)
		gateDir = append(gateDir, "GATE_DIR="+abs)
	}
	// The task and its setting are added after the call, as a literal of this file's.
	cmd.Args = append(cmd.Args, name)
	cmd.Args = append(cmd.Args, settings...)
	cmd.Dir = abs
	cmd.Env = append(append(configtest.Inherited(), gateDir...), "PATH="+bin+string(os.PathListSeparator)+inheritedPath())
	out, err := cmd.CombinedOutput()
	// A member's tool (prettier, gofmt, node) prints a file with the host's
	// separator, a backslash on Windows; the checks name a file with forward
	// slashes, so a member's output is read in that one form on every system
	// (gragas#1391). ToSlash is identity on Unix.
	return filepath.ToSlash(string(out)), err == nil
}

func TestTheFormatMemberIsRedOnAPlantedFileAndGreenOnAFormattedOne(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.ts")
	if err := os.WriteFile(file, []byte("const a   =  1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, ok := runTask(t, "explorer-format", "FORMAT_DIRS="+filepath.ToSlash(dir)); ok || !strings.Contains(out, "x.ts") {
		t.Errorf("a misformatted file: ok %v, output %q, want a failure naming x.ts", ok, out)
	}
	if err := os.WriteFile(file, []byte("const a = 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, ok := runTask(t, "explorer-format", "FORMAT_DIRS="+filepath.ToSlash(dir)); !ok {
		t.Errorf("a formatted file: ok %v, output %q, want a pass", ok, out)
	}
	if err := os.WriteFile(file, []byte("const a   =  1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if before, _ := os.ReadFile(file); string(before) != "const a   =  1\n" {
		t.Fatal("the plant was not kept")
	}
	runTask(t, "explorer-format", "FORMAT_DIRS="+filepath.ToSlash(dir))
	if after, _ := os.ReadFile(file); string(after) != "const a   =  1\n" {
		t.Errorf("the check rewrote the file: %q", after)
	}
}

// formatRoot is a copied root explorer-format runs in: the repository's
// Taskfile, .gitignore and pinned Prettier, ignore as its .prettierignore, and
// one unformatted file in each place the check must not read (gragas#1315):
// explorer's build output, and a generated .js and .d.ts outside it. authored
// is the authored file's text, at explorer/packages/contract/src/z.ts. It
// answers the copy's Taskfile. Prettier reads the ignore files of the folder it
// runs in alone, so a plant outside a root holding them would test nothing.
func formatRoot(t *testing.T, ignore, authored string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"Taskfile.yml", ".gitignore", "internal/journeys/goldens.txt"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		put(t, dir, name, b)
	}
	copyTree(t, "tools/.node/node_modules/prettier", dir)
	put(t, dir, ".prettierignore", []byte(ignore))
	const js, dts = "const a   =  1\n", "export declare const a :number\n"
	for rel, body := range map[string]string{
		"explorer/packages/contract/dist/x.gen.js":   js,
		"explorer/packages/contract/dist/x.gen.d.ts": dts,
		"explorer/packages/contract/dist/plain.js":   js,
		"explorer/packages/contract/src/y.gen.js":    js,
		"explorer/packages/contract/src/y.gen.d.ts":  dts,
		"explorer/packages/contract/src/z.ts":        authored,
	} {
		put(t, dir, rel, []byte(body))
	}
	return filepath.Join(dir, "Taskfile.yml")
}

// TestTheFormatMemberReadsNoBuildOutputAndNoGeneratedFile: in a copied root
// with the repository's .prettierignore, explorer-format is green over
// explorer's dist/ and generated .js and .d.ts files, and still red on an
// unformatted authored .ts beside them, naming it alone. The control is the
// same root under an empty ignore file, where every planted file is listed:
// they are files Prettier would change. Risk: a push red on an explorer
// build's output, for a reason not in its diff (gragas#1315).
func TestTheFormatMemberReadsNoBuildOutputAndNoGeneratedFile(t *testing.T) {
	t.Parallel()
	own, err := os.ReadFile(filepath.Join(root, ".prettierignore"))
	if err != nil {
		t.Fatal(err)
	}
	planted := []string{"dist/x.gen.js", "dist/x.gen.d.ts", "dist/plain.js", "src/y.gen.js", "src/y.gen.d.ts"}
	t.Run("formatted", func(t *testing.T) {
		t.Parallel()
		if out, ok := runTaskFrom(t, formatRoot(t, string(own), "const a = 1;\n"), "explorer-format", "FORMAT_DIRS=explorer"); !ok {
			t.Errorf("the build output and generated files beside a formatted file: want a pass, got:\n%s", out)
		}
	})
	t.Run("authored", func(t *testing.T) {
		t.Parallel()
		out, ok := runTaskFrom(t, formatRoot(t, string(own), "const a   =  1\n"), "explorer-format", "FORMAT_DIRS=explorer")
		if ok || !strings.Contains(out, "src/z.ts") {
			t.Errorf("an unformatted authored file: ok %v, want a failure naming src/z.ts:\n%s", ok, out)
		}
		for _, p := range planted {
			if strings.Contains(out, p) {
				t.Errorf("the check read %s, which the ignore file holds out:\n%s", p, out)
			}
		}
	})
	t.Run("control", func(t *testing.T) {
		t.Parallel()
		out, ok := runTaskFrom(t, formatRoot(t, "", "const a = 1;\n"), "explorer-format", "FORMAT_DIRS=explorer")
		if ok {
			t.Errorf("the planted files under an empty ignore file: want a failure, got a pass:\n%s", out)
		}
		for _, p := range planted {
			if !strings.Contains(out, p) {
				t.Errorf("under an empty ignore file, %s is not listed, so its plant tests nothing:\n%s", p, out)
			}
		}
	})
}

// copyTree copies the files under from, by path from the root, into the same
// paths under to.
func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(filepath.Join(root, from), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dest := filepath.Join(to, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return err
		}
		return os.WriteFile(dest, b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTheBoardsMemberIsRedOnAPlantedEditAndGreenOnACleanCopy(t *testing.T) {
	copyDir := t.TempDir()
	copyTree(t, "docs/explorer-boards", copyDir)
	copyTree(t, "explorer/core", copyDir)
	if out, ok := runTask(t, "boards", "BOARDS_ROOT="+filepath.ToSlash(copyDir)); !ok {
		t.Fatalf("a clean copy: ok %v, output %q, want a pass counting its files", ok, out)
	}
	page := filepath.Join(copyDir, "docs", "explorer-boards", "canvas", "RulesDesktop.dc.html")
	b, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, append(b, "<!-- planted -->\n"...), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, ok := runTask(t, "boards", "BOARDS_ROOT="+filepath.ToSlash(copyDir)); ok || !strings.Contains(out, "canvas/ differs") {
		t.Errorf("an edited canvas page: ok %v, output %q, want a failure saying canvas/ differs", ok, out)
	}
	if err := os.WriteFile(page, b, 0o600); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(copyDir, "docs", "explorer-boards", "canvas", "RetiredViewDesktop.dc.html")
	if err := os.WriteFile(stray, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, ok := runTask(t, "boards", "BOARDS_ROOT="+filepath.ToSlash(copyDir)); ok || !strings.Contains(out, "Only in") || !strings.Contains(out, "RetiredViewDesktop.dc.html") {
		t.Errorf("a page no script writes: ok %v, output %q, want a failure naming the stray page", ok, out)
	}
	if err := os.Remove(stray); err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(copyDir, "explorer", "core", "parts.seed.yaml")
	s, err := os.ReadFile(seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seed, append(s, "# planted\n"...), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, ok := runTask(t, "boards", "BOARDS_ROOT="+filepath.ToSlash(copyDir)); ok || !strings.Contains(out, "seed differs") {
		t.Errorf("an edited seed: ok %v, output %q, want a failure saying the seed differs", ok, out)
	}
}

// inheritedPath is the PATH the tests were started with, read from the inherited environment.
func inheritedPath() string {
	for _, kv := range configtest.Inherited() {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			return v
		}
	}
	return ""
}
