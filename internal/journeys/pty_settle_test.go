package journeys

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// TestSettleWaitsForItsConditionAndGivesUpAtItsDeadline: the step's wait is a condition and a
// deadline, never a sleep. A condition that never holds gives up at the deadline, answering false and what the
// screen lacked, so the step fails with it and the last screen; a condition that holds after a few looks answers
// the screen it judged; no condition is as it was; and a screen without the text waited for lacks that text.
// Risk: a stall that hangs, or a step that goes on with a screen no condition held of.
func TestSettleWaitsForItsConditionAndGivesUpAtItsDeadline(t *testing.T) {
	t.Parallel()
	still := frames{shown: []string{"hello world"}, at: []time.Duration{0}}
	pageOf := func(screen string) string { return page(t, "", screen) }
	// within is the deadline of the waits that must give up; the waits that must hold have the journey's.
	within := 3 * ptyQuiet

	got, lacking, ok := settle(still, pageOf, within, "hello", func(string) string { return "row 7 is still cut" })
	if ok || got != "" || lacking != "row 7 is still cut" {
		t.Errorf("a condition that never holds: settle = %q, %q, %v, want false and what the screen lacked", got, lacking, ok)
	}

	looks := 0
	got, lacking, ok = settle(still, pageOf, ptyWait, "hello", func(string) string {
		if looks++; looks < 2 {
			return "not yet"
		}
		return ""
	})
	if !ok || got != "hello world\n" || lacking != "" || looks != 2 {
		t.Errorf("a condition that holds on the second look: settle = %q, %q, %v after %d looks", got, lacking, ok, looks)
	}

	got, _, ok = settle(still, pageOf, ptyWait, "hello", nil)
	if !ok || got != "hello world\n" {
		t.Errorf("no condition: settle = %q, %v, want the screen", got, ok)
	}

	_, lacking, ok = settle(still, pageOf, within, "goodbye", func(string) string { return "" })
	if ok || !strings.Contains(lacking, `"goodbye" is not on the screen`) {
		t.Errorf("a screen without the text: settle = %q, %v, want false saying the text is not on it", lacking, ok)
	}

	// A screen that never holds still for ptyQuiet is never judged: a frame every 50 ms of the waiter's looks, for
	// longer than within lasts.
	var busy frames
	for i := range 40 {
		busy.shown = append(busy.shown, "hello world")
		busy.at = append(busy.at, time.Duration(i)*50*time.Millisecond)
	}
	judged := false
	_, _, ok = settle(busy, pageOf, within, "hello", func(string) string { judged = true; return "" })
	if ok || judged {
		t.Errorf("a screen that never held still: settle = %v, judged %v, want false and never judged", ok, judged)
	}
}

// TestTheGoldenSettledConditionHoldsOfNoScreenButTheGoldensOwn: at 80 columns a picture waits for
// the screen's part to be the golden's part, line for line. The golden's own part holds; a screen with a row cut
// short, a row too many, a row gone, or each row cut at 60 columns (the first frame a menu draws) does not, and
// names the first line that differs; a title the golden has no part of is no condition. Risk: the 80-column runs
// taking a picture of a screen that is not yet the one the golden says.
func TestTheGoldenSettledConditionHoldsOfNoScreenButTheGoldensOwn(t *testing.T) {
	t.Parallel()
	var c ptyCase
	for _, k := range ptyCases() {
		if k.name == "menu-rulebook" {
			c = k
		}
	}
	if c.name == "" {
		t.Fatal("no menu-rulebook case")
	}
	_, _, parts := partsOf(goldenOf(t, "pty-menu-rulebook-80"))
	var want []string
	for _, p := range parts {
		if p.title == "--- menu" {
			want = p.lines
		}
	}
	if len(want) < 8 {
		t.Fatalf("the golden's menu part has %d lines", len(want))
	}
	settled := goldenSettled(t, c)
	join := func(lines []string) string { return strings.Join(lines, "\n") + "\n" }
	cut := slices.Clone(want)
	last := len(cut) - 1
	for cut[last] == "" || strings.TrimSpace(cut[last]) == "┃" {
		last--
	}
	cut[last] = cut[last][:len(cut[last])-1]
	narrow := make([]string, len(want))
	for i, l := range want {
		if utf8.RuneCountInString(l) > 60 {
			l = string([]rune(l)[:60])
		}
		narrow[i] = l
	}
	for name, tc := range map[string]struct {
		screen string
		holds  bool
	}{
		"the golden's own part": {join(want), true},
		"a row cut short":       {join(cut), false},
		"a row too many":        {join(append(slices.Clone(want), "a planted row")), false},
		"a row gone":            {join(want[:len(want)-1]), false},
		"every row cut at 60":   {join(narrow), false},
		"nothing drawn yet":     {"", false},
	} {
		why := settled("menu", tc.screen)
		if (why == "") != tc.holds {
			t.Errorf("%s: settled = %q, holds %v, want holds %v", name, why, why == "", tc.holds)
		}
		if !tc.holds && !strings.Contains(why, "line ") {
			t.Errorf("%s: %q does not name the first line that differs", name, why)
		}
	}
	if why := settled("a title the golden has no part of", join(cut)); why != "" {
		t.Errorf("a title with no part in the golden is no condition, got %q", why)
	}
}

// TestUpdatingReadsTheFlagAsUitestDoes: -update in any of its spellings is an update run, where there is no
// golden to wait for yet; no other word is, and the arguments after "--" are not the binary's.
func TestUpdatingReadsTheFlagAsUitestDoes(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		args []string
		want bool
	}{
		"-update":          {[]string{"-test.v", "-update"}, true},
		"--update":         {[]string{"--update"}, true},
		"-update=true":     {[]string{"-update=true"}, true},
		"-update=1":        {[]string{"-update=1"}, true},
		"-update=false":    {[]string{"-update=false"}, false},
		"none":             {[]string{"-test.v", "-test.run=x"}, false},
		"a word beginning": {[]string{"-updated", "--no-update-notifier", "update"}, false},
		"after --":         {[]string{"--", "-update"}, false},
	} {
		if got := updating(c.args); got != c.want {
			t.Errorf("%s: updating(%q) = %v, want %v", name, c.args, got, c.want)
		}
	}
}
