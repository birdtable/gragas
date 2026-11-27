package term

import (
	"fmt"
	"os"

	"github.com/birdtable/go-console/shell"
	"github.com/birdtable/go-console/ui"

	"github.com/birdtable/gragas/internal/model"
)

// HereDialect is the dialect of the shell gragas was started from, as the kit
// finds it (ui.HereDialect): on Windows the nearest shell the kit knows among
// the parent processes, else PowerShell; shell.POSIX elsewhere. When that
// shell is one the kit writes no quoting for, it is 0 and an error that
// wraps ui.ErrNoDialect, whose text is the kit's: host/term takes no other
// dialect in its place and adds no shell's name, and the caller decides.
// It reads the system, so main calls it, and only for the
// shell setting's auto.
func HereDialect() (shell.Dialect, error) {
	d, err := ui.HereDialect()
	if err != nil {
		return 0, fmt.Errorf("the shell gragas was started from: %w", err)
	}
	return d, nil
}

// Measure is what the kit finds f to be, asked of it alone and under no
// environment: whether it is a terminal, as the system answers it for the
// descriptor (a pipe, a file and /dev/null are not), and its width in
// columns, 0 when it is not one. main hands it standard output; the cap on a
// line is the look's, and a COLUMNS variable is the caller's override, not
// read here.
func Measure(f *os.File) (terminal bool, width int) {
	on := ui.Detect(f, f, f, nil, ui.EnvRules{})
	if !on.Interactive {
		return false, 0
	}
	return true, on.Width
}

// asking is the kit's session over this terminal's streams, for a question,
// with gragas's endings and messages, which a menu needs at a terminal: a
// menu is offered only where In, Out and Err are all a terminal. Its error is
// NewTable's, which main checks before anything runs.
func (t *Terminal) asking() (*ui.Session, error) {
	return kitSession(t.on(t.Look(), t.opt.Out, t.opt.Err), ui.In(t.opt.In), ui.Out(t.opt.Out), ui.Err(t.opt.Err))
}

// CanAsk is whether a question could be answered here, as the kit answers it.
func (t *Terminal) CanAsk() bool {
	s, err := t.asking()
	return err == nil && s.CanAsk()
}

// Pick offers choices in the kit's menu and answers the Value of the one
// picked; picked is false off a terminal, with no choice and on backing out.
func (t *Terminal) Pick(title string, choices []model.Choice, flag string) (string, bool) {
	rows := make([]ui.Choice, len(choices))
	for i, c := range choices {
		rows[i] = ui.Choice{Label: c.Label, Blurb: c.Blurb, Value: c.Value, Gap: c.Gap}
	}
	s, err := t.asking()
	if err != nil {
		return "", false
	}
	return s.Pick(title, rows, flag)
}
