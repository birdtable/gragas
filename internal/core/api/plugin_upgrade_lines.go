package coreapi

import (
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model"
	modelplugin "github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/pluginbundle"
)

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
