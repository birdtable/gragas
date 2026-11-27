package coreapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// presetsDir is the folder that holds the presets generated from a rulebook's
// bindings, one folder a tool. It is read from the root of the repository, the
// folder that holds the rulebook's own folder, and not from under the book's
// folder: projects import the presets by that path (architecture, the layout
// of a rulebook).
const presetsDir = "presets"

// presetChange is what a plan does to the generated presets: the lines of the
// plan's tool-entries section, the preset files it writes with their targets,
// and the paths the scope of the write must hold.
type presetChange struct {
	standing, proof []string
	lines           []string
	// unserved are the tools the change binds that no installed plugin writes a
	// preset for: the record is written and their presets are not.
	unserved []string
	written  []string
	targets  []model.Target
}

// presetsOf is the preset change of plan pl over the book l. The book after
// the change is read back from the plan's own rendered files through an
// overlay, so the presets are generated from what the change would write and
// not from a second account of it. A plan with no change, or no adapter table,
// changes no preset.
func presetsOf(ctx context.Context, p port.Ports, root model.Root, l format.Loaded, names *format.Names, pl lifecycle.Plan) (_ presetChange, err error) {
	if !touchesRules(pl.Delta) {
		return presetChange{}, nil
	}
	// A tool an installed plugin serves is written by the plugin, here as in
	// gragas preset (generatePresets); a tool none serves has no preset to
	// change, and the plan says so by name.
	served, err := presetPlugins(ctx, p, root, l.Book.Identity.Tested)
	if err != nil {
		return presetChange{}, err
	}
	defer func() {
		if cerr := closePresetPlugins(served); cerr != nil && err == nil {
			err = cerr
		}
	}()
	after, err := bookAfter(p, root, l, names, pl)
	if err != nil {
		return presetChange{}, err
	}
	return presetsServedBy(ctx, p, root, l.Book, after, served)
}

// bookAfter is the book after plan pl, read back from the plan's own rendered
// files through an overlay.
func bookAfter(p port.Ports, root model.Root, l format.Loaded, names *format.Names, pl lifecycle.Plan) (model.Book, error) {
	rendered, err := format.Render(p.Tree, p.Schema, root, l, pl.Delta)
	if err != nil {
		return model.Book{}, err
	}
	files := map[string][]byte{}
	for _, t := range rendered {
		if !t.Append {
			files[t.Path] = t.Data
		}
	}
	after, err := format.LoadWith(overlay{Tree: p.Tree, files: files}, p.Schema, root, format.Options{Names: names})
	if err != nil {
		return model.Book{}, fmt.Errorf("the book after the change could not be read to generate its presets: %w", err)
	}
	return after.Book, nil
}

// presetsServedBy is presetsOf's change once the plugins that serve a tool are
// open: the presets of the book before and after made by generatePresets.
func presetsServedBy(ctx context.Context, p port.Ports, root model.Root, was, now model.Book, served map[string]port.Plugin) (presetChange, error) {
	var out presetChange
	out.standing, out.proof = standingOf(was, now)
	out.unserved = unservedByChange(was, now, served)
	for _, tool := range out.unserved {
		out.lines = append(out.lines, tool+": no installed plugin writes its preset, so none is written; install one with `gragas plugin add <folder>`")
	}
	before, err := generatePresets(ctx, p, root, was, served)
	if err != nil {
		return presetChange{}, err
	}
	next, err := generatePresets(ctx, p, root, now, served)
	if err != nil {
		return presetChange{}, err
	}
	for _, n := range next.Notes {
		if !slices.Contains(before.Notes, n) {
			out.lines = append(out.lines, n)
		}
	}
	if err := out.diffs(p, root, before, next); err != nil {
		return presetChange{}, err
	}
	return out, nil
}

// unservedByChange is, in canon order, the tools of the bindings the change adds
// that no plugin of served writes a preset for.
func unservedByChange(was, now model.Book, served map[string]port.Plugin) []string {
	have := map[model.BindingRef]bool{}
	for _, r := range was.Rules {
		for _, b := range r.Bindings {
			have[b.Ref(r.ID)] = true
		}
	}
	added := map[string]bool{}
	for _, r := range now.Rules {
		for _, b := range r.Bindings {
			if !have[b.Ref(r.ID)] {
				added[b.Tool] = true
			}
		}
	}
	var out []string
	for _, tool := range link.Unserved(now, func(tool string) bool { _, ok := served[tool]; return ok }) {
		if added[tool] {
			out = append(out, tool)
		}
	}
	return out
}

