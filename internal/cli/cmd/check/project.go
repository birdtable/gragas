package check

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/kit"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// failOnFlag is the flag that names the kinds of finding that end a project
// check on findings.
const failOnFlag = "fail-on"

// failOnOf is the kinds --fail-on names, read from the command's flags.
func failOnOf(cmd *cobra.Command) ([]model.GateKind, error) {
	f := cmd.Flags().Lookup(failOnFlag)
	values, err := cmd.Flags().GetStringSlice(failOnFlag)
	if err != nil {
		return nil, err
	}
	return kindsFor(f != nil && f.Changed, values)
}

// kindsFor reads the --fail-on values: every one a kind of the closed set
// model.ProjectKinds lists. A value that is empty, or not on the list, is a
// usage error that names the list; a flag not given names no kind.
func kindsFor(given bool, values []string) ([]model.GateKind, error) {
	if !given {
		return nil, nil
	}
	if len(values) == 0 {
		return nil, errors.New("--" + failOnFlag + " names no kind: " + kindList())
	}
	var out []model.GateKind
	for _, v := range values {
		k := model.GateKind(strings.TrimSpace(v))
		if !slices.Contains(model.ProjectKinds(), k) {
			return nil, fmt.Errorf("--%s names %q, which is no kind of finding: %s", failOnFlag, v, kindList())
		}
		out = append(out, k)
	}
	return out, nil
}

// kindList is the kinds a project check has, as help and usage say them.
func kindList() string {
	kinds := model.ProjectKinds()
	words := make([]string, len(kinds))
	for i, k := range kinds {
		words[i] = string(k)
	}
	return "the kinds are " + strings.Join(words, ", ")
}

// runProject ends check below a pin on what the core found: not ready when a
// tool with bindings could not be asked (whatever else was found, and
// whatever --fail-on says), findings when a finding is of a kind --fail-on
// names, otherwise done with the findings listed.
func runProject(s kit.Session, failOn []model.GateKind, p model.ProjectCheck, ref *model.Refusal, err error) int {
	switch {
	case err != nil:
		return notReady(s, err)
	case ref != nil:
		return s.End(kit.Ending{Word: ref.Word(), What: ref.Observed(), Next: ref.Hint()})
	}
	word := p.Word()
	if word == model.Done && p.FailOn(failOn) {
		word = model.Findings
	}
	what := projectSaid(p)
	if word == model.NotReady {
		what = unjudged(p) + "; " + what
	}
	end := kit.Ending{Word: word, What: what}
	if word == model.NotReady {
		var because string
		end.Next, because = nextOf(p)
		if because != "" {
			end.What += "; " + because
		}
	}
	if s.JSON() {
		s.Document(coreapi.ProjectDocOf(p, word))
		return s.End(end) // draws nothing under --json
	}
	end.Lead = projectLead(p)
	return s.End(end)
}

// projectSaid is the sentence beside the closing word.
func projectSaid(p model.ProjectCheck) string {
	return fmt.Sprintf("%s, %s exempt, %s not checked", model.Plural(len(p.Findings), "finding"), model.Count(len(p.Exempt)),
		model.Count(len(p.NotChecked)))
}

// unjudged names each tool that was not checked and how many of its bindings
// were not judged.
func unjudged(p model.ProjectCheck) string {
	rows := p.Unjudged()
	words := make([]string, len(rows))
	for i, n := range rows {
		words[i] = fmt.Sprintf("%s: %s not judged", n.Tool, model.Plural(n.Bindings, "binding"))
	}
	return strings.Join(words, "; ")
}

// projectLead is the listing above the closing word: each finding with its
// kind, the exempt rows, then each row that says something was not checked.
func projectLead(p model.ProjectCheck) []model.Block {
	var out []model.Block
	if len(p.Findings) > 0 {
		out = append(out, model.Heading{Text: "Findings"}, gateTable(p.Findings))
	}
	if len(p.Exempt) > 0 {
		out = append(out, model.Heading{Text: "Exempt"}, model.List{Items: slices.Clone(p.Exempt)})
	}
	if len(p.NotChecked) > 0 {
		items := make([]string, len(p.NotChecked))
		for i, n := range p.NotChecked {
			items[i] = n.Msg
			if n.Tool != "" {
				items[i] = fmt.Sprintf("%s (%s): %s", n.Tool, model.Plural(n.Bindings, "binding"), n.Msg)
			}
		}
		out = append(out, model.Heading{Text: "Not checked"}, model.List{Items: items})
	}
	return out
}
