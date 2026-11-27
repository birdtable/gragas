package coreapi_test

import (
	"context"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const ruleStructure = `format: 3
layers:
  - slug: language
    name: Language
    parts:
      - slug: go
        name: Go
        concerns:
          - slug: errors
            name: Errors
          - slug: layout
            name: Layout
kinds: []
`

const ruleGuide = "# Go\n\n```gragas\nanchor: layout\nrules:\n  - id: go.layout.1\n    statement: Keep a package small.\n    strength: should\n    status: proposed\n    origin: authored\n```\n"

func allSchemas() []string {
	return []string{"v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json", "v3/rule.schema.json", "v3/block.schema.json", "v3/sources.schema.json", "v3/decisions.schema.json", "v3/history.schema.json", "v3/common.schema.json"}
}

func ruleWorld(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{
		"book/rulebook.yaml": bookIdentity, "book/structure.yaml": ruleStructure,
		"book/language/go/rules/errors.yaml": bookRules, "book/language/go/guide.md": ruleGuide,
	})
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
	return w, root
}

// testWho is what a test's change names as who made it, as the command tree
// hands in a command's path.
const testWho = "gragas test"

func changeAsk(root model.Root, cmd string) model.ChangeAsk {
	return model.ChangeAsk{Root: root, Command: cmd, Applies: cmd + " --apply", Who: testWho, Run: "run-1", Engine: "v0.0.0-test"}
}

// runTwo is the ask of a second run: a run id names one run.
func runTwo(a model.ChangeAsk) model.ChangeAsk { a.Run = "run-2"; return a }

func read(t *testing.T, w *porttest.World, root model.Root, p string) string {
	t.Helper()
	b, err := w.Ports().Tree.Read(root, p)
	if err != nil {
		return ""
	}
	return string(b)
}

func newRuleReq() model.NewRule {
	return model.NewRule{Part: "go", Concern: "errors", Statement: "Return errors, do not panic.", Strength: model.Must}
}

func TestRuleNewWritesTheRuleAndOneHistoryLineAndASecondRunHasNothingToDo(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	c, ref, err := coreapi.RuleNew(context.Background(), w.Ports(), changeAsk(root, "gragas rule new"), newRuleReq())
	if err != nil || ref != nil {
		t.Fatalf("ref = %v, err = %v", ref, err)
	}
	shown, ok := c.Preview(context.Background())
	if !ok || len(shown.Plan.Sections) != 2 || shown.Plan.Digest == "" {
		t.Fatalf("preview ok=%v word=%q plan=%+v", ok, shown.Word, shown.Plan)
	}
	if got := read(t, w, root, "book/language/go/rules/errors.yaml"); strings.Contains(got, "panic") {
		t.Fatal("the dry run wrote the rule")
	}
	out := c.Apply(context.Background(), "")
	if out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	rules := read(t, w, root, "book/language/go/rules/errors.yaml")
	if !strings.Contains(rules, "  - id: go.errors.2\n    statement: \"Return errors, do not panic.\"\n") || !strings.Contains(rules, "status: proposed") {
		t.Errorf("rules file:\n%s", rules)
	}
	hist := read(t, w, root, "book/history.jsonl")
	if strings.Count(hist, "\n") != 1 || !strings.Contains(hist, `"kind":"new"`) || !strings.Contains(hist, `"entity":"rule:go.errors.2"`) || !strings.Contains(hist, `"who":"`+testWho+`"`) {
		t.Errorf("history:\n%s", hist)
	}
	again, ref, err := coreapi.RuleNew(context.Background(), w.Ports(), runTwo(changeAsk(root, "gragas rule new")), newRuleReq())
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	second := again.Apply(context.Background(), "")
	if second.Word != model.Done || !second.Nothing {
		t.Errorf("a second identical run ended %q nothing=%v (%v); want done, nothing to do", second.Word, second.Nothing, second.Cause)
	}
	if read(t, w, root, "book/history.jsonl") != hist {
		t.Error("the second run wrote history")
	}
}

