package coreapi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	modelplugin "github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// ErrNoSuchPlugin is a reference that names no plugin the lock holds, and
// ErrManyVersions one that names a plugin the lock holds in more than one
// version, without saying which. Both end the run refused, with nothing read
// further and nothing recorded.
var (
	ErrNoSuchPlugin = errors.New("no such plugin")
	ErrManyVersions = errors.New("more than one version")
	// ErrDigestGiven is a digest the person gave that is not the installed
	// bundle's: the approval is of the bytes shown, and these are others.
	ErrDigestGiven = errors.New("the digest given is not the installed bundle's")
)

// ShowPlugin is what an approval of the plugin ref names covers, read from its
// manifest alone. ref is a name, or a name and a version as name@version; a name the lock holds in more than one version needs the
// version (ErrManyVersions). The bundle is judged first, by pluginbundle.Judge:
// a bundle that changed since it was locked or whose manifest is refused is
// that error, and no screen of grants is built for it, so a refused manifest is
// never shown for a yes. Every text is written by plugin.Shown. A record of
// approvals that cannot be read is an error, never "not approved".
func ShowPlugin(ctx context.Context, b port.Bundles, t port.Trust, root model.Root, ref string) (model.PluginApproval, error) {
	var out model.PluginApproval
	which, err := pluginRefOf(ctx, b, root, ref)
	if err != nil {
		return out, err
	}
	bundle, err := b.Read(ctx, string(root), which)
	if err != nil {
		return out, err
	}
	m, err := pluginbundle.Judge(which, bundle)
	if err != nil {
		return out, err
	}
	g, err := pluginbundle.GrantsDigest(m)
	if err != nil {
		return out, err
	}
	held, err := pluginbundle.GrantsHeld(m)
	if err != nil {
		return out, err
	}
	ok, err := t.Approved(ctx, bundle.Read)
	if err != nil {
		return out, err
	}
	since, serr := grantsSince(ctx, t, m.Name, held)
	if serr != nil {
		return out, serr
	}
	out = model.PluginApproval{
		Name: modelplugin.Shown(m.Name), Version: modelplugin.Shown(m.Version),
		Digest: bundle.Read, GrantsDigest: g, Held: held, Since: since,
		Tool: modelplugin.Shown(m.Tool), ToolVersions: modelplugin.Shown(m.ToolVersions),
		Approved: ok,
	}
	for _, p := range m.Programs {
		out.Grants = append(out.Grants, grantOf(p))
	}
	if m.Preset != nil {
		out.Preset = modelplugin.Shown(m.Preset.Path)
	}
	out.ResolveFrom, out.Install = linksShown(m)
	return out, nil
}

// linksShown is what an approval shows of how a run reaches past the bundle:
// the folder it links in, "" for none, and the tool's install words, empty
// for none. Plugin upgrade compares the two of the old and the new manifest.
func linksShown(m modelplugin.Manifest) (resolveFrom string, install []string) {
	install = []string{}
	if m.ResolveFrom != nil {
		resolveFrom = modelplugin.Shown(*m.ResolveFrom)
	}
	if m.Wiring != nil {
		for _, w := range m.Wiring.Install {
			install = append(install, modelplugin.Shown(w))
		}
	}
	return resolveFrom, install
}

// ApprovePlugin records the approval of the plugin ref names through Trust,
// when digest is the installed bundle's: the bytes the person read are the
// bytes approved. A digest that is not (ErrDigestGiven) changes nothing, and
// there is no default: an empty digest is the same refusal. It answers what
// ShowPlugin does, with Recorded and Approved true.
func ApprovePlugin(ctx context.Context, b port.Bundles, t port.Trust, root model.Root, ref string, digest model.Digest) (model.PluginApproval, error) {
	got, err := ShowPlugin(ctx, b, t, root, ref)
	if err != nil {
		return got, err
	}
	if digest != got.Digest {
		if digest == "" {
			return got, fmt.Errorf("%w: it is %s, and no digest was given", ErrDigestGiven, got.Digest)
		}
		return got, fmt.Errorf("%w: it is %s, and %s was given", ErrDigestGiven, got.Digest, modelplugin.Shown(string(digest)))
	}
	err = t.Approve(ctx, model.Approval{Digest: got.Digest, Ref: got.Name + "@" + got.Version, Grants: got.GrantsDigest, Held: got.Held})
	if err != nil {
		return got, err
	}
	got.Approved, got.Recorded = true, true
	return got, nil
}

