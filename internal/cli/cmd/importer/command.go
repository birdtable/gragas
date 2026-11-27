// Package importer is the import command: it brings a guide and the config a
// project keeps already into the rulebook as rules, one plan shown first and
// written only with --apply. The command is named import; its folder cannot be,
// since the folder is the package name and import is a Go keyword. The root
// joins it from the generated join_gen.go.
package importer

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 90

// Row is the one declaration of import (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "import", Order: Order, Spec: true, Context: kit.InRulebook, Ends: kit.EndsPlan, State: kit.Built,
		Summary: "Bring a guide and a tool's config in as rules",
		Examples: [2]kit.Example{
			{Line: "gragas import <folder>", Does: "show the plan"},
			{Line: "gragas import <folder> --apply", Does: "carry it out"},
		},
		Writes:   []string{"the rules file of the concern it fills", "sources.yaml", "decisions.yaml", "history.jsonl"},
		Exits:    []model.Word{model.Done, model.Declined},
		JSONKind: "plan",
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	var in model.ImportInput
	var attribution string
	c := &cobra.Command{
		Use:   "import [<folder>]",
		Short: "Bring a guide and a tool's config in as rules, grandfathering what the config holds",
		Long: `Draft the rules of a guide and match them to the config a project
keeps already. A rule whose name is an entry of the config enters
active, origin imported, the guide its source, with a binding at the
entry's own severity; its pair is still to write, so the binding is
unproven. Every other rule enters proposed and ships in no preset. An
entry no rule names is a to-do. The plan lists the rules in those two
groups and every to-do, and writes only with --apply: the rules, the
guide as a source, one decision for the import and the history. A run
again after a yes plans nothing for the rules already in.

The folder holds the guide's Markdown files; the configs are read in
their own format as the tool's adapter says. Paths are relative to the
repository's root.

  gragas import docs/guide --into-part go --into-concern style --config rules.json --tool mytool --target source
  gragas import docs/guide --into-part go --into-concern style --guide-slug handlers --guide-title "Handler guide" --guide-url https://example.org/handlers --guide-tag v1.2.0 --apply`,
		Args: cobra.MaximumNArgs(1),
	}
	c.Annotations = map[string]string{"writes": "with --apply"}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			in.GuideDir = args[0]
		}
		in.Guide.Attribution = attribution
		return s.Exit(s.Import(cmd.Context(), cmd, in))
	}
	f := c.Flags()
	f.StringVar(&in.Part, "into-part", "", "The part the rules are drafted into")
	f.StringVar(&in.Concern, "into-concern", "", "The concern of that part the rules are drafted into")
	f.StringVar(&in.Guide.Slug, "guide-slug", "", "The guide's source, as lower-case words joined by hyphens")
	f.StringVar(&in.Guide.Title, "guide-title", "", "What the guide is called")
	f.StringVar(&in.Guide.URL, "guide-url", "", "Where the guide is found")
	f.StringVar(&in.Guide.Tag, "guide-tag", "", "The release of the guide the drafts are read at")
	f.StringVar(&attribution, "guide-attribution", "", "Who wrote the guide")
	f.StringVar(&in.Tool, "tool", "", "The tool's slug, as gragas tools shows")
	f.StringVar((*string)(&in.Target), "target", "", "What the entry is about, such as config")
	f.StringArrayVar(&in.Configs, "config", nil, "A config file to bring in, once for each")
	f.Bool("apply", false, "Write the files")
	return c
}
