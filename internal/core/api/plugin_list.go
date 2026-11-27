package coreapi

import (
	"context"

	"github.com/birdtable/gragas/internal/model"
	modelplugin "github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// ListPlugins lists the plugins the lock of root names, each with the tool its
// manifest serves, its digest as the bundle lies on disk and whether that
// digest is approved on this machine. It reads through
// Bundles.List and Bundles.Read and judges nothing it shows as fit to run: a
// bundle whose bytes are not its lock's, or whose manifest is refused, is a row
// with a note, and is not approved. A bundle that cannot be read, a lock that
// cannot, and a record of approvals that cannot be read are errors; the listing
// never says a plugin is unapproved for a record it could not read.
func ListPlugins(ctx context.Context, b port.Bundles, t port.Trust, root model.Root) (model.PluginsListing, error) {
	var out model.PluginsListing
	refs, err := b.List(ctx, string(root))
	if err != nil {
		return out, err
	}
	for _, ref := range refs {
		bundle, err := b.Read(ctx, string(root), ref)
		if err != nil {
			return out, err
		}
		row := model.PluginRow{Name: ref.Name, Version: ref.Version, Digest: bundle.Read}
		m, err := pluginbundle.Judge(ref, bundle)
		judged := err == nil
		if err != nil {
			row.Note = modelplugin.Shown(err.Error())
		} else {
			row.Tool = m.Tool
		}
		ok, err := t.Approved(ctx, bundle.Read)
		if err != nil {
			return out, err
		}
		row.Approved = ok && judged && bundle.Read == bundle.Locked
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// PluginList is ListPlugins over the ports the run was handed: Bundles for the
// lock and its bundles, Trust for the approvals.
func PluginList(ctx context.Context, p port.Ports, root model.Root) (model.PluginsListing, error) {
	return ListPlugins(ctx, p.Bundles, p.Trust, root)
}
