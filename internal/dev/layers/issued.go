package layers

import (
	"fmt"
	"regexp/syntax"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// IDsFile is where the rule ids ever issued are listed, one to a line.
const IDsFile = "rulebook/ids.lock"

// A head is a rule id a row of the lint file opens with: the row, as the
// problem names it, its line, and the id.
type head struct {
	line     int
	what, id string
}

// Unissued is each rule id a row of the lint file opens with that ids, every
// id ever issued, does not hold. Those rows are a forbidigo row's message, a
// depguard deny's reason, and each string an exclusion's text spells up to
// its first colon. A message that opens with a rule id cites that rule, so an
// id no rule ever had lets a ban cite a rule that does not exist, and lets an
// exclusion confine itself to one (gragas#1861). A text that spells no rule
// id is not read here; voids judges it. With no id, or no head read, nothing
// is held, and that is a problem too.
func Unissued(lint []byte, ids []string) []string {
	v, err := ordered.ParseYAML(lint)
	doc, ok := v.(*ordered.Map)
	if err != nil || !ok {
		return []string{fmt.Sprintf("the lint file does not read as a mapping: %v", err)}
	}
	if len(ids) == 0 {
		return []string{IDsFile + " holds no id, so no rule id of the lint file can be held to it"}
	}
	heads := lintHeads(doc)
	if len(heads) == 0 {
		return []string{"no row of the lint file opens with a rule id, so nothing was held to " + IDsFile}
	}
	var out []string
	for _, h := range heads {
		if !slices.Contains(ids, h.id) {
			out = append(out, fmt.Sprintf("line %d: %s opens with %s, which %s does not hold: a row cites a rule that was issued, or none", h.line, h.what, h.id, IDsFile))
		}
	}
	return out
}

// lintHeads is the rule id each forbidigo row, depguard deny and exclusion of
// doc opens with, in the file's order of kinds.
func lintHeads(doc *ordered.Map) []head {
	settings := sub(doc, "linters", "settings")
	var out []head
	for _, f := range rowsOf(sub(settings, "forbidigo"), "forbid") {
		if id, ok := msgHead(f.Str("msg")); ok {
			out = append(out, head{f.Line, "the forbidigo ban " + f.Str("pattern"), id})
		}
	}
	rules := sub(settings, "depguard", "rules")
	if rules == nil {
		rules = &ordered.Map{}
	}
	for _, name := range rules.Keys() {
		for _, d := range rowsOf(rules.Sub(name), "deny") {
			if id, ok := msgHead(d.Str("desc")); ok {
				out = append(out, head{d.Line, "the depguard ban of " + d.Str("pkg") + " in " + name, id})
			}
		}
	}
	for _, e := range rowsOf(sub(doc, "linters", "exclusions"), "rules") {
		for _, id := range textHeads(e.Str("text")) {
			out = append(out, head{e.Line, "the exclusion of " + excluded(e), id})
		}
	}
	return out
}

// msgHead is the rule id msg opens with, before its first colon, and
// whether it opens with one.
func msgHead(msg string) (string, bool) {
	h, _, ok := strings.Cut(msg, ":")
	if !ok || !ruleHead.MatchString(h+":") {
		return "", false
	}
	return h, true
}

// textHeads is each rule id an exclusion's text spells up to its first
// colon, as confined spells it; a text that does not spell out, or spells a
// string that is no rule id, gives none.
func textHeads(text string) []string {
	re, err := syntax.Parse(text, syntax.Perl)
	if err != nil || text == "" {
		return nil
	}
	spelled, ok := spell(re.Simplify(), []string{""})
	if !ok {
		return nil
	}
	var out []string
	for _, s := range spelled {
		if ruleHead.MatchString(s) {
			out = append(out, strings.TrimSuffix(s, ":"))
		}
	}
	return out
}

// rowsOf is the mappings of m's list at key, and none when m is not there.
func rowsOf(m *ordered.Map, key string) []*ordered.Map {
	if m == nil {
		return nil
	}
	return ordered.Maps(m.List(key))
}
