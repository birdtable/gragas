package coreapi

// The history of a rulebook, listed: the entries of history.jsonl in order,
// what the reader could not read, and what the check of the chain found. The
// file is read from one of two sources by one function: the working tree
// through the Tree port, or a revision through the History port. The reading
// itself is the lifecycle context's, which takes the lines as arguments.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// HistorySchema is the schema number of HistoryListing.
const HistorySchema = 1

// The types below live in the model, so the kit can name them.
type (
	// HistoryAsk is what a listing is asked for, as the model holds it.
	HistoryAsk = model.HistoryAsk
	// HistoryListing is the answer of a history listing, as the model holds it.
	HistoryListing = model.HistoryListing
	// HistoryRow is one entry of the history, as the model holds it.
	HistoryRow = model.HistoryRow
	// HistorySkipped is a line that could not be read, as the model holds it.
	HistorySkipped = model.HistorySkipped
	// HistoryFinding is a broken link or a gap in the chain, as the model holds it.
	HistoryFinding = model.HistoryFinding
)

// History lists the entries of the rulebook's history.jsonl. A missing file
// is an empty history with a note, not an error; a rev that does not exist, or
// a file that cannot be read, is.
func History(ctx context.Context, p port.Ports, ask HistoryAsk) (HistoryListing, error) {
	book, err := LoadBook(ctx, p, ask.Root)
	if err != nil {
		return HistoryListing{}, err
	}
	file := path.Join(book.Dir, format.HistoryFile)
	out := HistoryListing{Schema: HistorySchema, Source: "the working tree", File: file,
		Entries: []HistoryRow{}, Skipped: []HistorySkipped{}, Findings: []HistoryFinding{}, Notes: []string{}}
	var data []byte
	if ask.Rev == "" {
		data, err = p.Tree.Read(ask.Root, file)
	} else {
		out.Source = ask.Rev
		data, err = p.History.Blob(ctx, ask.Root, ask.Rev, file)
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		out.Notes = append(out.Notes, notThere(file, ask.Rev))
		out.Next = 1
		return out, nil
	case err != nil:
		return HistoryListing{}, fmt.Errorf("%s could not be read: %w", file, err)
	}
	chain := lifecycle.ReadFile(data)
	out.Total, out.Next = len(chain.Entries), chain.Next()
	keep, note := entityFilter(ask, book)
	if note != "" {
		out.Notes = append(out.Notes, note)
	}
	for _, e := range chain.Entries {
		if keep(e.Entity) {
			out.Entries = append(out.Entries, rowOf(e.HistoryEntry))
		}
	}
	for _, s := range chain.Skipped {
		out.Skipped = append(out.Skipped, HistorySkipped{Line: s.Line, Why: s.Why})
	}
	for _, f := range chain.Findings {
		out.Findings = append(out.Findings, HistoryFinding{Kind: string(f.Kind), Number: f.Number, Line: f.Line, Msg: f.Msg})
	}
	return out, nil
}

func notThere(file, rev string) string {
	if rev == "" {
		return file + " is not there, so the rulebook has no history yet"
	}
	return file + " is not there at " + rev + ", so the rulebook had no history yet"
}

// entityFilter answers which entities the ask keeps. A rule is kept under its
// id and every alias it has had; an id the book does not hold is matched as
// written, and the note says so.
func entityFilter(ask HistoryAsk, book BookReport) (keep func(string) bool, note string) {
	var names []string
	if ask.Rule != "" {
		names = []string{"rule:" + ask.Rule}
		if r, ok := book.Resolve(model.RuleID(ask.Rule)); ok {
			names = []string{"rule:" + string(r.ID)}
			for _, a := range r.Aliases {
				names = append(names, "rule:"+string(a))
			}
		} else {
			note = ask.Rule + " is not a rule of this rulebook, so its entries are matched by that name as written"
			if n := len(book.Findings); n > 0 {
				note += fmt.Sprintf(". The rulebook has %d findings, and a rule with one is not read, so the id may be one it holds", n)
			}
		}
	}
	return func(entity string) bool {
		return (ask.Rule == "" || slices.Contains(names, entity)) && (ask.Entity == "" || entity == ask.Entity)
	}, note
}

func rowOf(e model.HistoryEntry) HistoryRow {
	r := HistoryRow{Number: e.Number, Kind: e.Kind, Entity: e.Entity, Before: e.Before, After: e.After,
		Who: e.Who, When: e.When, Plan: string(e.Plan)}
	if e.Previous != "" {
		prev := string(e.Previous)
		r.Previous = &prev
	}
	if e.Decision != "" {
		r.Decision = &e.Decision
	}
	return r
}
