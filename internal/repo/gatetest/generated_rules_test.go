package gatetest

import (
	"slices"
	"testing"

	"github.com/birdtable/gragas/internal/core/link"
)

// TestGeneratedRulesCountsTheIDsOfTheGeneratedBlocksAlone:
// generatedRules decides what "held" means to the register test, which fails only on too few
// held rules, so this table holds it against counting too many. Planted failures: m1, the mark
// check taken out (the unmarked and line-2 rows name ids); m3, the comment ids of every line
// counted (the outside row names hand.made.9); and a block counted before its end comes (the
// unended row names open.block.4).
func TestGeneratedRulesCountsTheIDsOfTheGeneratedBlocksAlone(t *testing.T) {
	t.Parallel()
	mark := "# " + link.Mark()
	for _, c := range []struct {
		name string
		text string
		want []string
	}{
		{"a marked file names the ids of its block alone", mark + "\n# generated:lint:begin\n  wrap: true # gen.served.1\n# generated:lint:end\n", []string{"gen.served.1"}},
		{"the ids on lines outside the block are not counted", mark + "\nsettings: x # hand.made.9\n# generated:lint:begin\n  wrap: true # gen.served.1\n# generated:lint:end\nmore: y # hand.made.10\n", []string{"gen.served.1"}},
		{"an unmarked file names none", "# a project's lint file\n# generated:lint:begin\n  wrap: true # gen.served.1\n# generated:lint:end\n", nil},
		{"the mark on line 2 names none", "# a project's file\n" + mark + "\n# generated:lint:begin\n  wrap: true # gen.served.1\n# generated:lint:end\n", nil},
		{"a block whose end never comes names none", mark + "\n# generated:lint:begin\n  wrap: true # gen.served.1\n# generated:lint:end\n# generated:more:begin\n  open: true # open.block.4\n", []string{"gen.served.1"}},
		{"two blocks name the ids of both, in the file's order", mark + "\n# generated:a:begin\n  x: 1 # one.first.1, one.second.2\n# generated:a:end\nkept: 1 # hand.made.9\n# generated:b:begin\n  y: 2 (two.third.3)\n  z: 3 # two.third.3\n# generated:b:end\n", []string{"one.first.1", "one.second.2", "two.third.3"}},
		{"the end of another block does not end this one", mark + "\n# generated:a:begin\n  x: 1 # one.first.1\n# generated:b:end\n  y: 2 # one.second.2\n", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := generatedRules(c.text); !slices.Equal(got, c.want) {
				t.Errorf("generatedRules = %q, want %q", got, c.want)
			}
		})
	}
}
