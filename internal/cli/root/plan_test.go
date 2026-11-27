package root

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-console/uitest"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The question the form asks, as a person reads it.
const question = "apply? (--apply answers it in a script)"

// A planWorld is a rulebook root in the fake world with a terminal of 80
// columns scripted with answers, and the change of the test-only plan
// command: retire rules/a.yaml, with identity.yaml read as an input and the
// yes appended to history.jsonl.
type planWorld struct {
	t      *testing.T
	w      *porttest.World
	diag   *porttest.World
	root   model.Root
	change *coreapi.Change
	// moved, when set, runs before the first question: another process
	// changing the tree while the person reads.
	moved func()
}

func newPlanWorld(t *testing.T, answers ...string) *planWorld {
	t.Helper()
	w, diag := porttest.NewWorld(t), porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	w.SeedTerminal(porttest.Term{Width: 80, Answers: answers})
	diag.SeedTerminal(porttest.Term{})
	pw := &planWorld{t: t, w: w, diag: diag}
	pw.root = w.Seed("/work/rulebook", map[string]string{
		"identity.yaml": "name: demo\n", "rules/a.yaml": "status: active\n", "history.jsonl": `{"n":1}` + "\n",
	})
	pw.change = coreapi.NewChange(w.Ports(), pw.root, coreapi.Spec{
		Command: "gragas test", Applies: "gragas test --apply", Engine: "v0.0.0-test", Run: "run-1",
		Scope: []string{"rules/*.yaml", "history.jsonl"}, Plan: pw.plan, Render: pw.render, Record: pw.record,
	})
	return pw
}

func (pw *planWorld) file(p string) string {
	s, _ := pw.w.File(pw.root, p)
	return s
}

func (pw *planWorld) digest(p string) model.Digest {
	return model.Digest(digest.Prefixed([]byte(pw.file(p))))
}

func (pw *planWorld) plan(context.Context) (model.Plan, error) {
	var p model.Plan
	if pw.file("rules/a.yaml") != "status: retired\n" {
		p.Delta.Changes = []model.Change{{Entity: "rule:a", Field: "status", Before: "active", After: "retired"}}
		p.Sections = []model.Section{{Title: "what changes", Lines: []string{"rule a: active, then retired"}}}
	}
	p.Yes = &model.Yes{Who: "A Person <a@example.test>", At: 1794132000, Command: "gragas test"}
	for _, in := range []string{"identity.yaml", "rules/a.yaml", "history.jsonl"} {
		p.Inputs = append(p.Inputs, model.Input{Path: in, Digest: pw.digest(in)})
	}
	return p, nil
}

func (pw *planWorld) render(d model.Delta) ([]model.Target, error) {
	if len(d.Changes) == 0 {
		return nil, nil
	}
	return []model.Target{{Path: "rules/a.yaml", Before: pw.digest("rules/a.yaml"), Data: []byte("status: retired\n")}}, nil
}

func (pw *planWorld) record(y model.Yes) (model.Target, error) {
	line := `{"who":"` + y.Who + `","plan":"` + string(y.Plan) + `"}`
	return model.Target{Path: "history.jsonl", Before: pw.digest("history.jsonl"), Data: []byte(line), Append: true}, nil
}

// movingTerminal runs moved before the first question it asks.
type movingTerminal struct {
	port.Terminal
	moved func()
}

func (m *movingTerminal) Ask(ctx context.Context, q string) (string, error) {
	if m.moved != nil {
		m.moved()
		m.moved = nil
	}
	return m.Terminal.Ask(ctx, q)
}

