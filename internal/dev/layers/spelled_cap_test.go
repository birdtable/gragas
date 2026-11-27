package layers_test

import (
	"slices"
	"strings"
	"testing"
)

// TestAnExclusionIsSpelledOutToMaxSpelledStringsAndNoMore (boundary: maxSpelled):
// an exclusion text that spells exactly maxSpelled rule ids
// before its colon, 8 digits by 8 digits, is confined to other rules' ids and
// passes; one that spells one more, the same with a ninth id beside them, can
// no longer be spelled out, is not known to be confined, and is refused as no
// ban. A cap one more or one fewer fails one of the two checks. (A character
// class too wide for the cap is held by the same limit; a class wide enough to
// cross it has no digit-only spelling, so no valid rule id comes of it to tell
// it from a refusal.)
func TestAnExclusionIsSpelledOutToMaxSpelledStringsAndNoMore(t *testing.T) {
	t.Parallel()
	reg := shared(t, registry)
	lint := joined(t, reg)
	end := "  exclusions:\n    rules:\n"
	if !strings.Contains(lint, end) {
		t.Fatal("the rendered lint file has no exclusion rules")
	}
	held := func(text string) []string {
		rule := "      - path: ^h/\n        linters: [forbidigo]\n        text: \"" + text + "\"\n"
		return reg.HeldBy([]byte(strings.Replace(lint, end, end+rule, 1)))
	}
	const at = `go\\.errors\\.[0-7][0-7]:`
	if got := held(at); len(got) != 0 {
		t.Errorf("an exclusion spelling 8 x 8 = 64 ids: HeldBy = %q, want no finding", got)
	}
	const over = `go\\.errors\\.(?:[0-7][0-7]|99):`
	if got := held(over); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "the exclusion of ^h/ is no ban") }) {
		t.Errorf("an exclusion spelling 64 + 1 = 65 ids: HeldBy = %q, want it refused", got)
	}
}
