package lifecycle

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// PlanBind plans a binding added to a rule the rulebook owns. A binding the
// rule holds already, at the same tool, target and key, plans nothing, so a
// second identical run ends "done, nothing to do".
func PlanBind(b model.Book, r model.BindRule, m Moment) (Plan, error) {
	rule, err := bindable(b, r.ID, "bind")
	if err != nil {
		return Plan{}, err
	}
	nb := r.Binding
	if err := checkBinding(rule, nb); err != nil {
		return Plan{}, err
	}
	if i := indexOfBinding(rule, nb.Tool, nb.Target, nb.Key); i >= 0 {
		if bindingRecord(rule.Bindings[i]) == bindingRecord(nb) {
			return nothing("bind", rule, fmt.Sprintf("%s holds the binding at %s already", r.ID, entryName(nb))), nil
		}
		return Plan{}, refuse("change it with gragas rebind "+r.ID+" "+nb.Tool+" "+string(nb.Target)+" "+nb.Key,
			"%s holds a binding at %s already, with other fields", r.ID, entryName(nb))
	}
	entity := "rule:" + r.ID
	before := lifecycleOf(rule)
	d := model.Delta{Changes: []model.Change{{Entity: entity, Field: "bindings{}", After: bindingRecord(nb)}}}
	bound := fmt.Sprintf("bind %s to %s, severity %s, rung %d, run at %s", r.ID, entryName(nb), nb.Severity, nb.Rung, nb.RunPoint)
	if len(nb.Options) > 0 {
		bound += ", options " + optionsText(nb.Options)
	}
	changes := []string{bound}
	written := []string{r.Files.Rules + ": adds a binding to " + r.ID, r.Files.History + ": appends one entry (bind)"}
	return Plan{Delta: d, Record: Record{Kind: "bind", Entity: entity, Before: &before, After: before}, Sections: sections(changes, written, "none")}, nil
}

// PlanRebind plans a change to one binding of a rule the rulebook owns: the
// binding at the entry the request names loses its record and takes the new
// one, in the same place. A request that leaves the binding as it is plans
// nothing. A weaker severity is a loosening, and a loosening stands on a
// decision (S70); the rung and the run point are not here (S34).
func PlanRebind(b model.Book, r model.RebindRule, m Moment) (Plan, error) {
	rule, err := bindable(b, r.ID, "rebind")
	if err != nil {
		return Plan{}, err
	}
	i := indexOfBinding(rule, r.Tool, r.Target, r.Key)
	if i < 0 {
		return Plan{}, refuse("list the bindings of the rule with gragas explain "+r.ID, "%s holds no binding at %s", r.ID, entryName(model.Binding{Tool: r.Tool, Target: r.Target, Key: r.Key}))
	}
	old := rule.Bindings[i]
	next := old
	var words []string
	set := func(field string, changed bool, from, to any) {
		if changed {
			words = append(words, fmt.Sprintf("%s: %v becomes %v", field, from, to))
		}
	}
	if r.ToTool != nil {
		set("tool", *r.ToTool != old.Tool, old.Tool, *r.ToTool)
		next.Tool = *r.ToTool
	}
	if r.ToTarget != nil {
		set("target", *r.ToTarget != old.Target, old.Target, *r.ToTarget)
		next.Target = *r.ToTarget
	}
	if r.ToKey != nil {
		set("key", *r.ToKey != old.Key, old.Key, *r.ToKey)
		next.Key = *r.ToKey
	}
	if r.Value != nil {
		set("value", fmt.Sprint(*r.Value) != fmt.Sprint(old.Value), old.Value, *r.Value)
		next.Value = *r.Value
	}
	if r.Options != nil {
		next.Options = reOptions(r.Options, old.Options, set)
	}
	if r.Severity != nil {
		set("severity", *r.Severity != old.Severity, old.Severity, *r.Severity)
		next.Severity = *r.Severity
	}
	if r.Remedy != nil {
		set("remedy", *r.Remedy != old.Remedy, old.Remedy, *r.Remedy)
		next.Remedy = *r.Remedy
	}
	inPlace, err := checkRebind(rule, r, old, next)
	if err != nil {
		return Plan{}, err
	}
	if len(words) == 0 {
		return nothing("rebind", rule, fmt.Sprintf("the binding of %s at %s is as asked already", r.ID, entryName(old))), nil
	}
	if j := indexOfBinding(rule, next.Tool, next.Target, next.Key); j >= 0 && j != i {
		return Plan{}, refuse("unbind that one first, or choose another entry", "%s holds a binding at %s already", r.ID, entryName(next))
	}
	entity := "rule:" + r.ID
	var d model.Delta
	var written []string
	slug := ""
	if field := loosens(r, old, next); field != "" {
		if r.Why == nil {
			return Plan{}, refuse("give the decision with --decision <slug>, or take one with --decision-title and --decision-text",
				"the %s of the binding of %s changes (%s) in a way that loosens it or cannot be compared, and a loosening stands on a decision (S70)", field, r.ID, strings.Join(words, "; "))
		}
		if err := checkWhy(b, *r.Why, "rebind", r.ID); err != nil {
			return Plan{}, err
		}
		slug = decisionSlug(*r.Why, "rebind", r.ID)
		wd, wc, ww := whyChanges(*r.Why, slug, m, r.Files)
		d.Changes = append(d.Changes, wd.Changes...)
		words = append(words, wc...)
		written = append(written, ww...)
	}
	d.Changes = append(d.Changes, rebindChanges(entity, old, next, inPlace)...)
	before := lifecycleOf(rule)
	changes := append([]string{fmt.Sprintf("rebind %s at %s", r.ID, entryName(old))}, words...)
	written = append([]string{r.Files.Rules + ": changes a binding of " + r.ID}, written...)
	written = append(written, r.Files.History+": appends one entry (rebind)")
	return Plan{Delta: d, Record: Record{Kind: "rebind", Entity: entity, Before: &before, After: before, Decision: slug},
		Sections: sections(changes, written, "none")}, nil
}

