// Package kit holds what every command of gragas shares: the row that declares
// a command and the registry that holds the rows to the rules of
// cli.commands.26. It imports no library and no adapter, so a command package
// below internal/cli/cmd and the root both build on it.
package kit

// The command registry (cli.commands.26): every command gragas decided on,
// declared once. Help, the bare menu, the README's command table, docs/commands.md
// and the command-tree file are all made from it, so a document says only what
// the code does (cli.commands.9): a command decided and not built is a row here
// marked not built and is absent from the command tree, and a test fails when
// one appears in the tree. A command enters the tree in the milestone that
// builds it and its row changes state in the same change.

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
)

// Context is where a command runs (spec 15.1): R a rulebook's repository,
// P a project, B both, A anywhere, also with neither.
type Context string

// The four contexts.
const (
	InRulebook Context = "R"
	InProject  Context = "P"
	InBoth     Context = "B"
	Anywhere   Context = "A"
)

// Ends is what a command leaves on the screen: a plan shown before a yes, a
// report that can be written as a document, or a listing, read and gone.
type Ends string

// The three endings.
const (
	EndsPlan    Ends = "plan"
	EndsReport  Ends = "report"
	EndsListing Ends = "listing"
)

// State is where a command stands in the tree.
type State string

// The three states. Built is in the tree. Legacy is in the tree as the old
// command mounted from internal/cli/legacy until M6 replaces it. NotBuilt is
// decided and absent from the tree.
const (
	Built    State = "built"
	Legacy   State = "legacy"
	NotBuilt State = "not built"
)

// FailsClosed is the annotation of a command whose crash is a no: the
// root's one recover ends it refused, exit 5, and not failed, and the
// annotation's value is the line to type beside the word. Only
// guard edit carries it, so an agent's hook never reads a crashed guard as a
// pass; every other command's crash ends failed.
const FailsClosed = "fails-closed"

// Example is one line to type and what it does.
type Example struct{ Line, Does string }

// Command is one row of the registry.
type Command struct {
	// Name is the first word typed: a group (rule, term, source, decision,
	// contract) or a flat verb.
	Name string
	// Then are the words that may follow Name. After a group they are its
	// verbs, from the base list. After a flat command they are its modes
	// (journal list, cache clear). A family is one row.
	Then []string
	// Spec is whether the row is one of the commands of spec 15.1. A legacy
	// command the spec does not name (gen) is a row with Spec false.
	Spec bool
	// Context, Ends and OptIn: where it runs, what it ends in, and whether it
	// ends in a plan only in its opt-in form (agents, review) or its accept
	// form (docs), whose plain form ends in a listing or a report.
	Context Context
	Ends    Ends
	OptIn   bool
	// Summary is one sentence for help, the menu and the README.
	Summary string
	// Examples are two lines a person can type.
	Examples [2]Example
	// Writes are the files a run of it writes, in the words of the format
	// spec. A command that ends in a listing and writes nothing has none.
	Writes []string
	// Exits are the closing words it ends on. Any command may also end
	// usage, busy or not ready (spec 15.1).
	Exits []model.Word
	// JSONKind is the document its --json answer holds: plan, report,
	// proposal, changelog, sarif, or the command's own listing.
	JSONKind string
	// SARIF is whether it lists findings, and so takes --sarif: its answer is
	// then one SARIF document on stdout (spec 15.5). A row that takes it ends
	// on findings among its words.
	SARIF bool
	// NotYet are the words of Then that are not built while the row is: a
	// family whose first verb ships before its last. Each stays on the
	// not-built list as "name word" and in the documents, and is absent from
	// the command tree.
	NotYet []string
	State  State
	// Order is the row's place in the table of spec 15.1, so two commands
	// cannot claim one slot. A row declared with none takes the next free
	// slot when the registry is assembled.
	Order int
	// Menu says whether and where the bare menu offers the command.
	Menu Menu
}

// Menu is how the bare menu offers a command; the zero Menu is not offered.
// Lead names the group of a place that leads with the command (rulebook,
// project or start), Rank its position among the commands that group leads
// with by this field, after those its hand-kept list names. A command with no
// Lead is offered after the groups, where its context fits the place.
type Menu struct {
	Offered bool
	Lead    string
	Rank    int
}

// Slot is the distance between the Orders the registry gives a row that
// declares none, so a command that takes a slot of its own has room.
const Slot = 10

