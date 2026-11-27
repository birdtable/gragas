package main

import (
	"bytes"
	"os"
	"path"
	"regexp"
	"testing"

	"github.com/birdtable/gragas/internal/host/assets"
	"github.com/birdtable/gragas/internal/host/jsonschema"
)

// The contract schemas of v1 as a Schema port compiled over the folder
// core/contract names, contract/schema. Only this package may join a host
// adapter with the core, so the documents core/contract encodes are checked
// against the real schemas here, as the bytes they are.

// minimalDocs is the folder of core/contract's minimal documents, one per
// document of the contract, from this package's folder.
const minimalDocs = "../../internal/core/contract/testdata"

// TestEveryMinimalContractDocumentPassesItsRealSchema: the golden bytes core/
// contract's Encode is held to are documents the real schema of contract 1.0
// takes, and the schema refuses the inputs ReadBundle's steps plant.
func TestEveryMinimalContractDocumentPassesItsRealSchema(t *testing.T) {
	t.Parallel()
	s := jsonschema.Embedded(assets.New(), "contract/schema")
	read := func(kind string) []byte {
		t.Helper()
		b, err := os.ReadFile(path.Join(minimalDocs, kind+".json"))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, kind := range []string{"bundle", "report", "changelog", "sarif", "plan", "proposal"} {
		got, err := s.Check("v1/document-"+kind+".schema.json", read(kind))
		if err != nil || len(got) != 0 {
			t.Errorf("Check(minimal %s) = %v, %v, want none", kind, got, err)
		}
	}
	// The SARIF core/contract's EncodeFindings is held to, with a result of
	// each shape it writes (A34).
	if got, err := s.Check("v1/document-sarif.schema.json", read("findings.sarif")); err != nil || len(got) != 0 {
		t.Errorf("Check(EncodeFindings' golden) = %v, %v, want none", got, err)
	}
	bundle := read("bundle")
	planted := map[string][]byte{
		"a bundle with no fields but its contract": []byte(`{"contract": "1.0"}`),
		"a key that differs only in case":          bytes.Replace(bundle, []byte(`"released": false,`), []byte(`"released": false, "Rules": [],`), 1),
		"an unknown field":                         bytes.Replace(bundle, []byte(`"released": false,`), []byte(`"released": false, "trusted": true,`), 1),
		// ReadBundle's drift check passes a lone empty key that differs from
		// an omitempty field's name only in case; the schema refuses it.
		"a lone empty case variant of exclusions": bytes.Replace(bundle, []byte(`"released": false,`), []byte(`"released": false, "Exclusions": [],`), 1),
		"a lone null case variant of conflicts":   bytes.Replace(bundle, []byte(`"released": false,`), []byte(`"released": false, "Conflicts": null,`), 1),
		"a lone empty case variant of presets":    bytes.Replace(bundle, []byte(`"released": false,`), []byte(`"released": false, "PRESETS": [],`), 1),
	}
	for name, doc := range planted {
		if bytes.Equal(doc, bundle) {
			t.Fatalf("%s: the plant did not change the bundle", name)
		}
		if got, err := s.Check("v1/document-bundle.schema.json", doc); err != nil || len(got) == 0 {
			t.Errorf("Check(%s) = %v, %v, want problems", name, got, err)
		}
	}
}

// TestEachProposalGoldenPassesItsRealSchemaAndEvidenceNamesNoProject:
// the proposal in each of its four states is a document contract 1.0 takes,
// and evidence that holds a key a project could be named by, a kind the
// spec does not give, a kind or a text alone, or bare text as before is
// refused (spec 16.1, S65).
func TestEachProposalGoldenPassesItsRealSchemaAndEvidenceNamesNoProject(t *testing.T) {
	t.Parallel()
	s := jsonschema.Embedded(assets.New(), "contract/schema")
	const schema = "v1/document-proposal.schema.json"
	var open []byte
	for _, state := range []string{"open", "accepted", "declined", "withdrawn"} {
		b, err := os.ReadFile(path.Join(minimalDocs, "proposal-"+state+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if got, err := s.Check(schema, b); err != nil || len(got) != 0 {
			t.Errorf("Check(proposal %s) = %v, %v, want none", state, got, err)
		}
		if state == "open" {
			open = b
		}
	}
	const kind = `"kind": "waiver-pattern",`
	text := regexp.MustCompile(`,\s*"text": "[^"]*"`)
	planted := map[string][]byte{
		"a project named in the evidence":     bytes.Replace(open, []byte(kind), []byte(kind+` "project": "fixture-project",`), 1),
		"a path in the evidence":              bytes.Replace(open, []byte(kind), []byte(kind+` "path": "/repo/main.go",`), 1),
		"a repository in the evidence":        bytes.Replace(open, []byte(kind), []byte(kind+` "repository": "https://example.com/fixture-project",`), 1),
		"an evidence kind the spec gives not": bytes.Replace(open, []byte(kind), []byte(`"kind": "commit",`), 1),
		"evidence with no text":               text.ReplaceAll(open, nil),
		"evidence with no kind":               bytes.Replace(open, []byte(kind), nil, 1),
		"evidence as bare text":               bytes.Replace(open, []byte(`"evidence": [`), []byte(`"evidence": ["a waiver pattern", `), 1),
	}
	for name, doc := range planted {
		if bytes.Equal(doc, open) {
			t.Fatalf("%s: the plant did not change the proposal", name)
		}
		if got, err := s.Check(schema, doc); err != nil || len(got) == 0 {
			t.Errorf("Check(%s) = %v, %v, want problems", name, got, err)
		}
	}
}

// TestValidateSARIFPassesTheRealSARIFSchema: what the built binary writes
// under validate --sarif, held by the journeys to their goldens, is a SARIF
// document the contract's real schema of 1.0 takes.
func TestValidateSARIFPassesTheRealSARIFSchema(t *testing.T) {
	t.Parallel()
	s := jsonschema.Embedded(assets.New(), "contract/schema")
	// The journeys' goldens of validate --sarif, the binary's stdout.
	for _, name := range []string{"../../internal/journeys/testdata/sarif-validate-done.golden", "../../internal/journeys/testdata/sarif-validate-findings.golden"} {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := s.Check("v1/document-sarif.schema.json", b); err != nil || len(got) != 0 {
			t.Errorf("Check(%s) = %v, %v, want none", name, got, err)
		}
	}
}
