package model

import (
	"strconv"
	"strings"
)

// Count is n as a person reads it, 165,440 and not 165440 (cli.look.3). The
// shared kit's own helper draws the same, and host/term's tests hold the two
// to one another, so the command tree, which may not import the kit's look,
// says a number the way the terminal does.
func Count(n int) string {
	if n < 0 {
		return "-" + Count(-n)
	}
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Plural is n and its noun, 1 rule, 2 rules, 3 people (cli.look.3). A noun
// ending in y after a consonant ends in ies, and person is people; the
// others take an s.
func Plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return Count(n) + " " + pluralOf(noun)
}

func pluralOf(noun string) string {
	if rest, ok := strings.CutSuffix(noun, "person"); ok && (rest == "" || strings.HasSuffix(rest, " ")) {
		return rest + "people"
	}
	if n := len(noun); n > 1 && noun[n-1] == 'y' && !strings.ContainsRune("aeiou", rune(noun[n-2])) {
		return noun[:n-1] + "ies"
	}
	return noun + "s"
}
