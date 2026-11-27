package term_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/term"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// TestEveryTextColourInTheTokenFileIsReadable: each role's colour clears 4.5:1
// on the ground it is drawn on, light and dark (cli.look.2). Planted: a pale
// colour on white, and a dark one on the dark ground, each named.
func TestEveryTextColourInTheTokenFileIsReadable(t *testing.T) {
	t.Parallel()
	p, err := term.LoadPalette()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Text) < 5 {
		t.Fatalf("the token file holds %d text roles, want the five the look draws with", len(p.Text))
	}
	for _, bad := range term.Unreadable(p) {
		t.Error(bad)
	}
	t.Run("a pale colour on white is named", func(t *testing.T) {
		t.Parallel()
		q := p
		q.Text = map[string]term.Ground{"warn": {Light: "#ffe58a", Dark: p.Text["good"].Dark}}
		got := term.Unreadable(q)
		if len(got) != 1 || !strings.HasPrefix(got[0], "warn on the light ground") {
			t.Errorf("Unreadable = %v, want the one light colour", got)
		}
	})
	t.Run("a dark colour on the dark ground is named", func(t *testing.T) {
		t.Parallel()
		q := p
		q.Text = map[string]term.Ground{"warn": {Light: p.Text["good"].Light, Dark: "#333333"}}
		got := term.Unreadable(q)
		if len(got) != 1 || !strings.HasPrefix(got[0], "warn on the dark ground") {
			t.Errorf("Unreadable = %v, want the one dark colour", got)
		}
	})
}

// TestTheTerminalDrawsTheRolesOfTheTokenDocument: the terminal's palette is
// the explorer's token document read as it stands: the same text role names,
// each with the document's light and dark values, on the document's page
// ground (web.tokens.19, cli.look.2). The document is read as JSON
// here, since a dev package is no import of the host; the other half, that
// palette.json is exactly what the document generates, is in internal/dev/tokens.
func TestTheTerminalDrawsTheRolesOfTheTokenDocument(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../../explorer/packages/tokens/tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Grounds map[string]term.Ground `json:"grounds"`
		Text    map[string]term.Ground `json:"text"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	p, err := term.LoadPalette()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Text) != len(doc.Text) || len(doc.Text) == 0 {
		t.Errorf("the terminal draws %d text roles, the document names %d", len(p.Text), len(doc.Text))
	}
	for role, want := range doc.Text {
		if got, ok := p.Text[role]; !ok || got != want {
			t.Errorf("role %s: the terminal has %v (present %v), the document %v", role, got, ok, want)
		}
	}
	if p.Grounds != doc.Grounds["page"] {
		t.Errorf("the terminal's ground is %v, the document's page %v", p.Grounds, doc.Grounds["page"])
	}
}

// TestTheContrastIsTheWCAGRatio: white on black is 21:1 and a colour on itself
// is 1:1, so the number the palette test reads is the standard's.
func TestTheContrastIsTheWCAGRatio(t *testing.T) {
	t.Parallel()
	if r, err := term.Contrast("#ffffff", "#000000"); err != nil || r < 20.99 || r > 21.01 {
		t.Errorf("white on black = %v, %v, want 21", r, err)
	}
	if r, err := term.Contrast("#777777", "#777777"); err != nil || r != 1 {
		t.Errorf("a colour on itself = %v, %v, want 1", r, err)
	}
	if _, err := term.Contrast("red", "#000000"); err == nil {
		t.Error("a colour that is not #rrggbb was taken")
	}
}

// TestAPaletteWithoutAColourIsRefused: a file that holds no text colours, or a
// colour that is no hex, does not load.
func TestAPaletteWithoutAColourIsRefused(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, doc string }{
		{"no text roles", `{"grounds":{"light":"#ffffff","dark":"#000000"},"text":{}}`},
		{"a colour that is no hex", `{"grounds":{"light":"#ffffff","dark":"#000000"},"text":{"x":{"light":"blue","dark":"#ffffff"}}}`},
		{"not JSON", `{`},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := term.ParsePalette([]byte(c.doc)); err == nil {
				t.Error("the palette was taken")
			}
		})
	}
}
