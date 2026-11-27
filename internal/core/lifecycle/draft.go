package lifecycle

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/markdown"
	"github.com/birdtable/gragas/internal/model"
)

// preamble is the anchor of the lines that stand above the guide's first
// heading, which have no heading to take an anchor from.
const preamble = "preamble"

// Draft is one rule read from a guide: the line that carries a strength word,
// or, for a section with no such line, its heading. It holds the one-line
// statement a maintainer edits afterwards and where it stands; no other guide
// prose (S50). A draft is proposed until it is matched to a config entry.
type Draft struct {
	// ID is part.concern.n, numbered in the order the guide gives the drafts,
	// so the same guide at the same start gives the same ids.
	ID string
	// Anchor is the section the draft stands in, the heading's anchor.
	Anchor string
	// Statement is the guide's line, or the heading's text for a section with
	// no strength line.
	Statement string
	// Strength is the word's strength; a heading is a should to be confirmed.
	Strength model.Strength
	// Line is the line of the guide the draft is read from.
	Line int
	// Todo says what a person must settle, "" for none.
	Todo string
}

// strengthOf is the strength a strength word names.
func strengthOf(word string) model.Strength {
	switch word {
	case "MUST":
		return model.Must
	case "SHOULD":
		return model.Should
	}
	return model.May
}

// DraftsOf reads the drafts of a guide: a draft per MUST, SHOULD or MAY line
// and, for a section with no such line, one for its heading. A line with two
// strength words takes the first and a to-do to keep one. Numbers start at
// first, in document order.
func DraftsOf(doc *markdown.Doc, part, concern string, first int) []Draft {
	var out []Draft
	add := func(d Draft) {
		d.ID = fmt.Sprintf("%s.%s.%d", part, concern, first+len(out))
		out = append(out, d)
	}
	lines := map[string][]markdown.Statement{}
	for _, s := range doc.Statements {
		a := s.Section
		if a == "" {
			a = preamble
		}
		lines[a] = append(lines[a], s)
	}
	section := func(anchor, heading string, line int) {
		if sts := lines[anchor]; len(sts) > 0 {
			for _, s := range sts {
				d := Draft{Anchor: anchor, Statement: s.Text, Strength: strengthOf(s.Words[0]), Line: s.Line}
				if len(s.Words) > 1 {
					d.Todo = "the line carries " + strings.Join(s.Words, " and ") + "; keep one strength"
				}
				add(d)
			}
			return
		}
		// a setext heading may span lines; the statement is one line
		if heading = strings.Join(strings.Fields(heading), " "); heading != "" {
			add(Draft{Anchor: anchor, Statement: heading, Strength: model.Should, Line: line,
				Todo: "the heading carries no strength word; set the rule and its strength"})
		}
	}
	section(preamble, "", 0)
	for _, h := range doc.Headings {
		section(h.Anchor, h.Text, h.Line)
	}
	return out
}

// slugOfTag is a tag as the words of a slug.
func slugOfTag(tag string) string {
	var w strings.Builder
	dash := true
	for _, r := range strings.ToLower(tag) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			w.WriteRune(r)
			dash = false
		} else if !dash {
			w.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(w.String(), "-")
}

// ImportDecisionSlug is the slug of the decision a guide's import records.
func ImportDecisionSlug(g model.GuideRef) string {
	return "imported-" + g.Slug + "-" + slugOfTag(g.Tag)
}

// guideLocator is where the guide is found at its tag: the one place a
// citation of it depends on.
func guideLocator(g model.GuideRef) string { return g.URL + "@" + g.Tag }

func checkGuide(b model.Book, r model.DraftGuide) error {
	g := r.Guide
	switch {
	case !hasPart(b, r.Part):
		return refuse("name a part of structure.yaml with --into-part", "the part %q is not in the structure", r.Part)
	case !hasConcern(b, r.Part, r.Concern):
		return refuse("name a concern of "+r.Part+" with --into-concern", "the concern %q is not a concern of %s", r.Concern, r.Part)
	case !validSlug(g.Slug):
		return refuse("write the guide's source as lower-case words joined by hyphens", "the guide slug %q is not a slug", g.Slug)
	case strings.TrimSpace(g.Title) == "" || !clean(g.Title) || strings.TrimSpace(g.URL) == "" || !clean(g.URL):
		return refuse("give the guide a title and a URL, each on one line", "the guide has no title or no URL, or one runs over more than one line")
	case slugOfTag(g.Tag) == "" || !clean(g.Tag):
		return refuse("give the guide's release tag on one line", "the guide has no tag")
	case !clean(g.Attribution):
		return refuse("write the attribution on one line", "the attribution runs over more than one line")
	}
	if s, ok := sourceOf(b, g.Slug); ok && s.Locator != guideLocator(g) {
		return refuse("pick another slug, or import the release the source records", "the source %s is in the book at %s, not at %s", g.Slug, s.Locator, guideLocator(g))
	}
	return nil
}

