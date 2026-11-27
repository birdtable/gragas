package coreapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
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

// upgradeLines are the plan's lines: the two digests, each grant that
// changed with what moved in it and the command that asks its version, each
// way past the bundle that moved, the files and the lock, what stays, and that
// the upgrade approves nothing.
func upgradeLines(u plugins.UpgradePlan) ([]string, error) {
	if u.Nothing {
		return []string{"nothing to do: " + u.Old.Name + "@" + u.Old.Version + " is installed at these files"}, nil
	}
	ref := u.NewManifest.Name + "@" + u.NewManifest.Version
	lines := []string{
		"from " + u.Old.Name + "@" + u.Old.Version + ": " + string(u.OldDigest),
		"to " + ref + ": " + string(u.Digest),
	}
	if u.Older {
		lines = append(lines, ref+" is older than "+u.Old.Version+", as named")
	}
	grants, err := grantLines(u.OldManifest, u.NewManifest)
	if err != nil {
		return nil, err
	}
	lines = append(lines, grants...)
	lines = append(lines, linkLines(u.OldManifest, u.NewManifest)...)
	for _, c := range u.Delta.Changes {
		switch {
		case c.After != "":
			lines = append(lines, c.Entity+": "+c.After)
		case c.Field == "lock":
			lines = append(lines, c.Entity+": lock entry removed")
		default:
			lines = append(lines, c.Entity+": removed")
		}
	}
	for _, s := range u.Stays {
		lines = append(lines, s.Name+"@"+s.Version+" stays: the lock names it")
	}
	return append(lines, "this approves nothing: "+ref+" is installed and not approved by this upgrade; gragas plugin approve "+ref+" shows what an approval covers"), nil
}

// grantLines are the grants added, removed or changed between two manifests,
// compared as plugin approve compares an approval with a bundle
// (changesBetween), each with the command that asks its program's version.
func grantLines(old, now modelplugin.Manifest) ([]string, error) {
	before, err := pluginbundle.GrantsHeld(old)
	if err != nil {
		return nil, err
	}
	after, err := pluginbundle.GrantsHeld(now)
	if err != nil {
		return nil, err
	}
	changes := changesBetween(before, after)
	if len(changes) == 0 {
		return []string{"grants: none added, removed or changed"}, nil
	}
	was, is := grantsByID(old), grantsByID(now)
	lines := make([]string, 0, len(changes))
	for _, c := range changes {
		switch c.Change {
		case model.GrantRemoved:
			lines = append(lines, "grant "+c.ID+" removed: its version was asked with "+argvShown(was[c.ID]))
		case model.GrantAdded:
			lines = append(lines, "grant "+c.ID+" added: its version is asked with "+argvShown(is[c.ID]))
		default:
			lines = append(lines, "grant "+c.ID+" changed: "+grantMoved(was[c.ID], is[c.ID]))
		}
	}
	return lines, nil
}

// grantMoved is what moved in a grant that kept its id, read from the two
// grants as plugin approve shows them (grantOf): how its program is found,
// its place, its script's manifest, pointer and interpreter, its version
// range and pattern, each as it is now and as it was, and then the command
// that asks its version. A grant whose digest moved in nothing the approval
// shows by field says plugin approve shows the rest.
func grantMoved(was, is model.PluginGrant) string {
	var parts []string
	for _, f := range []struct{ what, was, is string }{
		{"found by", was.Where, is.Where},
		{"path", was.Path, is.Path},
		{"manifest", was.Manifest, is.Manifest},
		{"script at", was.ScriptAt, is.ScriptAt},
		{"interpreter", was.Interpreter, is.Interpreter},
		{"versions", was.Versions, is.Versions},
		{"version pattern", was.VersionPattern, is.VersionPattern},
	} {
		if f.was != f.is {
			parts = append(parts, f.what+" "+orNone(f.is)+" (was "+orNone(f.was)+")")
		}
	}
	argv := "its version is asked with " + argvShown(is)
	same := slices.Equal(was.VersionArgv, is.VersionArgv)
	if !same {
		argv += " (was " + argvShown(was) + ")"
	}
	parts = append(parts, argv)
	if same && len(parts) == 1 {
		parts = append(parts, "plugin approve shows the rest")
	}
	return strings.Join(parts, "; ")
}

// linkLines are a line for each way a run reaches past the bundle that moved
// between two manifests, as plugin approve shows them (linksShown): the folder
// a run links in and the tool's install words, and the config the project's
// config is wired to extend.
func linkLines(old, now modelplugin.Manifest) []string {
	var lines []string
	wasFrom, wasInstall := linksShown(old)
	isFrom, isInstall := linksShown(now)
	if wasFrom != isFrom {
		lines = append(lines, "resolve_from changed: "+orNone(isFrom)+" (was "+orNone(wasFrom)+")")
	}
	if !slices.Equal(wasInstall, isInstall) {
		lines = append(lines, "the install words changed: "+orNone(strings.Join(isInstall, " "))+" (was "+orNone(strings.Join(wasInstall, " "))+")")
	}
	if was, is := importShown(old), importShown(now); was != is {
		lines = append(lines, "the wiring import changed: "+orNone(is)+" (was "+orNone(was)+")")
	}
	return lines
}

// importShown is where a manifest's wiring says the project's config names the
// config it extends, and in what form, as shown text; "" for none.
func importShown(m modelplugin.Manifest) string {
	if m.Wiring == nil || m.Wiring.Import == nil {
		return ""
	}
	return modelplugin.Shown(m.Wiring.Import.At) + " as " + modelplugin.Shown(string(m.Wiring.Import.Form))
}

// grantsByID is each grant of m as the screen shows it, by its shown id.
func grantsByID(m modelplugin.Manifest) map[string]model.PluginGrant {
	out := make(map[string]model.PluginGrant, len(m.Programs))
	for _, p := range m.Programs {
		g := grantOf(p)
		out[g.ID] = g
	}
	return out
}

// argvShown is the arguments a grant's program is asked its version with, as
// one line.
func argvShown(g model.PluginGrant) string {
	if len(g.VersionArgv) == 0 {
		return "no arguments"
	}
	return strings.Join(g.VersionArgv, " ")
}
