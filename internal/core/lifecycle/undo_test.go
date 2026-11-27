package lifecycle_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
)

// kindsWritten are the kinds of entry the code of this package writes: each
// literal that names the kind of a record, however the plan is made.
func kindsWritten(t *testing.T) []string {
	t.Helper()
	re := regexp.MustCompile(`(?:Kind: |nothing\w*\(|exclusionRecord\()"([a-z][a-z-]*)"`)
	seen := map[string]bool{}
	// the format package writes the entries of init and migrate
	for _, dir := range []string{".", "../format"} {
		files, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".go") || strings.HasSuffix(f.Name(), "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range re.FindAllSubmatch(b, -1) {
				seen[string(m[1])] = true
			}
		}
	}
	var out []string
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// TestEveryKindOfEntryHasAnUndoClassAndNoClassIsForAKindNobodyWrites: the table
// of classes holds each kind the plans write and no other, so a new kind with
// no class fails here, and a class left behind for a kind that went does too.
// Planted failure: a plan writes Kind: "frobnicate" with no class.
func TestEveryKindOfEntryHasAnUndoClassAndNoClassIsForAKindNobodyWrites(t *testing.T) {
	t.Parallel()
	written, classed := kindsWritten(t), lifecycle.UndoKinds()
	for _, k := range written {
		if _, ok := lifecycle.ClassOf(k); !ok {
			t.Errorf("a plan writes entries of kind %q and undo has no class for it", k)
		}
	}
	for _, k := range classed {
		if !slices.Contains(written, k) {
			t.Errorf("undo has a class for %q, which no plan writes", k)
		}
	}
	if len(written) < 13 {
		t.Errorf("the scan found %d kinds: %v, the code writes at least thirteen", len(written), written)
	}
}

func lc(s model.Status, edit ...func(*model.Lifecycle)) model.Lifecycle {
	l := model.Lifecycle{Status: s}
	for _, e := range edit {
		e(&l)
	}
	return l
}

// dig is the digest the test entry numbered n has: one for each number, so two
// entries that share one are given their own by the test.
func dig(n int) model.Digest { return model.Digest(fmt.Sprintf("sha256:%064x", n)) }

func histEntry(n int, kind, entity string, before *model.Lifecycle, after model.Lifecycle) model.HistoryEntry {
	return model.HistoryEntry{Number: n, Digest: dig(n), Kind: kind, Entity: entity, Before: before, After: after, Who: "Ada", When: "2026-11-08T10:00:00Z"}
}

// bookWith is the book of the rule tests with rule go.style.1 and term "pkg"
// holding the given fields, and the history.
func bookWith(rule model.Lifecycle, history ...model.HistoryEntry) model.Book {
	b := book()
	for i := range b.Rules {
		if b.Rules[i].ID == "go.style.1" {
			b.Rules[i].Status, b.Rules[i].Owner, b.Rules[i].ReviewBy, b.Rules[i].ReplacedBy = rule.Status, rule.Owner, rule.ReviewBy, rule.ReplacedBy
		}
	}
	b.Terms = []model.Term{{Slug: "pkg", Literal: "package", Status: model.Active}}
	b.History = history
	return b
}

