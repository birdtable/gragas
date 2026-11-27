package lifecycle_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/markdown"
	"github.com/birdtable/gragas/internal/model"
)

func guideDoc(t *testing.T) (*markdown.Doc, []byte) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "guide", "handlers.md"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := markdown.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return d, src
}

func guideReq() model.DraftGuide {
	return model.DraftGuide{Part: "go", Concern: "style", Files: files(), Guide: model.GuideRef{
		Slug: "handler-guide", Title: "Handler guide", URL: "https://example.org/handlers", Tag: "v1.2.0", Attribution: "The handlers team"}}
}

func listDrafts(ds []lifecycle.Draft) string {
	var b strings.Builder
	for _, d := range ds {
		fmt.Fprintf(&b, "%s #%s line %d %s %q", d.ID, d.Anchor, d.Line, d.Strength, d.Statement)
		if d.Todo != "" {
			fmt.Fprintf(&b, " TODO %q", d.Todo)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", "guide", name))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s is:\n%s\nwant:\n%s", name, got, want)
	}
}

// TestTheFixtureGuideGivesItsDraftsInOrderWithTheirStrengthWords: six headings
// and nine MUST lines, one rule a line, the heading of a section with no strength
// line a rule of its own, SHOULD and MAY mapped to should and may, and the line
// with two words taking the first and a to-do. Planted failure: a MUST NOT line
// read as may, or the ambiguous line without its to-do.
func TestTheFixtureGuideGivesItsDraftsInOrderWithTheirStrengthWords(t *testing.T) {
	t.Parallel()
	doc, _ := guideDoc(t)
	if len(doc.Headings) != 6 || len(doc.Musts) != 9 {
		t.Fatalf("the fixture has %d headings and %d MUST lines, want 6 and 9", len(doc.Headings), len(doc.Musts))
	}
	golden(t, "drafts.golden", listDrafts(lifecycle.DraftsOf(doc, "go", "style", 1)))
}

// TestTheDraftsPlanIsTheGuideAsASourceOneDecisionAndEachRuleProposedAndImported:
// the plan screen and the changes, in a concern file and in a guide, where a
// section of several rules is one block under its anchor.
func TestTheDraftsPlanIsTheGuideAsASourceOneDecisionAndEachRuleProposedAndImported(t *testing.T) {
	t.Parallel()
	doc, _ := guideDoc(t)
	p, err := lifecycle.PlanDraftGuide(book(), doc, guideReq(), moment())
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	out.WriteString(screenOf(p))
	for _, c := range p.Delta.Changes {
		fmt.Fprintf(&out, "--- %s\n%s", strings.TrimSpace(c.Entity+" "+c.Field), c.After)
	}
	golden(t, "plan.golden", out.String())
	if p.Record.Kind != "import" || len(p.Also) != 11 || p.Record.Decision != "imported-handler-guide-v1-2-0" {
		t.Errorf("record = %+v, also %d", p.Record, len(p.Also))
	}

	g := guideReq()
	g.Files.Rules = "rulebook/language/go/guide.md"
	pg, err := lifecycle.PlanDraftGuide(book(), doc, g, moment())
	if err != nil {
		t.Fatal(err)
	}
	var blocks, rules int
	for _, c := range pg.Delta.Changes {
		if strings.HasPrefix(c.Entity, "block:") {
			blocks++
			rules += strings.Count(c.After, "\n  - id: ")
		}
	}
	if blocks != 6 || rules != 12 {
		t.Errorf("a guide form has %d blocks and %d rules, want 6 and 12", blocks, rules)
	}
}

// TestNoGuideParagraphIsCopiedIntoADraft scans the drafts and the written
// rules for any run of the guide's prose beyond one line: a statement is one
// line of the guide, and nothing else of its paragraphs is in the plan.
func TestNoGuideParagraphIsCopiedIntoADraft(t *testing.T) {
	t.Parallel()
	doc, _ := guideDoc(t)
	p, err := lifecycle.PlanDraftGuide(book(), doc, guideReq(), moment())
	if err != nil {
		t.Fatal(err)
	}
	var written strings.Builder
	for _, c := range p.Delta.Changes {
		written.WriteString(c.After)
	}
	for _, para := range doc.Paragraphs {
		if strings.Contains(para.Text, "\n") && strings.Contains(written.String(), para.Text) {
			t.Errorf("the paragraph %q is copied", para.Text)
		}
	}
	if strings.Contains(written.String(), "The intro carries no strength word") {
		t.Error("prose with no strength word is copied")
	}
	for _, d := range lifecycle.DraftsOf(doc, "go", "style", 1) {
		if strings.Contains(d.Statement, "\n") {
			t.Errorf("the statement of %s runs over more than one line", d.ID)
		}
	}
}

