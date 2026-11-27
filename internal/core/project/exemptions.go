// Package project is the project context: what gragas holds a project to
// beside its rulebook. Today that is the written exceptions to its rules, the
// rows of gate/exemptions.yaml, which gragas check reads when the file is
// there. When the rulebook's waivers land they replace this reader whole:
// nothing outside this file and the operation that calls it knows its shape.
package project

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"time"

	"github.com/birdtable/gragas/internal/ordered"
)

// ExemptionsFile is where a project keeps its exceptions, from its root.
const ExemptionsFile = "gate/exemptions.yaml"

// artifact is what an exemption's exit names: an issue of the estate
// (owner/repo#N) or a file of this repository.
var artifact = regexp.MustCompile(`^(?:[a-z][a-z0-9-]*/[a-z][a-z0-9-]*#[0-9]+|[A-Za-z0-9._][A-Za-z0-9._/-]*\.[a-z]+)$`)

// Finding is one problem with a row: the file, the line of the row (0 for
// the whole file) and what is wrong, as the rulebook's findings are.
type Finding struct {
	Path string
	Line int
	Msg  string
}

// String is "path:line: message", or "path: message" with no line.
func (f Finding) String() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Msg)
	}
	return f.Path + ": " + f.Msg
}

// Exemptions is what the check found: the findings, how many rows the file
// holds, and the soonest expiry still ahead ("" when none is).
type Exemptions struct {
	Findings   []Finding
	Rows       int
	NextExpiry string
}

// CheckExemptions holds the one exception format: each row names its rule and
// its path, a reason, the artifact whose landing retires it, an owner and an
// expiry. data is the file's bytes; active holds the ids of the rulebook's
// active rules; today is the date, YYYY-MM-DD; repo is the project's files,
// where an exit that names a file must stand. An expired row fails, a row
// for a rule that is not active fails (an exception for a rule that cannot
// fire is unused), an id listed twice fails, and so does an exit that names
// no artifact or a file that is not there (gates.exemptions.2,
// gates.exemptions.4). A row's expiry is a date or a finding: a row whose
// expiry cannot be compared would never expire. So is the file's shape: an
// exemptions that is not a list, and a row that is not a mapping, are each a
// finding, since a row that cannot be read is a row that is never judged.
// The file is read strictly (ordered.ParseStrict): a key written twice, an
// anchor, an alias, a merge key or a tag is a finding of the whole file, since
// a second "expires" in a row, or a second "exemptions" list, would otherwise
// stand in for the first and hide an expired row. A row with no exit, owner or
// reason passes here: gragas-dev's gate schema refuses it, and gragas check
// leaves those fields to it.
// gragas-dev's gate calls this itself, with its own clock.
func CheckExemptions(data []byte, active map[string]bool, today string, repo fs.StatFS) Exemptions {
	var out Exemptions
	add := func(line int, format string, args ...any) {
		out.Findings = append(out.Findings, Finding{Path: ExemptionsFile, Line: line, Msg: fmt.Sprintf(format, args...)})
	}
	if !isDate(today) {
		add(0, "no date was given, so no expiry can be judged")
		return out
	}
	v, err := ordered.ParseStrict(data)
	doc, ok := v.(*ordered.Map)
	switch {
	case err != nil:
		add(0, "%v", err)
		return out
	case !ok:
		add(0, "is not a mapping")
		return out
	}
	rows, n, isList := rowsOf(doc, add)
	if !isList {
		return out
	}
	out.Rows = n
	seen := map[string]bool{}
	for _, row := range rows {
		id, rule := row.Str("id"), row.Str("rule")
		if seen[id] {
			add(row.Line, "exemption %s is listed twice", id)
		}
		seen[id] = true
		switch exp := dateOf(row, "expires"); {
		case !isDate(exp):
			add(row.Line, "%s: its expiry %q is not a date (YYYY-MM-DD), so it would never expire", id, exp)
		case exp < today:
			add(row.Line, "%s expired on %s: retire it or renew it with a reason", id, exp)
		case out.NextExpiry == "" || exp < out.NextExpiry:
			out.NextExpiry = exp
		}
		if exit := row.Str("exit"); exit != "" && (!artifact.MatchString(exit) || !strings.Contains(exit, "#") && !fs.ValidPath(exit)) {
			add(row.Line, "%s: exit %q names no artifact (an issue like owner/repo#12 or a file)", id, exit)
		} else if exit != "" && !strings.Contains(exit, "#") && !exists(repo, exit) {
			add(row.Line, "%s: its exit %s is a file that does not exist", id, exit)
		}
		if !active[rule] {
			add(row.Line, "%s is unused: %s is not an active rule of the rulebook", id, rule)
		}
	}
	return out
}

// rowsOf is the rows of the file's exemptions list that are mappings and
// how many rows the list holds, each other row a finding through add; false
// when exemptions is not a list, which is a finding too.
func rowsOf(doc *ordered.Map, add func(line int, format string, args ...any)) ([]*ordered.Map, int, bool) {
	list, isList := doc.Val("exemptions").([]any)
	if !isList {
		add(doc.KeyLine("exemptions"), "exemptions is not a list of rows, so no row could be judged")
		return nil, 0, false
	}
	rows := make([]*ordered.Map, 0, len(list))
	for i, v := range list {
		row, isRow := v.(*ordered.Map)
		if !isRow {
			add(doc.KeyLine("exemptions"), "row %d of exemptions is not a mapping, so it could not be judged", i+1)
			continue
		}
		rows = append(rows, row)
	}
	return rows, len(list), true
}

// dateOf is a string or a date under k.
func dateOf(m *ordered.Map, k string) string {
	switch v := m.Val(k).(type) {
	case string:
		return v
	case ordered.Timestamp:
		return string(v)
	}
	return ""
}

// isDate holds s to YYYY-MM-DD, a real day, so two dates compare as text.
func isDate(s string) bool {
	t, err := time.Parse(time.DateOnly, s)
	return err == nil && t.Format(time.DateOnly) == s
}

// exists says whether path, local and slash-separated, is a file in repo:
// a folder of that name is no file the exit could name.
func exists(repo fs.StatFS, path string) bool {
	if repo == nil {
		return false
	}
	info, err := repo.Stat(path)
	return err == nil && !info.IsDir()
}
