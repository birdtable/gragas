package coreapi

import (
	"context"
	"strings"

	"github.com/birdtable/gragas/internal/core/contract"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ConflictsSchema is the schema number of ConflictsDoc, the document the
// conflicts command answers with under --json.
const ConflictsSchema = 1

// ListConflicts lists the entry conflicts of the rulebook at root: each tool
// entry that two bindings name with different values, options or severity,
// the values compared through the rank op of the installed plugin that serves
// the tool, at the version the tested set names where the plugin serves it
// and for each the rule a decision names as the winner, if one does. A folder
// with no rulebook is ErrNoBook, a rulebook with findings is FindingsError,
// and an open run and a newer format are as for the other listings. Two
// values that differ as text, of a tool no installed plugin serves, end it
// UnservedTools; a plugin refused, or one that fails to rank, ends it with no
// listing (fail closed). A listing whose values are the same as text opens no
// plugin.
func ListConflicts(ctx context.Context, p port.Ports, root model.Root) (model.ConflictsListing, error) {
	if err := ctx.Err(); err != nil {
		return model.ConflictsListing{}, err
	}
	in, err := readBook(ctx, p, root)
	if err != nil {
		return model.ConflictsListing{}, err
	}
	if !in.hasBook {
		return model.ConflictsListing{}, ErrNoBook
	}
	if n := len(in.rep.Findings); n > 0 {
		return model.ConflictsListing{}, &FindingsError{Count: n}
	}
	rk := newRankers(p.Plugins, root).reading(in.rep.Book.Identity)
	conflicts := link.Conflicts(in.rep.Book, rk.of(ctx))
	if err := rk.close(); err != nil {
		return model.ConflictsListing{}, err
	}
	return model.ConflictsListing{Dir: in.rep.Dir, Conflicts: nonNilConflicts(conflicts)}, nil
}

func nonNilConflicts(c []model.Conflict) []model.Conflict {
	if c == nil {
		return []model.Conflict{}
	}
	return c
}

// ResolutionLines are the three ways to resolve an open conflict, as printed
// lines: each is its own plan elsewhere, and gragas picks no winner (S8).
func ResolutionLines() []string {
	return []string{
		"split the entry: give each rule an entry of its own, since different options are usually different entries",
		"rebind one rule to another entry or value: gragas rebind",
		"declare a conflicts relation on both rules and record a decision that names the winning binding: gragas decision new",
	}
}

// ConflictsDoc is the conflicts listing as the document under --json.
type ConflictsDoc struct {
	Schema   int    `json:"schema"`
	Contract string `json:"contract"`
	Command  string `json:"command"`
	// Word is the closing word: findings when any conflict is open, else done.
	Word string `json:"word"`
	// Dir is the folder of the rulebook below the root, "" for the root.
	Dir string `json:"dir"`
	// Open is how many conflicts block their bindings, Resolved how many a
	// decision settles.
	Open     int `json:"open"`
	Resolved int `json:"resolved"`
	// Conflicts are every conflict, open and resolved.
	Conflicts []model.Conflict `json:"conflicts"`
	// Resolutions are the three ways to resolve an open conflict, said only
	// while one is open.
	Resolutions []string `json:"resolutions"`
}

// ConflictsDocument is l as the document.
func ConflictsDocument(l model.ConflictsListing) ConflictsDoc {
	d := ConflictsDoc{Schema: ConflictsSchema, Contract: model.ContractVersion, Command: "conflicts", Word: string(l.Word()),
		Dir: l.Dir, Open: l.Open(), Conflicts: append([]model.Conflict{}, l.Conflicts...), Resolutions: []string{}}
	d.Resolved = len(l.Conflicts) - d.Open
	if d.Open > 0 {
		d.Resolutions = ResolutionLines()
	}
	return d
}

// conflictsCheckText is the sentence SARIF shows beside model.ConflictsCheck.
const conflictsCheckText = "The conflicts check: two rules bind one tool entry to different values, options or severity, so neither binding ships"

// ConflictsSARIF is the open conflicts of l as one SARIF document of the
// contract (cli.output.2), a result for each, with no file or line: a conflict
// is between two bindings, not at a place. tag is the engine's release tag, as
// for ValidationSARIF.
func ConflictsSARIF(l model.ConflictsListing, tag string) ([]byte, error) {
	version := tag
	if len(tag) > 1 && tag[0] == 'v' {
		version = tag[1:]
	}
	tool := contract.Tool{Name: "gragas", Version: version, Checks: []contract.Check{{ID: model.ConflictsCheck, Text: conflictsCheckText}}}
	results := []contract.Result{}
	for _, c := range l.Conflicts {
		if !c.Open() {
			continue
		}
		results = append(results, contract.Result{Check: model.ConflictsCheck, Msg: ConflictLine(c), Fix: ResolutionLines()[0]})
	}
	return contract.EncodeFindings(tool, results)
}

// ConflictLine is one conflict as a sentence: the entry, the rules that hold
// it and what they differ in.
func ConflictLine(c model.Conflict) string {
	rules := make([]string, len(c.Sides))
	for i, s := range c.Sides {
		rules[i] = string(s.Rule)
	}
	return c.Entry.Tool + " " + string(c.Entry.Target) + ": " + c.Entry.Key + " is set differently by " +
		strings.Join(rules, " and ") + " (" + strings.Join(c.Differs, ", ") + ")"
}