// EndsInPlan is whether a run of it can end in a plan: the 23 of spec 15.1.
func (c Command) EndsInPlan() bool { return c.Ends == EndsPlan || c.OptIn }

// Groups are the commands that take a verb: the rest are flat (cli.commands.26).
func Groups() []string {
	return []string{"rule", "term", "source", "decision", "contract", "plugin"}
}

// Refused are the verbs the registry never takes: get, make and destroy say
// nothing a plain imperative of the base list does not say better.
func Refused() []string { return []string{"get", "make", "destroy"} }

// BaseVerbs is the one list every command word comes from. Adding a word is a
// reviewed line here.
func BaseVerbs() []string {
	return []string{
		"add", "agents", "approve", "archive", "attention", "baseline", "bind", "cache", "changelog", "check",
		"clear", "confirm", "conflicts", "coverage", "diff", "docs", "edit", "exclude", "explain",
		"explorer", "fields", "find", "fix", "fixture", "gen", "guard", "handbook", "health", "help",
		"history", "import", "include", "index", "init", "journal", "list", "migrate", "new", "preset", "promote",
		"propose", "prove", "rebind", "record", "recover", "release", "renumber", "replace", "report", "resolve",
		"restore", "retire", "review", "rules", "sarif", "show", "signals", "staged", "status", "supersede",
		"switch", "test", "tools", "unbind", "undo", "upgrade", "validate", "verify", "version", "waive",
		"withdraw",
	}
}

// Registry is the rows and nothing else; build one with NewRegistry.
type Registry struct{ rows []Command }

// NewRegistry holds rows to the registry's rules: every word is on the base
// list or is a group, none is a refused verb, a name is declared once, a group
// is one of the six, a flat command takes no verb of a group, a row has two
// examples that begin with its name, and exit words come from the eight.
func NewRegistry(rows []Command) (Registry, error) {
	var errs []error
	seen := map[string]bool{}
	for _, c := range rows {
		for _, p := range c.problems(seen[c.Name]) {
			errs = append(errs, fmt.Errorf("%s: %s", c.Name, p))
		}
		seen[c.Name] = true
	}
	slots := map[int]string{}
	for _, c := range rows {
		if c.Order < 0 {
			errs = append(errs, fmt.Errorf("%s: the order %d is below 0", c.Name, c.Order))
		}
		if prev, twice := slots[c.Order]; c.Order != 0 && twice {
			errs = append(errs, fmt.Errorf("%s: the order %d is %s's already", c.Name, c.Order, prev))
		}
		slots[c.Order] = c.Name
	}
	if len(errs) > 0 {
		return Registry{}, errors.Join(errs...)
	}
	return Registry{rows: slices.Clone(rows)}, nil
}

// problems are what is wrong with a row, given whether its name was declared before.
func (c Command) problems(twice bool) []string {
	var out []string
	fail := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	for _, w := range append([]string{c.Name}, c.Then...) {
		switch {
		case slices.Contains(Refused(), w):
			fail("the verb %q is refused", w)
		case !slices.Contains(BaseVerbs(), w) && !slices.Contains(Groups(), w):
			fail("%q is not on the base list of verbs", w)
		}
	}
	if twice {
		fail("declared twice")
	}
	if slices.Contains(Groups(), c.Name) && len(c.Then) == 0 {
		fail("a group with no verb")
	}
	for i, e := range c.Examples {
		if e.Line != "gragas "+c.Name && !strings.HasPrefix(e.Line, "gragas "+c.Name+" ") || e.Does == "" {
			fail("example %d must be a line that starts with gragas %s and say what it does", i+1, c.Name)
		}
	}
	if c.Summary == "" || c.JSONKind == "" || len(c.Exits) == 0 || c.Context == "" || c.Ends == "" || c.State == "" {
		fail("a field is empty")
	}
	for _, w := range c.NotYet {
		if !slices.Contains(c.Then, w) {
			fail("%q is not built yet and is not one of its words", w)
		}
	}
	return append(out, c.exitProblems()...)
}

// exitProblems are what is wrong with a row's closing words.
func (c Command) exitProblems() []string {
	var out []string
	for _, w := range c.Exits {
		if !model.Known(w) {
			out = append(out, fmt.Sprintf("%q is not a closing word", w))
		}
	}
	if c.SARIF && !slices.Contains(c.Exits, model.Findings) {
		out = append(out, "it takes --sarif and never ends on findings")
	}
	return out
}

