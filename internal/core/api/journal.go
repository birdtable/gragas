package coreapi

// The journal of a root, listed: each run that wrote, from the record alone.
// An answer is built from the fixed fields of the records, never from a tool,
// and names the root by its digest, never by its folder (cli.commands.24).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// JournalSchema is the schema number of JournalListing.
const JournalSchema = 1

// ErrNoRun is a run id the journal holds no record of.
var ErrNoRun = errors.New("the journal holds no record of that run")

// ErrRunOpen is a run that has no close, which restore cannot tell a moved
// file from: recover settles it first.
var ErrRunOpen = errors.New("the run has no close, so it is still open")

// The types below live in the model, so the kit can name them.
type (
	// JournalRun is one run, as its records say, as the model holds it.
	JournalRun = model.JournalRun
	// JournalListing is the runs of one root, as the model holds it.
	JournalListing = model.JournalListing
)

// A copyOf is the path a copy record names.
type copyDetail struct {
	Path    string `json:"path"`
	Existed bool   `json:"existed"`
	Before  string `json:"before"`
}

// closeDetail is the part of a close record restore and list read.
type closeDetail struct {
	Outcome string `json:"outcome"`
	Files   []struct {
		Path, Before, After string
	} `json:"files"`
}

// group is one run's records and where in the journal they begin.
type group struct {
	run   JournalRun
	first int
	recs  []model.Record
	shut  *model.Record
}

// groups takes the records of a root by run, in the order each run began.
func groups(recs []model.Record) []*group {
	var out []*group
	by := map[string]*group{}
	for i, r := range recs {
		g := by[r.Run]
		if g == nil {
			g = &group{first: i, run: JournalRun{Run: r.Run, State: "open", Files: []string{}}}
			by[r.Run], out = g, append(out, g)
		}
		g.recs = append(g.recs, r)
		if g.run.Command == "" {
			g.run.Command = r.Command
		}
		if g.run.Plan == "" {
			g.run.Plan = r.Plan
		}
		at := r.At
		if g.run.Started == nil {
			g.run.Started = &at
		}
		switch r.Kind {
		case model.KindCopy:
			var c copyDetail
			if json.Unmarshal(r.Detail, &c) == nil && c.Path != "" {
				g.run.Files = append(g.run.Files, c.Path)
			}
		case model.KindClose:
			g.shut = &g.recs[len(g.recs)-1]
			g.run.Ended = &at
			g.run.State = outcomeOf(r)
		}
	}
	return out
}

// outcomeOf is the outcome a close record names.
func outcomeOf(r model.Record) string {
	var c closeDetail
	if err := json.Unmarshal(r.Detail, &c); err != nil || c.Outcome == "" {
		return "unknown"
	}
	return c.Outcome
}

// JournalList lists the runs of root from its journal. A root with no journal
// holds no runs. It reads through the Journal port alone.
func JournalList(ctx context.Context, ports port.Ports, root model.Root) (JournalListing, error) {
	recs, torn, err := ports.Journal.Records(ctx, root)
	if err != nil {
		return JournalListing{}, fmt.Errorf("the journal could not be read: %w", err)
	}
	out := JournalListing{Schema: JournalSchema, Torn: torn, Runs: []JournalRun{}}
	if len(recs) > 0 {
		out.Root = recs[0].Root
	}
	for _, g := range groups(recs) {
		out.Runs = append(out.Runs, g.run)
	}
	return out, nil
}
