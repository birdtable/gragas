package contract

import "github.com/birdtable/gragas/internal/model"

// Validation is what Validate found in one document.
type Validation struct {
	// Kind is the document.
	Kind Kind
	// Contract is the contract version the document names, and the zero
	// Version when it names none.
	Contract Version
	// Findings are what is wrong with it, nil when it is valid: not read,
	// naming no contract version or one this engine does not read, failing
	// the schema of the version it names, or holding a deprecated shape past
	// its window.
	Findings []model.Finding
}

// Validate checks doc, a document of kind k, against the schema of the
// contract version it names (S25, domain 10). A document in a newer major
// than this engine reads is *NotReady, which names both versions; everything
// else wrong with it is a finding. An error is a port that failed.
func Validate(k Kind, doc []byte, s Schemas) (Validation, error) {
	_, ver, findings, err := read(k, doc, s)
	if err != nil {
		return Validation{}, err
	}
	return Validation{Kind: k, Contract: ver, Findings: findings}, nil
}
