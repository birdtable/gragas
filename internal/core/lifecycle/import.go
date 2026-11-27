package lifecycle

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/birdtable/gragas/internal/markdown"
	"github.com/birdtable/gragas/internal/model"
)

// PlanImport plans the import of a guide and the config a project keeps
// already: the drafts of the guide (PlanDraftGuide), each matched by name to
// the entries of the config. A draft whose entry is in the config enters
// active, origin imported, the guide its source, with a binding at the
// severity the entry had and no pair, so the binding is unproven; its history
// entry stands on the import's decision, "imported from the guide at its tag".
// A draft with no entry enters proposed and ships in no preset. An entry no
// draft names is a to-do. A rule already in the concern is left alone, so a
// second run plans nothing (idempotent by statement and id). The trial applies
// to rules new since the import, so none is planned here.
func PlanImport(b model.Book, docs []*markdown.Doc, r model.ImportRequest, m Moment) (Plan, error) {
	if len(docs) == 0 && len(r.Entries) == 0 {
		return Plan{}, refuse("name the guide's folder and the config files to bring in", "there is no guide and no config to import")
	}
	if len(docs) == 0 {
		return Plan{}, refuse("give the guide's folder: its rules are what the config's entries are matched to", "the config has no guide to name its rules")
	}
	day, err := time.Parse("2006-01-02", m.Date)
	if err != nil {
		return Plan{}, refuse("take the day as YYYY-MM-DD", "the day of the change %q is not a date", m.Date)
	}
	// a grandfathered rule answers to the rulebook's own owner: m.Who names
	// the command that made the change, not a person
	var owner string
	if len(b.Identity.Owners) > 0 {
		owner = b.Identity.Owners[0]
	}
	days := b.Identity.Governance.ReviewDays
	reviewBy := day.AddDate(0, 0, days).Format("2006-01-02")
	used := make([]bool, len(r.Entries))
	named := draftsNamed(b, docs, r)
	match := func(d Draft) *grandfather {
		var g grandfather
		for i, e := range r.Entries {
			if named[i] == 1 && nameMatches(e.Key, d) {
				g.entries = append(g.entries, e)
				used[i] = true
			}
		}
		if len(g.entries) == 0 {
			return nil
		}
		g.tool, g.target, g.owner, g.reviewBy = r.Tool, r.Target, owner, reviewBy
		return &g
	}
	todo := func() []string { return unmatched(r.Entries, used, named) }
	p, err := planDrafts(b, docs, r.Draft, m, match, todo)
	if err != nil {
		return Plan{}, err
	}
	if len(p.Delta.Changes) > 0 && owner == "" && grandfathered(p) {
		return Plan{}, refuse("name an owner in the rulebook's owners", "the rulebook names no owner, so a grandfathered rule has nobody to answer for it")
	}
	if len(p.Delta.Changes) > 0 && days < 1 && grandfathered(p) {
		return Plan{}, refuse("set review_days in the rulebook's governance", "the governance gives no review cadence, so an active rule has no date to be reviewed by")
	}
	return p, nil
}

func grandfathered(p Plan) bool {
	return slices.ContainsFunc(p.Also, func(r Record) bool { return r.After.Status == model.Active }) || p.Record.After.Status == model.Active
}

// nameMatches is whether the key of a config entry names the draft: the key,
// as words, is the draft's section anchor or runs whole inside the words of
// its statement. A key of fewer than three letters names nothing, so a short
// word cannot grandfather a rule by chance.
func nameMatches(key string, d Draft) bool {
	k := words(key)
	if len(strings.Join(k, "")) < 3 {
		return false
	}
	if slices.Equal(k, words(d.Anchor)) {
		return true
	}
	return contains(words(d.Statement), k)
}

// words are the lower-case runs of letters and digits of s.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') })
}

// contains is whether sub is a run of whole words of ws.
func contains(ws, sub []string) bool {
	for i := 0; i+len(sub) <= len(ws); i++ {
		if slices.Equal(ws[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

// draftsNamed is how many drafts each entry names. An entry that names more
// than one grandfathers none of them: one entry would make several rules
// active, so it is a to-do instead.
func draftsNamed(b model.Book, docs []*markdown.Doc, r model.ImportRequest) []int {
	named := make([]int, len(r.Entries))
	for _, d := range newDrafts(b, docs, r.Draft) {
		for i, e := range r.Entries {
			if nameMatches(e.Key, d) {
				named[i]++
			}
		}
	}
	return named
}

// unmatched is the to-do line of each entry that grandfathered nothing: one
// that names no drafted rule, and one that names several.
func unmatched(entries []model.ImportEntry, used []bool, named []int) []string {
	var out []string
	for i, e := range entries {
		if named[i] > 1 {
			out = append(out, fmt.Sprintf("the entry %s of %s names %d drafted rules: write the rule that holds it, or name one of them", e.Key, e.Config, named[i]))
		} else if !used[i] {
			out = append(out, fmt.Sprintf("the entry %s of %s names no drafted rule: write the rule that holds it", e.Key, e.Config))
		}
	}
	return out
}
