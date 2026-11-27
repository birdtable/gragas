package census

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The kinds of a decision in gate/os-api.txt.
const (
	// KindBan is an identifier the lint refuses: the rest of the line is
	// the rule whose row refuses it.
	KindBan = "ban"
	// KindAllow is an identifier any package may use: the rest of the line
	// is why it is safe on every system.
	KindAllow = "allow"
	// KindLater is an identifier a row of gragas#1750 bans and has not
	// banned yet: the rest of the line names the issue and the row. The
	// kind only shrinks against the merge base.
	KindLater = "later"
)

// Decision is one line of gate/os-api.txt: an identifier, as the census
// names it, or a whole package as <import path>.*, its kind, and the rest
// of the line.
type Decision struct {
	Name, Kind, Rest string
	Line             int
}

// Path is the import path the decision's name starts with.
func (d Decision) Path() string { p, _ := split(d.Name); return p }

// Whole is whether the decision is for every identifier of a package.
func (d Decision) Whole() bool { _, k := split(d.Name); return k == "*" }

// split is a name's import path and the rest after it: the path ends at the
// first dot after its last slash that no digit follows, since a name never
// starts with one, so crypto/internal/entropy/v1.0.0.Seed is Seed of
// crypto/internal/entropy/v1.0.0.
func split(name string) (path, key string) {
	at := strings.LastIndex(name, "/") + 1
	for {
		dot := strings.Index(name[at:], ".")
		if dot < 0 {
			return name, ""
		}
		at += dot + 1
		if at == len(name) || name[at] < '0' || name[at] > '9' {
			return name[:at-1], name[at:]
		}
	}
}

var (
	ruleID   = regexp.MustCompile(`^[a-z]+\.[a-z]+\.[0-9]+$`)
	laterRow = regexp.MustCompile(`^#[0-9]+ L[0-9]+[a-z]?$`)
	keyShape = regexp.MustCompile(`^(\*|[A-Z][A-Za-z0-9_]*(\.[A-Z][A-Za-z0-9_]*)?)$`)
	// rootShape is a name the reach passes through that no program can
	// write: a func, var or const of either case, or a method as .<name>, of
	// whichever type of the package declares it.
	rootShape = regexp.MustCompile(`^\.?[A-Za-z_][A-Za-z0-9_]*$`)
)

// ParseDecisions reads gate/os-api.txt: a decision a line, as <name> <kind>
// <rest>; a blank line or one that starts with # says nothing. The lines
// are in byte order of their names, each name once, so the list is exact
// and a diff of it reads as one. It returns the decisions it read and what
// is wrong with the text, each as one line naming its line number.
func ParseDecisions(text string) ([]Decision, []string) {
	var out []Decision
	var bad []string
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		f := strings.Fields(t)
		if len(f) < 3 {
			bad = append(bad, fmt.Sprintf("line %d: %q is not <name> <kind> <rest>: a decision with nothing after its kind says nothing", n, t))
			continue
		}
		d := Decision{Name: f[0], Kind: f[1], Rest: strings.Join(f[2:], " "), Line: n}
		if p := shapeProblem(d); p != "" {
			bad = append(bad, fmt.Sprintf("line %d: %s %s", n, d.Name, p))
			continue
		}
		if len(out) > 0 {
			switch prev := out[len(out)-1]; {
			case prev.Name == d.Name:
				bad = append(bad, fmt.Sprintf("line %d: %s is decided twice (line %d too): one line a name", n, d.Name, prev.Line))
				continue
			case prev.Name > d.Name:
				bad = append(bad, fmt.Sprintf("line %d: %s comes before %s (line %d) in byte order: the list is kept sorted", n, d.Name, prev.Name, prev.Line))
			}
		}
		out = append(out, d)
	}
	return out, bad
}

