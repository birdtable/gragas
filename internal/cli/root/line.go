package root

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/go-console/shell"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/birdtable/gragas/internal/model"
)

// commandLine is the line a plan command prints to make the same plan again,
// and the apply line it ends on: base, then every flag the person set on cmd,
// the whole argv quoted at once by pasteLine. It is not a chosen subset of the
// flags, so no flag that shapes the change is dropped. skip names flags base
// already holds. The root's flags (--json) and --apply are not the change's:
// the caller adds --apply. fill holds the flags a command answered itself, by
// asking or by what git says, which stand in for the same flag when it was
// set. A value no line can carry is an error that names it.
func commandLine(d shell.Dialect, cmd *cobra.Command, base string, skip []string, fill map[string][]string) (string, error) {
	return pasteLine(d, commandArgv(cmd, base, skip, fill))
}

// setFlags is the flags the person set on cmd, by name, with their values: a
// bool has none, and off holds the bools set to false, replayed as --x=false,
// for a bare --x means true (gragas#1642). Inherited flags, --apply and skip
// are left out.
func setFlags(cmd *cobra.Command, skip []string) (set map[string][]string, off map[string]bool) {
	set, off = map[string][]string{}, map[string]bool{}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if f.Name == "apply" || slices.Contains(skip, f.Name) || cmd.InheritedFlags().Lookup(f.Name) != nil {
			return
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			set[f.Name] = sv.GetSlice()
			return
		}
		if f.Value.Type() != "bool" {
			set[f.Name] = []string{f.Value.String()}
			return
		}
		set[f.Name] = nil
		off[f.Name] = f.Value.String() == "false"
	})
	return set, off
}

// commandArgv is the words of commandLine, before any quoting.
func commandArgv(cmd *cobra.Command, base string, skip []string, fill map[string][]string) []string {
	set, off := setFlags(cmd, skip)
	maps.Copy(set, fill)
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	slices.Sort(names)
	argv := strings.Fields(base)
	for _, n := range names {
		if _, filled := fill[n]; len(set[n]) == 0 && off[n] && !filled {
			argv = append(argv, "--"+n+"=false")
		} else if len(set[n]) == 0 {
			argv = append(argv, "--"+n)
		}
		for _, v := range set[n] {
			argv = append(argv, "--"+n, v)
		}
	}
	return argv
}

// withApply is argv with --apply added at end, the index where its flags end:
// len(argv) when no -- ends them, else the index of that --. The caller says
// it, since a -- found by searching argv can be a flag's value (--name --), and
// --apply put before it would be that flag's value; a copy, argv is not changed.
func withApply(argv []string, end int) []string {
	return slices.Insert(slices.Clone(argv), end, applyFlag)
}

// planLines is argv, a gragas command without --apply, as the two lines a
// plan is made with. record is the spelling the plan, its digest, the journal
// and a rule's history hold: always POSIX, whatever the run's dialect, so the
// record is data that does not move with the shell. A plan made in PowerShell
// is the same plan checked from Git Bash, and guard staged splits a history
// line back into its words by POSIX's rules alone (splitLine). applies is
// argv with --apply as the run prints it to type, in the run's dialect.
// A value either line cannot carry is an error.
func (s *session) planLines(argv []string) (record, applies string, err error) {
	return s.planLinesAt(argv, len(argv))
}

// planLinesAt is planLines for an argv whose flags end at end (withApply): the
// index of the -- that ends them, as ruleArgv says it.
func (s *session) planLinesAt(argv []string, end int) (record, applies string, err error) {
	if record, err = recordLine(argv); err != nil {
		return "", "", err
	}
	if applies, err = s.line(withApply(argv, end)); err != nil {
		return "", "", err
	}
	return record, applies, nil
}

// pasteLine is argv as the one line a person pastes into a shell of dialect
// d, made by one shell.Quote call over the whole argv, so the first-word rule
// of go-console reads only the program's name (A5, A6). It is the
// one line maker of the command tree: the session hands its dialect to it,
// and contract and the old commands reach it through the session.
// When a word is a value no line of d can carry (a
// NUL, a carriage return or a newline; in Cmd a % or a !), it is an error
// that names that word, and the caller prints no line at all: never a line
// quoted some other way. So is a word the screen would not show as written,
// a tab, ESC, another control, a format character such as U+200B or a space
// other than U+0020: shell.Quote refuses each (shell.ErrControl),
// and its rule is the only one that decides. In PowerShell and cmd, an argv whose first word is
// not gragas gives no line, model.ErrOtherProgram, and the caller says the
// step in words: gragas cannot know there that the program reads its words
// as written. A line that does start with gragas is written
// there because the release's gragas is a native executable, gragas.exe on
// windows, which reads its command line as written and is no batch file or
// script; internal's TestTheReleaseBuildsGragasAsANativeProgram holds
// .goreleaser.yaml to it. In PowerShell, a word Windows PowerShell hands
// a program wrong whatever the quoting (shell.ErrPowerShellArg) is refused by
// its index in argv. A dialect this build does not know gives no
// line for any argv, model.ErrInWords, so a value go-console adds later never
// falls through to the quoting of another shell.
func pasteLine(d shell.Dialect, argv []string) (string, error) {
	switch d {
	case shell.POSIX:
	case shell.PowerShell, shell.Cmd:
		if len(argv) == 0 || argv[0] != "gragas" {
			return "", model.ErrOtherProgram
		}
	default:
		return "", noLine{fmt.Sprintf("this build of gragas knows no quoting for shell %d", int(d))}
	}
	return quoted(d, argv)
}

