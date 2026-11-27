package lifecycle_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

func lintBinding(sev model.Severity) model.Binding {
	return model.Binding{Tool: "lint-a", Target: model.TargetType("config"), Key: "wrap-errors", Value: true, Severity: sev, Rung: 1, RunPoint: "commit"}
}

func bound(sev model.Severity) model.Book {
	return ready(model.Must, model.Proposed, func(r *model.Rule) { r.Bindings = []model.Binding{lintBinding(sev)} })
}

func refusalOf(t *testing.T, err error) *lifecycle.Refusal {
	t.Helper()
	var r *lifecycle.Refusal
	if !errors.As(err, &r) {
		t.Fatalf("err = %v, want a refusal", err)
	}
	return r
}

// TestBindAddsOneRecordAndASecondRunHasNothingToDo, with the refusals that
// name the field. Risk: a binding written twice, or a bad field that reaches the
// rule's file.
func TestBindAddsOneRecordAndASecondRunHasNothingToDo(t *testing.T) {
	t.Parallel()
	req := model.BindRule{ID: "go.errors.9", Binding: lintBinding(model.SeverityError), Files: files()}
	p, err := lifecycle.PlanBind(ready(model.Must, model.Proposed), req, moment())
	if err != nil || len(p.Delta.Changes) != 1 || p.Delta.Changes[0].Field != "bindings{}" || p.Record.Kind != "bind" {
		t.Fatalf("plan = %+v, err = %v", p, err)
	}
	if want := "tool: lint-a\ntarget: config\nkey: wrap-errors\nvalue: true\nseverity: error\nrung: 1\nrun_point: commit\n"; p.Delta.Changes[0].After != want {
		t.Errorf("record:\n%s\nwant:\n%s", p.Delta.Changes[0].After, want)
	}
	again, err := lifecycle.PlanBind(bound(model.SeverityError), req, moment())
	if err != nil || len(again.Delta.Changes) != 0 {
		t.Errorf("a second bind plans %+v (%v), want nothing", again.Delta, err)
	}
	for name, edit := range map[string]func(*model.BindRule){
		"severity": func(r *model.BindRule) { r.Binding.Severity = "loud" },
		"target":   func(r *model.BindRule) { r.Binding.Target = "nowhere" },
		"key":      func(r *model.BindRule) { r.Binding.Key = "" },
		"pair":     func(r *model.BindRule) { r.Binding.Pair = 7 },
		"options":  func(r *model.BindRule) { r.Binding.Options = map[string]any{"a": map[string]any{"b": 1}} },
	} {
		bad := req
		edit(&bad)
		_, err := lifecycle.PlanBind(ready(model.Must, model.Proposed), bad, moment())
		if r := refusalOf(t, err); !strings.Contains(r.Msg, name) && name != "options" || r.Hint == "" {
			t.Errorf("%s: refusal %+v does not name the field with a hint", name, r)
		}
	}
	other := req
	other.Binding.Severity = model.SeverityWarning
	if _, err := lifecycle.PlanBind(bound(model.SeverityError), other, moment()); refusalOf(t, err).Hint == "" {
		t.Error("a bind over a binding with other fields names no rebind")
	}
}

