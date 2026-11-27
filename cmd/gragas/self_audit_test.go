package main

import (
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/host/clock"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// selfRow is one row of the table of docs/self-check.md that lists what gragas check finds in gragas: the
// rule and the key of the finding, how many there are, and the open issue that closes them.
type selfRow struct {
	rule, key string
	count     int
	issue     int
	exemption string
}

// selfRows reads the rows of the section "What `gragas check` finds in gragas" of the document text, and
// the problems with them: a row that is not six cells, a count that is not a number above 0, an issue
// that is not #N, and a rule and key that two rows give.
func selfRows(text string) ([]selfRow, []string) {
	const head = "## What `gragas check` finds in gragas"
	_, section, ok := strings.Cut(text, head+"\n")
	if !ok {
		return nil, []string{"docs/self-check.md has no section " + head}
	}
	if i := strings.Index(section, "\n## "); i >= 0 {
		section = section[:i]
	}
	var rows []selfRow
	var problems []string
	seen := map[string]bool{}
	for line := range strings.SplitSeq(section, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Rule |") || strings.HasPrefix(line, "|---") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if len(cells) != 6 {
			problems = append(problems, "a row is not six cells: "+line)
			continue
		}
		count, err := strconv.Atoi(cells[2])
		m := regexp.MustCompile(`^#([1-9][0-9]*)$`).FindStringSubmatch(cells[3])
		switch {
		case err != nil || count < 1:
			problems = append(problems, "a row's count is not a number above 0: "+line)
		case m == nil:
			problems = append(problems, "a row names no open issue as #N: "+line)
		case seen[cells[0]+" "+cells[1]]:
			problems = append(problems, "two rows for "+cells[0]+" "+cells[1])
		default:
			seen[cells[0]+" "+cells[1]] = true
			n, _ := strconv.Atoi(m[1])
			rows = append(rows, selfRow{rule: cells[0], key: cells[1], count: count, issue: n, exemption: cells[5]})
		}
	}
	return rows, problems
}

// compareSelf is every difference between the findings of the check and the rows: a finding with no row, a
// row with no finding, and a count that is not the finding's. Two-way, so a new gap is red and so is a fixed
// gap whose row stays.
func compareSelf(findings []coreapi.ConfigFinding, rows []selfRow) []string {
	got := map[string]int{}
	for _, f := range findings {
		got[f.Rule+" "+f.Key]++
	}
	want := map[string]int{}
	for _, r := range rows {
		want[r.rule+" "+r.key] = r.count
	}
	var out []string
	for _, k := range slices.Sorted(maps.Keys(got)) {
		switch n, ok := want[k]; {
		case !ok:
			out = append(out, fmt.Sprintf("a finding has no row in docs/self-check.md: %s (%d)", k, got[k]))
		case n != got[k]:
			out = append(out, fmt.Sprintf("a row says %d for %s, and the check finds %d", n, k, got[k]))
		}
	}
	for _, k := range slices.Sorted(maps.Keys(want)) {
		if _, ok := got[k]; !ok {
			out = append(out, "a row has no finding, so the gap is closed and the row goes: "+k)
		}
	}
	return out
}

// selfInputDir holds the parts and the systems gragas's own audit is run with. They are not a .gragas folder at
// the root: gragas check over gragas stays the rulebook check, and narrowing these to hide a finding is a change that
// takes the gate's owner.
const selfInputDir = "../../internal/testdata/self-audit/"

// selfInput is the repository with the committed parts and systems laid over the two files the check reads from
// its .gragas folder.
type selfInput struct {
	legacy.Source
	over map[string][]byte
}

func (s selfInput) ReadFile(name string) ([]byte, error) {
	if b, ok := s.over[name]; ok {
		return b, nil
	}
	return s.Source.ReadFile(name)
}

