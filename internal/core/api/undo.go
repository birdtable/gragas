package coreapi

import (
	"context"
	"path"
	"strings"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Undo is the change that takes back the last change the history of the
// rulebook in ask.Root records, by a new entry that returns the lifecycle
// fields it moved. entry is the number or digest the run names ("" for none) and applying
// says the run has --apply: see lifecycle.PlanUndoEntry.
func Undo(ctx context.Context, p port.Ports, ask model.ChangeAsk, entry string, applying bool) (*Change, *model.Refusal, error) {
	return composeTo(ctx, p, ask, undoFiles, unranked(lifecycle.PlanUndoEntry(entry, applying)))
}

// UndoEntry is the reference (a number, or a digest when a number is shared) of the entry undo would take back now in the
// rulebook of root, for the apply line a dry run ends on; false when there is
// none or the book cannot be read (the plan says why).
func UndoEntry(p port.Ports, root model.Root, namesFile string) (string, bool) {
	names, _, err := readBookNames(p.Owned, namesFile)
	if err != nil {
		return "", false
	}
	l, err := format.LoadWith(p.Tree, p.Schema, root, format.Options{Names: names})
	if err != nil {
		return "", false
	}
	t, ok := lifecycle.UndoTarget(l.Book.History)
	return lifecycle.UndoRef(l.Book.History, t), ok
}

// undoFiles are the files an undo writes: the history, and the file of the
// entity the entry to take back names. No target, or an entity no plan of undo
// writes, leaves the history alone, and the plan says why it is refused.
func undoFiles(l format.Loaded) (model.PlanFiles, error) {
	files := model.PlanFiles{History: path.Join(l.Dir, format.HistoryFile)}
	target, ok := lifecycle.UndoTarget(l.Book.History)
	if !ok {
		return files, nil
	}
	kind, key, _ := strings.Cut(target.Entity, ":")
	switch kind {
	case "rule":
		rules, err := format.RuleFile(l, key)
		if err != nil {
			return model.PlanFiles{}, &lifecycle.Refusal{Msg: err.Error(), Hint: "check the history with gragas history"}
		}
		files.Rules = rules
	case "term":
		files.Terms = path.Join(l.Dir, format.TermsFile)
	}
	return files, nil
}

// unranked is a plan function that reads nothing over the reading's ctx.
func unranked(plan func(model.Book, model.PlanFiles, lifecycle.Moment) (lifecycle.Plan, error)) planOf {
	return func(_ context.Context, b model.Book, f model.PlanFiles, m lifecycle.Moment) (lifecycle.Plan, error) {
		return plan(b, f, m)
	}
}
