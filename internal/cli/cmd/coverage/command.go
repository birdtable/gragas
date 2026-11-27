// Package coverage is the coverage command: in a rulebook it lists, both ways,
// the rules an effective binding holds and the entries the rules explain, with
// the upstream entries nothing explains. It lives in a package of its own
// below internal/cli/cmd; the root joins it from the generated join_gen.go. It
// reads nothing itself: it asks the session, which asks the core, and draws
// what comes back.
package coverage

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
const Order = 190

// Row is the one declaration of coverage (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "coverage", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsListing, State: kit.Built,
		Summary: "List the rules tools hold and those none hold",
		Examples: [2]kit.Example{
			{Line: "gragas coverage", Does: "list it"},
			{Line: "gragas coverage --json", Does: "as one JSON document"},
		},
		Exits:    []model.Word{model.Done},
		JSONKind: "coverage",
	}
}

// long is the help text of the command.
const long = `Lists coverage both ways. Rules to entries: each rule, its standing,
whether an effective binding holds it, and the entries its bindings
name, with the date a planned rule should be held by where one is
set. Entries to rules: each entry a rule explains, with the rules
that hold it. An upstream preset adopted whole at rung 0 counts as
one set under its rung-0 rule and lists its entries beneath it. The
upstream entries that no binding and no adopted set explains are
listed apart, from the flattened sets in the rulebook's upstream
folder; a tool with no such file has none. It always
ends done: an unmapped entry is a line of the listing, and the
rulebook's gate reports it as a finding. It reads and changes
nothing.

  gragas coverage
  gragas coverage --json`

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use: "coverage", Short: Row().Summary, Long: long, Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return s.Exit(run(c, s)) },
	}
}

func run(c *cobra.Command, s kit.Session) int {
	cov, err := s.Coverage(c.Context())
	if err != nil {
		return notReady(s, err)
	}
	what := said(cov)
	if s.JSON() {
		s.Document(coreapi.CoverageDocument(cov))
		return s.End(kit.Ending{Word: model.Done, What: what}) // draws nothing under --json
	}
	return s.End(kit.Ending{Word: model.Done, What: what, Next: next(cov), Lead: lead(cov)})
}

// notReady ends the run on a folder whose coverage cannot be listed.
func notReady(s kit.Session, err error) int {
	var findings *coreapi.FindingsError
	var refused *coreapi.UpstreamRefused
	var newer *coreapi.UpstreamNotReady
	switch {
	case errors.As(err, &newer):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error(), Next: "gragas version"})
	case errors.As(err, &refused):
		return s.End(kit.Ending{Word: model.Refused, What: err.Error() + ": write the file again, or remove it", Next: "gragas coverage"})
	case coreapi.IsOpenRecord(err):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: coreapi.RecoverLine})
	case errors.Is(err, coreapi.ErrNotReady):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: "gragas version"})
	case errors.Is(err, coreapi.ErrNoBook), errors.Is(err, coreapi.ErrTwoBooks):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error() + ". coverage lists a rulebook's here", Next: "gragas init"})
	case errors.As(err, &findings):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error(), Next: "gragas validate"})
	}
	return s.End(kit.Ending{Word: model.Failed, What: "the coverage could not be listed: " + err.Error()})
}

// said is the sentence beside the closing word.
func said(c model.Coverage) string {
	s := fmt.Sprintf("%s held, %s explained", model.Plural(c.RulesHeld(), "rule"), model.Plural(len(c.Entries), "entry"))
	if n := len(c.Unmapped); n > 0 {
		s += ", " + model.Plural(n, "unmapped entry")
	}
	return s
}

// next is the line to type after the word: with nothing bound, offers bind.
func next(c model.Coverage) string {
	if len(c.Entries) == 0 {
		return "gragas bind"
	}
	return ""
}

// lead is the listing above the closing word: the rules, the entries, and the
// unmapped upstream entries apart.
func lead(c model.Coverage) []model.Block {
	var out []model.Block
	if len(c.Entries) == 0 {
		out = append(out, model.Note{Text: "nothing is bound: 0 rules held, 0 entries explained. Bind a rule to a tool entry with `gragas bind`."})
	}
	if len(c.Rules) > 0 {
		t := model.Table{Columns: []model.Column{{Title: "rule", Priority: 9}, {Title: "standing", Priority: 8}, {Title: "entries", Priority: 7}}}
		for _, r := range c.Rules {
			t.Rows = append(t.Rows, []string{string(r.Rule), standing(r), entries(r)})
		}
		out = append(out, model.Heading{Text: "Rules to entries"}, t)
	}
	if len(c.Entries) > 0 {
		t := model.Table{Columns: []model.Column{{Title: "entry", Priority: 9}, {Title: "held by", Priority: 8}}}
		for _, e := range c.Entries {
			t.Rows = append(t.Rows, []string{e.Tool + " " + unit(e), ids(e.Rules)})
		}
		out = append(out, model.Heading{Text: "Entries to rules"}, t)
		for _, e := range c.Entries {
			if e.Set != "" {
				out = append(out, model.Note{Text: "set " + e.Set + " holds " + strings.Join(e.Members, ", ")})
			}
		}
	}
	if len(c.Unmapped) > 0 {
		items := make([]string, len(c.Unmapped))
		for i, u := range c.Unmapped {
			items[i] = u.Tool + " " + u.Entry
		}
		out = append(out, model.Heading{Text: "Unmapped: held upstream, explained by no rule"}, model.List{Items: items})
	}
	return out
}

// standing is the rule's standing and, for a planned one, its date.
func standing(r model.CoverRule) string {
	if r.ZeroBy != "" && !r.Held {
		return string(r.Standing) + ", by " + r.ZeroBy
	}
	return string(r.Standing)
}

func entries(r model.CoverRule) string {
	if len(r.Entries) == 0 {
		return "none"
	}
	return strings.Join(r.Entries, ", ")
}

// unit is an entry as listed: an adopted set is one line that says how many
// entries it holds and names them.
func unit(e model.CoverEntry) string {
	if e.Set == "" {
		return e.Entry
	}
	return fmt.Sprintf("%s (set of %d)", e.Entry, len(e.Members))
}

func ids(rules []model.RuleID) string {
	out := make([]string, len(rules))
	for i, id := range rules {
		out[i] = string(id)
	}
	return strings.Join(out, ", ")
}
