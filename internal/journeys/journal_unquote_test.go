package journeys

import "testing"

// TestTheMaskReadsAQuotedStatePathAsABareOne: a state path that
// the product quoted, as a Windows one is, reads as the bare word it is on a
// POSIX path once the mask has replaced it; nothing else loses its quotes.
// Risk: the restore goldens fail on a machine whose state path is quoted.
func TestTheMaskReadsAQuotedStatePathAsABareOne(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct{ in, want string }{
		"a quoted state path":          {"cp '$STATE/ROOT.copies/r1/ab12' a.txt", "cp $STATE/ROOT.copies/r1/ab12 a.txt"},
		"a bare one stays":             {"cp $STATE/ROOT.copies/r1/ab12 a.txt", "cp $STATE/ROOT.copies/r1/ab12 a.txt"},
		"a word with a space stays":    {"cp '$STATE/a b' a.txt", "cp '$STATE/a b' a.txt"},
		"a word with a quote's escape": {`cp '$STATE/it'\''s' a.txt`, `cp '$STATE/it'\''s' a.txt`},
		"quoted prose stays":           {"run 'gragas journal list' and 'x'", "run 'gragas journal list' and 'x'"},
	} {
		if got := unquoteMasked(c.in); got != c.want {
			t.Errorf("%s: unquoteMasked(%q) = %q, want %q", name, c.in, got, c.want)
		}
	}
}

// TestTheMaskReadsAStatePathInAJSONDocument: a Windows state
// path is written in a --json document with each backslash doubled, so the
// mask must find that spelling too, then read the quoted word as a bare one.
// Risk: json-journal-restore fails on Windows with the path left in the line.
func TestTheMaskReadsAStatePathInAJSONDocument(t *testing.T) {
	t.Parallel()
	c := tableOf(t, "mask-json-state.txt")[0]
	w := &journalWorld{state: c[0], root: "ROOTROOTROOTROOT"}
	in, want := c[1], c[2]
	if got := w.scrubbed(in); got != want {
		t.Errorf("scrubbed = %q, want %q", got, want)
	}
}
