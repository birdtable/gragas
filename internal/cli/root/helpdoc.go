package root

import (
	"github.com/birdtable/gragas/internal/cli/kit"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// helpTarget is the command whose help the arguments ask for: the words of
// "help <command>", or the command before --help. A word that names no
// command is the root.
func helpTarget(tree *cobra.Command, args []string) *cobra.Command {
	var words []string
	for _, a := range beforeDashes(args) {
		if !strings.HasPrefix(a, "-") && a != "help" {
			words = append(words, a)
		}
	}
	c, _, err := tree.Find(words)
	if err != nil || c == nil {
		return tree
	}
	return c
}

// helpDoc is the help of c as a document: what the screen says, with the
// flags, their types and defaults, and the closing words the registry gives
// the command.
func helpDoc(r kit.Registry, tree, c *cobra.Command) coreapi.Help {
	d := coreapi.Help{Contract: model.ContractVersion, Command: strings.TrimSpace(strings.TrimPrefix(c.CommandPath(), tree.Name())),
		Summary: c.Short, About: strings.TrimSpace(c.Long), Usage: c.UseLine(),
		Commands: []coreapi.HelpCommand{}, Flags: flagsOf(c.NonInheritedFlags()), Global: flagsOf(tree.PersistentFlags())}
	for _, sub := range c.Commands() {
		if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
			continue
		}
		d.Commands = append(d.Commands, coreapi.HelpCommand{Name: sub.Name(), Summary: sub.Short})
	}
	slices.SortFunc(d.Commands, func(a, b coreapi.HelpCommand) int { return strings.Compare(a.Name, b.Name) })
	if c == tree {
		d.Exits = []coreapi.TreeExit{}
		for _, w := range model.Words() {
			code, _ := exit.Code(w)
			d.Exits = append(d.Exits, coreapi.TreeExit{Word: string(w), Code: code})
		}
		return d
	}
	row, _ := r.Find(c.Name())
	d.Exits = exitsOf(row)
	return d
}

// helpAsJSON answers a help request under --json: the document on standard
// output and nothing else. It returns the exit code.
func (s *session) helpAsJSON(tree *cobra.Command, args []string) int {
	s.document(helpDoc(commands(), tree, helpTarget(tree, args)))
	return exit.Done
}
