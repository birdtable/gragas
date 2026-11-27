package run_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/run"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// host keeps what a run did instead of drawing it.
type host struct {
	said, diag []string
	died       string
	ended      *run.Ending
	lead       []model.Block
	changes    []string
	applied    bool
	answers    []string
	asked      []string
}

func (h *host) Say(l string)          { h.said = append(h.said, l) }
func (h *host) Diag(l, m string)      { h.diag = append(h.diag, l+m) }
func (h *host) Die(m string) int      { h.died = m; return 1 }
func (h *host) SetChanges(l []string) { h.changes = l }
func (h *host) AddChanges(l []string) { h.changes = append(h.changes, l...) }
func (h *host) MarkApplied()          { h.applied = true }
func (h *host) End(e run.Ending, lead ...model.Block) int {
	h.ended, h.lead = &e, lead
	return 0
}
func (h *host) Ask(_ context.Context, p string) (string, error) {
	h.asked = append(h.asked, p)
	if len(h.answers) == 0 {
		return "", io.EOF
	}
	a := h.answers[0]
	h.answers = h.answers[1:]
	return a, nil
}

// change answers a preview with one file and an apply with what the test set.
type change struct {
	applied coreapi.Outcome
	moves   int
}

func (c *change) Preview(context.Context) (coreapi.Outcome, bool) {
	return coreapi.Outcome{Plan: model.Plan{Files: []model.Target{{Path: "a.yaml", Before: "x"}}}}, true
}

func (c *change) Apply(context.Context, model.Digest) coreapi.Outcome {
	if c.moves > 0 {
		c.moves--
		return coreapi.Outcome{Moved: true, Plan: model.Plan{Files: []model.Target{{Path: "b.yaml"}}}}
	}
	return c.applied
}

func plan(c run.Changer, mod func(*run.Plan)) run.Plan {
	p := run.Plan{Path: "gragas test", Takes: true, Change: c, Next: "gragas test --apply"}
	if mod != nil {
		mod(&p)
	}
	return p
}

// TestAFormNamesOnlyAFlagItsCommandTakes: a flag the command lacks, or one
// written without its dashes, is refused with the command's path.
func TestAFormNamesOnlyAFlagItsCommandTakes(t *testing.T) {
	t.Parallel()
	ran := 0
	for _, c := range []struct {
		takes bool
		flag  string
		ok    bool
	}{{true, "--apply", true}, {false, "--apply", false}, {true, "apply", false}, {true, "--", false}} {
		_, err := run.NewForm(c.takes, "gragas test", "apply?", c.flag)
		if (err == nil) != c.ok || (err != nil && !strings.Contains(err.Error(), "gragas test")) {
			t.Errorf("%+v: err = %v", c, err)
		}
		ran++
	}
	if ran != 4 {
		t.Fatalf("ran %d of 4", ran)
	}
	if f, _ := run.NewForm(true, "x", "apply?", run.ApplyFlag); f.Prompt() != "apply? (--apply answers it in a script)" {
		t.Errorf("prompt = %q", f.Prompt())
	}
}

func TestACommandWithoutTheFlagDiesBeforeAnythingIsPlanned(t *testing.T) {
	t.Parallel()
	h := &host{}
	if code := run.Run(t.Context(), h, plan(&change{}, func(p *run.Plan) { p.Takes = false })); code != 1 || !strings.Contains(h.died, "does not take") || h.ended != nil {
		t.Fatalf("code %d, died %q, ended %+v", code, h.died, h.ended)
	}
}

func TestApplyingWritesAndEndsDoneNamingTheRun(t *testing.T) {
	t.Parallel()
	h := &host{}
	c := &change{applied: coreapi.Outcome{Word: model.Done, Changes: []string{"a.yaml", "b.yaml"}, Run: "r1"}}
	run.Run(t.Context(), h, plan(c, func(p *run.Plan) { p.Applying = true }))
	if !h.applied || h.ended.Word != model.Done || h.ended.What != "2 files written, run r1" || !slices.Equal(h.changes, []string{"wrote a.yaml", "wrote b.yaml"}) {
		t.Fatalf("%+v %v", h.ended, h.changes)
	}
}

// closer is a change whose applied run says more, as plugin upgrade's does.
type closer struct {
	change
	what, next string
}

func (c *closer) Closing() (string, string) { return c.what, c.next }

// TestAnAppliedCloserSaysWhatItLeftAndAFileRemovedIsNotWritten: the words of
// a Closer follow the count and its line is the next; a file the plan removed
// is recorded as removed and counted apart from the writes; a Closer with
// nothing to say adds nothing. Planted: the closing words dropped, a removal
// recorded as a write, or a removal counted as one.
func TestAnAppliedCloserSaysWhatItLeftAndAFileRemovedIsNotWritten(t *testing.T) {
	t.Parallel()
	out := coreapi.Outcome{Word: model.Done, Changes: []string{"new.yaml", "old.yaml"}, Run: "r1",
		Plan: model.Plan{Files: []model.Target{{Path: "new.yaml"}, {Path: "old.yaml", Before: "sha256:x", Delete: true}}}}
	h := &host{}
	c := &closer{applied: out, what: "p@2.0.0 is installed and not approved by this upgrade", next: "gragas plugin approve p@2.0.0"}
	run.Run(t.Context(), h, plan(c, func(p *run.Plan) { p.Applying = true }))
	if h.ended.What != "1 file written, 1 removed, run r1; p@2.0.0 is installed and not approved by this upgrade" || h.ended.Next != "gragas plugin approve p@2.0.0" {
		t.Errorf("ended %+v", h.ended)
	}
	if !slices.Equal(h.changes, []string{"wrote new.yaml", "removed old.yaml"}) {
		t.Errorf("changes = %v", h.changes)
	}
	quiet := &host{}
	run.Run(t.Context(), quiet, plan(&closer{applied: out}, func(p *run.Plan) { p.Applying = true }))
	if quiet.ended.What != "1 file written, 1 removed, run r1" || quiet.ended.Next != "" {
		t.Errorf("a closer with nothing to say ended %+v", quiet.ended)
	}
}