// run runs the form, given --apply or not, with a person to answer or not.
func (pw *planWorld) run(apply, ask bool) int {
	pw.t.Helper()
	c := &cobra.Command{Use: "test"}
	c.Flags().Bool("apply", false, "Apply the plan")
	if apply {
		if err := c.Flags().Set("apply", "true"); err != nil {
			pw.t.Fatal(err)
		}
	}
	p := pw.w.Ports()
	p.Terminal = &movingTerminal{Terminal: p.Terminal, moved: pw.moved}
	s := &session{p: Process{World: p, Diagnostics: pw.diag.Ports().Terminal}, rec: &recorder{}}
	return s.plan(pw.t.Context(), planRun{cmd: c, change: pw.change, next: "gragas test --apply", ask: ask})
}

// closing is the last screen's closing block.
func (pw *planWorld) closing() model.Closing {
	pw.t.Helper()
	if len(pw.w.Shown) == 0 {
		pw.t.Fatal("no screen was shown")
	}
	bl := pw.w.Shown[len(pw.w.Shown)-1].Blocks()
	c, _ := bl[len(bl)-1].(model.Closing)
	return c
}

// runs are the run ids of the journal, in the order they first appear,
// with each record's kind.
func (pw *planWorld) runs() map[string][]model.Record {
	pw.t.Helper()
	recs, _, err := pw.w.Ports().Journal.Records(pw.t.Context(), pw.root)
	if err != nil {
		pw.t.Fatal(err)
	}
	out := map[string][]model.Record{}
	for _, r := range recs {
		out[r.Run] = append(out[r.Run], r)
	}
	return out
}

// TestEveryFormOfThePlanEndsOnItsWord walks every form the plan command
// has: --apply, off a terminal, and each answer at a terminal. --apply asks
// nothing and applies; off a terminal nothing is asked, the plan prints and
// the run ends declined, exit 6, with the one footer naming --apply before
// the word; at a terminal the question, naming its flag, is asked once, and
// only y or yes applies. Risk: a form that asks when nobody can answer, or
// applies on an answer that was not a yes (cli.environment.8,
// cli.environment.14, cli.look.6, cli.commands.1).
func TestEveryFormOfThePlanEndsOnItsWord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		apply, ask bool
		answers    []string
		code       int
		asked      int
	}{
		{name: "--apply applies and asks nothing", apply: true, code: exit.Done},
		{name: "--apply at a terminal asks nothing either", apply: true, ask: true, answers: []string{"n"}, code: exit.Done},
		{name: "off a terminal the plan prints and ends declined", code: exit.Declined},
		{name: "a yes at a terminal applies", ask: true, answers: []string{"y"}, code: exit.Done, asked: 1},
		{name: "Yes in any case and space applies", ask: true, answers: []string{"  Yes "}, code: exit.Done, asked: 1},
		{name: "a no declines", ask: true, answers: []string{"n"}, code: exit.Declined, asked: 1},
		{name: "anything else declines", ask: true, answers: []string{"apply"}, code: exit.Declined, asked: 1},
		{name: "the end of the input declines", ask: true, code: exit.Declined, asked: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pw := newPlanWorld(t, tc.answers...)

			code := pw.run(tc.apply, tc.ask)

			if code != tc.code {
				t.Fatalf("exit %d, want %d; said %q", code, tc.code, pw.w.Said)
			}
			if len(pw.w.Asked) != tc.asked || (tc.asked > 0 && pw.w.Asked[0] != question) {
				t.Errorf("asked %q, want %q %d times", pw.w.Asked, question, tc.asked)
			}
			applied := pw.file("rules/a.yaml") == "status: retired\n"
			if applied != (code == exit.Done) {
				t.Errorf("rules/a.yaml holds %q after exit %d", pw.file("rules/a.yaml"), code)
			}
			c := pw.closing()
			if code == exit.Done {
				if c.Word() != model.Done || !strings.HasPrefix(c.What(), "2 files written") {
					t.Errorf("closing %q %q, want done with what was written", c.Word(), c.What())
				}
				return
			}
			bl := pw.w.Shown[len(pw.w.Shown)-1].Blocks()
			if f, ok := bl[len(bl)-2].(model.DryRunFooter); !ok || f.Flag != "--apply" {
				t.Errorf("the block before the word is %+v, want the footer naming --apply", bl[len(bl)-2])
			}
			if c.Word() != model.Declined || c.Next() != "gragas test --apply" {
				t.Errorf("closing %q next %q, want declined on the apply line", c.Word(), c.Next())
			}
			if len(pw.runs()) != 0 {
				t.Errorf("a declined plan journalled %v", pw.runs())
			}
		})
	}

	t.Run("a plan with nothing to change is done and asks nothing", func(t *testing.T) {
		t.Parallel()
		pw := newPlanWorld(t, "y")
		pw.w.Seed("/work/rulebook", map[string]string{"identity.yaml": "name: demo\n", "rules/a.yaml": "status: retired\n", "history.jsonl": ""})
		if code := pw.run(false, true); code != exit.Done || len(pw.w.Asked) != 0 || pw.closing().What() != "nothing to do" {
			t.Errorf("exit %d, asked %q, closing %q", code, pw.w.Asked, pw.closing().What())
		}
	})
}

