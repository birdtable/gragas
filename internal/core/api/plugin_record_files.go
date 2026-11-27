package coreapi

// The files and the preview of a recording: the case name, the
// dry run's plan and the change that writes the case and the raw output.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/core/wire"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

func recordLine(ask PluginRecordAsk) string {
	var line strings.Builder
	line.WriteString("gragas plugin record " + ask.Dir + " --op " + ask.Op)
	for _, c := range ask.Config {
		line.WriteString(" --config " + c)
	}
	for _, f := range ask.Fixtures {
		line.WriteString(" --fixture " + f)
	}
	line.WriteString(" --apply")
	return line.String()
}

// newRefusal is a refusal that is never an error of its own: the words are
// ours and the hint is never empty.
func newRefusal(w model.Word, observed, hint string) *model.Refusal {
	r, err := model.NewRefusal(w, observed, nil, hint)
	if err != nil {
		// A word that refuses and a hint are ours to give: this is the fallback of
		// a mistake in the call, and it still ends the run on a line to type.
		r, err = model.NewRefusal(model.Refused, observed+" ("+err.Error()+")", nil, "gragas plugin record --help")
		if err != nil {
			return &r
		}
	}
	return &r
}

// refuseOr answers an error of the rounds or of the runner as the refusal it
// is: a program the plugin is not granted and a plugin's refusal are refused,
// a tool or level this machine lacks is not ready; any other error is nil and
// the caller's error stands.
func refuseOr(err error, ask PluginRecordAsk, place recordPlace, what string) *model.Refusal {
	switch {
	case errors.Is(err, port.ErrGrant):
		return newRefusal(model.Refused, what+": "+err.Error(), recordLine(ask))
	case errors.Is(err, port.ErrStepNotReady), errors.Is(err, plugins.ErrNotReady):
		return newRefusal(model.NotReady, what+": "+err.Error(), recordLine(ask))
	case errors.Is(err, plugins.ErrRefused):
		return newRefusal(model.Refused, what+": "+err.Error(), recordLine(ask))
	}
	return newRefusal(model.Refused, what+": "+err.Error(), recordLine(ask))
}

// caseName is the name of the case folder: the number, a dash and the op.
func caseName(p port.Ports, ask PluginRecordAsk, place recordPlace) (string, error) {
	if ask.Case != "" {
		if _, err := strconv.Atoi(ask.Case); err != nil {
			return "", fmt.Errorf("--case names %q: say a whole number", ask.Case)
		}
		return ask.Case + "-" + ask.Op, nil
	}
	listed, err := p.Tree.List(place.root, "testdata/cases", model.Every)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("testdata/cases of %s could not be listed: %w", place.dir, err)
	}
	next := 1
	for _, e := range listed {
		rel := strings.TrimPrefix(e.Path, "testdata/cases/")
		name, _, _ := strings.Cut(rel, "/")
		head, _, _ := strings.Cut(name, "-")
		if n, err := strconv.Atoi(head); err == nil && n >= next {
			next = n + 1
		}
	}
	return strconv.Itoa(next) + "-" + ask.Op, nil
}

