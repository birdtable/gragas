package root

// The root's side of the plan seam (S43, A9; architecture 5, step 6b of):
// the form itself is internal/cli/run, which holds no cobra. Here the root
// says whether a command takes --apply and was given it, and is the Host the
// form draws through.

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/run"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// applyFlag is the one yes (A42): the flag that applies a plan, and the
// answer to the form's question when nobody is there to give it.
const applyFlag = run.ApplyFlag

// A planForm is the one question a plan command asks at a terminal, with the
// flag that answers it in a script (cli.environment.14). Build it with
// newForm.
type planForm = run.Form

// newForm is the form of c that asks question and is answered by flag. A
// flag c does not take is refused: a form never names an answer its command
// lacks.
func newForm(c *cobra.Command, question, flag string) (planForm, error) {
	name, _ := strings.CutPrefix(flag, "--")
	return run.NewForm(c.Flags().Lookup(name) != nil, c.CommandPath(), question, flag)
}

// changer is what the form runs: core/api's Change.
type changer = run.Changer

// A planRun is one plan command's run of the form.
type planRun struct {
	// cmd is the command that asks; it takes --apply.
	cmd    *cobra.Command
	change changer
	// next is the command line with --apply, the line a declined run ends on.
	next string
	// ask is whether a person can answer: s.canAsk(), so not under --json,
	// in CI or off a terminal.
	ask bool
}

// plan runs pr's change through the form of internal/cli/run and returns the
// exit code.
func (s *session) plan(ctx context.Context, pr planRun) int {
	if s.capture != nil {
		c, ok := pr.change.(*coreapi.Change)
		if !ok {
			return s.die("a plan made again is not the core's change")
		}
		s.capture.change = c
		return exit.Done
	}
	applying, err := pr.cmd.Flags().GetBool(strings.TrimPrefix(applyFlag, "--"))
	return run.Run(ctx, planHost{s}, run.Plan{
		Path: pr.cmd.CommandPath(), Takes: pr.cmd.Flags().Lookup(strings.TrimPrefix(applyFlag, "--")) != nil,
		Applying: err == nil && applying, Change: pr.change, Next: pr.next, Ask: pr.ask,
	})
}

// planHost is the session as the form's Host.
type planHost struct{ s *session }

func (h planHost) Say(line string)        { h.s.say(line) }
func (h planHost) Diag(label, msg string) { h.s.diag(label, msg) }
func (h planHost) Die(msg string) int     { return h.s.die("%s", msg) }
func (h planHost) End(e run.Ending, lead ...model.Block) int {
	return h.s.end(ending{word: e.Word, what: e.What, next: e.Next}, lead...)
}
func (h planHost) Ask(ctx context.Context, prompt string) (string, error) {
	return h.s.p.World.Terminal.Ask(ctx, prompt)
}
func (h planHost) SetChanges(lines []string) { h.s.changes = lines }
func (h planHost) AddChanges(lines []string) { h.s.changes = append(h.s.changes, lines...) }
func (h planHost) MarkApplied()              { h.s.applied = true }
