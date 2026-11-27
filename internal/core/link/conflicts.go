package link

import (
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// Conflicts is the entry conflicts of the resolved rulebook (S8, S32): each
// tool entry that two or more bindings name with different values, options or
// severity, in the order the entries first appear in the rules. The same entry
// with the same value held by two rules is the many-to-many case and is not a
// conflict. Values are compared through the rank op of the plugin that serves
// the tool, so two spellings it reads as one value,
// off among them, are the same value; options by their members; severity as
// the engine's.
//
// Only the bindings of rules that hold, status active or trial, at rung 0 to 5
// are compared: a proposed or retired rule holds no entry, and a rung-6
// binding names a review question and no tool entry.
//
// Across a chain, a binding that tightens a rule of the parent or of an import
// (Binding.Tightens) wins over that rule's binding where it is tighter by S70's
// words: a stricter value by the plugin's rank or a stricter severity, with
// the same options and nothing looser (options carry no word, gragas#1554).
// Where it is not tighter, the two stay in conflict and the importer resolves.
//
// A conflict is resolved, and has a Winner, when a rule that is one of its
// sides declares a conflicts relation with another side (or is named by one)
// and a decision not since superseded cites the winning rule's binding on the
// entry. gragas never picks a winner itself: with no such decision the
// conflict is open and blocks each of its bindings.
//
// rank answers the Rank of a tool's plugin; nil, or a nil Rank, ranks none.
func Conflicts(book model.Book, rank func(tool string) Rank) []model.Conflict {
	order, groups := holders(book)
	var out []model.Conflict
	for _, ref := range order {
		var r Rank
		if rank != nil {
			r = rank(ref.Tool)
		}
		if c, ok := conflictOf(book, ref, withoutTightened(r, groups[ref]), r); ok {
			out = append(out, c)
		}
	}
	return out
}

// held is a binding with the rule that holds it.
type held struct {
	rule model.Rule
	b    model.Binding
}

func (h held) binding() model.Binding { return h.b }

// holders is the entries the rules hold, in the order they first appear, each
// with its bindings in the order of the rules: only the bindings of an active or
// trial rule, at rung 0 to 5.
func holders(book model.Book) ([]model.EntryRef, map[model.EntryRef][]held) {
	var order []model.EntryRef
	groups := map[model.EntryRef][]held{}
	for _, r := range book.Rules {
		if r.Status != model.Active && r.Status != model.Trial {
			continue
		}
		for _, b := range r.Bindings {
			if b.Rung >= model.MaxRung {
				continue
			}
			ref := model.EntryRef{Tool: b.Tool, Target: b.Target, Key: b.Key}
			if _, ok := groups[ref]; !ok {
				order = append(order, ref)
			}
			groups[ref] = append(groups[ref], held{r, b})
		}
	}
	return order, groups
}

// withoutTightened is the bindings less each one that a binding of a child
// tightens: the child's wins over it (S70).
func withoutTightened(r Rank, hs []held) []held {
	gone := make([]bool, len(hs))
	for i, c := range hs {
		if c.b.Tightens == "" {
			continue
		}
		for j, p := range hs {
			if i != j && p.rule.ID == c.b.Tightens && tightensBinding(r, c.b, p.b) {
				gone[j] = true
			}
		}
	}
	var live []held
	for i, h := range hs {
		if !gone[i] {
			live = append(live, h)
		}
	}
	return live
}

// conflictOf is the conflict the bindings of one entry make, if they make one:
// two or more that differ in value, options or severity.
func conflictOf(book model.Book, ref model.EntryRef, hs []held, r Rank) (model.Conflict, bool) {
	if len(hs) < 2 {
		return model.Conflict{}, false
	}
	c := model.Conflict{Entry: ref}
	for _, aspect := range []string{model.DiffValue, model.DiffOptions, model.DiffSeverity} {
		if aspectDiffers(r, aspect, hs, held.binding) {
			c.Differs = append(c.Differs, aspect)
		}
	}
	if len(c.Differs) == 0 {
		return model.Conflict{}, false
	}
	for _, h := range hs {
		c.Sides = append(c.Sides, model.ConflictSide{
			Ref: h.b.Ref(h.rule.ID), Rule: h.rule.ID, Value: h.b.Value, Options: h.b.Options, Severity: h.b.Severity,
		})
	}
	c.Winner = winner(book, ref, c.Sides)
	return c, true
}

// aspectDiffers says whether any two of the bindings differ in the aspect.
func aspectDiffers[T any](r Rank, aspect string, hs []T, of func(T) model.Binding) bool {
	for i := 1; i < len(hs); i++ {
		if !sameAspect(r, aspect, of(hs[0]), of(hs[i])) {
			return true
		}
	}
	return false
}

// sameAspect is whether a and b, at one entry, are the same in the aspect: a
// value the plugin ranks the same, the same options, the same severity.
func sameAspect(r Rank, aspect string, a, b model.Binding) bool {
	switch aspect {
	case model.DiffValue:
		return rankOf(r, a.Key, a.Value, b.Value) == plugin.WordSame
	case model.DiffOptions:
		return stable(a.Options) == stable(b.Options)
	}
	return a.Severity == b.Severity
}

// tightensBinding is whether the child's binding is the parent's tightened and
// nothing looser (S70): each of value, options and severity the same or
// tighter, and at least one not the same.
func tightensBinding(r Rank, child, parent model.Binding) bool {
	if sameAspect(r, model.DiffValue, child, parent) && sameAspect(r, model.DiffOptions, child, parent) &&
		sameAspect(r, model.DiffSeverity, child, parent) {
		return false
	}
	return valueNotLooser(r, child.Key, child.Value, parent.Value) &&
		severityRank(child.Severity) >= severityRank(parent.Severity) &&
		sameAspect(r, model.DiffOptions, child, parent)
}

// winner is the rule a decision names as the winner of the conflict, or "":
// a rule of the conflict declares a conflicts relation with another rule of it,
// and a decision not superseded cites the winner's binding on the entry as
// "<rule id> <tool> <key>". One winner only; two named is no decision.
func winner(book model.Book, ref model.EntryRef, sides []model.ConflictSide) model.RuleID {
	ids := make([]model.RuleID, len(sides))
	for i, s := range sides {
		ids[i] = s.Rule
	}
	if !declaresConflict(book, ids) {
		return ""
	}
	named := namedWinners(book, ref, ids)
	if len(named) != 1 {
		return ""
	}
	return named[0]
}

// declaresConflict is whether one of the rules declares a conflicts relation
// with another of them.
func declaresConflict(book model.Book, ids []model.RuleID) bool {
	for _, r := range book.Rules {
		if r.Relations == nil || !slices.Contains(ids, r.ID) {
			continue
		}
		if slices.ContainsFunc(r.Relations.Conflicts, func(o model.RuleID) bool { return o != r.ID && slices.Contains(ids, o) }) {
			return true
		}
	}
	return false
}

// namedWinners are the rules of ids whose binding on the entry a decision, not
// superseded, cites.
func namedWinners(book model.Book, ref model.EntryRef, ids []model.RuleID) []model.RuleID {
	var named []model.RuleID
	for _, d := range book.Decisions {
		if d.SupersededBy != "" || d.Cites == nil {
			continue
		}
		for _, c := range d.Cites.Bindings {
			f := strings.Fields(c)
			if len(f) != 3 || f[1] != ref.Tool || f[2] != ref.Key {
				continue
			}
			if id := model.RuleID(f[0]); slices.Contains(ids, id) && !slices.Contains(named, id) {
				named = append(named, id)
			}
		}
	}
	return named
}
