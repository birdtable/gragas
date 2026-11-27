package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The lines init prints for the rulebook's guard (S67, agents.guards.1): the
// pre-commit hook's, the agent's settings', and the pre-merge-commit hook's.
// git runs no pre-commit hook for a merge it commits itself, with no
// conflict: without the merge hook, guard staged's three-way rule would run
// only for a merge a person finishes with git commit. A missing binary fails
// closed.
const (
	GuardStagedLine = "gragas guard staged || exit 2"
	GuardEditLine   = "gragas guard edit || exit 2"
	GuardMergeLine  = "gragas guard staged --merge-hook || exit 2"
	// GateRule is the rule the gate holds: a change to a lifecycle field
	// without its history entry is refused.
	GateRule = "gragas guard staged"
)

// InitSchema is the schema number of Init, the document init answers with
// under --json.
const InitSchema = 1

// Init is what init prints besides its plan, so that a script reads it too:
// the guard's lines, the gate's rule and the line to keep the state file out
// of git.
type Init struct {
	Schema int `json:"schema"`
	// Guard is the pre-commit hook's line, the agent settings' and the
	// pre-merge-commit hook's, in that order.
	Guard  []string `json:"guard"`
	Gate   string   `json:"gate"`
	Ignore string   `json:"ignore"`
	// HashNote says the content hashes were recorded as given and not
	// verified.
	HashNote string `json:"hash_note,omitempty"`
}

// InitDoc is the Init of a form: ignore is the line for the state file, which
// the config's one path function names.
func InitDoc(f model.InitForm, ignore string) Init {
	d := Init{Schema: InitSchema, Guard: []string{GuardStagedLine, GuardEditLine, GuardMergeLine}, Gate: GateRule, Ignore: ignore}
	if f.Extends != nil || len(f.Imports) > 0 {
		d.HashNote = "the content hashes were recorded as given and not verified: fetching the parent and the imports comes later"
	}
	return d
}

// InitRulebook is the rulebook form of init (S67): the change that writes
// identity and structure from the form and adds the first history entry as its
// yes. l is the command's two lines, run the run id. A form with something
// wrong in it is an error that names the flag.
func InitRulebook(p port.Ports, root model.Root, f model.InitForm, l Lines, run, engine string) (*Change, error) {
	command := l.Command
	if problem := format.InitProblem(f); problem != "" {
		return nil, errors.New(problem)
	}
	var targets []model.Target
	plan := func(ctx context.Context) (model.Plan, error) {
		who, err := recorded(l.Who)
		if err != nil {
			return model.Plan{}, err
		}
		inputs, err := format.InitInputs(p.Tree, root)
		if err != nil {
			return model.Plan{}, err
		}
		want, err := format.PlanRulebookInit(f)
		if err != nil {
			return model.Plan{}, err
		}
		pl := model.Plan{Inputs: inputs}
		targets = nil
		for _, t := range want {
			have, err := p.Tree.Read(root, t.Path)
			switch {
			case err == nil && string(have) == string(t.Data):
				continue
			case err == nil:
				t.Before = apply.DigestBytes(have)
			case !errors.Is(err, fs.ErrNotExist):
				return model.Plan{}, fmt.Errorf("%s could not be read: %w", t.Path, err)
			}
			targets = append(targets, t)
			pl.Delta.Changes = append(pl.Delta.Changes, model.Change{Entity: "file:" + t.Path, Field: "content", After: summary(f, t.Path)})
		}
		pl.Sections = []model.Section{
			{Title: "what changes", Lines: []string{
				fmt.Sprintf("a %s rulebook %s at version %s", f.Kind, f.Name, f.Version),
				"its first layer " + f.Layer.Slug + " and part " + f.Part.Slug + ", with no rules yet",
			}},
		}
		pl.Yes = &model.Yes{Who: who, At: p.Clock.Now().Unix(), Command: command}
		return pl, nil
	}
	render := func(model.Delta) ([]model.Target, error) { return targets, nil }
	return NewChange(p, root, Spec{
		Command: command, Applies: l.Applies, Engine: engine, Run: run,
		Scope: append(format.InitFiles(), format.HistoryFile),
		Plan:  plan, Render: render, Record: format.InitRecord(p.Tree, p.Schema, root, f),
	}), nil
}

func summary(f model.InitForm, file string) string {
	if file == format.IdentityFile {
		return "who the rulebook is: " + f.Name + " " + f.Version
	}
	return "its layers and parts"
}
