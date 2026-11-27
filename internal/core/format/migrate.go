package format

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// V2 is the owner's schema v2 rulebook as the migration reads it: the text of
// each file, by path relative to the rulebook's folder. Only the files the
// migration reads are in it.
type V2 map[string][]byte

// Carried is how many of each kind of thing the migration carries over.
type Carried struct {
	Rules, Bindings, Sources, Guides, Kinds int
}

// NotCarried is one thing of the old layout with no home in the new one nor
// in the parked file, and why. Item is the file and field, the source, or the
// id.
type NotCarried struct {
	Item, Reason string
}

// Migration is the plan of a move: the files it writes, what carries over, what
// is parked, what is neither, and the facets it turns into kinds.
type Migration struct {
	Targets []model.Target
	Carried Carried
	// Parked is the side file of every value with no field at format 3.
	Parked     Parked
	NotCarried []NotCarried
	// Strengths is each strength of format 3 with the old words that map to
	// it and the count of rules it was given.
	Strengths []Strength
	// Relations is each old how word with the relation of format 3 it goes to
	// and the count of relations that held it; Wins is how many relations
	// named a winner, each parked.
	Relations []Relation
	Wins      int
	// Converted are the facets of the old layout that became kinds (S73).
	Converted []string
	// Statements is the digest of each statement the conversion writes, by
	// rule id, for the history entry to record; empty, not nil,
	// when no rule is carried.
	Statements map[model.RuleID]model.Digest
}

// MigrateProblem is the first thing wrong with the identity the new layout
// needs and the old one never held, "" when there is none. It names the flag.
func MigrateProblem(f model.InitForm) string {
	g := f
	g.Layer, g.Part = model.Named{Slug: "x", Name: "x"}, model.Named{Slug: "x", Name: "x"}
	return InitProblem(g)
}

// rungs maps an old check class to a rung of the ladder.
func rungs() map[string]int {
	return map[string]int{"preset-rule": 0, "preset-config": 1, "analyzer": 3, "hook": 3, "fact": 4, "test-kit": 5}
}

// runPoints maps an old check tier to the point it runs at.
func runPoints() map[string]string {
	return map[string]string{"0": "edit", "1": "commit", "2": "push", "3": "land", "4": "release", "R": "review", "G": "deploy"}
}

// targets maps an old binding word to a target type.
func targets() map[string]string {
	return map[string]string{"go": "source", "ts": "source", "any": "source", "sql": "migration"}
}

// sourceKinds maps an old source kind to the new one; a kind with no new one
// is absent.
func sourceKinds() map[string]string {
	return map[string]string{
		"standard": "standard", "article": "guide", "book": "guide", "paper": "study", "incident": "incident",
		"decision": "decision-record", "vendor-doc": "vendor-document", "target": "own-decision",
	}
}

// V2Files are the paths of v2 the migration reads, from the list of files of
// the rulebook's folder.
func V2Files(all []string) []string {
	var out []string
	for _, p := range all {
		switch {
		case p == "parts.yaml", p == "products.yaml", p == "sources.yaml", p == "ids.lock":
			out = append(out, p)
		case strings.Contains(p, "/rules/") && strings.HasSuffix(p, ".yaml") && strings.Count(p, "/") == 3,
			strings.HasSuffix(p, "/guide.md") && strings.Count(p, "/") == 2:
			out = append(out, p)
		}
	}
	return out
}

// IsV2 says the folder holds the old layout: a parts file, and no identity.
func IsV2(files []string) bool {
	var parts, identity bool
	for _, p := range files {
		parts = parts || p == "parts.yaml"
		identity = identity || p == IdentityFile
	}
	return parts && !identity
}

type migrator struct {
	v2  V2
	m   Migration
	bad []string
	// parked is every value parked, by subject, then field.
	parked *ordered.Map
}

func (g *migrator) skip(item, reason string) {
	g.m.NotCarried = append(g.m.NotCarried, NotCarried{item, reason})
}

func (g *migrator) doc(p string) (*ordered.Map, error) {
	data, ok := g.v2[p]
	if !ok {
		return nil, fmt.Errorf("%s is not in the rulebook", p)
	}
	v, err := ordered.ParseYAML(data)
	if err != nil {
		return nil, fmt.Errorf("%s could not be read: %w", p, err)
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		return nil, fmt.Errorf("%s is not a map", p)
	}
	return m, nil
}

