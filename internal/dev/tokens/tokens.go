// Package tokens reads the explorer's token document and writes what is
// generated from it: the CSS custom properties, the Tailwind 4 theme and the
// terminal's palette file, so a standing reads the same in the browser and in
// the terminal (web.tokens.19, web.tokens.10). It also holds the contrast
// check over the document (web.tokens.8, web.accessibility.15).
//
// The document is written by hand and read here, never the other way round: a
// colour has a value in light and in dark or the document is refused
// (web.tokens.11, web.tokens.18). The package reads and renders bytes; the
// generator's main is the one place that touches the disk.
package tokens

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
)

// File is the token document and the three files generated from it.
const (
	File        = "explorer/packages/tokens/tokens.json"
	CSSFile     = "explorer/packages/tokens/src/tokens.gen.css"
	ThemeFile   = "explorer/packages/tokens/src/theme.gen.css"
	PaletteFile = "internal/host/term/palette.json"
)

// Version names this generator's output format. It is in the header of each
// generated file, so a change of the format is a change of the files.
const Version = "1"

// Prefix is the name every custom property of the document starts with.
const Prefix = "gr"

// ErrDocument is a token document the generator cannot take.
var ErrDocument = errors.New("the token document cannot be taken")

// Mode is light or dark.
type Mode string

// The two modes every colour has a value in.
const (
	Light Mode = "light"
	Dark  Mode = "dark"
)

// Modes are the modes in the order they are written.
func Modes() []Mode { return []Mode{Light, Dark} }

// Colour is a colour role: one #rrggbb value for each mode, and the grounds it
// may sit on, which the contrast check reads (web.tokens.9). A ground has
// none of its own.
type Colour struct {
	Light string   `json:"light"`
	Dark  string   `json:"dark"`
	On    []string `json:"on,omitempty"`
}

// In is the colour's value in a mode.
func (c Colour) In(m Mode) string {
	if m == Dark {
		return c.Dark
	}
	return c.Light
}

// Shadow is a box shadow for each mode.
type Shadow struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

// In is the shadow's value in a mode.
func (s Shadow) In(m Mode) string {
	if m == Dark {
		return s.Dark
	}
	return s.Light
}

// Fonts are the two font stacks.
type Fonts struct {
	Sans string `json:"sans"`
	Mono string `json:"mono"`
}

// Document is the token document.
type Document struct {
	Grounds      map[string]Colour `json:"grounds"`
	Text         map[string]Colour `json:"text"`
	Standing     map[string]Colour `json:"standing"`
	Marks        map[string]Colour `json:"marks"`
	Observations map[string]Colour `json:"observations"`
	Parts        map[string]Colour `json:"parts"`
	Spacing      map[string]string `json:"spacing"`
	Radius       map[string]string `json:"radius"`
	Shadow       map[string]Shadow `json:"shadow"`
	Breakpoints  map[string]string `json:"breakpoints"`
	Fonts        Fonts             `json:"fonts"`
}

// The names of the three breakpoints, in the order of their width.
func breakpointNames() []string { return []string{"compact", "medium", "wide"} }

// The standing roles of spec 17.2, which the document names.
func standingNames() []string {
	return []string{"amber", "blue", "green", "grey", "slate", "violet"}
}

var (
	hexColour = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	// A length is a number and a unit, or zero: what a scale step may be.
	length = regexp.MustCompile(`^(0|[0-9]+(\.[0-9]+)?(rem|px))$`)
	name   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// A shadow is an offset, a blur and a colour: the document's only colour
	// inside a longer value.
	fontStack   = regexp.MustCompile(`^[A-Za-z0-9 ",-]+$`)
	shadowValue = regexp.MustCompile(`^[0-9]+(px)? [0-9]+px [0-9]+px #[0-9a-f]{6}([0-9a-f]{2})?$`)
)

// Parse reads the document from r. A field it does not know, a colour that is
// not lower-case #rrggbb, a colour with no value in one mode, a role on a
// ground the document does not have, a breakpoint missing and a name that is
// no word are each an error that names where, so a hand edit that leaves one
// out fails here and nowhere later.
func Parse(r io.Reader) (Document, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var d Document
	if err := dec.Decode(&d); err != nil {
		return Document{}, fmt.Errorf("%w: %w", ErrDocument, err)
	}
	var rest json.RawMessage
	if err := dec.Decode(&rest); !errors.Is(err, io.EOF) {
		return Document{}, fmt.Errorf("%w: more than one document", ErrDocument)
	}
	if err := d.validate(); err != nil {
		return Document{}, fmt.Errorf("%w: %w", ErrDocument, err)
	}
	return d, nil
}

// ParseBytes is Parse of b.
func ParseBytes(b []byte) (Document, error) { return Parse(bytes.NewReader(b)) }

