package root

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	legacycmd "github.com/birdtable/gragas/internal/cli/legacy"
	"github.com/birdtable/gragas/internal/config"
	"github.com/birdtable/gragas/internal/exit"
)

// writes is the annotation a command that can write carries, and what it
// says: the run that writes is the one given --apply. mayWrite reads it, so
// the list of writing commands is the commands' own (cli.commands.1).
const writes = legacycmd.Writes

func (s *session) newRoot(ctx context.Context) *cobra.Command {
	root := &cobra.Command{
		Use:   "gragas",
		Short: "The rulebook, as presets you import",
		Long: `Gragás is a rulebook plus presets: shared configs that
projects import into their own tool configs. This
program checks the rulebook and writes the files made
from it.

` + helpCommands(commands()) + `

Environment:
  ` + config.NamesFileVar + `  a private file of names public
                     text must not hold, one regular
                     expression a line; unset skips it
  ` + stateVarHelp() + `where gen --apply keeps its lock
                     and run record (gragas/ in it),
                     else the user's state folder
  CI                 set: no colour, banner or question
  NO_COLOR, TERM, COLUMNS, ACCESSIBLE  as everywhere

Exit codes: 0 done, 1 refused or failed, 2 usage,
3 busy, 4 not ready.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.HiddenDefaultCmd = true
	// Args left unset: cobra's default names an unknown command and the
	// nearest one, and the error handler says what to type.
	root.RunE = func(c *cobra.Command, _ []string) error {
		if !s.canAsk() {
			// Nobody can answer a menu: the help, and the usage ending.
			if err := c.Help(); err != nil {
				return err
			}
			s.dieSaying("gragas --help", "%s", "no command given, and nobody to ask which one\nsee `gragas --help`")
			return exitErr(exit.Usage)
		}
		argv, picked := s.menu()
		if !picked {
			return nil // backed out of the menu: an answer, and nothing to say
		}
		next := s.newRoot(ctx)
		next.SetArgs(argv)
		return next.ExecuteContext(ctx)
	}
	pf := root.PersistentFlags()
	pf.Bool("json", false, "Answer in one JSON document")
	pf.Bool("plain", false, "Print plainly: nothing moves")
	pf.Bool("no-color", false, "No colour (NO_COLOR)")
	pf.Bool("accessible", false, "Plain questions (ACCESSIBLE)")
	pf.String(strings.TrimPrefix(config.ConfigFlag, "--"), "", "The settings file ("+config.ConfigVar+")")
	root.PersistentPreRunE = s.preRun
	root.AddCommand(legacycmd.Gen(ctx, host{s}, s.legacyDeps()))
	root.AddCommand(s.initCmd(ctx), s.ruleCmd(ctx), s.termCmd(ctx))
	for _, b := range built() {
		c := b.New(kitSession{s})
		if b.Row.SARIF {
			c.Flags().Bool("sarif", false, "Answer in one SARIF document")
		}
		root.AddCommand(c)
	}
	return root
}

// stopHeard is the line a run says on standard error when a signal has
// cancelled it: the run ends at its next step, on failed.
const stopHeard = "stopping on a signal (%v): the run ends at its next step"

// preRun is the root's first hook. It ties the kit's stop to the commands':
// the kit cancels the context it hands the tree (c's), and the commands hold
// the one Execute made cancellable. Then it ends the run on a
// settings problem, and claims the record of a run that writes.
func (s *session) preRun(c *cobra.Command, _ []string) error {
	if s.stop != nil {
		// The signal reaches c's context through two goroutines of the kit,
		// so a run can read on past it. The run says it heard the signal
		// once the cancel has landed, and ends only after saying so, so a
		// reader of standard error knows from the line that the run is
		// stopping, never from a wait.
		s.heard = make(chan struct{})
		s.unhook = context.AfterFunc(c.Context(), func() {
			defer close(s.heard)
			// A context that ends with the command has Canceled itself as its
			// cause; a signal's cause names the signal and wraps Canceled. The
			// kit that listens for the signal tells the two apart by its own
			// name (Process.Stopped), and the line says the
			// cause as the kit wrote it.
			if s.p.Stopped != nil && s.p.Stopped(c.Context()) {
				s.diag("", fmt.Sprintf(stopHeard, context.Cause(c.Context())))
			}
			s.stop()
		})
	}
	if s.p.Problem != nil {
		return exitErr(s.endSettings(s.p.Problem))
	}
	if mayWrite(c) {
		if e := s.rec.claim(dirOf(c)); e != nil {
			return exitErr(s.end(*e))
		}
	}
	return nil
}

// dirOf is the folder c's -C names, its default "." included, and "" for a
// command with no -C: the root a run that writes writes in, which the
// recorder's lock holds the state folder apart from.
func dirOf(c *cobra.Command) string {
	if f := c.Flags().Lookup("dir"); f != nil {
		return f.Value.String()
	}
	return ""
}

// mayWrite is a command that writes on this run: one that carries the
// writes annotation and was given --apply.
func mayWrite(c *cobra.Command) bool {
	if _, ok := c.Annotations[writes]; !ok {
		return false
	}
	if apply, err := c.Flags().GetBool("apply"); err != nil || !apply {
		return false
	}
	if c.Flags().Lookup("check") == nil {
		return true
	}
	check, err := c.Flags().GetBool("check")
	return err == nil && !check
}

// menu offers the commands that fit the working folder at a terminal and
// returns the argv picked. It collects and never applies: the command it
// returns runs with its own form.
func (s *session) menu() ([]string, bool) {
	picked, ok := s.p.World.Terminal.Pick(menuTitle, menuChoices(commands(), placeOf(s.p.World.Tree)), menuHint)
	if !ok || picked == "" {
		return nil, false
	}
	return []string{picked}, true
}

// canAsk is whether the run may show the menu: a person at a terminal, not
// under --json and not in CI.
func (s *session) canAsk() bool { return !s.json && !s.p.Config.CI && s.p.World.Terminal.CanAsk() }

// withRun adds the run's id to what a run that is recorded says when it
// fails, so a person can quote it.
func (s *session) withRun(what string) string {
	if id := s.rec.id(); id != "" {
		return what + " (run " + id + ")"
	}
	return what
}