// shapeProblem is what is wrong with one decision's shape, or "".
func shapeProblem(d Decision) string {
	path, key := split(d.Name)
	switch {
	case path == "" || key == "":
		return "is not <import path>.<Name>, <import path>.<Type>.<Name> or <import path>.*"
	case key == "*" && !Importable(path):
		return "names a package no program outside std may import"
	case !keyShape.MatchString(key) || !Importable(path):
		if !rootShape.MatchString(key) {
			return "is not <import path>.<Name>, <import path>.<Type>.<Name>, <import path>.*, or a root as <import path>.<name> or <import path>..<method>"
		}
		if d.Kind != KindAllow {
			return "is a root, which only allow decides: a root is no name a program writes, so no lint row can refuse it, and only an allow stops the reach, so a ban or later here would settle nothing. Each name that reaches it takes its own line"
		}
	}
	switch d.Kind {
	case KindBan:
		if !ruleID.MatchString(d.Rest) {
			return fmt.Sprintf("ban %q: a ban names one rule id, as part.concern.n, whose lint row refuses it", d.Rest)
		}
	case KindLater:
		if !laterRow.MatchString(d.Rest) {
			return fmt.Sprintf("later %q: a later line names the issue and the row that bans it, as #1750 L5", d.Rest)
		}
	case KindAllow:
		if key == "*" {
			return "allow: a whole package is banned or later, never allowed: allow each identifier with its own reason"
		}
	default:
		return fmt.Sprintf("has the kind %q: the kinds are %s, %s and %s", d.Kind, KindBan, KindAllow, KindLater)
	}
	return ""
}

// Forbid is one forbidigo row of the lint file: its pattern, its pkg
// pattern when it has one, and its message, which opens with the rule ids
// it enforces and a colon.
type Forbid struct{ Pattern, Pkg, Msg string }

// Deny is one deny entry of a depguard rule: the package, its desc, which
// opens with the rule ids and a colon, and the files of its rule.
type Deny struct {
	Pkg, Desc string
	Files     []string
}

// Lint is what of the lint file a ban is held to.
type Lint struct {
	Forbid []Forbid
	Deny   []Deny
}

// Hold holds the decisions to the census and the lint: every identifier
// the census found has one line, by its name or by its package's .* line,
// unless it differs only by reaching allowed names; every line names
// something the census found; and every ban has the lint row that refuses
// it. It returns each problem as one line.
func Hold(c *Census, ds []Decision, lint Lint) []string {
	var bad []string
	if c == nil || len(c.Differ) == 0 {
		return []string{"the census found no identifier that differs by system, so nothing was held: " + ErrNoIdent.Error()}
	}
	exact, whole := index(ds)
	used := map[string]bool{}
	settled := c.settled(func(path, name string) bool {
		return exact[path+"."+name].Kind == KindAllow
	})
	for _, id := range c.Differ {
		path, _ := split(id.Name)
		e, byName := exact[id.Name]
		w, byPkg := whole[path]
		switch {
		case byName && byPkg:
			bad = append(bad, fmt.Sprintf("line %d: %s is decided by its own line and by line %d's %s: one decision an identifier", e.Line, id.Name, w.Line, w.Name))
		case !byName && !byPkg && !settled[id.Name]:
			bad = append(bad, fmt.Sprintf("%s differs by system (%s) and has no line in gate/os-api.txt: add one, as ban, allow or later (gragas#1750 L14)", id.Name, strings.Join(id.Why, "; ")))
		}
		if byName {
			used[e.Name] = true
		}
		if byPkg {
			used[w.Name] = true
		}
	}
	for _, d := range ds {
		if !used[d.Name] && !c.foundRoot(d.Name) {
			bad = append(bad, fmt.Sprintf("line %d: %s names nothing the census found to differ by system: the line goes", d.Line, d.Name))
		}
		if d.Kind == KindBan {
			bad = append(bad, banned(d, lint)...)
		}
	}
	return bad
}

// index is the decisions by name, and the whole-package ones by path.
func index(ds []Decision) (exact, whole map[string]Decision) {
	exact, whole = map[string]Decision{}, map[string]Decision{}
	for _, d := range ds {
		if d.Whole() {
			whole[d.Path()] = d
		} else {
			exact[d.Name] = d
		}
	}
	return exact, whole
}

// foundRoot is whether name is a root the census found to differ: a name
// the reach passes through that is no exported identifier of an importable
// package. A census built by hand, with nothing read, has none.
func (c *Census) foundRoot(name string) bool {
	path, key := split(name)
	if c.w == nil || (keyShape.MatchString(key) && Importable(path)) {
		return false
	}
	d := c.w.resolve(path)
	if d == nil || d.syms[key] == nil {
		return false
	}
	_, ok := c.found[d.syms[key]]
	return ok
}

