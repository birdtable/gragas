package format

import (
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
)

// The hashed form of the names list (supply.secrets.4). A line of exactly 64
// hexadecimal digits is the SHA-256 of one name: its words folded to lower
// case and joined by one space, or, for a name of several words, the same
// words run together. It is the line gragas-dev names hash writes for the
// push scan, so one file serves that scan and this check, and it matches the
// scan's reading of a text word for word: a word is a run of letters, marks
// and digits, split where its case turns, and a name is a run of one to
// MaxNameWords words. Any other line of the file stays a regular expression.
// The list holds no name in the clear, and a finding never prints one: it
// says MaskedName where the name was.

// MaxNameWords is the most words a hashed name holds, and the longest run of
// words a text is hashed in.
const MaxNameWords = 4

// MaskedName stands in a finding for a name found by its hash.
const MaskedName = "<name>"

var hashLine = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// isHashLine is whether a trimmed line of a names file is a hashed name.
func isHashLine(s string) bool { return hashLine.MatchString(s) }

// span is one word of a text, folded to lower case.
type span struct{ word string }

// The patterns words splits text by: a run of letters, marks and digits;
// within it, a part, upper-case letters and then the rest; and a part's
// upper-case head, and the lower-case letter that may follow it.
var (
	wordRun   = regexp.MustCompile(`[\p{L}\p{M}\p{Nd}]+`)
	casePart  = regexp.MustCompile(`[\p{Lu}\p{Lt}]*[^\p{Lu}\p{Lt}]*`)
	upperHead = regexp.MustCompile(`^[\p{Lu}\p{Lt}]+`)
	lowerNext = regexp.MustCompile(`^\p{Ll}`)
)

// words is text's words, each folded to lower case, and, separately, each run
// that splits into more than one, folded whole, so a name written run
// together is found as one word.
func words(text string) (parts, wholes []span) {
	for _, run := range wordRun.FindAllStringIndex(text, -1) {
		seg := splitParts(text[run[0]:run[1]])
		parts = append(parts, seg...)
		if len(seg) > 1 {
			wholes = append(wholes, span{strings.ToLower(text[run[0]:run[1]])})
		}
	}
	return parts, wholes
}

// splitParts splits a run of letters, marks and digits where its case turns:
// before an upper-case letter that follows a lower-case one or a digit, and
// before the last of two or more upper-case letters that a lower-case one
// follows, so HTTPServer is HTTP and Server.
func splitParts(run string) []span {
	var out []span
	for _, m := range casePart.FindAllStringIndex(run, -1) {
		seg := run[m[0]:m[1]]
		if seg == "" {
			continue
		}
		head := upperHead.FindString(seg)
		if rs := []rune(head); len(rs) > 1 && lowerNext.MatchString(seg[len(head):]) {
			at := len(head) - len(string(rs[len(rs)-1]))
			out = append(out, span{strings.ToLower(seg[:at])}, span{strings.ToLower(seg[at:])})
			continue
		}
		out = append(out, span{strings.ToLower(seg)})
	}
	return out
}

// holdsHash is whether the list holds the hash of s.
func (n *Names) holdsHash(s string) bool {
	_, in := n.hashes[digest.Sum([]byte(s))]
	return in
}

// findHashed is whether text holds a name of the list by its hash: a run of
// one to MaxNameWords words, or a word written run together.
func (n *Names) findHashed(text string) bool {
	if len(n.hashes) == 0 {
		return false
	}
	ps, wholes := words(text)
	for i := range ps {
		var run strings.Builder
		for k := 0; k < MaxNameWords && i+k < len(ps); k++ {
			if k > 0 {
				run.WriteByte(' ')
			}
			run.WriteString(ps[i+k].word)
			if n.holdsHash(run.String()) {
				return true
			}
		}
	}
	for _, w := range wholes {
		if n.holdsHash(w.word) {
			return true
		}
	}
	return false
}

// addHash puts a hashed name in the list.
func (n *Names) addHash(line string) {
	b, err := hex.DecodeString(line)
	if err != nil || len(b) != digest.Size {
		return // isHashLine held it to 64 digits; nothing else gets here
	}
	if n.hashes == nil {
		n.hashes = map[[digest.Size]byte]struct{}{}
	}
	n.hashes[[digest.Size]byte(b)] = struct{}{}
}