// TestAnInputChangedBetweenTheShowAndTheYesIsShownAndAskedAgain: while the
// person reads the plan another process changes an input. Their yes takes
// the lock and makes the plan again, which has another digest, so nothing is
// written: the new plan is shown with a line saying it changed and the
// question is asked again. A second yes applies the new plan under a run id
// of its own; a no after the re-show applies nothing at all. Risk: the
// first answer applied to a plan the person never saw (A44,
// cli.commands.22, cli.commands.17).
func TestAnInputChangedBetweenTheShowAndTheYesIsShownAndAskedAgain(t *testing.T) {
	t.Parallel()
	move := func(pw *planWorld) func() {
		return func() {
			if err := pw.w.Ports().Writer.Replace(pw.root, "identity.yaml", []byte("name: renamed\n")); err != nil {
				pw.t.Fatal(err)
			}
		}
	}
	changed := "The plan changed while you read it, and nothing was written. This is the plan now:"

	t.Run("a second yes applies the new plan", func(t *testing.T) {
		t.Parallel()
		pw := newPlanWorld(t, "y", "y")
		pw.moved = move(pw)

		code := pw.run(false, true)

		if code != exit.Done || len(pw.w.Asked) != 2 || !strings.Contains(strings.Join(pw.w.Said, " "), changed) {
			t.Fatalf("exit %d, asked %q, said %q; want done after a second question", code, pw.w.Asked, pw.w.Said)
		}
		runs := pw.runs()
		first, second := runs["run-1"], runs["run-1-2"]
		if len(runs) != 2 || len(first) != 1 || first[0].Kind != model.KindClose {
			t.Fatalf("the journal holds %v, want run-1's refusal alone and run-1-2", runs)
		}
		var detail struct{ Outcome, Agreed, Planned string }
		if err := json.Unmarshal(first[0].Detail, &detail); err != nil || detail.Outcome != "moved" || detail.Agreed == detail.Planned {
			t.Fatalf("run-1's close is %s (%v), want moved with two digests", first[0].Detail, err)
		}
		if second[0].Kind != model.KindIntent || second[0].Plan != detail.Planned {
			t.Errorf("run-1-2 applied plan %s, want the plan shown second, %s", second[0].Plan, detail.Planned)
		}
		if !strings.Contains(pw.file("history.jsonl"), detail.Planned) || strings.Contains(pw.file("history.jsonl"), detail.Agreed) {
			t.Errorf("history.jsonl holds %q, want the yes of the second plan alone", pw.file("history.jsonl"))
		}
	})

	t.Run("a no after the re-show applies nothing", func(t *testing.T) {
		t.Parallel()
		pw := newPlanWorld(t, "y", "n")
		pw.moved = move(pw)

		code := pw.run(false, true)

		if code != exit.Declined || len(pw.w.Asked) != 2 {
			t.Fatalf("exit %d, asked %q; want declined after a second question", code, pw.w.Asked)
		}
		if pw.file("rules/a.yaml") != "status: active\n" || pw.file("history.jsonl") != `{"n":1}`+"\n" {
			t.Errorf("the first yes was applied: %q, %q", pw.file("rules/a.yaml"), pw.file("history.jsonl"))
		}
		for id, recs := range pw.runs() {
			for _, r := range recs {
				if r.Kind != model.KindClose {
					t.Errorf("run %s journalled %s", id, r.Kind)
				}
			}
		}
	})
}

