// Package contract is the contract command: validate checks a document against
// the contract's schemas, fields lists an entity's fields with the version
// each came in, and fixture writes the fixture rulebook's documents to a
// folder. It lives in a package of its own below internal/cli/cmd; the root
// joins it from the generated join_gen.go.
package contract

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

// Order is the row's place in the table of spec 15.1.
const Order = 500

// Limit is how many findings a listing shows, as validate's.
const Limit = 50

// Row is the one declaration of contract (cli.commands.20, .26).
func Row() kit.Command {
	return kit.Command{
		Name: "contract", Then: []string{"validate", "fields", "fixture"}, Order: Order, Spec: true,
		Context: kit.Anywhere, Ends: kit.EndsListing, State: kit.Built,
		Summary: "Check documents, read fields, write a fixture",
		Examples: [2]kit.Example{
			{Line: "gragas contract validate <file>", Does: "check a document"},
			{Line: "gragas contract fields rule", Does: "list the rule entity's fields"},
		},
		Writes:   []string{"the fixture's documents, with --apply"},
		Exits:    []model.Word{model.Done, model.Findings, model.NotReady},
		JSONKind: "contract",
		SARIF:    true,
	}
}

// New is the cobra command, drawn through the session of the run.
func New(s kit.Session) *cobra.Command {
	c := &cobra.Command{
		Use:   "contract",
		Short: Row().Summary,
		Long: `The contract is the set of documents gragas reads and writes: the
bundle, the report, the changelog, the SARIF, the plan and the
proposal. validate checks one against the schemas of the version
it names; fields lists what an entity holds and when each field
came in; fixture writes the fixture rulebook's documents, to read
or to test against.

  gragas contract validate bundle.json
  gragas contract fields rule
  gragas contract fixture --out ./fixture --apply`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	c.AddCommand(validate(s), fields(s), fixture(s))
	return c
}

func validate(s kit.Session) *cobra.Command {
	var kind string
	c := &cobra.Command{
		Use:   "validate <file>",
		Short: "Check a document against the contract",
		Long: `Reads the file and checks it against the schema of the contract
version it names. The kind of document is told from the fields only
it has, or named with --as: bundle, report, changelog, sarif, plan
or proposal. A finding is something the schema refuses; any one ends
on findings. A document in a newer contract than this engine reads
ends not ready and names both versions. It reads and changes
nothing. --sarif writes the findings as one SARIF document.

  gragas contract validate bundle.json
  gragas contract validate out.json --as report --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return s.Exit(runValidate(c, s, args[0], kind))
		},
	}
	c.Flags().StringVar(&kind, "as", "", "Read the file as this document, when its fields do not tell")
	return c
}

func runValidate(c *cobra.Command, s kit.Session, file, kind string) int {
	got, err := s.ContractValidate(c.Context(), file, kind)
	if err != nil {
		return s.End(kit.Ending{Word: model.Failed, What: err.Error()})
	}
	word := got.Word()
	switch {
	case got.Usage != "":
		if s.JSON() {
			s.Document(got)
		}
		next, why := nextLine(s.Line, []string{"gragas", "contract", "validate"}, file, "--as", got.Known[0])
		return s.End(kit.Ending{Word: model.Usage, What: got.Usage + ". The kinds are listed above" + why,
			Next: next, Lead: []model.Block{model.List{Items: got.Known}}})
	case got.NotReady != nil:
		what := fmt.Sprintf("the %s is written in contract %s and this engine reads up to contract %s", got.Kind, got.NotReady.Found, got.NotReady.Reads)
		if s.JSON() {
			s.Document(got)
		}
		return s.End(kit.Ending{Word: word, What: what, Next: "gragas version"})
	}
	what := fmt.Sprintf("%s, contract %s, %s", got.Kind, got.Contract, model.Plural(len(got.Findings), "finding"))
	if s.JSON() {
		s.Document(got)
		return s.End(kit.Ending{Word: word, What: what})
	}
	if s.SARIF() {
		doc, err := coreapi.ValidationSARIF(model.Validation{Findings: got.Findings}, s.Build().Version)
		if err != nil {
			return s.End(kit.Ending{Word: model.Failed, What: "the findings could not be written as SARIF: " + err.Error()})
		}
		s.Encoded(doc)
		return s.End(kit.Ending{Word: word, What: what})
	}
	var lead []model.Block
	if len(got.Findings) > 0 {
		t := model.Table{Columns: []model.Column{{Title: "about", Priority: 9}, {Title: "what is wrong, and the fix", Priority: 9}}}
		for _, f := range got.Findings[:min(Limit, len(got.Findings))] {
			msg := f.Msg
			if f.Fix != "" {
				msg += " Fix: " + f.Fix
			}
			t.Rows = append(t.Rows, []string{f.Entity, msg})
		}
		lead = append(lead, model.Heading{Text: "Findings"}, t)
		if len(got.Findings) > Limit {
			lead = append(lead, model.Note{Text: fmt.Sprintf("%d of %d findings shown. Fix these and run it again to see the rest.", Limit, len(got.Findings))})
		}
	}
	return s.End(kit.Ending{Word: word, What: what, Lead: lead})
}

func fields(s kit.Session) *cobra.Command {
	return &cobra.Command{
		Use:   "fields <entity>",
		Short: "List an entity's fields and when each came in",
		Long: `Lists the fields of one entity of the contract, such as rule or
finding, each with its type, the contract version it came in and
its default. An entity the contract lacks ends usage and lists the
ones it has. It reads and changes nothing.

  gragas contract fields rule
  gragas contract fields rule --json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			entity := ""
			if len(args) == 1 {
				entity = args[0]
			}
			return s.Exit(runFields(c, s, entity))
		},
	}
}

