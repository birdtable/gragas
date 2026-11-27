package trust

import (
	"strings"

	shell "github.com/birdtable/go-console/shell"
	gotrust "github.com/birdtable/go-files/trust"
)

// fixLine is go-files's fix (gotrust.Fix.Commands) as the lines a
// person types to make it: each step's argv quoted for the shell the step is
// written for, one step a line, never joined with " && " (which Windows
// PowerShell 5.1 reads as an error). go-files writes the words, so a refusal
// reads as it does there, /L on every icacls step and the owner set by icacls. A
// fix no run of commands makes, the zero Fix among them, gives ok false, and
// fixLine is "": the caller says what it would with none, a mode beyond the nine
// bits, an owner not in the form of an account (go-files' Account), and a revoke from a macOS or
// NFSv4 list whose entries take more than a name to remove.
//
// An icacls step is quoted for shell.Windows, go-console's one answer that cmd
// and PowerShell both read as the same one command:
// no bare `;`, `&` or `|` splits it in the shell a person pasted it into. chmod
// and chown are quoted for a POSIX shell. A step go-console quotes for neither,
// a path holding a per-cent cmd expands or a character no shell carries as
// itself, is no line at all, and so is the whole fix: every line printed is one
// go-console quoted and so reads back as its step, and the caller says in words
// what to do in its place, as before (a step printed with its path's
// separators bare would run part of the path when pasted).
// go-files adds no privilege word, so an administrator's step takes a sudo word
// on a POSIX shell; on Windows it is run from an elevated prompt, which takes
// none. What the fix is decides the shell, not the system this runs on.
func fixLine(f gotrust.Fix) string {
	steps, ok := f.Commands()
	if !ok {
		return ""
	}
	lines := make([]string, 0, len(steps))
	for _, s := range steps {
		d := shell.POSIX
		if len(s.Argv) > 0 && s.Argv[0] == "icacls" {
			d = shell.Windows
		}
		line, err := shell.Quote(d, s.Argv)
		if err != nil {
			return ""
		}
		if s.Admin && d == shell.POSIX {
			line = "sudo " + line
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
