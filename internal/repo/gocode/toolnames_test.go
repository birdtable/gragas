package gocode

import (
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// The four tools M3 brings adapters for, as a person writes them. Each is a
// document under adapters/, so neither the enforcement link's reader nor the
// command tree names one, in any file: code, test, comment or test data.
func adapterTools() []string { return []string{"golangci-lint", "ESLint", "tsc", "ast-grep"} }

// noToolFolders are the folders the four names stay out of, from the root.
func noToolFolders() []string { return []string{"internal/core/link", "internal/cli"} }

// toolMentions is every line of every file below roots in fsys that names one
// of tools as a word of its own, in any case, or names the TypeScript
// compiler.
func toolMentions(fsys fs.FS, roots, tools []string) ([]string, error) {
	words := make([]*regexp.Regexp, 0, len(tools)+1)
	for _, tool := range tools {
		words = append(words, regexp.MustCompile(`(?i)(?:^|[^a-z0-9_-])(`+regexp.QuoteMeta(tool)+`)(?:$|[^a-z0-9_-])`))
	}
	words = append(words, regexp.MustCompile(`(?i)(typescript\s+compiler)`))
	var out []string
	for _, r := range roots {
		err := fs.WalkDir(fsys, r, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(data), "\n") {
				for _, w := range words {
					if m := w.FindStringSubmatch(line); m != nil {
						out = append(out, p+":"+strconv.Itoa(i+1)+": names "+m[1])
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// TestNoAdapterToolIsNamedInTheLinkOrTheCommandTree is that check
// over every file of the two folders, and the four names are
// in the source table's tools, so sourceWords refuses them in code too.
func TestNoAdapterToolIsNamedInTheLinkOrTheCommandTree(t *testing.T) {
	t.Parallel()
	found, err := toolMentions(os.DirFS(root), noToolFolders(), adapterTools())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range found {
		t.Error(f)
	}
	tb := table(t)
	for _, tool := range adapterTools() {
		if !slices.Contains(tb.Source.Tools, strings.ToLower(tool)) {
			t.Errorf("source.tools in layers.yaml lacks %s", strings.ToLower(tool))
		}
	}
}

// TestAToolNamedAnywhereBelowTheFoldersFails is its plant, with the clean
// control: a name in a comment, a test, test data or a nested folder fails;
// one in another folder, or as part of a longer word, passes.
func TestAToolNamedAnywhereBelowTheFoldersFails(t *testing.T) {
	t.Parallel()
	planted := fstest.MapFS{
		"internal/core/link/a.go":            {Data: []byte("package link\n\n// runs ast-grep\n")},
		"internal/core/link/a_test.go":       {Data: []byte("package link\n\nconst x = \"ESLINT\"\n")},
		"internal/core/link/testdata/x.yaml": {Data: []byte("tool: tsc\n")},
		"internal/cli/root/deep/b.md":        {Data: []byte("see golangci-lint\n")},
		"internal/cli/c.txt":                 {Data: []byte("the TypeScript\ncompiler\n\nthe TypeScript compiler\n")},
		"internal/host/tool/ok.go":           {Data: []byte("package tool\n\nconst x = \"tsc\"\n")},
		"internal/core/link/clean.go":        {Data: []byte("package link\n\n// tscx, golangci-linter, eslintrc-ish, mast-grep\n")},
	}
	found, err := toolMentions(planted, noToolFolders(), adapterTools())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"internal/cli/c.txt:4: names TypeScript compiler",
		"internal/cli/root/deep/b.md:1: names golangci-lint",
		"internal/core/link/a.go:3: names ast-grep",
		"internal/core/link/a_test.go:3: names ESLINT",
		"internal/core/link/testdata/x.yaml:1: names tsc",
	}
	slices.Sort(found)
	if !slices.Equal(found, want) {
		t.Errorf("found:\n%s\nwant:\n%s", strings.Join(found, "\n"), strings.Join(want, "\n"))
	}
}
