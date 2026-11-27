package layers

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// A repo carries what it must call and what it may not hold in two files the rulebook names no product for:
// .gragas/uses.yaml, the repo's picks from the register (or from a first-party library), the ban row its lint
// holds for each, the jobs that are not for this repo and its findings, and .gragas/library.yaml, which a
// first-party library carries: each package, its job and the banned calls of that job (supply.intake.6). The
// uses file is the canonical form: it holds every fact of the published matrix, which is generated from it.
const (
	UsesFile      = ".gragas/uses.yaml"
	UsesSchema    = "uses"
	LibraryFile   = ".gragas/library.yaml"
	LibrarySchema = "library"
)

// The states of a finding. An open finding is held by its ticket until the ticket lands, a later one waits for
// a change that is not scheduled, and a closed one stays in the file one release as the record. A file that
// says none is open, or later when the finding carries the `later` mark of the matrix.
const (
	FindingOpen   = "open"
	FindingLater  = "later"
	FindingClosed = "closed"
)

// ErrUses is a uses file or a library file that does not hold: to itself, to its schema or to the register.
var ErrUses = errors.New("the uses file does not hold")

// A Pick is one job the repo does with a library: the register key, written as a row key, tool:<tool> or std:<name>
// for the standard library, or first-party:<path>, the ban row its lint holds, and the calls of the job it may no
// longer hold. A job with no import to ban says "none: review only" in ban and carries a pattern.
type Pick struct {
	Item        string
	RegisterKey string
	Job         string
	Scope       string
	Ban         string
	BannedCalls []string
	Pattern     string
	Sources     []string
	AdoptedBy   []string
	Conditional string
	Line        int
}

// A NotFor is a job the repo does not do, with why and a source. `not used here` is allowed only for a job listed here.
type NotFor struct {
	Item    string
	Why     string
	Sources []string
	Line    int
}

// A Finding is a hand-written copy standing in the repo: where, what it declares (a symbol) or uses (a pattern),
// or both, the ticket that removes it and its state. A cut finding names no replacing library: the plan deletes
// the code. Lines is the count the matrix gives, which is a number, a text such as "about 60 sites" or nothing.
type Finding struct {
	Where      string
	What       string
	Lines      any
	Symbol     string
	Pattern    string
	ReplacedBy []string
	RemovedBy  []string
	Sources    []string
	Note       string
	KeptIssue  string
	Carried    bool
	Cut        bool
	Later      bool
	State      string
	Line       int
}

// A Blocker is a ticket a repo's work waits for, and why.
type Blocker struct {
	Ticket string
	Why    string
}

// Uses is the uses file of a repo.
type Uses struct {
	Repo          string
	PlanKeyPrefix string
	IssueKey      string
	IsLibrary     bool
	Related       []string
	Picks         []Pick
	MustUse       []string
	NotFor        []NotFor
	Findings      []Finding
	Blockers      []Blocker
	Notes         []string
}

// A LibraryPackage is one package of a first-party library: its job and the calls of that job a program may no
// longer hold.
type LibraryPackage struct {
	Package     string
	Job         string
	BannedCalls []string
	Line        int
}

// FirstParty is the library file of a first-party library.
type FirstParty struct{ Packages []LibraryPackage }

var (
	// pickKey is how a pick names its library: a register row, tool:<tool> or std:<name>, or first-party:<path>.
	pickKey    = regexp.MustCompile(`^([a-z][a-z0-9-]*|(tool|std):[a-z0-9][a-z0-9/._-]*|first-party:[a-z0-9][a-z0-9-]*(/[a-z0-9][a-z0-9._-]*)+)$`)
	callName   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*(\.[A-Za-z][A-Za-z0-9]*)?$`)
	repoName   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	packageRel = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*(/[a-z0-9][a-z0-9._-]*)*$`)
	bareKey    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// unique says a key written twice, with the line of the first, and records this one.
func (p *parser) unique(seen map[string]int, line int, what, key string) {
	if at, twice := seen[key]; twice {
		p.add(line, "%s %s is there twice (line %d)", what, key, at)
	}
	seen[key] = line
}