// TestUndoTakesBackEveryKindOfEntryInItsClass: one case for every kind of
// entry, each in the class the table gives it. A kind that inverts returns the
// fields it moved to their before values in a new entry that names the entry
// reverted; a kind that retires retires what it made; a kind with no inverse is
// refused with the command to use instead. The table asserts it ran every kind.
// Planted failure: a class inverts the wrong field, or a refused kind plans.
func TestUndoTakesBackEveryKindOfEntryInItsClass(t *testing.T) {
	t.Parallel()
	rule := "rule:go.style.1"
	cases := []struct {
		kind       string
		book       model.Book
		hint       string // the hint of the refusal; "" when the case plans
		sourceEdit bool   // an edit of a source: refused though edit inverts
		field      string // the field the plan changes
		before     string
		after      string
	}{
		{kind: "edit", book: bookWith(lc(model.Active, func(l *model.Lifecycle) { l.Owner = "bo" }),
			histEntry(1, "edit", rule, new(lc(model.Active, func(l *model.Lifecycle) { l.Owner = "ada" })), lc(model.Active, func(l *model.Lifecycle) { l.Owner = "bo" }))),
			field: "owner", before: "bo", after: "ada"},
		{kind: "replace", book: bookWith(lc(model.Retired, func(l *model.Lifecycle) { l.ReplacedBy = "go.style.2" }),
			histEntry(1, "replace", rule, new(lc(model.Active)), lc(model.Retired, func(l *model.Lifecycle) { l.ReplacedBy = "go.style.2" }))),
			field: "status", before: "retired", after: "active"},
		{kind: "retire", book: bookWith(lc(model.Retired), histEntry(1, "retire", rule, new(lc(model.Active)), lc(model.Retired))),
			field: "status", before: "retired", after: "active"},
		{kind: "restore", book: bookWith(lc(model.Active), histEntry(1, "restore", rule, new(lc(model.Retired)), lc(model.Active))),
			field: "status", before: "active", after: "retired"},
		{kind: "promote", book: bookWith(lc(model.Trial), histEntry(1, "promote", rule, new(lc(model.Proposed)), lc(model.Trial))),
			field: "status", before: "trial", after: "proposed"},
		{kind: "confirm", book: bookWith(lc(model.Active, func(l *model.Lifecycle) { l.ReviewBy = "2027-02-01" }),
			histEntry(1, "confirm", rule, new(lc(model.Active, func(l *model.Lifecycle) { l.ReviewBy = "2026-11-01" })), lc(model.Active, func(l *model.Lifecycle) { l.ReviewBy = "2027-02-01" }))),
			field: "review_by", before: "2027-02-01", after: "2026-11-01"},
		{kind: "new", book: bookWith(lc(model.Proposed), histEntry(1, "new", rule, nil, lc(model.Proposed))),
			field: "status", before: "proposed", after: "retired"},
		{kind: "import", book: bookWith(lc(model.Proposed), histEntry(1, "import", rule, nil, lc(model.Proposed))),
			field: "status", before: "proposed", after: "retired"},
		{kind: "edit", book: bookWith(lc(model.Active), histEntry(1, "edit", "source:effective-go", new(lc(model.Active)), lc(model.Active))), hint: "gragas source edit", sourceEdit: true},
		{kind: "renumber", book: bookWith(lc(model.Active), histEntry(1, "renumber", rule, new(lc(model.Active)), lc(model.Active))), hint: "gragas rule renumber"},
		{kind: "exclude", book: bookWith(lc(model.Active), histEntry(1, "exclude", "exclusion:o/go.style.1/1", nil, lc(model.Active))), hint: "gragas rule include"},
		{kind: "include", book: bookWith(lc(model.Active), histEntry(1, "include", "exclusion:o/go.style.1/1", new(lc(model.Active)), lc(model.Retired))), hint: "gragas rule exclude"},
		{kind: "supersede", book: bookWith(lc(model.Active), histEntry(1, "supersede", "decision:wrap", new(lc(model.Active, func(l *model.Lifecycle) { l.Literal = "head" })), lc(model.Active, func(l *model.Lifecycle) { l.Literal = "superseded by x" }))),
			hint: "gragas decision supersede"},
		{kind: "decision-superseded", book: bookWith(lc(model.Active), histEntry(1, "decision-superseded", rule, new(lc(model.Active)), lc(model.Active))), hint: "gragas decision supersede"},
		{kind: "source-edit", book: bookWith(lc(model.Active), histEntry(1, "source-edit", rule, new(lc(model.Active)), lc(model.Active))), hint: "gragas source edit"},
		{kind: "undo", book: bookWith(lc(model.Active), histEntry(1, "undo", rule, new(lc(model.Active)), lc(model.Active))), hint: "go.style.1"},
		{kind: "bind", book: bookWith(lc(model.Active), histEntry(1, "bind", rule, new(lc(model.Active)), lc(model.Active))), hint: "gragas unbind"},
		{kind: "rebind", book: bookWith(lc(model.Active), histEntry(1, "rebind", rule, new(lc(model.Active)), lc(model.Active))), hint: "gragas rebind"},
		{kind: "unbind", book: bookWith(lc(model.Active), histEntry(1, "unbind", rule, new(lc(model.Active)), lc(model.Active))), hint: "gragas bind"},
		{kind: "init", book: bookWith(lc(model.Active), histEntry(1, "init", "rulebook:o@1", nil, lc(model.Active))), hint: "created or migrated"},
		{kind: "migrate", book: bookWith(lc(model.Active), histEntry(1, "migrate", "rulebook:o@1", nil, lc(model.Active))), hint: "created or migrated"},
	}
	ran := map[string]bool{}
	for _, c := range cases {
		ran[c.kind] = ran[c.kind] || !c.sourceEdit
		p, err := lifecycle.PlanUndo(c.book, model.PlanFiles{}, moment())
		if c.hint != "" {
			// a lone undo entry is never a target; the refusal of the others names the command
			if c.kind == "undo" {
				refusal(t, err, "", "no change to undo")
				continue
			}
			says := "has no inverse"
			if c.sourceEdit {
				says = "undo returns only a rule or a term"
			}
			refusal(t, err, c.hint, says)
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.kind, err)
			continue
		}
		ch := changeFor(t, p, c.book.History[0].Entity, c.field)
		if ch.Before != c.before || ch.After != c.after {
			t.Errorf("%s: %s goes from %q to %q, want %q to %q", c.kind, c.field, ch.Before, ch.After, c.before, c.after)
		}
		if p.Record.Kind != "undo" || p.Record.Reverts != dig(1) || p.Record.Entity != c.book.History[0].Entity {
			t.Errorf("%s: the entry is %+v, want kind undo naming entry 1", c.kind, p.Record)
		}
	}
	for _, k := range lifecycle.UndoKinds() {
		if !ran[k] {
			t.Errorf("the table ran no case for kind %q", k)
		}
	}
}

