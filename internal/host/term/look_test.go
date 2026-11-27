package term_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"

	"github.com/birdtable/gragas/internal/host/term"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port/contracttest"
)

// looks is the two forms the goldens hold: plain, what a pipe gets, and
// accessible, which paints nothing and leaves the banner out. Both are drawn
// at the width each test gives them (cli.environment.2); the pty runs at 80
// are M0b-12.
func looks() map[string]term.Look {
	return map[string]term.Look{
		"plain":      {},
		"accessible": {Rich: true, Accessible: true},
	}
}

// section is one titled block of a golden.
func section(b *strings.Builder, title, body string) {
	fmt.Fprintf(b, "=== %s\n%s", title, body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
}

// everyWord is each of the eight closing words, with the line to type for the
// ones that refuse.
func everyWord(t *testing.T) []term.Ending {
	t.Helper()
	var out []term.Ending
	for _, w := range model.Words() {
		next := ""
		switch w {
		case model.Usage, model.Busy, model.NotReady, model.Refused, model.Declined:
			next = "gragas " + strings.ReplaceAll(string(w), " ", "-") + " --help"
		}
		out = append(out, term.Ending{Word: w, What: "what happened to the run, in its own terms", Next: next})
	}
	if len(out) != 8 {
		t.Fatalf("%d closing words, want eight", len(out))
	}
	return out
}

// TestTheGoldensHoldTheLook: the banner, a table with a wrapped cell and a
// dropped column, each of the eight closing words, and an error with a pasted
// hint, at 80, 60 and 40 columns in plain and accessible form, each compared
// with its golden and never written by the gate (tests.generated.5). A golden
// is written only by go test -run TestTheGoldensHoldTheLook -update, and a
// person reads the diff. Risk: a look that changes shape unseen.
func TestTheGoldensHoldTheLook(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	cols := []term.Column{{Title: "rule", Priority: 4}, {Title: "owner", Priority: 1}, {Title: "status", Priority: 3}, {Title: "reason", Priority: 5}}
	rows := [][]string{
		{"cli.look.5", "host/term", "active", "The closing words are a closed set, built with their next line by constructors only, and nothing else ends a run."},
		{"arch.ports.6", "layers", "active", "Every library has one owner."},
		{"cli.look.7", "layers", "active", "Short."},
	}
	n := 0
	for name, look := range looks() {
		for _, width := range []int{80, 60, 40} {
			t.Run(name+"-"+strconv.Itoa(width), func(t *testing.T) {
				look.Width = width
				var b strings.Builder
				section(&b, "banner", look.Banner("v0.1.0"))
				section(&b, "table", look.Table(cols, rows))
				for _, e := range everyWord(t) {
					section(&b, "closing "+string(e.Word), look.Closing(e))
				}
				section(&b, "error with a pasted hint", look.Problem("unknown flag: --aply\ndid you mean `gragas gen --apply`?"))
				uitest.Golden(t, "look-"+name+"-"+strconv.Itoa(width), b.String())
				n++
			})
		}
	}
	if n != 6 {
		t.Errorf("%d goldens held, want six", n)
	}
}

// TestAScreenOfEveryBlockHasGoldens: Terminal.Show's blocks in order, in the two
// forms at the three widths, ending on the dry-run footer and the closing word
// (cli.look.6).
func TestAScreenOfEveryBlockHasGoldens(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	n := 0
	for name, look := range looks() {
		for _, width := range []int{80, 60, 40} {
			t.Run(name+"-"+strconv.Itoa(width), func(t *testing.T) {
				look.Width = width
				got, err := look.Screen(contracttest.EveryBlock(t))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasSuffix(strings.TrimSpace(got), "nothing was changed\n    gragas gen --apply") {
					t.Errorf("the screen does not end on the word and its line:\n%s", got)
				}
				uitest.Golden(t, "screen-"+name+"-"+strconv.Itoa(width), got)
				n++
			})
		}
	}
	if n != 6 {
		t.Errorf("%d goldens held, want six", n)
	}
}

// TestNoLineIsWiderThanTheScreen: a table at 30 columns drops the columns of
// least priority, wraps the last, marks what went, and keeps every line inside
// the width; at 96 and wider nothing is dropped (cli.look.7).
func TestNoLineIsWiderThanTheScreen(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	cols := []term.Column{{Title: "rule", Priority: 4}, {Title: "owner", Priority: 1}, {Title: "reason", Priority: 5}}
	rows := [][]string{{"cli.look.5", "host/term", "A reason long enough that it has to wrap at forty columns, and then some."}}
	got := term.Look{Width: 30}.Table(cols, rows)
	for l := range strings.SplitSeq(strings.TrimRight(got, "\n"), "\n") {
		if n := len([]rune(l)); n > 30 {
			t.Errorf("a line is %d columns wide: %q", n, l)
		}
	}
	if !strings.Contains(got, "1 column not shown: owner") {
		t.Errorf("the dropped column is not marked:\n%s", got)
	}
	if strings.Contains(got, "│") || strings.Contains(got, "─") || strings.Contains(got, "|") {
		t.Errorf("a pipe gets rules:\n%s", got)
	}
	if wide := (term.Look{Width: 200}).Table(cols, rows); strings.Contains(wide, "not shown") || !strings.Contains(wide, "owner") {
		t.Errorf("a wide table dropped a column:\n%s", wide)
	}
}

// TestAPaintedTableLinesUpAsAPlainOne: a table on a colour terminal, with its
// sequences taken out, is the table a pipe gets at the same width, so its bold
// header pads to the columns of the rows under it and stays inside the screen
// (cli.look.7). Risk: a header padded by the bytes of its own colour, wider than
// its rows and than the screen (found by the pty journeys at 60 and 40 columns,
// tests.journeys.8).
func TestAPaintedTableLinesUpAsAPlainOne(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	cols := []term.Column{{Title: "rule", Priority: 4}, {Title: "owner", Priority: 1}, {Title: "reason", Priority: 5}}
	rows := [][]string{{"cli.look.5", "host/term", "A reason long enough that it has to wrap at forty columns, and then some."}}
	for _, width := range []int{80, 40, 30} {
		plain := term.Look{Width: width}.Table(cols, rows)
		rich := term.Look{Rich: true, Width: width}.Table(cols, rows)
		if !strings.Contains(rich, "\x1b[") {
			t.Fatalf("at %d columns the painted table has no colour, so it proves nothing: %q", width, rich)
		}
		var lines []string
		for l := range strings.SplitSeq(rich, "\n") {
			lines = append(lines, ansi.ReplaceAllString(l, ""))
		}
		if got := strings.Join(lines, "\n"); got != plain {
			t.Errorf("at %d columns the painted table, unpainted, is\n%s\nand a pipe's is\n%s", width, got, plain)
		}
	}
}

// TestAColourTerminalPaintsAndAPipeDoesNot: the same word, painted with the
// kit's sequences when the look is rich and bare when it is not.
func TestAColourTerminalPaintsAndAPipeDoesNot(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	e := term.Ending{Word: model.Findings, What: "2 problems", Next: ""}
	if plain := (term.Look{}).Closing(e); strings.Contains(plain, "\x1b") {
		t.Errorf("a pipe was painted: %q", plain)
	}
	if rich := (term.Look{Rich: true, Width: 80}).Closing(e); !strings.Contains(rich, "\x1b[") {
		t.Errorf("a colour terminal got no colour: %q", rich)
	}
	if acc := (term.Look{Rich: true, Accessible: true, Width: 80}).Closing(e); strings.Contains(acc, "\x1b") {
		t.Errorf("the accessible form was painted: %q", acc)
	}
}

// TestAWordOutsideTheEightIsDrawnAsFailed: the closed set stays closed, and a
// Screen that was not built is refused by Show.
func TestAWordOutsideTheEightIsDrawnAsFailed(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	got := term.Look{}.Closing(term.Ending{Word: "shrugged", What: "x"})
	if !strings.Contains(got, "failed") || strings.Contains(got, "shrugged") {
		t.Errorf("Closing = %q, want failed", got)
	}
	if _, err := (term.Look{}).Screen(model.Screen{}); err == nil {
		t.Error("a screen with no block was drawn")
	}
}

// TestTheCommandTreeSaysNumbersAsTheKitDoes: model.Count and model.Plural, which
// the command tree uses because it may not import the look (cli.look.3,
// arch.shared.7), give a count its separators and a noun its plural, 1 rule, 2
// rules, 165,440 rules, 3 people. go-console v0.16.0 dropped the ui.Plural the
// look borrowed before, so these are held to the forms the table
// names: 1 gives "1 " and the singular, any other count gives model.Count and
// the plural the table pairs with it.
func TestTheCommandTreeSaysNumbersAsTheKitDoes(t *testing.T) {
	t.Parallel()
	nouns := [][2]string{{"rule", "rules"}, {"entry", "entries"}, {"key", "keys"}, {"person", "people"}, {"other person", "other people"}, {"day", "days"}}
	ran := 0
	for _, n := range []int{-5, 0, 1, 2, 999, 1000, 12345, 165440, 1234567} {
		for _, noun := range nouns {
			ran++
			want := model.Count(n) + " " + noun[1]
			if n == 1 {
				want = "1 " + noun[0]
			}
			if got := model.Plural(n, noun[0]); got != want {
				t.Errorf("model.Plural(%d, %q) = %q, want %q", n, noun[0], got, want)
			}
		}
	}
	if ran == 0 {
		t.Fatal("the table of counts and nouns is empty")
	}
}

// TestADoneThatWaitsShowsTheLineToType: a dry run ends done with the line that
// applies it, drawn under the word, and a done with none draws no second line.
func TestADoneThatWaitsShowsTheLineToType(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	l := term.Look{Width: 80}
	got := l.Closing(term.Ending{Word: model.Done, What: "nothing was written yet", Next: "gragas gen --apply"})
	if !strings.HasSuffix(got, "nothing was written yet\n    gragas gen --apply\n") {
		t.Errorf("Closing = %q, want the apply line under the word", got)
	}
	if got := l.Closing(term.Ending{Word: model.Done, What: "all current"}); strings.Count(got, "\n") != 1 {
		t.Errorf("Closing = %q, want one line", got)
	}
}

// TestAHeadingAfterATableStandsApartFromIt: a heading that is not the first
// block, as validate's Notices after its findings table, has one blank line
// before it (docs/boards/findings.md), and the first heading none.
func TestAHeadingAfterATableStandsApartFromIt(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	c, err := model.NewClosing(model.Done, "done", "")
	if err != nil {
		t.Fatal(err)
	}
	s, err := model.NewScreen(model.Heading{Text: "validate"},
		model.Table{Columns: []model.Column{{Title: "rule", Priority: 1}}, Rows: [][]string{{"a.b.1"}}},
		model.Heading{Text: "Notices"}, model.List{Items: []string{"t.yaml:3: stale"}}, c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := term.Look{}.Screen(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "a.b.1\n\nNotices\n") || strings.HasPrefix(got, "\n") || strings.Contains(got, "\n\n\nNotices") {
		t.Errorf("Notices does not stand one blank line after the table:\n%q", got)
	}
}
