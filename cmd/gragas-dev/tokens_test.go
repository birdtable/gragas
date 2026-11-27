package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/dev/tokens"
)

// tokensTree is a folder with the token document alone, copied from the
// repository, so a run writes the three files into it and nowhere else.
func tokensTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", tokens.File))
	if err != nil {
		t.Fatal(err)
	}
	// The folders of the four files exist, as they do in the repository; the
	// generator replaces a file, it makes no folder.
	for _, f := range []string{tokens.File, tokens.CSSFile, tokens.ThemeFile, tokens.PaletteFile} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(f)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, tokens.File), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runTokensIn(dir string, args ...string) (code int, out, errs string) {
	var o, e bytes.Buffer
	code = runTokens(append(args, "-C", dir), &o, &e)
	return code, o.String(), e.String()
}

// TestTokensWritesTheThreeFilesAndCheckFindsThemCurrent: a run writes the CSS,
// the theme and the palette, a second run writes none, and --check ends on its
// count line of the three (supply.provenance.4, gragas#695).
func TestTokensWritesTheThreeFilesAndCheckFindsThemCurrent(t *testing.T) {
	t.Parallel()
	dir := tokensTree(t)
	code, out, errs := runTokensIn(dir)
	if code != exitDone || strings.Count(out, "wrote ") != 3 {
		t.Fatalf("first run = %d, %q, %q, want three files written", code, out, errs)
	}
	if code, out, _ := runTokensIn(dir); code != exitDone || strings.Contains(out, "wrote") {
		t.Errorf("second run = %d, %q, want nothing written", code, out)
	}
	code, out, errs = runTokensIn(dir, "--check")
	if code != exitDone || !strings.Contains(out, "count: 3 files") {
		t.Errorf("--check = %d, %q, %q, want current and a count of three files", code, out, errs)
	}
}

// TestAHandEditOfAGeneratedFileFailsTheCheck: each of the three files, edited
// by hand or deleted, is named stale and the check writes nothing back (the
// guard of supply.provenance.4 and gates.own-rules.3).
func TestAHandEditOfAGeneratedFileFailsTheCheck(t *testing.T) {
	t.Parallel()
	for _, file := range []string{tokens.CSSFile, tokens.ThemeFile, tokens.PaletteFile} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			dir := tokensTree(t)
			if code, _, errs := runTokensIn(dir); code != exitDone {
				t.Fatal(errs)
			}
			full := filepath.Join(dir, file)
			good, err := os.ReadFile(full)
			if err != nil {
				t.Fatal(err)
			}
			edited := append(append([]byte{}, good...), []byte(" \n")...)
			if err := os.WriteFile(full, edited, 0o644); err != nil {
				t.Fatal(err)
			}
			code, out, errs := runTokensIn(dir, "--check")
			if code != exitFailed || !strings.Contains(out, "stale "+file) || !strings.Contains(errs, "stale") {
				t.Errorf("--check = %d, %q, %q, want stale %s", code, out, errs, file)
			}
			if now, _ := os.ReadFile(full); !bytes.Equal(now, edited) {
				t.Error("--check wrote the file back")
			}
		})
	}
}

// TestADocumentWithAPairBelowItsFloorWritesNothing: the generator refuses it,
// names the pair, and leaves no generated file for a build to read.
func TestADocumentWithAPairBelowItsFloorWritesNothing(t *testing.T) {
	t.Parallel()
	dir := tokensTree(t)
	full := filepath.Join(dir, tokens.File)
	doc, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	lowered := regexp.MustCompile(`("muted": \{\s*"light": ")#5a5850`).ReplaceAll(doc, []byte("${1}#c0c0c0"))
	if bytes.Equal(lowered, doc) {
		t.Fatal("the plant found nothing to lower")
	}
	if err := os.WriteFile(full, lowered, 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runTokensIn(dir)
	if code != exitFailed || !strings.Contains(out, "low text.muted on page in light") || !strings.Contains(errs, "below their floor") {
		t.Errorf("run = %d, %q, %q, want the pair named and a refusal", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, tokens.CSSFile)); err == nil {
		t.Error("a file was written from a document below its floor")
	}
}
