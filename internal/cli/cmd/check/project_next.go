package check

import (
	"github.com/birdtable/gragas/internal/model"
)

// nextRow is what cures a not-ready cause, in the words docs/settings.md and
// docs/commands.md use, and why it comes where it does.
type nextRow struct {
	cause model.GateCause
	next  string
	why   string
}

// nextOrder is the one table of the Next line of a not-ready project check,
// in the order the lines come first: a run held by more than one cause says
// the first row's line, and says why. The history and the base
// come before the tools, since the names, the tracked files and the counts
// read nothing without them, and a tool run again leaves them as they were.
func nextOrder() []nextRow {
	return []nextRow{
		{model.CauseHistory, "name git in the programs.git setting, and run in the top folder of the repository", "nothing of the history is read until it opens"},
		{model.CauseBase, "fetch the base ref, or set base-ref (GRAGAS_BASE_REF) to one this clone has", "the counts are held against the merge base"},
		{model.CauseTool, "gragas tools", "the tool rows are judged by the tools themselves"},
		{model.CauseNamesList, "set names-file (GRAGAS_NAMES_FILE) to a private file this account can read", "the names check reads its list first"},
		{model.CauseText, "fix the text the ending names: it must be there, readable, under 8 MiB, owned by this account and not writable by others", "the names check reads the texts named"},
	}
}

// nextOf is the Next line of a not-ready project check, from the causes of its
// rows that end the run (never from a message), and the sentence that says
// which comes first when more than one holds. A row with no cause is the tool
// cause, as every row was before the cause was set.
func nextOf(p model.ProjectCheck) (next, because string) {
	held := map[model.GateCause]bool{}
	for _, n := range p.Unjudged() {
		c := n.Cause
		if c == "" {
			c = model.CauseTool
		}
		held[c] = true
	}
	var first *nextRow
	var more []string
	rows := nextOrder()
	for i, r := range rows {
		if !held[r.cause] {
			continue
		}
		if first == nil {
			first = &rows[i]
			continue
		}
		more = append(more, string(r.cause))
	}
	if first == nil {
		return "gragas tools", ""
	}
	if len(more) > 0 {
		because = "first the " + string(first.cause) + " cause, then " + joinWords(more) + ": " + first.why
	}
	return first.next, because
}

func joinWords(w []string) string {
	if len(w) == 1 {
		return w[0]
	}
	return joinWords(w[:len(w)-1]) + " and " + w[len(w)-1]
}
