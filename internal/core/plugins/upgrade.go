package plugins

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// ErrOlder is an upgrade to a version older than the one it replaces when
// that version was not named: the newest version a source holds is never
// taken as a step back.
var ErrOlder = errors.New("older than the version installed")

// ErrNotPinned is an upgrade from an installed bundle whose files are not
// what its lock entry pins: its grants are never compared, since a view of
// what changed would be of a bundle nobody locked.
var ErrNotPinned = errors.New("the installed bundle is not what the lock pins")

// ErrNotSemver is an upgrade from or to a version that is not a semver
// version, such as 02.0.0: versions are ordered by model.CompareVersions,
// which has no order for one, so neither the version replaced nor the newest
// could be told.
var ErrNotSemver = errors.New("not a semver version")

// UpgradeInput is everything PlanUpgrade decides on, all of it read by the
// caller before the plan is made, as AddInput is.
type UpgradeInput struct {
	// Add is the new version as PlanAdd takes it: its ref, source and files,
	// the lock as it is, and what the new version's folder holds.
	Add AddInput
	// Named says the version was named, not taken as the newest the source
	// holds: only a named version may be older than the one it replaces.
	Named bool
	// Old is the version upgraded from, UpgradeFrom of the lock, and OldFiles
	// every file below its folder, by path below it. Both are unread when Old
	// is Add.Ref, a version already installed.
	Old      model.PluginRef
	OldFiles []model.File
}

// UpgradePlan is what PlanUpgrade decides.
type UpgradePlan struct {
	// AddPlan is the add of the new version: Nothing when it is installed at
	// these files, else its files, the lock with the old entry swapped for the
	// new, and then the old version's files removed. Digest is the new one's.
	AddPlan
	// Old is the version replaced and OldDigest its digest as the lock pins it.
	Old       model.PluginRef
	OldDigest model.Digest
	// OldManifest and NewManifest are the two manifests, each judged, whose
	// grants the plan compares.
	OldManifest, NewManifest plugin.Manifest
	// Stays are the other versions of the plugin the lock holds, which keep
	// their entries and their folders.
	Stays []model.PluginRef
	// Older says the new version is older than the one it replaces, as named.
	Older bool
}

// UpgradeFrom is the version of the plugin called name that an upgrade to
// version replaces: version itself when the lock holds it, which is nothing
// to do, and otherwise the newest version the lock holds of that name, by
// model.CompareVersions. A lock that holds no version of name, and no lock,
// match port.ErrNotInstalled: an upgrade is of a plugin that is installed.
// version, and each version the lock holds of name, that CompareVersions
// does not order is refused by name (ErrNotSemver).
func UpgradeFrom(lock []byte, name, version string) (model.PluginRef, error) {
	if !model.VersionOrdered(version) {
		return model.PluginRef{}, fmt.Errorf("the upgrade is to %s@%s, which is %w, so it has no order against the version installed", plugin.Shown(name), plugin.Shown(version), ErrNotSemver)
	}
	var entries []pluginbundle.LockEntry
	if lock != nil {
		var err error
		if entries, err = pluginbundle.DecodePluginLock(lock); err != nil {
			return model.PluginRef{}, err
		}
	}
	for _, e := range entries {
		if e.Ref.Name == name && !model.VersionOrdered(e.Ref.Version) {
			return model.PluginRef{}, fmt.Errorf("the lock holds %s@%s, which is %w, so the version an upgrade replaces cannot be told", plugin.Shown(name), plugin.Shown(e.Ref.Version), ErrNotSemver)
		}
	}
	var old model.PluginRef
	for _, e := range entries {
		switch {
		case e.Ref.Name != name:
		case e.Ref.Version == version:
			return e.Ref, nil
		case old.Name == "" || model.CompareVersions(e.Ref.Version, old.Version) > 0:
			old = e.Ref
		}
	}
	if old.Name == "" {
		return model.PluginRef{}, fmt.Errorf("%w: the lock holds no %s", port.ErrNotInstalled, plugin.Shown(name))
	}
	return old, nil
}

