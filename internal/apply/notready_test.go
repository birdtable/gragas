package apply_test

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/model"
)

// TestAPlanErrorACommandTypedNotReadyEndsNotReady: a plan that returns a
// *NotReadyError, bare or wrapped, ends on the closing word "not ready" with
// its reason and the command to type, and writes nothing. Risk: a refusal a
// command's registry row declares ending on failed (plan).
func TestAPlanErrorACommandTypedNotReadyEndsNotReady(t *testing.T) {
	t.Parallel()
	typed := &apply.NotReadyError{Reason: "the rulebook is in a format this engine cannot read"}
	for name, err := range map[string]error{"bare": typed, "wrapped": fmt.Errorf("while planning: %w", typed)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec()
			s.Plan = func(context.Context) (model.Plan, error) { return model.Plan{}, err }
			o := f.run(s)
			if o.Word != model.NotReady || o.Refusal == nil || o.Failure != nil {
				t.Fatalf("outcome %+v, want not ready with a refusal and no failure", o)
			}
			if !strings.Contains(o.Refusal.Observed(), typed.Reason) || o.Refusal.Hint() != s.Command {
				t.Errorf("refusal says %q and hints %q", o.Refusal.Observed(), o.Refusal.Hint())
			}
			if len(f.w.Writes) != 0 {
				t.Errorf("wrote %v, want nothing", f.w.Writes)
			}
		})
	}
}

// TestAPlainPlanErrorStillEndsFailedPlan: an error with the same words as a
// not-ready refusal, but not typed, is a failed plan. apply never guesses
// from text.
func TestAPlainPlanErrorStillEndsFailedPlan(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := f.plan(edit{path: "rules/a.yaml", data: "status: retired\n"}).spec()
	plain := errors.New("not ready: the rulebook is in a format this engine cannot read")
	s.Plan = func(context.Context) (model.Plan, error) { return model.Plan{}, plain }
	o := f.run(s)
	if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailPlan || !errors.Is(o.Cause, plain) {
		t.Fatalf("outcome %+v, want failed (plan) with the cause kept", o)
	}
}

// TestApplyMatchesNoErrorText: a tree test. Nothing in apply's own code
// reads an error's text to decide how a run ends: no strings function is
// called on an Error() result.
func TestApplyMatchesNoErrorText(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(src, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := c.Fun.(*ast.SelectorExpr); !ok || !isIdent(sel.X, "strings") {
				return true
			}
			for _, a := range c.Args {
				ast.Inspect(a, func(m ast.Node) bool {
					if call, ok := m.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" && len(call.Args) == 0 {
							t.Errorf("%s calls a strings function on an Error() text", name)
						}
					}
					return true
				})
			}
			return true
		})
	}
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}
