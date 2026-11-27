package generate

import "github.com/birdtable/gragas/internal/ordered"

// Book is what generate reads of a loaded rulebook. generate declares it
// here, on its own side, so it imports no other domain (arch.ports.8):
// internal/core/api hands it the rulebook domain's book through this
// interface, and layers.yaml lists the domains that may not import each
// other.
type Book interface {
	// Root is the rulebook's folder, from the repository root.
	Root() string
	// Parts are the parts, in the order of the parts table.
	Parts() []Part
	// Concerns are a part's concern files, in any order.
	Concerns(part string) []Concern
	// Tools are a part's tools, in file order.
	Tools(part string) []*ordered.Map
	// Registry is the product kinds and the source registry.
	Registry() Registry
	// HasGuide is whether a part has a guide of its own.
	HasGuide(part string) bool
	// Rules are every rule of the book, ordered by id.
	Rules() []Rule
	// Standing is a rule's derived standing, by id.
	Standing(id string) string
	// Check is what the rulebook found of one check.
	Check(check *ordered.Map) CheckState
	// LockIDs is every rule id ever issued, sorted.
	LockIDs() []string
}

// Registry is what products.yaml and sources.yaml hold.
type Registry struct {
	// Kinds are the product kinds, in file order.
	Kinds []*ordered.Map
	// Sources is the source registry, by key.
	Sources *ordered.Map
}

// CheckState is what the rulebook found of one check.
type CheckState struct {
	// Resolves is whether the check's tool and key resolve.
	Resolves bool
	// Holds is whether the check holds its rule: it resolves and, for a
	// check of our own code, its test exists.
	Holds bool
}

// Part is one part of the book.
type Part struct {
	Short, Layer, Name, Description string
	// LayerName is the display name of the part's layer.
	LayerName string
	// BuildsOn are the short names of the parts it builds on.
	BuildsOn []string
	// Data is the part's row of the parts table, as written.
	Data *ordered.Map
}

// Concern is one concern file of a part.
type Concern struct {
	// Name is the concern, as its file is named.
	Name string
	// Order is where the concern sits in its part.
	Order int
	// Doc is the file as written.
	Doc *ordered.Map
}

// Rule is one rule, with the part and concern it is filed under.
type Rule struct {
	ID, Part, Concern string
	Data              *ordered.Map
}