func runFields(c *cobra.Command, s kit.Session, entity string) int {
	got, err := s.ContractFields(c.Context(), entity)
	if err != nil {
		return s.End(kit.Ending{Word: model.Failed, What: "the fields could not be listed: " + err.Error()})
	}
	if got.Usage != "" {
		if s.JSON() {
			s.Document(got)
		}
		return s.End(kit.Ending{Word: model.Usage, What: got.Usage, Next: "gragas contract fields " + got.Known[0],
			Lead: []model.Block{model.List{Items: got.Known}}})
	}
	what := fmt.Sprintf("%s, %s, contract %s", got.Entity, model.Plural(len(got.Fields), "field"), got.Contract)
	t := model.Table{Columns: []model.Column{{Title: "Field", Priority: 9}, {Title: "Type", Priority: 8}, {Title: "Since", Priority: 7}, {Title: "Default", Priority: 3}}}
	for _, f := range got.Fields {
		t.Rows = append(t.Rows, []string{f.Name, f.Type, f.Since, f.Default})
	}
	return kit.EndListing(s, got, what, []model.Block{model.Heading{Text: "gragas contract fields " + got.Entity}, t})
}

func fixture(s kit.Session) *cobra.Command {
	var out string
	var apply bool
	c := &cobra.Command{
		Use:         "fixture --out <dir>",
		Short:       "Write the fixture rulebook to a folder",
		Annotations: map[string]string{kit.Writes: "with --apply"},
		Long: `Writes the fixture rulebook's documents, the ones every document
check of the contract is held to, into the folder --out, which must
exist. Without --apply it lists the files it would write and writes
nothing. It writes only inside --out, takes the folder's lock while
it does and adds no history line. The gate compares what it writes
with the repository's contract/fixture.

  gragas contract fixture --out ./fixture
  gragas contract fixture --out ./fixture --apply`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return s.Exit(runFixture(c, s, out, apply))
		},
	}
	c.Flags().StringVar(&out, "out", "", "The folder to write into")
	c.Flags().BoolVar(&apply, "apply", false, "Write the files")
	return c
}

func runFixture(c *cobra.Command, s kit.Session, out string, apply bool) int {
	if out == "" {
		return s.End(kit.Ending{Word: model.Usage, What: "fixture needs the folder to write into", Next: "gragas contract fixture --out ./fixture"})
	}
	got, err := s.ContractFixture(c.Context(), out, apply)
	if err != nil {
		w := model.Failed
		if errors.Is(err, coreapi.ErrFolderBusy) {
			w = model.Busy
		}
		return s.End(kit.Ending{Word: w, What: err.Error()})
	}
	verb := "would write"
	if got.Wrote {
		verb = "wrote"
	}
	what := fmt.Sprintf("%s %s into %s", verb, model.Plural(len(got.Files), "file"), got.Out)
	var lead []model.Block
	if s.JSON() {
		s.Document(got)
	} else {
		lead = []model.Block{model.List{Items: got.Files}}
	}
	if !got.Wrote {
		next, why := nextLine(s.Line, []string{"gragas", "contract", "fixture", "--out"}, out, "--apply")
		return s.End(kit.Ending{Word: model.Declined, What: what + why, Next: next, Lead: lead})
	}
	return s.End(kit.Ending{Word: model.Done, What: what, Lead: lead})
}

// nextLine is the line a screen ends on: head, then rest, as one line the
// session's Line quotes for the run's shell. When a
// word is a value no such line can carry, or one the screen would not show as
// written, there is no line and none quoted some other way: next is the help
// of the command, which holds no value, and why is Line's reason, which names
// the value escaped, for the ending's sentence (A5, A6).
func nextLine(line func([]string) (string, error), head []string, rest ...string) (next, why string) {
	said, err := line(append(slices.Clone(head), rest...))
	if err == nil {
		return said, ""
	}
	words := slices.DeleteFunc(slices.Clone(head[1:]), func(w string) bool { return strings.HasPrefix(w, "-") })
	return "gragas help " + strings.Join(words, " "), "; " + err.Error()
}
