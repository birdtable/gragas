package journeys

import (
	"slices"
	"strings"
	"unicode"
)

// The words of a screen line, apart from its frame. Both pty_test.go
// (stepFinding) and the narrow journeys of pty_narrow_test.go read lines with
// them.

// token is a word of a line and where it starts, in runes, counted from one.
type token struct {
	text string
	col  int
}

// tokensOf is the words of a line and where each starts.
func tokensOf(line string) []token {
	var out []token
	start := -1
	rs := []rune(line + " ")
	for i, r := range rs {
		switch {
		case !unicode.IsSpace(r) && start < 0:
			start = i
		case unicode.IsSpace(r) && start >= 0:
			out = append(out, token{text: string(rs[start:i]), col: start + 1})
			start = -1
		}
	}
	return out
}

// unframed is the words of a line that are not frame.
func unframed(line string) []token {
	return slices.DeleteFunc(tokensOf(line), func(f token) bool {
		return !strings.ContainsFunc(f.text, func(r rune) bool { return !isFrame(r) })
	})
}

// isFrame is whether r is a box-drawing or block character.
func isFrame(r rune) bool { return r >= 0x2500 && r <= 0x259f }
