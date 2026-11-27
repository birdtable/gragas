package coreapi

// journal restore: the lines that put a run's files back, printed and never
// run (cli.commands.25). It reads the journal and the tree and calls no
// Writer method, no Journal write and no tool.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The types below live in the model, so the kit can name them.
type (
	// RestoreAsk is the run to restore, as the model holds it.
	RestoreAsk = model.RestoreAsk
	// Moved is a file that does not hold what the run left, as the model holds it.
	Moved = model.Moved
	// Damaged is a file whose kept copy is damaged, as the model holds it.
	Damaged = model.Damaged
	// RestoreFile is one file a restore puts back, as the model holds it.
	RestoreFile = model.RestoreFile
	// Restoration is what a restore of one run says, as the model holds it.
	Restoration = model.Restoration
)

// Liner is argv as the one line a person pastes into a shell, or an error
// when one of its words is a value no line can carry (a NUL or a newline).
// The command tree hands it in, so this package names no shell and holds no
// quoting of its own.
type Liner func(argv []string) (string, error)

// ErrNoLine is a file whose way back no pasted line can carry: its path, or
// the path of its kept copy, holds a value the Liner refused. No line is
// said for any file then, as for a moved one.
var ErrNoLine = errors.New("no line a person pastes can carry it")

// ErrUnsafePath is a record that names a path under .git, or one that is not clean and inside the root. Nothing of it is read.
var ErrUnsafePath = errors.New("the record names a path no run writes")

// JournalRestore says how to put run's files back, each line made by line.
// When line gives model.ErrInWords, as it does for rm and cp in a shell
// other than POSIX (model.ErrOtherProgram) or in one with no safe quoting,
// Lines holds none and Files alone says each step: Lines are lines to type,
// never words a program would run.
// It is refused, by Moved holding the files, when a file's digest is not the
// one the run left. A run with no close is ErrRunOpen, a run the journal does
// not hold ErrNoRun, and a path line refuses ErrNoLine.
func JournalRestore(ctx context.Context, ports port.Ports, root model.Root, ask RestoreAsk, line Liner) (Restoration, error) {
	recs, _, err := ports.Journal.Records(ctx, root)
	if err != nil {
		return Restoration{}, fmt.Errorf("the journal could not be read: %w", err)
	}
	gs := groups(recs)
	i := slices.IndexFunc(gs, func(g *group) bool { return g.run.Run == ask.Run })
	switch {
	case i < 0:
		return Restoration{}, fmt.Errorf("%w: %s", ErrNoRun, ask.Run)
	case gs[i].shut == nil:
		return Restoration{}, fmt.Errorf("%w: %s", ErrRunOpen, ask.Run)
	}
	out := Restoration{Schema: JournalSchema, Run: ask.Run, Root: recs[0].Root, Files: []RestoreFile{}, Moved: []Moved{}, Damaged: []Damaged{}, Lines: []string{}}
	left := leftBy(gs[i].shut.Detail)
	for _, rec := range slices.Backward(gs[i].recs) {
		var cp copyDetail
		if rec.Kind != model.KindCopy || json.Unmarshal(rec.Detail, &cp) != nil || cp.Path == "" {
			continue
		}
		if err := vetPath(ports, line, cp.Path); err != nil {
			return Restoration{}, err
		}
		m, moved, err := movedSince(ports, root, cp, left, gs[i+1:])
		if err != nil {
			return Restoration{}, err
		}
		if moved {
			out.Moved = append(out.Moved, m)
			continue
		}
		f, argv, d, bad := wayBack(ports, root, ask, recs[0].Root, cp)
		if bad {
			out.Damaged = append(out.Damaged, d)
			continue
		}
		said, err := sayLine(line, argv, cp.Path)
		if err != nil {
			return Restoration{}, err
		}
		out.Files = append(out.Files, f)
		if said != "" {
			out.Lines = append(out.Lines, said)
		}
	}
	if len(out.Moved) > 0 || len(out.Damaged) > 0 {
		out.Files, out.Lines = []RestoreFile{}, []string{}
	}
	return out, nil
}

