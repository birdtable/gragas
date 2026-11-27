package plugin

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
)

// recordCmd is gragas plugin record: it runs the real tool
// through an installed, approved plugin and files what it printed, and the
// case it makes, in the author's folder.
func recordCmd(s kit.Session) *cobra.Command {
	c := &cobra.Command{Use: "record <folder>", Short: "Run the real tool through a plugin and file the recording in its folder"}
	c.Long = `Records the plugin of <folder>, which is <name>/<version> in the project: it
runs the installed, approved plugin of that name and version by its own plan,
runs the real tool for each step it asks for, and files what the tool printed
under testdata/recordings/<tool version>/ and the case under
testdata/cases/<n>-<op>/ (request.json and response.json) of <folder>. The
scratch folder is written as /scratch, so no path of this machine is filed.

The installed copy is never written to. A folder whose plugin.wasm is not the
installed bundle's module is refused before any step runs: add the folder's
build and approve it first. A tool whose version is outside the plugin's
tool_versions is refused.

The dry run asks the tool its version and the plugin its first plan, lists the
steps and the files, and writes nothing. It ends on the apply line; --apply runs
the tool and writes, with no question asked.

  gragas plugin record plugins/alpha/1.0.0 --op entries --config .alpharc
  gragas plugin record plugins/alpha/1.0.0 --op findings --fixture src/a.txt --apply`
	c.Args = cobra.ExactArgs(1)
	c.Annotations = map[string]string{kit.Writes: "with --apply"}
	c.RunE = func(c *cobra.Command, args []string) error {
		return s.Exit(s.PluginRecord(c.Context(), c, args[0]))
	}
	c.Flags().Bool("apply", false, "Write the files")
	c.Flags().String("op", "entries", "The reading call to record: entries or findings")
	c.Flags().StringArray("config", nil, "A config file to bring in, once for each")
	c.Flags().StringArray("fixture", nil, "A file of the project the tool runs over (repeatable)")
	c.Flags().String("case", "", "The number of the case folder; the next free one when not given")
	return c
}
