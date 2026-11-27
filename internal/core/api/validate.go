package coreapi

import (
	"context"
	"errors"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ValidateSchema is the schema number of Validation, the document validate
// answers with under --json.
const ValidateSchema = 1

// OpenRecordError is a root whose journal holds a run that never closed. A
// reading command ends "not ready" on it, naming journal recover, because what
// the files say may be half of a run (cli.errors.1). It matches ErrNotReady.
type OpenRecordError struct{ Run string }

func (e *OpenRecordError) Error() string {
	return "a run that wrote here never closed (run " + e.Run + ")"
}

// Is makes an open record a not-ready ending.
func (e *OpenRecordError) Is(target error) bool { return target == ErrNotReady }

// RecoverLine is what to type to settle an open run.
const RecoverLine = "gragas journal recover --apply"

// Validate reads the rulebook at root through the ports and says what is
// wrong with it and what is only worth saying. An open run in the root's
// journal is ErrOpenRecord, before anything else is read. A format newer than
// the engine is ErrNotReady, naming both versions; findings are in the value,
// not the error. namesFile, when not empty, turns the names check on.
func Validate(ctx context.Context, p port.Ports, root model.Root, namesFile string) (model.Validation, error) {
	if err := ctx.Err(); err != nil {
		return model.Validation{}, err
	}
	if err := refuseOpenRun(ctx, p, root); err != nil {
		return model.Validation{}, err
	}
	rep, err := LoadBookNamed(ctx, p, root, namesFile)
	if err != nil {
		return model.Validation{}, err
	}
	return model.Validation{Dir: rep.Dir, Rules: len(rep.Book.Rules), Findings: rep.Findings, Notices: rep.Notices, Held: rep.Held}, nil
}

// IsOpenRecord is whether err is a root with an open run.
func IsOpenRecord(err error) bool {
	_, ok := errors.AsType[*OpenRecordError](err)
	return ok
}

// Reason is the sentence of a not-ready error without the closing word it
// ends on: both versions of a format newer than the engine, the run that
// never closed, the line of a names list that does not compile, a parent
// not fetched at its pin.
func Reason(err error) string {
	if nr, ok := errors.AsType[*format.NotReady](err); ok {
		return nr.Error()
	}
	if open, ok := errors.AsType[*OpenRecordError](err); ok {
		return open.Error()
	}
	if words := namesWords(err); words != "" {
		return words
	}
	if parent, ok := errors.AsType[*ParentError](err); ok {
		return parent.Error()
	}
	return err.Error()
}

// Validation is validate's document under --json. A listing longer than Shown
// says so: Total is the count, and Findings holds the first Shown.
type Validation struct {
	Schema   int    `json:"schema"`
	Contract string `json:"contract"`
	Command  string `json:"command"`
	// Word is the closing word: done or findings.
	Word string `json:"word"`
	// Dir is the folder of rulebook.yaml below the root, "" for the root.
	Dir   string `json:"dir"`
	Rules int    `json:"rules"`
	// Held is how many migrated rules the prose lint holds.
	Held int `json:"held"`
	// Total is how many findings there are, and Shown how many Findings holds.
	Total    int             `json:"total"`
	Shown    int             `json:"shown"`
	Findings []model.Finding `json:"findings"`
	Notices  []model.Notice  `json:"notices"`
	Next     string          `json:"next,omitempty"`
}

// ValidationDoc is v as the document, listing at most limit findings.
func ValidationDoc(v model.Validation, limit int, next string) Validation {
	shown := v.Findings[:min(limit, len(v.Findings))]
	d := Validation{Schema: ValidateSchema, Contract: model.ContractVersion, Command: "validate", Word: string(v.Word()),
		Dir: v.Dir, Rules: v.Rules, Held: v.Held, Total: len(v.Findings), Shown: len(shown), Findings: append([]model.Finding{}, shown...),
		Notices: append([]model.Notice{}, v.Notices...), Next: next}
	return d
}

// refuseOpenRun is an OpenRecordError when the root's journal holds a run that
// never closed, nil otherwise: every reading command asks it first.
func refuseOpenRun(ctx context.Context, p port.Ports, root model.Root) error {
	listing, err := JournalList(ctx, p, root)
	if err != nil {
		return err
	}
	for _, r := range listing.Runs {
		if r.State == "open" {
			return &OpenRecordError{Run: r.Run}
		}
	}
	return nil
}
