package layers

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/birdtable/gragas/internal/ordered"
)

// The register is one row a job that a maintained library does: the library to call, where an import of it
// is allowed, what it replaces, the ban that holds the replaced code out, the gap that stays ours, a source
// and a state. It is read in the shape of the shared registry: strictly, each field held to its form on the
// parsed value, and one problem a line. The rows that are chosen and ban something feed the Go preset
// (supply.intake.6).
const (
	RegisterFile   = "rulebook/lang/go/libraries.yaml"
	RegisterSchema = "libraries"
	RegisterRule   = "supply.intake.6"
)

// The states of a row. A chosen row is built on. A needs-spike row waits for a spike, and a conditional row
// applies only if the proof it names has passed; neither feeds the preset.
const (
	StateChosen      = "chosen"
	StateNeedsSpike  = "needs-spike"
	StateConditional = "conditional"
)

// ErrRegister is a register that does not hold: to itself, to its schema or to the rules it feeds.
var ErrRegister = errors.New("the register of libraries does not hold")

// A Replaced is a module a row refuses, and why.
type Replaced struct {
	Name string
	Why  string
}

// A Forbid is a forbidigo pattern of a row, with the package its calls come from when the pattern names none.
type Forbid struct {
	Pattern string
	Pkg     string
}

// A Library is one row of the register.
type Library struct {
	Key      string
	Job      string
	Module   string
	Version  string
	Released string
	Age      string
	Package  string
	// Licence is the SPDX name of the module's licence; a row that has not been checked says none.
	Licence  string
	Scope    string
	Replaces []Replaced
	ByHand   []string
	Deny     []string
	Forbid   []Forbid
	Planned  []string
	BanNote  string
	Linters  []string
	Gap      string
	Source   string
	State    string
	Proof    string
	Open     []string
	Line     int
}

// Banned is whether the row feeds the preset: it is chosen and bans an import or a call.
func (l Library) Banned() bool {
	return l.State == StateChosen && (len(l.Deny) > 0 || len(l.Forbid) > 0)
}

// A RegisterTool is a tool the register points to, with the file of the rulebook that says which rule uses it.
type RegisterTool struct {
	Key      string
	Version  string
	ToolsRow string
	Replaces string
	Source   string
	Line     int
}

// A Dropped is a name the register no longer holds, and what took its place. It has no row and no ban.
type Dropped struct {
	Name string
	By   string
	Line int
}

// Register is the register of libraries.
type Register struct {
	Rows    []Library
	Tools   []RegisterTool
	Dropped []Dropped
}

