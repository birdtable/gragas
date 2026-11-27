// Package plugin is the plugin command: fill in what it does. It lives in a package of its own
// below internal/cli/cmd; the root joins it from the generated join_gen.go,
// which task gen writes, so a new command is a new folder and no shared line.
package plugin

import (
	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1; 0 takes the next free one.
const Order = 215

// Row is the one declaration of plugin (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "plugin", Order: Order, Spec: true, Context: kit.InBoth, Ends: kit.EndsListing, State: kit.Built,
		Then:    []string{"test", "contract", "list", "approve", "add", "upgrade", "record"},
		Summary: "Prove, list, approve, add or upgrade a plugin, or say which contract this gragas speaks",
		Examples: [2]kit.Example{
			{Line: "gragas plugin test <folder>", Does: "run the checks over one bundle"},
			{Line: "gragas plugin contract --json", Does: "say the contract number and features, as one JSON document"},
		},
		Exits:    []model.Word{model.Done, model.Findings, model.NotReady, model.Refused, model.Declined},
		Writes:   []string{"this machine's record of approvals (approve)", "plugins/<name>/<version>/ files and plugins/lock.yaml (add)", "plugins/<name>/<version>/ files and plugins/lock.yaml, and the replaced version's files removed (upgrade)", "plugins/<name>/<version>/testdata/ recordings and cases (record)"},
		JSONKind: "plugin",
	}
}

// New is the cobra command, drawn through the session of the run. It is a stub
// that refuses until it is built.
func New(s kit.Session) *cobra.Command {
	c := &cobra.Command{Use: "plugin", Short: Row().Summary, Args: cobra.NoArgs}
	c.AddCommand(testCmd(s), contractCmd(s), listCmd(s), approveCmd(s), addCmd(s), upgradeCmd(s), recordCmd(s))
	return c
}
