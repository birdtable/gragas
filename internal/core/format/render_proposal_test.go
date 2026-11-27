package format

import (
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const wrapFile = "proposals/tighten-wrap.yaml"

// commentedProposal is a proposal file with what a person leaves in one:
// comments at the top and in the document, a trailing comment on a moved
// line and on its twin, and a block scalar.
const commentedProposal = `# a proposal to the base book
format: 3
slug: tighten-wrap
target: base
state: open   # moved by the engine
link: https://example.test/pull/1
opened: "2026-11-04"
document:
  contract: "1.0"
  # what the target's owner reads
  proposal:
    slug: tighten-wrap
    target: base
    change:
      anatomy: |-
        A wrapped error names its step,
        so its message says where.
    state: open
    link: https://example.test/pull/1   # the pull request
    opened: "2026-11-04"
`

// renderProposalIn renders d over a book whose proposal file is body, after
// each plant is made in the world.
func renderProposalIn(t *testing.T, body string, d model.Delta, plants ...func(*porttest.World)) ([]model.Target, error) {
	t.Helper()
	files := fullBook(t)
	files[wrapFile] = body
	w, root := world(t, files)
	for _, p := range plants {
		p(w)
	}
	got, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	return Render(p.Tree, p.Schema, root, got, d)
}

func moveWrap(field, before, after string) model.Delta {
	return model.Delta{Changes: []model.Change{{Entity: "proposal:tighten-wrap", Field: field, Before: before, After: after}}}
}

// TestAProposalMovesInItsFileAndItsDocumentAndNothingElse: a state or
// link change edits the scalar in the file and its twin in document.proposal
// together, and every other byte, comments and the block scalar included, is
// the same. Planted failure: the twin left as it was (the document then says
// other than the file, and the edit is refused), or replaceValue dropping a
// trailing comment.
func TestAProposalMovesInItsFileAndItsDocumentAndNothingElse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		d    model.Delta
		want string
	}{
		{"open to accepted", moveWrap("state", "open", "accepted"), strings.NewReplacer(
			"state: open   # moved by the engine", "state: accepted   # moved by the engine",
			"    state: open\n", "    state: accepted\n").Replace(commentedProposal)},
		{"a new link", moveWrap("link", "https://example.test/pull/1", "https://example.test/pull/2"), strings.NewReplacer(
			"link: https://example.test/pull/1\n", "link: \"https://example.test/pull/2\"\n",
			"    link: https://example.test/pull/1   # the pull request", "    link: \"https://example.test/pull/2\"   # the pull request").Replace(commentedProposal)},
		{"the link taken out", moveWrap("link", "https://example.test/pull/1", ""), strings.NewReplacer(
			"link: https://example.test/pull/1\n", "",
			"    link: https://example.test/pull/1   # the pull request\n", "").Replace(commentedProposal)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.want == commentedProposal {
				t.Fatal("the case changes nothing")
			}
			ts, err := renderProposalIn(t, commentedProposal, c.d)
			if err != nil || len(ts) != 1 || ts[0].Path != wrapFile {
				t.Fatalf("targets = %v, err = %v", ts, err)
			}
			if got := string(ts[0].Data); got != c.want {
				t.Errorf("the edit is not the owned lines alone:\n%s", diff(c.want, got))
			}
			if ts[0].Before != digestOf([]byte(commentedProposal)) {
				t.Errorf("Before = %q, want the digest of the file it replaces", ts[0].Before)
			}
		})
	}
	t.Run("a link added where there is none", func(t *testing.T) {
		t.Parallel()
		body := strings.NewReplacer("link: https://example.test/pull/1\n", "", "    link: https://example.test/pull/1   # the pull request\n", "").Replace(commentedProposal)
		ts, err := renderProposalIn(t, body, moveWrap("link", "", "https://example.test/pull/3"))
		if err != nil || len(ts) != 1 {
			t.Fatalf("targets = %v, err = %v", ts, err)
		}
		got := string(ts[0].Data)
		if strings.Count(got, "link: \"https://example.test/pull/3\"\n") != 2 {
			t.Errorf("the link is not in the file and its document:\n%s", got)
		}
		left := strings.ReplaceAll(strings.ReplaceAll(got, "    link: \"https://example.test/pull/3\"\n", ""), "link: \"https://example.test/pull/3\"\n", "")
		if left != body {
			t.Errorf("bytes other than the two links changed:\n%s", diff(body, left))
		}
	})
}

// TestAProposalMoveTheLifecycleDoesNotGiveIsRefused: a proposal moves
// once, from open, to accepted, declined or withdrawn; any other state change
// is refused before the file is touched. Planted failure: CanBecome not
// asked.
func TestAProposalMoveTheLifecycleDoesNotGiveIsRefused(t *testing.T) {
	t.Parallel()
	accepted := strings.ReplaceAll(commentedProposal, "state: open", "state: accepted")
	cases := []struct {
		name, body, before, after string
	}{
		{"open to open", commentedProposal, "open", "open"},
		{"open to a state the spec does not give", commentedProposal, "open", "merged"},
		{"open to no state", commentedProposal, "open", ""},
		{"accepted to declined", accepted, "accepted", "declined"},
		{"accepted back to open", accepted, "accepted", "open"},
	}
	for _, c := range cases {
		ts, err := renderProposalIn(t, c.body, moveWrap("state", c.before, c.after))
		if !errors.Is(err, ErrNoSuchMove) || ts != nil {
			t.Errorf("%s: targets = %v, err = %v; want ErrNoSuchMove and no target", c.name, ts, err)
		}
	}
}

