package coreapi

// The operation behind gragas plugin record: it runs the
// real tool through an installed, approved plugin's own plan, by the same
// rounds and the same step runner as every other read, and files what the
// steps printed and the cases it makes in the author's folder of the bundle.
// It is the one plugin command that runs a real tool.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path"
	"unicode/utf8"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// recordSeconds is the deadline of one step of a recording. It sets no memory
// limit: gragas sets none of its own.
const recordSeconds = 300

// A PluginRecordAsk is a request to record the plugin of the author's folder
// Dir, <name>/<version>, at Ask.Root.
type PluginRecordAsk struct {
	Ask model.ChangeAsk
	// Dir is the author's folder as the command was given it.
	Dir string
	// Op is entries or findings, the reading call the case is made of.
	Op string
	// Config are the config files the plugin reads, and Fixtures the files
	// the tool runs over, each as a path in the project.
	Config, Fixtures []string
	// Case is the number the case folder carries; "" takes the next free one.
	Case string
	// Apply says the tool is run and the files written; without it nothing
	// but the tool's version is asked and nothing is written.
	Apply bool
}

// recordPlace is where a recording is, as every refusal and plan needs it.
type recordPlace struct {
	ref  model.PluginRef
	root model.Root // the author's folder, opened by the Tree
	dir  string     // the folder as the command was given it
}

// PluginRecord is the change that files a recording in the author's folder.
// The dry run asks the tool its version, asks the plugin for its first plan
// and lists the steps and the files, and writes nothing. --apply runs the
// rounds, and the files go through the one write path. Every start is
// journalled before it is made (StartNote). A refusal is a *model.Refusal.
func PluginRecord(ctx context.Context, p port.Ports, ask PluginRecordAsk) (ch *Change, refused *model.Refusal, err error) {
	root := ask.Ask.Root
	place, pl, ref, err := openRecorded(ctx, p, ask)
	if ref != nil || err != nil {
		return nil, ref, err
	}
	defer func() {
		if cerr := pl.Close(); cerr != nil && err == nil {
			ch, refused, err = nil, nil, fmt.Errorf("the plugin %s@%s could not be closed: %w", place.ref.Name, place.ref.Version, cerr)
		}
	}()
	in, cleanup, err := recordInput(ctx, p, ask, pl.Manifest())
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if cerr := cleanup(); cerr != nil && err == nil {
			ch, refused, err = nil, nil, fmt.Errorf("the scratch folder could not be removed: %w", cerr)
		}
	}()

	stepsRun := ask.Ask.Run + "-steps"
	note := StartNote(p.Journal, root, stepsRun, ask.Ask.Command, ask.Ask.Engine)
	// The steps run is closed on every way out, with the outcome the path set.
	outcome := "failed"
	defer func() {
		if cerr := closeSteps(ctx, p, ask, stepsRun, outcome); cerr != nil && err == nil {
			ch, refused, err = nil, nil, cerr
		}
	}()
	steps, err := p.Steps.Bind(ctx, plugin.Scope{Manifest: pl.Manifest(), Root: root, Scratch: in.scratch, Floor: in.floor, Note: note})
	if err != nil {
		return nil, refuseOr(err, ask, place, fmt.Sprintf("the tool of %s could not be started", place.ref.Name)), nil
	}
	// A tool outside the plugin's tool_versions is not bound: the host's Bind
	// refuses it as not ready, and a manifest cannot grant a wider range.
	tool := steps.Tool().Version
	in.plan.ToolVersion = tool
	in.plan.Scratch = &in.scratch.Dir

	if !ask.Apply {
		first, err := pl.Plan(ctx, firstPlan(in.plan, plugin.Op(ask.Op)))
		if err != nil {
			return nil, nil, fmt.Errorf("the plugin's first plan could not be asked: %w", err)
		}
		outcome = "done"
		return recordPreview(p, ask, place, tool, first), nil, nil
	}

	rec, err := plugins.Record(ctx, pl, steps, in.plan, plugin.Op(ask.Op))
	if err != nil {
		return nil, refuseOr(err, ask, place, "the read did not finish"), nil
	}
	outcome = "done"
	return recordChange(p, ask, place, tool, rec)
}