// ParseUses reads a repo's uses file and holds it to its schema through validate.
func ParseUses(data []byte, validate func(doc any) []string) (*Uses, error) {
	doc, err := readDoc(data, validate, UsesFile)
	if err != nil {
		return nil, err
	}
	p := &parser{}
	p.keys(doc, []string{"repo", "plan_key_prefix", "issue_key", "is_library", "related", "picks", "must_use", "not_for_this_repo", "findings", "blockers", "notes"}, []string{"repo", "picks"})
	p.placeholders(doc, "", doc.Line)
	u := &Uses{Repo: doc.Str("repo"), PlanKeyPrefix: doc.Str("plan_key_prefix"), IssueKey: doc.Str("issue_key"), IsLibrary: doc.Truthy("is_library")}
	if !repoName.MatchString(u.Repo) {
		p.add(doc.KeyLine("repo"), "repo %q is not a repository name", u.Repo)
	}
	u.Related = p.strs(doc, "related")
	u.Notes = p.textOrList(doc, "notes")
	seen := map[string]int{}
	for _, m := range ordered.Maps(doc.List("picks")) {
		pk := p.pick(m)
		p.unique(seen, m.Line, "pick", pk.Item)
		u.Picks = append(u.Picks, pk)
	}
	if len(u.Picks) == 0 {
		p.add(doc.Line, "picks holds no pick")
	}
	u.MustUse = p.strs(doc, "must_use")
	for _, k := range u.MustUse {
		if !pickKey.MatchString(k) {
			p.add(doc.KeyLine("must_use"), "must_use %q is a register key or a first-party path", k)
		}
	}
	for _, m := range ordered.Maps(doc.List("not_for_this_repo")) {
		u.NotFor = append(u.NotFor, p.notFor(m))
	}
	for _, m := range ordered.Maps(doc.List("findings")) {
		u.Findings = append(u.Findings, p.finding(m))
	}
	for _, m := range ordered.Maps(doc.List("blockers")) {
		p.keys(m, []string{"ticket", "why"}, []string{"ticket", "why"})
		u.Blockers = append(u.Blockers, Blocker{Ticket: m.Str("ticket"), Why: strings.TrimSpace(m.Str("why"))})
	}
	p.heldByItself(u)
	if len(p.bad) > 0 {
		return nil, fmt.Errorf("%s: %w: %s", UsesFile, ErrUses, strings.Join(p.bad, "; "))
	}
	return u, nil
}

// readDoc reads strictly, holds the schema's answer and returns the mapping.
func readDoc(data []byte, validate func(doc any) []string, file string) (*ordered.Map, error) {
	if validate == nil {
		return nil, fmt.Errorf("%s: %w: no schema check was given, so nothing was read", file, ErrUses)
	}
	v, err := ordered.ParseStrict(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %w", file, ErrUses, err)
	}
	if bad := validate(v); len(bad) > 0 {
		return nil, fmt.Errorf("%s: %w: %s", file, ErrUses, strings.Join(bad, "; "))
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return nil, fmt.Errorf("%s: %w: not a mapping", file, ErrUses)
	}
	return doc, nil
}

// strs reads a list of non-empty one-line strings.
func (p *parser) strs(m *ordered.Map, key string) []string {
	var out []string
	for _, s := range m.List(key) {
		str, ok := s.(string)
		if !ok || strings.TrimSpace(str) == "" || strings.ContainsFunc(str, control) {
			p.add(m.KeyLine(key), "%s holds an item that is not a one-line string", key)
			continue
		}
		out = append(out, str)
	}
	return out
}

// textOrList reads a field the matrix writes as one text or as a list of texts, and returns it as a list: one
// spelling of the same facts.
func (p *parser) textOrList(m *ordered.Map, key string) []string {
	if s, ok := m.Get(key); ok {
		if str, isText := s.(string); isText {
			if strings.TrimSpace(str) == "" {
				p.add(m.KeyLine(key), "%s is empty", key)
				return nil
			}
			return []string{str}
		}
	}
	var out []string
	for _, s := range m.List(key) {
		str, ok := s.(string)
		if !ok || strings.TrimSpace(str) == "" {
			p.add(m.KeyLine(key), "%s holds an item that is not a text", key)
			continue
		}
		out = append(out, str)
	}
	return out
}