// PlanUnbind plans a binding taken out of a rule the rulebook owns. The rule
// stays, and its standing is derived again from the bindings it keeps. A
// binding the rule does not hold plans nothing.
func PlanUnbind(b model.Book, r model.UnbindRule, m Moment) (Plan, error) {
	rule, err := bindable(b, r.ID, "unbind")
	if err != nil {
		return Plan{}, err
	}
	i := indexOfBinding(rule, r.Tool, r.Target, r.Key)
	here := model.Binding{Tool: r.Tool, Target: r.Target, Key: r.Key}
	if i < 0 {
		return nothing("unbind", rule, fmt.Sprintf("%s holds no binding at %s", r.ID, entryName(here))), nil
	}
	entity := "rule:" + r.ID
	before := lifecycleOf(rule)
	d := model.Delta{Changes: []model.Change{{Entity: entity, Field: "bindings{}", Before: bindingRecord(rule.Bindings[i])}}}
	changes := []string{fmt.Sprintf("unbind %s from %s", r.ID, entryName(here))}
	written := []string{r.Files.Rules + ": takes a binding out of " + r.ID, r.Files.History + ": appends one entry (unbind)"}
	return Plan{Delta: d, Record: Record{Kind: "unbind", Entity: entity, Before: &before, After: before}, Sections: sections(changes, written, "none")}, nil
}

const (
	optionsHint = "an option holds one value or a list of values; write a mapping inside one in the rule's file by hand"
	valueHint   = "the rule's record editor carries single values and lists only; write the value in the rule's file by hand"
)

// bindable is the rule a binding command may change: one the rulebook owns.
// A borrowed rule's bindings are the guard's (S70), which is not here yet.
func bindable(b model.Book, id, verb string) (model.Rule, error) {
	rule, err := ruleOf(b, id)
	if err != nil {
		return model.Rule{}, err
	}
	if rule.Status == model.Retired {
		return model.Rule{}, refuse("bring it back with gragas rule restore "+id, "%s is retired, and a retired rule takes no %s", id, verb)
	}
	if rule.Origin == model.Imported {
		return model.Rule{}, refuse("an importer tightens an imported rule through a binding of its own, which is not built yet",
			"%s is an imported rule, and %s does not change a borrowed rule's bindings", id, verb)
	}
	return rule, nil
}

