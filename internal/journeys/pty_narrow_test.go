package journeys

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// The pty journeys below 80 columns are held to properties, not goldens
// (tests.journeys.8). The golden at 80 is the oracle a person read; at each
// narrower width the same journey runs again and its screens must keep three properties against that golden: no line wider than the
// screen, nothing cut off, and the same rows in the same order. A width is a
// line of data here and adds no golden file.

// narrowWidths are the widths the properties hold at.
func narrowWidths() []int { return []int{60, 40} }

// TestThePtyJourneysHoldAtNarrowWidths: every pty journey at 60 and 40 columns
// says what its 80-column golden says, every word in the same order, on lines
// no wider than the screen. A property that fails prints the width, the row and
// the measure. The plant is the same run with its last line cut off, which must
// fail; the control is the clean run. Risk: a menu or a help that loses a row,
// or draws past the edge, on a narrow terminal, where no person reads a golden.
func TestThePtyJourneysHoldAtNarrowWidths(t *testing.T) {
	t.Parallel()
	m := tableMeasuresOf(t)
	for _, cols := range narrowWidths() {
		for _, c := range ptyCases() {
			t.Run(c.name+"-"+strconv.Itoa(cols), func(t *testing.T) {
				oracle := goldenOf(t, "pty-"+c.name+"-80")
				// A picture is taken once the screen holds what the golden's part of that name says, as the
				// properties below read it. The program draws a menu twice (the rows cut at the edge first,
				// wrapped after), and under load a quiet screen is the first draw: waiting for quiet alone
				// took the picture of it. A program that never draws the settled screen still
				// fails, when ptyWait is out, with the screen it had.
				_, _, want := partsOf(oracle)
				settled := func(title, screen string) string {
					for _, p := range want {
						if p.title == "--- "+title {
							return partProblem(m, cols, p.title, p.lines, strings.Split(strings.TrimSuffix(screen, "\n"), "\n"))
						}
					}
					return ""
				}
				r := ptyRunOf(t, c, cols, settled)
				for _, f := range narrowFindings(m, cols, oracle, r.transcript, r.overruns) {
					t.Error(f)
				}
				if len(narrowFindings(m, cols, oracle, cutLastLine(r.transcript), nil)) == 0 {
					t.Errorf("plant: the run with its last screen line cut off passed the properties")
				}
			})
		}
	}
}

// cutLastLine is the transcript without the last line of its last screen that
// says something: a line of frame alone, such as the bar under a menu, holds no
// word the properties compare, so cutting it would prove nothing.
func cutLastLine(transcript string) string {
	lines := strings.Split(strings.TrimSuffix(transcript, "\n"), "\n")
	for i := len(lines) - 1; i > 0; i-- {
		if !strings.HasPrefix(lines[i], "--- ") && len(unframed(lines[i])) > 0 {
			return strings.Join(slices.Delete(lines, i, i+1), "\n") + "\n"
		}
	}
	return transcript
}

