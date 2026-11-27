package jsonschema_test

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// The code fences of the format spec (docs/format, M1-22) held to the
// schemas, through the real adapter (docs.samples.1): a fence is either text,
// which is not code, or a YAML or JSON document that the schema it names
// accepts. The field tables of the same documents are held to the schemas in
// the repository's own docs test.

// formatDocsDir is the folder of the prose, from this package's folder.
const formatDocsDir = "../../../docs/format"

var fenceLine = regexp.MustCompile("^```(.*)$")

// fence is one code fence of a document.
type fence struct {
	info string
	body string
	line int
}

// fencesOf are the fenced blocks of a document, in order, and a problem for
// a fence left open.
func fencesOf(text string) ([]fence, []string) {
	var out []fence
	var problems []string
	var cur *fence
	var body []string
	for i, l := range strings.Split(text, "\n") {
		m := fenceLine.FindStringSubmatch(l)
		switch {
		case cur == nil && m != nil:
			cur = &fence{info: strings.TrimSpace(m[1]), line: i + 1}
			body = nil
		case cur != nil && m != nil && strings.TrimSpace(m[1]) == "":
			cur.body = strings.Join(body, "\n") + "\n"
			out = append(out, *cur)
			cur = nil
		case cur != nil:
			body = append(body, l)
		}
	}
	if cur != nil {
		problems = append(problems, "a fence is never closed")
	}
	return out, problems
}

// fenceProblems says what is wrong with the fences of the documents of
// docs, by path.
func fenceProblems(t *testing.T, docs map[string]string) []string {
	t.Helper()
	s := formatSchema(t)
	var out []string
	for _, name := range slices.Sorted(maps.Keys(docs)) {
		fences, problems := fencesOf(docs[name])
		for _, p := range problems {
			out = append(out, name+": "+p)
		}
		for _, f := range fences {
			kind, schema, _ := strings.Cut(f.info, " ")
			at := name + ": line " + strconv.Itoa(f.line)
			switch {
			case f.info == "text":
			case (kind != "yaml" && kind != "json") || schema == "":
				out = append(out, at+": a fence marked \""+f.info+"\" is neither text nor a yaml or json document with its schema")
			case !fileKindSchema(schema):
				out = append(out, at+": the fence names \""+schema+"\", which is no schema of the format")
			default:
				doc, err := ordered.ParseYAML([]byte(f.body))
				if err != nil {
					out = append(out, at+": the "+kind+" does not parse: "+err.Error())
					continue
				}
				b, err := json.Marshal(ordered.Plain(doc))
				if err != nil {
					t.Fatal(err)
				}
				ps, err := s.Check("v3/"+schema, b)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range ps {
					out = append(out, at+": "+p.Msg+" ("+schema+")")
				}
			}
		}
	}
	return out
}

// fileKindSchema is whether name is a schema file of the format's folder.
func fileKindSchema(name string) bool {
	_, err := os.Stat(filepath.Join(assetsDir, "format", "schema", "v3", name))
	return err == nil && !strings.Contains(name, "/")
}

// formatDocs are the documents of docs/format, by path.
func formatDocs(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(formatDocsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, err := os.ReadFile(p)
		out[filepath.ToSlash(p)] = string(b)
		return err
	})
	if err != nil || len(out) == 0 {
		t.Fatalf("docs/format: %d documents, %v", len(out), err)
	}
	return out
}

func TestEveryFenceOfTheFormatSpecIsTextOrADocumentItsSchemaAccepts(t *testing.T) {
	t.Parallel()
	docs := formatDocs(t)
	for _, p := range fenceProblems(t, docs) {
		t.Error(p)
	}
	n := 0
	for _, text := range docs {
		f, _ := fencesOf(text)
		n += len(f)
	}
	if n < 10 {
		t.Errorf("the spec has %d fences, which is too few to be the examples of its file kinds", n)
	}
}

func TestTheFenceCheckFiresOnAPlantedBreach(t *testing.T) {
	t.Parallel()
	docs := formatDocs(t)
	terms := docs[formatDocsDir+"/terms.md"]
	if terms == "" {
		t.Fatal("no terms.md to plant a breach in")
	}
	plant := func(extra string) map[string]string { return map[string]string{"terms.md": terms + extra} }
	tests := []struct {
		name, extra, says string
	}{
		{"a fence the schema refuses", "\n```yaml terms.schema.json\nformat: 3\nterms:\n  - slug: gate\n```\n", "terms.schema.json"},
		{"a fence marked with no schema", "\n```yaml\nformat: 3\n```\n", "is neither text nor a yaml or json document with its schema"},
		{"a fence marked with a language only", "\n```go\nx := 1\n```\n", "is neither text nor a yaml or json document with its schema"},
		{"a fence naming no schema", "\n```yaml nothing.schema.json\nformat: 3\n```\n", "which is no schema of the format"},
		{"a fence that does not parse", "\n```yaml terms.schema.json\n: : [\n```\n", "does not parse"},
		{"a fence never closed", "\n```text\nopen\n", "a fence is never closed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := fenceProblems(t, plant(tc.extra))
			if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, tc.says) }) {
				t.Errorf("said %q, want a problem saying %q", got, tc.says)
			}
		})
	}
	t.Run("a fence marked text is not held to a schema", func(t *testing.T) {
		t.Parallel()
		if got := fenceProblems(t, plant("\n```text\nnot: [a document\n```\n")); len(got) != 0 {
			t.Errorf("a text fence was held to a schema: %q", got)
		}
	})
	t.Run("the clean control passes", func(t *testing.T) {
		t.Parallel()
		if got := fenceProblems(t, map[string]string{"terms.md": terms}); len(got) != 0 {
			t.Errorf("the committed terms.md has problems: %q", got)
		}
	})
}
