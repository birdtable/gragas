package term

import (
	"context"
	"io"

	"github.com/birdtable/go-console/ui"
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// Execute runs the command tree once and draws it as the kit does: help, a
// command line it did not understand, and the flags, in this terminal's look.
// The kit draws help from the cobra command itself, so the adapter is handed
// the tree and not a struct. onError is told of an error the tree
// returned, before Execute returns it, and writes it as the program's own
// words.
//
// The banner, the version flag, the closing word and the five flags the kit
// can give (--plain, --accessible, --yes, --json, --no-color) are the
// program's, not the kit's: gragas defines its own, so the kit is given no
// flag names and registers none, and its session is held, so it prints no
// ending of its own and no JSON document. gragas prints its own closing word
// where it always has, on stdout and on stderr under --json (cli.output.7):
// the kit's End prints every ending but done on stderr and has no choice of
// stream.
func (t *Terminal) Execute(ctx context.Context, tree *cobra.Command, onError func(io.Writer, error)) error {
	l := t.Look()
	out, errOut := tree.OutOrStdout(), tree.ErrOrStderr() // the kit wraps the tree's own in Execute
	run, err := kitSession(t.on(l, out, errOut), ui.Out(out), ui.Err(errOut), ui.Held())
	if err == nil {
		err = ui.Execute(ctx, tree, run, gragas(), ui.WithoutVersionFlag())
	}
	if err != nil {
		onError(errOut, err)
	}
	return err
}

// Stopped is whether os.Interrupt or SIGTERM, the signals Execute's kit
// listens for, ended ctx, the context of a command Execute runs or one made
// from it, as the kit tells it by its own name (ui.Stopped):
// false while the run goes on, and when ctx ended another way.
func Stopped(ctx context.Context) bool {
	_, ok := ui.Stopped(ctx)
	return ok
}

// on is what the kit finds in, out and errOut to be under this terminal's
// environment, in this terminal's look. The environment's locale is never
// read (EnvRules names no locale variable): the terminal draws UTF-8, as the
// look always has, so the goldens do not move with a LANG the tests drop
// (gragas names no locale variable).
func (t *Terminal) on(l Look, out, errOut io.Writer) ui.Terminal {
	on := ui.Detect(t.opt.In, out, errOut, t.opt.Env, ui.EnvRules{})
	on.Colour, on.ErrColour = l.Rich, l.Rich
	on.Width, on.Accessible, on.Dark, on.Unicode = l.Width, l.Accessible, true, true
	return on
}

// kitSession is a session of the kit that runs a command or asks a question
// over on: gragas's endings and messages, no flag of the kit's, the line
// capped at maxLine, and the fixed palette, with opts beside them. Its error
// is NewTable's, which main has already checked before anything runs.
func kitSession(on ui.Terminal, opts ...ui.SessionOption) (*ui.Session, error) {
	table, err := exit.NewTable()
	if err != nil {
		return nil, err
	}
	return ui.New(append([]ui.SessionOption{ui.On(on), ui.MaxWidth(maxLine), ui.WithTheme(ui.Contrast()),
		ui.Endings(table, endKinds()), ui.WithMessages(messages()), ui.FlagNames(ui.Flags{})}, opts...)...), nil
}

// endKinds names, in gragas's table, the kinds the kit ends a run on: each is
// a closing word of the closed set, as the table names its kinds by word. Each
// word is painted in the role Look.Closing paints it in.
func endKinds() ui.EndKinds {
	return ui.EndKinds{
		Failed: string(model.Failed), Usage: string(model.Usage), Declined: string(model.Declined),
		Busy: string(model.Busy), NotReady: string(model.NotReady), Refused: string(model.Refused),
		Roles: map[string]ui.EndRole{
			string(model.Done):     ui.EndGood,
			string(model.Findings): ui.EndBad,
			string(model.Refused):  ui.EndBad,
			string(model.Failed):   ui.EndBad,
			string(model.Usage):    ui.EndAttn,
			string(model.Busy):     ui.EndAttn,
			string(model.NotReady): ui.EndAttn,
			string(model.Declined): ui.EndAttn,
		},
	}
}