// noLine is a model.ErrInWords that carries the reason it gives no line, so
// the session says the reason without reading an error's text (go.errors.8).
type noLine struct{ why string }

func (e noLine) Error() string { return model.ErrInWords.Error() + ": " + e.why }
func (e noLine) Unwrap() error { return model.ErrInWords }

// recordLine is argv as a plan, its journal and a rule's history keep it:
// written for POSIX in every shell, as data that does not move with the shell
// the run was made in (planLines). It is never printed to paste, so only the
// values no line can carry refuse it, as they refuse every line.
func recordLine(argv []string) (string, error) { return quoted(shell.POSIX, argv) }

// quoted is argv quoted for d by one shell.Quote call, or the error that
// names the word no line of d can carry (pasteLine).
func quoted(d shell.Dialect, argv []string) (string, error) {
	line, err := shell.Quote(d, argv)
	if err == nil {
		return line, nil
	}
	for i, w := range argv {
		_, bad := shell.Quote(d, []string{"x", w})
		switch {
		case errors.Is(bad, shell.ErrPowerShellArg):
			// Windows PowerShell hands a program another
			// argv than any quoting of this word says. The kit's text names
			// the index of the probe, so the index here is argv's own.
			return "", fmt.Errorf("the argument at index %d can't be passed safely to PowerShell: %q: %w", i, w, shell.ErrPowerShellArg)
		case errors.Is(bad, shell.ErrControl):
			// A tab, or a character the screen draws as
			// nothing, which no line to paste carries in any shell. The kit's
			// text names the probe's index, so the index here is argv's own
			// and the word is shown escaped.
			return "", fmt.Errorf("the argument at index %d holds a character no line to paste can carry: %q: %w", i, w, shell.ErrControl)
		case bad != nil:
			return "", fmt.Errorf("%q cannot be written in a line to paste, so none is printed: %w", w, bad)
		}
	}
	return "", fmt.Errorf("no line to paste is printed: %w", err)
}

// dialect is the shell the run's lines to paste are written for: the one
// main handed, POSIX when it handed none.
func (s *session) dialect() shell.Dialect {
	if s.p.Dialect == 0 {
		return shell.POSIX
	}
	return s.p.Dialect
}

// line is argv as the one line to paste in the run's dialect (pasteLine).
// When the run's shell is one go-console knows no safe quoting for
// (Process.NoQuoting), no argv gives a line. A gragas command that gives no
// line by model.ErrInWords is said in words in the line's place
// (inWords), so the run's word is not changed by it, and the reason is said
// once on standard error; any other program's step is the caller's to say.
func (s *session) line(argv []string) (string, error) {
	var err error
	if s.p.NoQuoting != nil {
		err = noLine{noQuotingWhy(s.p.NoQuoting)}
	} else {
		var line string
		if line, err = pasteLine(s.dialect(), argv); err == nil {
			return line, nil
		}
	}
	if !errors.Is(err, model.ErrInWords) {
		return "", err
	}
	// PowerShell and cmd give rm or cp no line by design
	// (model.ErrOtherProgram), and the caller's words say so; any other cause
	// is a noLine, said once
	if nl := (noLine{}); errors.As(err, &nl) && !s.saidWhy {
		s.saidWhy = true
		s.warn("%s", nl.why)
	}
	if len(argv) < 2 || argv[0] != "gragas" {
		return "", err
	}
	return inWords(argv), nil
}

// noQuotingWhy is the reason a run whose shell has no safe quoting gives no
// line (Process.NoQuoting). The kit's text names the shell, a name that comes
// from the system, so it is shown only as %q, and not at all when it holds a
// character the screen would not show as written (shownAs): never as
// raw bytes on the person's terminal.
func noQuotingWhy(why error) string {
	return "your shell has no safe quoting gragas can use (" + shownAs(why.Error(), "its name holds a character the screen does not show as written") + "); set GRAGAS_SHELL or the shell key to posix, powershell or cmd if you use one of those"
}

// shownAs is text for a person to read, as %q, or hidden in its place when
// it holds a character the screen would not show as written. It names a value
// in prose and is no line to paste, but go-console's rule for what a screen
// does not show as itself is reached only through shell.Quote, which refuses
// such a character in an argument (shell.ErrControl), so the text is asked of
// it as the one word after a plain one: a refusal for any reason, a NUL or a
// mark that begins the text included, hides it, which is the safe side. No
// exported predicate of go-console says it alone.
func shownAs(text, hidden string) string {
	if _, err := shell.Quote(shell.POSIX, []string{"x", text}); err != nil {
		return hidden
	}
	return strconv.Quote(text)
}

// inWords is a gragas command said in words, each word after gragas as %q,
// for a run that writes it no line to paste (session.line).
func inWords(argv []string) string {
	words := make([]string, 0, len(argv)-1)
	for _, w := range argv[1:] {
		words = append(words, strconv.Quote(w))
	}
	return "run gragas with the words " + strings.Join(words, ", ")
}

// helpLine is the line to type when no paste line can be printed: the help of
// c, which holds no value the person gave, so a refusing word still carries a
// line (cli.errors.2) and nothing of the refused value is quoted some other way.
func helpLine(c *cobra.Command) string {
	return "gragas help " + strings.TrimPrefix(c.CommandPath(), "gragas ")
}