// PlanDraftGuide plans the drafts of a guide: the guide as a source, one
// decision for the import, and a rule for each draft, proposed, origin
// imported, citing the guide by anchor. A draft whose statement the concern
// holds already is left alone, so a second run plans nothing.
func PlanDraftGuide(b model.Book, doc *markdown.Doc, r model.DraftGuide, m Moment) (Plan, error) {
	return planDrafts(b, []*markdown.Doc{doc}, r, m, nil, nil)
}

// grandfather is what makes a draft an active rule: the entries of the old
// config that hold it, and who owns it and when it is next reviewed.
type grandfather struct {
	entries  []model.ImportEntry
	tool     string
	target   model.TargetType
	owner    string
	reviewBy string
}

// planDrafts plans the drafts of the docs in order. match says which entries
// of the old config hold a draft: with some the rule enters active with those
// entries as its bindings, with none, or no match function, proposed. extra
// are lines the plan screen adds to its to-dos.
func planDrafts(b model.Book, docs []*markdown.Doc, r model.DraftGuide, m Moment, match func(Draft) *grandfather, extra func() []string) (Plan, error) {
	if err := checkGuide(b, r); err != nil {
		return Plan{}, err
	}
	drafts := newDrafts(b, docs, r)
	grand := make([]*grandfather, len(drafts))
	if match != nil {
		for i, d := range drafts {
			grand[i] = match(d)
		}
	}
	if len(drafts) == 0 {
		return Plan{Record: Record{Kind: "import", Entity: "source:" + r.Guide.Slug, After: sourceLifecycle(model.Source{Locator: guideLocator(r.Guide)})},
			Sections: sections([]string{"nothing: the concern holds every rule of this guide already"}, nil, "none")}, nil
	}
	d, changes, written := guideChanges(b, r, m)
	decision := ImportDecisionSlug(r.Guide)
	rules, err := draftChanges(b, r, drafts, grand, decision)
	if err != nil {
		return Plan{}, err
	}
	d.Changes = append(d.Changes, rules...)
	s := showDrafts(drafts, grand, r, decision, match != nil)
	changes = append(changes, s.plain...)
	if len(s.gf) > 0 {
		changes = append(changes, fmt.Sprintf("grandfathered (%d), imported from the guide at %s:", len(s.gf), r.Guide.Tag))
		changes = append(changes, s.gf...)
	}
	if len(s.pr) > 0 {
		changes = append(changes, fmt.Sprintf("proposed (%d):", len(s.pr)))
		changes = append(changes, s.pr...)
	}
	if extra != nil {
		s.todos = append(s.todos, extra()...)
	}
	for _, t := range s.todos {
		changes = append(changes, "to do: "+t)
	}
	written = append([]string{fmt.Sprintf("%s: adds %d rule(s)", r.Files.Rules, len(drafts))}, written...)
	written = append(written, fmt.Sprintf("%s: appends %d entries (import)", r.Files.History, len(s.recs)))
	consumers := "none: a proposed rule binds no project"
	if len(s.gf) > 0 {
		consumers = "none yet: each grandfathered binding is unproven until its pair is written, and no preset changes"
	}
	return Plan{Delta: d, Record: s.recs[0], Also: s.recs[1:], Sections: sections(changes, written, consumers)}, nil
}

