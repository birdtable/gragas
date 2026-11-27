package root

import (
	"slices"
	"testing"

	"github.com/birdtable/go-console/shell"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// replayed is a command with a default-true bool --keep, a default-false bool
// --loud and a plain --name, as a command of the tree may one day have.
func replayed() *cobra.Command {
	c := &cobra.Command{Use: "replayed", Run: func(*cobra.Command, []string) {}}
	(&cobra.Command{Use: "gragas"}).AddCommand(c)
	c.Flags().Bool("keep", true, "")
	c.Flags().Bool("loud", false, "")
	c.Flags().Bool("apply", false, "")
	c.Flags().String("name", "", "")
	return c
}

// parsedBack is the command parsed from the words of line, after the program
// and the command's own name.
func parsedBack(t *testing.T, line string) (*cobra.Command, []string) {
	t.Helper()
	words, err := splitLine(line)
	if err != nil || len(words) < 2 {
		t.Fatalf("split %q: %q, %v", line, words, err)
	}
	c := replayed()
	if err := c.ParseFlags(words[2:]); err != nil {
		t.Fatalf("parse %q: %v", line, err)
	}
	return c, c.Flags().Args()
}

// TestAnApplyLineReplaysABoolFlagSetToFalseAsFalse (gragas#1642): a bool flag
// the caller set to false is replayed as --x=false, and the line, parsed back,
// sets it to false; a bool set to true stays a bare --x, and a flag not set is
// not replayed. The plant is the bare --keep, which parses back as true.
func TestAnApplyLineReplaysABoolFlagSetToFalseAsFalse(t *testing.T) {
	t.Parallel()
	c := replayed()
	for _, set := range [][2]string{{"keep", "false"}, {"loud", "true"}} {
		if err := c.Flags().Set(set[0], set[1]); err != nil {
			t.Fatal(err)
		}
	}
	for name, line := range map[string]func() (string, error){
		"rule line":    func() (string, error) { return applies(shell.POSIX, c, nil) },
		"command line": func() (string, error) { return commandLine(shell.POSIX, c, "gragas replayed", nil, nil) },
	} {
		got, err := line()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		back, _ := parsedBack(t, got)
		if keep, _ := back.Flags().GetBool("keep"); keep {
			t.Errorf("%s: %s parses back with keep true", name, got)
		}
		if loud, _ := back.Flags().GetBool("loud"); !loud {
			t.Errorf("%s: %s parses back with loud false", name, got)
		}
		if back.Flags().Changed("name") {
			t.Errorf("%s: %s replays a flag that was not set", name, got)
		}
	}
}

// TestAnApplyLineWritesAnArgumentThatStartsWithADashAfterTheDashes
// (gragas#1642, a review nit on PR 1641): `gragas rule promote -- -x`
// replays -x after --, before no flag and with --apply ahead of the --, so the
// line parses back with -x as the argument. An argument with no dash stays
// where it was typed.
func TestAnApplyLineWritesAnArgumentThatStartsWithADashAfterTheDashes(t *testing.T) {
	t.Parallel()
	c := replayed()
	if err := c.Flags().Set("name", "n"); err != nil {
		t.Fatal(err)
	}
	line, err := applies(shell.POSIX, c, []string{"-x", "go.errors.2"})
	if err != nil {
		t.Fatal(err)
	}
	words, err := splitLine(line)
	want := []string{"gragas", "replayed", "--name", "n", "--apply", "--", "-x", "go.errors.2"}
	if err != nil || !slices.Equal(words, want) {
		t.Fatalf("words of %s = %q, %v; want %q", line, words, err, want)
	}
	plain, err := applies(shell.POSIX, c, []string{"go.errors.2"})
	if err != nil {
		t.Fatal(err)
	}
	if words, _ := splitLine(plain); !slices.Equal(words, []string{"gragas", "replayed", "go.errors.2", "--name", "n", "--apply"}) {
		t.Errorf("an argument with no dash moved: %q", words)
	}
}

// TestNoBoolFlagOfTheTreeDefaultsToTrueUnlessItIsReplayedAsFalse (gragas#1642):
// every bool flag of every command defaults to false, so a replay of one set to
// false never changes the value the person meant; replays write --x=false for
// the first that does not, and this row then needs no change.
func TestNoBoolFlagOfTheTreeDefaultsToTrueUnlessItIsReplayedAsFalse(t *testing.T) {
	t.Parallel()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() == "bool" && f.DefValue != "false" {
				probe := &cobra.Command{Use: c.Name()}
				probe.Flags().AddFlag(f)
				_ = probe.Flags().Set(f.Name, "false")
				if line, err := applies(shell.POSIX, probe, nil); err != nil || !slices.Contains(mustSplit(t, line), "--"+f.Name+"=false") {
					t.Errorf("%s --%s defaults to %s and is replayed as %q, %v", c.CommandPath(), f.Name, f.DefValue, line, err)
				}
			}
		})
		for _, k := range c.Commands() {
			walk(k)
		}
	}
	walk(tree(t))
}

func mustSplit(t *testing.T, line string) []string {
	t.Helper()
	words, err := splitLine(line)
	if err != nil {
		t.Fatal(err)
	}
	return words
}

// TestAnApplyLineAppliesWhenAFlagsValueIsTheDashes (gragas#1642, a review
// change on PR 2402): a flag whose value is -- (--name --) is replayed as
// --name --, and --apply goes after that value, not before it: the line parsed
// back has name set to -- and apply true, with and without a -- of its own
// that ends the flags. The plant is withApply that looks for the first -- in
// argv, which finds the value, puts --apply there and gives name=--apply and
// no apply.
func TestAnApplyLineAppliesWhenAFlagsValueIsTheDashes(t *testing.T) {
	t.Parallel()
	check := func(what, line string, wantArgs []string) {
		t.Helper()
		c, args := parsedBack(t, line)
		name, _ := c.Flags().GetString("name")
		applied, _ := c.Flags().GetBool("apply")
		if name != "--" || !applied || !slices.Equal(args, wantArgs) {
			t.Errorf("%s: %s parses back as name %q, apply %v, args %q; want name --, apply, args %q", what, line, name, applied, args, wantArgs)
		}
	}
	c := replayed()
	if err := c.Flags().Set("name", "--"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"-x", "go.errors.2"}} {
		line, err := applies(shell.POSIX, c, args)
		if err != nil {
			t.Fatal(err)
		}
		check("rule path", line, args)
	}
	s := &session{p: Process{Dialect: shell.POSIX}}
	cmd := replayed()
	if err := cmd.Flags().Set("name", "--"); err != nil {
		t.Fatal(err)
	}
	_, line, err := s.planLines(commandArgv(cmd, "gragas replayed", nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	check("plan path", line, nil)
}
