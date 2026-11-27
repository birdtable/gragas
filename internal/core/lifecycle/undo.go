package lifecycle

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// Undo takes back the last change the history records, by writing a new entry
// that returns the lifecycle fields the reverted entry moved to the values it
// found. Nothing is removed from the history and no line is edited
// (store.state-files.5). The history holds the lifecycle fields before and
// after each change and nothing else, so undo returns those fields and no
// others: a change of words is a file edit, and so is its correction.

// UndoClass says how undo treats the entries of one kind.
type UndoClass string

const (
	// UndoInverts says the entry moved lifecycle fields, and undo puts them back.
	UndoInverts UndoClass = "inverts"
	// UndoRetires says the entry made a rule or a term, which no entry can remove
	// (an id is frozen at birth), so undo retires it.
	UndoRetires UndoClass = "retires"
	// UndoRefuses says the entry has no inverse in the lifecycle fields, so undo is
	// refused and names the command that corrects it.
	UndoRefuses UndoClass = "refuses"
)

// undoKinds is the class of each kind of entry any plan writes. A kind with no
// row is a bug a test finds (TestEveryKindOfEntryHasAnUndoClass). Import writes
// one entry for each rule it drafts, born the way new's is (no before, a
// proposed or grandfathered after), so undo retires that rule; the one entry
// for the guide's source is a source made, which no entry can remove, and is
// refused as a source made by new is. Bind, rebind and unbind move a binding,
// which is no lifecycle field, so the entry holds the same fields before and
// after and undo refuses it, naming the command that puts the binding back.
func undoKinds() map[string]UndoClass {
	return map[string]UndoClass{
		"new":                 UndoRetires,
		"import":              UndoRetires,
		"edit":                UndoInverts,
		"replace":             UndoInverts,
		"retire":              UndoInverts,
		"restore":             UndoInverts,
		"promote":             UndoInverts,
		"confirm":             UndoInverts,
		"renumber":            UndoRefuses,
		"exclude":             UndoRefuses,
		"include":             UndoRefuses,
		"supersede":           UndoRefuses,
		"decision-superseded": UndoRefuses,
		"source-edit":         UndoRefuses,
		"undo":                UndoRefuses,
		"bind":                UndoRefuses,
		"rebind":              UndoRefuses,
		"unbind":              UndoRefuses,
		"init":                UndoRefuses,
		"migrate":             UndoRefuses,
	}
}

