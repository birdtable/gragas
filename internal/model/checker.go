package model

// CheckersDir is the folder of a rulebook that holds its checker code, one
// folder per artifact, each with its declaration (architecture 7.1).
const CheckersDir = "checkers"

// CheckerFile is the name of an artifact's declaration in its folder:
// checkers/<name>/checker.yaml.
const CheckerFile = "checker.yaml"

// Checker is one declaration of checker code in a rulebook (S23, S3):
// the rulebook's own plugin, analyzer, pattern rule or policy, with its own
// tests, or an import's artifact the rulebook declares by reference. It makes
// the artifact known, by name and publisher, so a consumer's tools load
// nothing the rulebook did not declare. It is no trust: nothing reads,
// loads or runs the code, and its digest is the release's.
type Checker struct {
	// Format is the format version the declaration is written in. Decides:
	// whether the reader reads it at all.
	Format int `json:"format"`
	// Name is the artifact's name, equal to its folder under checkers/.
	// Decides: which declaration a binding's checker names.
	Name string `json:"name"`
	// Tool is the tool whose plugin, analyzer, pattern rule or policy the
	// artifact is. Decides: whether a binding of another tool may name it.
	Tool string `json:"tool"`
	// Publisher is who publishes the artifact. Decides: an import's artifact
	// is declared only by its own name and publisher.
	Publisher string `json:"publisher"`
	// Tests is the folder or file of the artifact's own tests, relative to its
	// folder; "" where the declaration gives none, which is a finding for the
	// rulebook's own artifact. Decides: whether the artifact has tests.
	Tests string `json:"tests,omitempty"`
	// Licence is the artifact's licence, as a person writes it. Display only.
	Licence string `json:"licence,omitempty"`
	// From is the rulebook name of the import whose artifact this declares by
	// reference; "" for the rulebook's own. Decides: whether the artifact is
	// the rulebook's own, with its code and tests in its folder, or an
	// import's, listed under the import's name and publisher.
	From string `json:"from,omitempty"`
}

// CheckerArtifact is one artifact of checker code a parent's or an import's
// rulebook carries, by its name and publisher, as a fetched rulebook gives it.
// Nothing in it is loaded.
type CheckerArtifact struct {
	// Name is the artifact's name in its own rulebook. Decides: the importer's
	// declaration of it is found by it.
	Name string `json:"name"`
	// Publisher is who publishes it. Decides: the importer declares it by this
	// publisher and no other.
	Publisher string `json:"publisher"`
	// Tool is the tool it is for. Display only.
	Tool string `json:"tool"`
}

// CheckerRow is one artifact of checker code as the tools listing shows it:
// the rulebook's own under the rulebook's name, an import's by reference under
// the import's. Display only.
type CheckerRow struct {
	// Name is the artifact's name. Display only.
	Name string `json:"name"`
	// Tool is the tool it is for. Display only.
	Tool string `json:"tool"`
	// Publisher is who publishes it. Display only.
	Publisher string `json:"publisher"`
	// Rulebook is the rulebook it is the artifact of: this one, or the import
	// it is declared from. Display only.
	Rulebook string `json:"rulebook"`
	// ByReference is whether the artifact is an import's, declared here by
	// reference and not published by this rulebook. Display only.
	ByReference bool `json:"by_reference"`
}