// diffs adds a line for each preset file the change writes, adds or removes,
// and for each one written, its path and the bytes it will hold, with what the
// file holds now as its digest.
func (out *presetChange) diffs(p port.Ports, root model.Root, before, next link.Generated) error {
	byPath := map[string]link.PresetFile{}
	for _, f := range next.Files {
		byPath[path.Join(f.Tool, f.File.Path)] = f
	}
	beforeBy := map[string]link.PresetFile{}
	for _, f := range before.Files {
		beforeBy[path.Join(f.Tool, f.File.Path)] = f
	}
	for _, c := range link.DiffPresets(before, next) {
		key := path.Join(c.Tool, c.Path)
		dest := path.Join(presetsDir, key)
		out.lines = append(out.lines, fmt.Sprintf("%s: %s", dest, c.Kind))
		for _, s := range c.Removed {
			out.lines = append(out.lines, "  - "+s)
		}
		for _, s := range c.Added {
			out.lines = append(out.lines, "  + "+s)
		}
		if c.Kind == link.FileRemoved {
			if err := out.removal(p, root, dest, beforeBy[key]); err != nil {
				return err
			}
			continue
		}
		var was model.Digest
		switch b, err := p.Tree.Read(root, dest); {
		case err == nil:
			if handEdited(b, beforeBy[key], byPath[key]) {
				out.lines = append(out.lines, "  (hand-edited: left as it is, never overwritten; delete it by hand to take the generated file)")
				continue
			}
			was = apply.DigestBytes(b)
		case !errors.Is(err, fs.ErrNotExist):
			return err
		}
		out.written = append(out.written, dest)
		out.targets = append(out.targets, model.Target{Path: dest, Before: was, Data: byPath[key].File.Data})
	}
	return nil
}

// removal is what the plan does with a preset file the new render no longer
// makes: delete it when it holds the bytes gragas rendered before the change,
// byte for byte, and keep it and say so when it holds anything else, since a
// file someone edited is theirs. A file that is not there needs nothing.
func (out *presetChange) removal(p port.Ports, root model.Root, dest string, was link.PresetFile) error {
	b, err := p.Tree.Read(root, dest)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		out.lines = append(out.lines, "  (the file is not there)")
	case err != nil:
		return err
	case !bytes.Equal(b, was.File.Data):
		out.lines = append(out.lines, "  (hand-edited: left as it is, never deleted; delete it by hand to drop it)")
	default:
		out.lines = append(out.lines, "  (the file is deleted: it is the one gragas wrote)")
		out.written = append(out.written, dest)
		out.targets = append(out.targets, model.Target{Path: dest, Before: apply.DigestBytes(b), Delete: true})
	}
	return nil
}

// touchesRules says whether the delta changes a rule or an exclusion, the only
// entities a preset is generated from; a term or a source plan changes none
// and shows no tool entries.
func touchesRules(d model.Delta) bool {
	for _, c := range d.Changes {
		if strings.HasPrefix(c.Entity, "rule:") || strings.HasPrefix(c.Entity, "exclusion:") {
			return true
		}
	}
	return false
}

// standingOf is what the plan does to the standing of the rules whose
// standing or bindings it changes: one line for each standing that moves, and
// the proof to run for each binding the change adds, all not run (the grid is
// the proof's, and no cell is recorded before the change). A book whose
// standing cannot be derived says no standing, as the change is not the place
// to report why.
func standingOf(before, after model.Book) (standing, proof []string) {
	was, err := link.Derive(before, nil, model.Chain{})
	if err != nil {
		return nil, nil
	}
	now, err := link.Derive(after, nil, model.Chain{})
	if err != nil {
		return nil, nil
	}
	held := map[model.RuleID]model.Held{}
	for _, h := range was {
		held[h.Rule] = h
	}
	have := map[model.BindingRef]bool{}
	for _, r := range before.Rules {
		for _, b := range r.Bindings {
			have[b.Ref(r.ID)] = true
		}
	}
	for _, h := range now {
		if old, ok := held[h.Rule]; ok && old.String() != h.String() {
			standing = append(standing, fmt.Sprintf("%s: %s before, %s after", h.Rule, old, h))
		}
	}
	for _, r := range after.Rules {
		for _, b := range r.Bindings {
			if !have[b.Ref(r.ID)] {
				proof = append(proof, fmt.Sprintf("%s at %s %s %s: not run", r.ID, b.Tool, b.Target, b.Key))
			}
		}
	}
	return standing, proof
}

// handEdited says whether the file on disk is neither what the book before the
// change generates nor what the book after it does: someone wrote it, and a
// plan lists it and leaves it.
func handEdited(now []byte, before, next link.PresetFile) bool {
	return !bytes.Equal(now, before.File.Data) && !bytes.Equal(now, next.File.Data)
}