// MigrateV2 plans the move of a schema v2 rulebook to the format: identity and
// structure from the parts and products, one rules file per concern, sources,
// the ids that survive. It reads nothing but v2 and decides nothing outside it.
func MigrateV2(v2 V2, id model.InitForm) (Migration, error) {
	if p := MigrateProblem(id); p != "" {
		return Migration{}, errors.New(p)
	}
	g := &migrator{v2: v2, parked: newDoc(), m: Migration{Statements: map[model.RuleID]model.Digest{}, Strengths: strengthFolds(), Relations: relationFolds()}}
	parts, err := g.doc("parts.yaml")
	if err != nil {
		return Migration{}, err
	}
	products, err := g.doc("products.yaml")
	if err != nil {
		return Migration{}, err
	}
	kinds, err := g.kinds(parts, products)
	if err != nil {
		return Migration{}, err
	}
	concerns := map[string][]concernRef{}
	var ruleFiles []string
	for p := range v2 {
		if strings.Contains(p, "/rules/") {
			ruleFiles = append(ruleFiles, p)
		}
	}
	slices.Sort(ruleFiles)
	have := map[string]bool{}
	for _, f := range ruleFiles {
		doc, err := g.doc(f)
		if err != nil {
			return Migration{}, err
		}
		g.unread(f, nil, doc, "part", "concern", "order", "title", "rules")
		part, concern := doc.Str("part"), strings.TrimSuffix(path.Base(f), ".yaml")
		concerns[part] = append(concerns[part], concernRef{slug: concern, name: doc.Str("title"), order: intOf(doc.Val("order"))})
		out, ids, err := g.rules(doc, part, concern, f, kinds)
		if err != nil {
			return Migration{}, err
		}
		for _, i := range ids {
			have[i] = true
		}
		g.m.Targets = append(g.m.Targets, model.Target{Path: f, Data: []byte(out)})
	}
	g.unparked("parts.yaml", parts, "layers", "parts")
	g.unparked("products.yaml", products, "kinds")
	if err := g.sources(); err != nil {
		return Migration{}, err
	}
	g.ids(have)
	for _, p := range sortedKeys(v2) {
		if strings.HasSuffix(p, "/guide.md") {
			g.m.Carried.Guides++
		}
	}
	structure := g.structure(parts, concerns, kinds)
	if p := g.problem(); p != "" {
		return Migration{}, errors.New(p)
	}
	g.m.Targets = append(g.m.Targets,
		model.Target{Path: IdentityFile, Data: []byte(identityText(id))},
		model.Target{Path: StructureFile, Data: []byte(structure)})
	g.parkedTarget()
	return g.m, nil
}

type concernRef struct {
	slug, name string
	order      int
}

func sortedKeys(m V2) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// kind is one kind of the new structure.
type kind struct{ slug, name, description string }

// kinds are the product kinds of the old products file and the facets of the
// old parts, each facet a kind of the same name (S73).
func (g *migrator) kinds(parts, products *ordered.Map) ([]kind, error) {
	var out []kind
	seen := map[string]bool{}
	for _, k := range ordered.Maps(products.List("kinds")) {
		g.unread("kind "+k.Str("key"), nil, k, "key", "name", "description", "instances")
		if _, ok := k.Get("instances"); ok {
			g.skip("kind "+k.Str("key")+" instances", notParked("instances"))
		}
		out = append(out, kind{k.Str("key"), k.Str("name"), strings.TrimSpace(k.Str("description"))})
		seen[k.Str("key")] = true
	}
	for _, p := range ordered.Maps(parts.List("parts")) {
		g.unread("part "+p.Str("short"), nil, p, "short", "layer", "name", "facets", "owner")
		for _, f := range ordered.Strings(p.List("facets")) {
			if seen[f] {
				continue
			}
			seen[f] = true
			out = append(out, kind{f, f, "a facet of the part " + p.Str("short") + " before the format; a rule scoped to it names this kind"})
			g.m.Converted = append(g.m.Converted, f)
		}
	}
	g.m.Carried.Kinds = len(out)
	return out, nil
}

func (g *migrator) structure(parts *ordered.Map, concerns map[string][]concernRef, kinds []kind) string {
	doc := newDoc()
	put(doc, "format", model.FormatVersion)
	var layers []any
	for _, l := range ordered.Maps(parts.List("layers")) {
		g.unread("layer "+l.Str("key"), nil, l, "key", "name")
		lm := newDoc()
		put(lm, "slug", l.Str("key"))
		put(lm, "name", l.Str("name"))
		var ps []any
		for _, p := range ordered.Maps(parts.List("parts")) {
			if p.Str("layer") != l.Str("key") {
				continue
			}
			pm := newDoc()
			put(pm, "slug", p.Str("short"))
			put(pm, "name", p.Str("name"))
			if o := p.Str("owner"); o != "" {
				put(pm, "owners", []any{o})
			}
			cs := concerns[p.Str("short")]
			slices.SortStableFunc(cs, func(a, b concernRef) int { return a.order - b.order })
			var cl []any
			for _, c := range cs {
				cm := newDoc()
				put(cm, "slug", c.slug)
				put(cm, "name", c.name)
				cl = append(cl, cm)
			}
			put(pm, "concerns", orEmpty(cl))
			ps = append(ps, pm)
		}
		put(lm, "parts", ps)
		layers = append(layers, lm)
	}
	put(doc, "layers", layers)
	var ks []any
	for _, k := range kinds {
		km := newDoc()
		put(km, "slug", k.slug)
		put(km, "name", k.name)
		if k.description != "" {
			put(km, "description", k.description)
		}
		ks = append(ks, km)
	}
	put(doc, "kinds", orEmpty(ks))
	var b strings.Builder
	emitYAML(&b, doc, 0)
	return b.String()
}