// TestUndoOfATermMadeRetiresIt: a term made is retired, never removed.
func TestUndoOfATermMadeRetiresIt(t *testing.T) {
	t.Parallel()
	b := bookWith(lc(model.Active), histEntry(1, "new", "term:pkg", nil, model.Lifecycle{Status: model.Active, Literal: "package"}))
	p, err := lifecycle.PlanUndo(b, model.PlanFiles{}, moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, p, "term:pkg", "status"); c.Before != "active" || c.After != "retired" {
		t.Errorf("status: %+v", c)
	}
}

// TestUndoOfASourceOrADecisionMadeIsRefused: neither can be removed, so the
// refusal says to edit or supersede.
func TestUndoOfASourceOrADecisionMadeIsRefused(t *testing.T) {
	t.Parallel()
	for _, e := range []string{"source:effective-go", "decision:wrap"} {
		b := bookWith(lc(model.Active), histEntry(1, "new", e, nil, lc(model.Active)))
		_, err := lifecycle.PlanUndo(b, model.PlanFiles{}, moment())
		refusal(t, err, "never removed", "made something no entry can remove")
	}
}

// TestUndoRefusesAnEntryItCannotNameOrPlace: the last entry has no digest, has
// a kind with no class, names an entity the book does not hold, or moved no
// lifecycle field; each is refused and plans nothing. A digest prefix that two
// entries share is refused too.
// Planted failure: one of the checks goes, and undo plans over a bad entry.
func TestUndoRefusesAnEntryItCannotNameOrPlace(t *testing.T) {
	t.Parallel()
	rule := "rule:go.style.1"
	nodigest := histEntry(1, "promote", rule, new(lc(model.Proposed)), lc(model.Trial))
	nodigest.Digest = ""
	cases := []struct {
		name string
		last model.HistoryEntry
		says string
	}{
		{"no digest", nodigest, "has no digest to name it by"},
		{"an unknown kind", histEntry(1, "frobnicate", rule, new(lc(model.Active)), lc(model.Active)), "does not know entries of kind frobnicate"},
		{"an entity the book lacks", histEntry(1, "promote", "rule:go.style.99", new(lc(model.Proposed)), lc(model.Trial)), "which the book does not hold"},
		{"no lifecycle field moved", histEntry(1, "edit", rule, new(lc(model.Active)), lc(model.Active)), "moved no lifecycle field"},
	}
	for _, c := range cases {
		_, err := lifecycle.PlanUndo(bookWith(lc(model.Trial), c.last), model.PlanFiles{}, moment())
		var r *lifecycle.Refusal
		if !errors.As(err, &r) || !strings.Contains(r.Msg, c.says) {
			t.Errorf("%s: err = %v, want a refusal saying %q", c.name, err, c.says)
		}
	}
	a := histEntry(1, "promote", rule, new(lc(model.Proposed)), lc(model.Trial))
	a.Digest = "sha256:" + model.Digest(strings.Repeat("a", 32)+strings.Repeat("0", 32))
	b := histEntry(2, "promote", rule, new(lc(model.Proposed)), lc(model.Trial))
	b.Digest = "sha256:" + model.Digest(strings.Repeat("a", 32)+strings.Repeat("1", 32))
	book := bookWith(lc(model.Trial), a, b)
	if _, err := lifecycle.PlanUndoEntry(strings.Repeat("a", 8), true)(book, model.PlanFiles{}, moment()); err == nil || !strings.Contains(err.Error(), "names more than one entry") {
		t.Errorf("a prefix two entries share: err = %v, want the refusal naming it", err)
	}
}