// openRecorded finds the folder, checks the op and the module and opens the
// installed, approved plugin; a refusal or an error leaves nothing open.
func openRecorded(ctx context.Context, p port.Ports, ask PluginRecordAsk) (recordPlace, port.Plugin, *model.Refusal, error) {
	place, ref, err := recordFolder(p, ask)
	if ref != nil || err != nil {
		return place, nil, ref, err
	}
	if plugin.Op(ask.Op) != plugin.OpEntries && plugin.Op(ask.Op) != plugin.OpFindings {
		return place, nil, nil, fmt.Errorf("--op names %q: say entries or findings", ask.Op)
	}
	if ref := guardModule(ctx, p, ask, place); ref != nil {
		return place, nil, ref, nil
	}
	pl, err := p.Plugins.Open(ctx, string(ask.Ask.Root), place.ref)
	if errors.Is(err, port.ErrNotApproved) || errors.Is(err, port.ErrNotInstalled) {
		// The host's own words: there is no second wording of them here.
		return place, nil, newRefusal(model.Refused, err.Error(), "gragas plugin approve "+place.ref.Name+"@"+place.ref.Version), nil
	}
	if err != nil {
		return place, nil, nil, fmt.Errorf("the plugin %s@%s could not be opened: %w", place.ref.Name, place.ref.Version, err)
	}
	return place, pl, nil, nil
}

// closeSteps closes the journal run the steps were journalled under, with the
// outcome the path set; it is made even when the caller's context is done.
func closeSteps(ctx context.Context, p port.Ports, ask PluginRecordAsk, run, outcome string) error {
	detail := []byte(`{"outcome":"` + outcome + `"}`)
	rec := model.Record{Run: run, Kind: model.KindClose, Command: ask.Ask.Command, Engine: ask.Ask.Engine, Detail: detail}
	if err := p.Journal.Append(context.WithoutCancel(ctx), ask.Ask.Root, rec); err != nil {
		return fmt.Errorf("the steps of the recording could not be closed in the journal: %w", err)
	}
	return nil
}

// firstPlan is the first plan's request for op.
func firstPlan(in plugin.PlanRequest, op plugin.Op) plugin.PlanRequest {
	in.Round, in.Have = 0, nil
	in.For = plugin.ForConfig
	if op == plugin.OpFindings {
		in.For = plugin.ForRun
	}
	return in
}

// recordFolder reads where the author's folder is: <name>/<version>, in the
// project or outside it, and not the installed copy.
func recordFolder(p port.Ports, ask PluginRecordAsk) (recordPlace, *model.Refusal, error) {
	root, err := p.Tree.Open(ask.Dir)
	if err != nil {
		return recordPlace{}, nil, fmt.Errorf("the bundle %s could not be opened: %w", ask.Dir, err)
	}
	folder := path.Clean(string(root))
	ref := model.PluginRef{Name: path.Base(path.Dir(folder)), Version: path.Base(folder)}
	if ref.Name == "." || ref.Name == "/" || ref.Version == "." || ref.Version == "/" {
		return recordPlace{}, nil, fmt.Errorf("the folder %s is not <name>/<version>: name the version's folder of the plugin", ask.Dir)
	}
	rel, err := p.Tree.Within(ask.Ask.Root, folder)
	switch {
	case errors.Is(err, port.ErrOutside):
		rel = ""
	case err != nil:
		return recordPlace{}, nil, fmt.Errorf("where the bundle %s lies in the project could not be told: %w", folder, err)
	}
	if rel != "" && rel == pluginbundle.FolderOf(ref) {
		return recordPlace{}, newRefusal(model.Refused, "the folder "+ask.Dir+" is the installed copy, and a recording is never written into it: its digest is the lock's", recordLine(ask)), nil
	}
	return recordPlace{ref: ref, root: root, dir: ask.Dir}, nil, nil
}

