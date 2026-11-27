// Package tools is the tools command: in a rulebook it lists the tools the
// rules bind to, the entries those bindings hold and the rule that holds each.
// It lives in a package of its own below internal/cli/cmd; the root joins it
// from the generated join_gen.go. It reads nothing itself: it asks the session,
// which asks the core, and draws what comes back.
package tools

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
const Order = 200

// Row is the one declaration of tools (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Listing("tools", Order, "List the tools the rules bind to", [2]kit.Example{
		{Line: "gragas tools", Does: "list them"},
		{Line: "gragas tools --json", Does: "as one JSON document"},
	})
}

// long is the help text of the command.
const long = `Lists, for each tool a rule of this rulebook binds to, the
entries the bindings hold, each with the rules that hold it, the
tool version the rulebook's tested set names, and the installed
plugin that serves it, by name and version. A tool the tested set
names that no rule binds to is listed with no entry; a tool with
no plugin says so, and while a tool a rule binds to has none the
run ends not ready and names the step that installs one. Then the checker code the rulebook declares, by name
and publisher: its own, and an import's by reference under the
import's name; nothing of the code is read or run. It does not
read the tools' own configs: the entries
that no rule explains are counted by coverage, which owns them,
so none is listed here. This is the rulebook form; the form for
a project comes with the project commands. It reads and
changes nothing.

  gragas tools
  gragas tools --json`

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	c := &cobra.Command{Use: "tools", Short: Row().Summary, Long: long, Args: cobra.NoArgs}
	c.RunE = func(c *cobra.Command, _ []string) error {
		l, err := s.Tools(c.Context())
		if err != nil {
			return s.Exit(notReady(s, err))
		}
		d := coreapi.ToolsDocument(l)
		if len(d.Unserved) > 0 {
			return s.Exit(unserved(s, d, lead(l, d.Entries)))
		}
		return s.Exit(kit.EndListing(s, d, said(d), lead(l, d.Entries)))
	}
	return c
}

// unserved ends a listing in which a tool a rule binds to has no installed
// plugin: not ready, for no check reads that tool's config, naming each such
// tool and the step that installs a plugin. The listing is drawn
// above it, and under --json the document says the same.
func unserved(s kit.Session, d coreapi.ToolsDoc, lead []model.Block) int {
	reads := ", so no check reads its config"
	if len(d.Unserved) > 1 {
		reads = ", so no check reads their configs"
	}
	what := said(d) + "; no plugin is installed for " + strings.Join(d.Unserved, ", ") + reads
	if s.JSON() {
		s.Document(d)
		return s.End(kit.Ending{Word: model.NotReady, What: what, Next: "gragas plugin add <folder>"})
	}
	return s.End(kit.Ending{Word: model.NotReady, What: what, Next: "gragas plugin add <folder>", Lead: lead})
}

// notReady ends the run on a folder whose tools cannot be listed, always not
// ready (the spec gives tools the word done and any command may be not ready):
// an open run, a rulebook newer than this engine, no rulebook, a rulebook
// with findings, or two, or an installed plugin not approved, whose approval
// it names. A plugin refused for anything else fails: its bytes are not what
// the lock holds.
func notReady(s kit.Session, err error) int {
	var findings *coreapi.FindingsError
	var unapproved *coreapi.UnapprovedPlugin
	switch {
	case errors.As(err, &unapproved):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error(), Next: "gragas plugin approve " + unapproved.Ref.Name + "@" + unapproved.Ref.Version})
	case coreapi.IsOpenRecord(err):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: coreapi.RecoverLine})
	case errors.Is(err, coreapi.ErrNotReady):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: "gragas version"})
	case errors.Is(err, coreapi.ErrNoBook), errors.Is(err, coreapi.ErrTwoBooks):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error() + ". tools lists a rulebook's tools here, and the form for a project comes later", Next: "gragas init"})
	case errors.As(err, &findings):
		return s.End(kit.Ending{Word: model.NotReady, What: err.Error(), Next: "gragas validate"})
	}
	return s.End(kit.Ending{Word: model.Failed, What: "the tools could not be listed: " + err.Error()})
}

// said is the sentence beside the closing word.
func said(d coreapi.ToolsDoc) string {
	return fmt.Sprintf("%s, %s, %s held", model.Plural(len(d.Tools), "tool"), model.Plural(d.Entries, "entry"), model.Plural(d.Rules, "rule"))
}

// lead is the listing above the closing word: for each tool a line that says
// the version its tested set names and the plugin that serves it, then its
// entries as a table, then the two empty states in a line each.
func lead(l model.ToolsListing, entries int) []model.Block {
	if len(l.Rows) == 0 {
		return append([]model.Block{model.Note{Text: "no bindings: 0 rules held"}}, checkers(l.Checkers)...)
	}
	var out []model.Block
	for _, r := range l.Rows {
		out = append(out, model.Heading{Text: heading(r)})
		if len(r.Entries) == 0 {
			out = append(out, model.Note{Text: "no rule binds it"})
			continue
		}
		t := model.Table{Columns: []model.Column{{Title: "entry", Priority: 9}, {Title: "held by", Priority: 8}}}
		for _, e := range r.Entries {
			rules := make([]string, len(e.Rules))
			for i, id := range e.Rules {
				rules[i] = string(id)
			}
			t.Rows = append(t.Rows, []string{string(e.Target) + ": " + e.Key, strings.Join(rules, ", ")})
		}
		out = append(out, t)
	}
	if entries == 0 {
		out = append(out, model.Note{Text: "no bindings: 0 rules held"})
	}
	out = append(out, checkers(l.Checkers)...)
	return append(out, model.Note{Text: "no preset entries read: 0 unexplained (coverage lists them)"})
}

// checkers is the checker code the rulebook declares, by name and publisher:
// its own, and an import's by reference under the import's name.
// Nothing of the code is read or run; a rulebook that declares none shows no
// section.
func checkers(rows []model.CheckerRow) []model.Block {
	if len(rows) == 0 {
		return nil
	}
	t := model.Table{Columns: []model.Column{{Title: "artifact", Priority: 9}, {Title: "tool", Priority: 7}, {Title: "publisher", Priority: 8}, {Title: "of", Priority: 6}}}
	for _, r := range rows {
		of := r.Rulebook
		if r.ByReference {
			of += ", by reference"
		}
		t.Rows = append(t.Rows, []string{r.Name, r.Tool, r.Publisher, of})
	}
	return []model.Block{model.Heading{Text: "checker code, declared by name and publisher"}, t}
}

// heading is the line over one tool's entries: its name, the version of it the
// tested set names, and the plugin that serves it.
func heading(r model.ToolRow) string {
	tested, served := "not tested", "no plugin"
	if r.Tested != "" {
		tested = "tested " + r.Tested
	}
	if r.Plugin != nil {
		served = "plugin " + r.Plugin.Name + " " + r.Plugin.Version
	}
	return r.Tool + ", " + tested + ", " + served
}