// TestRebindLoosensOnlyOnADecisionAndUnbindTakesTheRecordOut: a stricter
// severity needs none, a weaker one is refused naming the field until a
// decision is cited, and unbind of a binding the rule lacks has nothing to do.
func TestRebindLoosensOnlyOnADecisionAndUnbindTakesTheRecordOut(t *testing.T) {
	t.Parallel()
	warn, errSev := model.SeverityWarning, model.SeverityError
	re := func(sev *model.Severity, why *model.Why) model.RebindRule {
		return model.RebindRule{ID: "go.errors.9", Tool: "lint-a", Target: "config", Key: "wrap-errors", Severity: sev, Why: why, Files: files()}
	}
	p, err := lifecycle.PlanRebind(bound(model.SeverityWarning), re(&errSev, nil), moment())
	if err != nil || len(p.Delta.Changes) != 2 || p.Delta.Changes[0].Before == "" || p.Delta.Changes[1].After == "" {
		t.Fatalf("a tightening plans %+v (%v), want a record out and a record in", p.Delta, err)
	}
	_, err = lifecycle.PlanRebind(bound(model.SeverityError), re(&warn, nil), moment())
	if r := refusalOf(t, err); !strings.Contains(r.Msg, "severity") || !strings.Contains(r.Msg, "decision") {
		t.Errorf("a loosening ends %q, want the field and the decision", r.Msg)
	}
	p, err = lifecycle.PlanRebind(bound(model.SeverityError), re(&warn, &model.Why{Cite: "taken"}), moment())
	if err != nil || p.Record.Decision != "taken" {
		t.Errorf("a loosening on a decision: %+v (%v)", p.Record, err)
	}
	same, err := lifecycle.PlanRebind(bound(model.SeverityError), re(&errSev, nil), moment())
	if err != nil || len(same.Delta.Changes) != 0 {
		t.Errorf("a rebind to what it is plans %+v (%v)", same.Delta, err)
	}
	un := model.UnbindRule{ID: "go.errors.9", Tool: "lint-a", Target: "config", Key: "wrap-errors", Files: files()}
	p, err = lifecycle.PlanUnbind(bound(model.SeverityError), un, moment())
	if err != nil || len(p.Delta.Changes) != 1 || p.Delta.Changes[0].Before != bindingBefore() || p.Record.Kind != "unbind" {
		t.Errorf("unbind plans %+v (%v)", p.Delta, err)
	}
	p, err = lifecycle.PlanUnbind(ready(model.Must, model.Proposed), un, moment())
	if err != nil || len(p.Delta.Changes) != 0 {
		t.Errorf("unbind of nothing plans %+v (%v)", p.Delta, err)
	}
	imp := ready(model.Must, model.Proposed, func(r *model.Rule) { r.Origin = model.Imported })
	if _, err := lifecycle.PlanUnbind(imp, un, moment()); refusalOf(t, err).Hint == "" {
		t.Error("an imported rule is unbound with no refusal")
	}
}

func bindingBefore() string {
	return "tool: lint-a\ntarget: config\nkey: wrap-errors\nvalue: true\nseverity: error\nrung: 1\nrun_point: commit\n"
}

// TestRebindOfAValueOrAnEntryStandsOnADecisionWhenItLoosensOrCannotBeCompared:
// a value the caller's scale ranks lower, a value it cannot rank (no scale at
// all), and a move to another entry each end refused naming the field until a
// decision is cited; a value the scale ranks higher plans with none. Risk: a
// rebind that turns an entry off, or moves it, with no decision (S70).
func TestRebindOfAValueOrAnEntryStandsOnADecisionWhenItLoosensOrCannotBeCompared(t *testing.T) {
	t.Parallel()
	off, on, other := any(false), any(true), "other-key"
	weaker := func(old, next model.Binding) bool { return next.Value == false && old.Value != false }
	for _, tc := range []struct {
		name    string
		value   *any
		toKey   *string
		loosens func(old, next model.Binding) bool
		field   string
	}{
		{"turned off", &off, nil, weaker, "value"},
		{"no scale, changed value", &off, nil, nil, "value"},
		{"moved to another entry", nil, &other, weaker, "entry"},
		{"ranked higher", &on, nil, weaker, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			book := ready(model.Must, model.Proposed, func(r *model.Rule) {
				b := lintBinding(model.SeverityError)
				b.Value = tc.value != &on // a tightening starts from off
				r.Bindings = []model.Binding{b}
			})
			req := model.RebindRule{ID: "go.errors.9", Tool: "lint-a", Target: "config", Key: "wrap-errors", Value: tc.value, ToKey: tc.toKey, Loosens: tc.loosens, Files: files()}
			_, err := lifecycle.PlanRebind(book, req, moment())
			if tc.field == "" {
				if err != nil {
					t.Fatalf("a tightening ends %v, want a plan", err)
				}
				return
			}
			if r := refusalOf(t, err); !strings.Contains(r.Msg, tc.field) || !strings.Contains(r.Msg, "decision") {
				t.Fatalf("ends %q, want the field %s and the decision", r.Msg, tc.field)
			}
			req.Why = &model.Why{Cite: "taken"}
			if p, err := lifecycle.PlanRebind(book, req, moment()); err != nil || p.Record.Decision != "taken" {
				t.Errorf("on a decision: %+v (%v)", p.Record, err)
			}
		})
	}
}