// narrowFindings is each property the transcript at cols columns breaks against
// the 80-column golden, and the lines the program drew past the screen's edge.
// Each says the width, the row and the measure. Within a part, the first word
// that differs is the finding: what follows it would only repeat it.
func narrowFindings(m tableMeasures, cols int, oracle, got string, over []overrun) []string {
	out := overrunFindings(cols, over)
	wantCommand, _, want := partsOf(oracle)
	gotCommand, gotWidth, have := partsOf(got)
	if gotCommand != wantCommand {
		out = append(out, fmt.Sprintf("at %d columns, the run is %q, and the golden's is %q", cols, gotCommand, wantCommand))
	}
	if w := "--- " + strconv.Itoa(cols) + " columns"; gotWidth != w {
		out = append(out, fmt.Sprintf("at %d columns, the transcript says %q, not %q", cols, gotWidth, w))
	}
	titles := func(ps []transcriptPart) []string {
		var ts []string
		for _, p := range ps {
			ts = append(ts, p.title)
		}
		return ts
	}
	if !slices.Equal(titles(have), titles(want)) {
		return append(out, fmt.Sprintf("at %d columns, the same rows in the same order: the parts are %q, and the golden's are %q",
			cols, titles(have), titles(want)))
	}
	for i := range want {
		if f := partProblem(m, cols, want[i].title, want[i].lines, have[i].lines); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// partProblem is the finding for one part of a screen against the golden's part
// of the same title, "" when the part holds what the golden says.
func partProblem(m tableMeasures, cols int, title string, wantLines, haveLines []string) string {
	f := dropFinding(m, cols, title, wantLines, haveLines)
	if f == "" {
		w, h := withoutMarkedColumns(wantLines, haveLines)
		f = partFinding(cols, title, w, h)
	}
	return f
}

// droppedNote is the line a table adds when the width made it leave columns
// out, naming them (cli.look.7: a dropped column is marked; term.Look.Table).
var droppedNote = regexp.MustCompile(`^\s*\S+ columns? not shown: (.+)$`)

// withoutMarkedColumns is the words of a golden part and of a narrow part,
// less a column the narrow table left out and said so: the note, and that
// column's cells in the golden. A cell is the column's when it starts at or
// after its title in the golden's header and before the next title, in the
// lines from the header to the first empty one. A column left out and not
// marked stays, and is cut off. Whether the marked columns may go at all is
// dropFinding's to say.
func withoutMarkedColumns(wantLines, haveLines []string) (want, have []word) {
	want, have = wordsOf(wantLines), wordsOf(haveLines)
	var titles []string
	notes := map[int]bool{}
	for i, l := range haveLines {
		if m := droppedNote.FindStringSubmatch(l); m != nil {
			titles = append(titles, strings.Split(m[1], ", ")...)
			notes[i+1] = true
		}
	}
	have = slices.DeleteFunc(have, func(w word) bool { return notes[w.line] })
	for _, title := range titles {
		cells := columnOf(wantLines, title)
		want = slices.DeleteFunc(want, func(w word) bool { return cells[[2]int{w.line, w.col}] })
	}
	return want, have
}

// tableMeasures is testdata/table-measures.json: term.Look.Table's measures,
// the gap between columns and the least a last column is given, since it wraps,
// before another column goes; and the tables the pty journeys draw, each with
// the order term.Look.Table leaves its columns out in when the width is short:
// the lowest priority first, the later of equals first. The first column is in
// no order, since the last one standing wraps and is never left out. A person
// reads each order from the command that builds the table (from), as a golden
// is read; the binary is never asked (tests.journeys.2). term's own test holds
// its gap and least to this file, so the two cannot drift apart.
type tableMeasures struct {
	Gap     int          `json:"gap"`
	MinLast int          `json:"minLast"`
	Orders  []tableOrder `json:"orders"`
}

// tableOrder is one table's titles, as its 80-column golden draws them, and
// the order its columns are left out in.
type tableOrder struct {
	Titles []string `json:"titles"`
	Order  []string `json:"order"`
	From   string   `json:"from"`
}

// tableMeasuresOf reads testdata/table-measures.json, every field set.
func tableMeasuresOf(t testing.TB) tableMeasures {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "table-measures.json"))
	if err != nil {
		t.Fatal(err)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var m tableMeasures
	if err := d.Decode(&m); err != nil {
		t.Fatalf("testdata/table-measures.json: %v", err)
	}
	if m.Gap < 1 || m.MinLast < 1 || len(m.Orders) == 0 {
		t.Fatalf("testdata/table-measures.json: gap %d, minLast %d and %d orders, each must be set", m.Gap, m.MinLast, len(m.Orders))
	}
	return m
}

// goldenTable is a table of an 80-column golden: its column titles, the width
// each has there, which is its natural width, the order its columns are left
// out in, and term.Look.Table's measures.
type goldenTable struct {
	titles []string
	widths []int
	order  []string
	m      tableMeasures
}

// tableTitle is a column title: words one space apart. Columns are two or more
// apart.
var tableTitle = regexp.MustCompile(`\S+(?: \S+)*`)

// goldenTableOf is the table in lines whose titles have a drop order, or false.
// At 80 columns a table leaves nothing out, so a column's width is where the
// next title starts less the gap, and the last column's is its longest cell.
func goldenTableOf(m tableMeasures, lines []string) (goldenTable, bool) {
	for i, l := range lines {
		var titles []string
		var at []int
		for _, s := range tableTitle.FindAllStringIndex(l, -1) {
			titles = append(titles, l[s[0]:s[1]])
			at = append(at, utf8.RuneCountInString(l[:s[0]]))
		}
		k := slices.IndexFunc(m.Orders, func(o tableOrder) bool { return slices.Equal(o.Titles, titles) })
		if k < 0 {
			continue
		}
		last := len(titles) - 1
		widths := make([]int, len(titles))
		for c := range last {
			widths[c] = at[c+1] - at[c] - m.Gap
		}
		for j := i; j < len(lines) && strings.TrimSpace(lines[j]) != ""; j++ {
			widths[last] = max(widths[last], utf8.RuneCountInString(strings.TrimRight(lines[j], " "))-at[last])
		}
		return goldenTable{titles: titles, widths: widths, order: m.Orders[k].Order, m: m}, true
	}
	return goldenTable{}, false
}

// width is the room the table needs without the columns left out, as
// term.Look.Table measures it: the last column kept needs only room to wrap in.
func (g goldenTable) width(without []string) int {
	n, kept, last := 0, 0, 0
	for k, title := range g.titles {
		if slices.Contains(without, title) {
			continue
		}
		if kept > 0 {
			n += g.m.Gap
		}
		n += g.widths[k]
		last = g.widths[k]
		kept++
	}
	return n - last + min(last, g.m.MinLast)
}

// dropFinding is whether the columns a narrow part's table marks as not shown
// are the ones the width forces out (cli.look.7), as a finding, or "".
// They must be the lowest of the golden table's drop order, in that order, so
// the first column always shows, and no more than cols leaves no room for: the
// one left out last, put back, must not fit. The note names the column left
// out last first. A part that marks nothing has no finding here.
func dropFinding(m tableMeasures, cols int, title string, wantLines, haveLines []string) string {
	var named []string
	for _, l := range haveLines {
		if m := droppedNote.FindStringSubmatch(l); m != nil {
			named = append(named, strings.Split(m[1], ", ")...)
		}
	}
	if len(named) == 0 {
		return ""
	}
	gone := slices.Clone(named)
	slices.Reverse(gone)
	g, ok := goldenTableOf(m, wantLines)
	switch {
	case !ok:
		return fmt.Sprintf("at %d columns, only the columns the width forces out: in %q, %q are marked not shown, and the golden has no table with a drop order",
			cols, title, named)
	case len(gone) > len(g.order):
		return fmt.Sprintf("at %d columns, the first column always shows: in %q, %q are marked not shown, %d of the table's %d columns",
			cols, title, named, len(gone), len(g.titles))
	case !slices.Equal(gone, g.order[:len(gone)]):
		return fmt.Sprintf("at %d columns, only the lowest-priority columns, in their order: in %q, the table leaves out %q, and its order leaves out %q",
			cols, title, gone, g.order[:len(gone)])
	}
	if w := g.width(gone[:len(gone)-1]); w <= cols {
		return fmt.Sprintf("at %d columns, no more than the width forces: in %q, the table with %q put back needs %d columns, inside the %d",
			cols, title, gone[len(gone)-1], w, cols)
	}
	return ""
}

// columnOf is where the words of the column titled title sit in lines, by line
// and column, both counted from one, or none when no line holds the title.
func columnOf(lines []string, title string) map[[2]int]bool {
	for i, l := range lines {
		fs := tokensOf(l)
		for k, f := range fs {
			if f.text != title {
				continue
			}
			from, to := f.col, math.MaxInt
			if k+1 < len(fs) {
				to = fs[k+1].col
			}
			out := map[[2]int]bool{}
			for j := i; j < len(lines) && strings.TrimSpace(lines[j]) != ""; j++ {
				for _, g := range tokensOf(lines[j]) {
					if g.col >= from && g.col < to {
						out[[2]int{j + 1, g.col}] = true
					}
				}
			}
			return out
		}
	}
	return nil
}

// word is one word of a screen and the line it is on, counted from one within
// its part.
type word struct {
	text string
	line int
	col  int    // where it starts on its line, in runes, counted from one
	row  string // the whole line, for the finding
}

// partFinding compares the words of one part, the golden's against the narrow
// run's, and answers the first difference as a finding, or "".
func partFinding(cols int, title string, want, have []word) string {
	i := 0
	for i < len(want) && i < len(have) && want[i].text == have[i].text {
		i++
	}
	switch {
	case i == len(want) && i == len(have):
		return ""
	case i == len(want):
		h := have[i]
		return fmt.Sprintf("at %d columns, the same rows: in %q, line %d (%q) has %q after the golden's last row",
			cols, title, h.line, h.row, h.text)
	}
	w := want[i]
	on, of := rowCount(want, i)
	if i == len(have) {
		return fmt.Sprintf("at %d columns, nothing cut off: in %q, row %d of the golden (%q) stops after %d of its %d words, at %q",
			cols, title, w.line, w.row, on, of, w.text)
	}
	h := have[i]
	if slices.ContainsFunc(have[i:], func(x word) bool { return x.text == w.text }) {
		return fmt.Sprintf("at %d columns, the same rows in the same order: in %q, line %d (%q) has %q where row %d of the golden (%q) has %q, word %d of its %d",
			cols, title, h.line, h.row, h.text, w.line, w.row, w.text, on+1, of)
	}
	return fmt.Sprintf("at %d columns, nothing cut off: in %q, %q of row %d of the golden (%q) is missing, word %d of its %d; line %d reads %q",
		cols, title, w.text, w.line, w.row, on+1, of, h.line, h.row)
}

// rowCount is how many words of want[i]'s row come before it, and how many the
// row has.
func rowCount(want []word, i int) (before, of int) {
	for j, w := range want {
		if w.line != want[i].line {
			continue
		}
		of++
		if j < i {
			before++
		}
	}
	return before, of
}

// wordsOf is the words of a screen in reading order, with the banner and the
// frame left out. The banner is drawn as a two-row wordmark with a tagline
// where it fits and as one heading line, "gragas" and the version, where it
// does not (term.Look.Banner); the wordmark counts as that heading, so the
// version is still compared and the tagline, which carries nothing a person
// needs (ui.Banner: "No information lives here"), is not. A frame word is one
// made only of box-drawing and block characters, such as the bar beside a
// question.
func wordsOf(lines []string) []word {
	var out []word
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		fs := unframed(line)
		if hasBlock(line) && i+1 < len(lines) && hasBlock(lines[i+1]) {
			i++
			line = lines[i]
			fs = append([]token{{text: "gragas"}}, unframed(line)...)
		}
		for _, f := range fs {
			out = append(out, word{text: f.text, line: i + 1, col: f.col, row: strings.TrimSpace(line)})
		}
	}
	return out
}

