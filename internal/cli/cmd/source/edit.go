package source

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// editCmd is gragas source edit.
func editCmd(s kit.Session) *cobra.Command {
	var title, locator, attribution string
	c := &cobra.Command{
		Use:   "edit <slug>",
		Short: "Change a source's title, locator or attribution",
		Long: `Change what a source is called, where it is found or who wrote
it. An empty attribution takes it away. The change is noted in the
history of every rule that cites the source, in the same plan, and
the plan lists those rules. Words the source has already end
"nothing to do".

  gragas source edit <slug> --locator "https://example.com/new"
  gragas source edit <slug> --title "A new title" --attribution "" --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"writes": "with --apply"},
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		req := model.EditSource{Slug: args[0]}
		if cmd.Flags().Changed("title") {
			req.Title = &title
		}
		if cmd.Flags().Changed("locator") {
			req.Locator = &locator
		}
		if cmd.Flags().Changed("attribution") {
			req.Attribution = &attribution
		}
		return s.Exit(s.SourceEdit(cmd.Context(), cmd, req))
	}
	f := c.Flags()
	f.StringVar(&title, "title", "", "What it is called")
	f.StringVar(&locator, "locator", "", "A URL, an ISBN, a DOI, an RFC or a decision slug")
	f.StringVar(&attribution, "attribution", "", "Who wrote it")
	f.Bool("apply", false, "Write the files")
	return c
}
