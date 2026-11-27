package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model"
	modelplugin "github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// PluginUpgradeFrom is the change that upgrades an installed plugin to a
// version the folder source holds. ref is <name> or
// <name>@<version>: the bundle is source/<name>/<version>, read as plugin add
// reads one, and with no version the newest version folder the source holds,
// by model.CompareVersions. The version it replaces is the newest the lock
// holds of that name (plugins.UpgradeFrom).
//
// The plan is the add of the new version and the view of what changed: the
// old and the new digest, and each grant added, removed or changed with the
// command that asks its version. --apply writes the new files, then the lock
// with the old entry swapped for the new, then removes the old version's
// files, which no entry names any longer; other versions of the plugin the
// lock holds stay, and the plan says so. It approves nothing: the closing
// words say the new version is installed and not approved, and name plugin
// approve.
//
// A version already installed at these files is nothing to do. A plugin the
// lock does not hold, a version older than the installed one that was not
// named, an installed bundle that is not what its lock entry pins, and a
// version in the lock or the source that is not semver (02.0.0) are refused,
// each with the line to type.
func PluginUpgradeFrom(ctx context.Context, p port.Ports, source, ref string, ask model.ChangeAsk) (*Change, *model.Refusal, error) {
	name, version, named := strings.Cut(ref, "@")
	if !pluginbundle.NameFormed(name) || (named && !pluginbundle.VersionFormed(version)) {
		r, err := model.NewRefusal(model.Usage, fmt.Sprintf("%s is not <name> or <name>@<version> of a plugin", modelplugin.Shown(ref)), nil, "gragas plugin upgrade <source> <name>[@<version>]")
		if err != nil {
			return nil, nil, err
		}
		return nil, &r, nil
	}
	add, err := upgradeAddAsk(p, source, name, version, ask)
	if err != nil {
		if r, ok := upgradeRefusal(err, source, model.PluginRef{Name: name, Version: version}); ok {
			return nil, &r, nil
		}
		return nil, nil, err
	}
	version = add.Ref.Version
	first, err := pluginUpgradePlan(p, add, named)
	if err != nil {
		if r, ok := upgradeRefusal(err, source, add.Ref); ok {
			return nil, &r, nil
		}
		return nil, nil, err
	}
	var cur plugins.UpgradePlan
	paths := make([]string, 0, len(first.Targets))
	for _, t := range first.Targets {
		paths = append(paths, t.Path)
	}
	shown := name + "@" + version
	spec := Spec{
		Command: ask.Command, Applies: ask.Applies, Engine: ask.Engine, Run: ask.Run,
		Scope: paths,
		Plan: func(context.Context) (model.Plan, error) {
			u, err := pluginUpgradePlan(p, add, named)
			if err != nil {
				return model.Plan{}, err
			}
			cur = u
			lines, err := upgradeLines(u)
			if err != nil {
				return model.Plan{}, err
			}
			return model.Plan{Command: ask.Command, Delta: u.Delta, Sections: []model.Section{{Title: "Plugin " + shown, Lines: lines}}, Inputs: u.Inputs}, nil
		},
		Render: func(model.Delta) ([]model.Target, error) { return cur.Targets, nil },
	}
	c := NewChange(p, ask.Root, spec)
	c.closing = func() (string, string) {
		if cur.Nothing {
			return "", ""
		}
		// Trust is not asked: the words say what this upgrade did, and a
		// digest approved before stays approved
		return shown + " is installed and not approved by this upgrade", "gragas plugin approve " + shown
	}
	return c, nil, nil
}

// upgradeAddAsk reads the new version's bundle from the folder source as plugin
// add reads one: source/<name>/<version>, and with version "" the newest
// version folder the source holds of name.
func upgradeAddAsk(p port.Ports, source, name, version string, ask model.ChangeAsk) (PluginAddAsk, error) {
	src, err := p.Tree.Open(source)
	if err != nil {
		return PluginAddAsk{}, fmt.Errorf("the source %s could not be opened: %w", source, err)
	}
	if version == "" {
		if version, err = newestIn(p, src, source, name); err != nil {
			return PluginAddAsk{}, err
		}
	}
	folder := path.Join(name, version)
	bounds := pluginbundle.DefaultBounds().Held()
	files, err := readBundleBelow(p, src, folder, source+"/"+folder, bounds)
	if err != nil {
		return PluginAddAsk{}, err
	}
	at, err := bundleSource(p, ask.Root, path.Join(path.Clean(string(src)), folder))
	if err != nil {
		return PluginAddAsk{}, err
	}
	return PluginAddAsk{Ask: ask, Ref: model.PluginRef{Name: name, Version: version}, Source: at, Files: files, Bounds: bounds}, nil
}