func orEmpty(l []any) []any {
	if l == nil {
		return []any{}
	}
	return l
}

// sources carries sources.yaml: the keyed map of v2 becomes a list.
func (g *migrator) sources() error {
	doc, err := g.doc("sources.yaml")
	if err != nil {
		return err
	}
	src := doc.Sub("sources")
	if src == nil {
		src = doc
	}
	out := newDoc()
	put(out, "format", model.FormatVersion)
	var list []any
	for _, slug := range src.Keys() {
		s := src.Sub(slug)
		if s == nil {
			continue
		}
		g.unread("source "+slug, nil, s, "kind", "title", "locator", "verified")
		g.unread("source "+slug, []string{"verified"}, s.Sub("verified"), "on")
		kind, ok := sourceKinds()[s.Str("kind")]
		if !ok {
			g.skip("source "+slug, "the kind "+s.Str("kind")+" has no kind at format 3")
			continue
		}
		sm := newDoc()
		put(sm, "slug", slug)
		put(sm, "kind", kind)
		put(sm, "title", strings.TrimSpace(s.Str("title")))
		put(sm, "locator", s.Str("locator"))
		if v := s.Sub("verified"); v != nil && v.Str("on") != "" {
			put(sm, "verified", v.Str("on"))
		}
		list = append(list, sm)
		g.m.Carried.Sources++
	}
	put(out, "sources", orEmpty(list))
	var b strings.Builder
	emitYAML(&b, out, 0)
	g.m.Targets = append(g.m.Targets, model.Target{Path: "sources.yaml", Before: digestOf(g.v2["sources.yaml"]), Data: []byte(b.String())})
	return nil
}

// ids checks the ids the lock holds against the rules: an id that is a rule
// stays that rule's id, one that is not has no rule to keep it as an alias of.
func (g *migrator) ids(have map[string]bool) {
	for line := range strings.SplitSeq(string(g.v2["ids.lock"]), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || have[line] {
			continue
		}
		g.skip("id "+line, "the lock holds it and no rule does, so no rule can keep it as an alias")
	}
}

// rules writes one concern file at format 3.
func (g *migrator) rules(doc *ordered.Map, part, concern, file string, kinds []kind) (string, []string, error) {
	out := newDoc()
	put(out, "format", model.FormatVersion)
	put(out, "part", part)
	put(out, "concern", concern)
	var list []any
	var ids []string
	for _, r := range ordered.Maps(doc.List("rules")) {
		id := r.Str("id")
		ids = append(ids, id)
		list = append(list, g.rule(r, id))
		g.m.Carried.Rules++
	}
	put(out, "rules", orEmpty(list))
	var b strings.Builder
	emitYAML(&b, out, 0)
	_ = file
	return b.String(), ids, nil
}

// rule is one old rule as a rule of the format: the parts that map one to one
// here, the parts with a rule of their own in the helpers below.
func (g *migrator) rule(r *ordered.Map, id string) *ordered.Map {
	nr := newDoc()
	put(nr, "id", id)
	statement := strings.TrimSpace(r.Str("rule"))
	put(nr, "statement", statement)
	g.m.Statements[model.RuleID(id)] = StatementDigest(statement)
	put(nr, "strength", g.strength(id, r.Str("strength")))
	status := r.Str("status")
	if status == "later" {
		// later is not a status at format 3: the rule is proposed, and the
		// old word is parked.
		status = "proposed"
		g.park(id, nil, "status", "later")
	}
	if !slices.Contains([]string{"proposed", "trial", "active", "retired"}, status) {
		g.refuse(id, "status", status)
	}
	put(nr, "status", status)
	put(nr, "origin", "migrated")
	putIf(nr, "scope", scopeOf(r.Sub("when")))
	putIf(nr, "exemptions", exemptionsOf(r))
	putIf(nr, "reasoning", reasoningOf(r))
	putIf(nr, "relations", g.relations(r, id))
	putIf(nr, "sources", citationsOf(r))
	putIf(nr, "bindings", g.bindingsOf(r, id))
	if v := r.Str("replaced_by"); v != "" {
		put(nr, "replaced_by", v)
	}
	g.unread(id, nil, r, "id", "rule", "strength", "status", "when", "unless", "detail", "why", "relates", "sources", "checks", "replaced_by")
	g.unread(id, []string{"when"}, r.Sub("when"), "kinds", "facets", "paths")
	for i, u := range ordered.Maps(r.List("unless")) {
		g.unread(id, []string{"unless", strconv.Itoa(i + 1)}, u, "kinds", "facets", "paths", "reason")
	}
	for i, c := range ordered.Maps(r.List("sources")) {
		g.unread(id, []string{"sources", strconv.Itoa(i + 1)}, c, "ref", "where", "says", "backs")
	}
	g.reorder(id, r.Keys())
	return nr
}

