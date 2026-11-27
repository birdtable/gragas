package format

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// The guards of the renderers that no other test turns red.

// refusesStatus is a Schema that holds a rule's status to the values the real
// schema allows: the double accepts any document, so it is asked to refuse one.
type refusesStatus struct{ port.Schema }

func (s refusesStatus) Check(name string, doc []byte) ([]model.Problem, error) {
	if bytes.Contains(doc, []byte("bogus")) {
		return []model.Problem{{Line: 1, Msg: "status is not one of the allowed values"}}, nil
	}
	return s.Schema.Check(name, doc)
}

func TestAnEditThatLeavesTheFileInvalidIsRefusedAndAValueWithYAMLSyntaxIsQuoted(t *testing.T) {
	t.Parallel()
	w, root, got := renderWorld(t, commented)
	p := w.Ports()
	_, err := Render(p.Tree, refusesStatus{p.Schema}, root, got, setRule("go.errors.1", "status", "active", "bogus"))
	if err == nil || !strings.Contains(err.Error(), "would leave") {
		t.Errorf("an enum set to a value it does not have: err = %v; want the edit refused as one that would leave the file invalid", err)
	}
	if _, err := Render(p.Tree, refusesStatus{p.Schema}, root, got, setRule("go.errors.1", "status", "active", "proposed")); err != nil {
		t.Errorf("the clean control was refused: %v", err)
	}
	out := one(t, commented, setRule("go.errors.1", "owner", "team-a", "a: [b"))
	if !strings.Contains(out, `owner: "a: [b"`) {
		t.Errorf("a value with YAML syntax in it was not quoted:\n%s", diff(commented, out))
	}
}

func TestAnAliasTheRuleAlreadyHoldsIsNotAddedAgain(t *testing.T) {
	t.Parallel()
	_, err := render(t, commented, setRule("go.errors.1", "aliases[]", "", "go.errs.1"))
	if !errors.Is(err, ErrMoved) || !strings.Contains(err.Error(), "already holds") {
		t.Errorf("err = %v; want ErrMoved saying the rule already holds it", err)
	}
}

func TestAListItemIsAddedWithNoBeforeValue(t *testing.T) {
	t.Parallel()
	_, err := render(t, commented, setRule("go.errors.2", "aliases[]", "go.old.2", "go.errs.2"))
	if !errors.Is(err, ErrMoved) || !strings.Contains(err.Error(), "no before value") {
		t.Errorf("err = %v; want ErrMoved: a list item is added with no before value", err)
	}
}

func TestAFieldInsideAFlowMappingIsRefusedAsFlowStyle(t *testing.T) {
	t.Parallel()
	body := strings.Replace(commented, "    reasoning:\n      definition: |-\n        An error says what failed\n        and what was being done.\n      pros:\n        - the cause is found\n        # a note in the list\n        - the message is short\n      decision: wrap-errors\n",
		"    reasoning: {definition: short}\n", 1)
	if body == commented {
		t.Fatal("the plant did not take")
	}
	_, err := render(t, body, setRule("go.errors.1", "reasoning.definition", "short", "longer"))
	if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "written in flow style") {
		t.Errorf("err = %v; want ErrUnsupported: written in flow style", err)
	}
}

func TestABlockAddedToAGuideTakesTheGuidesLineEndingBOMAndMissingFinalNewline(t *testing.T) {
	t.Parallel()
	d := model.Delta{Changes: []model.Change{{Entity: "block:" + guideFile + "#tests", After: "anchor: tests\nrules:\n  - id: go.tests.9\n    statement: Test a unit alone.\n    strength: should\n    status: proposed\n    origin: authored\n"}}}
	guides := map[string]string{
		"CRLF":                asCRLF(proseGuide),
		"a BOM":               "\ufeff" + proseGuide,
		"no final newline":    strings.TrimSuffix(proseGuide, "\n"),
		"CRLF, BOM, no final": "\ufeff" + strings.TrimSuffix(asCRLF(proseGuide), "\r\n"),
	}
	for name, g := range guides {
		out, err := guideRender(t, g, d)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		s := string(out)
		if !strings.Contains(s, "anchor: tests") || !strings.HasSuffix(s, "```"+map[bool]string{true: "\r\n", false: "\n"}[strings.Contains(g, "\r\n")]) {
			t.Errorf("%s: the block is not added whole at the end: %q", name, s[max(0, len(s)-80):])
		}
		if strings.Contains(g, "\r\n") && strings.Count(s, "\n") != strings.Count(s, "\r\n") {
			t.Errorf("%s: a bare newline in a CRLF guide", name)
		}
		if strings.HasPrefix(g, "\ufeff") != strings.HasPrefix(s, "\ufeff") {
			t.Errorf("%s: the BOM moved", name)
		}
	}
}