// TestTheDeclinedPlanAt80Columns: the plan off a terminal, as the fake
// terminal of 80 columns takes it: the plan, the footer naming --apply, and
// declined on the apply line. The golden is written only by go test
// ./internal/cli/root -run TestTheDeclinedPlanAt80Columns -update, by a
// person who reads the diff (cli.look.6, S43).
func TestTheDeclinedPlanAt80Columns(t *testing.T) {
	t.Parallel()
	pw := newPlanWorld(t)
	if code := pw.run(false, false); code != exit.Declined {
		t.Fatalf("exit %d, want declined", code)
	}
	if pw.w.Ports().Terminal.Width() != 80 || len(pw.w.Shown) != 1 {
		t.Fatalf("width %d, %d screens; want one screen at 80 columns", pw.w.Ports().Terminal.Width(), len(pw.w.Shown))
	}
	got := strings.Join(pw.w.Said, "\n") + said(pw.w.Shown[0])
	uitest.Golden(t, "plan-declined-80", got)
}

// TestEveryFormNamesAFlagItsCommandTakes: every call of newForm in this
// package names its flag as a constant, the flag is the one yes, and the
// constructor refuses a flag the asking command does not take; every command
// that writes takes it (cli.environment.14).
func TestEveryFormNamesAFlagItsCommandTakes(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range src.Decls {
			fn, isFn := d.(*ast.FuncDecl)
			if !isFn {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				id, isID := callee(call, ok)
				switch {
				case isID && id == "newForm":
					calls++
					// askFor hands its caller's flag on; its callers are held below.
					if flag, ok := call.Args[2].(*ast.Ident); !ok || (flag.Name != "applyFlag" && (flag.Name != "flag" || fn.Name.Name != "askFor")) {
						t.Errorf("%s: a form names its flag as %T, want the constant applyFlag", f, call.Args[2])
					}
				case isID && id == "askFor":
					calls++
					if lit, ok := call.Args[3].(*ast.BasicLit); !ok || !strings.HasPrefix(lit.Value, `"--`) {
						t.Errorf("%s: a question names its flag as a string such as \"--kind\"", f)
					}
				}
				return true
			})
		}
	}
	if calls == 0 {
		t.Fatal("no call of newForm was found")
	}

	takes := &cobra.Command{Use: "takes"}
	takes.Flags().Bool("apply", false, "Apply")
	if _, err := newForm(takes, "apply?", applyFlag); err != nil {
		t.Errorf("the form of a command that takes --apply: %v", err)
	}
	for _, flag := range []string{"--yes", "apply", "--"} {
		if _, err := newForm(takes, "apply?", flag); err == nil {
			t.Errorf("a form naming %q was built", flag)
		}
	}

	s := &session{rec: &recorder{}}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if _, ok := c.Annotations[writes]; ok && c.Flags().Lookup(strings.TrimPrefix(applyFlag, "--")) == nil {
			t.Errorf("%s writes and does not take %s", c.CommandPath(), applyFlag)
		}
		for _, k := range c.Commands() {
			walk(k)
		}
	}
	walk(s.newRoot(t.Context()))
}

// callee is the name a call calls, when it calls a plain identifier.
func callee(call *ast.CallExpr, ok bool) (string, bool) {
	if !ok {
		return "", false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "", false
	}
	return id.Name, true
}