// TestAProposalChangeRenderCannotMakeIsRefusedByName: each change the
// proposal kind does not make, or whose before value is not what the file
// holds, is refused, naming why, and nothing is written.
func TestAProposalChangeRenderCannotMakeIsRefusedByName(t *testing.T) {
	t.Parallel()
	newOne := strings.ReplaceAll(commentedProposal, "tighten-wrap", "tighten-new")
	twinMoved := strings.Replace(commentedProposal, "    link: https://example.test/pull/1   # the pull request", "    link: https://example.test/pull/9", 1)
	cases := []struct {
		name, body string
		d          model.Delta
		is         error
		want       string
	}{
		{"a field written once", commentedProposal, moveWrap("target", "base", "other"), ErrUnsupported, "written once"},
		{"a key that is not a slug", commentedProposal, model.Delta{Changes: []model.Change{{Entity: "proposal:../rulebook", Field: "state", Before: "open", After: "accepted"}}}, ErrUnsupported, "not a slug"},
		{"a before value the file does not hold", commentedProposal, moveWrap("link", "https://example.test/pull/2", "https://example.test/pull/3"), ErrMoved, `"https://example.test/pull/1" now`},
		{"a twin that says otherwise", twinMoved, moveWrap("link", "https://example.test/pull/1", "https://example.test/pull/3"), ErrMoved, "document.proposal.link of the proposal tighten-wrap"},
		{"a proposal not in the tree", commentedProposal, model.Delta{Changes: []model.Change{{Entity: "proposal:tighten-gone", Field: "state", Before: "open", After: "accepted"}}}, ErrMoved, "not in the tree"},
		{"a new proposal over one in the tree", commentedProposal, model.Delta{Changes: []model.Change{{Entity: "proposal:tighten-wrap", After: commentedProposal}}}, ErrMoved, "the plan adds it"},
		{"the file named as another kind too", commentedProposal, model.Delta{Changes: []model.Change{
			{Entity: "proposal:tighten-wrap", Field: "state", Before: "open", After: "accepted"},
			{Entity: "file:" + wrapFile, After: "{}"},
		}}, nil, "the proposal tighten-wrap's alone"},
		{"a new proposal that is not open", commentedProposal, model.Delta{Changes: []model.Change{{Entity: "proposal:tighten-new", After: strings.ReplaceAll(newOne, "state: open", "state: accepted")}}}, nil, `a new proposal is open, and this one is "accepted"`},
		{"a new proposal whose document says otherwise", commentedProposal, model.Delta{Changes: []model.Change{{Entity: "proposal:tighten-new", After: strings.Replace(newOne, "    target: base", "    target: other", 1)}}}, nil, "write the same in both"},
		{"a new proposal named for another file", commentedProposal, model.Delta{Changes: []model.Change{{Entity: "proposal:tighten-other", After: newOne}}}, nil, "the file is named tighten-other"},
	}
	for _, c := range cases {
		ts, err := renderProposalIn(t, c.body, c.d)
		if err == nil || (c.is != nil && !errors.Is(err, c.is)) || !strings.Contains(err.Error(), c.want) || ts != nil {
			t.Errorf("%s: targets = %v, err = %v; want %v holding %q", c.name, ts, err, c.is, c.want)
		}
	}
}

// TestANewProposalIsWrittenWholeAsItsAfter: a proposal with no file is
// written as the YAML the change holds, byte for byte. Planted failure: the
// new-file case read as a move.
func TestANewProposalIsWrittenWholeAsItsAfter(t *testing.T) {
	t.Parallel()
	newOne := strings.ReplaceAll(commentedProposal, "tighten-wrap", "tighten-new")
	ts, err := renderProposalIn(t, commentedProposal, model.Delta{Changes: []model.Change{{Entity: "proposal:tighten-new", After: newOne}}})
	if err != nil || len(ts) != 1 || ts[0].Path != "proposals/tighten-new.yaml" || string(ts[0].Data) != newOne || ts[0].Before != "" {
		t.Fatalf("targets = %+v, err = %v", ts, err)
	}
}

// TestAProposalTheSchemaRefusesIsNotWritten: the new file is read back
// with the proposals schema, and a problem the schema finds refuses the edit,
// naming the file and the line. Planted failure: the schema's findings not
// asked.
func TestAProposalTheSchemaRefusesIsNotWritten(t *testing.T) {
	t.Parallel()
	newOne := strings.ReplaceAll(commentedProposal, "tighten-wrap", "tighten-new")
	ts, err := renderProposalIn(t, commentedProposal, model.Delta{Changes: []model.Change{{Entity: "proposal:tighten-new", After: newOne}}}, func(w *porttest.World) {
		w.Plant(proposalsSchema, docOf(t, newOne), model.Problem{Line: 5, Msg: "schema at state: must be one of the states"})
	})
	if err == nil || ts != nil || !strings.Contains(err.Error(), "proposals/tighten-new.yaml:5: schema at state") {
		t.Errorf("targets = %v, err = %v; want the schema's problem at line 5", ts, err)
	}
}
