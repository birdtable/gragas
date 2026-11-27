package plugin

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// testCmd is gragas plugin test.
func testCmd(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use:   "test <bundle>",
		Short: "Hold a plugin bundle to the conformance checks",
		Long: `Reads the bundle at plugins/<name>/<version> and holds it to the
conformance checks of the plugin design, numbered C1 to C24 so a failure
names its check. This run holds checks C1 to C23: the bundle (the manifest,
its contract and needs, its name, version and digest, its grants), the
wire (each case's request), the evidence of each operation (recordings,
planted and clean outputs, refusals, rank pairs) and how the plugin
behaves. It runs the bundle's module in the sandbox, with the limits of
an installed plugin, without asking for approval: the module is under
test, and nothing it answers is run. None of the steps a plan answers is
run, a program it names is held to the grants and the path rules (C13)
and never started. A check that cannot run says why; none is faked. A
check for an operation the manifest does not provide is not asked, names
the operation, and counts toward done. A bundle that fails a check ends
on findings; one where none failed and a check that applies did not run
ends not ready. It reads the bundle and writes nothing.

  gragas plugin test <folder>
  gragas plugin test <folder> --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return s.Exit(runTest(c, s, args[0]))
		},
	}
}

func runTest(c *cobra.Command, s kit.Session, dir string) int {
	got, err := s.PluginTest(c.Context(), dir)
	if err != nil {
		return s.End(kit.Ending{Word: model.Failed, What: err.Error()})
	}
	count := map[string]int{}
	for _, k := range got.Checks {
		count[k.Status]++
	}
	what := fmt.Sprintf("%s: %d passed, %d failed, %d not run, %d not asked", got.Bundle, count[model.CheckPassed], count[model.CheckFailed], count[model.CheckNotRun], count[model.CheckNotAsked])
	word := got.Word()
	next := ""
	if word == model.NotReady {
		// What did not run is told in full in the document.
		next = "gragas plugin test " + dir + " --json"
	}
	if s.JSON() {
		s.Document(got)
		return s.End(kit.Ending{Word: word, What: what, Next: next})
	}
	t := model.Table{Columns: []model.Column{
		{Title: "check", Priority: 9}, {Title: "result", Priority: 9}, {Title: "what it holds, and what is wrong", Priority: 9},
	}}
	for _, k := range got.Checks {
		text := k.Name
		if len(k.What) > 0 {
			text += ": " + k.What[0]
			if len(k.What) > 1 {
				text += fmt.Sprintf(" (and %d more)", len(k.What)-1)
			}
		}
		t.Rows = append(t.Rows, []string{fmt.Sprintf("C%d", k.N), k.Status, text})
	}
	return s.End(kit.Ending{Word: word, What: what, Next: next, Lead: []model.Block{model.Heading{Text: "Checks"}, t}})
}
