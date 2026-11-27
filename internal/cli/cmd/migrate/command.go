// Package migrate is the migrate command: it plans the move of a schema v2
// rulebook to the format, with the carry-over count per kind and every item it
// could not carry, and writes it with --apply. It lives in a package of its own
// below internal/cli/cmd; the root joins it from the generated join_gen.go.
package migrate

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 20

// Row is the one declaration of migrate (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "migrate", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsPlan, State: kit.Built,
		Summary: "Move a rulebook to the format this build reads",
		Examples: [2]kit.Example{
			{Line: "gragas migrate --name rb --owner me", Does: "show the plan"},
			{Line: "gragas migrate --name rb --owner me --apply", Does: "carry it out"},
		},
		Writes:   []string{"every file of the rulebook that changes format", "history.jsonl"},
		Exits:    []model.Word{model.Done, model.Declined, model.NotReady},
		JSONKind: "plan",
	}
}

// flags are what the identity of the migrated rulebook is answered with: the
// old layout held none, so each is a flag.
type flags struct {
	name, version, language, licence, presets, docs string
	owners                                          []string
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	var f flags
	c := &cobra.Command{
		Use:   "migrate",
		Short: Row().Summary,
		Long: `Plans the move of a schema v2 rulebook to the format: its
identity and structure, its rules re-keyed, its sources and
bindings carried, each old facet made a kind. The plan prints
how many of each kind it carries and every item it could not
carry, with the reason. Before anything is written the result
is read back and held to the same counts.

Without --apply it prints the plan and writes nothing. A
rulebook already in the format ends done, nothing to do.

  gragas migrate --name my-rules --owner me
  gragas migrate --name my-rules --owner me --apply`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"writes": "with --apply"},
	}
	fl := c.Flags()
	fl.StringVar(&f.name, "name", "", "The rulebook's short name, a slug")
	fl.StringVar(&f.version, "version", "1.0.0", "The starting version the first release publishes")
	fl.StringVar(&f.language, "language", "en", "The authoritative language")
	fl.StringVar(&f.licence, "licence", "CC-BY-4.0", "The licence of the rules")
	fl.StringVar(&f.presets, "presets-licence", "MIT", "The licence of the presets")
	fl.StringVar(&f.docs, "docs-url", "", "The base URL of the published pages")
	fl.StringArrayVar(&f.owners, "owner", nil, "Who owns the rulebook, shown in its views; it grants nothing (repeat for more)")
	fl.Bool("apply", false, "Write the files")
	c.RunE = func(cmd *cobra.Command, _ []string) error { return s.Exit(run(cmd, s, f)) }
	return c
}

func run(c *cobra.Command, s kit.Session, f flags) int {
	return s.Migrate(c.Context(), c, f.form())
}

func (f flags) form() model.InitForm {
	g := model.Governance{Contribution: model.ContributionProposals, DecisionRequired: true, TrialReleases: 3, ReviewDays: 90}
	return model.InitForm{
		Name: f.name, Kind: model.KindBase, Version: f.version, Language: f.language,
		Licences: model.Licences{Rules: f.licence, Presets: f.presets}, DocsURL: f.docs, Owners: f.owners, Governance: g,
	}
}