// TestADryRunSaysAFileItWouldRemove: a delete target is "would remove", not
// "would replace". Planted: verbOf without its delete case.
func TestADryRunSaysAFileItWouldRemove(t *testing.T) {
	t.Parallel()
	h := &host{}
	run.Run(t.Context(), h, plan(&removes{}, nil))
	if !slices.Equal(h.changes, []string{"would remove old.yaml"}) {
		t.Errorf("changes = %v", h.changes)
	}
}

// removes previews a plan that removes one file.
type removes struct{ change }

func (*removes) Preview(context.Context) (coreapi.Outcome, bool) {
	return coreapi.Outcome{Plan: model.Plan{Files: []model.Target{{Path: "old.yaml", Before: "sha256:x", Delete: true}}}}, true
}

// TestNobodyToAskPrintsThePlanAndDeclines: the plan, the footer naming the
// flag and the word declined, with nothing asked.
func TestNobodyToAskPrintsThePlanAndDeclines(t *testing.T) {
	t.Parallel()
	h := &host{}
	run.Run(t.Context(), h, plan(&change{}, nil))
	if len(h.asked) != 0 || h.ended.Word != model.Declined || h.ended.Next != "gragas test --apply" || !slices.Equal(h.changes, []string{"would replace a.yaml"}) {
		t.Fatalf("%+v %v asked %v", h.ended, h.changes, h.asked)
	}
	if foot, ok := h.lead[len(h.lead)-1].(model.DryRunFooter); !ok || foot.Flag != "--apply" {
		t.Errorf("lead = %+v", h.lead)
	}
}

// TestAtATerminalAYesAppliesTheShownPlanAndAnythingElseDeclines, and a plan
// that moved while the person read is shown again and asked of again.
func TestAtATerminalAYesAppliesTheShownPlanAndAnythingElseDeclines(t *testing.T) {
	t.Parallel()
	done := coreapi.Outcome{Word: model.Done, Changes: []string{"a.yaml"}, Run: "r2"}
	for name, c := range map[string]struct {
		answers []string
		moves   int
		word    model.Word
		asked   int
	}{
		"yes":            {[]string{"Yes"}, 0, model.Done, 1},
		"no":             {[]string{"n"}, 0, model.Declined, 1},
		"the input ends": {nil, 0, model.Declined, 1},
		"it moved":       {[]string{"y", "y"}, 1, model.Done, 2},
	} {
		h := &host{answers: c.answers}
		run.Run(t.Context(), h, plan(&change{applied: done, moves: c.moves}, func(p *run.Plan) { p.Ask = true }))
		if h.ended.Word != c.word || len(h.asked) != c.asked {
			t.Errorf("%s: %+v asked %d", name, h.ended, len(h.asked))
		}
		if moved := slices.Contains(h.said, "The plan changed while you read it, and nothing was written. This is the plan now:"); moved != (c.moves > 0) {
			t.Errorf("%s: said %q", name, h.said)
		}
	}
}

func TestAFailureWithNoRecordEndsFailedWithTheBareWords(t *testing.T) {
	t.Parallel()
	h := &host{}
	c := &change{applied: coreapi.Outcome{Word: model.Failed, RolledBack: true}}
	run.Run(t.Context(), h, plan(c, func(p *run.Plan) { p.Applying = true }))
	if h.ended.Word != model.Failed || h.ended.What != "the run failed" {
		t.Fatalf("%+v", h.ended)
	}
}

// unwritten is a change that binds tools no plugin writes a preset for.
type unwritten struct {
	change
	tools []string
}

func (u *unwritten) PresetsUnwritten() []string { return u.tools }

// TestAnApplyThatBindsAToolNoPluginWritesEndsNotReady: the record
// is written and the preset is not, so the run ends not ready naming the tools
// and the install line; a change that leaves none unwritten ends done.
func TestAnApplyThatBindsAToolNoPluginWritesEndsNotReady(t *testing.T) {
	t.Parallel()
	applied := coreapi.Outcome{Word: model.Done, Changes: []string{"a.yaml"}, Run: "r1"}
	h := &host{}
	run.Run(t.Context(), h, plan(&unwritten{applied: applied, tools: []string{"lint-a", "lint-b"}}, func(p *run.Plan) { p.Applying = true }))
	if !h.applied || h.ended.Word != model.NotReady || h.ended.Next != "gragas plugin add <folder>" ||
		h.ended.What != "1 file written, run r1; no installed plugin writes a preset for lint-a, lint-b" {
		t.Fatalf("%+v", h.ended)
	}
	h = &host{}
	run.Run(t.Context(), h, plan(&unwritten{applied: applied}, func(p *run.Plan) { p.Applying = true }))
	if h.ended.Word != model.Done {
		t.Fatalf("%+v", h.ended)
	}
}
