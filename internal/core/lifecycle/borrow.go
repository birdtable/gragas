package lifecycle

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// Lent is a rule a rulebook borrows: a parent's rule in an overlay, or an
// imported one (S34, S70). Its bindings are the lender's; the rulebook's own
// binding of it is a record of bindings/, under the rulebook's prefix, whose
// subject is the rule's id.
type Lent struct {
	// From names the lender in a sentence: the parent or the import, by its
	// rulebook and version.
	From string
	// Imported is true for an imported rule and false for a parent's.
	Imported bool
	// Rule is the rule as the lender holds it, with its bindings.
	Rule model.Rule
	// Records are the rulebook's records of the rule, in path order.
	Records []model.Tightening
}

// Compare is S34's guard of one borrowed binding: the binding was as the
// rulebook holds it now, next as a plan would leave it. same is a change
// that changes nothing; field names the field the change is held back by,
// "" when it tightens, and why says how. The operations hand in link's.
type Compare func(was, next model.Binding) (same bool, field, why string)

// The fields Compare names, as link names them.
const (
	againstEntry    = "entry"
	againstRung     = "rung"
	againstRunPoint = "run_point"
)

// PlanBindBorrowed plans a binding of a borrowed rule: a record under the
// rulebook's own prefix, in bindings/, with the rule's id as its subject. A
// binding at an entry the rule holds no binding at adds one, and adding is
// tightening; at an entry it holds, the binding is held to the one there by
// cmp, and only a tightening is planned. The same binding plans nothing.
func PlanBindBorrowed(b model.Book, l Lent, r model.BindRule, cmp Compare, m Moment) (Plan, error) {
	id := string(l.Rule.ID)
	if err := lendable(l, "bind"); err != nil {
		return Plan{}, err
	}
	nb := r.Binding
	nb.Tightens = ""
	if err := checkBorrowed(l.Rule, nb); err != nil {
		return Plan{}, err
	}
	was, rec, held := effective(l, nb.Tool, nb.Target, nb.Key)
	if held {
		if err := holdsLender(l, nb, cmp); err != nil {
			return Plan{}, err
		}
		same, field, why := cmp(was, nb)
		if field != "" {
			return Plan{}, against(l, field, why)
		}
		if same {
			return nothing("bind", l.Rule, fmt.Sprintf("%s is held at %s as asked already", id, entryName(nb))), nil
		}
	}
	return borrowedRecord(l, "bind", fmt.Sprintf("bind %s, which %s lends, to %s, severity %s, rung %d, run at %s", id, l.From, entryName(nb), nb.Severity, nb.Rung, nb.RunPoint),
		recordFor(b, l, rec), nb, r.Files)
}

// PlanRebindBorrowed plans a change to the binding a borrowed rule holds at
// one entry, as the rulebook holds it: its record, else the lender's. The
// changed binding is held to the one there by cmp, and only a tightening is
// planned, as the rulebook's record; a move to another entry is refused.
func PlanRebindBorrowed(b model.Book, l Lent, r model.RebindRule, cmp Compare, m Moment) (Plan, error) {
	id := string(l.Rule.ID)
	if err := lendable(l, "rebind"); err != nil {
		return Plan{}, err
	}
	was, rec, held := effective(l, r.Tool, r.Target, r.Key)
	if !held {
		return Plan{}, refuse("list the bindings of the rule with gragas explain "+id,
			"%s holds no binding at %s", id, entryName(model.Binding{Tool: r.Tool, Target: r.Target, Key: r.Key}))
	}
	next := was
	next.Tightens = ""
	if r.ToTool != nil {
		next.Tool = *r.ToTool
	}
	if r.ToTarget != nil {
		next.Target = *r.ToTarget
	}
	if r.ToKey != nil {
		next.Key = *r.ToKey
	}
	if r.Value != nil {
		next.Value = *r.Value
	}
	if r.Options != nil {
		next.Options = r.Options
	}
	if r.Severity != nil {
		next.Severity = *r.Severity
	}
	if r.Remedy != nil {
		next.Remedy = *r.Remedy
	}
	if err := checkBorrowed(l.Rule, next); err != nil {
		return Plan{}, err
	}
	if err := holdsLender(l, next, cmp); err != nil {
		return Plan{}, err
	}
	same, field, why := cmp(was, next)
	if field != "" {
		return Plan{}, against(l, field, why)
	}
	if same {
		return nothing("rebind", l.Rule, fmt.Sprintf("the binding of %s at %s is as asked already", id, entryName(was))), nil
	}
	return borrowedRecord(l, "rebind", fmt.Sprintf("rebind %s, which %s lends, at %s", id, l.From, entryName(was)), recordFor(b, l, rec), next, r.Files)
}

// PlanUnbindBorrowed refuses: a borrowed rule's binding is the lender's, and
// taking one out would weaken it (S34). The way to stop holding the rule is
// the hint.
func PlanUnbindBorrowed(l Lent, r model.UnbindRule) (Plan, error) {
	if err := lendable(l, "unbind"); err != nil {
		return Plan{}, err
	}
	return Plan{}, refuse(leaveHint(l),
		"%s is %s's rule, and unbinding it at %s would weaken it: a borrowed rule keeps every binding it is lent (S34)",
		l.Rule.ID, l.From, entryName(model.Binding{Tool: r.Tool, Target: r.Target, Key: r.Key}))
}

// holdsLender refuses a binding that weakens the lender's own at its entry,
// whatever the rulebook's last record there says: a stale or weak record is
// not what a borrowed rule is held to (S34).
func holdsLender(l Lent, next model.Binding, cmp Compare) error {
	i := indexOfBinding(l.Rule, next.Tool, next.Target, next.Key)
	if i < 0 {
		return nil
	}
	if _, field, why := cmp(l.Rule.Bindings[i], next); field != "" {
		return against(l, field, why)
	}
	return nil
}

