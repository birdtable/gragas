package coreapi

import (
	"context"
	"fmt"

	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ExplainSchema is the schema number of ExplainDoc.
const ExplainSchema = 1

// ResolveSchema is the schema number of ResolveDoc.
const ResolveSchema = 1

// UnknownRule is the answer to a rule id no rule holds under any name.
type UnknownRule struct{ Asked string }

func (e *UnknownRule) Error() string { return fmt.Sprintf("no rule has the id or alias %s", e.Asked) }

// rulebookHere reads the rulebook at root for a read-only listing: the book,
// its folder, and the generated presets' shipped bindings.
func rulebookHere(ctx context.Context, p port.Ports, root model.Root) (model.Book, string, []link.Shipped, []model.Held, error) {
	in, err := readBook(ctx, p, root)
	if err != nil {
		return model.Book{}, "", nil, nil, err
	}
	if !in.hasBook {
		return model.Book{}, "", nil, nil, ErrNoBook
	}
	if n := len(in.rep.Findings); n > 0 {
		return model.Book{}, "", nil, nil, &FindingsError{Count: n}
	}
	held, err := link.Derive(in.rep.Book, model.Grid{}, model.Chain{})
	if err != nil {
		return model.Book{}, "", nil, nil, err
	}
	// What ships is what the installed plugins' presets would write: the
	// bindings link.Shippable gives, at the file each manifest names. A tool
	// no plugin serves ships nothing.
	served, err := presetPlugins(ctx, p, root, in.rep.Book.Identity.Tested)
	if err != nil {
		return model.Book{}, "", nil, nil, err
	}
	shipped := shippedBy(in.rep.Book, served)
	if err := closePresetPlugins(served); err != nil {
		return model.Book{}, "", nil, nil, err
	}
	return in.rep.Book, in.rep.Dir, shipped, held, nil
}

// ExplainRule is what the rulebook at root says of the rule asked for, by id or
// alias. It reads and writes nothing else.
func ExplainRule(ctx context.Context, p port.Ports, root model.Root, asked string) (model.Explained, error) {
	if err := ctx.Err(); err != nil {
		return model.Explained{}, err
	}
	book, _, shipped, held, err := rulebookHere(ctx, p, root)
	if err != nil {
		return model.Explained{}, err
	}
	out, ok := link.ExplainRule(book, held, shipped, asked)
	if !ok {
		return model.Explained{}, &UnknownRule{Asked: asked}
	}
	return out, nil
}

// ExplainEntry is the rules of the rulebook at root that hold the entry key of
// tool; none is the unmapped answer, not an error.
func ExplainEntry(ctx context.Context, p port.Ports, root model.Root, tool, key string) (model.Explained, error) {
	if err := ctx.Err(); err != nil {
		return model.Explained{}, err
	}
	book, _, shipped, held, err := rulebookHere(ctx, p, root)
	if err != nil {
		return model.Explained{}, err
	}
	return link.ExplainEntry(book, held, shipped, tool, key), nil
}

// ExplainDoc is an explanation as the document under --json.
type ExplainDoc struct {
	Schema   int    `json:"schema"`
	Contract string `json:"contract"`
	Command  string `json:"command"`
	Word     string `json:"word"`
	model.Explained
}

// ExplainDocument is e as the document.
func ExplainDocument(e model.Explained) ExplainDoc {
	if e.Rules == nil {
		e.Rules = []model.ExplainedRule{}
	}
	return ExplainDoc{Schema: ExplainSchema, Contract: model.ContractVersion, Command: "explain", Word: string(model.Done), Explained: e}
}

// ResolveDoc is a resolved chain as the document under --json.
type ResolveDoc struct {
	Schema   int                   `json:"schema"`
	Contract string                `json:"contract"`
	Command  string                `json:"command"`
	Word     string                `json:"word"`
	Dir      string                `json:"dir"`
	Layers   []string              `json:"layers"`
	Entries  []model.ResolvedEntry `json:"entries"`
}

// ResolveDocument is r as the document.
func ResolveDocument(r model.ResolveReport) ResolveDoc {
	d := ResolveDoc{Schema: ResolveSchema, Contract: model.ContractVersion, Command: "resolve", Word: string(model.Done), Dir: r.Dir,
		Layers: append([]string{}, r.Resolved.Layers...), Entries: append([]model.ResolvedEntry{}, r.Resolved.Entries...)}
	return d
}
