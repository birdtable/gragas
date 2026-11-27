package root

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// TestUnderSARIFARulebookThatCannotBeReadLeavesStdoutEmpty: with no rulebook
// to read there is no SARIF document, so stdout holds nothing, and the
// closing word a person reads is drawn on standard error, with no banner.
// Planted failures: the ending, or the banner, drawn on stdout under --sarif.
func TestUnderSARIFARulebookThatCannotBeReadLeavesStdoutEmpty(t *testing.T) {
	t.Parallel()
	p, out := process(t, world(t), "validate", "--sarif")
	p, sc := attach(t, p)
	sc.out.Seed(".", map[string]string{"README.md": "no rulebook here\n"})
	code := Execute(t.Context(), p)
	if code != exit.Refused || out.Len() != 0 || len(sc.out.Shown) != 0 || len(sc.out.Said) != 0 || len(sc.out.Banners) != 0 {
		t.Fatalf("exit %d, stdout %q, shown %d, said %q, banners %q; want refused and stdout empty", code, out, len(sc.out.Shown), sc.out.Said, sc.out.Banners)
	}
	if len(sc.diag.Shown) != 1 || !strings.HasPrefix(said(sc.diag.Shown[0]), "\n"+string(model.Refused)+" ") {
		t.Errorf("standard error showed %d screens, want the refused ending: %v", len(sc.diag.Shown), sc.diag.Shown)
	}
}

// TestJSONAndSARIFTogetherIsAUsageErrorWithStdoutEmpty: each asks for the
// run's one document on stdout, so the run writes neither and says why on
// standard error. Planted failure: the check of the two flags removed.
func TestJSONAndSARIFTogetherIsAUsageErrorWithStdoutEmpty(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"validate", "--json", "--sarif"}, {"validate", "--sarif=true", "--json=true"}} {
		p, out := process(t, world(t), args...)
		code, sc := execute(t.Context(), t, p)
		want := []string{"error: --json and --sarif cannot be given together: each is the run's one document on stdout", "       see `gragas validate --help`"}
		if code != exit.Usage || out.Len() != 0 || len(sc.out.Shown) != 0 || !slices.Equal(sc.diag.Said, want) {
			t.Errorf("%q: exit %d, stdout %q, diagnostics %q; want the usage code, stdout empty and %q", args, code, out, sc.diag.Said, want)
		}
	}
}

// TestOnlyACommandThatListsFindingsTakesSARIF: --sarif is on validate and on
// no command whose row does not list findings, and a command without it
// refuses it as an unknown flag with stdout empty. Planted failure: the flag
// registered on every command.
func TestOnlyACommandThatListsFindingsTakesSARIF(t *testing.T) {
	t.Parallel()
	takes := map[string]bool{}
	for _, b := range built() {
		takes[b.Row.Name] = b.Row.SARIF
	}
	s := &session{rec: &recorder{}}
	var on []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Flags().Lookup("sarif") != nil {
			on = append(on, c.Name())
			if !takes[c.Name()] {
				t.Errorf("%s takes --sarif, and its row does not list findings", c.CommandPath())
			}
		}
		for _, k := range c.Commands() {
			walk(k)
		}
	}
	walk(s.newRoot(t.Context()))
	if !slices.Contains(on, "validate") {
		t.Errorf("--sarif is on %v, want validate among them", on)
	}
	p, out := process(t, world(t), "version", "--sarif")
	code, sc := execute(t.Context(), t, p)
	if code != exit.Usage || out.Len() != 0 || len(sc.diag.Said) == 0 || !strings.Contains(sc.diag.Said[0], "unknown flag: --sarif") {
		t.Errorf("version --sarif: exit %d, stdout %q, diagnostics %q; want an unknown flag", code, out, sc.diag.Said)
	}
}

// TestSARIFIsSeenBeforeAnyDashDash: like --json, --sarif after a -- is an
// argument, not the flag.
func TestSARIFIsSeenBeforeAnyDashDash(t *testing.T) {
	t.Parallel()
	for args, want := range map[string]bool{"validate --sarif": true, "validate --sarif=true": true, "validate -- --sarif": false, "validate --sarif=false": false, "validate": false} {
		if got := wantsSARIF(strings.Fields(args)); got != want {
			t.Errorf("wantsSARIF(%s) = %v, want %v", args, got, want)
		}
	}
}

// TestUnderSARIFALineAPersonReadsGoesToStandardError: stdout holds the SARIF
// document alone. Planted failure: say writing on stdout under --sarif.
func TestUnderSARIFALineAPersonReadsGoesToStandardError(t *testing.T) {
	t.Parallel()
	p, out := process(t, world(t))
	p, sc := attach(t, p)
	s := &session{p: p, rec: &recorder{}, sarif: true}
	kitSession{s}.Say("a line")
	if out.Len() != 0 || len(sc.out.Said) != 0 || !slices.Equal(sc.diag.Said, []string{"a line"}) {
		t.Errorf("stdout %q, said %q, diagnostics %q; want the line on standard error alone", out, sc.out.Said, sc.diag.Said)
	}
}
