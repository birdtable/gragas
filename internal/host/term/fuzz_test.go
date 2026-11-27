package term_test

import (
	"regexp"
	"testing"

	"github.com/birdtable/gragas/internal/host/term"
	"github.com/birdtable/gragas/internal/model"
)

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// paletteSeed is a token file of one role, as the committed file is shaped.
const paletteSeed = `{"grounds":{"light":"#ffffff","dark":"#000000"},"text":{"good":{"light":"#116611","dark":"#66dd66"}}}`

// FuzzParsePalette (tests.generated.1): the token file is input the program
// did not write once a person edits it. Whatever the bytes, the parser does
// not panic; it answers a palette in which every colour is #rrggbb and there
// is at least one text role, or an error and no palette.
func FuzzParsePalette(f *testing.F) {
	for _, s := range []string{paletteSeed, `{}`, ``, `null`, `[]`, `{"text":{}}`, `{"grounds":{"light":"#fff","dark":"#000"},"text":{"a":{"light":"#ffffff","dark":"#000000"}}}`,
		`{"grounds":{"light":"#ffffff","dark":"#000000"},"text":{"a":{"light":"#ggg000","dark":"#000000"}}}`, paletteSeed[:len(paletteSeed)-1]} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := term.ParsePalette(b)
		if err != nil {
			if len(p.Text) > 0 || p.Grounds != (term.Ground{}) {
				t.Fatalf("a palette %+v beside the error %v", p, err)
			}
			return
		}
		if len(p.Text) == 0 {
			t.Fatal("a palette with no text role")
		}
		all := []string{p.Grounds.Light, p.Grounds.Dark}
		for _, g := range p.Text {
			all = append(all, g.Light, g.Dark)
		}
		for _, c := range all {
			if !hexColour.MatchString(c) {
				t.Fatalf("colour %q is not #rrggbb", c)
			}
		}
	})
}

// FuzzStrip: whatever the text, Strip leaves
// nothing the terminal acts on and nothing the screen does not show as
// written: no ESC, no control character, no bidi control and no character
// drawn as nothing (model.Unshowable), each such character, a sequence's ESC
// among them, being written as its escape. And it is done in one pass: Strip of its own
// output is the same. The seeds, checked in too as testdata/fuzz/FuzzStrip,
// hold an escape sequence and the characters of
// TestACharacterDrawnAsNothingInANameIsShownEscaped, so the fuzzer starts
// where the rule bites.
func FuzzStrip(f *testing.F) {
	for _, s := range []string{"", "café ✓ 日本語", "a\x1b]0;x\x07b\x1b[2Jc", "/p/bin/a\u202Etxt.md", "a\u2066b\u2069c\u061C", "a\u200B\u200C\u200Db\u2060\uFEFF\u180E", "a\u034Fb\u3164\uFE0F\U000E0100\u2028\t", "\x1b\u202E[2J", "\xff\xfe"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := term.Strip(s)
		for _, r := range got {
			if r == 0x1b || model.Unshowable(r) {
				t.Fatalf("Strip(%q) = %q keeps %U", s, got, r)
			}
		}
		if again := term.Strip(got); again != got {
			t.Fatalf("Strip(%q) = %q, and Strip of that = %q", s, got, again)
		}
	})
}

// TestThePaletteSeedReachesTheParser is the anti-vacuity check of
// FuzzParsePalette: the seed parses to its one role, one colour made a
// non-colour is refused naming it, and the seed cut by one byte is refused,
// so the colours are read and not set aside.
func TestThePaletteSeedReachesTheParser(t *testing.T) {
	t.Parallel()
	p, err := term.ParsePalette([]byte(paletteSeed))
	if err != nil || len(p.Text) != 1 || p.Text["good"].Dark != "#66dd66" {
		t.Fatalf("the seed parses to %+v, %v", p, err)
	}
	if _, err := term.ParsePalette([]byte(`{"grounds":{"light":"#ffffff","dark":"#000000"},"text":{"good":{"light":"#11661","dark":"#66dd66"}}}`)); err == nil {
		t.Error("a colour of five digits was taken")
	}
	if _, err := term.ParsePalette([]byte(paletteSeed[:len(paletteSeed)-1])); err == nil {
		t.Error("a cut token file was taken")
	}
}
