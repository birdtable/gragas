package rulebook_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/rulebook"
)

// FuzzParseNames holds the names parser to its promises on any file: it never
// panics, an accepted file finds only text that is in the input, and a file
// with no patterns finds nothing.
func FuzzParseNames(f *testing.F) {
	for _, seed := range []struct{ file, text string }{
		{"# made-up names\n\nacme-corp\n\\bwidgetco\\b\n", "made by Acme-Corp"},
		{"(unclosed\n", "anything"},
		{"\n\n# only comments\n", "acme-corp"},
		{"a{2,3}\n\x00\n", "aaaa"},
	} {
		f.Add([]byte(seed.file), seed.text)
	}
	f.Fuzz(func(t *testing.T, file []byte, text string) {
		n, err := rulebook.ParseNames(bytes.NewReader(file))
		if err != nil {
			return
		}
		m := n.Find(text)
		if m != "" && !strings.Contains(text, m) {
			t.Fatalf("Find(%q) = %q, which is not in the text", text, m)
		}
		if m != "" && !hasPattern(file) {
			t.Fatalf("a file with no patterns found %q", m)
		}
	})
}

// hasPattern reports whether a names file has a line that is not blank or a comment.
func hasPattern(file []byte) bool {
	for line := range strings.SplitSeq(string(file), "\n") {
		if l := strings.TrimSpace(line); l != "" && !strings.HasPrefix(l, "#") {
			return true
		}
	}
	return false
}