// settled are the identifiers of the census that differ only by reach, and
// reach nothing once the reach stops at what allowed says is allowed as well.
// An allowed name settles what differs through it alone, so an allow on a
// root settles its leaves. A ban or a
// later stops nothing: it says the difference matters, so each name that
// reaches it is open and takes its own line, and a wrapper of a banned name
// cannot pass under another spelling (the ruling, 11:31Z). A census built
// by hand, with nothing read, settles none.
func (c *Census) settled(allowed stop) map[string]bool {
	out := map[string]bool{}
	if c.w == nil {
		return out
	}
	both := func(path, name string) bool { return runtimeHelper(path, name) || allowed(path, name) }
	open := c.w.reach(c.order, both)
	for _, id := range c.Differ {
		if !slices.Equal(id.Why, []string{WhyReach}) {
			continue
		}
		path, key := split(id.Name)
		d := c.w[path]
		self := selfOf(key)
		out[id.Name] = !slices.ContainsFunc(d.perPort, func(m map[string]decl) bool {
			x, ok := m[key]
			return ok && c.w.via(d, x.uses, self, open, both) != ""
		})
	}
	return out
}

// banned is what is wrong with a ban's lint row: none when a forbidigo row
// refuses the identifier, or a depguard deny on every file refuses the
// whole package, under the ban's rule.
func banned(d Decision, lint Lint) []string {
	path, key := split(d.Name)
	if key == "*" {
		for _, dn := range lint.Deny {
			if dn.Pkg == path && slices.Contains(dn.Files, "$all") && slices.Contains(ruleList(dn.Desc), d.Rest) {
				return nil
			}
		}
		return []string{fmt.Sprintf("line %d: %s is banned under %s and no depguard rule on every file denies %s with a desc that opens with %s", d.Line, d.Name, d.Rest, path, d.Rest)}
	}
	text := pkgName(path) + "." + key
	var bad []string
	for _, f := range lint.Forbid {
		re, err := regexp.Compile(f.Pattern)
		if err != nil {
			bad = append(bad, fmt.Sprintf("the forbidigo pattern %q does not compile: %v", f.Pattern, err))
			continue
		}
		if !re.MatchString(text) || !slices.Contains(ruleList(f.Msg), d.Rest) {
			continue
		}
		if f.Pkg != "" {
			pre, err := regexp.Compile(f.Pkg)
			if err != nil || !pre.MatchString(path) {
				continue
			}
		}
		return nil
	}
	return append(bad, fmt.Sprintf("line %d: %s is banned under %s and no forbidigo row matches %s with a message that opens with %s", d.Line, d.Name, d.Rest, text, d.Rest))
}

// ruleList is the rule ids a lint message opens with, before its first
// colon.
func ruleList(msg string) []string {
	head, _, ok := strings.Cut(msg, ":")
	if !ok {
		return nil
	}
	var out []string
	for r := range strings.SplitSeq(head, ",") {
		out = append(out, strings.TrimSpace(r))
	}
	return out
}

// major is a last path element that is a module's major version.
var major = regexp.MustCompile(`^v[0-9]+$`)

// pkgName is the name a file gives the package at path when it imports it
// with no name: its last element, or the one before a major version.
func pkgName(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 1 && major.MatchString(parts[len(parts)-1]) {
		return parts[len(parts)-2]
	}
	return parts[len(parts)-1]
}

// LaterGrown is what the later lines of head hold that base's do not: a
// later line whose name the base holds as no later line, so the kind only
// shrinks against the merge base. A base that holds no file is the
// caller's to say: it holds nothing to this.
func LaterGrown(head, base []Decision) []string {
	was := map[string]bool{}
	for _, d := range base {
		if d.Kind == KindLater {
			was[d.Name] = true
		}
	}
	var bad []string
	for _, d := range head {
		if d.Kind == KindLater && !was[d.Name] {
			bad = append(bad, fmt.Sprintf("line %d: %s is later and was not later at the merge base with origin/main: the later lines only shrink; ban it or allow it with its reason", d.Line, d.Name))
		}
	}
	return bad
}
