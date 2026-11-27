package model

// ChainLayer is one layer of a chain, supplied as a value: the rulebook as
// read, under the name the chain knows it by. Fetching a layer is no part of
// resolving (A10).
type ChainLayer struct {
	// Name is the layer's name. Display only.
	Name string `json:"name"`
	// Book is the layer's rulebook.
	Book Book `json:"-"`
}

// Resolution is the provenance word of a resolved entry: what its layer did to it.
type Resolution string

// The three things a layer does to an entry (S3, S8).
const (
	// ResolvedSet is an entry the layer set.
	ResolvedSet Resolution = "set"
	// ResolvedChanged is a parent's entry an overlay tightened.
	ResolvedChanged Resolution = "changed"
	// ResolvedExcluded is a parent's entry an overlay dropped with a decision.
	ResolvedExcluded Resolution = "excluded"
)

// Resolutions is the closed set, in the order the spec gives them.
func Resolutions() []Resolution {
	return []Resolution{ResolvedSet, ResolvedChanged, ResolvedExcluded}
}

// ResolvedEntry is one binding of the effective rulebook and where it came
// from.
type ResolvedEntry struct {
	// Rule is the id of the rule that holds the entry, as its layer wrote it.
	// Decides: which rule the entry belongs to.
	Rule RuleID `json:"rule"`
	// Binding is the entry as the effective rulebook holds it. Decides.
	Binding Binding `json:"binding"`
	// Layer is the layer that set the entry, or last changed it. Display only.
	Layer string `json:"layer"`
	// How is what that layer did. Decides: an excluded entry is no part of a
	// preset.
	How Resolution `json:"how"`
	// Decision is the decision behind a change or an exclusion, "" for none.
	// Display only.
	Decision string `json:"decision,omitempty"`
}

// Resolved is the effective rulebook of a chain: every entry with its
// provenance, in the order the chain gives them.
type Resolved struct {
	// Layers are the names of the layers, the root first.
	Layers []string `json:"layers"`
	// Entries are the entries, in the order of the layers and their rules.
	Entries []ResolvedEntry `json:"entries"`
}
