package gate

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// Rule is one rule of the rulebook as the gate reads it: what the register test, the docs' citation
// test, the retired-tool test and the exemptions check need of it, and its fields for the one that
// reads prose.
type Rule struct {
	// ID, Part and Concern are the rule's id and the part and concern of the file it is in.
	ID, Part, Concern string
	// Status and Strength are the words the file gives: proposed, trial, active or retired, and must,
	// should or may.
	Status, Strength string
	// File is the rule file, relative to the rulebook folder, and Line the line of the rule's id.
	File string
	Line int
	// Fields is the rule as written.
	Fields *ordered.Map
}

// Unread is the error of a rulebook whose files were found but not all read: one line a problem, the file and line first.
type Unread struct{ Lines []string }

func (u *Unread) Error() string { return strings.Join(u.Lines, "\n") }

var (
	errNoRuleFiles = errors.New("no rule file read: no <layer>/<part>/rules/<concern>.yaml under the folder, so there is no rule to hold")
)

// ReadRules reads every rule of the rulebook folder fsys holds: the concern files
// <layer>/<part>/rules/<concern>.yaml, each parsed by ordered.ParseStrict, which refuses a duplicate
// key. Each problem is one line naming its file and line, and every one is reported, with the rules
// that did read. A folder that is not there, a folder with no rule file, an empty file, a file that is
// not a mapping, a rule with no id, no status or no strength, a status or strength no reader knows and
// an id used twice are each refused. It never answers an empty list as "no active rule": no rule
// read is an error.
func ReadRules(fsys fs.FS) ([]Rule, error) {
	if _, err := fs.Stat(fsys, "."); err != nil {
		return nil, fmt.Errorf("the rulebook folder is not read: %w", err)
	}
	var files []string
	if err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && path.Ext(p) == ".yaml" && path.Base(path.Dir(p)) == "rules" {
			files = append(files, p)
		}
		return err
	}); err != nil {
		return nil, fmt.Errorf("the rulebook folder is not read: %w", err)
	}
	if len(files) == 0 {
		return nil, errNoRuleFiles
	}
	slices.Sort(files)
	var (
		rules    []Rule
		problems []string
		first    = map[string]string{}
	)
	for _, f := range files {
		got, bad := readRuleFile(fsys, f)
		problems = append(problems, bad...)
		for _, r := range got {
			if at, dup := first[r.ID]; dup {
				problems = append(problems, fmt.Sprintf("%s:%d: the id %s is also at %s", f, r.Line, r.ID, at))
				continue
			}
			first[r.ID] = fmt.Sprintf("%s:%d", f, r.Line)
			rules = append(rules, r)
		}
	}
	if len(rules) == 0 && len(problems) == 0 {
		problems = append(problems, "no rule read: every rule file holds an empty list")
	}
	if len(problems) > 0 {
		return rules, &Unread{Lines: problems}
	}
	return rules, nil
}

func readRuleFile(fsys fs.FS, file string) ([]Rule, []string) {
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, []string{file + ": not read: " + err.Error()}
	}
	v, err := ordered.ParseStrict(data)
	if err != nil {
		return nil, []string{file + ": " + err.Error()}
	}
	if v == nil {
		return nil, []string{file + ": the file is empty"}
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return nil, []string{file + ": the file is not a mapping"}
	}
	if _, isList := doc.Get("rules"); !isList || doc.List("rules") == nil {
		return nil, []string{file + ": the file has no rules list"}
	}
	part, concern := doc.Str("part"), doc.Str("concern")
	var out []Rule
	var bad []string
	for i, item := range doc.List("rules") {
		m, ok := item.(*ordered.Map)
		if !ok {
			bad = append(bad, fmt.Sprintf("%s: rule %d is not a mapping", file, i+1))
			continue
		}
		r := Rule{ID: m.Str("id"), Part: part, Concern: concern, Status: m.Str("status"), Strength: m.Str("strength"), File: file, Line: m.KeyLine("id"), Fields: m}
		at := fmt.Sprintf("%s:%d", file, r.Line)
		switch {
		case r.ID == "":
			bad = append(bad, at+": a rule with no id")
		case part == "":
			bad = append(bad, at+": "+r.ID+" is in a file with no part")
		case r.Status == "":
			bad = append(bad, at+": "+r.ID+" has no status")
		case !slices.Contains([]string{"proposed", "trial", "active", "retired"}, r.Status):
			bad = append(bad, at+": "+r.ID+" has the status "+r.Status+", which is none of "+"proposed, trial, active, retired")
		case !slices.Contains([]string{"must", "should", "may"}, r.Strength):
			bad = append(bad, at+": "+r.ID+" has the strength \""+r.Strength+"\", which is none of "+"must, should, may")
		default:
			out = append(out, r)
		}
	}
	return out, bad
}

// ActiveRules are the ids of the active rules of rules, which gragas-dev holds the exemptions to.
func ActiveRules(rules []Rule) map[string]bool {
	live := map[string]bool{}
	for _, r := range rules {
		if r.Status == "active" {
			live[r.ID] = true
		}
	}
	return live
}
