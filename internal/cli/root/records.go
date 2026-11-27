package root

// The operations of the history, the cache and the journal as the command
// packages reach them: the folder and the settings are the session's, the
// answer is the core's, and what to say of it is the command's.

import (
	"context"
	"errors"

	"github.com/birdtable/gragas/internal/cli/kit"
	"github.com/birdtable/gragas/internal/config"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

var _ kit.Records = kitSession{}

// folder is the working folder's root, the one the journal is of.
func (k kitSession) folder() (model.Root, error) {
	root, err := k.s.p.World.Tree.Open("")
	if err != nil {
		return "", kit.Said{What: "the working folder could not be opened: " + err.Error()}
	}
	return root, nil
}

// keepSeconds is the keep setting in seconds.
func (k kitSession) keepSeconds() (int64, error) {
	secs, err := k.s.p.Config.Settings.KeepSeconds()
	if err != nil {
		return 0, kit.Said{What: err.Error() + ". Set keep in the settings file to a number of days (30d) or hours (72h)"}
	}
	return secs, nil
}

// History lists the entries of the history of the working folder.
func (k kitSession) History(ctx context.Context, ask model.HistoryAsk) (model.HistoryListing, error) {
	root, err := k.folder()
	if err != nil {
		return model.HistoryListing{}, err
	}
	ask.Root = root
	return coreapi.History(ctx, k.s.p.World, ask)
}

// CacheList lists what the kinds of the cache hold.
func (k kitSession) CacheList(ctx context.Context) (model.CacheListing, error) {
	st := k.s.p.Config.Settings
	secs, err := k.keepSeconds()
	if err != nil {
		return model.CacheListing{}, err
	}
	ask := coreapi.CacheAsk{Keep: st.Keep, KeepSeconds: secs}
	for _, kind := range config.CacheKinds() {
		dir := k.s.p.Config.StateDir
		if kind.Place == config.InCache {
			dir = k.s.p.Config.CacheDir
		}
		ask.Kinds = append(ask.Kinds, coreapi.CacheKind{Name: kind.Name, Holds: kind.Holds, Dir: dir, Names: kind.Names, Journal: kind.Journal, Built: kind.Built, Nested: kind.Nested})
	}
	got, err := coreapi.CacheList(ctx, k.s.p.World, ask)
	if errors.Is(err, port.ErrCarried) {
		// A state folder a clone carries ends the listing refused, as the
		// recorder's lock ends a run that writes there.
		return model.CacheListing{}, kit.Refusing{What: err.Error(), Next: carriedFix + joinArgv(k.s.rec.argv)}
	}
	return got, err
}

// CacheClearPlan counts what a clear would remove.
func (k kitSession) CacheClearPlan(ctx context.Context) (model.ClearPlan, error) {
	secs, err := k.keepSeconds()
	if err != nil {
		return model.ClearPlan{}, err
	}
	root, err := k.folder()
	if err != nil {
		return model.ClearPlan{}, err
	}
	return coreapi.CacheClearPlan(ctx, k.s.p.World, root, secs)
}

// CacheClear removes the closed runs past the keep age.
func (k kitSession) CacheClear(ctx context.Context) (model.Cleared, error) {
	root, err := k.folder()
	if err != nil {
		return model.Cleared{}, err
	}
	return coreapi.CacheClear(ctx, k.s.p.World, root)
}

// JournalList lists the runs that wrote on the working folder.
func (k kitSession) JournalList(ctx context.Context) (model.JournalListing, error) {
	root, err := k.folder()
	if err != nil {
		return model.JournalListing{}, err
	}
	return coreapi.JournalList(ctx, k.s.p.World, root)
}

// JournalRestore says how to put one run's files back.
func (k kitSession) JournalRestore(ctx context.Context, run string) (model.Restoration, error) {
	root, err := k.folder()
	if err != nil {
		return model.Restoration{}, err
	}
	return coreapi.JournalRestore(ctx, k.s.p.World, root, model.RestoreAsk{Run: run, StateDir: k.s.p.Config.StateDir}, k.s.line)
}

// JournalRecover rolls back the runs that were left open.
func (k kitSession) JournalRecover(ctx context.Context) (model.Recovery, error) {
	root, err := k.folder()
	if err != nil {
		return model.Recovery{}, err
	}
	return coreapi.JournalRecover(ctx, k.s.p.World, root, model.RecoverAsk{Run: k.s.rec.id(), Engine: k.s.build.String()})
}

// ApplyFlag is the variable --apply sets.
func (k kitSession) ApplyFlag() *bool { return &k.s.applied }

// Applying is whether the run was given --apply.
func (k kitSession) Applying() bool { return k.s.applied }

// Data sets the document the run answers with.
func (k kitSession) Data(v any) { k.s.data = v }

// Change is the line of one change, kept for the record.
func (k kitSession) Change(verb, path string) string { return k.s.change(verb, path) }

// WithRun names the run a sentence is about.
func (k kitSession) WithRun(what string) string { return k.s.withRun(what) }

// folder is the working folder's root, the one the journal is of; a folder
// that will not open ends the run failed.
func (s *session) folder() (model.Root, bool) {
	root, err := s.p.World.Tree.Open("")
	if err != nil {
		s.end(ending{word: model.Failed, what: "the working folder could not be opened: " + err.Error()})
		return "", false
	}
	return root, true
}
