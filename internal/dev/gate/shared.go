package gate

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// sharedShrink holds gate/shared.yaml to the base's, as subsetsShrink holds
// the subset list. The file says its allowances only shrink, and the count
// in gate/baselines.yaml lets both sides grow, so this reads each row of the
// base and finds what the head's row lets in that the base's did not: a
// package added to allow, a tests reprieve the base's row did not have (or
// one that now ends on another wait), a row turned back to later, which drops
// its ban, a row removed or renamed, a primitive dropped and an owner moved.
// Each stands on a decision
// record the base does not hold, named by a decision key on that allow entry,
// on the reprieve or on the row; a row that is gone has no row to carry it,
// so the head's removed list names it, by its old capability.
func (r *Registry) sharedShrink() {
	if r.in.Base == nil || r.in.Read == nil {
		return
	}
	cur, err := r.in.Read(Shared)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			r.add(Shared, "could not be read, so it was not held to the base's: %v", err)
		}
		return
	}
	headDoc, head, ok := r.sharedRows(cur, "the head's")
	if !ok {
		return
	}
	data, err := r.in.Base(Shared)
	if err != nil {
		r.add(Shared, "the base's file could not be read, so it was not held to it: %v", err)
		return
	}
	if data == nil {
		r.lengths = append(r.lengths, "shared: the base holds no file, so none was held to it")
		return
	}
	_, base, ok := r.sharedRows(data, "the base's")
	if !ok {
		return
	}
	for _, b := range base {
		h := head[b.Str("capability")]
		if h == nil {
			r.sharedGone(b, headDoc)
			continue
		}
		r.sharedRow(b, h)
	}
}

// sharedRows is a file's rows by capability.
func (r *Registry) sharedRows(data []byte, whose string) (*ordered.Map, map[string]*ordered.Map, bool) {
	v, err := ordered.ParseStrict(data)
	doc, isMap := v.(*ordered.Map)
	if err != nil || !isMap {
		r.add(Shared, "%s file does not read (%v), so it was not held to the base's", whose, err)
		return nil, nil, false
	}
	rows := map[string]*ordered.Map{}
	for _, m := range ordered.Maps(doc.List("shared")) {
		rows[m.Str("capability")] = m
	}
	return doc, rows, true
}

// sharedGone is a finding for a base row the head does not hold, unless the
// head's removed list names its capability on a decision that is new. A row
// renamed is a row gone and another added, so it is named the same way.
func (r *Registry) sharedGone(b, headDoc *ordered.Map) {
	name := b.Str("capability")
	id := ""
	for _, g := range ordered.Maps(headDoc.List("removed")) {
		if g.Str("capability") == name {
			id = g.Str("decision")
		}
	}
	r.sharedDecision(fmt.Sprintf("the row %s, which the base holds, is gone or renamed: its bans are dropped", name), id)
}

// primitiveKeys are a row's primitives, each as its kind and name.
func primitiveKeys(row *ordered.Map) []string {
	var out []string
	for _, p := range ordered.Maps(row.List("primitives")) {
		for _, k := range p.Keys() {
			out = append(out, k+" "+p.Str(k))
		}
	}
	return out
}

// sharedRow is each way the head's row h lets in what the base's row b did
// not.
func (r *Registry) sharedRow(b, h *ordered.Map) {
	name := h.Str("capability")
	was := map[string]bool{}
	for _, a := range ordered.Maps(b.List("allow")) {
		was[a.Str("package")] = true
	}
	for _, a := range ordered.Maps(h.List("allow")) {
		if pkg := a.Str("package"); !was[pkg] {
			r.sharedDecision(fmt.Sprintf("line %d: the row %s allows %s beside its owner, which the base's row does not", a.Line, name, pkg), a.Str("decision"))
		}
	}
	bt, ht := b.Sub("tests"), h.Sub("tests")
	switch {
	case ht == nil:
	case bt == nil:
		r.sharedDecision(fmt.Sprintf("line %d: the row %s spares its test files, which the base's row does not", ht.Line, name), ht.Str("decision"))
	case bt.Str("until") != ht.Str("until") && !renamedWait(bt.Str("until"), ht.Str("until")):
		r.sharedDecision(fmt.Sprintf("line %d: the row %s spares its test files until %s, where the base's reprieve ends on %s", ht.Line, name, ht.Str("until"), bt.Str("until")), ht.Str("decision"))
	}
	if was, now := b.Str("owner"), h.Str("owner"); was != now {
		r.sharedDecision(fmt.Sprintf("line %d: the row %s moves to the owner %s, where the base's is %s: the ban's exclusion follows it", h.Line, name, now, was), h.Str("decision"))
	}
	have := primitiveKeys(h)
	for _, p := range primitiveKeys(b) {
		if !slices.Contains(have, p) {
			r.sharedDecision(fmt.Sprintf("line %d: the row %s no longer bans the primitive %s", h.Line, name, p), h.Str("decision"))
		}
	}
	if b.Str("status") != "later" && h.Str("status") == "later" {
		r.sharedDecision(fmt.Sprintf("line %d: the row %s is later again, where the base's is %s: its ban is dropped", h.Line, name, b.Str("status")), h.Str("decision"))
	}
	// A later row waits on what its until names; one that waits on another
	// thing puts its ban off, as a moved reprieve does. A base row with no
	// until is from before the key, and naming one is no widening.
	if was, now := b.Str("until"), h.Str("until"); b.Str("status") == "later" && h.Str("status") == "later" && was != "" && was != now && !renamedWait(was, now) {
		r.sharedDecision(fmt.Sprintf("line %d: the row %s is later until %s, where the base's waits on %s: its ban is put off", h.KeyLine("until"), name, now, was), h.Str("decision"))
	}
}

// issueNumber is the form an until had before it took words: an issue's
// number, alone or after a repository's name.
var issueNumber = regexp.MustCompile("^([a-z][a-z0-9-]*)?#[0-9]+$")

// renamedWait says whether a base's until, was, and the head's, now, are one
// wait renamed, which is no widening: was names an issue by its number and
// now says a wait in words, with no number. It is the change that put every
// until in words; it holds only against a base that still has a number,
// which the schema refuses from then on, so words that change, words that go
// back to a number, and any other change to the row still stand on a
// decision.
func renamedWait(was, now string) bool {
	return issueNumber.MatchString(was) && now != "" && !strings.Contains(now, "#")
}

// sharedDecision is a finding, what, unless id is a decision record the base
// does not hold.
func (r *Registry) sharedDecision(what, id string) {
	if id == "" {
		r.add(Shared, "%s, and names no decision: a widening stands on a decision record of its own", what)
		return
	}
	names, err := r.decisionNames()
	switch {
	case err != nil:
		r.add(Shared, "%s, and the decision records could not be listed, so %s was not found: %v", what, id, err)
		return
	case !slices.Contains(names, id+".md"):
		r.add(Shared, "%s, and %s holds no file named %s.md: a widening stands on a decision record that exists", what, ReasonsDir, id)
		return
	}
	r.newDecision(Shared, what, id)
}
