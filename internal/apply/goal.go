package apply

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// goal is what the plan's data files must hold once it is applied: each
// one's digest after (cli.commands.22). A file replaced whole holds its new
// bytes; a file a line is added to holds what it holds now, read under the
// lock, with the line added. The record of the yes is not in it: its bytes
// hold the plan digest, which covers the goal, so holds checks the record
// by its line instead. A path that leaves the root is refused, naming it,
// before the intent.
func (r *run) goal(files []model.Target) (model.Goal, Outcome, bool) {
	var g model.Goal
	for _, t := range files {
		var old []byte
		if t.Append {
			b, err := r.p.Tree.Read(r.root, t.Path)
			switch {
			case errors.Is(err, port.ErrOutside):
				return model.Goal{}, r.refuse(fmt.Sprintf("%s leaves the root", t.Path), nil, r.s.Command), false
			case err != nil && !errors.Is(err, fs.ErrNotExist):
				return model.Goal{}, r.fail(FailRead, fmt.Errorf("%s could not be read: %w", t.Path, err)), false
			}
			old = b
		}
		if t.Delete {
			g.Files = append(g.Files, model.Input{Path: t.Path}) // no digest: the file is not there
			continue
		}
		g.Files = append(g.Files, model.Input{Path: t.Path, Digest: DigestBytes(expected(old, t))})
	}
	return g, Outcome{}, true
}

// holds is the goal predicate, run once every file is written: each file of
// the goal is read again from the tree and must have its digest, and the
// record of the yes must be the last line of its file, or the whole file
// when it is replaced, with what the file held before it unchanged. It reads
// the tree, never the plan again: an empty re-plan would test the plan
// function, not the tree. A predicate that fails names the file.
func (r *run) holds() error {
	for _, g := range r.plan.Goal.Files {
		got, err := r.p.Tree.Read(r.root, g.Path)
		if g.Digest == "" { // a deleted file: the goal is that it is not there
			if !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("%s is still there, and the plan's goal is that it is not", g.Path)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("%s could not be read again for the goal: %w", g.Path, err)
		}
		if DigestBytes(got) != g.Digest {
			return fmt.Errorf("%s does not hold the plan's goal once every file is written", g.Path)
		}
	}
	rec := r.plan.Record
	if rec == nil {
		return nil
	}
	got, err := r.p.Tree.Read(r.root, rec.Path)
	if err != nil {
		return fmt.Errorf("%s could not be read again for the goal: %w", rec.Path, err)
	}
	if rec.Append && !endsWithLines(string(got), string(rec.Data)) {
		return fmt.Errorf("the last line of %s is not the record of the yes", rec.Path)
	}
	if DigestBytes(got) != DigestBytes(expected(r.seen[rec.Path].data, *rec)) {
		return fmt.Errorf("%s does not hold the record of the yes after what it held before", rec.Path)
	}
	return nil
}

// endsWithLines is whether s ends with the lines of a record, each whole and
// the last ended by its newline: a torn line is no record.
func endsWithLines(s, lines string) bool {
	want := lines + "\n"
	return strings.HasSuffix(s, want) && (len(s) == len(want) || s[len(s)-len(want)-1] == '\n')
}