// newDrafts are the drafts of the docs the concern does not hold already,
// numbered from the concern's next number.
func newDrafts(b model.Book, docs []*markdown.Doc, r model.DraftGuide) []Draft {
	prefix := r.Part + "." + r.Concern + "."
	held := func(statement string) bool {
		return slices.ContainsFunc(b.Rules, func(x model.Rule) bool {
			return x.Statement == statement && x.Status != model.Retired && strings.HasPrefix(string(x.ID), prefix)
		})
	}
	var drafts []Draft
	for _, doc := range docs {
		for _, d := range DraftsOf(doc, r.Part, r.Concern, 1) {
			if !held(d.Statement) {
				drafts = append(drafts, d)
			}
		}
	}
	for i := range drafts {
		drafts[i].ID = fmt.Sprintf("%s%d", prefix, nextNumber(b, r.Part, r.Concern)+i)
	}
	return drafts
}

// guideChanges adds the guide as a source and the import's decision when the
// book has neither: the delta, and the plan's lines for what changes and what
// is written where.
func guideChanges(b model.Book, r model.DraftGuide, m Moment) (model.Delta, []string, []string) {
	var d model.Delta
	var changes, written []string
	if !hasSource(b, r.Guide.Slug) {
		g := r.Guide
		var body strings.Builder
		fmt.Fprintf(&body, "slug: %s\nkind: %s\ntitle: %s\nlocator: %s\n", g.Slug, model.SourceGuide, scalar(g.Title), scalar(guideLocator(g)))
		if g.Attribution != "" {
			fmt.Fprintf(&body, "attribution: %s\n", scalar(g.Attribution))
		}
		d.Changes = append(d.Changes, model.Change{Entity: "source:" + g.Slug, After: body.String()})
		changes = append(changes, "add source "+g.Slug+" (guide, "+guideLocator(g)+")")
		written = append(written, r.Files.Sources+": adds source "+g.Slug)
	}
	decision := ImportDecisionSlug(r.Guide)
	if !hasDecision(b, decision) {
		d.Changes = append(d.Changes, model.Change{Entity: "decision:" + decision, After: fmt.Sprintf(
			"slug: %s\ntitle: %s\ntext: %s\nwho: %s\ndate: %s\n", decision, scalar("Imported from "+r.Guide.Title+" at "+r.Guide.Tag),
			scalar("The rules were drafted from the guide at its tag and cite it by section."), scalar(m.Who), scalar(m.Date))})
		changes = append(changes, "add decision "+decision)
		written = append(written, r.Files.Decisions+": adds decision "+decision)
	}
	return d, changes, written
}

// shown is how the plan screen tells the drafts: a line for each draft when no
// config was read (plain), else the proposed and the grandfathered ones, the
// to-dos and the history records.
type shown struct {
	plain, gf, pr, todos []string
	recs                 []Record
}

// showDrafts tells each draft; matched is whether a config was read.
func showDrafts(drafts []Draft, grand []*grandfather, r model.DraftGuide, decision string, matched bool) shown {
	var s shown
	for i, dr := range drafts {
		g := grand[i]
		switch {
		case g == nil && !matched:
			s.plain = append(s.plain, fmt.Sprintf("draft %s: %s, %s, from #%s line %d", dr.ID, dr.Strength, model.Proposed, dr.Anchor, dr.Line))
		case g == nil:
			s.pr = append(s.pr, fmt.Sprintf("%s: %s, proposed, ships in no preset, from #%s line %d", dr.ID, dr.Strength, dr.Anchor, dr.Line))
		default:
			s.gf = append(s.gf, grandLine(dr, g, r))
			s.todos = append(s.todos, dr.ID+": write the pair that proves its binding")
		}
		if dr.Todo != "" {
			s.todos = append(s.todos, dr.ID+": "+dr.Todo)
		}
		after := model.Lifecycle{Status: model.Proposed}
		if g != nil {
			after = model.Lifecycle{Status: model.Active, Owner: g.owner, ReviewBy: g.reviewBy}
		}
		s.recs = append(s.recs, Record{Kind: "import", Entity: "rule:" + dr.ID, After: after, Decision: decision})
	}
	return s
}

// grandLine is the plan's line for a grandfathered draft.
func grandLine(dr Draft, g *grandfather, r model.DraftGuide) string {
	keys := make([]string, len(g.entries))
	for j, e := range g.entries {
		keys[j] = g.tool + " " + e.Key
	}
	return fmt.Sprintf("%s: %s, active, origin imported, source %s#%s, binding %s, pair to write", dr.ID, dr.Strength, r.Guide.Slug, dr.Anchor, strings.Join(keys, " and "))
}