// UndoKinds are the kinds of entry undo has a class for, sorted.
func UndoKinds() []string {
	classes := undoKinds()
	kinds := make([]string, 0, len(classes))
	for k := range classes {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	return kinds
}

// ClassOf is how undo treats an entry of the kind; the false is a kind it has
// no class for.
func ClassOf(kind string) (UndoClass, bool) {
	c, ok := undoKinds()[kind]
	return c, ok
}

// UndoTarget is the entry the next undo takes back: the last entry, in the
// chain's order, that is not an undo and that no undo has taken back already.
// An undo takes an entry back by naming its digest and agreeing with it
// (takesBack); a number names no one entry, since two branches may each take
// the same one. The false is a history with none, which ends "not ready".
func UndoTarget(history []model.HistoryEntry) (model.HistoryEntry, bool) {
	ordered := slices.Clone(history)
	slices.SortStableFunc(ordered, func(a, b model.HistoryEntry) int {
		return cmp.Or(
			cmp.Compare(a.Number, b.Number),
			strings.Compare(a.When, b.When),
			strings.Compare(string(a.Plan), string(b.Plan)),
			strings.Compare(string(a.Digest), string(b.Digest)),
		)
	})
	byDigest := make(map[model.Digest]model.HistoryEntry, len(ordered))
	for _, e := range ordered {
		if e.Digest != "" {
			byDigest[e.Digest] = e
		}
	}
	taken := map[model.Digest]bool{}
	for _, u := range ordered {
		if t, ok := byDigest[u.Reverts]; ok && u.Kind == "undo" && takesBack(u, t) {
			taken[t.Digest] = true
		}
	}
	for _, e := range slices.Backward(ordered) {
		if e.Kind != "undo" && !taken[e.Digest] {
			return e, true
		}
	}
	return model.HistoryEntry{}, false
}

// undoable says whether PlanUndo writes an undo for the entity: a rule or a
// term. No undo line on a source or a decision is one it wrote, so none takes
// an entry back.
func undoable(entity string) bool {
	kind, _, _ := strings.Cut(entity, ":")
	return kind == "rule" || kind == "term"
}

// takesBack says whether the undo entry u is the undo of t: t is an earlier
// entry on the same entity, of a kind undo inverts or retires, and on each
// field t moved, u starts from the value t left and ends on the value t found
// (for a new, u retires what t made). An entry that only names a digest takes
// nothing back, so a line written by hand, or one that names an entry it does
// not undo, hides no entry from undo.
func takesBack(u, t model.HistoryEntry) bool {
	if t.Kind == "undo" || t.Number >= u.Number || u.Entity != t.Entity || u.Before == nil || !undoable(t.Entity) {
		return false
	}
	switch c, _ := ClassOf(t.Kind); c {
	case UndoRetires:
		return u.After.Status == model.Retired && u.Before.Status == t.After.Status
	case UndoInverts:
		var before model.Lifecycle
		if t.Before != nil {
			before = *t.Before
		}
		agrees := false
		for _, f := range lifecycleFields() {
			if fieldOf(before, f) == fieldOf(t.After, f) {
				continue
			}
			if fieldOf(*u.Before, f) != fieldOf(t.After, f) || fieldOf(u.After, f) != fieldOf(before, f) {
				return false
			}
			agrees = true
		}
		return agrees
	case UndoRefuses:
		return false
	}
	return false
}

// fieldOf is the value of one lifecycle field, by its name in the file.
func fieldOf(l model.Lifecycle, name string) string {
	switch name {
	case "status":
		return string(l.Status)
	case "owner":
		return l.Owner
	case "review_by":
		return l.ReviewBy
	case "zero_by":
		return l.ZeroBy
	case "replaced_by":
		return string(l.ReplacedBy)
	case "literal":
		return l.Literal
	}
	return ""
}

// withField is l with one field set.
func withField(l model.Lifecycle, name, v string) model.Lifecycle {
	switch name {
	case "status":
		l.Status = model.Status(v)
	case "owner":
		l.Owner = v
	case "review_by":
		l.ReviewBy = v
	case "zero_by":
		l.ZeroBy = v
	case "replaced_by":
		l.ReplacedBy = model.RuleID(v)
	case "literal":
		l.Literal = v
	}
	return l
}

// lifecycleFields are the fields of a lifecycle, in the order a plan lists them.
func lifecycleFields() []string {
	return []string{"status", "owner", "review_by", "zero_by", "replaced_by", "literal"}
}

// PlanUndo plans the entry that takes back the last change. The history is the
// book's. It is refused when a field the entry moved has changed since, naming
// the later entry that changed it; "not ready" when there is nothing to take back.
func PlanUndo(b model.Book, _ model.PlanFiles, _ Moment) (Plan, error) {
	target, ok := UndoTarget(b.History)
	if !ok {
		return Plan{}, refuseAs(model.NotReady, "make a change with gragas rule, term, source or decision first", "the history holds no change to undo")
	}
	if target.Digest == "" {
		return Plan{}, refuse("check the history with gragas history", "entry %d has no digest to name it by", target.Number)
	}
	class, known := ClassOf(target.Kind)
	if !known {
		return Plan{}, refuse("take the change back with the command that made it", "undo does not know entries of kind %s (entry %d)", target.Kind, target.Number)
	}
	entity := target.Entity
	if class == UndoRefuses {
		return Plan{}, refuse(refusedHint(target), "entry %d (%s of %s) has no inverse in the lifecycle fields", target.Number, target.Kind, entity)
	}
	kind, key, _ := strings.Cut(entity, ":")
	if !undoable(entity) {
		if class == UndoRetires {
			return Plan{}, refuse(refusedHint(target), "entry %d (%s of %s) made something no entry can remove", target.Number, target.Kind, entity)
		}
		return Plan{}, refuse(refusedHint(target), "entry %d (%s of %s) changed a %s, and undo returns only a rule or a term", target.Number, target.Kind, entity, kind)
	}
	now, found := currentLifecycle(b, kind, key)
	if !found {
		return Plan{}, refuse("the entry names an entity the book does not hold; check the history with gragas history",
			"entry %d names %s, which the book does not hold", target.Number, entity)
	}
	if class == UndoRetires {
		return undoPlan(b, target, now, model.Lifecycle{Status: model.Retired}, []string{"status"}, "retires it: an id is frozen at birth and nothing is deleted")
	}
	before := model.Lifecycle{}
	if target.Before != nil {
		before = *target.Before
	}
	var fields []string
	for _, f := range lifecycleFields() {
		if fieldOf(before, f) != fieldOf(target.After, f) {
			fields = append(fields, f)
		}
	}
	if len(fields) == 0 {
		return Plan{}, refuse("a change of words is corrected by editing the file", "entry %d (%s of %s) moved no lifecycle field, so there is nothing to undo", target.Number, target.Kind, entity)
	}
	return undoPlan(b, target, now, before, fields, "returns what it moved")
}

// undoPlan builds the inverse: each of fields goes from its value now to its
// value in want, after checking that it is still the value the entry left.
func undoPlan(b model.Book, target model.HistoryEntry, now, want model.Lifecycle, fields []string, how string) (Plan, error) {
	var d model.Delta
	var lines []string
	after := now
	for _, f := range fields {
		if got, left := fieldOf(now, f), fieldOf(target.After, f); got != left {
			return Plan{}, refuse("undo the later change first, or correct the field with the command that owns it",
				"%s of %s has changed since entry %d: it is %q now, the entry left it %q; %s", f, target.Entity, target.Number, got, left, laterEntry(b.History, target, f))
		}
		v := fieldOf(want, f)
		d.Changes = append(d.Changes, model.Change{Entity: target.Entity, Field: f, Before: fieldOf(now, f), After: v})
		lines = append(lines, fmt.Sprintf("%s of %s returns from %q to %q", f, target.Entity, fieldOf(now, f), v))
		after = withField(after, f, v)
	}
	changes := append([]string{fmt.Sprintf("undo entry %d (%s of %s by %s, %s): %s", target.Number, target.Kind, target.Entity, target.Who, target.When, how)}, lines...)
	if target.Kind == "replace" && target.After.ReplacedBy != "" {
		changes = append(changes, fmt.Sprintf("its successor %s is no entry of the history and stays as it is: retire it with gragas rule retire %s", target.After.ReplacedBy, target.After.ReplacedBy))
	}
	rec := Record{Kind: "undo", Entity: target.Entity, Before: &now, After: after, Reverts: target.Digest}
	return Plan{
		Delta:    d,
		Record:   rec,
		Sections: sections(changes, []string{"the file of " + target.Entity + ": sets " + strings.Join(fields, ", "), "history.jsonl: appends one entry (undo), naming entry " + fmt.Sprint(target.Number)}, "none"),
	}, nil
}

// currentLifecycle is the guarded fields of the entity as the book holds them.
func currentLifecycle(b model.Book, kind, key string) (model.Lifecycle, bool) {
	switch kind {
	case "rule":
		r, ok := ruleByID(b, model.RuleID(key))
		return lifecycleOf(r), ok
	case "term":
		t, ok := termOf(b, key)
		return termLifecycle(t), ok
	}
	return model.Lifecycle{}, false
}

// laterEntry names the last entry after target that moved the field of the same
// entity, or says that none did: then the field was changed outside the history, and
// the guard lists it.
func laterEntry(history []model.HistoryEntry, target model.HistoryEntry, field string) string {
	var last *model.HistoryEntry
	for i := range history {
		if e := &history[i]; e.Entity == target.Entity && e.Number > target.Number && moved(*e, field) && (last == nil || e.Number >= last.Number) {
			last = e
		}
	}
	if last == nil {
		return "no later entry names it, so it was changed outside the history"
	}
	return fmt.Sprintf("entry %d (%s) changed it", last.Number, last.Kind)
}

// refusedHint says what takes back an entry undo has no inverse for.
func refusedHint(e model.HistoryEntry) string {
	if e.Kind != "new" {
		switch strings.SplitN(e.Entity, ":", 2)[0] {
		case "source":
			return "change the source back with gragas source edit"
		case "decision":
			return "record a new decision with gragas decision supersede"
		}
	}
	switch e.Kind {
	case "renumber":
		return "renumber again with gragas rule renumber, or reword the file"
	case "exclude":
		return "lift it with gragas rule include"
	case "include":
		return "exclude it again with gragas rule exclude"
	case "supersede", "decision-superseded":
		return "record a new decision with gragas decision supersede"
	case "source-edit":
		return "change the source back with gragas source edit"
	case "bind":
		return "take the binding out again with gragas unbind"
	case "rebind":
		return "move the binding back with gragas rebind"
	case "unbind":
		return "put the binding back with gragas bind"
	case "undo":
		return "undo goes one step at a time, back from the last change"
	case "init", "migrate":
		return "a rulebook is created or migrated, not undone: make a change with gragas rule, term, source or decision first"
	case "new":
		return "a " + strings.SplitN(e.Entity, ":", 2)[0] + " is never removed: change it with its own command"
	}
	return "take the change back with the command that made it"
}

// moved says whether the entry changed the field.
func moved(e model.HistoryEntry, field string) bool {
	var before model.Lifecycle
	if e.Before != nil {
		before = *e.Before
	}
	return fieldOf(before, field) != fieldOf(e.After, field)
}

// UndoRef is how the dry run's closing line names the target: by its digest,
// always, since a number unique in one book names another entry in another
// (a branch switch, a second clone). A person may still type a number that
// is unique.
func UndoRef(_ []model.HistoryEntry, target model.HistoryEntry) string {
	return string(target.Digest)
}

// minDigestRef is the fewest hex digits of a digest that name an entry.
const minDigestRef = 8

// refersTo says whether ref names the entry: its digest, or a prefix of it
// of at least minDigestRef hex digits, with or without the algorithm's name.
func refersTo(ref string, e model.HistoryEntry) bool {
	d := strings.TrimPrefix(string(e.Digest), "sha256:")
	ref = strings.TrimPrefix(ref, "sha256:")
	return e.Digest != "" && len(ref) >= minDigestRef && strings.HasPrefix(d, ref)
}

// PlanUndoEntry is PlanUndo for a run that names the entry it takes back, by
// its number or by its digest (a prefix will do). The entry named must be the
// one undo would take back now, so an entry appended between the dry run and
// the apply is never taken back unseen. A number names an entry only when no
// other has it: after a merge two entries may share one, and the run is
// refused naming each with its digest. With no entry given (""), a dry run
// plans as PlanUndo does and an apply is refused, naming the entry the dry
// run would show.
func PlanUndoEntry(ref string, applying bool) func(model.Book, model.PlanFiles, Moment) (Plan, error) {
	return func(b model.Book, f model.PlanFiles, m Moment) (Plan, error) {
		target, ok := UndoTarget(b.History)
		if !ok || (ref == "" && !applying) {
			return PlanUndo(b, f, m)
		}
		again := fmt.Sprintf("see the plan with gragas undo, then run gragas undo --entry %s --apply", UndoRef(b.History, target))
		what := fmt.Sprintf("entry %d (%s of %s)", target.Number, target.Kind, target.Entity)
		if ref == "" {
			return Plan{}, refuse(again, "--apply names the entry it takes back: the dry run shows %s", what)
		}
		named, ref, err := namedEntry(b.History, target, ref)
		if err != nil {
			return Plan{}, err
		}
		if !named {
			return Plan{}, refuse(again, "entry %s is not the one undo would take back now: that is %s", ref, what)
		}
		return PlanUndo(b, f, m)
	}
}

// namedEntry says whether ref, a number or a digest, names the target and no
// other entry; the error is a refusal when it names more than one.
func namedEntry(history []model.HistoryEntry, target model.HistoryEntry, ref string) (bool, string, error) {
	if n, err := strconv.Atoi(ref); err == nil {
		var same []string
		for _, e := range history {
			if e.Number == n {
				same = append(same, fmt.Sprintf("%s of %s, %s", e.Kind, e.Entity, e.Digest))
			}
		}
		if len(same) > 1 {
			slices.Sort(same)
			return false, ref, refuse("name the entry by its digest: gragas undo --entry <digest> --apply",
				"%d entries are numbered %d (%s)", len(same), n, strings.Join(same, "; "))
		}
		return n == target.Number, ref, nil
	}
	if !refersTo(ref, target) {
		return false, ref, nil
	}
	for _, e := range history {
		if e.Digest != target.Digest && refersTo(ref, e) {
			return false, ref, refuse("name the entry by more of its digest", "%s names more than one entry", ref)
		}
	}
	return true, ref, nil
}