// recordPreview is the dry run: the steps of the first plan and the files the
// recording would write. A round after the first is asked for once the first's
// results are in, so its steps and files are not named here.
func recordPreview(p port.Ports, ask PluginRecordAsk, place recordPlace, tool string, first plugin.PlanResponse) *Change {
	name, nameErr := caseName(p, ask, place)
	var cur []model.Target
	var lines []string
	spec := Spec{
		Command: ask.Ask.Command, Applies: ask.Ask.Applies, Engine: ask.Ask.Engine, Run: ask.Ask.Run,
		Scope: []string{"testdata/*/*", "testdata/*/*/*"},
		Plan: func(context.Context) (model.Plan, error) {
			if nameErr != nil {
				return model.Plan{}, nameErr
			}
			cur, lines = nil, nil
			var delta model.Delta
			at := func(rel string) string { return path.Join("testdata", rel) }
			add := func(rel string) {
				cur = append(cur, model.Target{Path: at(rel)})
				delta.Changes = append(delta.Changes, model.Change{Entity: "file:" + at(rel), After: "written"})
			}
			for i, s := range first.Steps {
				switch {
				case s.Run != nil:
					lines = append(lines, fmt.Sprintf("round 0, step %d runs %s %s", i, s.Run.Program, strings.Join(s.Run.Argv, " ")))
					add(fmt.Sprintf("recordings/%s/0-%d.stdout", tool, i))
				case s.Read != nil:
					lines = append(lines, fmt.Sprintf("round 0, step %d reads %s", i, s.Read.Glob))
				}
			}
			if len(first.Steps) == 0 {
				lines = append(lines, "round 0 asks for no step: the config was enough")
			}
			if !first.Final {
				lines = append(lines, "the rounds after the first are asked for when the first's results are in")
			}
			add("cases/" + name + "/request.json")
			add("cases/" + name + "/response.json")
			lines = append(lines, "the tool is "+tool+": its version was asked, and nothing else was run")
			return model.Plan{Command: ask.Ask.Command, Delta: delta, Sections: []model.Section{{Title: "Record " + place.ref.Name + "@" + place.ref.Version, Lines: lines}}}, nil
		},
		Render: func(model.Delta) ([]model.Target, error) { return cur, nil },
	}
	return NewChange(p, place.root, spec)
}

// recordChange is the change that writes what rec holds: the raw output of the
// steps under testdata/recordings/<tool version>, and the case.
func recordChange(p port.Ports, ask PluginRecordAsk, place recordPlace, tool string, rec plugins.Recorded) (*Change, *model.Refusal, error) {
	name, err := caseName(p, ask, place)
	if err != nil {
		return nil, nil, err
	}
	raw, err := plugins.Raw(rec.Results)
	if err != nil {
		return nil, nil, err
	}
	req, err := wire.New(p.FileNames).EncodeRequest(wire.NewRequest(rec.Op, rec.Request))
	if err != nil {
		return nil, nil, fmt.Errorf("the request of the case could not be made: %w", err)
	}
	resp, err := canon.Encode(rec.Response.ToCanon(), canon.Canonical)
	if err != nil {
		return nil, nil, fmt.Errorf("the answer of the case could not be made: %w", err)
	}
	files := []model.File{
		{Path: "testdata/cases/" + name + "/request.json", Data: req},
		{Path: "testdata/cases/" + name + "/response.json", Data: resp},
	}
	for _, f := range raw {
		files = append(files, model.File{Path: "testdata/recordings/" + tool + "/" + f.Path, Data: f.Data})
	}
	var scope []string
	for _, f := range files {
		scope = append(scope, f.Path)
	}
	var cur []model.Target
	spec := Spec{
		Command: ask.Ask.Command, Applies: ask.Ask.Applies, Engine: ask.Ask.Engine, Run: ask.Ask.Run,
		Scope: scope,
		Plan: func(context.Context) (model.Plan, error) {
			cur = nil
			var delta model.Delta
			var inputs []model.Input
			var lines []string
			for _, f := range files {
				at := f.Path
				before, err := p.Tree.Read(place.root, at)
				var b model.Digest
				switch {
				case err == nil:
					b = apply.DigestBytes(before)
				case errors.Is(err, fs.ErrNotExist):
				default:
					return model.Plan{}, fmt.Errorf("%s could not be read: %w", at, err)
				}
				inputs = append(inputs, model.Input{Path: at, Digest: b})
				if b == apply.DigestBytes(f.Data) {
					continue
				}
				cur = append(cur, model.Target{Path: at, Before: b, Data: f.Data})
				delta.Changes = append(delta.Changes, model.Change{Entity: "file:" + at, After: "written"})
				lines = append(lines, at)
			}
			if len(lines) == 0 {
				lines = []string{"nothing to write: the files are what the tool printed"}
			}
			return model.Plan{Command: ask.Ask.Command, Delta: delta, Sections: []model.Section{{Title: "Recorded " + place.ref.Name + "@" + place.ref.Version + " at " + tool, Lines: lines}}, Inputs: inputs}, nil
		},
		Render: func(model.Delta) ([]model.Target, error) { return cur, nil },
	}
	return NewChange(p, place.root, spec), nil, nil
}