// draftChanges are the changes that add the drafts: a rule apiece to a concern
// file, or, when the concern is written in a guide, one block for each anchor
// holding every rule drafted under it, or a rule added to the block the anchor
// has already.
func draftChanges(b model.Book, r model.DraftGuide, drafts []Draft, grand []*grandfather, decision string) ([]model.Change, error) {
	var out []model.Change
	if !strings.HasSuffix(r.Files.Rules, ".md") {
		for i, dr := range drafts {
			out = append(out, model.Change{Entity: "rule:" + dr.ID, After: draftBody(r, dr, decision, grand[i])})
		}
		return out, nil
	}
	var anchors []string
	for _, dr := range drafts {
		if !slices.Contains(anchors, dr.Anchor) {
			anchors = append(anchors, dr.Anchor)
		}
	}
	for _, a := range anchors {
		out = append(out, anchorChanges(b, r, a, drafts, grand, decision)...)
	}
	return out, nil
}

// anchorChanges adds the drafts of one anchor: a rule to the block the anchor
// has already, or one new block holding them all.
func anchorChanges(b model.Book, r model.DraftGuide, a string, drafts []Draft, grand []*grandfather, decision string) []model.Change {
	entity := "block:" + r.Files.Rules + "#" + a
	exists := slices.ContainsFunc(b.Rules, func(x model.Rule) bool { return b.Anchors[x.ID] == a })
	var out []model.Change
	var block strings.Builder
	if !exists {
		fmt.Fprintf(&block, "anchor: %s\nrules:\n", a)
	}
	for i, dr := range drafts {
		if dr.Anchor != a {
			continue
		}
		body := draftBody(r, dr, decision, grand[i])
		if exists {
			out = append(out, model.Change{Entity: entity, Field: "rules[]", After: body})
			continue
		}
		block.WriteString(indentRule(body))
	}
	if !exists {
		out = append(out, model.Change{Entity: entity, After: block.String()})
	}
	return out
}

// indentRule is a rule's mapping as an item of a block's rules list.
func indentRule(body string) string {
	var w strings.Builder
	for i, l := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		if i == 0 {
			w.WriteString("  - " + l + "\n")
		} else {
			w.WriteString("    " + l + "\n")
		}
	}
	return w.String()
}

// draftBody is a drafted rule as the mapping Render adds: imported, citing the
// guide at the draft's anchor. A grandfathered rule enters active with its
// owner, its review date and a binding for each entry of the old config that
// holds it, each at the severity the config had and with no pair, so the
// binding is unproven until its pair is written.
func draftBody(r model.DraftGuide, d Draft, decision string, g *grandfather) string {
	status := model.Proposed
	if g != nil {
		status = model.Active
	}
	var w strings.Builder
	fmt.Fprintf(&w, "id: %s\nstatement: %q\nstrength: %s\nstatus: %s\norigin: %s\n", d.ID, d.Statement, d.Strength, status, model.Imported)
	if g != nil {
		fmt.Fprintf(&w, "owner: %s\nreview_by: %q\n", scalar(g.owner), g.reviewBy)
	}
	fmt.Fprintf(&w, "reasoning:\n  decision: %s\nsources:\n  - ref: %s\n    where: %s\n    backs: %s\n", decision, r.Guide.Slug, scalar("#"+d.Anchor), model.BacksRule)
	if g != nil {
		w.WriteString("bindings:\n")
		for _, e := range g.entries {
			fmt.Fprintf(&w, "  - tool: %s\n    target: %s\n    key: %s\n", g.tool, g.target, scalar(e.Key))
			if e.Value != "" {
				fmt.Fprintf(&w, "    value: %s\n", yamlValue(e.Value))
			}
			if e.Options != "" {
				fmt.Fprintf(&w, "    options: %s\n", e.Options)
			}
			fmt.Fprintf(&w, "    severity: %s\n    rung: 1\n    run_point: %s\n", e.Severity, model.RunPush)
		}
	}
	return w.String()
}

// yamlValue is a printed entry value as YAML, never as syntax of its own: a
// value that is one JSON value on one line (a number, a literal, a list, a
// map or a quoted string) is written as it is, which is valid YAML flow and
// reads back as the value link reads from the config; any other text is one
// quoted string.
func yamlValue(v string) string {
	if oneLineJSON(v) {
		return v
	}
	return strconv.Quote(v)
}
