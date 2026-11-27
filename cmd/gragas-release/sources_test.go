package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/exit"
)

// repoRoot is this repository, from the package's folder.
const repoRoot = "../.."

// plainTree is git ls-tree -r -z of a tree whose three sources are plain files,
// with a tracked link beside them that no source names.
func plainTree() string {
	return "100644 blob d645695673349e3947e8e5ae42332d0ac3164cd7\tLICENSE\x00" +
		"100644 blob fa279717ba7a372d70102a50b9a7caf4fc4ab777\tNOTICE\x00" +
		"100644 blob 7c10d2041980361eb452e09b3c3d021199ed143e\tnotices/gragas.txt\x00" +
		"120000 blob 47dc3e3d863cfb5727b87d785d09abf9743c0a72\tCLAUDE.md\x00"
}

// realConfig is this repository's .goreleaser.yaml.
func realConfig(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, configName))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// runSources runs the sources step over a clone that holds config, with the
// fake's tree.
func runSources(t *testing.T, config, tree string) (int, string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configName), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	code := run(t.Context(), []string{"sources", "-C", dir}, &out, &errs, &fakeChain{tree: tree})
	return code, out.String(), errs.String()
}

// The list the step reads from the config is the list verify holds the
// archives to carry, so a source added to one is held by the other.
func TestTheSourcesAreTheFilesVerifyChecks(t *testing.T) {
	t.Parallel()
	got, err := releaseSources([]byte(realConfig(t)))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, carried()) {
		t.Errorf(".goreleaser.yaml's sources are %v; verify checks %v", got, carried())
	}
}

// The real tree, listed by the real git with no variable of the environment,
// holds every source as a plain file: the tree tracks no link, so the plants
// below carry a link of their own (plainTree's CLAUDE.md).
func TestTheRealTreeHoldsEachSourceAPlainFile(t *testing.T) {
	t.Parallel()
	listing, err := newToolchain(func(string) string { return "" }, nil).Tree(t.Context(), repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	modes, err := treeModes(listing)
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range carried() {
		if modes[src] != plainMode {
			t.Errorf("%s is mode %q in HEAD, want %s", src, modes[src], plainMode)
		}
	}
}

func TestPlainSourcesPass(t *testing.T) {
	t.Parallel()
	code, out, errs := runSources(t, realConfig(t), plainTree())
	if code != exit.Done || errs != "" || out != "sources: the 3 sources of .goreleaser.yaml are plain files in HEAD: LICENSE, NOTICE, notices/gragas.txt\n" {
		t.Errorf("sources = %d, %q, %q", code, out, errs)
	}
}

// Each plant is red, and the error names the source and why.
func TestASourceThatIsNotAPlainFileIsRefusedByName(t *testing.T) {
	t.Parallel()
	real := realConfig(t)
	swap := func(from, to string) string {
		if !strings.Contains(real, from) {
			t.Fatalf(".goreleaser.yaml has no %q", from)
		}
		return strings.Replace(real, from, to, 1)
	}
	link := strings.Replace(plainTree(), "100644 blob d645", "120000 blob d645", 1)
	for name, c := range map[string]struct{ config, tree, want string }{
		"a source that is a link": {real, link,
			"LICENSE: a source of .goreleaser.yaml that is a symbolic link in HEAD; goreleaser would ship what it points to"},
		"a source that is executable": {real, strings.Replace(plainTree(), "100644 blob fa27", "100755 blob fa27", 1),
			"NOTICE: a source of .goreleaser.yaml that is mode 100755 in HEAD, not a plain file (100644)"},
		"a source that is not tracked": {real, strings.Replace(plainTree(), "\tnotices/gragas.txt", "\tnotices/other.txt", 1),
			"notices/gragas.txt: a source of .goreleaser.yaml that is not a file tracked in HEAD"},
		"the tracked link named as a source": {swap("      - src: NOTICE\n", "      - src: NOTICE\n        info: *file\n      - src: CLAUDE.md\n"), plainTree(),
			"CLAUDE.md: a source of .goreleaser.yaml that is a symbolic link in HEAD"},
		"a link among a checksum's extra files": {swap("checksum:\n", "checksum:\n  extra_files:\n    - glob: x\n      src: CLAUDE.md\n"), plainTree(),
			"CLAUDE.md: a source of .goreleaser.yaml that is a symbolic link in HEAD"},
		"a link among a package's contents": {real + "nfpms:\n  - contents:\n      - src: CLAUDE.md\n        dst: /work/doc/x\n", plainTree(),
			"CLAUDE.md: a source of .goreleaser.yaml that is a symbolic link in HEAD"},
		"a source written as a bare path": {swap("      - src: NOTICE\n        info: *file\n", "      - CLAUDE.md\n"), plainTree(),
			"CLAUDE.md: a source of .goreleaser.yaml that is a symbolic link in HEAD"},
		"a pattern": {swap("src: NOTICE", "src: NOT*"), plainTree(),
			`.goreleaser.yaml: line 54: files: "NOT*" is not a path to one file: a pattern or a template`},
		"a template": {swap("src: NOTICE", `src: "{{ .Env.NOTICE }}"`), plainTree(),
			"is not a path to one file: a pattern or a template"},
		"a mapping with no src": {swap("      - src: NOTICE\n", "      - dst: NOTICE\n"), plainTree(),
			"files: item 2 has no src that is a path"},
		"an item that is not a path": {swap("      - src: NOTICE\n        info: *file\n", "      - 7\n"), plainTree(),
			"files: item 2 is neither a path nor a mapping with src"},
		"files that is not a list": {swap("    files:\n      - src: LICENSE\n", "    files: LICENSE\n    other:\n      - src: LICENSE\n"), plainTree(),
			"files: is not a list"},
		"no source at all": {swap("    files:\n", "    kept:\n"), plainTree(),
			".goreleaser.yaml names no source under files, extra_files, templated_files, templated_extra_files, contents"},
		"a listing that is not git's": {real, "100644 blob LICENSE\x00", "is not a mode, a type, an object and a path"},
	} {
		code, out, errs := runSources(t, c.config, c.tree)
		if code != exit.Failed || out != "" || !strings.HasPrefix(errs, "gragas-release sources: ") || !strings.Contains(errs, c.want) {
			t.Errorf("%s: sources = %d, %q, %q; want it red with %q", name, code, out, errs, c.want)
		}
	}
}

func TestTheSourcesStepNeedsItsConfig(t *testing.T) {
	t.Parallel()
	var out, errs bytes.Buffer
	if code := run(t.Context(), []string{"sources", "-C", t.TempDir()}, &out, &errs, goodChain()); code != exit.Failed || !strings.Contains(errs.String(), configName) {
		t.Errorf("sources of a clone with no config = %d, %q", code, errs.String())
	}
}
