package root

// The questions of rule new, asked at a terminal one by one. Each names the
// flag that answers it in a script (cli.environment.14), and the same list
// is the check that a flag is missing off a terminal, so a question and its
// flag cannot drift apart.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/birdtable/go-console/shell"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/birdtable/gragas/internal/model"
)

// A prompted is one answer a command needs: the flag that gives it, the words
// asked at a terminal, and where the answer goes.
type prompted struct {
	flag, ask string
	to        *string
	// skip is true when the answer is not needed given the others.
	skip func() bool
}

// asked runs the questions of c in order. A flag given on the command line is
// its answer. A missing one is asked at a terminal, and the answer is set as
// its flag, so the line of the yes names it and guard staged can make the plan
// again from the line alone; off a terminal it ends the run as a usage
// error naming the flag. ok is false when the run has ended.
func (s *session) asked(ctx context.Context, c *cobra.Command, qs []prompted) (code int, ok bool) {
	for _, q := range qs {
		name := strings.TrimPrefix(q.flag, "--")
		if (q.skip != nil && q.skip()) || c.Flags().Changed(name) {
			continue
		}
		if !s.canAsk() {
			return s.end(ending{word: model.Usage, what: fmt.Sprintf("%s is needed: %s", q.flag, q.ask), next: helpLine(c)}), false
		}
		if c.Flags().Lookup(name) == nil {
			return s.die("the question %q names %s, which %s does not take", q.ask, q.flag, c.CommandPath()), false
		}
		answer, err := s.p.World.Terminal.Ask(ctx, q.prompt())
		if err != nil && !errors.Is(err, io.EOF) {
			return s.end(ending{word: model.Failed, what: "the answer could not be read: " + err.Error()}), false
		}
		answer = strings.TrimSpace(answer)
		if answer == "" {
			return s.end(ending{word: model.Declined, what: "no answer to: " + q.ask, next: helpLine(c)}), false
		}
		if err := c.Flags().Set(name, answer); err != nil {
			return s.end(ending{word: model.Usage, what: fmt.Sprintf("%s cannot be %q: %v", q.flag, answer, err), next: helpLine(c)}), false
		}
		*q.to = answer
	}
	return 0, true
}

// applies is the line that applies the command: the words typed so far with
// each given flag, then --apply. Each argument and each value is one shell
// word (pasteLine), so the line pastes as the command it names and guard
// staged splits it back into the same words. A value no line can carry is an
// error that names it, and there is no line.
func applies(d shell.Dialect, c *cobra.Command, args []string) (string, error) {
	argv, end := ruleArgv(c, args)
	return pasteLine(d, withApply(argv, end))
}

// ruleArgv is the words of applies, before --apply and any quoting, and the
// index where its flags end: that of the -- it writes, else len(argv). A bool flag the person set to false is replayed
// as --x=false, for a bare --x means true (gragas#1642). The arguments come
// after the flags and a -- where any starts with a dash, so none is replayed
// as a flag; with none that does, they stay before the flags as typed.
func ruleArgv(c *cobra.Command, args []string) ([]string, int) {
	argv := strings.Fields(c.CommandPath())
	dashed := slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "-") })
	if !dashed {
		argv = append(argv, args...)
	}
	c.Flags().Visit(func(f *pflag.Flag) {
		if f.Name == "apply" {
			return
		}
		if f.Value.Type() == "bool" {
			if f.Value.String() == "false" {
				argv = append(argv, "--"+f.Name+"=false")
			} else {
				argv = append(argv, "--"+f.Name)
			}
			return
		}
		for _, v := range flagValues(f) {
			argv = append(argv, "--"+f.Name, v)
		}
	})
	end := len(argv)
	if dashed {
		argv = append(argv, "--")
		argv = append(argv, args...)
	}
	return argv, end
}

// flagValues are the values a flag was given, one for a plain flag and each
// for a repeatable one.
func flagValues(f *pflag.Flag) []string {
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		return sv.GetSlice()
	}
	return []string{f.Value.String()}
}

// prompt is the question as it is asked, with its flag beside it
// (cli.environment.14).
func (q prompted) prompt() string { return q.ask + " (" + q.flag + " answers it in a script)" }
