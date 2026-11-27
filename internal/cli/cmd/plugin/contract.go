package plugin

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// contractCmd is gragas plugin contract.
func contractCmd(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use:   "contract",
		Short: "Print the plugin contract number and features",
		Long: `The number of the plugin contract this gragas speaks and the
features it has, sorted, so an author, the SDK and the plugins
repository's gate learn what this gragas speaks. It reads nothing
else: no rulebook, no plugin and no approval, so it runs in any
folder, and it writes nothing. --json gives the same as one JSON
document; contract 1 has no features.

  gragas plugin contract
  gragas plugin contract --json`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return s.Exit(runContract(s)) },
	}
}

func runContract(s kit.Session) int {
	d := coreapi.PluginContractDoc()
	if s.JSON() {
		s.Document(d)
		return exit.Done
	}
	features := "none"
	if len(d.Features) > 0 {
		features = strings.Join(d.Features, ", ")
	}
	s.Say("plugin contract " + strconv.Itoa(d.Contract) + ", features: " + features)
	return s.End(kit.Ending{Word: model.Done, What: "this is the plugin contract above"})
}