func (p *parser) pick(m *ordered.Map) Pick {
	p.keys(m, []string{"item", "register_key", "job", "scope", "ban", "banned_calls", "pattern", "sources", "adopted_by", "conditional"},
		[]string{"item", "register_key", "job", "scope", "ban"})
	pk := Pick{Item: m.Str("item"), RegisterKey: m.Str("register_key"), Job: strings.TrimSpace(m.Str("job")),
		Scope: strings.TrimSpace(m.Str("scope")), Ban: strings.TrimSpace(m.Str("ban")), Pattern: m.Str("pattern"),
		Sources: p.textOrList(m, "sources"), AdoptedBy: p.strs(m, "adopted_by"), Conditional: strings.TrimSpace(m.Str("conditional")), Line: m.Line}
	if !pickKey.MatchString(pk.RegisterKey) {
		p.add(m.KeyLine("register_key"), "pick %q names no register key and no first-party path", pk.Item)
	}
	for _, c := range p.strs(m, "banned_calls") {
		if !callName.MatchString(c) {
			p.add(m.KeyLine("banned_calls"), "pick %s: banned call %q is an import path or a package and a name", pk.Item, c)
		}
		pk.BannedCalls = append(pk.BannedCalls, c)
	}
	noImport := strings.HasPrefix(pk.Ban, "none")
	switch {
	case len(pk.BannedCalls) == 0 && !noImport:
		p.add(m.Line, "pick %s: a job with no banned call says `none: review only` in ban", pk.Item)
	case len(pk.BannedCalls) == 0 && pk.Pattern == "":
		p.add(m.Line, "pick %s: a job with no import to ban carries a pattern, so a review has something to look for", pk.Item)
	}
	if pk.Pattern != "" {
		if _, err := regexp.Compile(pk.Pattern); err != nil {
			p.add(m.KeyLine("pattern"), "pick %s: pattern %q does not compile as a Go regexp", pk.Item, pk.Pattern)
		}
	}
	return pk
}

func (p *parser) notFor(m *ordered.Map) NotFor {
	p.keys(m, []string{"item", "why", "sources"}, []string{"item", "why", "sources"})
	n := NotFor{Item: m.Str("item"), Why: strings.TrimSpace(m.Str("why")), Sources: p.textOrList(m, "sources"), Line: m.Line}
	if strings.TrimSpace(n.Item) == "" || n.Why == "" {
		p.add(m.Line, "a job that is not for this repo names the job and says why")
	}
	if len(n.Sources) == 0 {
		p.add(m.Line, "not_for_this_repo %q has no source: `not used here` is allowed only with one", n.Item)
	}
	return n
}

func (p *parser) finding(m *ordered.Map) Finding {
	p.keys(m, []string{"where", "what", "lines", "replaced_by", "removed_by", "sources", "pattern", "symbol", "carried", "cut", "later", "kept_issue", "note", "state"},
		[]string{"where", "what"})
	f := Finding{Where: m.Str("where"), What: m.Str("what"), Symbol: m.Str("symbol"), Pattern: m.Str("pattern"),
		ReplacedBy: p.strs(m, "replaced_by"), RemovedBy: p.strs(m, "removed_by"), Sources: p.textOrList(m, "sources"),
		Note: m.Str("note"), KeptIssue: m.Str("kept_issue"), Carried: m.Truthy("carried"), Cut: m.Truthy("cut"),
		Later: m.Truthy("later"), State: m.Str("state"), Line: m.Line}
	if v, ok := m.Get("lines"); ok {
		switch n := v.(type) {
		case nil, string:
			f.Lines = n
		case int64:
			if n < 0 {
				p.add(m.KeyLine("lines"), "finding at %s: lines is a count", f.Where)
			}
			f.Lines = n
		default:
			p.add(m.KeyLine("lines"), "finding at %s: lines is a count, a text or nothing", f.Where)
		}
	}
	if f.State == "" {
		f.State = FindingOpen
		if f.Later {
			f.State = FindingLater
		}
	}
	switch {
	case !slices.Contains([]string{FindingOpen, FindingLater, FindingClosed}, f.State):
		p.add(m.KeyLine("state"), "finding at %s: state %q is not one of open, later, closed", f.Where, f.State)
	case f.Later && f.State != FindingLater:
		p.add(m.Line, "finding at %s: it is marked later and its state is %s", f.Where, f.State)
	case f.Symbol == "" && f.Pattern == "":
		p.add(m.Line, "finding at %s: it carries a symbol or a pattern, or both", f.Where)
	case len(f.RemovedBy) == 0 && !f.Carried:
		p.add(m.Line, "finding at %s: it names the ticket that removes it, or is marked carried", f.Where)
	case f.Cut && len(f.ReplacedBy) > 0:
		p.add(m.Line, "finding at %s: a cut finding names no replacing library", f.Where)
	case !f.Cut && len(f.ReplacedBy) == 0:
		p.add(m.Line, "finding at %s: it names the library that replaces it, or is cut", f.Where)
	}
	if f.Pattern != "" {
		if _, err := regexp.Compile(f.Pattern); err != nil {
			p.add(m.KeyLine("pattern"), "finding at %s: pattern %q does not compile as a Go regexp", f.Where, f.Pattern)
		}
	}
	return f
}

