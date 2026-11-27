// Package resolve is the resolve command: in a rulebook it lists the effective
// rulebook of its chain, each entry with the layer that set, changed or excluded
// it. It fetches nothing (A10): an overlay whose parent is not here as a value
// ends not ready naming the pin. It reads nothing itself: it asks the session.
package resolve

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 220

// Row is the one declaration of resolve (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "resolve", Order: Order, Spec: true, Context: kit.InBoth, Ends: kit.EndsListing, State: kit.Built,
		Summary: "List the rulebook a chain resolves to, and where each entry came from",
		Examples: [2]kit.Example{
			{Line: "gragas resolve", Does: "list it"},
			{Line: "gragas resolve --json", Does: "as one JSON document"},
		},
		Exits:    []model.Word{model.Done, model.NotReady},
		JSONKind: "resolve",
	}
}

// long is the help text of the command.
const long = `Lists the effective rulebook of the chain: each tool entry with the
rule that holds it, the layer that set it or last changed it, and
what that layer did (set, changed or excluded). An excluded entry
names its decision and is no part of a preset. A rulebook with no
parent is the one layer and lists itself. An overlay needs its
parent release as a value; resolve fetches nothing, so it ends not
ready and names the pin it could not read. It reads and changes
nothing.

  gragas resolve
  gragas resolve --json`

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use: "resolve", Short: Row().Summary, Long: long, Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return s.Exit(run(c, s)) },
	}
}

func run(c *cobra.Command, s kit.Session) int {
	r, err := s.Resolve(c.Context())
	if err != nil {
		return notReady(s, err)
	}
	what := fmt.Sprintf("%s in %s", model.Plural(len(r.Resolved.Entries), "entry"), model.Plural(len(r.Resolved.Layers), "layer"))
	if s.JSON() {
		s.Document(coreapi.ResolveDocument(r))
		return s.End(kit.Ending{Word: model.Done, What: what}) // draws nothing under --json
	}
	return s.End(kit.Ending{Word: model.Done, What: what, Lead: lead(r.Resolved)})
}

// notReady ends the run on a chain that cannot be resolved.
func notReady(s kit.Session, err error) int {
	var parent *coreapi.ParentError
	var findings *coreapi.FindingsError
	switch {
	case errors.As(err, &parent):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error(), Next: "gragas validate"})
	case coreapi.IsOpenRecord(err):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: coreapi.RecoverLine})
	case errors.Is(err, coreapi.ErrNotReady):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: "gragas version"})
	case errors.Is(err, coreapi.ErrNoBook), errors.Is(err, coreapi.ErrTwoBooks):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error() + ". resolve reads a rulebook", Next: "gragas init"})
	case errors.As(err, &findings):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error(), Next: "gragas validate"})
	}
	return s.End(kit.Ending{Word: model.NotReady, What: "the chain could not be resolved: " + err.Error(), Next: "gragas validate"})
}

// lead is the listing above the closing word: the layers, and a table of the
// entries.
func lead(r model.Resolved) []model.Block {
	out := []model.Block{model.Note{Text: "layers, root first: " + strings.Join(r.Layers, ", ")}}
	if len(r.Entries) == 0 {
		return append(out, model.Note{Text: "no rule is bound to a tool entry"})
	}
	t := model.Table{Columns: []model.Column{{Title: "entry", Priority: 9}, {Title: "rule", Priority: 8}, {Title: "layer", Priority: 7}, {Title: "how", Priority: 6}}}
	for _, e := range r.Entries {
		how := string(e.How)
		if e.Decision != "" {
			how += " (" + e.Decision + ")"
		}
		t.Rows = append(t.Rows, []string{fmt.Sprintf("%s %s: %s", e.Binding.Tool, e.Binding.Target, e.Binding.Key), string(e.Rule), e.Layer, how})
	}
	return append(out, t)
}
