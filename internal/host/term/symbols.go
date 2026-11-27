package term

import (
	"github.com/birdtable/go-console/ui"

	"github.com/birdtable/gragas/internal/model"
)

// The symbols of the accessible form (spec section 17, cli.environment.7):
// where a word is coloured, accessible output puts a symbol and the word
// beside it, so nothing is told by colour alone. Status and standing take the
// symbols section 17 names. The closing words have none in the spec, so the
// set below is the board's (docs/boards/closing-words.md): ASCII, one
// character each, and none is a section 17 symbol, so no screen shows one
// character with two meanings. TestNoClosingSymbolIsAStatusOrStandingSymbol
// holds it.

// closingSymbol is the symbol drawn beside each closing word.
func closingSymbol(w model.Word) string {
	switch w {
	case model.Done:
		return "*"
	case model.Findings:
		return "!"
	case model.Failed:
		return "#"
	case model.Refused:
		return "<"
	case model.Declined:
		return ">"
	case model.Usage:
		return "/"
	case model.Busy:
		return "&"
	case model.NotReady:
		return "%"
	}
	return "#" // a word outside the eight is drawn as failed (displayWord)
}

// ClosingSymbol is the symbol the accessible form draws beside the closing
// word w.
func ClosingSymbol(w model.Word) string { return closingSymbol(w) }

// statusSymbol and standingSymbol are the symbols of spec sections 17.1 and
// 17.2, by word. A word they do not know has none.
func statusSymbol(word string) (string, bool) {
	sym, ok := map[string]string{"proposed": "?", "trial": "^", "active": "=", "retired": "x"}[word]
	return sym, ok
}

func standingSymbol(word string) (string, bool) {
	sym, ok := map[string]string{"proposed": "?", "enforced": "+", "reviewed": "~", "planned": "o", "advisory": "-", "retired": "x"}[word]
	return sym, ok
}

// Status is a rule's status as it is drawn: the word, in its colour where the
// look paints, and in the accessible and the plain form the symbol and the
// word. A word that is not a status is returned as it came.
func (l Look) Status(word string) string {
	return l.tag(statusSymbol, word, func(s *ui.Session) func(string) string { return s.Note })
}

// Standing is a rule's derived standing, drawn as Status is.
func (l Look) Standing(word string) string {
	return l.tag(standingSymbol, word, func(s *ui.Session) func(string) string {
		switch word {
		case "enforced":
			return s.Good
		case "planned":
			return s.Attn
		case "reviewed":
			return s.Action
		}
		return s.Note
	})
}

// tag is word as Status and Standing draw it, painted in the colour paint picks
// from the session it is drawn in.
func (l Look) tag(symbolOf func(string) (string, bool), word string, paint func(*ui.Session) func(string) string) string {
	word = Strip(word)
	sym, ok := symbolOf(word)
	if !ok {
		return word
	}
	if l.Accessible || !l.Rich {
		return sym + " " + word
	}
	var out string
	l.draw(func(s *ui.Session) { out = paint(s)(word) })
	return out
}