// lendable refuses a borrowed rule its lender retired: it holds nothing.
func lendable(l Lent, verb string) error {
	if l.Rule.Status == model.Retired {
		return refuse("choose a rule "+l.From+" holds, as gragas rule list shows",
			"%s is retired in %s, and a retired rule takes no %s", l.Rule.ID, l.From, verb)
	}
	return nil
}

// against is the refusal of a change cmp holds back, naming the field.
func against(l Lent, field, why string) error {
	var hint string
	switch field {
	case againstEntry:
		hint = "bind " + string(l.Rule.ID) + " at the other entry with gragas bind, and the borrowed binding stays as it is"
	case againstRung, againstRunPoint:
		hint = "a borrowed binding keeps its rung and run point; bind " + string(l.Rule.ID) + " at another entry to hold it elsewhere too"
	default:
		hint = leaveHint(l)
	}
	return refuse(hint, "%s is %s's rule, and the change is refused at its %s: %s (S34)", l.Rule.ID, l.From, fieldWords(field), why)
}

// fieldWords is a field as a sentence names it.
func fieldWords(field string) string {
	return strings.ReplaceAll(field, "_", " ")
}

// leaveHint is the way to stop holding a borrowed rule: a parent's rule is
// excluded on a decision; an import's is left out with its part.
func leaveHint(l Lent) string {
	if l.Imported {
		return "import without the part that holds " + string(l.Rule.ID) + ", or ask " + l.From + " to change its binding"
	}
	return "exclude it with gragas rule exclude " + string(l.Rule.ID) + ", which stands on a decision"
}

// effective is the binding a borrowed rule holds at an entry, as the
// rulebook holds it: its last record there, else the lender's binding. rec
// is the record, nil for the lender's.
func effective(l Lent, tool string, target model.TargetType, key string) (model.Binding, *model.Tightening, bool) {
	for i := len(l.Records) - 1; i >= 0; i-- {
		t := l.Records[i]
		if t.Binding.Tool == tool && t.Binding.Target == target && t.Binding.Key == key {
			return t.Binding, &l.Records[i], true
		}
	}
	if i := indexOfBinding(l.Rule, tool, target, key); i >= 0 {
		return l.Rule.Bindings[i], nil, true
	}
	return model.Binding{}, nil, false
}

// checkBorrowed is checkBinding for a borrowed rule's binding, whose options
// a record holds: each option's name a word, its value a single value or a
// list.
func checkBorrowed(rule model.Rule, b model.Binding) error {
	plain := b
	plain.Options = nil
	if err := checkBinding(rule, plain); err != nil {
		return err
	}
	for k, v := range b.Options {
		if !optionName.MatchString(k) {
			return refuse("name the option as the tool does, a word of letters, digits and . _ / -", "the option %q of the binding is not a word", k)
		}
		if s, ok := scalarOrList(v); !ok || s == "" {
			return refuse(optionsHint, "the option %s of the binding is not a single value or a list", k)
		}
	}
	return nil
}

var optionName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// recordFor is the record a plan of the borrowed rule writes: rec, or, with
// none, a new file of bindings/, with no text yet.
func recordFor(b model.Book, l Lent, rec *model.Tightening) model.Tightening {
	if rec != nil {
		return *rec
	}
	return model.Tightening{Path: newRecordPath(b, l.Rule.ID), Subject: l.Rule.ID}
}

// borrowedRecord is the plan that writes next as the rulebook's record of
// the borrowed rule into the record at: in place of its text, or, with none,
// as a new file of bindings/.
func borrowedRecord(l Lent, kind, change string, at model.Tightening, next model.Binding, files model.PlanFiles) (Plan, error) {
	id := string(l.Rule.ID)
	text := RecordText(l.Rule.ID, next)
	d := model.Delta{Changes: []model.Change{{Entity: "record:" + at.Path, Before: at.Text, After: text}}}
	verb := "writes a record of "
	if at.Text != "" {
		verb = "changes the record of "
	}
	where := files.Bindings + "/" + strings.TrimPrefix(at.Path, recordsDir+"/")
	written := []string{where + ": " + verb + id + ", under this rulebook's prefix", files.History + ": appends one entry (" + kind + ")"}
	now := lifecycleOf(l.Rule)
	return Plan{Delta: d, Record: Record{Kind: kind, Entity: "rule:" + id, Before: &now, After: now},
		Sections: sections([]string{change, "tightens the binding " + l.From + " lends, so the change stands on no decision (S34)"}, written, "none")}, nil
}

// recordsDir is the folder of the records, below the book's folder.
const recordsDir = "bindings"

// newRecordPath is the first free name of bindings/ for a record of id: the
// id with its dots as dashes, then with -2, -3 and on.
func newRecordPath(b model.Book, id model.RuleID) string {
	base := recordsDir + "/" + strings.ReplaceAll(string(id), ".", "-")
	taken := func(p string) bool {
		return slices.ContainsFunc(b.BindingRecords, func(o model.Opaque) bool { return o.Path == p })
	}
	p := base + ".yaml"
	for n := 2; taken(p); n++ {
		p = fmt.Sprintf("%s-%d.yaml", base, n)
	}
	return p
}

// RecordText is the record of a binding of the borrowed rule id: the subject
// on the first line, then the binding's keys in the order of a rule's
// binding, the options after the value.
func RecordText(id model.RuleID, b model.Binding) string {
	return "subject: " + string(id) + "\n" + bindingRecord(b)
}