func (d Document) validate() error {
	var errs []error
	if _, ok := d.Grounds["page"]; !ok {
		errs = append(errs, errors.New("grounds.page: the ground the terminal draws on is missing"))
	}
	for _, s := range d.colourSections() {
		for _, n := range sortedKeys(s.roles) {
			errs = append(errs, d.checkColour(s.name+"."+n, n, s.roles[n], s.name != "grounds"))
		}
	}
	errs = append(errs, d.checkStanding(), d.checkScales(), d.checkShadowsAndFonts())
	return errors.Join(errs...)
}

// checkStanding holds the standing roles to the six of spec 17.2.
func (d Document) checkStanding() error {
	var errs []error
	for _, want := range standingNames() {
		if _, ok := d.Standing[want]; !ok {
			errs = append(errs, fmt.Errorf("standing.%s: the standing role is missing", want))
		}
	}
	if len(d.Standing) != len(standingNames()) {
		errs = append(errs, fmt.Errorf("standing: %d roles, want the six of spec 17.2", len(d.Standing)))
	}
	return errors.Join(errs...)
}

// checkScales holds the spacing, radius and breakpoint scales to words and
// lengths, and the breakpoints to the three named.
func (d Document) checkScales() error {
	var errs []error
	for _, s := range []struct {
		name  string
		scale map[string]string
	}{{"spacing", d.Spacing}, {"radius", d.Radius}, {"breakpoints", d.Breakpoints}} {
		if len(s.scale) == 0 {
			errs = append(errs, fmt.Errorf("%s: none", s.name))
		}
		for _, n := range sortedKeys(s.scale) {
			if !name.MatchString(n) {
				errs = append(errs, fmt.Errorf("%s.%s: the name is no word", s.name, n))
			}
			if !length.MatchString(s.scale[n]) {
				errs = append(errs, fmt.Errorf("%s.%s: %q is no length", s.name, n, s.scale[n]))
			}
		}
	}
	if got := sortedKeys(d.Breakpoints); !slices.Equal(got, sortedCopy(breakpointNames())) {
		errs = append(errs, fmt.Errorf("breakpoints: %v, want the three named %v", got, breakpointNames()))
	}
	return errors.Join(errs...)
}

// checkShadowsAndFonts holds each shadow to its shape in both modes and each
// font stack to quoted names and words.
func (d Document) checkShadowsAndFonts() error {
	var errs []error
	for _, n := range sortedKeys(d.Shadow) {
		for _, m := range Modes() {
			if v := d.Shadow[n].In(m); !shadowValue.MatchString(v) {
				errs = append(errs, fmt.Errorf("shadow.%s: %s value %q is not a shadow or is missing", n, m, v))
			}
		}
	}
	if len(d.Shadow) == 0 {
		errs = append(errs, errors.New("shadow: none"))
	}
	for n, stack := range map[string]string{"sans": d.Fonts.Sans, "mono": d.Fonts.Mono} {
		if !fontStack.MatchString(stack) {
			errs = append(errs, fmt.Errorf("fonts.%s: %q is not a font stack of quoted names and words", n, stack))
		}
	}
	return errors.Join(errs...)
}

// checkColour holds one colour to its shape. A role has a value in each mode
// and at least one ground it sits on, each of which the document has.
func (d Document) checkColour(where, n string, c Colour, needsGround bool) error {
	var errs []error
	if !name.MatchString(n) {
		errs = append(errs, fmt.Errorf("%s: the name is no word", where))
	}
	for _, m := range Modes() {
		switch v := c.In(m); {
		case v == "":
			errs = append(errs, fmt.Errorf("%s: no value in %s", where, m))
		case !hexColour.MatchString(v):
			errs = append(errs, fmt.Errorf("%s: %s value %q is not lower-case #rrggbb", where, m, v))
		}
	}
	if needsGround && len(c.On) == 0 {
		errs = append(errs, fmt.Errorf("%s: names no ground it may sit on", where))
	}
	for _, g := range c.On {
		if _, ok := d.Grounds[g]; !ok {
			errs = append(errs, fmt.Errorf("%s: sits on %q, which is no ground", where, g))
		}
	}
	return errors.Join(errs...)
}

// section is a named group of colour roles.
type section struct {
	name  string
	roles map[string]Colour
}

// colourSections are the groups of colour roles in the order they are written.
func (d Document) colourSections() []section {
	return []section{
		{"grounds", d.Grounds}, {"text", d.Text}, {"standing", d.Standing},
		{"marks", d.Marks}, {"observations", d.Observations}, {"parts", d.Parts},
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func sortedCopy(s []string) []string {
	c := slices.Clone(s)
	slices.Sort(c)
	return c
}
