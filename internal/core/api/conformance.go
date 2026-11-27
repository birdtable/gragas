package coreapi

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/core/project"
	"github.com/birdtable/gragas/internal/ordered"
)

// PartsFile is where a repository names the parts it takes, until `gragas
// resolve` is built: one line, the part names separated by spaces or commas.
const PartsFile = ".gragas/parts"

// ConfigFinding is one rule the repository's config does not implement.
type ConfigFinding struct {
	Rule    string `json:"rule"`
	Tool    string `json:"tool"`
	Key     string `json:"key"`
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Missing string `json:"missing"`
}

// String is the finding as a person reads it, in the shape of the rulebook's
// findings: "file: rule (tool key): what is missing".
func (f ConfigFinding) String() string {
	return f.where() + ": " + f.Rule + " (" + f.Tool + " " + f.Key + "): " + f.Missing
}

// where is the file, and the line after a colon when the finding has one.
func (f ConfigFinding) where() string {
	if f.Line > 0 {
		return f.File + ":" + strconv.Itoa(f.Line)
	}
	return f.File
}

// ConfigExempt is a finding that a row of gate/exemptions.yaml waives until
// Until.
type ConfigExempt struct {
	ConfigFinding
	Until string `json:"until"`
}

// String is the waived finding with its expiry.
func (e ConfigExempt) String() string {
	return e.ConfigFinding.String() + " (exempt until " + e.Until + ")"
}

// ConfigNotChecked says a tool's rules were not judged, and why. It is not a
// finding and not a pass.
type ConfigNotChecked struct {
	Tool    string `json:"tool"`
	Why     string `json:"why"`
	Partial bool   `json:"partial,omitempty"`
}

// String is the tool and the reason.
func (n ConfigNotChecked) String() string {
	if n.Partial {
		return n.Tool + ": " + n.Why
	}
	return n.Tool + " rules were not checked: " + n.Why
}

// Conformance is what check says of a repository's own tool configs: the
// rules of the parts it takes that those configs do not implement. Present is
// false when the repository names no parts, which is no finding. Halt is a
// tool the check was to ask and could not vouch for on this system, which
// ends the check red in the tool's own words: a run held to a
// memory limit this system neither holds nor reads. AsRefusal reads it.
type Conformance struct {
	Present    bool
	Parts      []string
	Findings   []ConfigFinding
	Exempt     []ConfigExempt
	NotChecked []ConfigNotChecked
	Halt       error
}

// TOML reads a TOML document into maps, lists and scalars; the layer above
// gives the one reader the program links.
type TOML func(data []byte) (map[string]any, error)

// Tools are what the layer above gives the read: the readers table (which
// tools, which files, which kind of read) and a TOML reader.
type Tools struct {
	Readers []Reader
	TOML    TOML
	// Plugins is the ports the tools' installed plugins are read through; its
	// zero value reads no tool (ErrNoPlugin).
	Plugins AuditPlugins
	// Tiers is the tier table; empty means the tier contract is not read.
	Tiers TierTable
}

// partsIn is the part names a parts file holds: separated by spaces, commas,
// tabs and line ends, a CR among them, so a CRLF file names the same parts as an
// LF one and never a part named `std\r`.
func partsIn(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool { return r == ' ' || r == ',' || r == '\n' || r == '\r' || r == '\t' })
}

// CheckConformance reads the repository's configs against the active,
// preset-rule bindings of the parts it names in PartsFile. A rule with a live
// row in gate/exemptions.yaml (today, YYYY-MM-DD, not past its expiry) is
// exempt, not a finding. It runs no tool and writes no file.
func CheckConformance(ctx context.Context, l Loaded, tools Tools, today func() string) Conformance {
	if l.src == nil || l.book == nil {
		return Conformance{}
	}
	raw, err := l.src.ReadFile(PartsFile)
	if err != nil {
		return Conformance{}
	}
	parts := partsIn(string(raw))
	if len(parts) == 0 {
		return Conformance{}
	}
	bs := slices.DeleteFunc(bindingsOf(l, parts), func(b audit.Binding) bool { return b.Tool == audit.Program && audit.Computed(b.Key) })
	date := ""
	if today != nil {
		date = today()
	}
	// The legacy book has no tested set, so entries(nil) keeps the first plugin of a
	// tool in List order where two serve it.
	res := audit.Check(ctx, audit.Input{
		Repo: l.src, Sources: sourcesOf(tools.Readers), Bindings: bs, Exempt: liveRows(l.src, date),
		ExemptAt: liveRowsAt(l.src, date), Entries: tools.Plugins.entries(nil), TOML: audit.TOML(tools.TOML),
	})
	res = merge(res, audit.CheckTiers(audit.TierInput{Repo: l.src, Sources: sourcesOf(tools.Readers), Tiers: tools.Tiers.table, SystemsFile: SystemsFile, Systems: declaredSystems(l.src), Exempt: liveRows(l.src, date)}))
	return conformanceOf(parts, res)
}

// sourcesOf is the readers table in the audit context's terms.
func sourcesOf(rs []Reader) []audit.Source {
	out := make([]audit.Source, len(rs))
	for i, r := range rs {
		out[i] = audit.Source{Tool: r.Tool, Files: r.Files, Kind: r.Kind, Host: r.Host}
	}
	return out
}

