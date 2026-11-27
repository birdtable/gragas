package term

import (
	"context"
	"fmt"
	"io"

	"github.com/birdtable/go-console/ui"
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/exit"
)

// Main runs a program that reads its own arguments and keeps its own exit
// codes under the kit's Main, which owns the process's signals: Ctrl+C and
// SIGTERM cancel the context run is given, on every system. It
// is for the programs that are no command tree of the kit's own, the dev
// tools; gragas itself runs its tree under Execute.
//
// run is the whole of the one command the kit's tree holds, named name. The
// kit reads no flag and no word of the line (DisableFlagParsing), and a
// command that returns a code gets that code back as it is (ui.OwnCode). Its
// session is held, so it prints no closing word, banner or document of its
// own: the tool's output is its own, as it always was. WithoutBuiltins leaves
// help, man and completion to the tool too, so the bare word help reaches run
// and the tool prints its own usage.
func Main(ctx context.Context, name string, stderr io.Writer, run func(ctx context.Context, args []string) int) int {
	table, err := exit.NewTable()
	if err != nil {
		if _, werr := fmt.Fprintln(stderr, name+": the table of endings is not whole, so nothing ran:", err); werr != nil {
			return exit.Failed // nowhere else to say it
		}
		return exit.Failed
	}
	own := ownRun{name: name, run: run}
	return ui.Main(ctx, func(*ui.Session) *cobra.Command { return own.command() }, gragas(), ui.WithoutVersionFlag(), ui.WithoutBuiltins(), ui.WithSession(ui.Endings(table, endKinds()), ui.WithMessages(messages()),
		ui.FlagNames(ui.Flags{}), ui.Held()))
}

// ownRun is the one command a dev tool is, named name, that hands its line to
// run. It holds the run apart from Main so the context the kit gives the
// command is read where the kit hands it over (cmd.Context()).
type ownRun struct {
	name string
	run  func(ctx context.Context, args []string) int
}

func (o ownRun) command() *cobra.Command {
	c := &cobra.Command{
		Use:                o.name,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		Args:               cobra.ArbitraryArgs,
		RunE:               o.runE,
	}
	c.CompletionOptions.DisableDefaultCmd = true
	return ui.OwnCode(c)
}

func (o ownRun) runE(cmd *cobra.Command, args []string) error {
	if code := o.run(cmd.Context(), args); code != 0 {
		return exitCode(code)
	}
	return nil
}

// exitCode is a run's exit code as the error the kit turns back into it.
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit %d", int(c)) }
func (c exitCode) ExitCode() int { return int(c) }
