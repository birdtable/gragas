// Package contract is the contract context (domain 10, section 7.3): the one
// encoder every document is written with, in canonical bytes that name their
// contract version (A20, cli.output.1), and the readers of a document from
// outside, which check it against the schema of the version it names before
// anything reads a field (supply.scans.5). It reads no file and runs
// nothing: bytes come in and go out, and the schemas come through the ports.
package contract

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// Kind is one of the documents the contract defines.
type Kind string

// The documents of the contract, one schema each (section 7.3).
const (
	Bundle    Kind = "bundle"
	Report    Kind = "report"
	Changelog Kind = "changelog"
	SARIF     Kind = "sarif"
	Plan      Kind = "plan"
	Proposal  Kind = "proposal"
	Grid      Kind = "grid"
)

// Kinds are the documents of the contract, in the order section 7.3 names
// them.
func Kinds() []Kind { return []Kind{Bundle, Report, Changelog, SARIF, Plan, Proposal, Grid} }

// known is whether k is a document of the contract.
func (k Kind) known() bool { return slices.Contains(Kinds(), k) }

// schema is the name of k's schema in contract major major, as the Schema
// port compiled over the contract's schema folder names it.
func (k Kind) schema(major int) string {
	return fmt.Sprintf("v%d/document-%s.schema.json", major, k)
}

// SchemaDir is the folder of the Assets port that holds the contract
// schemas, one folder per major: a Schema port made over it names each
// schema by its path below it, such as v1/document-bundle.schema.json.
const SchemaDir = "contract/schema"

// MaxDocument is the most bytes a document read from outside may hold. A
// bundle of ten thousand rules without its proof grid is a few megabytes
// (section 8.4); this is room for the grid as well, and no more, so a reader
// never holds a document it was not meant to.
const MaxDocument = 64 << 20

// Version is a contract version, major.minor (spec section 9).
type Version struct{ Major, Minor int }

// Current is the contract version this engine writes, and the newest it
// reads: model.ContractVersion, which a test holds to be one.
func Current() Version {
	v, err := ParseVersion(model.ContractVersion)
	if err != nil {
		return Version{}
	}
	return v
}

func (v Version) String() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

var versionText = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)

// ParseVersion reads a contract version, two whole numbers with no sign and
// no leading zero, each of at most nine digits, joined by a dot.
func ParseVersion(s string) (Version, error) {
	m := versionText.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("%q is no contract version, which is major.minor", clip(s))
	}
	return Version{Major: digits(m[1]), Minor: digits(m[2])}, nil
}

// digits is the whole number of at most nine decimal digits d, which the
// pattern has already held to that.
func digits(d string) int {
	n := 0
	for _, c := range d {
		n = n*10 + int(c-'0')
	}
	return n
}

// NotReady is a document written in a contract major newer than this engine
// reads (S25): it is named, not guessed at. It ends "not ready".
type NotReady struct {
	// Kind is the document.
	Kind Kind
	// Found is the contract version the document names.
	Found Version
	// Reads is the newest contract version this engine reads.
	Reads Version
}

func (e *NotReady) Error() string {
	return fmt.Sprintf("the %s is written in contract %s and this engine reads up to contract %s; update the engine", e.Kind, e.Found, e.Reads)
}

// Word is the closing word a not-ready document ends on.
func (e *NotReady) Word() model.Word { return model.NotReady }

// Refused is a document from outside that is not read: too large, not one
// JSON value two readers read alike, naming no contract version, or not of
// its schema. Findings, when the schema refused it, are what the schema
// says. It ends "refused".
type Refused struct {
	// Kind is the document.
	Kind Kind
	// Why is what is wrong, in a sentence.
	Why string
	// Findings are the schema's problems, when it was the schema.
	Findings []model.Finding
	// Err is the cause, when there is one.
	Err error
}

func (e *Refused) Error() string {
	s := "the " + string(e.Kind) + " is not read: " + e.Why
	if len(e.Findings) > 0 {
		s += " (" + e.Findings[0].Msg
		if len(e.Findings) > 1 {
			s += fmt.Sprintf(", and %d more", len(e.Findings)-1)
		}
		s += ")"
	}
	return s
}

func (e *Refused) Unwrap() error { return e.Err }

// Word is the closing word a refused document ends on.
func (e *Refused) Word() model.Word { return model.Refused }

// clip is s cut to a length a message can hold.
func clip(s string) string {
	const most = 64
	if len(s) <= most {
		return s
	}
	return strings.ToValidUTF8(s[:most], "") + "..."
}
