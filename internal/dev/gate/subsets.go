package gate

import (
	"errors"
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/ordered"
)

// ReasonsDir is where the reason record a subset names lives, as
// <decision>.md.
const ReasonsDir = "docs/reasons"

// subsets holds the subset builds' list (arch.layers.13, gragas#341): each
// subset's name is its own, and each decision it names, the row's and each
// allowed package's, is a record ReasonsDir lists by that exact name.
// Against the base the list only shrinks: a row that lets more in than the
// base's (another build, entry or package, a forbidden entry or a marker
// dropped, no_embeds turned off, a package allowed) stands on a decision
// record the base does not hold, and a row the base has cannot go. So a
// package joins a subset only with a decision record of its own. The builds
// themselves are internal/subset_test.go's and
// explorer/test/subset-builds.test.ts's.
func (r *Registry) subsets() {
	doc := r.docs[Subsets]
	if doc == nil {
		return
	}
	head := map[string]*ordered.Map{}
	for _, s := range ordered.Maps(doc.List("subsets")) {
		name := s.Str("name")
		if head[name] != nil {
			r.add(Subsets, "line %d: the subset %s is listed twice", s.Line, name)
		}
		head[name] = s
		r.decision(name, s.Str("decision"), s.Line)
		for _, a := range ordered.Maps(s.List("allowed")) {
			r.decision(name+" allows "+a.Str("package"), a.Str("decision"), a.Line)
		}
	}
	r.subsetsShrink(head)
}

// decision is a finding when the decision record what names is not there.
func (r *Registry) decision(what, id string, line int) {
	if id == "" {
		return // the schema requires it, and says so
	}
	names, err := r.decisionNames()
	switch {
	case err != nil:
		r.add(Subsets, "line %d: the decision records could not be listed, so %s was not found: %v", line, id, err)
	case !slices.Contains(names, id+".md"):
		r.add(Subsets, "line %d: %s names the decision %s, and %s holds no file named %s.md: a package joins a subset with a decision record", line, what, id, ReasonsDir, id)
	}
}

// decisionNames are the names of the files of ReasonsDir, listed once.
// The record is found by its exact name in the listing, never by opening
// the path: a file system that folds case opens A53.md for a53.md, and git,
// which the base is read from, never does.
func (r *Registry) decisionNames() ([]string, error) {
	if r.names == nil {
		if r.in.Names == nil {
			return nil, errors.New("no listing of the repository was given")
		}
		names, err := r.in.Names(ReasonsDir)
		if err != nil {
			return nil, err
		}
		r.names = names
	}
	return r.names, nil
}

// subsetsShrink holds the head's rows to the base's: each widening stands on
// a decision record that is new against the base.
func (r *Registry) subsetsShrink(head map[string]*ordered.Map) {
	if r.in.Base == nil {
		return
	}
	data, err := r.in.Base(Subsets)
	if err != nil {
		r.add(Subsets, "the base's list could not be read, so it was not held to it: %v", err)
		return
	}
	if data == nil {
		r.lengths = append(r.lengths, "subsets: the base holds no list, so none was held to it")
		return // every row is new
	}
	v, err := ordered.ParseStrict(data)
	base, ok := v.(*ordered.Map)
	if err != nil || !ok {
		r.add(Subsets, "the base's list does not read (%v), so it was not held to it", err)
		return
	}
	for _, b := range ordered.Maps(base.List("subsets")) {
		name := b.Str("name")
		h := head[name]
		if h == nil {
			r.add(Subsets, "the subset %s, which the base holds, is gone: a subset leaves the list only with the decision that ends it, in a change of its own", name)
			continue
		}
		for _, w := range wider(b, h) {
			r.newDecision(Subsets, fmt.Sprintf("line %d: the subset %s %s", h.Line, name, w), h.Str("decision"))
		}
		r.allowedShrink(name, b, h)
	}
}

// allowedShrink holds a subset's allowed packages to the base's: a package
// the base does not allow is a widening, and so is a files list the head
// drops or grows for a package whose list the base holds. Dropping it falls
// back to allowed by name, and a name added lets a file in (A72).
func (r *Registry) allowedShrink(name string, b, h *ordered.Map) {
	was := map[string]*ordered.Map{}
	for _, a := range ordered.Maps(b.List("allowed")) {
		was[a.Str("package")] = a
	}
	for _, a := range ordered.Maps(h.List("allowed")) {
		pkg := a.Str("package")
		ba, had := was[pkg]
		switch {
		case !had:
			r.newDecision(Subsets, fmt.Sprintf("line %d: the subset %s allows %s, which the base does not", a.Line, name, pkg), a.Str("decision"))
		case len(ba.List("files")) == 0:
		case len(a.List("files")) == 0:
			r.newDecision(Subsets, fmt.Sprintf("line %d: the subset %s allows %s with no files list, where the base lists the files it embeds", a.Line, name, pkg), a.Str("decision"))
		default:
			for _, f := range dropped(a.List("files"), ba.List("files")) {
				r.newDecision(Subsets, fmt.Sprintf("line %d: the subset %s lets %s embed %s, which the base's files list does not hold", a.Line, name, pkg, f), a.Str("decision"))
			}
		}
	}
}

// wider is how the head's row h lets more in than the base's row b, past its
// allowed packages: what it builds changed (its build, entry or package: the
// same list over another build holds that build to nothing it was held to),
// each forbidden entry and marker it dropped, and no_embeds turned off.
func wider(b, h *ordered.Map) []string {
	var out []string
	for _, k := range []string{"build", "entry", "package"} {
		if was, now := b.Str(k), h.Str(k); was != now {
			out = append(out, fmt.Sprintf("builds another %s, %q where the base has %q", k, now, was))
		}
	}
	for _, f := range dropped(b.List("forbidden"), h.List("forbidden")) {
		out = append(out, "no longer forbids "+f)
	}
	for _, mk := range dropped(b.List("markers"), h.List("markers")) {
		out = append(out, "no longer searches for the marker "+mk)
	}
	if b.Truthy("no_embeds") && !h.Truthy("no_embeds") {
		out = append(out, "no longer holds no_embeds")
	}
	return out
}

// dropped is the strings of was that now does not hold.
func dropped(was, now []any) []string {
	keep := strs(now)
	return slices.DeleteFunc(strs(was), func(s string) bool { return slices.Contains(keep, s) })
}

// newDecision is a finding, what, unless the decision id is a record the
// base does not hold: a widening stands on a decision of its own.
func (r *Registry) newDecision(file, what, id string) {
	data, err := r.in.Base(ReasonsDir + "/" + id + ".md")
	switch {
	case err != nil:
		r.add(file, "%s, and whether the decision %s is new could not be read: %v", what, id, err)
	case data != nil:
		r.add(file, "%s, on the decision %s, which the base already holds: a package joins a subset only with a decision record of its own", what, id)
	}
}

// strs is the strings of a list.
func strs(l []any) []string {
	out := make([]string, 0, len(l))
	for _, v := range l {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