func TestAnImportAlreadyInTheConcernPlansNothingAndABadGuideIsRefused(t *testing.T) {
	t.Parallel()
	doc, _ := guideDoc(t)
	b := book()
	for i, d := range lifecycle.DraftsOf(doc, "go", "style", 2) {
		b.Rules = append(b.Rules, model.Rule{ID: model.RuleID(d.ID), Statement: d.Statement, Status: model.Proposed, Origin: model.Imported})
		_ = i
	}
	p, err := lifecycle.PlanDraftGuide(b, doc, guideReq(), moment())
	if err != nil || len(p.Delta.Changes) != 0 || !strings.Contains(screenOf(p), "nothing") {
		t.Errorf("a second import: %+v, %v", p, err)
	}
	for name, mut := range map[string]func(*model.DraftGuide){
		"part":    func(r *model.DraftGuide) { r.Part = "nope" },
		"concern": func(r *model.DraftGuide) { r.Concern = "nope" },
		"slug":    func(r *model.DraftGuide) { r.Guide.Slug = "Bad Slug" },
		"tag":     func(r *model.DraftGuide) { r.Guide.Tag = "" },
		"url":     func(r *model.DraftGuide) { r.Guide.URL = "" },
	} {
		r := guideReq()
		mut(&r)
		if _, err := lifecycle.PlanDraftGuide(book(), doc, r, moment()); err == nil {
			t.Errorf("a bad %s is planned", name)
		}
	}
}

// FuzzDraftsOf holds the draft step to its promises on any guide: it never
// panics; reading the same guide twice gives the same ids and the same drafts;
// the ids are consecutive from the start; every statement is one line; and
// every draft stands under an anchor the guide has, or the preamble.
func FuzzDraftsOf(f *testing.F) {
	_, fixture := func() (any, []byte) {
		src, err := os.ReadFile(filepath.Join("testdata", "guide", "handlers.md"))
		if err != nil {
			f.Fatal(err)
		}
		return nil, src
	}()
	for _, seed := range []string{
		"",
		"# T\n\nIt MUST work.\n",
		"A MUST before any heading.\n\n# T\n",
		"# T\n\nA MUST and a SHOULD and a MAY.\n",
		"# T\n## T\nx MAY y\n",
		"# T\n\n```gragas\nid: a MUST\n```\n\n    indented MUST\n",
		"> quoted MUST\n- listed SHOULD\n",
		"\x00\xff MUST\r\n# \r\n",
		string(fixture),
	} {
		f.Add([]byte(seed), 1)
	}
	f.Fuzz(func(t *testing.T, data []byte, first int) {
		doc, _ := markdown.Parse(data)
		if first < 1 || first > 1<<20 {
			first = 1
		}
		a := lifecycle.DraftsOf(doc, "go", "style", first)
		b := lifecycle.DraftsOf(doc, "go", "style", first)
		if fmt.Sprint(a) != fmt.Sprint(b) {
			t.Fatalf("the same guide gave two lists of drafts:\n%v\n%v", a, b)
		}
		anchors := map[string]bool{"preamble": true}
		for _, h := range doc.Headings {
			anchors[h.Anchor] = true
		}
		for i, d := range a {
			if want := fmt.Sprintf("go.style.%d", first+i); d.ID != want {
				t.Fatalf("draft %d has id %s, want %s", i, d.ID, want)
			}
			if strings.Contains(d.Statement, "\n") || !anchors[d.Anchor] {
				t.Fatalf("draft %+v has a statement over several lines or an anchor the guide lacks", d)
			}
			if d.Strength != model.Must && d.Strength != model.Should && d.Strength != model.May {
				t.Fatalf("draft %+v has no strength", d)
			}
		}
	})
}
