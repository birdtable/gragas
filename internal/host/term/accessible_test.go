package term_test

import (
	"strconv"

	"github.com/birdtable/go-console/uitest"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/host/term"
	"github.com/birdtable/gragas/internal/model"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func accessibleLook() term.Look { return term.Look{Rich: true, Accessible: true, Width: 80} }
func colouredLook() term.Look   { return term.Look{Rich: true, Width: 80} }

func statusWords() []string { return []string{"proposed", "trial", "active", "retired"} }
func standingWords() []string {
	return []string{"proposed", "enforced", "reviewed", "planned", "advisory", "retired"}
}

// tokens is every coloured token the look can draw, as the text that draws it
// in a look: the eight closing words, the statuses and the standings.
func tokens(l term.Look) map[string]string {
	out := map[string]string{}
	for _, w := range model.Words() {
		out["closing "+string(w)] = l.Closing(term.Ending{Word: w, What: "the run ended"})
	}
	for _, w := range statusWords() {
		out["status "+w] = l.Status(w)
	}
	for _, w := range standingWords() {
		out["standing "+w] = l.Standing(w)
	}
	return out
}

// TestAccessibleOutputHasAWordAndASymbolForEveryColouredToken: for each token
// the coloured look paints, the accessible look gives the word and a symbol
// and no escape byte, so nothing is told by colour alone (cli.environment.7).
func TestAccessibleOutputHasAWordAndASymbolForEveryColouredToken(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	paintedAny := false
	cc := tokens(colouredLook())
	for name, got := range tokens(accessibleLook()) {
		word := name[strings.Index(name, " ")+1:]
		if strings.Contains(got, "\x1b") {
			t.Errorf("%s: an escape byte in accessible output: %q", name, got)
		}
		if !strings.Contains(got, word) {
			t.Errorf("%s: no word in %q", name, got)
		}
		fields := strings.Fields(got)
		if len(fields) < 2 || len([]rune(fields[0])) != 1 || strings.Trim(fields[0], "?^=x+~o-!*#/&%<>") != "" {
			t.Errorf("%s: no symbol before the word in %q", name, got)
		}
		if strings.Contains(cc[name], "\x1b") {
			paintedAny = true
		}
	}
	if !paintedAny {
		t.Fatal("the coloured look painted nothing, so the table proves nothing")
	}
}

// TestTheClosingSymbolsAreDistinct: two words never share a symbol, so the
// symbol alone names the word.
func TestTheClosingSymbolsAreDistinct(t *testing.T) {
	t.Parallel()
	seen := map[string]model.Word{}
	for _, w := range model.Words() {
		s := term.ClosingSymbol(w)
		if other, dup := seen[s]; dup {
			t.Errorf("%s and %s share %q", w, other, s)
		}
		seen[s] = w
	}
}

// TestPlainAndAccessibleCarryTheFactsOfColoured: strip the decoration (colour
// and the symbol) and the words are the coloured output's.
func TestPlainAndAccessibleCarryTheFactsOfColoured(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	facts := func(s string, symbols []string) []string {
		var out []string
		for f := range strings.FieldsSeq(ansi.ReplaceAllString(s, "")) {
			if !slices.Contains(symbols, f) {
				out = append(out, f)
			}
		}
		return out
	}
	symbols := strings.Fields("+ ! x - ~ ? o ^ = * # / & % < >")
	plain := term.Look{}
	rich := tokens(colouredLook())
	for name, acc := range tokens(accessibleLook()) {
		want := strings.Join(facts(rich[name], nil), " ")
		if got := strings.Join(facts(acc, symbols), " "); got != want {
			t.Errorf("%s: accessible says %q, coloured says %q", name, got, want)
		}
		if got := strings.Join(facts(tokens(plain)[name], symbols), " "); got != want {
			t.Errorf("%s: plain says %q, coloured says %q", name, got, want)
		}
	}
}

// TestALabelWithAnEscapeByteIsStrippedInAccessibleMode plants an escape
// sequence in the places a name from outside reaches.
func TestALabelWithAnEscapeByteIsStrippedInAccessibleMode(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	evil := "\x1b[31mred\x1b[0m"
	drawn := map[string]string{
		"closing sentence": accessibleLook().Closing(term.Ending{Word: model.Done, What: evil, Next: "gragas " + evil}),
		"status":           accessibleLook().Status(evil),
		"standing":         accessibleLook().Standing(evil),
		"table cell":       accessibleLook().Table([]term.Column{{Title: "t", Priority: 1}}, [][]string{{evil}}),
	}
	for name, got := range drawn {
		if strings.Contains(got, "\x1b") {
			t.Errorf("%s kept an escape byte: %q", name, got)
		}
	}
}

// TestAFindingsListingHasAccessibleGoldens: a listing of findings that ends on
// the findings word, in plain and accessible form at 80, 60 and 40 columns.
// The accessible form carries the symbol beside the word and every fact the
// plain form has.
func TestAFindingsListingHasAccessibleGoldens(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	end, err := model.NewClosing(model.Findings, "3 findings, 1 of them a failure", "gragas check --json")
	if err != nil {
		t.Fatal(err)
	}
	screen, err := model.NewScreen(
		model.Heading{Text: "check"}, model.Reads{What: "rulebook/ in the project"},
		model.Table{Columns: []model.Column{{Title: "rule", Priority: 4}, {Title: "where", Priority: 2}, {Title: "what", Priority: 5}},
			Rows: [][]string{
				{"cli.look.5", "internal/cli", "A closing word is built by hand, not by its constructor."},
				{"arch.ports.6", "go.mod", "A library has two owners."},
				{"cli.look.7", "docs/x.md", "A column is dropped without a mark."},
			}},
		end)
	if err != nil {
		t.Fatal(err)
	}
	plain, rich := map[int]string{}, map[int]string{}
	for _, width := range []int{80, 60, 40} {
		t.Run(strconv.Itoa(width)+" columns", func(t *testing.T) {
			acc := accessibleLook()
			acc.Width = width
			for name, look := range map[string]term.Look{"plain": {Width: width}, "accessible": acc} {
				got, err := look.Screen(screen)
				if err != nil {
					t.Fatal(err)
				}
				uitest.Golden(t, "findings-"+name+"-"+strconv.Itoa(width), got)
				if name == "plain" {
					plain[width] = got
				} else {
					rich[width] = got
				}
			}
			if !strings.Contains(rich[width], "! findings") {
				t.Errorf("no symbol beside the word:\n%s", rich[width])
			}
			if strings.Join(strings.Fields(strings.Replace(rich[width], "! findings", "findings", 1)), " ") != strings.Join(strings.Fields(plain[width]), " ") {
				t.Errorf("the accessible listing carries other facts than the plain one:\n%s\n--- plain\n%s", rich[width], plain[width])
			}
		})
	}
}

// TestNoClosingSymbolIsAStatusOrStandingSymbol: a closing word's symbol is
// none of the symbols section 17 gives a status or a standing, so a screen
// that ends on a word and lists rules never shows one character for two
// meanings.
func TestNoClosingSymbolIsAStatusOrStandingSymbol(t *testing.T) {
	t.Parallel() // each draw is a session of its own (look.go draw)
	section17 := map[string]string{}
	look := term.Look{}
	for _, w := range []string{"proposed", "trial", "active", "retired"} {
		sym, _, _ := strings.Cut(look.Status(w), " ")
		section17[sym] = "status " + w
	}
	for _, w := range []string{"proposed", "enforced", "reviewed", "planned", "advisory", "retired"} {
		sym, _, _ := strings.Cut(look.Standing(w), " ")
		section17[sym] = "standing " + w
	}
	if len(section17) < 8 {
		t.Fatalf("only %d section 17 symbols read: %v", len(section17), section17)
	}
	for _, w := range model.Words() {
		if meaning, clash := section17[term.ClosingSymbol(w)]; clash {
			t.Errorf("the symbol %q of %s is also the symbol of %s", term.ClosingSymbol(w), w, meaning)
		}
	}
}
