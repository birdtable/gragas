package coreapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// GuardSchema is the version of guard edit's document under --json.
const GuardSchema = 1

// GuardCommand is the line guard edit is run by, as the journal names it.
const GuardCommand = "gragas guard edit"

// GuardAsk is one guard edit: the repository's root, the hook's input or why
// it could not be read, and the run and engine its refusal is journalled
// under. A Run of "" is a run of the refusal's own.
type GuardAsk struct {
	Root   model.Root
	Input  []byte
	Unread error
	Run    string
	Engine string
}

// GuardEdit judges one proposed edit in the rulebook's repository at
// ask.Root: allowed when it changes no lifecycle field (S59), refused
// otherwise, naming each field and the command to use (A29, agents.guards.1).
// Whatever it cannot read or place is refused, never allowed: an input it
// cannot read or parse, a path that is not clean, a path outside the root or
// reached by a link that leads out, a path whose last part is a link, a file
// with more than one name by hard links, a file
// named in another case than its own, an edit the tool itself would refuse, a
// rulebook the edit leaves unreadable, and a repository that cannot be read.
// Every refusal made here is journalled, and a journal that cannot be written
// leaves it refused. A fault of the guard's own, a panic, is the root's one
// recover's: it ends guard edit refused, exit 5, and the root journals that
// refusal too, through JournalGuardCrash, once the guard has opened the
// rulebook's root; a crash before the root is open has no journal to keep it, and says so on
// standard error. So every refusal is journalled that has a root to be
// journalled in. The one error is a folder with no rulebook, or with one not
// ready to read (ErrNoBook, ErrTwoBooks, ErrNotReady): the guard is not for a
// project (S1).
func GuardEdit(ctx context.Context, p port.Ports, ask GuardAsk) (model.Guard, *model.Refusal, error) {
	g, why, hint, err := guardEdit(ctx, p, ask)
	switch {
	case errors.Is(err, ErrNoBook), errors.Is(err, ErrTwoBooks), errors.Is(err, ErrNotReady):
		return g, nil, err
	case err != nil:
		why, hint = "the repository could not be read, so the edit is refused: "+err.Error(), "gragas validate"
	case why == "":
		return g, nil, nil
	}
	ask.Run = guardRun(p, ask)
	if jerr := journalRefusal(context.WithoutCancel(ctx), p, ask, GuardCommand, g, why); jerr != nil {
		why += ". The refusal could not be journalled: " + jerr.Error()
	}
	r, rerr := model.NewRefusal(model.Refused, why, nil, hint)
	return g, &r, rerr
}

