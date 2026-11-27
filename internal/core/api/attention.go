package coreapi

// The attention list of a rulebook: what needs its maintainer, read from the
// book and the day the clock gives. The lifecycle context judges; this reads
// the book, asks the Clock port for the day and hands both over, so the engine
// keeps no timer (S55, arch.ports.1).

import (
	"context"
	"time"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// AttentionSchema is the schema number of AttentionDocument.
const AttentionSchema = 1

// OverrideRate is the override rate of the rules. The batches it is read from
// are not written yet, so it is not established, and says why (cli.output.9).
type OverrideRate struct {
	Value  *float64 `json:"value"`
	Reason string   `json:"reason"`
}

// AttentionDocument is the --json document of attention.
type AttentionDocument struct {
	Schema       int          `json:"schema"`
	OverrideRate OverrideRate `json:"override_rate"`
	model.AttentionListing
}

// Attention lists what needs the maintainer of the rulebook at root, as of the
// day the Clock says. A rulebook with findings is listed from the rules that
// were read, and the findings come with it. No rulebook is ErrNoBook, and one
// in a newer format is ErrNotReady.
func Attention(ctx context.Context, p port.Ports, root model.Root) (model.AttentionListing, error) {
	book, err := LoadBook(ctx, p, root)
	if err != nil {
		return model.AttentionListing{}, err
	}
	out := lifecycle.Attend(book.Book, p.Clock.Now().UTC().Format(time.DateOnly))
	out.Findings = append(out.Findings, book.Findings...)
	return out, nil
}

// AttentionDoc is the document of a listing: the listing and the override
// rate, which is null until batches are given.
func AttentionDoc(l model.AttentionListing) AttentionDocument {
	return AttentionDocument{Schema: AttentionSchema, AttentionListing: l,
		OverrideRate: OverrideRate{Reason: "no batches given"}}
}
