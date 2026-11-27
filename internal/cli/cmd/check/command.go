// Package check is the check command. In a folder that holds one rulebook it
// is the rulebook's gate (S14, S40): validate's findings, then HEAD's history
// held to the merge base of HEAD and the base ref the settings name. Anywhere
// else it is the old check of the rulebook tree under rulebook/, which the
// commands of the contexts replace at M6 (M0a-20, A41), so that side draws
// through the legacy host the root gives it. It lives in a package of its own
// below internal/cli/cmd; the root joins it from the generated join_gen.go.
package check

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// Order is the row's place in the table of spec 15.1.
const Order = 410

// Limit is how many of validate's findings a listing shows, as validate's
// own; a longer one says how many there are (cli.look.3). Every gate finding
// is shown.
const Limit = 50

// rulebookLong is what the help adds for a rulebook.
const rulebookLong = `

In a folder that holds one rulebook.yaml it checks the
rulebook instead: validate's findings, then its history
against the merge base of HEAD and the base ref (the
base-ref setting, or GRAGAS_BASE_REF): no line of the
merge base rewritten, no broken link or gap, each commit
since held to its own entries as guard staged holds one,
no pin or governance changed without its entry, and no
rule on trial past the merge base's window. Where the merge
base holds one rulebook, a branch that removes it or adds a
second is a finding. With no base ref it is not ready.`

// projectLong is what the help adds for a project.
const projectLong = `

Below a .gragas/pin.yaml, in the repository the pin lies
in, it checks the project: its tool configs held to the
bindings of the rulebook the pin names, its tier contract
and its exemptions. It ends done with the findings listed;
--fail-on names the kinds of finding that end it on
findings. A tool with bindings that could not be asked
ends it not ready.`

// Row is the one declaration of check (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "check", Order: Order, Spec: true, Context: kit.InBoth, Ends: kit.EndsListing, State: kit.Legacy,
		Summary: "Check a project against its rules, or a rulebook against its gate",
		Examples: [2]kit.Example{
			{Line: "gragas check", Does: "list the findings"},
			{Line: "gragas check --json", Does: "as one JSON document"},
		},
		Exits:    []model.Word{model.Done, model.Findings},
		JSONKind: "check",
		Menu:     kit.Menu{Offered: true},
	}
}

// Old is what the session of a run offers beside kit.Session while check is the
// old command: its cobra command, made over the legacy host the root holds.
// The package never imports the legacy package; the root does, and it leaves
// with it at M6.
type Old interface {
	OldCheck() *cobra.Command
}

// New is the cobra command, drawn through the session of the run: the old
// check, whose run asks first for the rulebook's. -C names a repository
// holding rulebook/, which only the old check reads.
func New(s kit.Session) *cobra.Command {
	o, ok := s.(Old)
	if !ok {
		panic("check: the session offers no old check")
	}
	c := o.OldCheck()
	old := c.RunE
	c.Long += rulebookLong + projectLong
	c.Flags().StringSlice(failOnFlag, nil, "end on findings when one is of this kind, below a pin ("+kindList()+")")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if f := cmd.Flags().Lookup("dir"); f != nil && f.Changed {
			return old(cmd, args)
		}
		if ran, err := inProject(cmd, s); ran {
			return err
		}
		ctx := cmd.Context()
		r, ref, err := s.CheckRulebook(ctx)
		if errors.Is(err, coreapi.ErrNoBook) || errors.Is(err, coreapi.ErrTwoBooks) {
			return old(cmd, args)
		}
		return s.Exit(run(s, r, ref, err))
	}
	return c
}

// run ends check in a rulebook on what the core found.
func run(s kit.Session, r model.RulebookCheck, ref *model.Refusal, err error) int {
	switch {
	case err != nil:
		return notReady(s, err)
	case ref != nil:
		return s.End(kit.Ending{Word: ref.Word(), What: ref.Observed(), Next: ref.Hint()})
	}
	if s.JSON() {
		s.Document(coreapi.CheckDoc(r, Limit))
		return s.End(kit.Ending{Word: r.Word(), What: said(r)}) // draws nothing under --json
	}
	return s.End(kit.Ending{Word: r.Word(), What: said(r), Lead: lead(r)})
}