// TestUndoWithNoHistoryIsNotReady.
// Planted failure: it ends refused, or plans nothing and ends done.
func TestUndoWithNoHistoryIsNotReady(t *testing.T) {
	t.Parallel()
	_, err := lifecycle.PlanUndo(book(), model.PlanFiles{}, moment())
	var r *lifecycle.Refusal
	if !errors.As(err, &r) || r.Word != model.NotReady {
		t.Fatalf("err = %v, want a refusal that ends on not ready", err)
	}
}

// TestUndoAfterALaterChangeToTheFieldIsRefusedNamingTheEntry: entry 3 took the
// owner to "cy" after entry 1 left it "bo", and entry 4 undid entry 3's
// neighbour, so entry 1 is the target; the owner is "cy" now.
// Planted failure: the refusal does not name the entry that moved the field, or
// undo plans over the later value.
func TestUndoAfterALaterChangeToTheFieldIsRefusedNamingTheEntry(t *testing.T) {
	t.Parallel()
	owner := func(o string) model.Lifecycle { return lc(model.Active, func(l *model.Lifecycle) { l.Owner = o }) }
	rule := "rule:go.style.1"
	b := bookWith(owner("cy"),
		histEntry(1, "edit", rule, new(owner("ada")), owner("bo")),
		histEntry(2, "edit", rule, new(owner("bo")), owner("cy")),
		func() model.HistoryEntry {
			e := histEntry(3, "undo", rule, new(owner("cy")), owner("bo"))
			e.Reverts = dig(2)
			return e
		}(),
	)
	_, err := lifecycle.PlanUndo(b, model.PlanFiles{}, moment())
	refusal(t, err, "undo the later change first", "owner of rule:go.style.1 has changed since entry 1")
	if !strings.Contains(err.Error(), "entry 3 (undo) changed it") {
		t.Errorf("the refusal does not name the entry: %v", err)
	}
	// a field changed by hand has no later entry
	b.History = b.History[:1]
	_, err = lifecycle.PlanUndo(b, model.PlanFiles{}, moment())
	refusal(t, err, "", "outside the history")
}

// TestUndoTwiceTakesBackTwoEntriesOneAtATime: after one undo, the next takes
// back the entry before, never the same one again, and never an undo; with
// nothing left it is not ready. Each step holds two lines: the original and the
// undo.
// Planted failure: a second undo takes back the same entry, or an undo entry.
func TestUndoTwiceTakesBackTwoEntriesOneAtATime(t *testing.T) {
	t.Parallel()
	rule := "rule:go.style.1"
	b := bookWith(lc(model.Retired),
		histEntry(1, "promote", rule, new(lc(model.Proposed)), lc(model.Active)),
		histEntry(2, "retire", rule, new(lc(model.Active)), lc(model.Retired)))
	first, err := lifecycle.PlanUndo(b, model.PlanFiles{}, moment())
	if err != nil {
		t.Fatal(err)
	}
	if first.Record.Reverts != dig(2) || first.Record.After.Status != model.Active {
		t.Fatalf("first undo: %+v", first.Record)
	}
	// apply: the rule is active again, and the history holds the undo
	b = bookWith(lc(model.Active), b.History[0], b.History[1])
	b.History = append(b.History, model.HistoryEntry{Number: 3, Digest: dig(3), Kind: "undo", Entity: rule, Before: first.Record.Before, After: first.Record.After, Reverts: dig(2)})
	second, err := lifecycle.PlanUndo(b, model.PlanFiles{}, moment())
	if err != nil {
		t.Fatal(err)
	}
	if second.Record.Reverts != dig(1) || second.Record.After.Status != model.Proposed {
		t.Fatalf("second undo: %+v", second.Record)
	}
	b = bookWith(lc(model.Proposed), b.History...)
	b.History = append(b.History, model.HistoryEntry{Number: 4, Digest: dig(4), Kind: "undo", Entity: rule, Before: second.Record.Before, After: second.Record.After, Reverts: dig(1)})
	_, err = lifecycle.PlanUndo(b, model.PlanFiles{}, moment())
	refusal(t, err, "", "no change to undo")
}