// movedSince is the file cp names when it does not hold what the run left, or
// what the run found if the run left no digest for it.
func movedSince(ports port.Ports, root model.Root, cp copyDetail, left map[string]string, later []*group) (Moved, bool, error) {
	now, err := digestNow(ports, root, cp.Path)
	if err != nil {
		return Moved{}, false, err
	}
	want, ok := left[cp.Path]
	if !ok {
		want = cp.Before
	}
	if now == want {
		return Moved{}, false, nil
	}
	return Moved{Path: cp.Path, Now: now, Expected: want, By: laterRuns(later, cp.Path)}, true, nil
}

// sayLine is argv as one line, or "" where the shell has no line for rm or cp
// (go-console#218): the file still says its step as data, and the command tree
// says it in words. A path no line can carry is refused; %q, since the path is
// the value the line refused, so it is printed escaped, never raw on the screen.
func sayLine(line Liner, argv []string, p string) (string, error) {
	said, err := line(argv)
	switch {
	case errors.Is(err, model.ErrInWords):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("%w: %q: %w", ErrNoLine, p, err)
	}
	return said, nil
}

// vetPath refuses a path a record names that no run writes: one under .git, or
// outside the root, before it is read or a line is said for it. A path no line
// can carry says that first, as it always has.
func vetPath(ports port.Ports, line Liner, p string) error {
	if apply.UnsafePath(ports.FileNames, p) == "" {
		return nil
	}
	if _, err := line([]string{"rm", "--", p}); err != nil && !errors.Is(err, model.ErrInWords) {
		return fmt.Errorf("%w: %q: %w", ErrNoLine, p, err)
	}
	return fmt.Errorf("%w: %q", ErrUnsafePath, p)
}

// wayBack is the file and the argv that put cp back: rm of a file the run
// made, cp of the kept copy over a file it changed. The file says the same as
// the argv as data, op, from and path, so a reader of --json needs no shell to
// read the line. bad is a kept copy that is damaged, and d says how; there is
// then no argv.
func wayBack(ports port.Ports, root model.Root, ask RestoreAsk, rootDigest string, cp copyDetail) (f RestoreFile, argv []string, d Damaged, bad bool) {
	if !cp.Existed {
		return RestoreFile{Path: cp.Path, Op: "remove", Digest: cp.Before}, []string{"rm", "--", cp.Path}, Damaged{}, false
	}
	// the journal port says where the copy lies, as the host spells the path
	from, err := ports.Journal.KeptAt(root, ask.Run, cp.Path)
	if err != nil {
		return RestoreFile{}, nil, Damaged{Path: cp.Path, Expected: cp.Before}, true
	}
	if d, bad := damaged(ports, root, ask.Run, cp, from); bad {
		return RestoreFile{}, nil, d, true
	}
	return RestoreFile{Path: cp.Path, Op: "restore", From: from, Digest: cp.Before}, []string{"cp", "--", from, cp.Path}, Damaged{}, false
}

// leftBy is the digest each file held when the run closed, by path.
func leftBy(detail []byte) map[string]string {
	left := map[string]string{}
	var c closeDetail
	if err := json.Unmarshal(detail, &c); err == nil {
		for _, f := range c.Files {
			left[f.Path] = f.After
		}
	}
	return left
}

// damaged reads the copy kept for cp, as the roll-back does, and says whether
// it is unreadable or does not hold the digest the copy record names. Kept is
// a read: restore still calls no Writer method.
func damaged(ports port.Ports, root model.Root, run string, cp copyDetail, from string) (Damaged, bool) {
	d := Damaged{Path: cp.Path, Copy: from, Expected: cp.Before}
	b, err := ports.Journal.Kept(root, run, cp.Path)
	if err != nil {
		return d, true
	}
	d.Got = string(apply.DigestBytes(b))
	return d, d.Got != cp.Before
}

// digestNow is the digest of the file at p, "" when it is not there.
func digestNow(ports port.Ports, root model.Root, p string) (string, error) {
	b, err := ports.Tree.Read(root, p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("%s could not be read: %w", p, err)
	}
	return string(apply.DigestBytes(b)), nil
}

// laterRuns are the runs of gs that kept a copy of p.
func laterRuns(gs []*group, p string) []string {
	by := []string{}
	for _, g := range gs {
		if slices.Contains(g.run.Files, p) {
			by = append(by, g.run.Run)
		}
	}
	return by
}
