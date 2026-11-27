package plugin

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// listCmd is gragas plugin list.
func listCmd(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the installed plugins and whether each is approved",
		Long: `Lists the plugins the rulebook's plugin lock names, by name and then
version: the tool each serves, the digest of the bundle as it lies on disk,
and whether that digest is approved on this machine. A bundle that changed
since it was locked, or whose manifest is refused, is listed with a note and
is not approved. It reads the bundles and the approvals and changes nothing;
a lock, a bundle or a record of approvals it cannot read ends the run failed,
and is never shown as unapproved.

  gragas plugin list
  gragas plugin list --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			l, err := s.PluginList(c.Context())
			if err != nil {
				return s.Exit(s.End(kit.Ending{Word: model.Failed, What: "the plugins could not be listed: " + err.Error()}))
			}
			approved := 0
			for _, r := range l.Rows {
				if r.Approved {
					approved++
				}
			}
			what := fmt.Sprintf("%s, %d approved", model.Plural(len(l.Rows), "plugin"), approved)
			return s.Exit(kit.EndListing(s, l, what, plugins(l)))
		},
	}
}

// plugins is the table above the closing word, or the one line that says there
// are none.
func plugins(l model.PluginsListing) []model.Block {
	if len(l.Rows) == 0 {
		return []model.Block{model.Note{Text: "no plugins are locked here"}}
	}
	t := model.Table{Columns: []model.Column{
		{Title: "plugin", Priority: 9}, {Title: "tool", Priority: 7}, {Title: "approved", Priority: 8},
		{Title: "digest", Priority: 3}, {Title: "note", Priority: 6},
	}}
	for _, r := range l.Rows {
		yes := "no"
		if r.Approved {
			yes = "yes"
		}
		t.Rows = append(t.Rows, []string{r.Name + "@" + r.Version, r.Tool, yes, string(r.Digest), r.Note})
	}
	return []model.Block{t}
}
