package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Lines are the two lines a plan command is made with (go-console#218).
// Command is the command as the plan, its digest, the journal and the history
// hold it: written for POSIX in every shell, so the record does not move with
// the shell it was made in. Applies is the same with --apply, as the run
// prints it to type, in the run's shell: the line a run that finds this one
// interrupted names. Who is what the yes records as who made it: the
// command's own name, such as "gragas migrate", and never a person's: a
// rulebook is shared and published.
type Lines struct {
	Command, Applies, Who string
}

// MigrateRun names the run a migrate is part of: its two lines, the id of
// the run and the engine's version.
type MigrateRun struct {
	Lines
	Run, Engine string
}

// Migrate is the migrate command: the change that moves a schema v2 rulebook
// at root to the format. It plans from the files of v2 and reads the result
// back, through the loader, before anything is written: the rules and sources
// it loads must equal the count carried and the result must hold no finding.
// A rulebook already in the format is a plan of nothing: done, nothing to do.
// report receives the migration of each plan made, for the screen that shows it.
func Migrate(p port.Ports, root model.Root, f model.InitForm, r MigrateRun, report func(Migration)) (*Change, error) {
	if problem := format.MigrateProblem(f); problem != "" {
		return nil, errors.New(problem)
	}
	m := &migrator{p: p, root: root, form: f, command: r.Command, who: r.Who, report: report}
	return NewChange(p, root, Spec{
		Command: r.Command, Applies: r.Applies, Engine: r.Engine, Run: r.Run,
		Scope:  []string{"*.yaml", "*/*.yaml", "*/*/rules/*.yaml", "*/*/*/rules/*.yaml", "history.jsonl", "*/history.jsonl"},
		Plan:   m.plan,
		Render: func(model.Delta) ([]model.Target, error) { return m.targets, nil },
		Record: func(y model.Yes) (model.Target, error) {
			return format.HistoryRecord(p.Tree, p.Schema, root, format.Loaded{Dir: m.dir}, format.MigrateEntry(f, m.statements))(y)
		},
	}), nil
}

// A migrator is one migrate's state between its plan and its write.
type migrator struct {
	p       port.Ports
	root    model.Root
	form    model.InitForm
	command string
	who     string
	report  func(Migration)
	dir     string
	targets []model.Target
	// statements is the digest of each statement the plan writes, by rule id,
	// which the history entry records.
	statements map[model.RuleID]model.Digest
}

func (m *migrator) plan(ctx context.Context) (model.Plan, error) {
	m.targets, m.statements = nil, nil
	files, err := m.p.Tree.List(m.root, "", model.Every)
	if err != nil {
		return model.Plan{}, fmt.Errorf("the rulebook could not be listed: %w", err)
	}
	found, all := format.V2Folder(files)
	who, err := recorded(m.who)
	if err != nil {
		return model.Plan{}, err
	}
	// The record is asked for on every plan, an empty one included.
	pl := model.Plan{Yes: &model.Yes{Who: who, At: m.p.Clock.Now().Unix(), Command: m.command}}
	if found == "-" {
		return pl, nil
	}
	m.dir = found
	v2, inputs, err := m.read(all)
	if err != nil {
		return model.Plan{}, err
	}
	pl.Inputs = inputs
	mig, err := format.MigrateV2(v2, m.form)
	if err != nil {
		return model.Plan{}, err
	}
	m.statements = mig.Statements
	for i := range mig.Targets {
		mig.Targets[i].Path = format.InFolder(m.dir, mig.Targets[i].Path)
	}
	mig.Parked.Path = format.InFolder(m.dir, mig.Parked.Path)
	prose, held, err := readBack(ctx, m.p, m.root, mig)
	if err != nil {
		return model.Plan{}, err
	}
	if pl.Delta.Changes, err = m.changed(mig.Targets); err != nil {
		return model.Plan{}, err
	}
	doc := migrationOf(mig, prose)
	if m.report != nil {
		m.report(doc)
	}
	pl.Sections = []model.Section{
		{Title: "what is carried over", Lines: MigrateLines(doc)},
		{Title: "checked before any write", Lines: []string{
			"the result was read back through the loader, with the statement digests the conversion built: counts equal, no finding but the prose lint's",
			model.Plural(held, "migrated rule") + " held from the prose lint until " + its(held) + " statement changes",
		}},
	}
	return pl, nil
}

// read is the files of v2 in the folder, and each one as an input of the plan.
func (m *migrator) read(all []string) (format.V2, []model.Input, error) {
	v2 := format.V2{}
	var inputs []model.Input
	for _, path := range format.V2Files(all) {
		full := format.InFolder(m.dir, path)
		b, err := m.p.Tree.Read(m.root, full)
		if err != nil {
			return nil, nil, fmt.Errorf("%s could not be read: %w", path, err)
		}
		v2[path] = b
		inputs = append(inputs, model.Input{Path: full, Digest: apply.DigestBytes(b)})
	}
	return v2, inputs, nil
}

// changed keeps in m.targets each file whose bytes differ from the tree's, with
// the digest of what it replaces, and says what changes.
func (m *migrator) changed(all []model.Target) ([]model.Change, error) {
	var out []model.Change
	for _, t := range all {
		have, err := m.p.Tree.Read(m.root, t.Path)
		switch {
		case err == nil && string(have) == string(t.Data):
			continue
		case err == nil:
			t.Before = apply.DigestBytes(have)
		case !errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("%s could not be read: %w", t.Path, err)
		}
		m.targets = append(m.targets, t)
		out = append(out, model.Change{Entity: "file:" + t.Path, Field: "content", After: "the format's form of " + t.Path})
	}
	return out, nil
}

// its is the possessive of n migrated rules.
func its(n int) string {
	if n == 1 {
		return "its"
	}
	return "their"
}

// readBack loads what the plan would write, without writing it, and holds it
// to the plan: as many rules and sources as the plan carries, and no finding
// but the prose lint's on a statement the lint does not hold. The load is
// handed the digests the conversion has just built, since the history line
// that records them is not written yet, so the hold applies here as it will
// to the written rulebook and nothing turns the lint off. It answers how many
// prose findings the result holds and how many rules the lint holds.
func readBack(ctx context.Context, p port.Ports, root model.Root, m format.Migration) (prose, held int, err error) {
	over := overlay{Tree: p.Tree, files: map[string][]byte{}}
	for _, t := range m.Targets {
		over.files[t.Path] = t.Data
	}
	q := p
	q.Tree = over
	r, err := loadBookWith(ctx, q, root, "", m.Statements)
	if err != nil {
		return 0, 0, fmt.Errorf("the migrated rulebook could not be read back: %w", err)
	}
	for i, f := range r.Findings {
		if i < len(r.prose) && r.prose[i] {
			prose++
			continue
		}
		// The registry row of migrate (spec 15.1) declares this refusal as not
		// ready, so the plan says so with the typed error; apply maps it.
		return 0, 0, &apply.NotReadyError{Reason: fmt.Sprintf("the migrated rulebook does not read back clean, and nothing was written: %s: %s", f.Entity, f.Msg)}
	}
	if got := len(r.Book.Rules); got != m.Carried.Rules {
		return 0, 0, fmt.Errorf("the migrated rulebook holds %d rules and %d were carried, and nothing was written", got, m.Carried.Rules)
	}
	if got := len(r.Book.Sources); got != m.Carried.Sources {
		return 0, 0, fmt.Errorf("the migrated rulebook holds %d sources and %d were carried, and nothing was written", got, m.Carried.Sources)
	}
	return prose, r.Held, nil
}

// overlay is a Tree that reads the files of a plan in place of the ones on
// disk, so the plan is loaded before it is written. A file the plan does not
// name is read from the tree.
type overlay struct {
	port.Tree
	files map[string][]byte
}

func (o overlay) Read(root model.Root, path string) ([]byte, error) {
	if b, ok := o.files[path]; ok {
		return b, nil
	}
	return o.Tree.Read(root, path)
}

func (o overlay) Stat(root model.Root, path string) (model.Entry, error) {
	if b, ok := o.files[path]; ok {
		return model.Entry{Path: path, Size: int64(len(b))}, nil
	}
	return o.Tree.Stat(root, path)
}

func (o overlay) List(root model.Root, dir string, scope model.Scope) ([]model.Entry, error) {
	have, err := o.Tree.List(root, dir, scope)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, e := range have {
		seen[e.Path] = true
	}
	for path, b := range o.files {
		if _, in := format.Below(dir, path); in && !seen[path] {
			have = append(have, model.Entry{Path: path, Size: int64(len(b))})
		}
	}
	slices.SortFunc(have, func(a, b model.Entry) int { return compare(a.Path, b.Path) })
	return have, nil
}

func compare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
