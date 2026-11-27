package model

import (
	"errors"
	"fmt"
	"unicode"
)

// ErrOtherProgram is a line to paste whose first word is a program other than
// gragas, for a shell that is not POSIX. PowerShell and cmd hand a program the
// words of its line as written only when it is a native program on the C
// runtime, never a batch file such as npm.cmd, and which of the two a name
// finds is the operating system's to know, not gragas's. So
// the root's line maker gives no such line there, and its caller says the
// step in words, each value as %q. It is one of ErrInWords.
var ErrOtherProgram = fmt.Errorf("a line that runs a program other than gragas is written only for a POSIX shell: %w", ErrInWords)

// ErrInWords is a step the root's line maker writes no line to paste for,
// though every word of it can be said: the program is not gragas in a shell
// that is not POSIX (ErrOtherProgram), the shell is one gragas knows no safe
// quoting for, or a word holds what some shell reads inside quotes. Its
// caller says the step in words, each value as %q, and the run's word is not
// changed by it. A value no line of any shell can carry, a
// newline or a NUL, is not one of these: it is refused.
var ErrInWords = errors.New("no line to paste is written for this step, so it is said in words")

// Unshowable says whether the screen does not show r as written: a control
// character (unicode.IsControl), a bidi control (unicode.Bidi_Control), which
// turns the text around it, or a character drawn as nothing. Those are
// Unicode's Default_Ignorable_Code_Point, which Go spells as the format
// characters (unicode.Cf), unicode.Other_Default_Ignorable_Code_Point (U+034F,
// the Hangul fillers U+115F, U+1160, U+3164, U+FFA0, U+17B4-17B5) and the
// variation selectors (unicode.Variation_Selector: U+180B-180F, U+FE00-FE0F,
// U+E0100-E01EF), together with the line and paragraph separators U+2028 and
// U+2029 (unicode.Zl, unicode.Zp). The terminal's Strip escapes a character in text the program did not
// write by it. Every rune it holds is also refused
// by shell.Quote, whose rule is wider and is the one that decides a word to
// paste.
//
// The rule fails to the safe side. A few format characters have a glyph
// (U+0600-0605, U+06DD, U+070F, U+0890-0891, U+08E2, U+110BD, U+110CD). Some
// characters of the set are spelling: U+FE0F after an emoji, U+200C and
// U+200D inside a word of Persian or a Devanagari conjunct, a Mongolian free
// variation selector, an ideographic variation selector, and the tag
// characters of a subdivision flag. Each of these is held unshowable too, so
// such a name is shown with an escape and said in words, never in a line to
// paste.
func Unshowable(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) || unicode.Is(unicode.Cf, r) ||
		unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) || unicode.Is(unicode.Variation_Selector, r) ||
		unicode.In(r, unicode.Zl, unicode.Zp)
}
