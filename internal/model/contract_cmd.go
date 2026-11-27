package model

// ContractCheckSchema, ContractFieldsSchema and ContractFixtureSchema are the
// schema numbers of the documents the contract commands answer with under
// --json.
const (
	ContractCheckSchema   = 1
	ContractFieldsSchema  = 1
	ContractFixtureSchema = 1
)

// ContractCheck is what `contract validate` found in one document: the kind it
// was read as, the contract version it names and its findings. A document the
// engine cannot read, or a kind that is none, is told in NotReady or Usage and
// carries no findings.
type ContractCheck struct {
	Schema   int       `json:"schema"`
	Kind     string    `json:"kind"`
	Contract string    `json:"contract,omitempty"`
	Findings []Finding `json:"findings"`
	// NotReady is set when the document is in a newer contract major.
	NotReady *ContractNotReady `json:"not_ready,omitempty"`
	// Usage is set, with Known, when the kind is unknown or cannot be told:
	// it says what to type instead.
	Usage string   `json:"usage,omitempty"`
	Known []string `json:"known,omitempty"`
}

// ContractNotReady names the two versions of a document in a newer major.
type ContractNotReady struct {
	Found string `json:"found"`
	Reads string `json:"reads"`
}

// Word is the closing word of the check.
func (c ContractCheck) Word() Word {
	switch {
	case c.Usage != "":
		return Usage
	case c.NotReady != nil:
		return NotReady
	case len(c.Findings) > 0:
		return Findings
	}
	return Done
}

// ContractField is one field of an entity, with the version that introduced it.
type ContractField struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Since   string `json:"since"`
	Default string `json:"default"`
}

// ContractFields is the fields of one entity of the contract.
type ContractFields struct {
	Schema   int             `json:"schema"`
	Entity   string          `json:"entity"`
	Source   string          `json:"source"`
	Contract string          `json:"contract"`
	Fields   []ContractField `json:"fields"`
	// Usage is set, with Known, for an entity the contract does not have.
	Usage string   `json:"usage,omitempty"`
	Known []string `json:"known,omitempty"`
}

// ContractFixture is what `contract fixture` writes, or would write, to --out.
type ContractFixture struct {
	Schema int      `json:"schema"`
	Out    string   `json:"out"`
	Files  []string `json:"files"`
	// Wrote says the files were written; a dry run lists the same files and
	// writes none.
	Wrote bool `json:"wrote"`
}