// TestUndoOfARealRetireReturnsTheRule: the plan retire makes is the one undo
// takes back, over the entry its own record becomes.
func TestUndoOfARealRetireReturnsTheRule(t *testing.T) {
	t.Parallel()
	b := book()
	p, err := lifecycle.PlanRuleRetire(b, retire(), moment())
	if err != nil {
		t.Fatal(err)
	}
	e := model.HistoryEntry{Number: 1, Digest: dig(1), Kind: p.Record.Kind, Entity: p.Record.Entity, Before: p.Record.Before, After: p.Record.After}
	for i := range b.Rules {
		if b.Rules[i].ID == "go.style.1" {
			b.Rules[i].Status = model.Retired
		}
	}
	b.History = []model.HistoryEntry{e}
	u, err := lifecycle.PlanUndo(b, model.PlanFiles{}, moment())
	if err != nil {
		t.Fatal(err)
	}
	if c := changeFor(t, u, "rule:go.style.1", "status"); c.Before != "retired" || c.After != "active" {
		t.Errorf("status: %+v", c)
	}
	if !strings.Contains(screenOf(u), "undo entry 1 (retire of rule:go.style.1") {
		t.Errorf("the screen does not name the entry:\n%s", screenOf(u))
	}
}

// undoOf is the undo entry numbered n of t, as the plan writes it: it starts
// from what t left and ends on what t found.
func undoOf(n int, t model.HistoryEntry) model.HistoryEntry {
	e := histEntry(n, "undo", t.Entity, new(t.After), *t.Before)
	e.Reverts = t.Digest
	return e
}

// TestAnUndoNamesTheEntryByItsDigestNotItsNumber: two branches each append
// entry 6; one undo takes back one of them, and the next offers the other.
// Planted failure: the entry named by its number, so one undo hides both.
func TestAnUndoNamesTheEntryByItsDigestNotItsNumber(t *testing.T) {
	t.Parallel()
	rule := "rule:go.style.1"
	five := histEntry(5, "edit", rule, new(lc(model.Proposed)), lc(model.Proposed))
	a := histEntry(6, "retire", rule, new(lc(model.Active)), lc(model.Retired))
	a.Digest, a.When = "sha256:"+model.Digest(strings.Repeat("a", 64)), "2026-11-08T10:00:00Z"
	c := histEntry(6, "promote", "rule:go.style.2", new(lc(model.Proposed)), lc(model.Active))
	c.Digest, c.When = "sha256:"+model.Digest(strings.Repeat("c", 64)), "2026-11-08T11:00:00Z"
	for name, order := range map[string][]model.HistoryEntry{"in one order": {five, a, c}, "in the other": {five, c, a}} {
		first, ok := lifecycle.UndoTarget(order)
		if !ok || first.Digest != c.Digest {
			t.Fatalf("%s: the first undo takes %+v, want the later of the two 6s by the chain's order (c)", name, first)
		}
		got, ok := lifecycle.UndoTarget(append(slices.Clone(order), undoOf(7, c)))
		if !ok || got.Digest != a.Digest {
			t.Errorf("%s: after an undo of c the next takes %+v, want a, the other entry numbered 6", name, got)
		}
	}
}

