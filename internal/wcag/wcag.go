// Package wcag holds the one WCAG 2.1 contrast ratio of the repository: the
// relative luminance of a #rrggbb colour and the ratio of two. The terminal's
// palette check (host/term) and the token generator's contrast check
// (dev/tokens) both read it, so the number a person is told in the terminal is
// the number the browser's tokens were held to (cli.look.2, web.tokens.8).
// It is a leaf: it reaches no host and links no kit.
package wcag

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// ErrColour is a colour that is not #rrggbb.
var ErrColour = errors.New("not #rrggbb")

// Luminance is the WCAG relative luminance of a #rrggbb colour, 0 for black
// and 1 for white.
func Luminance(hex string) (float64, error) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, fmt.Errorf("%q is %w", hex, ErrColour)
	}
	var l float64
	for i, w := range []float64{0.2126, 0.7152, 0.0722} {
		v, err := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		if err != nil {
			return 0, fmt.Errorf("%q is %w", hex, ErrColour)
		}
		c := float64(v) / 255
		if c <= 0.03928 {
			c /= 12.92
		} else {
			c = math.Pow((c+0.055)/1.055, 2.4)
		}
		l += w * c
	}
	return l, nil
}

// Contrast is the WCAG ratio of two #rrggbb colours, always at least 1. A
// colour that does not parse is an error.
func Contrast(a, b string) (float64, error) {
	x, err := Luminance(a)
	if err != nil {
		return 0, err
	}
	y, err := Luminance(b)
	if err != nil {
		return 0, err
	}
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05), nil
}