func TestRuleNewWithAnOwnDecisionMakesTheSourceAndDecisionFilesFromTheirHeads(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	r := newRuleReq()
	r.Own = true
	r.Decision = &model.NewDecision{Title: "Return errors", Text: "We return errors."}
	c, ref, err := coreapi.RuleNew(context.Background(), w.Ports(), changeAsk(root, "gragas rule new"), r)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(context.Background(), ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	src, dec := read(t, w, root, "book/sources.yaml"), read(t, w, root, "book/decisions.yaml")
	if !strings.HasPrefix(src, "format: 3\nsources:\n") || !strings.Contains(src, "kind: own-decision") || !strings.Contains(dec, "who: "+testWho+"\n") || !strings.Contains(dec, `date: "2026-11-08"`) {
		t.Errorf("sources:\n%s\ndecisions:\n%s", src, dec)
	}
	if !strings.Contains(read(t, w, root, "book/history.jsonl"), `"decision":"decide-go-errors-2"`) {
		t.Error("the history does not name the decision")
	}
}

func TestRuleNewOfAGuideConcernAddsToTheGuideBlock(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	r := newRuleReq()
	r.Concern, r.Statement = "layout", "Name a package for what it holds."
	c, ref, err := coreapi.RuleNew(context.Background(), w.Ports(), changeAsk(root, "gragas rule new"), r)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(context.Background(), ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	g := read(t, w, root, "book/language/go/guide.md")
	if !strings.Contains(g, "id: go.layout.2") || !strings.HasPrefix(g, "# Go\n\n```gragas\nanchor: layout") {
		t.Errorf("guide:\n%s", g)
	}
	if read(t, w, root, "book/language/go/rules/layout.yaml") != "" {
		t.Error("a concern file was made beside the guide")
	}
}

func TestARequestThePlanRefusesIsARefusalWithAHintAndNothingIsPlanned(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	r := newRuleReq()
	r.Part = "rust"
	c, ref, err := coreapi.RuleNew(context.Background(), w.Ports(), changeAsk(root, "gragas rule new"), r)
	if err != nil || c != nil || ref == nil || ref.Word() != model.Refused || ref.Hint() == "" || !strings.Contains(ref.Observed(), "rust") {
		t.Errorf("change=%v ref=%v err=%v", c, ref, err)
	}
}

// TestARuleChangeNeedsNoGitIdentityAndOneThatNamesNoCommandIsAnError: who is
// the command's name, which the command tree hands in, so a repository whose
// git names nobody plans as well as any, and an ask that names
// no command is an error, not a guess.
func TestARuleChangeNeedsNoGitIdentityAndOneThatNamesNoCommandIsAnError(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	w.SeedRepo(root, porttest.Repo{})
	if _, ref, err := coreapi.RuleNew(context.Background(), w.Ports(), changeAsk(root, "gragas rule new"), newRuleReq()); err != nil || ref != nil {
		t.Errorf("with git naming nobody: ref = %v, err = %v", ref, err)
	}
	ask := changeAsk(root, "gragas rule new")
	ask.Who = ""
	if _, _, err := coreapi.RuleNew(context.Background(), w.Ports(), ask, newRuleReq()); err == nil || !strings.Contains(err.Error(), "names no command") {
		t.Errorf("with no command named: err = %v", err)
	}
}

func TestRuleEditChangesOnlyTheFieldsItIsGivenAndAHandEditAfterwardsIsNotUndone(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	owner := "platform"
	edit := model.EditRule{ID: "go.errors.1", Owner: &owner}
	c, ref, err := coreapi.RuleEdit(context.Background(), w.Ports(), changeAsk(root, "gragas rule edit"), edit)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(context.Background(), ""); out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	file := "book/language/go/rules/errors.yaml"
	edited := strings.Replace(read(t, w, root, file), "    owner: platform\n", "    owner: platform\n    review_by: \"2027-05-05\"\n", 1)
	if err := w.Ports().Writer.Replace(root, file, []byte(edited)); err != nil {
		t.Fatal(err)
	}
	again, ref, err := coreapi.RuleEdit(context.Background(), w.Ports(), runTwo(changeAsk(root, "gragas rule edit")), edit)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := again.Apply(context.Background(), ""); out.Word != model.Done || !out.Nothing {
		t.Errorf("the repeat ended %q nothing=%v; want nothing to do", out.Word, out.Nothing)
	}
	if got := read(t, w, root, file); got != edited {
		t.Errorf("the repeat touched the file:\n%s", got)
	}
	if n := strings.Count(read(t, w, root, "book/history.jsonl"), "\n"); n != 1 {
		t.Errorf("history has %d lines; want the one edit", n)
	}
}

func TestRuleEditOfARuleThatIsNotThereIsARefusal(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	o := "x"
	_, ref, err := coreapi.RuleEdit(context.Background(), w.Ports(), changeAsk(root, "gragas rule edit"), model.EditRule{ID: "go.errors.9", Owner: &o})
	if err != nil || ref == nil || !strings.Contains(ref.Observed(), "go.errors.9") {
		t.Errorf("ref=%v err=%v", ref, err)
	}
}

func why() model.Why {
	return model.Why{New: &model.NewDecision{Slug: "decide-it", Title: "Decide it", Text: "Because."}}
}

func applied(t *testing.T, c *coreapi.Change, ref *model.Refusal, err error) coreapi.Outcome {
	t.Helper()
	if err != nil || ref != nil {
		t.Fatalf("ref = %v, err = %v", ref, err)
	}
	return c.Apply(context.Background(), "")
}

func rows(s string) int { return strings.Count(strings.TrimSpace(s), "\n") + 1 }

func TestRetireThenRestoreKeepsBothEventsAndTheRuleInItsFile(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	ctx := context.Background()
	c, ref, err := coreapi.RuleRetire(ctx, w.Ports(), changeAsk(root, "gragas rule retire go.errors.1"), model.RetireRule{ID: "go.errors.1", Why: why()})
	if out := applied(t, c, ref, err); out.Word != model.Done || out.Nothing {
		t.Fatalf("retire: %+v", out)
	}
	rules := read(t, w, root, "book/language/go/rules/errors.yaml")
	if !strings.Contains(rules, "status: retired") || !strings.Contains(rules, "id: go.errors.1") {
		t.Errorf("the retired rule is not kept:\n%s", rules)
	}
	again := runTwo(changeAsk(root, "gragas rule retire go.errors.1"))
	c, ref, err = coreapi.RuleRetire(ctx, w.Ports(), again, model.RetireRule{ID: "go.errors.1", Why: model.Why{Cite: "decide-it"}})
	if out := applied(t, c, ref, err); !out.Nothing || out.Word != model.Done {
		t.Fatalf("retire twice: %+v", out)
	}
	ask := changeAsk(root, "gragas rule restore go.errors.1")
	ask.Run = "run-3"
	c, ref, err = coreapi.RuleRestore(ctx, w.Ports(), ask, model.RestoreRule{ID: "go.errors.1", ReviewBy: "2027-02-01", Why: model.Why{New: &model.NewDecision{Slug: "decide-back", Title: "Back", Text: "It holds again."}}})
	if out := applied(t, c, ref, err); out.Word != model.Done || out.Nothing {
		t.Fatalf("restore: %+v", out)
	}
	rules = read(t, w, root, "book/language/go/rules/errors.yaml")
	if !strings.Contains(rules, "status: active") || !strings.Contains(rules, "review_by: ") {
		t.Errorf("the restored rule is not active:\n%s", rules)
	}
	h := read(t, w, root, "book/history.jsonl")
	if rows(h) != 2 || !strings.Contains(h, `"kind":"retire"`) || !strings.Contains(h, `"kind":"restore"`) {
		t.Errorf("history keeps %d lines, want a retire and a restore:\n%s", rows(h), h)
	}
	ask.Run = "run-4"
	c, ref, err = coreapi.RuleRestore(ctx, w.Ports(), ask, model.RestoreRule{ID: "go.errors.1", ReviewBy: "2027-02-01", Why: model.Why{Cite: "decide-back"}})
	if err != nil || ref == nil || !strings.Contains(ref.Observed(), "status") {
		t.Errorf("restore of an active rule: ref = %v, err = %v", ref, err)
	}
	_ = c
}

func TestReplaceAddsTheSuccessorAndRetiresTheOldRuleInOneHistoryLine(t *testing.T) {
	t.Parallel()
	w, root := ruleWorld(t)
	req := model.ReplaceRule{ID: "go.errors.1", Why: why(), Successor: model.NewRule{Statement: "Wrap an error with its call.", Strength: model.Must}}
	c, ref, err := coreapi.RuleReplace(context.Background(), w.Ports(), changeAsk(root, "gragas rule replace go.errors.1"), req)
	if out := applied(t, c, ref, err); out.Word != model.Done || out.Nothing {
		t.Fatalf("replace: %+v", out)
	}
	rules := read(t, w, root, "book/language/go/rules/errors.yaml")
	for _, want := range []string{"id: go.errors.2", "Wrap an error with its call.", "replaced_by: go.errors.2", "status: retired"} {
		if !strings.Contains(rules, want) {
			t.Errorf("errors.yaml lacks %q:\n%s", want, rules)
		}
	}
	if n := rows(read(t, w, root, "book/history.jsonl")); n != 1 {
		t.Errorf("history holds %d lines, want 1", n)
	}
	c, ref, err = coreapi.RuleReplace(context.Background(), w.Ports(), runTwo(changeAsk(root, "gragas rule replace go.errors.1")), req)
	if out := applied(t, c, ref, err); !out.Nothing {
		t.Errorf("replace twice: %+v", out)
	}
}
