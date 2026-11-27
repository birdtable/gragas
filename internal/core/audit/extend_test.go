package audit_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/birdtable/gragas/internal/core/audit"
)

func gitleaksBindings() []audit.Binding {
	return []audit.Binding{{"supply.secrets.1", "gitleaks", "extend.useDefault"}, {"supply.secrets.4", "gitleaks", "allowlist.paths"}}
}

// TestAConfigThatExtendsAFileIsNotCheckedAndNeverPassed: gitleaks 8.30.1 reads
// [extend] path (config.go: Translate, extendPath) as a file whose allowlists
// are appended to the config's own, so a config that names one is reported as
// not checked, naming the file and the key, and no binding of the tool is
// judged. Risk: a config that hides an allowlist in an extended file and
// passes as clean.
func TestAConfigThatExtendsAFileIsNotCheckedAndNeverPassed(t *testing.T) {
	t.Parallel()
	doc := map[string]any{"extend": map[string]any{"path": "base.toml"}}
	got := audit.Check(t.Context(), audit.Input{
		Sources: sources(), Repo: fstest.MapFS{".gitleaks.toml": {Data: []byte("x")}},
		Bindings: gitleaksBindings(), TOML: toml(doc),
	})
	if len(got.Findings) != 0 || len(got.NotChecked) != 1 {
		t.Fatalf("findings %v, not checked %v; want one not checked and no finding", got.Findings, got.NotChecked)
	}
	for _, want := range []string{".gitleaks.toml", "[extend] path", "base.toml"} {
		if !strings.Contains(got.NotChecked[0].Why, want) {
			t.Errorf("the reason %q does not name %q", got.NotChecked[0].Why, want)
		}
	}
}

// byContent is a stand-in reader that answers the document its test keyed to
// the file's bytes, so each file of a chain reads as its own.
func byContent(docs map[string]map[string]any) audit.TOML {
	return func(b []byte) (map[string]any, error) { return docs[string(b)], nil }
}

func ext(path string) map[string]any { return map[string]any{"extend": map[string]any{"path": path}} }

