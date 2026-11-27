package plugin

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
)

// addCmd is gragas plugin add: the plan of the files of a
// bundle and the lock last, written only with --apply. It approves nothing.
func addCmd(s kit.Session) *cobra.Command {
	c := &cobra.Command{
		Use:   "add <folder>",
		Short: "Add a plugin's bundle to the rulebook and lock it, approving nothing",
		Long: `Adds the bundle in <folder>, which is <name>/<version> of a plugin, to the
rulebook: its files go below plugins/<name>/<version>, and plugins/lock.yaml
names it, with the digest of the files and where they came from. The lock is
written last, so a run that stops early leaves no lock naming a half bundle.

The dry run lists each file and the lock entry and ends on the apply line.
A bundle whose files are over the sizes a bundle may be, whose manifest is
refused, that is locked already at other bytes, or whose folder is there and
not what the lock names, is refused and nothing is written. A second run of the
same bundle says there is nothing to write.

Adding approves nothing: the plugin does not run until a person has seen what
it is granted with gragas plugin approve.

  gragas plugin add ../plugins/alpha/1.0.0
  gragas plugin add ../plugins/alpha/1.0.0 --apply`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{kit.Writes: "with --apply"},
		RunE: func(c *cobra.Command, args []string) error {
			return s.Exit(s.PluginAdd(c.Context(), c, args[0]))
		},
	}
	c.Flags().Bool("apply", false, "Write the files")
	return c
}
