package coreapi

import (
	"slices"

	"github.com/birdtable/gragas/internal/core/wire"
)

// PluginContract is what `gragas plugin contract --json` prints: the number of
// the plugin contract this gragas speaks and the features it has, sorted. It
// is how an author, the SDK and the plugins repository's gate learn what this
// gragas speaks. Contract here is the plugin contract's number,
// not the version of the documents gragas emits.
type PluginContract struct {
	Contract int      `json:"contract"`
	Features []string `json:"features"`
}

// PluginContractDoc reads the codec's one value, wire.Contract and
// wire.Features, the value a request carries and `plugin test` reads, and
// nothing else: no rulebook, no plugin, no approval, no port. A feature added
// there changes the print with no other edit. A feature list is never null.
func PluginContractDoc() PluginContract {
	f := slices.Clone(wire.Features())
	slices.Sort(f)
	return PluginContract{Contract: wire.Contract, Features: append([]string{}, f...)}
}
