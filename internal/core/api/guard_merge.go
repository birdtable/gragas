package coreapi

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// merge judges a merge of theirs into ours. Every commit the
// merge brings in is judged first, each as its own commit was: a commit
// with one parent by its lines' commands replayed over that parent's tree,
// a merge commit by the three-way rule, and a commit with no parent from a
// tree with nothing in it. So no side's change passes for its own unless
// its own history records it, whatever made the commit: a stash, a clone
// with no hook, a commit made before the hook was. Then
// the merge itself is held to the three-way rule, which holds a line and
// its change together.
func (s *staged) merge(g model.Guard, ours, theirs string) (model.Guard, string, string, error) {
	commits, err := s.p.History.Commits(s.ctx, s.ask.Root, ours, theirs)
	if err != nil {
		return g, "", "", err
	}
	s.cache = newRevCache()
	defer func() { s.cache = nil }()
	for _, c := range commits {
		if cg, why, hint, err := s.broughtIn(c); why != "" || err != nil {
			if why != "" {
				why = "the merge brings in commit " + short(c.ID) + ", which is refused: " + why
			}
			return cg, why, hint, err
		}
	}
	return s.threeWay(g, ours, theirs)
}

// broughtIn judges one commit a merge brings in against its parents.
func (s *staged) broughtIn(c model.Commit) (model.Guard, string, string, error) {
	from := side{none: true}
	switch len(c.Parents) {
	case 0:
	case 1, 2:
		from = side{rev: c.Parents[0]}
	default:
		return model.Guard{}, fmt.Sprintf("it merges %d branches at once, and a merge is judged against one other side", len(c.Parents)), mergeOne, nil
	}
	t, g, why, hint, err := s.cache.open(s.ctx, s.p, s.ask, from, c.ID, broughtIn(c))
	if t == nil || why != "" || err != nil {
		return g, why, hint, err
	}
	if len(c.Parents) == 2 {
		return t.threeWay(g, c.Parents[0], c.Parents[1])
	}
	return t.replay(g)
}

// threeWay holds a merge of theirs into ours, s's tree after, to its merge
// base (A38, agents.guards.1). history.jsonl is ours's lines, then the other
// side's lines since the merge base that ours lacks, in that side's order,
// and no other line. Each lifecycle field is as the side that changed it
// since the merge base leaves it: ours's where only ours changed it, theirs's
// where only theirs did, the merge base's where neither did. Both sides
// changing one field to two values is refused: no line of either records
// the value a merge would pick.
func (s *staged) threeWay(g model.Guard, ours, theirs string) (model.Guard, string, string, error) {
	base, err := s.p.History.Base(s.ctx, s.ask.Root, ours, theirs)
	if err != nil {
		return g, "", "", err
	}
	baseTree, err := s.cache.listed(s.ctx, s.p, s.ask, base)
	if err != nil {
		return g, "", "", err
	}
	theirTree, err := s.cache.listed(s.ctx, s.p, s.ask, theirs)
	if err != nil {
		return g, "", "", err
	}
	// A side with no one book in force, more than one rulebook.yaml at its
	// shallowest depth, is read as a tree with nothing in it, as any commit
	// is judged. The tree after was read first, and holds one.
	for _, t := range []**revTree{&baseTree, &theirTree} {
		if _, err := format.Find(*t, s.ask.Root); errors.Is(err, ErrTwoBooks) {
			*t = (*t).empty()
		}
	}
	baseBook, why, hint := s.sideBook(baseTree, "the merge base")
	if why != "" {
		return g, why, hint, nil
	}
	theirBook, why, hint := s.sideBook(theirTree, "the other side")
	if why != "" {
		return g, why, hint, nil
	}
	baseHist, err := readOr(baseTree, s.ask.Root, s.hist)
	if err != nil {
		return g, "", "", err
	}
	theirHist, err := readOr(theirTree, s.ask.Root, s.hist)
	if err != nil {
		return g, "", "", err
	}
	if why := s.mergedLines(baseHist, theirHist); why != "" {
		return g, why, mergeLines, nil
	}
	return s.mergedFields(g, baseBook, theirBook)
}

// sideBook is the book of one side of a merge, an empty one where the side
// holds none, with why it cannot be judged: its rulebook cannot be read, or
// stands in another folder than the merge's. A side read at the merge's
// folder while its rulebook stands elsewhere would read as empty, and every
// change on it would pass as the merge base's.
func (s *staged) sideBook(tree port.Tree, name string) (model.Book, string, string) {
	l, err := format.Load(tree, s.p.Schema, s.ask.Root)
	switch {
	case errors.Is(err, ErrNoBook):
		return model.Book{}, "", ""
	case err != nil:
		return model.Book{}, name + "'s rulebook could not be read: " + err.Error(), "gragas validate"
	case l.Dir != s.dir:
		return model.Book{}, fmt.Sprintf("%s holds the rulebook in %s, and the merge in %s: a merge is judged only where the rulebook stands on every side", name, orRoot(l.Dir), orRoot(s.dir)), mergeMoved
	}
	return l.Book, "", ""
}

