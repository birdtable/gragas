package root

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagsMissingFrom lists the flags of c that its help text does not name: a
// flag a person can type and the help does not say (cli.commands.8). A hidden
// flag is the one allowed absence.
func flagsMissingFrom(c *cobra.Command, help string) []string {
	var out []string
	c.NonInheritedFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && !strings.Contains(help, "--"+f.Name) {
			out = append(out, f.Name)
		}
	})
	return out
}

// TestEveryFlagOfEveryCommandIsInItsHelp runs each command's help in process
// and holds it to the tree: a flag added to a command and left out of what the
// help prints fails here, and so does a planted one.
func TestEveryFlagOfEveryCommandIsInItsHelp(t *testing.T) {
	t.Parallel()
	tr := tree(t)
	checked := 0
	for _, c := range tr.Commands() {
		if c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		p, out := process(t, world(t), c.Name(), "--help")
		code, said := executeSaid(t.Context(), t, p)
		said += out.String()
		if code != 0 {
			t.Errorf("%s --help ended on %d:\n%s", c.Name(), code, said)
			continue
		}
		for _, f := range flagsMissingFrom(c, said) {
			t.Errorf("the help of %s does not name --%s", c.Name(), f)
		}
		c.NonInheritedFlags().VisitAll(func(*pflag.Flag) { checked++ })
	}
	if checked == 0 {
		t.Fatal("no flag was checked, so the test proves nothing")
	}
}

// A command with a flag its help does not name is told.
func TestAFlagMissingFromHelpFailsTheCheck(t *testing.T) {
	t.Parallel()
	c := &cobra.Command{Use: "x"}
	c.Flags().Bool("loud", false, "Say more")
	c.Flags().Bool("quiet", false, "Say less")
	if got := flagsMissingFrom(c, "  --loud  Say more"); len(got) != 1 || got[0] != "quiet" {
		t.Errorf("want quiet told missing, got %v", got)
	}
	c.Flags().MarkHidden("quiet")
	if got := flagsMissingFrom(c, "  --loud  Say more"); len(got) != 0 {
		t.Errorf("a hidden flag is no omission, got %v", got)
	}
}
