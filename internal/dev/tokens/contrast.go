package tokens

import (
	"fmt"
	"slices"

	"github.com/birdtable/gragas/internal/wcag"
)

// The floors of the contrast check: text and status words and symbols reach
// TextFloor on every ground they name, and the parts of a control (its border,
// the focus ring, a state graphic) reach PartFloor (web.tokens.8,
// web.accessibility.15, spec 17).
const (
	TextFloor = 4.5
	PartFloor = 3.0
)

// Floor is the ratio a section's colours reach, and false for a section that
// is read by no pair: the grounds themselves.
func Floor(section string) (float64, bool) {
	switch section {
	case "text", "standing", "marks", "observations":
		return TextFloor, true
	case "parts":
		return PartFloor, true
	}
	return 0, false
}

// Low is each pair of the document below its floor in a mode, as a sentence,
// sorted: the role, the ground and the mode, the two colours and the ratio. The
// ratio is computed from the document by internal/wcag, the one
// WCAG ratio in the repository and the terminal's own. Nothing when every
// pair holds. A colour it cannot read is a low pair too, so a document that
// was not parsed is never read as passing.
func (d Document) Low() []string {
	var out []string
	for _, s := range d.colourSections() {
		floor, ok := Floor(s.name)
		if !ok {
			continue
		}
		for role, c := range s.roles {
			for _, g := range c.On {
				for _, m := range Modes() {
					fg, bg := c.In(m), d.Grounds[g].In(m)
					r, err := wcag.Contrast(fg, bg)
					if err != nil || r < floor {
						out = append(out, fmt.Sprintf("%s.%s on %s in %s: %s on %s is %.2f:1, below %.1f:1", s.name, role, g, m, fg, bg, r, floor))
					}
				}
			}
		}
	}
	slices.Sort(out)
	return out
}