// guardModule refuses, before any step runs, a folder whose plugin.wasm is
// not the module of the installed bundle: a recording then always sits beside
// the module that made it. An installed bundle that is
// not there is the host's ErrNotInstalled, as Open says it.
func guardModule(ctx context.Context, p port.Ports, ask PluginRecordAsk, place recordPlace) *model.Refusal {
	root := ask.Ask.Root
	have, err := p.Tree.Read(place.root, "plugin.wasm")
	if err != nil {
		return newRefusal(model.Refused, fmt.Sprintf("%s/plugin.wasm could not be read: %v", place.dir, err), recordLine(ask))
	}
	b, err := p.Bundles.Read(ctx, string(root), place.ref)
	if err != nil {
		// Not installed, or not readable: Open tells which, in its own words.
		return nil
	}
	folderSum, installedSum := digest.Prefixed(have), digest.Prefixed(b.Module)
	if folderSum == installedSum {
		return nil
	}
	return newRefusal(model.Refused,
		fmt.Sprintf("the plugin.wasm in %s is %s and the installed %s@%s holds %s: a recording sits beside the module that made it", place.dir, folderSum, place.ref.Name, place.ref.Version, installedSum),
		"gragas plugin add "+place.dir+" --apply")
}

// recordInput is what a recording reads from the project: the scratch folder
// with the config mirrored and the fixtures placed, and the first plan's
// request over them.
type recordIn struct {
	scratch model.Scratch
	plan    plugin.PlanRequest
	// floor and level are what the record's configs ask (configsFloor): the
	// scratch folder is prepared at level and the steps bound at floor.
	floor plugin.Level
	level model.Level
}

func recordInput(ctx context.Context, p port.Ports, ask PluginRecordAsk, m plugin.Manifest) (recordIn, func() error, error) {
	var in recordIn
	in.floor, in.level = configsFloor(ask.Config, loadsCode(m))
	root := ask.Ask.Root
	var cfg, fix []model.File
	for _, name := range ask.Config {
		data, err := p.Tree.Read(root, name)
		if err != nil {
			return in, nil, fmt.Errorf("the config %s could not be read: %w", name, err)
		}
		cfg = append(cfg, model.File{Path: path.Base(name), Data: data})
		d := bytesOfFile(data)
		in.plan.Config = append(in.plan.Config, plugin.Config{Path: path.Base(name), Data: &d})
	}
	for _, name := range ask.Fixtures {
		data, err := p.Tree.Read(root, name)
		if err != nil {
			return in, nil, fmt.Errorf("the fixture %s could not be read: %w", name, err)
		}
		fix = append(fix, model.File{Path: name, Data: data})
		in.plan.Fixtures = append(in.plan.Fixtures, name)
	}
	if in.plan.Config == nil {
		in.plan.Config = []plugin.Config{}
	}
	in.plan.Target = plugin.TargetSource
	if len(m.Targets) > 0 {
		in.plan.Target = m.Targets[0]
	}
	sc, err := p.Sandbox.Prepare(ctx, model.Prepare{Level: in.level, Seconds: recordSeconds, Config: cfg, Fixtures: fix})
	if err != nil {
		return in, nil, fmt.Errorf("the scratch folder could not be made: %w", err)
	}
	in.scratch = sc
	return in, func() error { return p.Sandbox.Release(sc) }, nil
}

// bytesOfFile is data in the contract's form: text when it is valid UTF-8,
// else base64.
func bytesOfFile(data []byte) plugin.Bytes {
	if utf8.Valid(data) {
		s := string(data)
		return plugin.Bytes{Text: &s}
	}
	s := base64.StdEncoding.EncodeToString(data)
	return plugin.Bytes{Base64: &s}
}

// recordLine is the line that records, with --apply.
