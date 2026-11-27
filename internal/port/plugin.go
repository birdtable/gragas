package port

import (
	"context"
	"errors"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
)

// Plugins is the installed plugins of one root. The
// adapter is host/plugin, the one owner of the wasm runtime; the core is tested
// with the in-memory stand-in of internal/port/testing. The port names no tool:
// a plugin is its manifest and its five operations, and which tool it serves is
// the manifest's.
type Plugins interface {
	// List is the plugins installed under root, in the order of their name
	// and then their version as bytes; none is a nil list and no error.
	List(ctx context.Context, root string) ([]model.PluginRef, error)
	// Open is the plugin ref names, its manifest read and its module held to
	// their digest, contract, features, imports and approval. A plugin that
	// fails one is not opened and no code of it has run, and every refusal
	// matches a sentinel: ErrNotInstalled, ErrManifest, ErrDigest,
	// ErrContract, ErrFeature, ErrModule or ErrNotApproved. A cancelled or
	// expired ctx ends a call or an Open with the context's own error, which
	// matches context.Canceled or context.DeadlineExceeded and no sentinel
	// here.
	Open(ctx context.Context, root string, ref model.PluginRef) (Plugin, error)
	// Try is the module wasm of a bundle under test, which no lock names,
	// compiled and held to what a plugin's module is as Open holds it
	// (gragas plugin test, C5), with m the manifest the caller read, or the
	// zero value where it read none: the grants a plan's step is held to. It
	// is plugin test's alone. It asks for no lock, no digest and no approval,
	// since the checks are what judge the bundle; it is safe because nothing
	// a trial answers is run: plugin test runs no step, and the module runs in
	// the runtime Open's do, with no file, socket or environment. A module it
	// refuses matches ErrModule and none of its code has run; a ctx that ends
	// is its own error.
	Try(ctx context.Context, m plugin.Manifest, wasm []byte) (Trial, error)
}

// Trial is a module under test (Plugins.Try): a Plugin, whose typed calls are
// Open's, and a raw call, which sends a request as it is.
type Trial interface {
	Plugin
	// Raw runs one instance of the module on request, the bytes as they are,
	// held to the limits of a call, and answers what it wrote to stdout when it
	// exited with code 0. Any other end is an error, as a typed call's: a
	// plugin.LimitError, or one matching ErrTrap. The answer is not read: what
	// it holds is the caller's to judge.
	Raw(ctx context.Context, request []byte) ([]byte, error)
}

// Plugin is one opened plugin. Every call is its own instance:
// nothing one call does is seen by another, so the engine may call
// the operations in any order, any number of times, at once. A call is held to
// the engine's limits, and every failure of it is an error: never an empty
// answer for one (a call fails closed).
//
// Each answer is one that passed its schema and the rules of the plugin contract. An
// answer that did not is an *AnswerError, whose Path points into it.
type Plugin interface {
	// Manifest is the plugin's facts, its grants and what it provides, read
	// at Open: it is never a call.
	Manifest() plugin.Manifest
	// Plan is what to run or read next to learn the tool's config, from the
	// round and what the rounds before it gathered.
	Plan(ctx context.Context, in plugin.PlanRequest) (plugin.PlanResponse, error)
	// Entries turns what the tool printed into entries, sorted by key.
	Entries(ctx context.Context, in plugin.EntriesRequest) (plugin.EntriesResponse, error)
	// Findings turns a run over fixtures into findings, sorted.
	Findings(ctx context.Context, in plugin.FindingsRequest) (plugin.FindingsResponse, error)
	// Rank says, for each pair, whether the found value is tighter or weaker
	// than the preset's: one word for each pair, in order.
	Rank(ctx context.Context, in plugin.RankRequest) (plugin.RankResponse, error)
	// Preset writes the tool's file from entries; it answers CodeRefused,
	// with the path of the option, for one it cannot write.
	Preset(ctx context.Context, in plugin.PresetRequest) (plugin.PresetResponse, error)
	// Close frees what Open took. A plugin is not called after it.
	Close() error
}

// The host's failures of a plugin call, each matched with
// errors.Is, none retried by the engine. A plugin's own error, a call over a
// limit and an answer that is no answer carry their fields: they are
// plugin.CallError, plugin.LimitError and plugin.AnswerError, taken with
// errors.As. Every failure is an error, never an empty answer.
var (
	// ErrNotInstalled is what Open's error matches for a ref no plugin is
	// installed under: the caller's mistake, never a plugin's answer.
	ErrNotInstalled = errors.New("no such plugin is installed")
	// ErrManifest is what Open's error matches for a manifest the engine
	// refuses: a key it does not know, a value its schema does not allow.
	ErrManifest = errors.New("the plugin's manifest is refused")
	// ErrNotApproved is what Open's error matches for a plugin its digest
	// has not been approved for.
	ErrNotApproved = errors.New("the plugin is not approved")
	// ErrDigest is what Open's error matches when a digest does not hold: the
	// module's is not the one its manifest names, or the bundle's is not the
	// one the lock holds.
	ErrDigest = errors.New("the plugin's digest is not the manifest's")
	// ErrContract is what Open's error matches for a manifest of another
	// contract number than the engine's.
	ErrContract = errors.New("the plugin is of another contract")
	// ErrFeature is what Open's error matches for a manifest that needs a
	// feature the engine lacks.
	ErrFeature = errors.New("the plugin needs a feature the engine lacks")
	// ErrModule is what Open's error matches for a module that imports
	// anything past WASI preview 1, or that does not compile as one.
	ErrModule = errors.New("the plugin's module is refused")
	// ErrGrant is what a call's error matches when a step names a program
	// the manifest does not grant.
	ErrGrant = errors.New("the plugin named a program it is not granted")
	// ErrStepNotReady is what a step's error matches when the tool it runs
	// or the sandbox level it needs is missing on this machine: not ready,
	// and no failure of the plugin's (host's step runner
	// answers it).
	ErrStepNotReady = errors.New("a tool or a sandbox level the step needs is missing here")
	// ErrTrap is what a call's error matches when the module crashed or left
	// with a non-zero code.
	ErrTrap = errors.New("the plugin's module crashed")
)

// StepRunner is the steps of one read, bound to one scope (Steps.Bind): the
// manifest's grants, the root, the scratch folder and the floor. It is safe
// to call at once from several goroutines.
type StepRunner interface {
	// Tool is the program of the grant whose id is the manifest's tool, as
	// Bind found it: its absolute path and the version it printed, which a
	// plan request carries as tool_version. It never fails.
	Tool() model.Found
	// Granted answers nil for a read, and for a run whose program is one of
	// the manifest's grants; one that is not matches ErrGrant. It looks at
	// nothing on disk, so every step of a plan is judged before any runs.
	Granted(s plugin.Step) error
	// Run runs or reads step s, the step-th of round round, and answers what
	// it gave: a run's exit code, its stdout and stderr of at most
	// plugin.MaxStream bytes each, cut true when either reached it (and no
	// exit code then), and the files it named to collect that it wrote; a
	// read's files. The engine sets the result's round, step and kind. A
	// program that is not granted matches ErrGrant; a tool or level this
	// machine lacks matches ErrStepNotReady, nothing started; a word, path or
	// stdin past its cap is refused, matching ErrRefused, nothing started. A
	// run is journalled by the scope's note before anything of it is placed
	// or started, and a note that fails starts nothing. A run that ends past
	// its deadline is an error. ctx ending first is its own error.
	Run(ctx context.Context, round, step int, s plugin.Step) (plugin.Result, error)
}