// TestBindWritesOptionsAsAMappingAndASecondRunHasNothingToDo: the options of a
// binding are in the record the plan writes, in the order of their names, a
// bind of the same options plans nothing, and a rebind of them that changes one
// stands on a decision, as no scale ranks an option.
func TestBindWritesOptionsAsAMappingAndASecondRunHasNothingToDo(t *testing.T) {
	t.Parallel()
	b := lintBinding(model.SeverityError)
	b.Options = map[string]any{"skip": []any{"a", "b"}, "max": int64(3)}
	req := model.BindRule{ID: "go.errors.9", Binding: b, Files: files()}
	p, err := lifecycle.PlanBind(ready(model.Must, model.Proposed), req, moment())
	if err != nil || len(p.Delta.Changes) != 1 {
		t.Fatalf("plan = %+v, err = %v", p, err)
	}
	if want := "value: true\noptions:\n  max: 3\n  skip: [a, b]\nseverity: error\n"; !strings.Contains(p.Delta.Changes[0].After, want) {
		t.Errorf("record:\n%s\nwant it to hold:\n%s", p.Delta.Changes[0].After, want)
	}
	held := ready(model.Must, model.Proposed, func(r *model.Rule) { r.Bindings = []model.Binding{b} })
	if again, err := lifecycle.PlanBind(held, req, moment()); err != nil || len(again.Delta.Changes) != 0 {
		t.Errorf("a second bind plans %+v (%v), want nothing", again.Delta, err)
	}
	re := model.RebindRule{ID: "go.errors.9", Tool: "lint-a", Target: "config", Key: "wrap-errors", Options: map[string]any{"max": int64(4)}, Files: files()}
	if _, err := lifecycle.PlanRebind(held, re, moment()); refusalOf(t, err).Hint == "" {
		t.Error("a rebind that changes an option with no decision names no way on")
	}
}

// TestARebindChecksOnlyWhatItChanges: a binding that holds nested
// options written by hand is rebound to a stricter severity as one change in
// place, with no refusal for the options it leaves as they are; a request that
// sets options is still refused, naming them.
func TestARebindChecksOnlyWhatItChanges(t *testing.T) {
	t.Parallel()
	nested := lintBinding(model.SeverityWarning)
	nested.Options = map[string]any{"ignore": map[string]any{"tests": []any{"a", "b"}}, "depth": 2}
	book := ready(model.Must, model.Proposed, func(r *model.Rule) { r.Bindings = []model.Binding{nested} })
	errSev := model.SeverityError
	req := model.RebindRule{ID: "go.errors.9", Tool: "lint-a", Target: "config", Key: "wrap-errors", Severity: &errSev, Files: files()}
	p, err := lifecycle.PlanRebind(book, req, moment())
	if err != nil || len(p.Delta.Changes) != 1 {
		t.Fatalf("plan = %+v, err = %v; want one change", p.Delta, err)
	}
	c := p.Delta.Changes[0]
	if c.Field != "bindings{}" || !strings.Contains(c.Before, "severity: warning\n") || !strings.Contains(c.After, "severity: error\n") || strings.Contains(c.After, "options") {
		t.Errorf("change = %+v, want one record in place, severity warning to error, the options not in it", c)
	}
	changed := req
	changed.Options = map[string]any{"depth": 3}
	_, err = lifecycle.PlanRebind(book, changed, moment())
	if r := refusalOf(t, err); !strings.Contains(r.Msg, "options") {
		t.Errorf("a change to the options: refusal %+v does not name them", r)
	}
	remedy := "wrap it"
	withRemedy := req
	withRemedy.Remedy = &remedy
	if _, err := lifecycle.PlanRebind(book, withRemedy, moment()); refusalOf(t, err).Hint == "" {
		t.Error("a remedy added to a binding with hand-written options is not refused with a hint")
	}
}

// TestARebindJudgesTheOptionsItSets: an option a rebind sets that is no single
// value or list is refused, naming the option, on a binding with no options.
// Risk: the request's options dropped from the check, so a mapping reaches the
// record editor.
func TestARebindJudgesTheOptionsItSets(t *testing.T) {
	t.Parallel()
	book := ready(model.Must, model.Proposed, func(r *model.Rule) { r.Bindings = []model.Binding{lintBinding(model.SeverityError)} })
	req := model.RebindRule{ID: "go.errors.9", Tool: "lint-a", Target: "config", Key: "wrap-errors", Files: files(),
		Options: map[string]any{"ignore": map[string]any{"tests": "a"}}}
	_, err := lifecycle.PlanRebind(book, req, moment())
	if r := refusalOf(t, err); !strings.Contains(r.Msg, "ignore") {
		t.Errorf("refusal %+v does not name the option", r)
	}
}
