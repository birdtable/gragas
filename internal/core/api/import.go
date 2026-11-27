package coreapi

// import: the guide's drafts and a project's existing config, brought into the
// rulebook in one plan. This file reads the two inputs through the Tree port
// and composes the same steps every plan command does; what the drafts are and
// which of them a config entry holds is the lifecycle context's (PlanImport),
// and what an entry is is link's (ReadConfigs).

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/lifecycle"
	"github.com/birdtable/gragas/internal/core/link"
	"github.com/birdtable/gragas/internal/markdown"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// ErrNothingToImport is an import whose guide holds no Markdown file and whose
// configs hold no entry: there is nothing to plan, and the command says so.
var ErrNothingToImport = errors.New("nothing to import")

// maxGuideFiles caps the Markdown files of a guide's folder: it is a folder of
// another repository's, so it is bounded before it is read.
const maxGuideFiles = 256

// Import is the change that brings the guide in in.GuideDir and the config
// files in in.Configs into the rulebook in ask.Root. A request the lifecycle
// context refuses comes back as a refusal; an input that cannot be read, a
// config the tool's adapter does not read, or an import with nothing in it is
// an error (ErrNothingToImport for the last).
func Import(ctx context.Context, p port.Ports, ask model.ChangeAsk, in model.ImportInput) (*Change, *model.Refusal, error) {
	docs, err := guideDocs(p, ask.Root, in.GuideDir)
	if err != nil {
		return nil, nil, err
	}
	var entries []model.ImportEntry
	if len(in.Configs) > 0 {
		entries, err = configEntries(ctx, p, ask, in)
		if err != nil {
			return nil, nil, err
		}
	}
	if len(docs) == 0 && len(entries) == 0 {
		return nil, nil, ErrNothingToImport
	}
	req := model.ImportRequest{
		Draft: model.DraftGuide{Part: in.Part, Concern: in.Concern, Guide: in.Guide},
		Tool:  in.Tool, Target: in.Target, Entries: entries,
	}
	return compose(ctx, p, ask, in.Part+"."+in.Concern+".0", withFiles(&req, &req.Draft.Files, func(b model.Book, r model.ImportRequest, m lifecycle.Moment) (lifecycle.Plan, error) {
		return lifecycle.PlanImport(b, docs, r, m)
	}))
}

// guideDocs reads the Markdown files directly in dir, by name, as documents.
// No dir is no guide.
func guideDocs(p port.Ports, root model.Root, dir string) ([]*markdown.Doc, error) {
	if dir == "" {
		return nil, nil
	}
	list, err := p.Tree.List(root, dir, model.Every)
	if err != nil {
		return nil, fmt.Errorf("the guide's folder %s cannot be read: %w", dir, err)
	}
	var names []string
	for _, e := range list {
		if !e.Dir && path.Ext(e.Path) == ".md" && path.Dir(e.Path) == path.Clean(dir) {
			names = append(names, e.Path)
		}
	}
	slices.Sort(names)
	if len(names) > maxGuideFiles {
		return nil, fmt.Errorf("the guide's folder %s holds %d Markdown files, over the %d read", dir, len(names), maxGuideFiles)
	}
	var docs []*markdown.Doc
	for _, name := range names {
		src, err := p.Tree.Read(root, name)
		if err != nil {
			return nil, err
		}
		doc, err := markdown.Parse(src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// configEntries reads the entries of the config files through the plugin
// installed for the tool. With no plugin of the tool installed the read is
// refused (ErrNoPlugin), and no adapter answers it. The choice is the find of pluginOf.
func configEntries(ctx context.Context, p port.Ports, ask model.ChangeAsk, in model.ImportInput) ([]model.ImportEntry, error) {
	if in.Tool == "" || in.Target == "" {
		return nil, errors.New("a config needs the tool it is of and the target it is about")
	}
	// the tested set of the book at the root, where there is one, picks the
	// plugin as it does for the rank and the preset
	var tested *model.Tested
	switch rep, err := LoadBook(ctx, p, ask.Root); {
	case err == nil:
		tested = rep.Book.Identity.Tested
	case !errors.Is(err, ErrNoBook):
		return nil, err
	}
	pl, found, err := pluginOf(ctx, p.Plugins, ask.Root, in.Tool, tested)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, noPlugin(in.Tool)
	}
	return pluginConfigEntries(ctx, p, ask, in, pl)
}

// pluginConfigEntries is the read of the config files through pl, which it
// closes: one call of the plugin's entries per file, each over a scratch folder
// holding that file alone, at the floor its name gives (configFloor).
// A plugin that is refused ends the import, and so does a call it fails: no
// entry of a failed read is imported. A config reaching beyond its file is
// refused before any step (plugins.Entries and the plan, which plugin test's
// C20 holds); a tool whose data config can load code says so in its
// manifest's config_loads_code, and its reads run sandboxed.
func pluginConfigEntries(ctx context.Context, p port.Ports, ask model.ChangeAsk, in model.ImportInput, pl port.Plugin) (_ []model.ImportEntry, err error) {
	defer func() {
		if cerr := pl.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("the plugin of %s could not be closed: %w", in.Tool, cerr)
		}
	}()
	var files []link.ConfigFile
	for _, name := range in.Configs {
		data, err := p.Tree.Read(ask.Root, strings.TrimPrefix(path.Clean(name), "./"))
		if err != nil {
			return nil, fmt.Errorf("the config %s cannot be read: %w", name, err)
		}
		files = append(files, link.ConfigFile{Path: name, Data: data})
	}
	run, err := beginReadRun(ctx, p.Journal, p.Clock, ask, in.Tool)
	if err != nil {
		return nil, err
	}
	// The close is written whatever ends the read: the journal refuses a record
	// on a cancelled ctx, and a run left open refuses validate.
	defer func() { err = run.end(context.WithoutCancel(ctx), err) }()
	read := pluginRead{sandbox: p.Sandbox, steps: p.Steps, note: run.note, root: ask.Root, tree: p.Tree, writer: p.Writer}
	call := func(ctx context.Context, req plugin.PlanRequest) (plugin.EntriesResponse, error) {
		c := req.Config[0]
		return read.entries(ctx, pl, in.Tool, req.Target, c.Path, []byte(*c.Data.Text))
	}
	return link.ReadConfigs(ctx, call, "", in.Target, files)
}