// guardEdit is GuardEdit before the journal: the answer, and when the edit is
// refused, why and the line to type.
func guardEdit(ctx context.Context, p port.Ports, ask GuardAsk) (g model.Guard, why, hint string, err error) {
	const readHook = "the hook gives one Write, Edit or MultiEdit as JSON on standard input"
	asked := map[string]bool{}
	before, err := format.Load(asking{Tree: p.Tree, root: ask.Root, asked: asked}, p.Schema, ask.Root)
	if err != nil {
		return model.Guard{}, "", "", guardLoadErr(err)
	}
	if ask.Unread != nil {
		return model.Guard{}, "the proposed edit could not be read: " + ask.Unread.Error(), readHook, nil
	}
	e, err := ParseHookEdit(ask.Input)
	if err != nil {
		return model.Guard{}, "the proposed edit could not be read: " + err.Error(), readHook, nil
	}
	rel, err := p.Tree.Within(ask.Root, e.Path)
	if errors.Is(err, port.ErrNotClean) {
		// Within follows a link before a .., as the kernel does, and the
		// host's own tool may apply the .. to the spelling first, so link/..
		// could name one file to the guard and write another
		return model.Guard{}, "the path is not clean, so the guard cannot tell which file the host writes: " + e.Path,
			"name the file by its clean absolute path: no . or .. part, and no separator doubled or at the end", nil
	}
	if errors.Is(err, port.ErrLinked) {
		// a tool that writes in place writes the file the link leads to, and
		// one that writes a new file and renames it over replaces the link
		// itself with a file the guard never judged there
		return model.Guard{}, "the file is a link, so the guard cannot tell which file the host writes: " + err.Error(),
			"edit the file the link leads to, by its own path", nil
	}
	if errors.Is(err, port.ErrOutside) {
		return model.Guard{}, "the file is outside the rulebook's repository, or reached through a link that leads out: " + e.Path,
			"edit the file by its path inside the repository", nil
	}
	if err != nil {
		return model.Guard{}, "the file could not be placed in the repository: " + err.Error(), "name the file by its absolute path", nil
	}
	g.Path = rel
	// the path as the edit spells it: a host that answers Within in the
	// disk's own spelling must still find a part named in another case.
	// A path that reaches the root only through a link below it has no
	// written form in the root, and is judged by where it leads alone.
	written, err := p.Tree.Written(ask.Root, e.Path)
	if err != nil && !errors.Is(err, port.ErrOutside) {
		return g, "the file could not be placed in the repository: " + err.Error(), "name the file by its absolute path", nil
	}
	old, exists, why, hint := current(p.Tree, p.FileNames, ask.Root, rel, written, asked)
	if why != "" && hint == "" {
		hint = "edit the file by the name it has"
	}
	if why != "" {
		return g, why, hint, nil
	}
	now, err := e.Applied(old, exists)
	if err != nil {
		return g, "the edit cannot be made: " + err.Error(), "read the file again and make the edit on what it holds", nil
	}
	after, err := format.Load(proposed{Tree: p.Tree, root: ask.Root, path: rel, data: now}, p.Schema, ask.Root)
	if err != nil {
		return g, "the rulebook could not be read with the edit made: " + err.Error(), "gragas validate", nil
	}
	if f, ok := newFinding(before.Findings, after.Findings); ok {
		return g, "the edit leaves a finding the rulebook did not have: " + f, "fix the edit so gragas validate has nothing new to say", nil
	}
	if f, ok := findingAt(before.Findings, rel); ok && hasFindingAt(after.Findings, rel) {
		// a file the loader reads none of has no rules before or after an
		// edit inside it, so such an edit is judged only once it leaves the
		// file whole
		return g, "the file has a finding before the edit and after it, so the guard cannot read what the edit changes: " + f,
			"fix the file whole in one edit, so gragas validate has nothing to say of it", nil
	}
	g.Changes = lifecycle.GuardEdit(before.Book, after.Book, shown)
	if len(g.Changes) == 0 {
		return g, "", "", ctx.Err()
	}
	c := g.Changes[0]
	why = fmt.Sprintf("%s changes %s of %s by hand", rel, c.Field, c.Entity)
	if n := len(g.Changes); n > 1 {
		why = fmt.Sprintf("%s changes %d lifecycle fields by hand, first %s of %s", rel, n, c.Field, c.Entity)
	}
	hint = c.Command
	if hint == "" {
		hint = "leave " + c.Field + " as it is: no command changes it by hand"
	}
	return g, why, hint, nil
}

// findingAt is the first of findings in the file rel.
func findingAt(findings []format.Finding, rel string) (string, bool) {
	for _, f := range findings {
		if f.File == rel {
			return f.String(), true
		}
	}
	return "", false
}

// hasFindingAt is whether findings hold one in the file rel.
func hasFindingAt(findings []format.Finding, rel string) bool {
	_, ok := findingAt(findings, rel)
	return ok
}

// guardLoadErr is a rulebook that cannot be read before the edit: not one
// here, or not readable at all.
func guardLoadErr(err error) error {
	if nr, ok := errors.AsType[*format.NotReady](err); ok {
		return fmt.Errorf("%w: %w", ErrNotReady, nr)
	}
	return err
}

