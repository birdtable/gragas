package plugin

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/birdtable/gragas/internal/model"
)

// Shown is a plugin's text as a screen may print it: a finding's message, key
// and url, the why of a run that did not happen, a preset's notes, an
// error's message and path. The text is the tool's own words, or whatever a
// plugin chose to answer, and the wire schema holds its length alone. It is
// here, beside the types that carry that text, so that each is escaped once
// and by one spelling: CallError.Error writes the plugin's code, path and
// message by it, and every caller of that error gets them escaped.
//
// Every character the screen does not show as written is written as its
// escape, by the one rule the terminal escapes by (model.Unshowable): the
// controls, ESC and the one-character CSI U+009B among them, the format
// characters and the bidirectional controls, the line and paragraph
// separators, and the characters drawn as nothing. That set holds every
// character the manifest schema's "shown" class refuses, and more. A byte
// that is not UTF-8 is written as \x and its two digits. A backslash is
// written as two, so that no text a plugin sends can spell an escape Shown
// did not write: what is printed says which bytes were sent, and two texts
// that differ print differently. Letters of every script are left alone.
//
// The escapes are Go's (\n, \x1b, \u200b, \U000e0100), as the terminal's
// are. A valid character below U+0080 and a byte that is not UTF-8 never
// share a spelling: the first takes \x00 to \x7f, the second \x80 to \xff.
func Shown(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			const hex = "0123456789abcdef"
			b.WriteString(`\x`)
			b.WriteByte(hex[s[i]>>4])
			b.WriteByte(hex[s[i]&0xf])
		case r == '\\':
			b.WriteString(`\\`)
		case model.Unshowable(r):
			q := strconv.QuoteRuneToASCII(r)
			b.WriteString(q[1 : len(q)-1])
		default:
			b.WriteRune(r)
		}
		i += n
	}
	return b.String()
}
