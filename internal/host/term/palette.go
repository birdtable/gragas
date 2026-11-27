package term

import (
	_ "embed" // the palette file is committed beside the code
	"encoding/json"
	"fmt"
	"sort"

	"github.com/birdtable/gragas/internal/wcag"
)

// The palette is the one file the terminal's text colours come from
// (cli.look.2). It is generated from the explorer's token document by
// gragas-dev tokens (M8-04), so its shape is the token document's: a ground for each of light and dark, and a colour for each role
// on each ground. Every text colour must clear MinContrast on its ground.

//go:embed palette.json
var paletteFile []byte

// MinContrast is the WCAG 2.1 ratio body text must reach on its ground.
const MinContrast = 4.5

// Ground is a colour for the light ground and one for the dark.
type Ground struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

// Palette is the token file: the two grounds, and each text role's colours,
// as #rrggbb.
type Palette struct {
	Grounds Ground            `json:"grounds"`
	Text    map[string]Ground `json:"text"`
}

// LoadPalette parses the committed palette file. A file that is not the
// shape of Palette, or names a colour that is not #rrggbb, is an error.
func LoadPalette() (Palette, error) { return ParsePalette(paletteFile) }

// ParsePalette parses b as a Palette.
func ParsePalette(b []byte) (Palette, error) {
	var p Palette
	if err := json.Unmarshal(b, &p); err != nil {
		return Palette{}, fmt.Errorf("palette: %w", err)
	}
	if len(p.Text) == 0 {
		return Palette{}, fmt.Errorf("palette: no text colours")
	}
	all := []string{p.Grounds.Light, p.Grounds.Dark}
	for _, g := range p.Text {
		all = append(all, g.Light, g.Dark)
	}
	for _, c := range all {
		if _, err := luminance(c); err != nil {
			return Palette{}, err
		}
	}
	return p, nil
}

// luminance is the WCAG relative luminance of a #rrggbb colour, from the one
// ratio of internal/wcag, which the token generator holds the web's colours to.
func luminance(hex string) (float64, error) {
	l, err := wcag.Luminance(hex)
	if err != nil {
		return 0, fmt.Errorf("palette: %w", err)
	}
	return l, nil
}

// Contrast is the WCAG ratio of two #rrggbb colours, always at least 1. A
// colour that does not parse is an error.
func Contrast(a, b string) (float64, error) { return wcag.Contrast(a, b) }

// Unreadable is each text colour of p that is below MinContrast on its
// ground, as a sentence, sorted; nothing when every one is readable.
func Unreadable(p Palette) []string {
	var out []string
	for role, g := range p.Text {
		for _, c := range []struct{ ground, fg, bg string }{
			{"light", g.Light, p.Grounds.Light}, {"dark", g.Dark, p.Grounds.Dark},
		} {
			r, err := Contrast(c.fg, c.bg)
			if err != nil || r < MinContrast {
				out = append(out, fmt.Sprintf("%s on the %s ground: %s on %s is %.2f:1, below %.1f:1", role, c.ground, c.fg, c.bg, r, MinContrast))
			}
		}
	}
	sort.Strings(out)
	return out
}