var (
	libraryKey = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	pinVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	moduleName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*$`)
	toolsRow   = regexp.MustCompile(`^rulebook/[a-z/]+/tools\.yaml, key [a-z][a-z0-9-]*$`)
)

// ourSourceWords are what a source of ours would name: a source of a row is a module's own page or an upstream
// issue, never a document of the fleet, an audit, an answer or a target-state file.
func ourSource(u *url.URL) string {
	path := strings.ToLower(u.Path)
	for _, w := range []string{"audit", "answers", "target-state", "target_state", "/decisions/"} {
		if strings.Contains(path, w) {
			return "names " + w
		}
	}
	if strings.EqualFold(u.Host, "github.com") && strings.HasPrefix(strings.ToLower(strings.TrimPrefix(u.Path, "/")), "birdtable/") {
		return "is a repository of the owner"
	}
	return ""
}

// ParseRegister reads the register and holds it to its schema through validate, which answers each problem a
// line; with no validate nothing is read.
// either. It is read strictly:
// an anchor, an alias, a tag or a key written twice is refused. Every field is held to its form on the parsed
// value too, so the schema is not the only check.
func ParseRegister(data []byte, validate func(doc any) []string) (*Register, error) {
	if validate == nil {
		return nil, fmt.Errorf("%s: %w: no schema check was given, so nothing was read", RegisterFile, ErrRegister)
	}
	v, err := ordered.ParseStrict(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %w", RegisterFile, ErrRegister, err)
	}
	if bad := validate(v); len(bad) > 0 {
		return nil, fmt.Errorf("%s: %w: %s", RegisterFile, ErrRegister, strings.Join(bad, "; "))
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return nil, fmt.Errorf("%s: %w: not a mapping", RegisterFile, ErrRegister)
	}
	p := &parser{}
	p.keys(doc, []string{"part", "libraries", "tools", "dropped"}, []string{"part", "libraries", "dropped"})
	p.placeholders(doc, "", doc.Line)
	r := &Register{}
	keys := map[string]int{}
	for _, m := range ordered.Maps(doc.List("libraries")) {
		l := p.library(m)
		p.unique(keys, m.Line, "row", l.Key)
		r.Rows = append(r.Rows, l)
	}
	if len(r.Rows) == 0 {
		p.add(doc.Line, "libraries holds no row")
	}
	for _, m := range ordered.Maps(doc.List("tools")) {
		t := p.registerTool(m)
		p.unique(keys, m.Line, "tool", t.Key)
		r.Tools = append(r.Tools, t)
	}
	for _, m := range ordered.Maps(doc.List("dropped")) {
		r.Dropped = append(r.Dropped, p.dropped(m))
	}
	p.heldBy(r)
	if len(p.bad) > 0 {
		return nil, fmt.Errorf("%s: %w: %s", RegisterFile, ErrRegister, strings.Join(p.bad, "; "))
	}
	return r, nil
}

// library reads one row and holds each field to its form.
func (p *parser) library(m *ordered.Map) Library {
	p.keys(m, []string{"key", "job", "use", "scope", "replaces", "by_hand", "ban", "gap", "source", "state", "proof", "open"},
		[]string{"key", "job", "use", "scope", "ban", "gap", "source", "state"})
	l := Library{Key: m.Str("key"), Job: m.Str("job"), Scope: strings.TrimSpace(m.Str("scope")),
		Gap: strings.TrimSpace(m.Str("gap")), Source: m.Str("source"), State: m.Str("state"),
		Proof: strings.TrimSpace(m.Str("proof")), Line: m.Line}
	if !libraryKey.MatchString(l.Key) {
		p.add(m.KeyLine("key"), "row %q is not lower-case words joined by dashes", l.Key)
	}
	for field, text := range map[string]string{"job": l.Job, "scope": l.Scope, "gap": l.Gap} {
		if strings.TrimSpace(text) == "" || strings.ContainsFunc(strings.ReplaceAll(text, "\n", " "), control) {
			p.add(m.Line, "row %s: %s is empty or holds a control character", l.Key, field)
		}
	}
	if !slices.Contains([]string{StateChosen, StateNeedsSpike, StateConditional}, l.State) {
		p.add(m.KeyLine("state"), "row %s: state %q is not one of chosen, needs-spike, conditional", l.Key, l.State)
	}
	switch {
	case l.State == StateConditional && l.Proof == "":
		p.add(m.Line, "row %s: a conditional row names the proof it waits for", l.Key)
	case l.State != StateConditional && l.Proof != "":
		p.add(m.KeyLine("proof"), "row %s: only a conditional row has a proof", l.Key)
	}
	p.source(m, &l)
	p.use(m, &l)
	p.replaces(m, &l)
	p.ban(m, &l)
	l.Open = p.strings(m, "open", l.Key)
	if len(l.Planned) > 0 && len(l.Open) == 0 {
		p.add(m.Line, "row %s: a planned pattern says in open what proves it", l.Key)
	}
	return l
}

// source holds a row's source to an upstream page: an https address of a module or an upstream issue, and none
// of our own.
func (p *parser) source(m *ordered.Map, l *Library) {
	u, err := url.Parse(l.Source)
	switch {
	case err != nil || u.Scheme != "https" || u.Host == "" || strings.ContainsFunc(l.Source, control):
		p.add(m.KeyLine("source"), "row %s: source %q is not an https address", l.Key, l.Source)
	default:
		if why := ourSource(u); why != "" {
			p.add(m.KeyLine("source"), "row %s: source %s %s, and a source is the module's own page or an upstream issue", l.Key, l.Source, why)
		}
	}
}

// use reads the library a row calls: its module, the pin and the age line.
func (p *parser) use(m *ordered.Map, l *Library) {
	u := m.Sub("use")
	if u == nil {
		p.add(m.KeyLine("use"), "row %s: use is not a module, a version and a package", l.Key)
		return
	}
	p.keys(u, []string{"module", "version", "released", "age", "package", "licence"}, []string{"module", "version", "package"})
	l.Module, l.Version, l.Package = u.Str("module"), u.Str("version"), strings.TrimSpace(u.Str("package"))
	l.Licence = strings.TrimSpace(u.Str("licence"))
	if _, present := u.Get("licence"); present && (l.Licence == "" || strings.ContainsFunc(l.Licence, control)) {
		p.add(u.KeyLine("licence"), "row %s: licence is one line, the SPDX name of the module's licence", l.Key)
	}
	l.Released, l.Age = u.Str("released"), u.Str("age")
	for _, k := range []string{"released", "age"} {
		if _, present := u.Get(k); present && u.Str(k) == "" {
			p.add(u.KeyLine(k), "row %s: %s is a date in quotes (\"YYYY-MM-DD\")", l.Key, k)
		}
	}
	if !moduleName.MatchString(l.Module) {
		p.add(u.KeyLine("module"), "row %s: module %q is not an import path", l.Key, l.Module)
	}
	if l.Package == "" {
		p.add(u.Line, "row %s: use names no package", l.Key)
	}
	if !pinVersion.MatchString(l.Version) && !slices.Contains([]string{"floor", "toolchain", "none"}, l.Version) {
		p.add(u.KeyLine("version"), "row %s: version %q is a release (v1.2.3), floor, toolchain or none", l.Key, l.Version)
	}
	if l.State == StateChosen && pinVersion.MatchString(l.Version) && l.Released == "" {
		p.add(u.KeyLine("version"), "row %s: a chosen row's pinned release %s carries the day it was published in released, so the age window can be counted", l.Key, l.Version)
	}
	p.ageLine(u, l)
}

// ageLine checks the release date and the age line of a row against the window.
func (p *parser) ageLine(u *ordered.Map, l *Library) {
	_, rerr := parseDay(l.Released)
	_, perr := parseDay(l.Age)
	switch {
	case l.Released == "" && l.Age != "":
		p.add(u.KeyLine("age"), "row %s: an age line says when a release passes the window, so it needs the release's date in released", l.Key)
	case l.Released != "" && !pinVersion.MatchString(l.Version):
		p.add(u.KeyLine("released"), "row %s: only a release (v1.2.3) has a date", l.Key)
	case l.Released != "" && rerr != nil:
		p.add(u.KeyLine("released"), "row %s: released %q is not a date (YYYY-MM-DD)", l.Key, l.Released)
	case l.Age != "" && perr != nil:
		p.add(u.KeyLine("age"), "row %s: age %q is not a date (YYYY-MM-DD)", l.Key, l.Age)
	}
}

func parseDay(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.DateOnly, s)
}

// replaces reads the modules a row refuses and the hand-written code it refuses.
func (p *parser) replaces(m *ordered.Map, l *Library) {
	for _, rm := range ordered.Maps(m.List("replaces")) {
		p.keys(rm, []string{"name", "why"}, []string{"name", "why"})
		r := Replaced{Name: rm.Str("name"), Why: strings.TrimSpace(rm.Str("why"))}
		if strings.TrimSpace(r.Name) == "" || strings.ContainsFunc(r.Name, control) {
			p.add(rm.Line, "row %s: a replaced module has no name", l.Key)
		}
		l.Replaces = append(l.Replaces, r)
	}
	if len(m.List("replaces")) != len(l.Replaces) {
		p.add(m.KeyLine("replaces"), "row %s: a replaced module is not a name and a why", l.Key)
	}
	l.ByHand = p.strings(m, "by_hand", l.Key)
	if len(l.Replaces) == 0 && len(l.ByHand) == 0 {
		p.add(m.Line, "row %s: a row says what it replaces, a module or hand-written code (supply.intake.6)", l.Key)
	}
}

// ban reads the bans of a row: the import paths denied, the forbidigo patterns, and the patterns that are
// planned until a fixture proves them. A row that bans nothing says why in note.
func (p *parser) ban(m *ordered.Map, l *Library) {
	b := m.Sub("ban")
	if b == nil {
		p.add(m.KeyLine("ban"), "row %s: ban is not a mapping", l.Key)
		return
	}
	p.keys(b, []string{"deny", "forbid", "planned", "note", "linter"}, nil)
	l.BanNote = strings.TrimSpace(b.Str("note"))
	for _, d := range p.strings(b, "deny", l.Key) {
		switch {
		case !moduleName.MatchString(d):
			p.add(b.KeyLine("deny"), "row %s: denied import %q is not an import path", l.Key, d)
		case d == l.Module:
			p.add(b.KeyLine("deny"), "row %s: it denies its own library %s", l.Key, d)
		case slices.Contains(l.Deny, d):
			p.add(b.KeyLine("deny"), "row %s: %s is denied twice", l.Key, d)
		}
		l.Deny = append(l.Deny, d)
	}
	for _, fm := range ordered.Maps(b.List("forbid")) {
		p.keys(fm, []string{"pattern", "pkg"}, []string{"pattern"})
		f := Forbid{Pattern: fm.Str("pattern"), Pkg: fm.Str("pkg")}
		if _, err := regexp.Compile(f.Pattern); err != nil || f.Pattern == "" {
			p.add(fm.Line, "row %s: forbidigo pattern %q does not compile as a Go regexp", l.Key, f.Pattern)
		}
		l.Forbid = append(l.Forbid, f)
	}
	if len(b.List("forbid")) != len(l.Forbid) {
		p.add(b.KeyLine("forbid"), "row %s: a forbidden call is not a pattern and a package", l.Key)
	}
	for _, pl := range p.strings(b, "planned", l.Key) {
		if _, err := regexp.Compile(pl); err != nil || pl == "" {
			p.add(b.KeyLine("planned"), "row %s: planned pattern %q does not compile as a Go regexp", l.Key, pl)
		}
		l.Planned = append(l.Planned, pl)
	}
	if len(l.Deny) == 0 && len(l.Forbid) == 0 && l.BanNote == "" {
		p.add(b.Line, "row %s: it bans nothing and says in ban.note why it is review only", l.Key)
	}
	p.linters(b, l)
}

// strings reads a list of non-empty one-line strings.
func (p *parser) strings(m *ordered.Map, key, row string) []string {
	var out []string
	for _, s := range m.List(key) {
		str, ok := s.(string)
		if !ok || strings.TrimSpace(str) == "" || strings.ContainsFunc(str, control) {
			p.add(m.KeyLine(key), "row %s: %s holds an item that is not a one-line string", row, key)
			continue
		}
		out = append(out, str)
	}
	return out
}

// registerTool reads one tool the register points to.
func (p *parser) registerTool(m *ordered.Map) RegisterTool {
	p.keys(m, []string{"key", "version", "tools_yaml", "replaces", "source"}, []string{"key", "tools_yaml", "replaces", "source"})
	t := RegisterTool{Key: m.Str("key"), Version: m.Str("version"), ToolsRow: m.Str("tools_yaml"),
		Replaces: strings.TrimSpace(m.Str("replaces")), Source: m.Str("source"), Line: m.Line}
	if !libraryKey.MatchString(t.Key) {
		p.add(m.KeyLine("key"), "tool %q is not lower-case words joined by dashes", t.Key)
	}
	if t.ToolsRow != "pointer" && !toolsRow.MatchString(t.ToolsRow) {
		p.add(m.KeyLine("tools_yaml"), "tool %s: tools_yaml %q is pointer, or a tools.yaml file and key", t.Key, t.ToolsRow)
	}
	if t.Version != "" && !pinVersion.MatchString(t.Version) {
		p.add(m.KeyLine("version"), "tool %s: version %q is a release (v1.2.3)", t.Key, t.Version)
	}
	u, err := url.Parse(t.Source)
	switch {
	case err != nil || u.Scheme != "https" || u.Host == "":
		p.add(m.KeyLine("source"), "tool %s: source %q is not an https address", t.Key, t.Source)
	case ourSource(u) != "":
		p.add(m.KeyLine("source"), "tool %s: source %s %s", t.Key, t.Source, ourSource(u))
	}
	return t
}

// dropped reads one name the register no longer holds.
func (p *parser) dropped(m *ordered.Map) Dropped {
	p.keys(m, []string{"name", "by"}, []string{"name", "by"})
	d := Dropped{Name: m.Str("name"), By: strings.TrimSpace(m.Str("by")), Line: m.Line}
	if strings.TrimSpace(d.Name) == "" || strings.ContainsFunc(d.Name, control) {
		p.add(m.Line, "a dropped name is empty")
	}
	return d
}

// heldBy holds the rows to each other: a dropped name has no row, no tool row, and is no row's library.
func (p *parser) heldBy(r *Register) {
	picked := map[string]string{}
	for _, l := range r.Rows {
		picked[l.Module] = l.Key
	}
	for _, d := range r.Dropped {
		if k, ok := picked[d.Name]; ok {
			p.add(d.Line, "%s is dropped and row %s calls it", d.Name, k)
		}
		for _, l := range r.Rows {
			if l.Key == d.Name {
				p.add(d.Line, "%s is dropped and has a row (line %d)", d.Name, l.Line)
			}
		}
		for _, t := range r.Tools {
			if t.Key == d.Name {
				p.add(d.Line, "%s is dropped and has a tool row (line %d)", d.Name, t.Line)
			}
		}
	}
}

// PresetRow is what one row feeds the Go preset: the imports its depguard row denies and the calls its
// forbidigo rows refuse.
type PresetRow struct {
	Key     string
	Module  string
	Linters []string
	Deny    []string
	Forbid  []Forbid
}

// Feed is the preset rows of the register: each row that is chosen and bans an import or a call, in file
// order. A row that needs a spike, is conditional or has only planned patterns feeds none.
func (r *Register) Feed() []PresetRow {
	var out []PresetRow
	for _, l := range r.Rows {
		if l.Banned() {
			out = append(out, PresetRow{Key: l.Key, Module: l.Module, Linters: slices.Clone(l.Linters), Deny: slices.Clone(l.Deny), Forbid: slices.Clone(l.Forbid)})
		}
	}
	return out
}

// placeholder is a shell or template variable left unexpanded in a text: $name or ${name}.
var placeholder = regexp.MustCompile(`\$[A-Za-z_{]`)

// placeholders refuses a text anywhere in the file that holds an unexpanded placeholder. A pattern is a regexp, and
// ends in $ or holds it as an anchor, so the keys that hold patterns are left alone.
func (p *parser) placeholders(v any, key string, line int) {
	switch x := v.(type) {
	case *ordered.Map:
		for _, k := range x.Keys() {
			if k == "pattern" || k == "planned" {
				continue
			}
			p.placeholders(x.Val(k), k, x.KeyLine(k))
		}
	case []any:
		for _, e := range x {
			p.placeholders(e, key, line)
		}
	case string:
		if placeholder.MatchString(x) {
			p.add(line, "%s holds an unexpanded placeholder: %q", key, x)
		}
	}
}

// The linters a ban is held by in the Go preset: depguard holds an import path, gomodguard_v2 a module that may
// not be a direct dependency at all, forbidigo a call, and gomoddirectives a line of go.mod.
const (
	LinterDepguard       = "depguard"
	LinterForbidigo      = "forbidigo"
	LinterGomodguard     = "gomodguard_v2"
	LinterGomoddirective = "gomoddirectives"
)

// linters reads ban.linter, a name or a list of names, and holds it to what the row bans: a row that bans an
// import or a call names the linter that holds each, a row that bans nothing names none, and a linter is named
// only where the row has what it reads.
func (p *parser) linters(b *ordered.Map, l *Library) {
	p.readLinters(b, l)
	p.holdLinters(b, l)
}

// readLinters reads the names of ban.linter into the row.
func (p *parser) readLinters(b *ordered.Map, l *Library) {
	line := b.KeyLine("linter")
	switch v := b.Val("linter").(type) {
	case nil:
	case string:
		l.Linters = []string{v}
	case []any:
		for _, x := range v {
			s, ok := x.(string)
			if !ok {
				p.add(line, "row %s: ban.linter holds an item that is not a name", l.Key)
				continue
			}
			l.Linters = append(l.Linters, s)
		}
	default:
		p.add(line, "row %s: ban.linter is not a name or a list of names", l.Key)
	}
}

// holdLinters holds the names of the row to what the row bans.
func (p *parser) holdLinters(b *ordered.Map, l *Library) {
	line := b.KeyLine("linter")
	for i, n := range l.Linters {
		if !slices.Contains([]string{LinterDepguard, LinterForbidigo, LinterGomodguard, LinterGomoddirective}, n) {
			p.add(line, "row %s: ban.linter names %q, which is none of depguard, forbidigo, gomodguard_v2 and gomoddirectives", l.Key, n)
		}
		if slices.Contains(l.Linters[:i], n) {
			p.add(line, "row %s: ban.linter names %s twice", l.Key, n)
		}
	}
	has := func(n string) bool { return slices.Contains(l.Linters, n) }
	banned := len(l.Deny) > 0 || len(l.Forbid) > 0
	switch {
	case banned && len(l.Linters) == 0:
		p.add(b.Line, "row %s: it bans an import or a call and names no ban.linter that holds it", l.Key)
	case len(l.Deny) > 0 && !has(LinterDepguard) && !has(LinterGomodguard):
		p.add(line, "row %s: it denies imports and names neither depguard nor gomodguard_v2 for them", l.Key)
	case len(l.Forbid) > 0 && !has(LinterForbidigo):
		p.add(line, "row %s: it forbids calls and does not name forbidigo for them", l.Key)
	}
	if len(l.Forbid) == 0 && has(LinterForbidigo) {
		p.add(line, "row %s: it names forbidigo and forbids no call", l.Key)
	}
	if len(l.Deny) == 0 && (has(LinterDepguard) || has(LinterGomodguard)) {
		p.add(line, "row %s: it names a linter for denied imports and denies none", l.Key)
	}
	if !banned && len(l.Linters) > 0 && !has(LinterGomoddirective) {
		p.add(line, "row %s: it bans nothing and names a ban.linter", l.Key)
	}
}

// A Blocked is a module a chosen row refuses and replaces: the preset's gomodguard_v2 refuses it in go.mod, with
// the why of the replacement as its reason and the row's own module as what to use instead.
type Blocked struct {
	Row, Module, Why, Instead string
}

// Blocked is the modules of the rows held by gomodguard_v2: each module such a chosen row both replaces and denies,
// in file order. A denied import path that is only a package of the module, or a module the row does not replace,
// is the depguard row's and not here.
func (r *Register) Blocked() []Blocked {
	var out []Blocked
	for _, l := range r.Rows {
		if !l.Banned() || !slices.Contains(l.Linters, LinterGomodguard) {
			continue
		}
		for _, rep := range l.Replaces {
			if slices.Contains(l.Deny, rep.Name) {
				out = append(out, Blocked{Row: l.Key, Module: rep.Name, Why: rep.Why, Instead: l.Module})
			}
		}
	}
	return out
}