// checkBinding holds a binding to the schema's required fields and the closed
// sets, naming the field that is wrong and the command that says more.
func checkBinding(rule model.Rule, b model.Binding) error {
	id := string(rule.ID)
	switch {
	case b.Tool == "" || !validSlug(b.Tool):
		return refuse("name the tool by its slug, as gragas tool list shows", "the tool %q of the binding is not a slug", b.Tool)
	case !slices.Contains(model.TargetTypes(), b.Target):
		return refuse("choose one of "+joinTargets(), "the target %q of the binding is not one of %s", b.Target, joinTargets())
	case b.Key == "" || !clean(b.Key):
		return refuse("name the tool's entry with --key", "the key of the binding of %s is empty or holds a control character", id)
	case !slices.Contains(model.Severities(), b.Severity):
		return refuse("choose error, warning or info", "the severity %q of the binding is not one of error, warning, info", b.Severity)
	case b.Rung < 0 || b.Rung > model.MaxRung:
		return refuse("choose a rung from 0 to 6", "the rung %d of the binding is not from 0 to %d", b.Rung, model.MaxRung)
	case !slices.Contains(model.RunPoints(), b.RunPoint):
		return refuse("choose one of edit, commit, push, land, review, release, deploy", "the run point %q of the binding is not one of the seven", b.RunPoint)
	case b.Pair < 0:
		return refuse("give the number of one of the rule's pairs", "the pair %d of the binding is not a number from 1", b.Pair)
	case b.Pair > 0 && !slices.ContainsFunc(rule.Pairs, func(p model.Pair) bool { return p.Number == b.Pair }):
		return refuse("add the pair first with gragas rule edit "+id, "%s has no pair %d", id, b.Pair)
	}
	if _, ok := scalarOrList(b.Value); !ok {
		return refuse(valueHint, "the value of the binding is not a single value or a list")
	}
	return checkOptions(b.Options)
}

// checkOptions holds each option of a binding to a clean name and one value or
// a list of values, which the record editor can write; a mapping inside an
// option is for the file's own hand.
func checkOptions(o map[string]any) error {
	for _, k := range sortedKeys(o) {
		if k == "" || !clean(k) {
			return refuse("name each option with a word", "an option of the binding has a name that is empty or holds a control character")
		}
		if s, ok := scalarOrList(o[k]); !ok || s == "" {
			return refuse(optionsHint, "the option %q of the binding is not a single value or a list", k)
		}
	}
	return nil
}

// optionsText is the options as one line for a plan, name and value in the
// order of the names; no options read as none.
func optionsText(o map[string]any) string {
	if len(o) == 0 {
		return "none"
	}
	var parts []string
	for _, k := range sortedKeys(o) {
		s, _ := scalarOrList(o[k])
		parts = append(parts, scalar(k)+": "+s)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func joinTargets() string {
	var out []string
	for _, t := range model.TargetTypes() {
		out = append(out, string(t))
	}
	return strings.Join(out, ", ")
}

func indexOfBinding(rule model.Rule, tool string, target model.TargetType, key string) int {
	return slices.IndexFunc(rule.Bindings, func(x model.Binding) bool { return x.Tool == tool && x.Target == target && x.Key == key })
}

func entryName(b model.Binding) string { return fmt.Sprintf("%s %s %s", b.Tool, b.Target, b.Key) }

// loosens names the field by which the change from old to next makes the
// binding report less (S70), or "" when it does not: a move to another entry
// leaves the old one weaker; a severity below the old (error, then warning,
// then info); a value the tool's scale ranks below the old, or one it cannot
// rank, which stands on a decision as a loosening does.
func loosens(r model.RebindRule, old, next model.Binding) string {
	rank := func(s model.Severity) int { return slices.Index(model.Severities(), s) }
	switch {
	case next.Tool != old.Tool || next.Target != old.Target || next.Key != old.Key:
		return "entry"
	case rank(next.Severity) > rank(old.Severity):
		return "severity"
	case fmt.Sprint(next.Value) != fmt.Sprint(old.Value) && (r.Loosens == nil || r.Loosens(old, next)):
		return "value"
	case fmt.Sprint(next.Options) != fmt.Sprint(old.Options) && (len(next.Options) > 0 || len(old.Options) > 0):
		return "options" // no scale ranks an option, so a change of one stands on a decision
	}
	return ""
}

// scalarOrList says a value is a single value, a list of single values, or
// nothing: the shapes a rule's record carries.
func scalarOrList(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", true
	case bool:
		return strconv.FormatBool(x), true
	case int, int64:
		return fmt.Sprint(x), true
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), true
	case string:
		return scalar(x), true
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			s, ok := scalarOrList(e)
			if !ok || s == "" {
				return "", false
			}
			parts[i] = s
		}
		return "[" + strings.Join(parts, ", ") + "]", true
	case []string:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = scalar(e)
		}
		return "[" + strings.Join(parts, ", ") + "]", true
	}
	return "", false
}