// pluginUpgradePlan reads what plugins.PlanUpgrade decides on, the lock, the
// new version's folder and the old version's files, through the Tree port,
// and decides.
func pluginUpgradePlan(p port.Ports, add PluginAddAsk, named bool) (plugins.UpgradePlan, error) {
	in, err := pluginAddInput(p, add)
	if err != nil {
		return plugins.UpgradePlan{}, err
	}
	old, err := plugins.UpgradeFrom(in.LockFile, add.Ref.Name, add.Ref.Version)
	if err != nil {
		return plugins.UpgradePlan{}, err
	}
	up := plugins.UpgradeInput{Add: in, Named: named, Old: old}
	if old != add.Ref {
		// The old bundle is read from the rulebook's own tree, where the lock
		// says it lies, within the same bounds; a folder that is gone is no
		// files, since Tree.List answers it as an empty list, and the judge
		// refuses no files as not what the lock pins.
		dir := pluginbundle.FolderOf(old)
		if up.OldFiles, err = readBundleBelow(p, add.Ask.Root, dir, dir, in.Bounds); err != nil {
			return plugins.UpgradePlan{}, err
		}
	}
	return plugins.PlanUpgrade(up)
}

// newestIn is the newest version folder source holds of the plugin name: the
// second step of the path of each file below name, of three whole numbers, by
// model.CompareVersions. A folder that holds no file is no version, and a
// version that CompareVersions does not order is refused by name
// (plugins.ErrNotSemver), since the newest could not be told.
func newestIn(p port.Ports, src model.Root, source, name string) (string, error) {
	es, err := p.Tree.List(src, name, model.Every)
	if err != nil {
		return "", fmt.Errorf("the source %s holds no %s: %w", source, name, err)
	}
	newest := ""
	for _, e := range es {
		rest, ok := strings.CutPrefix(e.Path, name+"/")
		if !ok || e.Dir {
			continue
		}
		v, below, inside := strings.Cut(rest, "/")
		if !inside || below == "" || !pluginbundle.VersionFormed(v) {
			continue
		}
		if !model.VersionOrdered(v) {
			return "", fmt.Errorf("the source %s holds %s/%s, which is %w, so the newest version cannot be told; name the version", source, modelplugin.Shown(name), modelplugin.Shown(v), plugins.ErrNotSemver)
		}
		if newest == "" || model.CompareVersions(v, newest) > 0 {
			newest = v
		}
	}
	if newest == "" {
		return "", fmt.Errorf("the source %s holds no version of %s: a version is a folder %s/<version> of three whole numbers: %w", source, name, name, fs.ErrNotExist)
	}
	return newest, nil
}

// upgradeRefusal is err as the refusal a person reads, with the line to type,
// for the four ways an upgrade is refused before it is planned: a plugin the
// lock does not hold, an older version not named, an installed bundle that is
// not what its lock entry pins, and a version, in the lock or the source, that
// is not semver. Any other error is not one.
func upgradeRefusal(err error, source string, ref model.PluginRef) (model.Refusal, bool) {
	var hint string
	switch {
	case errors.Is(err, port.ErrNotInstalled):
		// source is the host's own spelling, joined here as text and never
		// cleaned as a slash path, and quoted as one word of a shell so the
		// line pastes as typed
		hint = "gragas plugin add " + apply.ShellWord(source+"/"+ref.Name+"/"+ref.Version)
	case errors.Is(err, plugins.ErrNotPinned), errors.Is(err, plugins.ErrOlder), errors.Is(err, plugins.ErrNotSemver):
		// the refusal says what to do (name the version, for an older one or
		// one not semver), and the next line is no step back to paste
		hint = "gragas plugin list"
	default:
		return model.Refusal{}, false
	}
	r, rerr := model.NewRefusal(model.Refused, err.Error(), nil, hint)
	return r, rerr == nil
}
