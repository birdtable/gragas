package rulebook

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"

	"github.com/birdtable/gragas/internal/ordered"
)

// Invariant is one count the rulebook holds to a baseline that may only
// fall: a gap the migration to schema v2 left, counted honestly.
type Invariant struct {
	// Name is its key in baseline.json.
	Name string
	// About says what it counts.
	About string
	count func(b *Book, r *ordered.Map) int
}

// Invariants are the counts held to a baseline, in report order.
func Invariants() []Invariant {
	return []Invariant{
		{"rules-without-source", "rules not retired that cite no source; an active one is refused outright",
			func(_ *Book, r *ordered.Map) int { return live(r, !r.Truthy("sources")) }},
		{"hard-rules-without-check", "Hard rules not retired with no check",
			func(_ *Book, r *ordered.Map) int {
				return live(r, r.Str("kind") == "rule" && r.Str("strength") == "Hard" && !r.Truthy("checks"))
			}},
		{"checks-unresolved", "checks of rules not retired whose tool or key does not resolve",
			func(b *Book, r *ordered.Map) int {
				return liveChecks(r, func(c *ordered.Map) bool { return !b.Resolves(c) })
			}},
		{"relates-unknown", "relates entries whose id is no rule",
			func(b *Book, r *ordered.Map) int {
				n := 0
				for _, x := range ordered.Maps(r.List("relates")) {
					if _, ok := b.Rules[x.Str("id")]; !ok {
						n++
					}
				}
				return n
			}},
		{"checks-without-test", "pattern, analyzer, policy and fact checks whose test is missing",
			func(b *Book, r *ordered.Map) int {
				return liveChecks(r, func(c *ordered.Map) bool {
					return slices.Contains(testedClasses(), c.Str("class")) && !b.testExists(c)
				})
			}},
		{"checks-without-fix", "checks that say nothing of a fix",
			func(_ *Book, r *ordered.Map) int {
				return liveChecks(r, func(c *ordered.Map) bool { return !c.Truthy("fix") })
			}},
		{"enforced-without-examples", "enforced rules with no examples",
			func(b *Book, r *ordered.Map) int {
				return bit(b.standingOf(r) == Enforced && !r.Truthy("examples"))
			}},
		{"measures-not-one-fact", "measures not held by exactly one fact check",
			func(_ *Book, r *ordered.Map) int { return live(r, r.Str("kind") == "measure" && !oneFact(r)) }},
		{"advisory-with-checks", "instructions and Review rules that carry checks",
			func(_ *Book, r *ordered.Map) int {
				return live(r, (r.Str("kind") == "instruction" || r.Str("strength") == "Review") && r.Truthy("checks"))
			}},
	}
}

func bit(ok bool) int {
	if ok {
		return 1
	}
	return 0
}

// live counts a rule that is not retired.
func live(r *ordered.Map, ok bool) int { return bit(ok && r.Str("status") != "retired") }

func liveChecks(r *ordered.Map, bad func(*ordered.Map) bool) int {
	if r.Str("status") == "retired" {
		return 0
	}
	n := 0
	for _, c := range ordered.Maps(r.List("checks")) {
		n += bit(bad(c))
	}
	return n
}

func oneFact(r *ordered.Map) bool {
	checks := ordered.Maps(r.List("checks"))
	return len(checks) == 1 && checks[0].Str("class") == "fact"
}

// CheckBaseline holds each count to baseline.json: above it is a finding,
// and so is below it, so the file is lowered in the commit that earns it.
// Load leaves it to the caller, so a test of the other checks needs no
// baseline.
func (b *Book) CheckBaseline() error {
	p := path.Join(Dir, BaselineFile)
	data, err := fs.ReadFile(b.src, p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		b.add(p, finding{msg: "missing: every invariant needs a baseline"})
		return nil
	case err != nil:
		return fmt.Errorf("read %s: %w", p, err)
	}
	v, err := ordered.ParseYAML(data)
	doc, ok := v.(*ordered.Map)
	if err != nil || !ok || doc.Sub("baselines") == nil {
		b.add(p, finding{msg: "not a JSON object with baselines"})
		return nil
	}
	base := doc.Sub("baselines")
	names := make([]string, 0, len(Invariants()))
	for _, inv := range Invariants() {
		names = append(names, inv.Name)
		b.compare(p, base, inv.Name)
	}
	for _, k := range base.Keys() {
		if !slices.Contains(names, k) {
			b.add(p, finding{line: base.KeyLine(k), msg: k + " is no invariant"})
		}
	}
	return nil
}

func (b *Book) compare(p string, base *ordered.Map, name string) {
	got := b.Counts[name]
	v, ok := base.Get(name)
	want, isInt := v.(int64)
	switch {
	case !ok || !isInt:
		b.add(p, finding{msg: fmt.Sprintf("%s has no baseline; it counts %d now", name, got)})
	case int64(got) > want:
		b.add(p, finding{line: base.KeyLine(name), msg: fmt.Sprintf("%s rose to %d, above its baseline of %d: a baseline may only fall", name, got, want)})
	case int64(got) < want:
		b.add(p, finding{line: base.KeyLine(name), msg: fmt.Sprintf("%s fell to %d: lower its baseline from %d to %d", name, got, want, got)})
	}
}