// Rows are the commands in the order of spec 15.1.
func (r Registry) Rows() []Command { return slices.Clone(r.rows) }

// Find is the row named name.
func (r Registry) Find(name string) (Command, bool) {
	for _, c := range r.rows {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

// InTree are the rows that belong in the command tree: built and legacy.
func (r Registry) InTree() []Command {
	var out []Command
	for _, c := range r.rows {
		if c.State != NotBuilt {
			out = append(out, c)
		}
	}
	return out
}

// NotBuilt is the not-built list: decided, absent from the tree.
func (r Registry) NotBuilt() []Command {
	var out []Command
	for _, c := range r.rows {
		if c.State == NotBuilt {
			out = append(out, c)
		}
	}
	return out
}

// NotBuiltWords are the not-built list as it is typed: a row not built, and
// each word of a built family that is not built yet.
func (r Registry) NotBuiltWords() []string {
	var out []string
	for _, c := range r.rows {
		if c.State == NotBuilt {
			out = append(out, c.Usage())
			continue
		}
		for _, w := range c.NotYet {
			out = append(out, c.Name+" "+w)
		}
	}
	return out
}

// TreeProblems lists where tree and registry disagree: inTree is the names of
// the commands the tree holds, and it is held against the rows. A command in
// the tree the registry does not list or lists as not built, and a built or
// legacy row the tree lacks. cobra's own help and completion are not commands
// of ours, so the caller leaves them out.
func (r Registry) TreeProblems(inTree []string) []string {
	var out []string
	has := map[string]bool{}
	for _, name := range inTree {
		if name == "help" || name == "completion" {
			continue
		}
		has[name] = true
		row, ok := r.Find(name)
		switch {
		case !ok:
			out = append(out, name+" is in the tree and not in the registry")
		case row.State == NotBuilt:
			out = append(out, name+" is on the not-built list and in the tree")
		}
	}
	for _, c := range r.InTree() {
		if c.Name != "help" && !has[c.Name] {
			out = append(out, c.Name+" is "+string(c.State)+" in the registry and not in the tree")
		}
	}
	slices.Sort(out)
	return out
}

// Usage is how a row is typed: a group or flat name, and its words.
func (c Command) Usage() string {
	if len(c.Then) == 0 {
		return c.Name
	}
	return c.Name + " (" + strings.Join(c.Then, ", ") + ")"
}

// Of is a registry of rows as they are, unchecked: NewRegistry is the check,
// and a test holds the rows of the program to it.
func Of(rows []Command) Registry { return Registry{rows: rows} }

// NotYetExample is whether the example types a word of the row that is not
// built yet, which help must not offer (cli.commands.9).
func (c Command) NotYetExample(e Example) bool {
	f := strings.Fields(e.Line)
	return len(f) > 2 && slices.Contains(c.NotYet, f[2])
}

// Assemble is the registry of the rows a table declares and the rows the command
// packages declare, in the order of their Order. A row with none takes the next
// free slot of Slot, counting from the first, so the table's rows keep their
// order and a package that claims a slot leaves it free of them. It checks
// nothing: NewRegistry is the check, and a test holds the program's rows to it.
func Assemble(table, packages []Command) Registry {
	taken := map[int]bool{}
	for _, c := range slices.Concat(table, packages) {
		if c.Order != 0 {
			taken[c.Order] = true
		}
	}
	rows := slices.Concat(table, packages)
	next := 0
	for i := range rows {
		if rows[i].Order != 0 {
			continue
		}
		for next += Slot; taken[next]; next += Slot {
		}
		rows[i].Order = next
	}
	slices.SortStableFunc(rows, func(a, b Command) int { return a.Order - b.Order })
	return Registry{rows: rows}
}

// Listing is the row of a command that lists, in a rulebook and in a project,
// and ends done: its name and place in the table, its summary and its two
// examples, and its listing as its own --json document (cli.commands.26).
func Listing(name string, order int, summary string, examples [2]Example) Command {
	return Command{Name: name, Order: order, Spec: true, Context: InBoth, Ends: EndsListing, State: Built,
		Summary: summary, Examples: examples, Exits: []model.Word{model.Done}, JSONKind: name}
}