// checkRebind holds the binding a rebind makes to the schema's fields, and
// says whether it is changed in place. A rebind checks what it changes: the
// options of a request are judged, but a value it does not set and the old
// binding's own options are not judged again. A binding that holds options the
// record cannot carry (nested ones, or a value that is no single value or
// list), or options and a request that does not set them, is changed where it
// stands, key by key, so those keys stay as written; a request
// that sets options on one that holds hand-written ones is refused, and so is a
// value or remedy added to or taken out of it, as the record cannot make that.
func checkRebind(rule model.Rule, r model.RebindRule, old, next model.Binding) (inPlace bool, err error) {
	judged := next
	if r.Options == nil {
		judged.Options = nil
	}
	if r.Value == nil {
		judged.Value = nil
	}
	if err := checkBinding(rule, judged); err != nil {
		return false, err
	}
	byHand := checkOptions(old.Options) != nil || !scalarValue(old.Value)
	if byHand && r.Options != nil {
		return false, refuse(optionsHint, "the binding of %s at %s holds options or a value written by hand, and a rebind cannot set its options", rule.ID, entryName(old))
	}
	if !byHand && (r.Options != nil || len(old.Options) == 0) {
		return false, nil
	}
	if (bindingText(old.Value) == "") != (bindingText(next.Value) == "") || (old.Remedy == "") != (next.Remedy == "") {
		return false, refuse(optionsHint, "the binding of %s at %s holds options written by hand, and a rebind of it cannot add or take out its value or remedy", rule.ID, entryName(old))
	}
	return true, nil
}

// rebindChanges are the changes that make old into next: one record changed in
// place, or the old record taken out and the new one added.
func rebindChanges(entity string, old, next model.Binding, inPlace bool) []model.Change {
	if inPlace {
		// the options stay as the file holds them, so the record leaves them out
		old.Options, next.Options = nil, nil
		return []model.Change{{Entity: entity, Field: "bindings{}", Before: bindingRecord(old), After: bindingRecord(next)}}
	}
	return []model.Change{
		{Entity: entity, Field: "bindings{}", Before: bindingRecord(old)},
		{Entity: entity, Field: "bindings{}", After: bindingRecord(next)}}
}

// scalarValue says a binding's value is one the record carries.
func scalarValue(v any) bool {
	_, ok := scalarOrList(v)
	return ok
}

// bindingText is the text of a value the record carries, "" for none.
func bindingText(v any) string {
	s, _ := scalarOrList(v)
	return s
}

// bindingRecord is a binding as the YAML mapping of its keys in the order the
// schema lists them, as the rule's file holds it: format adds this record, or
// takes out the one whose keys and texts are these.
func bindingRecord(b model.Binding) string {
	var out strings.Builder
	line := func(k, v string) { out.WriteString(k + ": " + v + "\n") }
	line("tool", scalar(b.Tool))
	line("target", scalar(string(b.Target)))
	line("key", scalar(b.Key))
	if v, _ := scalarOrList(b.Value); v != "" {
		line("value", v)
	}
	if len(b.Options) > 0 {
		out.WriteString("options:\n")
		for _, k := range sortedKeys(b.Options) {
			s, _ := scalarOrList(b.Options[k])
			out.WriteString("  " + scalar(k) + ": " + s + "\n")
		}
	}
	line("severity", scalar(string(b.Severity)))
	line("rung", strconv.Itoa(int(b.Rung)))
	line("run_point", scalar(string(b.RunPoint)))
	if b.Remedy != "" {
		line("remedy", scalar(b.Remedy))
	}
	if b.Pair > 0 {
		line("pair", strconv.Itoa(b.Pair))
	}
	if b.PairPath != "" {
		line("pair_path", scalar(b.PairPath))
	}
	if b.Tightens != "" {
		line("tightens", scalar(string(b.Tightens)))
	}
	if b.Checker != "" {
		line("checker", scalar(b.Checker))
	}
	return out.String()
}

// reOptions is the options a rebind leaves, none when it gives none, and says
// the change through set.
func reOptions(want, old map[string]any, set func(string, bool, any, any)) map[string]any {
	set("options", fmt.Sprint(want) != fmt.Sprint(old) && (len(want) > 0 || len(old) > 0), optionsText(old), optionsText(want))
	if len(want) == 0 {
		return nil
	}
	return want
}

// sortedKeys are the names of o in order.
func sortedKeys(o map[string]any) []string {
	var ks []string
	for k := range o {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}
