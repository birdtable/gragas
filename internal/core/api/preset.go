package coreapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/rendered"
)

// ErrSettingsInputMissing is what the error of a preset's inputs matches when a
// plugin's manifest names an input and the rulebook has no such file: the tool
// is named in the error, and nothing is generated.
var ErrSettingsInputMissing = errors.New("the plugin's manifest names a preset input and the rulebook has none")

// ErrNoPresets is what a render of the rulebook's presets answers when no tool
// has a preset file to write: the book binds nothing a preset holds. It is the
// compare's ErrNoFile, so a check over no file is not ready and never green.
var ErrNoPresets = rendered.ErrNoFile

// A PresetFile is one preset file of the rulebook as the book makes it: where
// it goes below the root, its bytes, whether a project's copy is its own after
// the first write, and the mark the file opens with when the plugin's manifest
// asks for one.
type PresetFile struct {
	Tool     string
	Path     string
	Data     []byte
	CopyOnce bool
	Mark     string
}

// owned is whether have, the copy of f in the tree, is a project's own: a
// copy-once file whose first line does not hold the mark. While a copy
// carries the mark it is the plugin's and is held to the book's bytes,
// whatever the manifest's extends says; the project takes it by removing the
// mark. The mark counts on the first line alone, where the plugin writes it,
// so a later line that quotes it does not hold the file.
func (f PresetFile) owned(have []byte) bool {
	first, _, _ := bytes.Cut(have, []byte("\n"))
	return f.CopyOnce && (f.Mark == "" || !bytes.Contains(first, []byte(f.Mark)))
}

// presetsMade is what presetFiles answers: the rulebook's folder, every
// preset file the book makes, and the tools the identity lists as
// hand-written, whose presets no plugin writes and nothing judges.
type presetsMade struct {
	dir         string
	files       []PresetFile
	handWritten []string
}

