package plugin

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
)

// upgradeCmd is gragas plugin upgrade: the add of a newer
// version of an installed plugin, with what its grants changed, written only
// with --apply. It approves nothing.
func upgradeCmd(s kit.Session) *cobra.Command {
	c := &cobra.Command{
		Use:   "upgrade <source> <name>[@<version>]",
		Short: "Upgrade an installed plugin to a version a folder holds, approving nothing",
		Long: `Upgrades the installed plugin <name> to the bundle <source>/<name>/<version>,
read as plugin add reads one. With no version it takes the newest version
folder the source holds. The version replaced is the newest the lock holds of
that name.

The dry run shows the old and the new digest, and each grant added, removed or
changed with the arguments its program is asked its version with; then the new
files, the lock entry swapped, and the old version's files removed, which no
lock entry names any longer. Other versions of the plugin the lock holds stay,
and the plan says so. It ends on the apply line.

A version already installed at these files is nothing to do. A plugin the
lock does not hold is refused, naming plugin add. A version older than the one
installed is refused unless it was named. An installed bundle that is not what
its lock entry pins is refused, and its grants are not compared.

Upgrading approves nothing: the new version is installed and not approved by
the upgrade, and a digest not approved does not run until a person has seen
what it is granted with gragas plugin approve.

  gragas plugin upgrade ../plugins alpha
  gragas plugin upgrade ../plugins alpha@1.1.0 --apply`,
		Args:        cobra.ExactArgs(2),
		Annotations: map[string]string{kit.Writes: "with --apply"},
		RunE: func(c *cobra.Command, args []string) error {
			return s.Exit(s.PluginUpgrade(c.Context(), c, args[0], args[1]))
		},
	}
	c.Flags().Bool("apply", false, "Write the files")
	return c
}
