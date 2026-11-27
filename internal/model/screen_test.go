package model_test

import (
	"errors"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

func closing(t *testing.T, w model.Word, next string) model.Closing {
	t.Helper()
	c, err := model.NewClosing(w, "what", next)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestAScreenEndsOnOneClosingWord: a screen with no closing word, two, or one
// that is not last is refused (cli.look.5). Planted: each break; control: a
// whole screen.
func TestAScreenEndsOnOneClosingWord(t *testing.T) {
	t.Parallel()
	done := closing(t, model.Done, "")
	good, err := model.NewScreen(model.Heading{Text: "check"}, model.Note{Text: "n"}, done)
	if err != nil || len(good.Blocks()) != 3 {
		t.Fatalf("a whole screen = %v, %v", good, err)
	}
	for _, c := range []struct {
		name   string
		blocks []model.Block
	}{
		{"none at all", nil},
		{"no closing word", []model.Block{model.Heading{Text: "check"}}},
		{"two", []model.Block{done, done}},
		{"one that is not last", []model.Block{done, model.Note{Text: "late"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := model.NewScreen(c.blocks...); !errors.Is(err, model.ErrNoClosing) {
				t.Errorf("NewScreen = %v, want ErrNoClosing", err)
			}
		})
	}
}

// TestTheDryRunFooterIsOneAndJustBeforeTheClosingWord: cli.look.6, planted as
// a footer first, a second footer, and a footer with no flag.
func TestTheDryRunFooterIsOneAndJustBeforeTheClosingWord(t *testing.T) {
	t.Parallel()
	declined := closing(t, model.Declined, "gragas gen --apply")
	foot := model.DryRunFooter{Flag: "--apply"}
	if _, err := model.NewScreen(model.Note{Text: "n"}, foot, declined); err != nil {
		t.Errorf("a footer before the closing word = %v", err)
	}
	for _, c := range []struct {
		name   string
		blocks []model.Block
		want   error
	}{
		{"first", []model.Block{foot, model.Note{Text: "n"}, declined}, model.ErrFooterPlace},
		{"twice", []model.Block{foot, foot, declined}, model.ErrFooterPlace},
		{"with no flag", []model.Block{model.DryRunFooter{}, declined}, model.ErrNoFlag},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := model.NewScreen(c.blocks...); !errors.Is(err, c.want) {
				t.Errorf("NewScreen = %v, want %v", err, c.want)
			}
		})
	}
}

// TestAClosingNeedsAKnownWordAndTheLineToType: only the eight words close, and
// the five that refuse carry their line (cli.errors.2).
func TestAClosingNeedsAKnownWordAndTheLineToType(t *testing.T) {
	t.Parallel()
	if _, err := model.NewClosing("shrugged", "x", ""); !errors.Is(err, model.ErrUnknownWord) {
		t.Errorf("an unknown word = %v, want ErrUnknownWord", err)
	}
	n := 0
	for _, w := range model.Words() {
		_, err := model.NewClosing(w, "x", "")
		switch w {
		case model.Usage, model.Busy, model.NotReady, model.Refused, model.Declined:
			n++
			if !errors.Is(err, model.ErrNoNext) {
				t.Errorf("%q with no line to type = %v, want ErrNoNext", w, err)
			}
		default:
			if err != nil {
				t.Errorf("%q needs no line to type, got %v", w, err)
			}
		}
	}
	if n != 5 {
		t.Errorf("%d words refuse, want 5", n)
	}
}

// TestABadBlockIsRefused: a nil block and a table with no column.
func TestABadBlockIsRefused(t *testing.T) {
	t.Parallel()
	done := closing(t, model.Done, "")
	for name, b := range map[string]model.Block{"nil": nil, "a table with no column": model.Table{}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := model.NewScreen(b, done); !errors.Is(err, model.ErrBadBlock) {
				t.Errorf("NewScreen = %v, want ErrBadBlock", err)
			}
		})
	}
}

// TestACallerCannotChangeAScreenThroughItsBlocks: Blocks answers a copy.
func TestACallerCannotChangeAScreenThroughItsBlocks(t *testing.T) {
	t.Parallel()
	s, err := model.NewScreen(model.Heading{Text: "a"}, closing(t, model.Done, ""))
	if err != nil {
		t.Fatal(err)
	}
	s.Blocks()[0] = model.Note{Text: "changed"}
	if h, ok := s.Blocks()[0].(model.Heading); !ok || h.Text != "a" {
		t.Error("a caller changed the screen")
	}
}
