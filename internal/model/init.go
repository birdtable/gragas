package model

// InitForm is what a person answers to start a rulebook (S67, S68). Every
// field has the flag that answers it, which the command names. It is data: the
// checks on it are format's.
type InitForm struct {
	// Name is the rulebook's short name, a slug.
	Name string
	// Kind is base, overlay, companion or fork. An archived rulebook is
	// never started.
	Kind IdentityKind
	// Version is the starting version the first release publishes unchanged.
	Version string
	// Language is the authoritative language.
	Language string
	// Licences are the licences per kind of content.
	Licences Licences
	// DocsURL is the base of the published pages, "" for none.
	DocsURL string
	// Owners are who own the rulebook, shown in its views; they grant nothing.
	Owners []string
	// Governance is how the rulebook takes changes.
	Governance Governance
	// Extends is the parent of an overlay at a pin, with the content hash of
	// that release as given. Nothing fetches the parent here, so the hash is
	// not verified.
	Extends *Extends
	// Imports are the parts taken from other rulebooks, each at a pin and
	// with its content hash as given.
	Imports []Import
	// Layer and Part are the first layer and part of the structure.
	Layer, Part Named
}

// Named is a slug with the name shown for it.
type Named struct{ Slug, Name string }