func (s selfInput) Close() error {
	if c, ok := s.Source.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// selfFiles opens the repository as selfInput.
type selfFiles struct {
	legacy.Files
	over map[string][]byte
}

func (f selfFiles) Repo(dir string) (legacy.Source, error) {
	src, err := f.Files.Repo(dir)
	if err != nil {
		return nil, err
	}
	return selfInput{Source: src, over: f.over}, nil
}

// TestGragasAuditsItself: the check of gragas's own Taskfile, hooks and tool configs, with the
// parts and systems of internal/testdata/self-audit, runs in process over the real tree, and the rows of
// docs/self-check.md that excuse its findings equal them. With -v it prints each finding with its row. Not run here: golangci-lint's own linter set, which
// needs the pinned program (the check says so). Planted failures: a finding with no row, a row with no
// finding and a changed count, each red.
func TestGragasAuditsItself(t *testing.T) {
	t.Parallel()
	over := map[string][]byte{}
	for name, file := range map[string]string{coreapi.PartsFile: "parts", coreapi.SystemsFile: "systems"} {
		b, err := os.ReadFile(selfInputDir + file)
		if err != nil {
			t.Fatal(err)
		}
		over[name] = b
	}
	p := ports(config.Config{})
	p.Files = selfFiles{Files: p.Files, over: over}
	l, err := coreapi.Load(t.Context(), p, "../..", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			t.Error(err)
		}
	}()
	w := world(nil, config.Config{})
	tiers, readers := coreapi.LoadTiers(w.Assets), coreapi.LoadReaders(w.Assets)
	if tiers.Refused != "" || readers.Refused != "" {
		t.Fatalf("the embedded tables are refused: %q %q", tiers.Refused, readers.Refused)
	}
	today := func() string { return clock.Clock{}.Now().UTC().Format(time.DateOnly) }
	got := coreapi.CheckConformance(t.Context(), l, coreapi.Tools{Readers: readers.Rows, TOML: ports(config.Config{}).TOML.Read, Tiers: tiers}, today)
	if !got.Present || !slices.Contains(got.Parts, "gates") || !slices.Contains(got.Parts, "go") {
		t.Fatalf("the check reads parts %v (present: %v): gragas takes its own parts in .gragas/parts", got.Parts, got.Present)
	}
	text, err := os.ReadFile("../../docs/self-check.md")
	if err != nil {
		t.Fatal(err)
	}
	// A row of the table is excused by its exemption, so the check reports the finding as waived:
	// the rows equal the findings and the waived findings together.
	rows, problems := selfRows(string(text))
	all := slices.Clone(got.Findings)
	for _, e := range got.Exempt {
		all = append(all, e.ConfigFinding)
	}
	for _, f := range all {
		issue := "no row"
		for _, r := range rows {
			if r.rule == f.Rule && r.key == f.Key {
				issue = fmt.Sprintf("row for #%d", r.issue)
			}
		}
		t.Logf("%s %s: %s (%s)", f.Rule, f.Key, f.Missing, issue)
	}
	for _, p := range problems {
		t.Error(p)
	}
	for _, d := range compareSelf(all, rows) {
		t.Error(d)
	}

	// The planted failures: the comparison is red for each.
	if len(all) > 0 && len(rows) > 0 {
		if d := compareSelf(all, rows[1:]); len(d) == 0 {
			t.Error("planted: a finding with no row is not red")
		}
		if d := compareSelf(all[1:], rows); len(d) == 0 {
			t.Error("planted: a row with no finding is not red")
		}
		bumped := slices.Clone(rows)
		bumped[0].count++
		if d := compareSelf(all, bumped); len(d) == 0 {
			t.Error("planted: a changed count is not red")
		}
	}
}

// TestTheRowsOfTheSelfCheckAreReadStrictly: a row with the wrong number of cells, a count that is not a
// number, an issue that is not #N and a key given twice are each a problem, so a row cannot excuse by being
// misread.
func TestTheRowsOfTheSelfCheckAreReadStrictly(t *testing.T) {
	t.Parallel()
	doc := func(rows ...string) string {
		return "## What `gragas check` finds in gragas\n\n| Rule | Key | Count | Issue | What is unmet | Exemption |\n|---|---|---|---|---|---|\n" + strings.Join(rows, "\n") + "\n\n## Releases\n"
	}
	good := "| gates.command.1 | pre-push empty | 1 | #1378 | x | e |"
	if rows, problems := selfRows(doc(good)); len(problems) != 0 || len(rows) != 1 || rows[0].issue != 1378 || rows[0].count != 1 {
		t.Errorf("a good row = %+v, %v", rows, problems)
	}
	for name, bad := range map[string]string{
		"five cells":     "| gates.command.1 | pre-push empty | #1378 | x | e |",
		"count zero":     "| gates.command.1 | pre-push empty | 0 | #1378 | x | e |",
		"count text":     "| gates.command.1 | pre-push empty | one | #1378 | x | e |",
		"issue no hash":  "| gates.command.1 | pre-push empty | 1 | 1378 | x | e |",
		"issue unnamed":  "| gates.command.1 | pre-push empty | 1 | later | x | e |",
		"issue gragas#N": "| gates.command.1 | pre-push empty | 1 | gragas#1378 | x | e |",
	} {
		if _, problems := selfRows(doc(bad)); len(problems) != 1 {
			t.Errorf("%s: problems %v, want one", name, problems)
		}
	}
	if _, problems := selfRows(doc(good, good)); len(problems) != 1 || !strings.Contains(problems[0], "two rows") {
		t.Errorf("a key twice: problems %v", problems)
	}
	if _, problems := selfRows("# nothing\n"); len(problems) != 1 {
		t.Errorf("no section: problems %v", problems)
	}
}