// heldByItself holds the file to itself: each must_use is a pick, and no pick is also a job that is not for the repo.
func (p *parser) heldByItself(u *Uses) {
	picked := map[string]bool{}
	for _, pk := range u.Picks {
		picked[pk.RegisterKey] = true
	}
	for _, k := range u.MustUse {
		if !picked[k] {
			p.add(0, "must_use %s is no pick of the file", k)
		}
	}
	for _, n := range u.NotFor {
		if picked[n.Item] {
			p.add(n.Line, "%s is a pick and a job that is not for this repo", n.Item)
		}
	}
}

// Hold holds the file to the register: a pick that names a row names a chosen one, a tool: pick names a tool the
// register points to, and a finding replaced by a bare register key names a row. A std: or first-party pick names
// nothing of the register, and a replacing path or module is not read against it.
func (u *Uses) Hold(r *Register) error {
	rows := map[string]Library{}
	for _, l := range r.Rows {
		rows[l.Key] = l
	}
	tools := map[string]bool{}
	for _, t := range r.Tools {
		tools[t.Key] = true
	}
	var bad []string
	for _, pk := range u.Picks {
		bad = append(bad, heldPick(pk, rows, tools)...)
	}
	for _, f := range u.Findings {
		for _, k := range f.ReplacedBy {
			if _, ok := rows[k]; bareKey.MatchString(k) && !ok {
				bad = append(bad, fmt.Sprintf("line %d: finding at %s is replaced by %s, which has no row", f.Line, f.Where, k))
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%s: %w: %s", UsesFile, ErrUses, strings.Join(bad, "; "))
	}
	return nil
}

// ParseLibrary reads a first-party library's library file.
func ParseLibrary(data []byte, validate func(doc any) []string) (*FirstParty, error) {
	doc, err := readDoc(data, validate, LibraryFile)
	if err != nil {
		return nil, err
	}
	p := &parser{}
	p.keys(doc, []string{"packages"}, []string{"packages"})
	p.placeholders(doc, "", doc.Line)
	l := &FirstParty{}
	seen := map[string]int{}
	for _, m := range ordered.Maps(doc.List("packages")) {
		p.keys(m, []string{"package", "job", "banned_calls"}, []string{"package", "job", "banned_calls"})
		pk := LibraryPackage{Package: m.Str("package"), Job: strings.TrimSpace(m.Str("job")), Line: m.Line}
		if !packageRel.MatchString(pk.Package) {
			p.add(m.KeyLine("package"), "package %q is a path in the library's module", pk.Package)
		}
		if pk.Job == "" {
			p.add(m.Line, "package %s names no job", pk.Package)
		}
		for _, c := range p.strs(m, "banned_calls") {
			if !callName.MatchString(c) {
				p.add(m.KeyLine("banned_calls"), "package %s: banned call %q is an import path or a package and a name", pk.Package, c)
			}
			pk.BannedCalls = append(pk.BannedCalls, c)
		}
		if len(pk.BannedCalls) == 0 {
			p.add(m.Line, "package %s names no banned call: the hand-written copy a program may no longer hold", pk.Package)
		}
		p.unique(seen, m.Line, "package", pk.Package)
		l.Packages = append(l.Packages, pk)
	}
	if len(l.Packages) == 0 {
		p.add(doc.Line, "packages holds no package")
	}
	if len(p.bad) > 0 {
		return nil, fmt.Errorf("%s: %w: %s", LibraryFile, ErrUses, strings.Join(p.bad, "; "))
	}
	return l, nil
}

// heldPick is what the register says against one pick: its tool the register does not point to, its key with no
// row, its row that is neither chosen nor conditional, or its conditional row when the pick does not say it is conditional.
func heldPick(pk Pick, rows map[string]Library, tools map[string]bool) []string {
	tool, isTool := strings.CutPrefix(pk.RegisterKey, "tool:")
	switch l, ok := rows[pk.RegisterKey]; {
	case isTool && !tools[tool]:
		return []string{fmt.Sprintf("line %d: pick %s names tool %s, which the register does not point to", pk.Line, pk.Item, tool)}
	case isTool || !bareKey.MatchString(pk.RegisterKey):
		return nil
	case !ok:
		return []string{fmt.Sprintf("line %d: pick %s names register key %s, which has no row", pk.Line, pk.Item, pk.RegisterKey)}
	case l.State == StateConditional && pk.Conditional == "":
		return []string{fmt.Sprintf("line %d: pick %s names row %s, which is conditional, and the pick does not say it is conditional", pk.Line, pk.Item, l.Key)}
	case l.State != StateChosen && l.State != StateConditional:
		return []string{fmt.Sprintf("line %d: pick %s names row %s, which is %s, not chosen", pk.Line, pk.Item, l.Key, l.State)}
	}
	return nil
}