// hasBlock is whether the line holds a block character, as the wordmark's rows
// do.
func hasBlock(line string) bool {
	return strings.ContainsFunc(line, func(r rune) bool { return r >= 0x2580 && r <= 0x259f })
}

// TestAGoldenTableIsMeasuredAsTermDoes: a golden table's columns are as wide as
// from one title to the next less the gap, the last as its longest cell up to
// the first empty line, and the room the table needs gives the last column kept
// no more than term.Look.Table's least. Risk: a bound that lets a column go that
// still fits, or holds one that does not.
func TestAGoldenTableIsMeasuredAsTermDoes(t *testing.T) {
	t.Parallel()
	m := tableMeasuresOf(t)
	g, ok := goldenTableOf(m, []string{
		"gragas journal list",
		"Run  State  Command             Files  Plan",
		"r1   done   gragas gen --apply  1      ba92826f120b" + strings.Repeat("x", 20),
		"",
		"  done       1 run listed, folder " + strings.Repeat("y", 60),
	})
	if !ok {
		t.Fatal("the journal list's table is not found")
	}
	if want := []int{3, 5, 18, 5, 32}; !slices.Equal(g.widths, want) {
		t.Errorf("widths %v, want %v", g.widths, want)
	}
	for _, tc := range []struct {
		without []string
		want    int
	}{
		{nil, 3 + 5 + 18 + 5 + 16 + 4*m.Gap},
		{[]string{"Plan"}, 3 + 5 + 18 + 5 + 3*m.Gap},
		{[]string{"Files", "Plan"}, 3 + 5 + 16 + 2*m.Gap},
	} {
		if got := g.width(tc.without); got != tc.want {
			t.Errorf("width without %q is %d, want %d", tc.without, got, tc.want)
		}
	}
	if _, ok := goldenTableOf(m, []string{"Run  State", "r1   done"}); ok {
		t.Error("a table with no drop order is found")
	}
}