// mergedLines is why history.jsonl after the merge is not ours's lines and
// then, byte for byte, each line the other side added since the merge base
// that ours lacks, in that side's order and each ended by a newline; ""
// when it is.
func (s *staged) mergedLines(baseHist, theirHist []byte) string {
	for _, h := range []struct {
		name string
		data []byte
	}{{"the other side's", theirHist}, {s.n.before + "'s", s.headHist}} {
		if !bytes.HasPrefix(h.data, baseHist) {
			return fmt.Sprintf("%s %s does not keep every line the merge base has, byte for byte", h.name, s.hist)
		}
	}
	ours := map[string]bool{}
	for _, l := range lifecycle.Lines(s.headHist) {
		ours[string(l)] = true
	}
	theirs := map[string]bool{}
	var want [][]byte
	for _, l := range lifecycle.Lines(theirHist[len(baseHist):]) {
		theirs[string(l)] = true
		if !ours[string(l)] {
			want = append(want, l)
		}
	}
	tail := s.staging[len(s.headHist):]
	n := lineAt(s.staging, len(s.headHist))
	got := 0
	for ; len(tail) > 0; n++ {
		l, rest, ended := bytes.Cut(tail, []byte("\n"))
		switch {
		case !ended:
			return fmt.Sprintf("line %d of %s is not ended by a newline", n, s.hist)
		case !theirs[string(l)]:
			return fmt.Sprintf("line %d of %s is new in this merge: the other side's history does not hold it as it is", n, s.hist)
		case got >= len(want) || !bytes.Equal(l, want[got]):
			return fmt.Sprintf("line %d of %s is the other side's, out of that side's order or twice", n, s.hist)
		}
		got++
		tail = rest
	}
	if got < len(want) {
		return fmt.Sprintf("the merge leaves out a line the other side's %s holds, %d of the %d it adds that %s lacks: %s", s.hist, got+1, len(want), s.n.before, want[got])
	}
	return ""
}

// mergedFields refuses a lifecycle value the merge holds otherwise than the
// side that changed it since the merge base leaves it. Each value is held
// whole, key by key (lifecycle.Values): a rule either side added, each of
// its fields, each exclusion and each bindings/ record, never a sum of
// them such as a list's count.
func (s *staged) mergedFields(g model.Guard, baseBook, theirBook model.Book) (model.Guard, string, string, error) {
	base, ours := lifecycle.Values(baseBook, shown), lifecycle.Values(s.before, shown)
	theirs, merged := lifecycle.Values(theirBook, shown), lifecycle.Values(s.after, shown)
	var keys []string
	for _, m := range []map[string]model.Guarded{base, ours, theirs, merged} {
		for k := range m {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range slices.Compact(keys) {
		b, o, t, m := base[k].After, ours[k].After, theirs[k].After, merged[k].After
		v := firstHolding(k, merged, theirs, ours, base)
		why, hint := "", mergeFields
		switch {
		case o != b && t != b && o != t:
			why = fmt.Sprintf("both sides change %s of %s since the merge base, %s to %s and the other side to %s: no line records the value a merge picks", v.Field, v.Entity, s.n.before, orNone(o), orNone(t))
			hint = mergeBoth
			if v.Entity == "exclusions.yaml" {
				// two branches took one number (A38): a hand renumber on
				// either is a change with no line, and no command removes
				// a record, so one side's change is dropped before merging
				hint = mergeTwice
			}
		case t != b && m != t:
			why = fmt.Sprintf("the merge leaves %s of %s at %s, and the other side's history leaves it at %s", v.Field, v.Entity, valueOr(m, b), orNone(t))
		case o != b && t == b && m != o:
			why = fmt.Sprintf("the merge leaves %s of %s at %s, and %s holds %s", v.Field, v.Entity, valueOr(m, b), s.n.before, orNone(o))
		case o == b && t == b && m != b:
			why = fmt.Sprintf("the merge changes %s of %s to %s, and neither side's history records it", v.Field, v.Entity, orNone(m))
		}
		if why != "" {
			g.Changes = append(g.Changes, model.Guarded{Entity: v.Entity, Field: v.Field, Before: b, After: m, Command: commandOf(baseBook, s.after, v)})
			return g, why, hint, nil
		}
	}
	return g, "", "", s.ctx.Err()
}

// firstHolding is the entity and field key names, from the first of ms that
// holds it.
func firstHolding(k string, ms ...map[string]model.Guarded) model.Guarded {
	for _, m := range ms {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return model.Guarded{}
}

// commandOf is the command GuardEdit names for v's change from before to
// after: the one of its entity and field, else of its entity, as a list's
// change is named whole; "" where it names none.
func commandOf(before, after model.Book, v model.Guarded) string {
	cs := lifecycle.GuardEdit(before, after, shown)
	for _, c := range cs {
		if c.Entity == v.Entity && c.Field == v.Field {
			return c.Command
		}
	}
	for _, c := range cs {
		if c.Entity == v.Entity {
			return c.Command
		}
	}
	return ""
}

// valueOr is the merge's value of a field, or "the merge base's value"
// where the merge leaves it as the merge base has it.
func valueOr(m, base string) string {
	if m == base {
		return "the merge base's value"
	}
	return orNone(m)
}

// orNone is a field's value as a refusal says it.
func orNone(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

// short is a commit's name as a refusal says it.
func short(id string) string { return id[:min(12, len(id))] }

// unrecorded judges a merge git commits with no conflict, as its
// pre-merge-commit hook meets it: git writes MERGE_HEAD only after the hook,
// so the other side cannot be named, and the merge cannot be held to the
// three-way rule. A merge that leaves every lifecycle value
// and history.jsonl as HEAD holds them lands no lifecycle change, and
// passes. Any other is refused: git then keeps the merge, and git commit
// runs the pre-commit hook, which judges it with its other side named.
func (s *staged) unrecorded(g model.Guard) (model.Guard, string, string, error) {
	before, after := lifecycle.Values(s.before, shown), lifecycle.Values(s.after, shown)
	same := bytes.Equal(s.headHist, s.staging) && len(before) == len(after)
	for k, v := range before {
		same = same && after[k] == v
	}
	if same {
		return g, "", "", s.ctx.Err()
	}
	return g, "git runs the pre-merge-commit hook before it records the merge, so a merge that changes the rulebook's lifecycle data or its history cannot be judged here. git keeps the merge, and the pre-commit hook judges it when it is committed", mergeCommit, nil
}