// guardRun is the run a guard's refusal is journalled under: the run's own
// record where it has one, and otherwise, since guard edit writes no file, a
// run of one close named by the instant and the input.
func guardRun(p port.Ports, ask GuardAsk) string {
	if ask.Run != "" {
		return ask.Run
	}
	return fmt.Sprintf("guard-%d-%s", p.Clock.Now().UnixNano(), digest.Hex(ask.Input)[:8])
}

// JournalGuardCrash journals the refusal of a guard that crashed, guard edit
// or guard staged as command names it, from the root's crash path, as a guard's own refusal is journalled: one close record
// under the guard's run, outcome refused, the command named and the cause a
// crash. It never holds the panic's text, which is the crashed code's and
// may carry anything the change held; the trace is on standard error.
func JournalGuardCrash(ctx context.Context, p port.Ports, ask GuardAsk, command string) error {
	detail, err := json.Marshal(map[string]any{"outcome": "refused", "cause": "crash",
		"why": command + " hit a bug and stopped, so the change is refused"})
	if err != nil {
		return err
	}
	return p.Journal.Append(ctx, ask.Root, model.Record{Run: guardRun(p, ask), Kind: model.KindClose, Command: command, Engine: ask.Engine, Detail: detail})
}

// journalRefusal writes the refusal of the guard command named to the journal
// of ask's root, under its run and engine, in one record (agents.guards.1; a
// refusal is journalled).
func journalRefusal(ctx context.Context, p port.Ports, ask GuardAsk, command string, g model.Guard, why string) error {
	fields := make([]map[string]string, 0, len(g.Changes))
	for _, c := range g.Changes {
		fields = append(fields, map[string]string{"entity": c.Entity, "field": c.Field})
	}
	detail, err := json.Marshal(map[string]any{"outcome": "refused", "path": g.Path, "why": why, "fields": fields})
	if err != nil {
		return err
	}
	return p.Journal.Append(ctx, ask.Root, model.Record{Run: ask.Run, Kind: model.KindClose, Command: command, Engine: ask.Engine, Detail: detail})
}

// GuardDocument is guard edit's answer under --json: the closing word, the
// file, every lifecycle field the edit changes, and on a refusal why and the
// line to type. Display only.
type GuardDocument struct {
	Schema   int             `json:"schema"`
	Contract string          `json:"contract"`
	Command  string          `json:"command"`
	Word     string          `json:"word"`
	Path     string          `json:"path"`
	Changes  []model.Guarded `json:"changes"`
	Why      string          `json:"why,omitempty"`
	Next     string          `json:"next,omitempty"`
}

// GuardDoc is g as the document: done with why "", refused otherwise, with
// why and the line to type.
func GuardDoc(g model.Guard, why, next string) GuardDocument {
	return GuardDocAs(GuardCommand, g, why, next)
}

// GuardDocAs is GuardDoc for the guard command named: guard edit's or guard
// staged's.
func GuardDocAs(command string, g model.Guard, why, next string) GuardDocument {
	d := GuardDocument{Schema: GuardSchema, Contract: model.ContractVersion, Command: command, Word: string(model.Done),
		Path: g.Path, Changes: append([]model.Guarded{}, g.Changes...)}
	if why != "" {
		d.Word, d.Why, d.Next = string(model.Refused), why, next
	}
	return d
}

// GuardCommands is every command line the guard's hints can name, as
// lifecycle.GuardCommands gives them, for the root to check against the
// commands it registers.
func GuardCommands() []string { return lifecycle.GuardCommands() }

// shown is v as the guard shows a lifecycle value: compact JSON, a string
// bare, and "" for a value that is empty or absent.
func shown(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "unreadable: " + err.Error()
	}
	switch string(b) {
	case "null", "[]", "{}":
		return ""
	}
	// A JSON string is bare: its quotes off and its escapes undone. json.Marshal
	// writes only escapes a Go string literal reads the same (\n, \", \\, \u00XX),
	// so the unquote is exact. Nothing here decodes: it reads no input.
	if b[0] == '"' {
		if s, err := strconv.Unquote(string(b)); err == nil {
			return s
		}
	}
	return string(b)
}