// inProject runs the project check when the working folder holds no
// rulebook.yaml of its own and a pin lies in it or above it, up to the
// repository's root, and says whether it did; the error is what the run
// returns.
func inProject(cmd *cobra.Command, s kit.Session) (bool, error) {
	ctx := cmd.Context()
	here, err := s.HoldsRulebook(ctx)
	if err != nil {
		return true, s.Exit(failed(s, err))
	}
	if here {
		return false, nil
	}
	pinned, err := s.HoldsPin(ctx)
	if err != nil {
		return true, s.Exit(failed(s, err))
	}
	if !pinned {
		return false, nil
	}
	kinds, err := failOnOf(cmd)
	if err != nil {
		return true, s.Exit(s.End(kit.Ending{Word: model.Usage, What: err.Error()}))
	}
	p, ref, err := s.CheckProject(ctx)
	return true, s.Exit(runProject(s, kinds, p, ref, err))
}

// failed ends the run on a working folder that could not be asked.
func failed(s kit.Session, err error) int {
	return s.End(kit.Ending{Word: model.Failed, What: "the working folder could not be checked: " + err.Error()})
}

// notReady ends the run on a rulebook that cannot be read, as validate's.
func notReady(s kit.Session, err error) int {
	switch {
	case coreapi.IsOpenRecord(err):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: coreapi.RecoverLine})
	case errors.Is(err, coreapi.ErrNotReady):
		return s.End(kit.Ending{Word: model.NotReady, What: coreapi.Reason(err), Next: "gragas version"})
	}
	return s.End(kit.Ending{Word: model.Failed, What: "the rulebook could not be checked: " + err.Error()})
}

// said is the sentence beside the closing word.
func said(r model.RulebookCheck) string {
	return fmt.Sprintf("%s, %s against %s, %s, %s", model.Plural(len(r.Validation.Findings), "finding"),
		model.Plural(len(r.Gate), "gate finding"), r.BaseRef, model.Plural(r.Commits, "commit"), model.Plural(r.Releases, "release tag"))
}

// gateTable is findings of the gate as rows: kind, entity, and the message
// with its fix.
func gateTable(found []model.GateFinding) model.Table {
	t := model.Table{Columns: []model.Column{{Title: "kind", Priority: 5}, {Title: "about", Priority: 9}, {Title: "what is wrong, and the fix", Priority: 9}}}
	for _, g := range found {
		t.Rows = append(t.Rows, []string{string(g.Kind), g.Entity, withFix(g.Finding)})
	}
	return t
}

// lead is the listing above the closing word: validate's findings, the
// gate's in the order of their kinds, the notices, then each value this
// build cannot give yet, with the ticket that adds it.
func lead(r model.RulebookCheck) []model.Block {
	var out []model.Block
	v := r.Validation
	if len(v.Findings) > 0 {
		t := model.Table{Columns: []model.Column{{Title: "about", Priority: 9}, {Title: "what is wrong, and the fix", Priority: 9}}}
		for _, f := range v.Findings[:min(Limit, len(v.Findings))] {
			t.Rows = append(t.Rows, []string{f.Entity, withFix(f)})
		}
		out = append(out, model.Heading{Text: "Findings"}, t)
		if len(v.Findings) > Limit {
			out = append(out, model.Note{Text: fmt.Sprintf("%d of %d findings shown. Fix these and run it again to see the rest.", Limit, len(v.Findings))})
		}
	}
	if len(r.Gate) > 0 {
		out = append(out, model.Heading{Text: "Gate"}, gateTable(r.Gate))
	}
	if len(v.Notices) > 0 {
		items := make([]string, len(v.Notices))
		for i, n := range v.Notices {
			items[i] = n.Entity + ": " + n.Msg
		}
		out = append(out, model.Heading{Text: "Notices"}, model.List{Items: items})
	}
	if len(r.Unavailable) > 0 {
		items := make([]string, len(r.Unavailable))
		for i, u := range r.Unavailable {
			items[i] = u.Name + ": " + u.Reason
		}
		out = append(out, model.Heading{Text: "Not available yet"}, model.List{Items: items})
	}
	return out
}

// withFix is a finding's sentence and, when one is offered, the fix beside it.
func withFix(f model.Finding) string {
	if f.Fix == "" {
		return f.Msg
	}
	return f.Msg + " Fix: " + f.Fix
}