// conformanceOf is the audit's result in this layer's own types, so no type of
// the context behind is reachable from a command.
func conformanceOf(parts []string, res audit.Result) Conformance {
	out := Conformance{Present: true, Parts: parts, Halt: res.Halt}
	for _, f := range res.Findings {
		out.Findings = append(out.Findings, ConfigFinding(f))
	}
	for _, e := range res.Exempt {
		out.Exempt = append(out.Exempt, ConfigExempt{ConfigFinding(e.Finding), e.Until})
	}
	for _, n := range res.NotChecked {
		out.NotChecked = append(out.NotChecked, ConfigNotChecked(n))
	}
	return out
}

// liveRows is a lookup from a rule to the expiry of its exemption row that has
// not passed today. A file that is not there, cannot be read or holds no date
// to compare gives no row, so its rules are findings; the exemptions check
// says what is wrong with the file.
func liveRows(src fs.FS, today string) func(string) (string, bool) {
	rows := map[string]string{}
	data, err := fs.ReadFile(src, project.ExemptionsFile)
	if err != nil && !errors.Is(err, fs.ErrNotExist) || today == "" {
		return func(string) (string, bool) { return "", false }
	}
	if err == nil {
		addLive(rows, data, today)
	}
	return func(rule string) (string, bool) { until, ok := rows[rule]; return until, ok }
}

// liveRowsAt is a lookup from a rule and a file, a slash path from the root,
// to the expiry of a live row of that rule whose path is the file or a folder
// of it ("." is every file). What liveRows reads as no row, so does this.
func liveRowsAt(src fs.FS, today string) func(rule, file string) (string, bool) {
	type row struct{ path, until string }
	rows := map[string][]row{}
	if data, err := fs.ReadFile(src, project.ExemptionsFile); err == nil {
		eachLive(data, today, func(rule, exp string, r *ordered.Map) {
			rows[rule] = append(rows[rule], row{strings.TrimSuffix(r.Str("path"), "/"), exp})
		})
	}
	return func(rule, file string) (string, bool) {
		for _, r := range rows[rule] {
			if r.path == "." || r.path == file || strings.HasPrefix(file, r.path+"/") {
				return r.until, true
			}
		}
		return "", false
	}
}

// addLive puts into rows each rule of the exemptions file data whose expiry is
// a real day not before today, the latest expiry where a rule has two rows.
func addLive(rows map[string]string, data []byte, today string) {
	eachLive(data, today, func(rule, exp string, _ *ordered.Map) {
		if cur, dup := rows[rule]; !dup || exp > cur {
			rows[rule] = exp
		}
	})
}

// eachLive calls fn with each row of the exemptions file data that names a
// rule and whose expiry is a real day not before today. A file that does not
// parse, or no today, gives no row.
func eachLive(data []byte, today string, fn func(rule, exp string, row *ordered.Map)) {
	v, err := ordered.ParseStrict(data)
	doc, ok := v.(*ordered.Map)
	if err != nil || !ok || today == "" {
		return
	}
	for _, row := range ordered.Maps(doc.List("exemptions")) {
		exp, rule := expiry(row.Val("expires")), row.Str("rule")
		if rule != "" && isDay(exp) && exp >= today {
			fn(rule, exp, row)
		}
	}
}

// expiry is the text of an expires value: a string, or a YAML timestamp.
func expiry(v any) string {
	switch e := v.(type) {
	case string:
		return e
	case ordered.Timestamp:
		return string(e)
	}
	return ""
}

// isDay holds s to YYYY-MM-DD, a real day, so two dates compare as text.
func isDay(s string) bool {
	t, err := time.Parse(time.DateOnly, s)
	return err == nil && t.Format(time.DateOnly) == s
}

// merge is the findings, exemptions and unchecked tools of b after a's.
func merge(a, b audit.Result) audit.Result {
	a.Findings = append(a.Findings, b.Findings...)
	a.Exempt = append(a.Exempt, b.Exempt...)
	a.NotChecked = append(a.NotChecked, b.NotChecked...)
	a.Halt = errors.Join(a.Halt, b.Halt)
	return a
}

// declaredSystems is the systems the repository declares in SystemsFile; none when
// the file is not there.
func declaredSystems(src fs.FS) []audit.Declared {
	raw, err := fs.ReadFile(src, SystemsFile)
	if err != nil {
		return nil
	}
	var out []audit.Declared
	for i, line := range strings.Split(string(raw), "\n") {
		for _, w := range strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' || r == '\r' }) {
			out = append(out, audit.Declared{Name: w, Line: i + 1})
		}
	}
	return out
}

// TierContract is the tier contract of the repository src alone, against the
// tier table and the readers table, in this layer's own types: what
// CheckConformance adds to its report, without the part rules. It runs no tool
// and writes no file.
func TierContract(src fs.FS, rs []Reader, t TierTable, today string) Conformance {
	return conformanceOf(nil, audit.CheckTiers(audit.TierInput{
		Repo: src, Sources: sourcesOf(rs), Tiers: t.table, SystemsFile: SystemsFile,
		Systems: declaredSystems(src), Exempt: liveRows(src, today),
	}))
}

// bindingsOf is the active preset-rule checks of the rules of the named parts.
func bindingsOf(l Loaded, parts []string) []audit.Binding {
	var bs []audit.Binding
	for id, r := range l.book.Rules {
		if r.Data == nil || r.Data.Str("status") != "active" || !slices.Contains(parts, r.Part) {
			continue
		}
		for _, c := range ordered.Maps(r.Data.List("checks")) {
			if c.Str("class") == "preset-rule" && c.Str("tool") != "" && c.Str("key") != "" {
				bs = append(bs, audit.Binding{Rule: id, Tool: c.Str("tool"), Key: c.Str("key")})
			}
		}
	}
	return bs
}
