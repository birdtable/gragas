package kit

import (
	"context"

	"github.com/birdtable/gragas/internal/model"
)

// Writes is the annotation a command that can write carries; the root's own
// is the same word, and a test of the root holds them equal.
const Writes = "writes"

// Contracts are the operations of the contract commands, each one operation of
// the core, called once. A kind or an entity that is none, and a document in a
// newer major, are told in the answer and are no error; an error is a read or
// a write that failed.
type Contracts interface {
	// ContractValidate checks the file at file as a document of kind, or of
	// the kind its fields name when kind is empty.
	ContractValidate(ctx context.Context, file, kind string) (model.ContractCheck, error)
	// ContractFields lists the fields of the contract's entity with the
	// version each came in.
	ContractFields(ctx context.Context, entity string) (model.ContractFields, error)
	// ContractFixture writes the fixture's documents into the folder out, or
	// with apply false lists what it would write and writes nothing.
	ContractFixture(ctx context.Context, out string, apply bool) (model.ContractFixture, error)
}

// Plugins are the operations of the plugin commands, each one operation of the
// core, called once.
type Plugins interface {
	// PluginTest reads the plugin bundle in dir and holds it to checks C1 to
	// C23 of the plugin design. A bundle that fails a check is told in the
	// answer and is no error; an error is a bundle that cannot be read.
	PluginTest(ctx context.Context, dir string) (model.PluginTest, error)
	// PluginList lists the plugins the working folder's lock names, each with the
	// tool it serves, its digest as read on disk and whether that digest is
	// approved on this machine. An error is a lock or a bundle that cannot be
	// read, or approvals that cannot.
	PluginList(ctx context.Context) (model.PluginsListing, error)
	// PluginApproval says what an approval of the plugin ref names (a name, or
	// name@version) covers, from its manifest alone; a bundle that changed or
	// whose manifest is refused is the judge's error and no grants. Errors that
	// match coreapi.ErrNoSuchPlugin and ErrManyVersions say which plugin is meant.
	PluginApproval(ctx context.Context, ref string) (model.PluginApproval, error)
	// PluginApprove records the approval of ref through the machine's trust
	// record when d is the installed bundle's digest, and writes nothing
	// otherwise (coreapi.ErrDigestGiven).
	PluginApprove(ctx context.Context, ref string, d model.Digest) (model.PluginApproval, error)
	// PluginRecord plans the recording of the plugin in the author's folder dir and,
	// under --apply, runs the real tool and files the recording and the case in
	// it. It never asks: --apply says yes.
	PluginRecord(ctx context.Context, cmd Cmd, dir string) int
	// PluginAdd plans the add of the bundle in dir, <name>/<version> of a plugin,
	// to the rulebook: its files and then the lock. It writes only under --apply
	// and approves nothing.
	PluginAdd(ctx context.Context, cmd Cmd, dir string) int
	// PluginUpgrade plans the upgrade of the installed plugin ref, <name> or
	// <name>@<version>, to the bundle source/<name>/<version>: the add of it,
	// the grants changed, and the old version's files removed. It writes only
	// under --apply and approves nothing.
	PluginUpgrade(ctx context.Context, cmd Cmd, source, ref string) int
}
