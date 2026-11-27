package contracttest

import (
	"strings"
	"testing"
)

// TestATableIsReadWithItsEscapes: a table's line reads \\, \n
// and \t as one backslash, a newline and a tab, keeps a backslash before
// anything else, and skips an empty line and a # comment; every table the
// suites read is embedded and holds a case. Planted: the escapes left as
// they are; a comment read as a case; a table embedded empty.
func TestATableIsReadWithItsEscapes(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		`one \\ two`:  "one \\ two",
		`one \n two`:  "one \n two",
		`one \t two`:  "one \t two",
		`one \q two`:  `one \q two`,
		`ends in \`:   `ends in \`,
		`one \\n two`: `one \n two`,
	} {
		if got := unescape(in); got != want {
			t.Errorf("unescape(%q) = %q, want %q", in, got, want)
		}
	}
	for _, name := range []string{"leaving", "read-outside", "sandbox-outside"} {
		cases := table(t, name)
		for _, c := range cases {
			if c == "" || strings.HasPrefix(c, "#") || strings.Contains(c, `\\`) {
				t.Errorf("table %s: case %q is a comment, empty, or holds an escape left unread", name, c)
			}
		}
	}
}
