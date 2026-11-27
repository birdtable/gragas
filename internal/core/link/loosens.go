package link

import "github.com/birdtable/gragas/internal/model"

// Loosens says whether next reports less than old on the same entry (S70):
// a lower severity, or a value that the plugin's rank op reads as weaker than
// old's, an off value included. A value change the plugin cannot rank, or one
// of a tool no plugin serves (a nil rank), counts as loosening: what cannot be
// compared is not known to be as tight.
func Loosens(rank Rank, old, next model.Binding) bool {
	return severityRank(next.Severity) < severityRank(old.Severity) || !valueNotLooser(rank, next.Key, next.Value, old.Value)
}
