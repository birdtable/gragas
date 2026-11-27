package root

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// writesTreeProblems is where a command of the tree and its annotation part
// (class 5): a command that takes --apply without the writes
// annotation, and one that carries it and takes no --apply. It also reports the
// count of commands read, so a walk that reads nothing is not a pass.
func writesTreeProblems(tree *cobra.Command) (problems []string, read int) {
	var walk func(c *cobra.Command, at []string)
	walk = func(c *cobra.Command, at []string) {
		if len(at) > 0 && c.Runnable() {
			read++
			_, marked := c.Annotations[writes]
			takes := c.LocalFlags().Lookup("apply") != nil
			name := strings.Join(at, " ")
			switch {
			case takes && !marked:
				problems = append(problems, name+" takes --apply and carries no writes annotation")
			case marked && !takes:
				problems = append(problems, name+" carries the writes annotation and takes no --apply")
			}
		}
		for _, k := range c.Commands() {
			walk(k, append(slices.Clone(at), k.Name()))
		}
	}
	walk(tree, nil)
	return problems, read
}

// TestWritesTreeHoldsApplyAndTheAnnotationTogether holds the whole
// registry: --apply and the writes annotation go together, both ways. The
// planted tree is red each way, its agreeing command green, and an empty tree
// reads no command, which is not a pass.
func TestWritesTreeHoldsApplyAndTheAnnotationTogether(t *testing.T) {
	t.Parallel()
	run := func(c *cobra.Command) *cobra.Command { c.Run = func(*cobra.Command, []string) {}; return c }
	apply := func(c *cobra.Command) *cobra.Command { c.Flags().Bool("apply", false, ""); return c }
	mark := func(c *cobra.Command) *cobra.Command {
		c.Annotations = map[string]string{writes: "with --apply"}
		return c
	}
	plant := &cobra.Command{Use: "gragas"}
	plant.AddCommand(
		mark(apply(run(&cobra.Command{Use: "good"}))),
		apply(run(&cobra.Command{Use: "unmarked"})),
		mark(run(&cobra.Command{Use: "noapply"})),
		run(&cobra.Command{Use: "reads"}),
	)
	got, read := writesTreeProblems(plant)
	want := []string{
		"unmarked takes --apply and carries no writes annotation",
		"noapply carries the writes annotation and takes no --apply",
	}
	if read != 4 || len(got) != 2 || !slices.Contains(got, want[0]) || !slices.Contains(got, want[1]) {
		t.Errorf("the planted tree gave %v over %d commands, want %v over 4", got, read, want)
	}
	if _, read := writesTreeProblems(&cobra.Command{Use: "gragas"}); read != 0 {
		t.Errorf("an empty tree read %d commands", read)
	}

	problems, read := writesTreeProblems(tree(t))
	if read == 0 {
		t.Fatal("the registry tree has no runnable command; the check read nothing")
	}
	for _, p := range problems {
		t.Error(p)
	}
}
