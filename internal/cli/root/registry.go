package root

import "github.com/spf13/cobra"

// treeNames are the names of the commands the tree holds.
func treeNames(tree *cobra.Command) []string {
	var out []string
	for _, c := range tree.Commands() {
		out = append(out, c.Name())
	}
	return out
}