// presetFiles is every tool's preset file as the book at root generates it,
// each written by the Preset of the plugin that serves the tool. A tool the
// identity lists in hand_written_presets is not written and not judged, and is
// answered by name; a book that binds a tool no plugin serves and the list
// leaves out is refused, naming the tool and how to install a plugin of it.
func presetFiles(ctx context.Context, p port.Ports, root model.Root) (made presetsMade, err error) {
	in, err := readBook(ctx, p, root)
	if err != nil {
		return presetsMade{}, err
	}
	if !in.hasBook {
		return presetsMade{}, ErrNoBook
	}
	if n := len(in.rep.Findings); n > 0 {
		return presetsMade{}, &FindingsError{Count: n}
	}
	served, err := presetPlugins(ctx, p, root, in.rep.Book.Identity.Tested)
	if err != nil {
		return presetsMade{}, err
	}
	defer func() {
		if cerr := closePresetPlugins(served); cerr != nil && err == nil {
			made, err = presetsMade{}, cerr
		}
	}()
	listed := in.rep.Book.Identity.HandWrittenPresets
	var missing []string
	for _, tool := range link.Unserved(in.rep.Book, func(tool string) bool { _, ok := served[tool]; return ok }) {
		if !slices.Contains(listed, tool) {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		return presetsMade{}, noPresetPlugin(missing)
	}
	gen, err := generatePresets(ctx, p, root, in.rep.Book, served)
	if err != nil {
		return presetsMade{}, err
	}
	made.dir = in.rep.Dir
	for _, tool := range listed {
		// a listed tool a plugin serves is a finding of the load, so the
		// run ended above; this keeps the answer to the tools nothing writes
		if _, ok := served[tool]; !ok {
			made.handWritten = append(made.handWritten, tool)
		}
	}
	slices.Sort(made.handWritten)
	for _, f := range gen.Files {
		made.files = append(made.files, PresetFile{Tool: f.Tool, Path: path.Join(presetsDir, f.Tool, f.File.Path), Data: f.File.Data, CopyOnce: f.CopyOnce, Mark: f.Mark})
	}
	return made, nil
}

// Preset is the change that writes every tool's preset file of the rulebook at
// ask.Root from its bindings: the dry run lists each file that is new or
// differs, and --apply writes them through the one write path. The rulebook's
// own preset is written again on every run; a file a project copied once
// (CopyOnce) is written when it is not there, and is never overwritten once
// the project has taken it by removing the mark; while it carries the mark it
// is written again like any other.
func Preset(ctx context.Context, p port.Ports, ask model.ChangeAsk) (*Change, *model.Refusal, error) {
	first, err := presetPlan(ctx, p, ask.Root)
	if err != nil {
		return nil, nil, err
	}
	var cur presetPlanned
	spec := Spec{
		Command: ask.Command, Applies: ask.Applies, Engine: ask.Engine, Run: ask.Run,
		Scope: first.paths,
		Plan: func(ctx context.Context) (model.Plan, error) {
			m, err := presetPlan(ctx, p, ask.Root)
			if err != nil {
				return model.Plan{}, err
			}
			cur = m
			return model.Plan{Command: ask.Command, Delta: m.delta, Sections: m.sections, Inputs: m.inputs}, nil
		},
		Render: func(model.Delta) ([]model.Target, error) { return cur.targets, nil },
	}
	return NewChange(p, ask.Root, spec), nil, nil
}

// presetPlanned is what a read of the tree decides: the targets to write, the
// delta and the sections that say so, and the inputs read.
type presetPlanned struct {
	targets  []model.Target
	paths    []string
	delta    model.Delta
	sections []model.Section
	inputs   []model.Input
}

func presetPlan(ctx context.Context, p port.Ports, root model.Root) (presetPlanned, error) {
	made, err := presetFiles(ctx, p, root)
	if err != nil {
		return presetPlanned{}, err
	}
	var out presetPlanned
	var lines []string
	for _, f := range made.files {
		have, err := p.Tree.Read(root, f.Path)
		in := model.Input{Path: f.Path}
		switch {
		case err == nil:
			in.Digest = apply.DigestBytes(have)
			if f.owned(have) || bytes.Equal(have, f.Data) {
				out.inputs = append(out.inputs, in)
				continue
			}
			lines = append(lines, f.Path+": differs")
		case errors.Is(err, fs.ErrNotExist):
			lines = append(lines, f.Path+": new")
		default:
			return presetPlanned{}, fmt.Errorf("%s could not be read: %w", f.Path, err)
		}
		out.inputs = append(out.inputs, in)
		out.paths = append(out.paths, f.Path)
		out.targets = append(out.targets, model.Target{Path: f.Path, Before: in.Digest, Data: f.Data})
		out.delta.Changes = append(out.delta.Changes, model.Change{Entity: "file:" + f.Path, Field: "content", After: "the preset of " + f.Tool})
	}
	if len(lines) == 0 {
		lines = []string{"nothing to write: every preset file is what the book makes"}
	}
	out.sections = []model.Section{{Title: "Preset files", Lines: lines}}
	return out, nil
}

// A PresetState is the answer of gragas preset under --json and the lines of
// its screen: each file with whether it was, or would be, written, is the same
// as the book makes, or differs.
type PresetState string

const (
	// PresetWritten is a file the run wrote: the root sets it only after the
	// change ended done. A dry run reports the file as differing.
	PresetWritten PresetState = "written"
	// PresetSame is a file already what the book makes.
	PresetSame PresetState = "same"
	// PresetDiffers is a file that is not what the book makes, or is not there.
	PresetDiffers PresetState = "differs"
)

// A PresetFileState is one file of the answer. First says, of a file that
// differs, where: that it is not there, or the first line that is not what the
// book makes.
type PresetFileState struct {
	Path  string      `json:"path"`
	Tool  string      `json:"tool"`
	State PresetState `json:"state"`
	First string      `json:"first,omitempty"`
}

// A PresetReport is what gragas preset answers with under --json: every file
// compared, how many that is, the files the project owns, and the tools the
// identity lists as hand-written. Neither of the last two is compared.
type PresetReport struct {
	Files       []PresetFileState `json:"files"`
	Compared    int               `json:"compared"`
	Owned       []string          `json:"owned,omitempty"`
	HandWritten []string          `json:"hand_written,omitempty"`
}

// PresetCheck compares each file the book makes with the tree and answers each
// as same or differs, writing nothing. A copy a project owns that is there is
// not compared and not counted, and is named by its path in Owned: a copy-once
// file whose mark the project removed, whatever it holds (a file never
// compared is not one of the files compared). A copy that carries the mark is
// held byte for byte, whatever the manifest's extends says. The tools of hand_written_presets are named and not compared. A book
// that makes no file is ErrNoPresets: the check has nothing to compare and
// says so, never green.
func PresetCheck(ctx context.Context, p port.Ports, root model.Root) (PresetReport, error) {
	if err := ctx.Err(); err != nil {
		return PresetReport{}, err
	}
	made, err := presetFiles(ctx, p, root)
	if err != nil {
		return PresetReport{}, err
	}
	by := map[string]PresetFile{}
	have := map[string][]byte{}
	owned := map[string]bool{}
	var rf []rendered.File
	for _, f := range made.files {
		by[f.Path] = f
		rf = append(rf, rendered.File{Path: f.Path, Data: f.Data})
	}
	differ, err := rendered.Compare(rf, func(rel string) ([]byte, error) {
		b, err := p.Tree.Read(root, rel)
		if err != nil {
			return b, err
		}
		have[rel] = b
		if by[rel].owned(b) {
			// the project's: answered as the book's bytes so the compare
			// passes it by, and named below, not counted
			owned[rel] = true
			return by[rel].Data, nil
		}
		return b, nil
	})
	if err != nil {
		return PresetReport{}, err
	}
	// files is never null under --json, though every file be the project's
	out := PresetReport{Files: []PresetFileState{}, HandWritten: made.handWritten}
	for _, f := range made.files {
		if owned[f.Path] {
			out.Owned = append(out.Owned, f.Path)
			continue
		}
		out.Compared++
		st := PresetFileState{Path: f.Path, Tool: f.Tool, State: PresetSame}
		if slices.Contains(differ, f.Path) {
			st.State, st.First = PresetDiffers, "it is not there"
			if b, ok := have[f.Path]; ok {
				st.First = firstDifference(b, f.Data)
			}
		}
		out.Files = append(out.Files, st)
	}
	return out, nil
}
