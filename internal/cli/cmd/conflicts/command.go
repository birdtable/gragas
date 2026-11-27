// Package conflicts is the conflicts command: in a rulebook it lists the tool
// entries that two rules bind to different values, options or severity, and
// offers the three ways to resolve each. It lives in a package of its own below
// internal/cli/cmd; the root joins it from the generated join_gen.go, which
// make gen writes, so a new command is a new folder and no shared line. It
// reads nothing itself: it asks the session, which asks the core, and draws what
// comes back. It picks no winner (S8): the listing offers, a person's plan
// decides.
package conflicts

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 180

// Row is the one declaration of conflicts (cli.commands.20, .26).
func Row() kit.Command {
	r := kit.Listing("conflicts", Order, "List rules whose settings disagree and offer the three resolutions", [2]kit.Example{
		{Line: "gragas conflicts", Does: "list them"},
		{Line: "gragas conflicts --json", Does: "as one JSON document"},
	})
	// It lists a rulebook's conflicts, not a project's, ends findings when one
	// is open, and takes --sarif.
	r.Context, r.Exits, r.SARIF = kit.InRulebook, []model.Word{model.Done, model.Findings}, true
	return r
}

// long is the help text of the command.
const long = `Lists each tool entry that two rules of this rulebook bind to
different values, options or severity, with the rules that hold it
and what they differ in. The same value held by two rules is not a
conflict. Values are compared by the adapter's own scale, so two
spellings the adapter reads as off are one value. Until a conflict
is resolved both bindings are blocked, and no preset is generated
for the entry. A conflict that a decision settles, by naming the
winning binding of rules that declare a conflicts relation, is
listed as resolved. It offers three resolutions and picks none:
each is a plan of its own. It reads and changes nothing.

--sarif writes each open conflict as one SARIF document.

  gragas conflicts
  gragas conflicts --json`

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	c := &cobra.Command{Use: "conflicts", Args: cobra.NoArgs}
	c.Short, c.Long = Row().Summary, long
	c.RunE = listing{s}.run
	return c
}

// listing is a run of the command: the session it draws through.
type listing struct{ s kit.Session }

func (l listing) run(c *cobra.Command, _ []string) error {
	found, err := l.s.Conflicts(c.Context())
	if err != nil {
		return l.s.Exit(l.notReady(err))
	}
	return l.s.Exit(l.end(found))
}

// end answers with the listing in the form the run asks for, and ends on the
// word its conflicts give: the document or the SARIF alone where it is asked
// for, the screen otherwise.
func (l listing) end(found model.ConflictsListing) int {
	e := kit.Ending{Word: found.Word(), What: said(found)}
	switch {
	case l.s.JSON():
		l.s.Document(coreapi.ConflictsDocument(found)) // the end draws nothing under --json
	case l.s.SARIF():
		doc, err := coreapi.ConflictsSARIF(found, l.s.Build().Version)
		if err != nil {
			return l.s.End(kit.Ending{Word: model.Failed, What: "the conflicts could not be written as SARIF: " + err.Error()})
		}
		l.s.Encoded(doc) // the end is on standard error under --sarif
	default:
		e.Next, e.Lead = next(found), lead(found)
	}
	return l.s.End(e)
}

// notReadyCase is one way a folder's conflicts cannot be listed: how to tell it
// by its error, what to say of it, and the line to type after.
type notReadyCase struct {
	is   func(error) bool
	what func(error) string
	next string
}

// notReadyCases are the ways in the order they are told, after the plugins'
// (coreapi.PluginNext): an open run, a rulebook newer than this engine, no
// rulebook or two, and a rulebook with findings.
func notReadyCases() []notReadyCase {
	var findings *coreapi.FindingsError
	text := func(err error) string { return err.Error() }
	return []notReadyCase{
		{coreapi.IsOpenRecord, coreapi.Reason, coreapi.RecoverLine},
		{func(err error) bool { return errors.Is(err, coreapi.ErrNotReady) }, coreapi.Reason, "gragas version"},
		{func(err error) bool { return errors.Is(err, coreapi.ErrNoBook) || errors.Is(err, coreapi.ErrTwoBooks) },
			func(err error) string { return err.Error() + ". conflicts lists a rulebook's conflicts" }, "gragas init"},
		{func(err error) bool { return errors.As(err, &findings) }, text, "gragas validate"},
	}
}

// notReady ends the run on a folder whose conflicts cannot be listed: not ready
// for a value no installed plugin ranks, a plugin not approved, and each of the
// ways, failed for any other.
func (l listing) notReady(err error) int {
	if next, ok := coreapi.PluginNext(err); ok {
		return l.s.End(kit.Ending{Word: model.NotReady, What: err.Error(), Next: next})
	}
	for _, c := range notReadyCases() {
		if c.is(err) {
			return l.s.End(kit.Ending{Word: model.NotReady, What: c.what(err), Next: c.next})
		}
	}
	return l.s.End(kit.Ending{Word: model.Failed, What: "the conflicts could not be listed: " + err.Error()})
}

// said is the sentence beside the closing word.
func said(l model.ConflictsListing) string {
	if len(l.Conflicts) == 0 {
		return "no conflicts"
	}
	return fmt.Sprintf("%s open, %d resolved", model.Plural(l.Open(), "conflict"), len(l.Conflicts)-l.Open())
}

// next is the line to type after the word: with a conflict open, the first of
// the resolutions that is a command.
func next(l model.ConflictsListing) string {
	if l.Open() > 0 {
		return "gragas rebind"
	}
	return ""
}

// lead is the listing above the closing word: each open conflict as a table of
// the rules that hold the entry and what each sets, the resolved ones in a line
// each, and the three resolutions while one is open.
func lead(l model.ConflictsListing) []model.Block {
	var out []model.Block
	for _, c := range l.Conflicts {
		if !c.Open() {
			continue
		}
		out = append(out, model.Heading{Text: fmt.Sprintf("%s %s: %s differs in %s", c.Entry.Tool, c.Entry.Target, c.Entry.Key, strings.Join(c.Differs, ", "))})
		t := model.Table{Columns: []model.Column{{Title: "rule", Priority: 9}, {Title: "value", Priority: 8}, {Title: "options", Priority: 6}, {Title: "severity", Priority: 7}}}
		for _, s := range c.Sides {
			t.Rows = append(t.Rows, []string{string(s.Rule), show(s.Value), show(s.Options), string(s.Severity)})
		}
		out = append(out, t)
	}
	for _, c := range l.Conflicts {
		if !c.Open() {
			out = append(out, model.Note{Text: fmt.Sprintf("resolved: %s %s: %s, a decision names %s", c.Entry.Tool, c.Entry.Target, c.Entry.Key, c.Winner)})
		}
	}
	if l.Open() > 0 {
		out = append(out, model.Heading{Text: "Resolve each in one of three ways"})
		for _, line := range coreapi.ResolutionLines() {
			out = append(out, model.Note{Text: line})
		}
	}
	return out
}

// show is a value or an options map as one short text, "-" for none.
func show(v any) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case map[string]any:
		if len(x) == 0 {
			return "-"
		}
	}
	return fmt.Sprint(v)
}