// PlanUpgrade decides the upgrade of the plugin in.Old names to in.Add.Ref.
// It asks, in this order, and the first that fails ends it:
//
//  1. in.Old is UpgradeFrom of the lock, so the caller read the version the
//     lock says is replaced;
//  2. a version the lock holds is PlanAdd's: nothing to do at these files,
//     refused at others;
//  3. a new version older than the old one is refused unless it was named
//     (ErrOlder);
//  4. the old version's files are judged against the digest its lock entry
//     pins (pluginbundle.Judge): a bundle that is not what the lock pins is
//     refused (ErrNotPinned), and its grants are never compared;
//  5. the add of the new version, every check of PlanAdd, with the lock it
//     writes holding no entry of the old version;
//  6. the old version's files are removed, after the lock, which names it no
//     longer (planAdd drops its entry, and a lock holds a ref once), each file
//     by the digest of the bytes judged.
//
// It approves nothing: the new version's digest is not approved until a
// person approves it.
func PlanUpgrade(in UpgradeInput) (UpgradePlan, error) {
	ref := in.Add.Ref
	old, err := UpgradeFrom(in.Add.LockFile, ref.Name, ref.Version)
	if err != nil {
		return UpgradePlan{}, err
	}
	if old != in.Old {
		return UpgradePlan{}, fmt.Errorf("the upgrade was read from %s@%s and the lock replaces %s@%s", in.Old.Name, in.Old.Version, old.Name, old.Version)
	}
	if old == ref {
		add, err := planAdd(in.Add, nil)
		if err != nil {
			return UpgradePlan{}, err
		}
		return UpgradePlan{AddPlan: add, Old: old}, nil
	}
	out := UpgradePlan{Old: old, Older: model.CompareVersions(ref.Version, old.Version) < 0}
	if out.Older && !in.Named {
		return UpgradePlan{}, fmt.Errorf("%w: %s@%s is the newest the source holds, and %s is installed; name the version to take an older one", ErrOlder, ref.Name, ref.Version, old.Version)
	}
	entries, err := pluginbundle.DecodePluginLock(in.Add.LockFile)
	if err != nil {
		return UpgradePlan{}, err
	}
	for _, e := range entries {
		switch {
		case e.Ref == old:
			out.OldDigest = e.Digest
		case e.Ref.Name == ref.Name:
			out.Stays = append(out.Stays, e.Ref)
		}
	}
	installed := pluginbundle.BundleOf(in.OldFiles)
	installed.Locked = out.OldDigest
	if out.OldManifest, err = pluginbundle.Judge(old, installed); err != nil {
		return UpgradePlan{}, fmt.Errorf("%w: %s@%s, and its grants are not compared: %w", ErrNotPinned, old.Name, old.Version, err)
	}
	if out.AddPlan, err = planAdd(in.Add, &old); err != nil {
		return UpgradePlan{}, err
	}
	if out.NewManifest, err = pluginbundle.Judge(ref, pluginbundle.BundleOf(in.Add.Files)); err != nil {
		return UpgradePlan{}, err
	}
	out.Delta.Changes = append(out.Delta.Changes, model.Change{Entity: "plugin:" + old.Name + "@" + old.Version, Field: "lock", Before: string(out.OldDigest)})
	files := slices.Clone(in.OldFiles)
	slices.SortFunc(files, func(a, b model.File) int { return strings.Compare(a.Path, b.Path) })
	for _, f := range files {
		p := path.Join(pluginbundle.FolderOf(old), f.Path)
		d := model.Digest(digest.Prefixed(f.Data))
		out.Targets = append(out.Targets, model.Target{Path: p, Before: d, Delete: true})
		out.Inputs = append(out.Inputs, model.Input{Path: p, Digest: d})
		out.Delta.Changes = append(out.Delta.Changes, model.Change{Entity: "file:" + p, Before: string(d)})
	}
	return out, nil
}
