package root

// What a command line cobra did not understand looks like: the error, the
// nearest command or flag, and the line to type, on stderr, with the usage
// code (cli.commands.10).

import (
	"errors"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// errorHandler writes an error the tree returned. A command's own exit code
// is left unprinted: the command printed its ending.
func (s *session) errorHandler(tree *cobra.Command, args []string) func(io.Writer, error) {
	return func(_ io.Writer, err error) {
		if _, ok := errors.AsType[exitCode](err); ok {
			return
		}
		msg, next := usageMessage(tree, args, err)
		s.dieSaying(next, "%s", msg)
	}
}

// usageMessage is err said for a person, with the line to type next in
// backticks, and that line.
func usageMessage(tree *cobra.Command, args []string, err error) (msg, next string) {
	msg = strings.TrimSpace(strings.SplitN(err.Error(), "\n", 2)[0])
	c, _, ferr := tree.Find(args)
	switch {
	case strings.HasPrefix(msg, "unknown command"):
		if len(args) > 0 {
			if near := tree.SuggestionsFor(args[0]); len(near) > 0 {
				return meant(msg, "gragas "+near[0])
			}
		}
		return seeLine(msg, "gragas --help")
	case ferr == nil && strings.HasPrefix(msg, "unknown flag: --"):
		bad := strings.TrimPrefix(msg, "unknown flag: --")
		if near := nearestFlag(c, bad); near != "" {
			return meant(msg, c.CommandPath()+" --"+near)
		}
	}
	if ferr == nil {
		return seeLine(msg, c.CommandPath()+" --help")
	}
	return seeLine(msg, "gragas --help")
}

// meant is msg with the line it nearly names, and that line.
func meant(msg, line string) (string, string) {
	return msg + "\ndid you mean `" + line + "`?", line
}

// seeLine is msg with the line to see, and that line.
func seeLine(msg, line string) (string, string) {
	return msg + "\nsee `" + line + "`", line
}

// nearestFlag is the flag of c closest to bad, if it is close enough to be a typo.
func nearestFlag(c *cobra.Command, bad string) string {
	var names []string
	c.Flags().VisitAll(func(f *pflag.Flag) { names = append(names, f.Name) })
	c.InheritedFlags().VisitAll(func(f *pflag.Flag) { names = append(names, f.Name) })
	best, bestD := "", 3
	for _, n := range names {
		if d := distance(bad, n); d < bestD {
			best, bestD = n, d
		}
	}
	return best
}

// distance is the Levenshtein distance between a and b.
func distance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur := make([]int, len(br)+1)
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(br)]
}