// TestALineThatNamesAnEntryItDoesNotUndoHidesNothing: a hand-written undo that
// names the refused entry, that names a digest no entry has, or that does not
// return what the entry moved, takes nothing back. Planted failure: any undo
// line that names a digest hides the entry, as it did.
func TestALineThatNamesAnEntryItDoesNotUndoHidesNothing(t *testing.T) {
	t.Parallel()
	rule := "rule:go.style.1"
	retire := histEntry(1, "retire", rule, new(lc(model.Active)), lc(model.Retired))
	sup := histEntry(2, "supersede", rule, new(lc(model.Retired)), lc(model.Retired))
	forged := func(n int, names model.Digest, before, after model.Lifecycle) model.HistoryEntry {
		e := histEntry(n, "undo", rule, new(before), after)
		e.Reverts = names
		return e
	}
	for name, u := range map[string]model.HistoryEntry{
		"it names the refused entry, before equal after": forged(3, sup.Digest, lc(model.Retired), lc(model.Retired)),
		"it names a digest no entry has":                 forged(3, dig(99), lc(model.Retired), lc(model.Active)),
		"it names the retire but changes nothing":        forged(3, retire.Digest, lc(model.Retired), lc(model.Retired)),
		"it names the retire and returns another field":  forged(3, retire.Digest, lc(model.Retired), lc(model.Retired, func(l *model.Lifecycle) { l.Owner = "x" })),
	} {
		got, ok := lifecycle.UndoTarget([]model.HistoryEntry{retire, sup, u})
		if !ok || got.Digest != sup.Digest {
			t.Errorf("%s: the target is %+v, want the supersede (entry 2), still standing", name, got)
		}
	}
	// A source or a decision: PlanUndo refuses its entry and writes no undo
	// for it, so a line that agrees with the entry still hides nothing.
	srcNew := histEntry(2, "new", "source:s", nil, lc(model.Active, func(l *model.Lifecycle) { l.Literal = "loc" }))
	decNew := histEntry(2, "new", "decision:d", nil, lc(model.Active, func(l *model.Lifecycle) { l.Literal = "head" }))
	srcEdit := histEntry(2, "edit", "source:s", new(lc(model.Active, func(l *model.Lifecycle) { l.Literal = "old" })),
		lc(model.Active, func(l *model.Lifecycle) { l.Literal = "loc" }))
	for name, c := range map[string]struct {
		t model.HistoryEntry
		u model.HistoryEntry
	}{
		"a new of a source":   {srcNew, forgedOn(3, srcNew, lc(model.Active, func(l *model.Lifecycle) { l.Literal = "loc" }), lc(model.Retired))},
		"a new of a decision": {decNew, forgedOn(3, decNew, lc(model.Active, func(l *model.Lifecycle) { l.Literal = "head" }), lc(model.Retired))},
		"an edit of a source": {srcEdit, forgedOn(3, srcEdit, lc(model.Active, func(l *model.Lifecycle) { l.Literal = "loc" }), lc(model.Active, func(l *model.Lifecycle) { l.Literal = "old" }))},
	} {
		got, ok := lifecycle.UndoTarget([]model.HistoryEntry{retire, c.t, c.u})
		if !ok || got.Digest != c.t.Digest {
			t.Errorf("%s: the target is %+v, want entry 2, still standing", name, got)
		}
	}
	good := undoOf(3, retire)
	if got, ok := lifecycle.UndoTarget([]model.HistoryEntry{retire, good}); ok {
		t.Errorf("an undo that returns the retire left %+v to undo, want none", got)
	}
}

// forgedOn is a hand-written undo line numbered n on t's entity that names t.
func forgedOn(n int, t model.HistoryEntry, before, after model.Lifecycle) model.HistoryEntry {
	e := histEntry(n, "undo", t.Entity, new(before), after)
	e.Reverts = t.Digest
	return e
}

// TestUndoOfAReplaceNamesTheSuccessorItLeaves: the plan says the successor
// stays and what retires it. Planted failure: the plan does not mention it.
func TestUndoOfAReplaceNamesTheSuccessorItLeaves(t *testing.T) {
	t.Parallel()
	rule := "rule:go.style.1"
	b := bookWith(lc(model.Retired, func(l *model.Lifecycle) { l.ReplacedBy = "go.style.2" }),
		histEntry(1, "replace", rule, new(lc(model.Active)), lc(model.Retired, func(l *model.Lifecycle) { l.ReplacedBy = "go.style.2" })))
	p, err := lifecycle.PlanUndo(b, model.PlanFiles{}, moment())
	if err != nil {
		t.Fatal(err)
	}
	if s := screenOf(p); !strings.Contains(s, "gragas rule retire go.style.2") {
		t.Errorf("the plan does not name the successor:\n%s", s)
	}
}

