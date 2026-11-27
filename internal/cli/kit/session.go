package kit

import (
	"context"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// Ending is how a run ended: the closing word, the sentence beside it and the
// line to type, which is the answer's next whether or not the screen shows it.
type Ending struct {
	Word       model.Word
	What, Next string
	// Lead are the blocks drawn above the closing word: a table, a list, a note.
	Lead []model.Block
}

// Build is the build the program was made as: its version, the commit it was
// built from ("" when the build recorded none), the Go toolchain that built
// it, as go-run's version.Build says it, and the line it is named by.
type Build struct {
	Version, Commit, Go, Line string
}

// Session is what a command package reaches of the run it is part of. The root
// fills it; a command draws and ends only through it, so it never touches a
// stream, the terminal or the exit.
type Session interface {
	Screen
	// Aside writes line on standard error, whatever the answer's form: the
	// hook hands it to the agent whose edit guard edit refuses.
	Aside(line string)
	// End draws how the run ended and returns the code it ends with.
	End(e Ending) int
	// Exit is the error a command returns to end with code.
	Exit(code int) error
	// Build is the build the program was made as.
	Build() Build
	// Line is argv as the one line a person pastes into the shell the run
	// writes its lines for, or an error that names a word no such line can
	// carry. A command then prints no line, never
	// one quoted some other way. The kit names no shell: the command tree
	// makes the line.
	Line(argv []string) (string, error)
	// Operations are the core's operations a command reaches.
	Operations
}

// Operations are the operations of the core a command reaches through the
// session, each called once.
type Operations interface {
	// Validate reads the rulebook of the working folder and says what is wrong
	// with it. An error is a rulebook that could not be read at all (not
	// ready, no rulebook).
	Validate(ctx context.Context) (model.Validation, error)
	// CheckRulebook is check in the working folder's rulebook: validate's
	// findings, then the gate over HEAD's history against the merge base of
	// HEAD and the base ref the settings name. The refusal is a run that
	// cannot be judged (no base ref, no merge base, a shallow clone, no git);
	// the error is a folder with no one rulebook, or one not ready to read.
	CheckRulebook(ctx context.Context) (model.RulebookCheck, *model.Refusal, error)
	Plans
	// Projects are the operations of check below a pin.
	Projects
	// Presets is the write of the tools' presets.
	Presets
	// Records are the operations of the history, the cache and the journal.
	Records
	// Listings are what the folder holds, listed, and the contract commands'
	// operations beside them.
	Listings
	// Guarding is what guard reaches.
	Guarding
}

// Guarding is what the guard reaches of a run: the core's judgement of the
// edit on standard input.
type Guarding interface {
	// GuardEdit reads the proposed edit from standard input, as the agent's
	// hook gives it, and judges it in the working folder: allowed with no
	// lifecycle field changed, refused otherwise, and refused whenever it
	// cannot be read or judged. The one error is a folder that holds no
	// rulebook or one not ready to read.
	GuardEdit(ctx context.Context) (model.Guard, *model.Refusal, error)
	// GuardStaged judges the change staged in the working folder's
	// repository, HEAD against the index: allowed when every lifecycle field
	// it changes is recorded by a history line it appends, and every line it
	// appends replays to its own plan digest and to the staged bytes; refused
	// otherwise, and whenever it cannot be read or replayed. The one error is
	// a folder that holds no rulebook or one not ready to read. mergeHook is
	// a run as git's pre-merge-commit hook, before git records the merge.
	GuardStaged(ctx context.Context, mergeHook bool) (model.Guard, *model.Refusal, error)
}

// Cmd is the cobra command of a run, which only the root knows the type of.
type Cmd any

// Column is s padded with spaces to width, at least one.
func Column(s string, width int) string {
	return s + strings.Repeat(" ", max(width-len([]rune(s)), 1))
}

// Package is a built command as its package declares it: Row, the one
// declaration of the command, and New, which makes its command over the session
// of a run. C is the type of the command the root's tree holds, so the kit
// itself imports no command library.
type Package[C any] struct {
	Row Command
	New func(Session) C
}

// Listings are the listings of what the working folder holds, each one
// operation of the core, called once.
type Listings interface {
	// Contracts are the contract commands' operations.
	Contracts
	// Plugins are the plugin commands' operations.
	Plugins
	// Explains are explain's and resolve's operations.
	Explains
	// Tools lists the tools the rulebook of the folder binds rules to, with the
	// entries and the rules that hold them. An error is a folder that cannot be
	// listed (no rulebook, findings, not ready).
	Tools(ctx context.Context) (model.ToolsListing, error)
	// Attention lists what needs the maintainer of the rulebook, as of the day the
	// run's clock gives. An error is a folder that cannot be listed (no
	// rulebook, not ready).
	Attention(ctx context.Context) (model.AttentionListing, error)
	// Conflicts lists the entry conflicts of the rulebook of the folder: each
	// tool entry two rules bind to different values, options or severity. An
	// error is a folder that cannot be listed (no rulebook, findings, not ready).
	Conflicts(ctx context.Context) (model.ConflictsListing, error)
	// Coverage lists the rulebook's coverage both ways: the rules a binding
	// holds, the entries they explain and the upstream entries nothing
	// explains. An error is a folder that cannot be listed (no rulebook,
	// findings, a refused or newer upstream set, not ready).
	Coverage(ctx context.Context) (model.Coverage, error)
}

// EndListing ends a run that lists: under --json the document alone, with the
// word done and what it says and nothing drawn; otherwise the lead above the
// word. It is what every listing command ends with (cli.output.1).
func EndListing(s Session, doc any, what string, lead []model.Block) int {
	if s.JSON() {
		s.Document(doc)
		return s.End(Ending{Word: model.Done, What: what})
	}
	return s.End(Ending{Word: model.Done, What: what, Lead: lead})
}

// Screen is what a run prints and the documents it answers with.
type Screen interface {
	// Say prints a line of the screen. Under --json it prints nothing, and
	// under --sarif it prints on standard error.
	Say(line string)
	// JSON is whether the run answers in JSON.
	JSON() bool
	// Document writes v as the run's one JSON document.
	Document(v any)
	// SARIF is whether the run answers in SARIF: the command was given
	// --sarif, which only a row that lists findings takes.
	SARIF() bool
	// Encoded writes doc, a document the contract encoded, as the run's one
	// answer, byte for byte.
	Encoded(doc []byte)
}

// Plans are the writes a session plans, each through the one form every plan
// command ends in.
type Plans interface {
	// Migrate plans the move of the rulebook of the working folder to the
	// format from the identity form, and runs it through the dry run, the
	// question at a terminal or the apply. cmd is the cobra command that was run,
	// which takes --apply; the line a declined run ends on is made from its flags.
	// A form the move refuses ends usage. It returns the code the run ends with.
	Migrate(ctx context.Context, cmd Cmd, form model.InitForm) int
	// SourceNew plans the record of a source. A field the request leaves empty
	// is asked at a terminal and, off one, ends the run on usage naming its flag.
	SourceNew(ctx context.Context, cmd Cmd, req model.NewSource) int
	// SourceEdit plans a change of a source's title, locator or attribution, and
	// a note on each rule that cites it.
	SourceEdit(ctx context.Context, cmd Cmd, req model.EditSource) int
	// Import plans the rules of a guide and the entries of the configs the
	// request names: a rule whose name is an entry enters active, the rest
	// proposed. A field the request leaves empty is asked at a terminal and,
	// off one, ends the run on usage naming its flag; an import with no guide
	// and no entries ends "done, nothing to import".
	Import(ctx context.Context, cmd Cmd, in model.ImportInput) int
	// DecisionNew plans the record of a decision. A field the request leaves
	// empty is asked at a terminal and, off one, ends the run on usage naming
	// its flag.
	DecisionNew(ctx context.Context, cmd Cmd, req model.NewDecisionRecord) int
	// DecisionSupersede plans a new decision that replaces the one the request
	// names: the old is marked, and each rule standing on it moves to the new.
	DecisionSupersede(ctx context.Context, cmd Cmd, req model.SupersedeDecision) int
	// Undo plans the entry that takes back the last change the history records.
	Undo(ctx context.Context, cmd Cmd) int
	// Bind plans the addition of a binding to a rule.
	Bind(ctx context.Context, cmd Cmd, req model.BindRule) int
	// Rebind plans the change of a binding of a rule.
	Rebind(ctx context.Context, cmd Cmd, req model.RebindRule) int
	// Unbind plans the removal of a binding of a rule.
	Unbind(ctx context.Context, cmd Cmd, req model.UnbindRule) int
}

// Presets is the write of the tools' presets, one operation.
type Presets interface {
	// Preset plans the write of every tool's preset from the rulebook's
	// bindings; with check it compares and writes nothing.
	Preset(ctx context.Context, cmd Cmd, check bool) int
}
