package contract

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"reflect"
	"testing"

	mc "github.com/birdtable/gragas/internal/model/contract"
)

// proposalOf is the fixture rulebook's proposal in state s, as the model
// value propose would encode (S65): the same change in each, and between
// them every kind of evidence, a link and none, and evidence and none.
func proposalOf(s mc.ProposalState) mc.ProposalDocument {
	link := mc.URL("https://example.com/fixture-parent/pull/1")
	p := mc.Proposal{
		Slug:   mc.Slug("tighten-" + string(s)),
		Target: "fixture-parent",
		Change: mc.ProposalChange{
			Anatomy: "A wrapped error names the step that failed, so its message says where.",
			Pair:    "pairs/go.errors.1/1",
			Binding: "bindings/std-errors-1.yaml",
			Sources: []mc.Slug{"effective-go", "outage-note"},
		},
		State: s,
		Link:  &link,
	}
	switch s {
	case mc.ProposalStateOpen:
		p.Opened = "2026-10-11"
		p.Evidence = []mc.ProposalEvidence{{Kind: mc.ProposalEvidenceKindWaiverPattern, Text: "Three waivers of the rule name one shape: an error returned bare from a loop over files."}}
	case mc.ProposalStateAccepted:
		p.Opened = "2026-10-12"
		p.Evidence = []mc.ProposalEvidence{{Kind: mc.ProposalEvidenceKindIncidentExample, Text: "An outage's log held \"open failed\" and no step, so its cause took an hour to find."}}
	case mc.ProposalStateDeclined:
		p.Opened = "2026-10-13"
		p.Evidence = []mc.ProposalEvidence{
			{Kind: mc.ProposalEvidenceKindAgentMiss, Text: "An agent returned a read's error unwrapped in four runs of one task."},
			{Kind: mc.ProposalEvidenceKindWaiverPattern, Text: "Two waivers of the rule name a test helper that returns the error it was given."},
		}
	case mc.ProposalStateWithdrawn:
		p.Opened, p.Link = "2026-10-14", nil
	}
	return mc.ProposalDocument{Proposal: p}
}

// proposalStates is every state of a proposal, in the schema's order.
func proposalStates() []mc.ProposalState {
	return []mc.ProposalState{mc.ProposalStateOpen, mc.ProposalStateAccepted, mc.ProposalStateDeclined, mc.ProposalStateWithdrawn}
}

// proposalGolden is the golden of the proposal in state s.
func proposalGolden(t *testing.T, s mc.ProposalState) []byte {
	t.Helper()
	b, err := os.ReadFile(path.Join("testdata", "proposal-"+string(s)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestEncodeProposalOfEachStateIsItsGoldenTwice: a proposal in each
// of its four states encodes to its golden bytes, and to the same bytes a
// second time (A20, tests.generated.5).
func TestEncodeProposalOfEachStateIsItsGoldenTwice(t *testing.T) {
	t.Parallel()
	for _, s := range proposalStates() {
		want := proposalGolden(t, s)
		first, err := EncodeProposal(proposalOf(s))
		if err != nil || !bytes.Equal(first, want) {
			t.Errorf("EncodeProposal(%s) = %v\n%s\nwant\n%s", s, err, first, want)
		}
		if second, err := EncodeProposal(proposalOf(s)); err != nil || !bytes.Equal(second, first) {
			t.Errorf("EncodeProposal(%s) a second time = %v\n%s\nwant\n%s", s, err, second, first)
		}
		var back mc.ProposalDocument
		dec := json.NewDecoder(bytes.NewReader(want))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&back); err != nil {
			t.Fatalf("decode the %s golden: %v", s, err)
		}
		if again, err := EncodeProposal(back); err != nil || !bytes.Equal(again, want) {
			t.Errorf("the %s golden read back and encoded = %v\n%s", s, err, again)
		}
	}
}

// TestTheEvidenceOfAProposalHoldsAKindAndATextAlone (spec 16.1): the
// generated evidence type has no field a project's name, path or repository
// could go in, so a proposal from a project carries no project identifier.
// The schema refuses such a key in the document (cmd/gragas's real-schema
// test). Planted failure: a field added to the evidence in the schema.
func TestTheEvidenceOfAProposalHoldsAKindAndATextAlone(t *testing.T) {
	t.Parallel()
	ty := reflect.TypeFor[mc.ProposalEvidence]()
	var got []string
	for f := range ty.Fields() {
		got = append(got, f.Tag.Get("json"))
	}
	if want := []string{"kind", "text"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the evidence holds %v, want %v alone", got, want)
	}
}