// TestAnApplyNamesTheEntryUndoTakesNow: --apply with no entry, or with an
// entry other than the one undo would take back now, is refused naming the
// entry it would take; the entry itself, and a dry run with none, plan.
// Planted failure: an apply with no entry plans, so an entry appended since
// the dry run is taken back unseen.
func TestAnApplyNamesTheEntryUndoTakesNow(t *testing.T) {
	t.Parallel()
	rule := "rule:go.style.1"
	one := histEntry(1, "promote", rule, new(lc(model.Proposed)), lc(model.Active))
	two := histEntry(2, "edit", rule, new(lc(model.Active)), lc(model.Active, func(l *model.Lifecycle) { l.Owner = "x" }))
	b := bookWith(lc(model.Active, func(l *model.Lifecycle) { l.Owner = "x" }), one, two)
	for name, c := range map[string]struct {
		entry    string
		applying bool
		refused  bool
	}{
		"a dry run with no entry":        {"", false, false},
		"an apply with no entry":         {"", true, true},
		"an apply of an older entry":     {"1", true, true},
		"a dry run of an older entry":    {"1", false, true},
		"an apply of an entry not there": {"9", true, true},
		"an apply of the entry":          {"2", true, false},
	} {
		_, err := lifecycle.PlanUndoEntry(c.entry, c.applying)(b, model.PlanFiles{}, moment())
		switch {
		case c.refused && (err == nil || !strings.Contains(err.Error(), "entry 2")):
			t.Errorf("%s: err %v, want a refusal naming entry 2", name, err)
		case !c.refused && err != nil:
			t.Errorf("%s: %v, want a plan", name, err)
		}
	}
	// an entry appended after the dry run: the entry the dry run showed is no longer the one
	three := histEntry(3, "edit", rule, new(lc(model.Active, func(l *model.Lifecycle) { l.Owner = "x" })), lc(model.Active, func(l *model.Lifecycle) { l.Owner = "y" }))
	b.History = append(b.History, three)
	if _, err := lifecycle.PlanUndoEntry("2", true)(b, model.PlanFiles{}, moment()); err == nil || !strings.Contains(err.Error(), "entry 3") {
		t.Errorf("an entry appended since the dry run: err %v, want a refusal naming entry 3", err)
	}
}

// TestAnEntryNumberedTwiceIsNamedByItsDigest: after a merge two entries may
// share a number, so a number names an entry only when no other has it. The
// dry run's closing line then carries the digest, the apply by the number is
// refused naming each entry, and the digest takes the entry. Planted failure:
// the number is checked against the target alone, so the apply takes back the
// merged-in entry the dry run never showed.
func TestAnEntryNumberedTwiceIsNamedByItsDigest(t *testing.T) {
	t.Parallel()
	rule := "rule:go.style.1"
	five := histEntry(5, "edit", rule, new(lc(model.Proposed)), lc(model.Proposed))
	a := histEntry(6, "retire", rule, new(lc(model.Active)), lc(model.Retired))
	a.Digest, a.When = "sha256:"+model.Digest(strings.Repeat("a", 64)), "2026-11-08T10:00:00Z"
	c := histEntry(6, "edit", rule, new(lc(model.Active)), lc(model.Active, func(l *model.Lifecycle) { l.Owner = "x" }))
	c.Digest, c.When = "sha256:"+model.Digest(strings.Repeat("c", 64)), "2026-11-08T11:00:00Z"
	b := bookWith(lc(model.Active, func(l *model.Lifecycle) { l.Owner = "x" }), five, a, c)
	if got := lifecycle.UndoRef(b.History, c); got != string(c.Digest) {
		t.Errorf("the closing line names %q, want the digest of the later 6", got)
	}
	if got := lifecycle.UndoRef([]model.HistoryEntry{five, c}, c); got != string(c.Digest) {
		t.Errorf("with one 6 the closing line names %q, want the digest", got)
	}
	for name, ref := range map[string]string{"the number": "6"} {
		_, err := lifecycle.PlanUndoEntry(ref, true)(b, model.PlanFiles{}, moment())
		if err == nil || !strings.Contains(err.Error(), string(a.Digest)) || !strings.Contains(err.Error(), string(c.Digest)) {
			t.Errorf("%s: err %v, want a refusal naming both entries with their digests", name, err)
		}
	}
	for name, ref := range map[string]string{"the digest": string(c.Digest), "a prefix": "sha256:" + strings.Repeat("c", 8), "a prefix without the name": strings.Repeat("c", 12)} {
		if _, err := lifecycle.PlanUndoEntry(ref, true)(b, model.PlanFiles{}, moment()); err != nil {
			t.Errorf("%s: %v, want a plan", name, err)
		}
	}
	for name, ref := range map[string]string{"the digest of the other": string(a.Digest), "a short prefix": "cccc"} {
		if _, err := lifecycle.PlanUndoEntry(ref, true)(b, model.PlanFiles{}, moment()); err == nil {
			t.Errorf("%s: planned, want a refusal", name)
		}
	}
}
