package format

import (
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/markdown"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const guideRules = `  - id: go.errors.9
    statement: Return the error; do not log and return it.
    strength: must
    status: active
    review_by: "2027-01-31"
    origin: authored
  - id: go.errors.10
    statement: Name the failed step in the message.
    strength: should
    status: active
    review_by: "2027-01-31"
    origin: authored
`

const guideMD = "# Go\n\nProse that is never touched.\n\n## Errors\n\nMore prose, with a `code span`.\n\n```gragas\nanchor: errors\nrules:\n" + guideRules + "```\n\n## Layout\n\nNo rules here.\n\n## Tests\n\n```go\nfmt.Println(\"not a gragas block\")\n```\n"

func guideWorld(t *testing.T, guide string) (Loaded, error) {
	t.Helper()
	files := fullBook(t)
	files["language/go/guide.md"] = guide
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	return load(t, w, w.Seed("repo", files))
}

func texts(l Loaded) string {
	var s []string
	for _, f := range l.Findings {
		s = append(s, f.String())
	}
	return strings.Join(s, "\n")
}

func TestAGuideBlockWithSeveralRulesLoadsAndEqualsTheYAMLForm(t *testing.T) {
	t.Parallel()
	got, err := guideWorld(t, guideMD)
	if err != nil || len(got.Findings) != 0 {
		t.Fatalf("Load = %v, %v", got.Findings, err)
	}
	v, err := ordered.ParseYAML([]byte("rules:\n" + guideRules))
	if err != nil {
		t.Fatal(err)
	}
	var want []model.Rule
	for _, m := range ordered.Maps(v.(*ordered.Map).List("rules")) {
		want = append(want, ruleOf(m))
	}
	have := got.Book.Rules[len(got.Book.Rules)-2:]
	if len(want) != 2 || !reflect.DeepEqual(have, want) {
		t.Errorf("guide rules = %+v\nYAML rules = %+v", have, want)
	}
	if got.Book.Anchors["go.errors.9"] != "errors" || got.Book.Anchors["go.errors.10"] != "errors" || len(got.Book.Anchors) != 2 {
		t.Errorf("anchors = %v", got.Book.Anchors)
	}
	if len(got.Book.Rules) != 5 {
		t.Errorf("%d rules; want the 3 of the YAML form and 2 of the guide", len(got.Book.Rules))
	}
}

func TestTheProseOfAGuideIsNeverChanged(t *testing.T) {
	t.Parallel()
	doc, err := markdown.Parse([]byte(guideMD))
	if err != nil {
		t.Fatal(err)
	}
	files := fullBook(t)
	files["language/go/guide.md"] = guideMD
	w := porttest.NewWorld(t)
	w.SeedSchemas(schemaNames()...)
	root := w.Seed("repo", files)
	if _, err := load(t, w, root); err != nil {
		t.Fatal(err)
	}
	if len(w.Writes) != 0 {
		t.Errorf("Load wrote %d times: %+v", len(w.Writes), w.Writes)
	}
	if got, ok := w.File(root, "language/go/guide.md"); !ok || got != guideMD {
		t.Errorf("the guide in the tree moved: ok=%v", ok)
	}
	if len(doc.Find(markdown.GragasInfo)) != 1 {
		t.Error("the go block was taken for a gragas one")
	}
}

func TestAMalformedBlockIsAFindingAtItsMarkdownLineAndTheNextBlockIsStillRead(t *testing.T) {
	t.Parallel()
	md := "# Go\n\n## One\n\n```gragas\nanchor: one\nrules: [\n```\n\n## Two\n\n```gragas\nanchor: two\nrules:\n" + guideRules + "```\n"
	got, err := guideWorld(t, md)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 1 || got.Findings[0].File != "language/go/guide.md" || got.Findings[0].Line < 6 || got.Findings[0].Line > 8 {
		t.Fatalf("findings = %s", texts(got))
	}
	if got.Book.Anchors["go.errors.9"] != "two" {
		t.Errorf("the second block was not read: %v", got.Book.Anchors)
	}
}

func TestTwoBlocksUnderOneAnchorAndARepeatedIDAreFindings(t *testing.T) {
	t.Parallel()
	block := "```gragas\nanchor: errors\nrules:\n" + guideRules + "```\n"
	got, err := guideWorld(t, "## Errors\n\n"+block+"\n"+block)
	if err != nil {
		t.Fatal(err)
	}
	all := texts(got)
	if len(got.Findings) != 1 || !strings.Contains(all, "language/go/guide.md:20: two blocks under the anchor \"errors\"") {
		t.Fatalf("findings = %s", all)
	}
	other := "```gragas\nanchor: more\nrules:\n" + guideRules + "```\n"
	got, _ = guideWorld(t, "## Errors\n\n"+block+"\n## More\n\n"+other)
	if len(got.Findings) != 2 || !strings.Contains(texts(got), "the id go.errors.9 is also at") {
		t.Fatalf("findings = %s", texts(got))
	}
	yaml := "```gragas\nanchor: errors\nrules:\n  - id: go.errors.1\n    statement: x\n    strength: must\n    status: active\n    review_by: \"2027-01-31\"\n    origin: authored\n```\n"
	got, _ = guideWorld(t, "## Errors\n\n"+yaml)
	if len(got.Findings) != 1 || !strings.Contains(texts(got), "the id go.errors.1 is also at language/go/rules/errors.yaml:") {
		t.Fatalf("across the two forms: %s", texts(got))
	}
}

func TestAnUnclosedFenceIsAFindingAtItsOpeningLine(t *testing.T) {
	t.Parallel()
	got, err := guideWorld(t, "# Go\n\n```gragas\nanchor: a\n")
	if err != nil || len(got.Findings) != 1 || got.Findings[0].Line != 3 {
		t.Fatalf("Load = %s, %v", texts(got), err)
	}
}

func FuzzGuideBlocks(f *testing.F) {
	f.Add([]byte(guideMD))
	f.Add([]byte("```gragas\n"))
	f.Add([]byte("```gragas\nanchor: [\n```\n"))
	w := porttest.NewWorld(f)
	w.SeedSchemas(schemaNames()...)
	schema := w.Ports().Schema
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, _ := markdown.Parse(data)
		for _, b := range doc.Find(markdown.GragasInfo) {
			m, bad, err := parseAt(schema, "g.md", []byte(b.Body), blockSchema, b.Line)
			if m != nil && (len(bad) > 0 || err != nil) {
				t.Fatalf("a block with problems was answered: %v, %v", bad, err)
			}
			for _, x := range bad {
				if x.Line < b.Line {
					t.Fatalf("finding above its block: %v", x)
				}
			}
		}
	})
}