// secondYes are the spellings of a second yes: a flag that would answer the
// form's question beside --apply, which cli.commands.11 and A42 refuse.
func secondYes() []string {
	return []string{
		"yes", "y", "assume-yes", "assumeyes", "force", "non-interactive", "noninteractive", "no-input",
		"no-prompt", "auto-approve", "confirm-all", "yes-all", "accept", "approve", "agree",
	}
}

// yesSpelling is why the flag named name, with its shorthand, is a second
// yes, or "".
func yesSpelling(name, shorthand string) string {
	switch {
	case slices.Contains(secondYes(), name):
		return "--" + name + " is a second yes"
	case shorthand == "y":
		return "-y is a second yes"
	}
	return ""
}

// TestTheOneYesIsApply: no command of the tree, and no flag defined anywhere
// in the repository's Go, is a second spelling of the yes: --apply is the
// one. The planted control is a command with --yes and one with -y, which
// the same check finds. Risk: a second flag for one act, which a script
// learns and a person does not (cli.commands.11, cli.environment.8, A42).
func TestTheOneYesIsApply(t *testing.T) {
	t.Parallel()
	treeYes := func(root *cobra.Command) []string {
		var found []string
		var walk func(c *cobra.Command)
		walk = func(c *cobra.Command) {
			visit := func(f *pflag.Flag) {
				if why := yesSpelling(f.Name, f.Shorthand); why != "" {
					found = append(found, c.CommandPath()+": "+why)
				}
			}
			c.Flags().VisitAll(visit)
			c.PersistentFlags().VisitAll(visit)
			for _, k := range c.Commands() {
				walk(k)
			}
		}
		walk(root)
		return found
	}
	s := &session{rec: &recorder{}}
	if found := treeYes(s.newRoot(t.Context())); len(found) != 0 {
		t.Errorf("the tree has a second yes: %q", found)
	}
	planted := &cobra.Command{Use: "planted"}
	child := &cobra.Command{Use: "child"}
	// The control names its flags through variables, so the source scan below
	// does not find them here.
	long, short := secondYes()[0], secondYes()[1]
	child.Flags().Bool(long, false, "Yes")
	other := &cobra.Command{Use: "other"}
	other.Flags().BoolP("quiet", short, false, "Quiet")
	planted.AddCommand(child, other)
	if found := treeYes(planted); len(found) != 2 {
		t.Errorf("the control found %q, want --yes and -y", found)
	}

	if found := sourceYes(t, filepath.Join("..", "..", "..")); len(found) != 0 {
		t.Errorf("the repository defines a second yes: %q", found)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "planted.go"), []byte("package p\n\nfunc f() { fs.BoolP(\"assume-yes\", \"\", false, \"\") }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if found := sourceYes(t, dir); len(found) != 1 {
		t.Errorf("the planted source found %q, want its --assume-yes", found)
	}
}

// flagDefiners are pflag's methods that define a flag, its name first, or
// after the variable for the Var forms.
func flagDefiners() []string {
	return []string{"Bool", "BoolP", "BoolVar", "BoolVarP", "String", "StringP", "StringVar", "StringVarP"}
}

// sourceYes is every flag definition under dir whose name is a second yes,
// read from the Go source, outside vendor and testdata.
func sourceYes(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == "testdata" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) && p != dir {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		src, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(src, func(n ast.Node) bool {
			if why := definesYes(n); why != "" {
				found = append(found, p+": "+why)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// definesYes is why the node n defines a second yes, or "".
func definesYes(n ast.Node) string {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !slices.Contains(flagDefiners(), sel.Sel.Name) {
		return ""
	}
	var lits []string
	for _, a := range call.Args {
		if b, ok := a.(*ast.BasicLit); ok && b.Kind == token.STRING {
			lits = append(lits, strings.Trim(b.Value, "\"`"))
		}
	}
	if len(lits) == 0 {
		return ""
	}
	short := ""
	if strings.HasSuffix(sel.Sel.Name, "P") && len(lits) > 1 {
		short = lits[1]
	}
	return yesSpelling(lits[0], short)
}
