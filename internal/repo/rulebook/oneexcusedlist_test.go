package rulebook

// What gragas's own check is excused is one list, not two: gate/exemptions.yaml holds
// the whole row, and the table "What `gragas check` finds in gragas" of docs/self-check.md keeps its prose and
// names the exemption's id in its last cell. The test reads the two files and needs no binary.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const selfCheckSection = "## What `gragas check` finds in gragas"

// tableRow is one row of that table: the rule, the issue and the exemption id its last cell names.
type tableRow struct{ rule, issue, id string }

// exemptionRow is one row of gate/exemptions.yaml: its id, the rule and the issue its exit names.
type exemptionRow struct{ id, rule, issue string }

var (
	issueCell = regexp.MustCompile(`^#([1-9][0-9]*)$`)
	exitIssue = regexp.MustCompile(`#([1-9][0-9]*)$`)
)

// tableRows reads the rows of the section of the document, and each row that is not six cells.
func tableRows(doc string) (rows []tableRow, bad []string) {
	_, section, ok := strings.Cut(doc, selfCheckSection+"\n")
	if !ok {
		return nil, []string{"docs/self-check.md has no section " + selfCheckSection}
	}
	if i := strings.Index(section, "\n## "); i >= 0 {
		section = section[:i]
	}
	for line := range strings.SplitSeq(section, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Rule |") || strings.HasPrefix(line, "|---") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if len(cells) != 6 {
			bad = append(bad, "a table row is not six cells: "+line)
			continue
		}
		rows = append(rows, tableRow{rule: cells[0], issue: strings.TrimPrefix(cells[3], "#"), id: strings.Trim(cells[5], "` ")})
		if !issueCell.MatchString(cells[3]) {
			bad = append(bad, "a table row names no issue as #N: "+line)
		}
	}
	return rows, bad
}

// exemptionRows reads the id, the rule and the exit's issue of each row of the exemptions file. The file is one
// block of "  - id:" rows with "    rule:" and "    exit:" lines (gate/exemptions.yaml's own header).
func exemptionRows(text string) []exemptionRow {
	var rows []exemptionRow
	for line := range strings.SplitSeq(text, "\n") {
		switch {
		case strings.HasPrefix(line, "  - id: "):
			rows = append(rows, exemptionRow{id: strings.TrimSpace(strings.TrimPrefix(line, "  - id: "))})
		case len(rows) > 0 && strings.HasPrefix(line, "    rule: "):
			rows[len(rows)-1].rule = strings.TrimSpace(strings.TrimPrefix(line, "    rule: "))
		case len(rows) > 0 && strings.HasPrefix(line, "    exit: "):
			if m := exitIssue.FindStringSubmatch(strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "    exit: ")), `"`)); m != nil {
				rows[len(rows)-1].issue = m[1]
			}
		}
	}
	return rows
}

// listProblems is every way the table and the exemptions are two lists: a table row whose id no exemption has,
// an exemption of a rule the table judges that no row cites (or two rows cite), and a pair that differs in its
// rule or its issue. A table with no row, and an exemptions file with no row, are problems, never "equal".
func listProblems(table []tableRow, exemptions []exemptionRow) []string {
	var bad []string
	if len(table) == 0 {
		bad = append(bad, "the table has no row")
	}
	if len(exemptions) == 0 {
		bad = append(bad, "the exemptions file has no row")
	}
	byID := map[string]exemptionRow{}
	for _, e := range exemptions {
		byID[e.id] = e
	}
	judged := map[string]bool{}
	cited := map[string]int{}
	for _, r := range table {
		judged[r.rule] = true
		cited[r.id]++
		e, ok := byID[r.id]
		switch {
		case !ok:
			bad = append(bad, fmt.Sprintf("the table row of %s cites %q, which gate/exemptions.yaml has no row for", r.rule, r.id))
		case e.rule != r.rule:
			bad = append(bad, fmt.Sprintf("%s: the table says rule %s and the exemption says %s", r.id, r.rule, e.rule))
		case e.issue != r.issue:
			bad = append(bad, fmt.Sprintf("%s: the table says issue #%s and the exemption's exit says #%s", r.id, r.issue, e.issue))
		}
	}
	for _, e := range exemptions {
		if judged[e.rule] && cited[e.id] != 1 {
			bad = append(bad, fmt.Sprintf("the exemption %s excuses %s, which the self-check judges, and %d table rows cite it, want 1", e.id, e.rule, cited[e.id]))
		}
	}
	slices.Sort(bad)
	return bad
}

// TestTheSelfCheckTableAndTheExemptionsAreOneList holds the two files to each other, and plants each break.
func TestTheSelfCheckTableAndTheExemptionsAreOneList(t *testing.T) {
	t.Parallel()
	docBytes, err := os.ReadFile(filepath.Join(root, "docs", "self-check.md"))
	if err != nil {
		t.Fatal(err)
	}
	exBytes, err := os.ReadFile(filepath.Join(root, "gate", "exemptions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	table, bad := tableRows(string(docBytes))
	for _, b := range bad {
		t.Error(b)
	}
	exemptions := exemptionRows(string(exBytes))
	for _, p := range listProblems(table, exemptions) {
		t.Error(p)
	}

	if len(table) < 2 || len(exemptions) < 2 {
		t.Fatalf("the plants need two rows each: the table has %d, the exemptions %d", len(table), len(exemptions))
	}
	plants := []struct {
		name  string
		table []tableRow
		ex    []exemptionRow
		want  string
	}{
		{"a table row with no exemption", append(slices.Clone(table), tableRow{rule: table[0].rule, issue: table[0].issue, id: "no-such-row"}), exemptions, "has no row for"},
		{"an exemption with no table row", table[1:], exemptions, "table rows cite it, want 1"},
		{"a row whose issue differs", table, differingIssue(exemptions, table[0].id), "the exemption's exit says"},
		{"a row whose rule differs", table, differingRule(exemptions, table[0].id), "the exemption says"},
		{"two rows for one exemption", append(slices.Clone(table), table[0]), exemptions, "table rows cite it, want 1"},
		{"a table with no row", nil, exemptions, "the table has no row"},
		{"an exemptions file with no row", table, nil, "the exemptions file has no row"},
	}
	for _, p := range plants {
		found := false
		for _, got := range listProblems(p.table, p.ex) {
			found = found || strings.Contains(got, p.want)
		}
		if !found {
			t.Errorf("%s: planted, and no problem says %q", p.name, p.want)
		}
	}
}

// differingIssue is the exemptions with the row of id given another issue.
func differingIssue(ex []exemptionRow, id string) []exemptionRow {
	out := slices.Clone(ex)
	for i := range out {
		if out[i].id == id {
			out[i].issue += "0"
		}
	}
	return out
}

// differingRule is the exemptions with the row of id given another rule.
func differingRule(ex []exemptionRow, id string) []exemptionRow {
	out := slices.Clone(ex)
	for i := range out {
		if out[i].id == id {
			out[i].rule += "x"
		}
	}
	return out
}
