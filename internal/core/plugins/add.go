package plugins

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
)

// AddInput is everything PlanAdd decides on, all of it read by the caller
// before the plan is made, so the plan reads no file (the write path of
// arch.ports.14).
type AddInput struct {
	// Ref is the plugin asked for, and Source where its bundle came from, as
	// the lock records it.
	Ref    model.PluginRef
	Source string
	// Files are every file of the bundle at its source, by path below its
	// folder.
	Files []model.File
	// LockFile is the bytes of the rulebook's plugin lock, nil where it has none,
	// and LockDigest the digest of those bytes, "" where it has none.
	LockFile   []byte
	LockDigest model.Digest
	// Present are the paths of the files already below the version's folder,
	// plugins/<name>/<version>, whatever the lock says, relative to the root.
	Present []string
	// Bounds are the sizes a bundle is held to, the reader's own.
	Bounds pluginbundle.Bounds
}

// AddPlan is what PlanAdd decides.
type AddPlan struct {
	// Nothing says the bundle is locked already at the digest of these files,
	// so there is nothing to write.
	Nothing bool
	// Targets are the files to write in order: the bundle's files by path,
	// and the lock last, so a run that stops before the lock leaves a folder
	// no lock names, which is no plugin, and the journal rolls it back.
	Targets []model.Target
	// Inputs are the files the plan read, each with its digest.
	Inputs []model.Input
	// Delta says what is added.
	Delta model.Delta
	// Digest is the digest of the bundle, as the lock will hold it.
	Digest model.Digest
}

// PlanAdd decides an add of in.Files as the bundle of in.Ref. It asks, in this
// order, and the first that fails ends it:
//
//  1. pluginbundle.Judge of the files, as a lock holding them would be read:
//     its bounds, paths, manifest and module (the refusal matches the port's
//     sentinels);
//  2. the reader's own bounds on every file and on the total, so a bundle
//     added is one Read does not refuse (pluginbundle.ErrOverBound);
//  3. the lock, read whole, and no ref in it twice;
//  4. the ref locked at these files' digest is nothing to do, and locked at
//     another is refused, never replaced (fs.ErrExist);
//  5. a folder of the ref that is there with files and no lock entry is
//     refused, naming it, since the lock would pin a bundle that is not what
//     the folder holds (fs.ErrExist);
//  6. the lock with the entry added is within its bound.
func PlanAdd(in AddInput) (AddPlan, error) {
	return planAdd(in, nil)
}

// planAdd is PlanAdd, and with drop the lock it writes holds no entry of that
// ref: the version an upgrade replaces. The entry dropped is still read in
// the steps above, and the lock target replaces the lock as it is.
func planAdd(in AddInput, drop *model.PluginRef) (AddPlan, error) {
	bundle := pluginbundle.BundleOf(in.Files)
	if _, err := pluginbundle.Judge(in.Ref, bundle); err != nil {
		return AddPlan{}, err
	}
	if err := in.Bounds.Holds(in.Files); err != nil {
		return AddPlan{}, err
	}
	var entries []pluginbundle.LockEntry
	if in.LockFile != nil {
		var err error
		if entries, err = pluginbundle.DecodePluginLock(in.LockFile); err != nil {
			return AddPlan{}, err
		}
	}
	out := AddPlan{Digest: bundle.Locked}
	if i := slices.IndexFunc(entries, func(e pluginbundle.LockEntry) bool { return e.Ref == in.Ref }); i >= 0 {
		if entries[i].Digest == out.Digest {
			// Nothing to write only when the folder holds exactly the bundle:
			// a folder gone or partial is one Read would refuse.
			if err := folderIsTheBundle(in); err != nil {
				return AddPlan{}, err
			}
			out.Nothing = true
			return out, nil
		}
		return AddPlan{}, fmt.Errorf("%s@%s is locked at %s, and these files are %s: remove it first: %w", in.Ref.Name, in.Ref.Version, entries[i].Digest, out.Digest, fs.ErrExist)
	}
	folder := pluginbundle.FolderOf(in.Ref)
	if n := len(in.Present); n > 0 {
		return AddPlan{}, fmt.Errorf("%s holds %d files and the lock names no %s@%s: a folder the lock does not name is no plugin; move it away first: %w", folder, n, in.Ref.Name, in.Ref.Version, fs.ErrExist)
	}
	entry := pluginbundle.LockEntry{Ref: in.Ref, Source: in.Source, Digest: out.Digest}
	kept := slices.DeleteFunc(slices.Clone(entries), func(e pluginbundle.LockEntry) bool { return drop != nil && e.Ref == *drop })
	next := pluginbundle.EncodePluginLock(append(kept, entry))
	if _, err := pluginbundle.DecodePluginLock(next); err != nil {
		return AddPlan{}, fmt.Errorf("the lock entry for %s@%s cannot be written: %w", in.Ref.Name, in.Ref.Version, err)
	}
	if int64(len(next)) > in.Bounds.Lock {
		return AddPlan{}, fmt.Errorf("the lock would hold %d bytes, over the %d it may: %w", len(next), in.Bounds.Lock, pluginbundle.ErrOverBound)
	}
	files := slices.Clone(in.Files)
	slices.SortFunc(files, func(a, b model.File) int { return strings.Compare(a.Path, b.Path) })
	for _, f := range files {
		p := path.Join(folder, f.Path)
		out.Targets = append(out.Targets, model.Target{Path: p, Data: f.Data})
		out.Inputs = append(out.Inputs, model.Input{Path: p})
		out.Delta.Changes = append(out.Delta.Changes, model.Change{Entity: "file:" + p, After: "added"})
	}
	out.Targets = append(out.Targets, model.Target{Path: pluginbundle.PluginLockPath, Before: in.LockDigest, Data: next})
	out.Inputs = append(out.Inputs, model.Input{Path: pluginbundle.PluginLockPath, Digest: in.LockDigest})
	out.Delta.Changes = append(out.Delta.Changes, model.Change{Entity: "plugin:" + in.Ref.Name + "@" + in.Ref.Version, Field: "lock", After: string(out.Digest)})
	return out, nil
}

// folderIsTheBundle is nil when the files below the version's folder are
// exactly the bundle's paths, and otherwise a refusal that names the folder
// (fs.ErrExist), since a lock that names a bundle the folder does not hold
// is a plugin Read refuses.
func folderIsTheBundle(in AddInput) error {
	folder := pluginbundle.FolderOf(in.Ref)
	want := make([]string, 0, len(in.Files))
	for _, f := range in.Files {
		want = append(want, path.Join(folder, f.Path))
	}
	have := slices.Clone(in.Present)
	slices.Sort(want)
	slices.Sort(have)
	if slices.Equal(want, have) {
		return nil
	}
	return fmt.Errorf("%s@%s is locked at these files' digest, but %s holds %d files and the bundle is %d: the folder is not what the lock names; move it away and add again: %w", in.Ref.Name, in.Ref.Version, folder, len(have), len(want), fs.ErrExist)
}