func chainRepo(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for name, content := range files {
		m[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

func runChain(t *testing.T, files map[string]string, docs map[string]map[string]any) audit.Result {
	t.Helper()
	return audit.Check(t.Context(), audit.Input{
		Sources: sources(), Repo: chainRepo(files), Bindings: gitleaksBindings(), TOML: byContent(docs),
	})
}

// TestAnExtendedFileIsReadAndJudgedWithTheConfig: gitleaks 8.30.1 (config.go:
// Translate, extend) appends the allowlists of an extended file to the
// config's own and reads [extend] of that file too, so the file's useDefault
// counts and its path allowlist is the config's. Risk: a path allowlist hidden
// in the extended file passes.
func TestAnExtendedFileIsReadAndJudgedWithTheConfig(t *testing.T) {
	t.Parallel()
	files := map[string]string{".gitleaks.toml": "top", "base.toml": "base"}
	top := ext("base.toml")
	t.Run("useDefault in the extended file counts", func(t *testing.T) {
		t.Parallel()
		got := runChain(t, files, map[string]map[string]any{"top": top, "base": {"extend": map[string]any{"useDefault": true}}})
		if len(got.Findings) != 0 || len(got.NotChecked) != 0 {
			t.Errorf("findings %v, not checked %v", got.Findings, got.NotChecked)
		}
	})
	t.Run("a path allowlist in the extended file is found", func(t *testing.T) {
		t.Parallel()
		base := map[string]any{"extend": map[string]any{"useDefault": true}, "allowlist": map[string]any{"paths": []any{"vendor/"}}}
		got := runChain(t, files, map[string]map[string]any{"top": top, "base": base})
		if len(got.Findings) != 1 || got.Findings[0].Rule != "supply.secrets.4" || !strings.Contains(got.Findings[0].Missing, "base.toml") {
			t.Errorf("findings %v", got.Findings)
		}
	})
}

// TestAnExtendThatCannotBeFollowedIsNotCheckedAndNeverPassed: each row is a
// path the tool would follow (or refuse) and the checker does not read.
func TestAnExtendThatCannotBeFollowedIsNotCheckedAndNeverPassed(t *testing.T) {
	t.Parallel()
	ok := map[string]any{"extend": map[string]any{"useDefault": true}}
	for _, row := range []struct {
		name  string
		files map[string]string
		docs  map[string]map[string]any
		want  string
	}{
		{"a path above the repository", map[string]string{".gitleaks.toml": "top"}, map[string]map[string]any{"top": ext("../x.toml")}, "leaves the repository"},
		{"an absolute path", map[string]string{".gitleaks.toml": "top"}, map[string]map[string]any{"top": ext("/work/scratch/x.toml")}, "absolute"},
		{"a URL", map[string]string{".gitleaks.toml": "top"}, map[string]map[string]any{"top": ext("https://example.org/x.toml")}, "URL"},
		{"a backslash", map[string]string{".gitleaks.toml": "top"}, map[string]map[string]any{"top": ext(table(t, "backslash_paths.txt")[0] + ".toml")}, "backslash"},
		{"a trailing slash", map[string]string{".gitleaks.toml": "top", "real.toml": "a"}, map[string]map[string]any{"top": ext("real.toml/")}, "trailing slash"},
		{"a missing file", map[string]string{".gitleaks.toml": "top"}, map[string]map[string]any{"top": ext("gone.toml")}, "cannot be read"},
		{"a loop", map[string]string{".gitleaks.toml": "top", "a.toml": "a"}, map[string]map[string]any{"top": ext("a.toml"), "a": ext(".gitleaks.toml")}, "loops"},
		{"too long a chain", map[string]string{".gitleaks.toml": "top", "a": "a", "b": "b", "c": "c", "d": "d", "e": "e"},
			map[string]map[string]any{"top": ext("a"), "a": ext("b"), "b": ext("c"), "c": ext("d"), "d": ext("e"), "e": ok}, "longer than"},
		{"path with useDefault", map[string]string{".gitleaks.toml": "top", "a.toml": "a"},
			map[string]map[string]any{"top": {"extend": map[string]any{"path": "a.toml", "useDefault": true}}, "a": ok}, "together"},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			got := runChain(t, row.files, row.docs)
			if len(got.Findings) != 0 || len(got.NotChecked) != 1 || !strings.Contains(got.NotChecked[0].Why, row.want) {
				t.Errorf("findings %v, not checked %v; want one not checked with %q", got.Findings, got.NotChecked, row.want)
			}
		})
	}
}

// TestAChainOfFourExtendedFilesIsRead holds the longest chain read: four files
// below the config are read and judged, the fifth is refused (the row "too
// long a chain" above). boundary: maxExtendDepth
func TestAChainOfFourExtendedFilesIsRead(t *testing.T) {
	t.Parallel()
	files := map[string]string{".gitleaks.toml": "top", "a": "a", "b": "b", "c": "c", "d": "d"}
	docs := map[string]map[string]any{
		"top": ext("a"), "a": ext("b"), "b": ext("c"), "c": ext("d"),
		"d": {"extend": map[string]any{"useDefault": true}, "allowlist": map[string]any{"paths": []any{"vendor/"}}},
	}
	got := runChain(t, files, docs)
	if len(got.NotChecked) != 0 || len(got.Findings) != 2 {
		t.Fatalf("findings %v, not checked %v; want the chain read to its fourth file", got.Findings, got.NotChecked)
	}
}

// refusing is a repository whose file name cannot be read, for a reason that
// is no absence: the answer of the real folder to a link out, a FIFO or a
// folder.
type refusing struct {
	fstest.MapFS
	name string
}

func (r refusing) ReadFile(name string) ([]byte, error) {
	if name == r.name {
		return nil, errors.New("it is a FIFO, not a file")
	}
	return r.MapFS.ReadFile(name)
}

// TestAFileThatIsThereAndRefusedIsNotCheckedNeverMissing: the config, or an
// extended file, that the repository refuses to read (a link out, a FIFO) is
// the tool not checked; it is never "there is no config", which would be a
// finding the project did not earn, and it is never passed.
func TestAFileThatIsThereAndRefusedIsNotCheckedNeverMissing(t *testing.T) {
	t.Parallel()
	for _, row := range []struct{ name, refused string }{
		{"the config", ".gitleaks.toml"}, {"the extended file", "base.toml"},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			repo := refusing{fstest.MapFS{".gitleaks.toml": {Data: []byte("top")}, "base.toml": {Data: []byte("a")}}, row.refused}
			got := audit.Check(t.Context(), audit.Input{
				Sources: sources(), Repo: repo, Bindings: gitleaksBindings(),
				TOML: byContent(map[string]map[string]any{"top": ext("base.toml")}),
			})
			if len(got.Findings) != 0 || len(got.NotChecked) != 1 || !strings.Contains(got.NotChecked[0].Why, "cannot be read") {
				t.Errorf("findings %v, not checked %v; want one not checked", got.Findings, got.NotChecked)
			}
		})
	}
}