// narrowOracle is an 80-column transcript for the guards below: the wordmark,
// a framed menu and a table.
const narrowOracle = `$ gragas
--- 80 columns
--- menu

 ▄▀▄▄ █▀▀ █▀█   the rulebook, as presets you import
▀▀█▀  █▄█ █▀▄   v1.2.3 (abc1234)

┃ > check    Check a project against its rules, or a rulebook against its gate
┃   gen      Say which generated files are out of date
┃
--- screen at the end
Run  State  Command             Files  Plan
r1   done   gragas gen --apply  1      ba92826f120b

  done       1 run listed
--- exit 0
`

// narrowClean is narrowOracle as a 40-column screen draws it: the one-line
// banner, each row wrapped, the frame drawn on every line, and the table's
// last column left out and marked.
const narrowClean = `$ gragas
--- 40 columns
--- menu
gragas  v1.2.3 (abc1234)
┃ > check    Check a project against its
┃   rules, or a rulebook against its
┃   gate
┃   gen      Say which generated files
┃   are out of date
--- screen at the end
Run  State  Command             Files
r1   done   gragas gen --apply  1
1 column not shown: Plan

  done       1 run listed
--- exit 0
`

// TestTheNarrowPropertiesFindEachBreak: each property, broken once in a
// 40-column transcript, is a finding that names the width, the row and the
// measure; the clean transcript, wrapped and with a marked column, is none.
// Risk: a property that passes whatever the screen shows.
func TestTheNarrowPropertiesFindEachBreak(t *testing.T) {
	t.Parallel()
	m := tableMeasuresOf(t)
	// table40 is the clean screen's table, the part a drop plant rewrites.
	const table40 = "Run  State  Command             Files\nr1   done   gragas gen --apply  1\n1 column not shown: Plan\n"
	edit := func(from, to string) string {
		if !strings.Contains(narrowClean, from) {
			t.Fatalf("the plant's text %q is not in the clean transcript", from)
		}
		return strings.Replace(narrowClean, from, to, 1)
	}
	for _, tc := range []struct {
		name string
		got  string
		over []overrun
		want []string // what the one finding says; none for a control
	}{
		{name: "control: wrapped, framed and a marked column", got: narrowClean},
		{name: "control: the wordmark at 40 too", got: edit("gragas  v1.2.3 (abc1234)", " ▄▀▄▄ █▀▀ █▀█   tagline\n▀▀█▀  █▄█ █▀▄   v1.2.3 (abc1234)")},
		{name: "a line past the edge", got: narrowClean, over: []overrun{{row: 7, width: 47, text: "┃   gen      Say which generated files"}},
			want: []string{"at 40 columns", "wider than the screen", "row 7", `"┃   gen      Say which generated files"`, "47 columns wide, 7 past the edge"}},
		{name: "the last row cut off", got: edit("┃   are out of date\n", ""),
			want: []string{"at 40 columns", "nothing cut off", `"--- menu"`, "row 6 of the golden", "stops after 5 of its 9 words", `"are"`}},
		{name: "a word dropped from a row", got: edit("rules, or a", "rules, a"),
			want: []string{"at 40 columns", "nothing cut off", `"or" of row 5`, "word 9 of its 14", `line 3 reads "┃   rules, a rulebook against its"`}},
		{name: "two rows swapped", got: edit("┃ > check    Check a project against its\n┃   rules, or a rulebook against its\n┃   gate\n┃   gen      Say which generated files\n┃   are out of date\n",
			"┃   gen      Say which generated files\n┃   are out of date\n┃ > check    Check a project against its\n┃   rules, or a rulebook against its\n┃   gate\n"),
			want: []string{"at 40 columns", "the same rows in the same order", `has "gen" where row 5 of the golden`, `has ">"`, "word 1 of its 14"}},
		{name: "a row added", got: edit("  done       1 run listed\n", "  done       1 run listed\n  and one more\n"),
			want: []string{"at 40 columns", "the same rows", `line 6 ("and one more") has "and" after the golden's last row`}},
		{name: "a column left out and not marked", got: edit("1 column not shown: Plan\n", ""),
			want: []string{"at 40 columns", "nothing cut off", `"Plan" of row 1`}},
		{name: "a column marked that is not the one left out", got: edit("not shown: Plan", "not shown: Files"),
			want: []string{"at 40 columns", "only the lowest-priority columns, in their order", `leaves out ["Files"]`, `its order leaves out ["Plan"]`}},
		{name: "a column left out of its order", got: edit(table40, "Run  State  Command             Plan\nr1   done   gragas gen --apply  ba92826f120b\n1 column not shown: Files\n"),
			want: []string{"at 40 columns", "only the lowest-priority columns, in their order", `in "--- screen at the end"`, `leaves out ["Files"]`, `its order leaves out ["Plan"]`}},
		{name: "two columns left out where the width forces one", got: edit(table40, "Run  State  Command\nr1   done   gragas gen --apply\n2 columns not shown: Files, Plan\n"),
			want: []string{"at 40 columns", "no more than the width forces", `with "Files" put back needs 37 columns, inside the 40`}},
		{name: "every column marked not shown", got: edit(table40, "5 columns not shown: Run, State, Command, Files, Plan\n"),
			want: []string{"at 40 columns", "the first column always shows", `5 of the table's 5 columns`}},
		{name: "a column marked where the golden draws no table", got: edit("┃   are out of date\n", "┃   are out of date\n1 column not shown: gen\n"),
			want: []string{"at 40 columns", `in "--- menu"`, "the golden has no table with a drop order"}},
		{name: "another version in the banner", got: edit("v1.2.3 (abc1234)", "v9.9.9 (abc1234)"),
			want: []string{"at 40 columns", "nothing cut off", `"v1.2.3" of row 3`, `line 1 reads "gragas  v9.9.9 (abc1234)"`}},
		{name: "a part missing", got: edit("--- menu\n", ""),
			want: []string{"at 40 columns", "the parts are", `"--- screen at the end"`}},
		{name: "another width", got: edit("--- 40 columns", "--- 80 columns"),
			want: []string{"at 40 columns", `says "--- 80 columns", not "--- 40 columns"`}},
		{name: "another command", got: edit("$ gragas\n", "$ gragas help\n"),
			want: []string{"at 40 columns", `the run is "$ gragas help"`}},
		{name: "another ending", got: edit("--- exit 0", "--- exit 1"),
			want: []string{"at 40 columns", "the parts are", `"--- exit 1"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := narrowFindings(m, 40, narrowOracle, tc.got, tc.over)
			if len(tc.want) == 0 {
				if len(got) != 0 {
					t.Fatalf("control: findings %q", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("findings %q, want one", got)
			}
			for _, w := range tc.want {
				if !strings.Contains(got[0], w) {
					t.Errorf("the finding %q does not say %q", got[0], w)
				}
			}
		})
	}
}