// PluginApproval is ShowPlugin over the ports the run was handed.
func PluginApproval(ctx context.Context, p port.Ports, root model.Root, ref string) (model.PluginApproval, error) {
	return ShowPlugin(ctx, p.Bundles, p.Trust, root, ref)
}

// PluginApprove is ApprovePlugin over the ports the run was handed.
func PluginApprove(ctx context.Context, p port.Ports, root model.Root, ref string, digest model.Digest) (model.PluginApproval, error) {
	return ApprovePlugin(ctx, p.Bundles, p.Trust, root, ref, digest)
}

// pluginRefOf finds the one locked plugin ref names.
func pluginRefOf(ctx context.Context, b port.Bundles, root model.Root, ref string) (model.PluginRef, error) {
	refs, err := b.List(ctx, string(root))
	if err != nil {
		return model.PluginRef{}, err
	}
	name, version, versioned := strings.Cut(ref, "@")
	var hit []model.PluginRef
	for _, r := range refs {
		if r.Name == name && (!versioned || r.Version == version) {
			hit = append(hit, r)
		}
	}
	switch len(hit) {
	case 0:
		return model.PluginRef{}, fmt.Errorf("%w: the lock holds none called %s", ErrNoSuchPlugin, modelplugin.Shown(ref))
	case 1:
		return hit[0], nil
	}
	vs := make([]string, len(hit))
	for i, r := range hit {
		vs[i] = modelplugin.Shown(r.Name + "@" + r.Version)
	}
	slices.Sort(vs)
	return model.PluginRef{}, fmt.Errorf("%w: the lock holds %s; say which", ErrManyVersions, strings.Join(vs, ", "))
}

// grantOf is one program of a manifest as the screen shows it.
func grantOf(p modelplugin.Program) model.PluginGrant {
	var g model.PluginGrant
	var pattern *string
	switch {
	case p.Found != nil:
		g.ID, g.Versions, g.VersionArgv, pattern = p.Found.ID, p.Found.Versions, p.Found.VersionArgv, p.Found.VersionPattern
		g.Where, g.WholeLine = "pinned", true
	case p.At != nil:
		g.ID, g.Versions, g.VersionArgv, pattern = p.At.ID, p.At.Versions, p.At.VersionArgv, p.At.VersionPattern
		g.Where, g.Path, g.WholeLine = "path", modelplugin.Shown(p.At.Path), true
	case p.Script != nil:
		g.ID, g.Versions, g.VersionArgv, pattern = p.Script.ID, p.Script.Versions, p.Script.VersionArgv, p.Script.VersionPattern
		g.Where = "script"
		g.Manifest, g.ScriptAt, g.Interpreter = modelplugin.Shown(p.Script.Manifest), modelplugin.Shown(p.Script.ScriptAt), modelplugin.Shown(p.Script.Interpreter)
	}
	g.ID, g.Versions = modelplugin.Shown(g.ID), modelplugin.Shown(g.Versions)
	argv := make([]string, len(g.VersionArgv))
	for i, a := range g.VersionArgv {
		argv[i] = modelplugin.Shown(a)
	}
	g.VersionArgv = argv
	if pattern != nil {
		g.VersionPattern = modelplugin.Shown(*pattern)
	}
	vars := p.CacheVars()
	g.CacheVars = make([]string, len(vars))
	for i, v := range vars {
		g.CacheVars[i] = modelplugin.Shown(v)
	}
	return g
}

// PluginRefused is whether err is a refusal of the person's request, with
// nothing recorded and nothing wrong with the machine: a plugin that is not
// meant, a digest that is not the bundle's, or a bundle the judge refuses. Any
// other error is a failure: a record or a bundle that could not be read.
func PluginRefused(err error) bool {
	for _, target := range []error{ErrNoSuchPlugin, ErrManyVersions, ErrDigestGiven, port.ErrDigest, port.ErrManifest, port.ErrModule, port.ErrContract} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