// putIf sets a key to v unless v is nil or an empty list or map.
func putIf(m *ordered.Map, key string, v any) {
	switch x := v.(type) {
	case nil:
		return
	case []any:
		if len(x) == 0 {
			return
		}
	case *ordered.Map:
		if x == nil || x.Len() == 0 {
			return
		}
	}
	put(m, key, v)
}

// kindsAndPaths is the paths and kinds (facets count as kinds) of an old when
// or unless, as the map the new scope and exemptions are made of.
func kindsAndPaths(w *ordered.Map) *ordered.Map {
	out := newDoc()
	if w == nil {
		return out
	}
	if p := ordered.Strings(w.List("paths")); len(p) > 0 {
		put(out, "paths", anyStrings(p))
	}
	if k := append(ordered.Strings(w.List("kinds")), ordered.Strings(w.List("facets"))...); len(k) > 0 {
		put(out, "kinds", anyStrings(k))
	}
	return out
}

func scopeOf(w *ordered.Map) any {
	sc := kindsAndPaths(w)
	if sc.Len() == 0 {
		return nil
	}
	return sc
}

func exemptionsOf(r *ordered.Map) []any {
	var ex []any
	for _, u := range ordered.Maps(r.List("unless")) {
		em := kindsAndPaths(u)
		put(em, "reason", strings.TrimSpace(u.Str("reason")))
		ex = append(ex, em)
	}
	return ex
}

func reasoningOf(r *ordered.Map) any {
	rs := newDoc()
	if d := strings.TrimSpace(r.Str("detail")); d != "" {
		put(rs, "definition", d)
	}
	if w := strings.TrimSpace(r.Str("why")); w != "" {
		put(rs, "pros", []any{w})
	}
	return rs
}

func citationsOf(r *ordered.Map) []any {
	var out []any
	for _, c := range ordered.Maps(r.List("sources")) {
		cm := newDoc()
		put(cm, "ref", c.Str("ref"))
		for _, f := range []string{"where", "says"} {
			if v := strings.TrimSpace(c.Str(f)); v != "" {
				put(cm, f, v)
			}
		}
		put(cm, "backs", c.Str("backs"))
		out = append(out, cm)
	}
	return out
}

// bindingsOf turns the checks that name a tool and a key into bindings. One
// that names neither is no binding and is parked whole; of one that does, a
// field with no home (the note, and the fix, which names no fixer program, so
// remedy stays unset) is parked beside its tool and key.
func (g *migrator) bindingsOf(r *ordered.Map, id string) []any {
	var bs []any
	for i, c := range ordered.Maps(r.List("checks")) {
		item := fmt.Sprintf("%s check %d", id, i+1)
		at := []string{"checks", strconv.Itoa(i + 1)}
		tool, key := c.Str("tool"), c.Str("key")
		if tool == "" || key == "" {
			g.unread(id, at, c)
			continue
		}
		bound := []string{"tool", "key", "binding", "class", "tier"}
		if slices.ContainsFunc(c.Keys(), func(k string) bool { return !slices.Contains(bound, k) }) {
			g.unread(id, at, c, "binding", "class", "tier")
		}
		bm := newDoc()
		put(bm, "tool", tool)
		put(bm, "target", g.mapped(item, "binding", c.Str("binding"), targets()))
		put(bm, "key", key)
		put(bm, "severity", "error")
		rung, ok := rungs()[c.Str("class")]
		if !ok {
			g.refuse(item, "class", c.Str("class"))
		}
		put(bm, "rung", rung)
		put(bm, "run_point", g.mapped(item, "tier", c.Str("tier"), runPoints()))
		bs = append(bs, bm)
		g.m.Carried.Bindings++
	}
	return bs
}

func anyStrings(l []string) []any {
	out := make([]any, len(l))
	for i, s := range l {
		out[i] = s
	}
	return out
}
