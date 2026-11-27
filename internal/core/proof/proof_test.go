package proof_test

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/proof"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

// good is a cell that holds the invariant.
func good() model.Cell {
	return model.Cell{Binding: "a.b.1#00", ToolVersion: "1.0.0", Result: model.TruthPass, Rung: 3, Level: model.RunPair}
}

// lines is a stand-in decoder: one cell a line, fields split by "|", so the
// tests hold the context to what it does and not to a document's bytes.
func lines(data []byte) (model.ProofGrid, error) {
	var out []model.Cell
	for l := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if l == "" {
			continue
		}
		f := strings.Split(l, "|")
		if len(f) != 6 {
			return model.ProofGrid{}, errors.New("a cell has six fields")
		}
		var rung int
		if _, err := fmt.Sscan(f[3], &rung); err != nil {
			return model.ProofGrid{}, err
		}
		out = append(out, model.Cell{Binding: f[0], ToolVersion: f[1], Result: model.Truth(f[2]), Rung: rung, Level: model.RunLevel(f[4]), Reason: model.Reason(f[5])})
	}
	return model.ProofGrid{Cells: out}, nil
}

func read(t *testing.T, files map[string]string, decode proof.Decode) (model.ProofGrid, error) {
	t.Helper()
	w := porttest.NewWorld(t)
	root := w.Seed("repo", files)
	return proof.Read(w.Ports().Tree, root, decode)
}

func TestAnAbsentGridFileIsTheEmptyGrid(t *testing.T) {
	t.Parallel()
	called := false
	grid, err := read(t, map[string]string{"rulebook.yaml": "x"}, func([]byte) (model.ProofGrid, error) { called = true; return model.ProofGrid{}, nil })
	if err != nil || len(grid.Cells) != 0 || grid.TestedSet != nil || called {
		t.Errorf("Read of no file = %v, %v (decoder called: %v), want the empty grid", grid, err, called)
	}
}

func TestAGridFileIsDecodedAndHeld(t *testing.T) {
	t.Parallel()
	grid, err := read(t, map[string]string{proof.Path: "a#1|1.0|pass|3|pair|\nb#2|1.0|not run|0|fixture|no pair\n"}, lines)
	if err != nil || len(grid.Cells) != 2 || grid.Cells[1].Reason != model.ReasonNoPair {
		t.Errorf("Read = %+v, %v", grid, err)
	}
}

type newer struct{}

func (newer) Error() string { return "written in contract 2.0; this engine reads up to contract 1.0" }

func TestADecoderErrorPassesThroughNamed(t *testing.T) {
	t.Parallel()
	_, err := read(t, map[string]string{proof.Path: "x"}, func([]byte) (model.ProofGrid, error) { return model.ProofGrid{}, newer{} })
	var n newer
	if !errors.As(err, &n) || !strings.Contains(err.Error(), "2.0") || !strings.Contains(err.Error(), "1.0") {
		t.Errorf("Read = %v, want the decoder's error, unwrapped by errors.As and naming both versions", err)
	}
}

// TestABlankCellTurnsTheInvariantRed is the plant: each cell below is one
// breach of the never-blank invariant, held alone against the control.
func TestABlankCellTurnsTheInvariantRed(t *testing.T) {
	t.Parallel()
	if err := proof.Validate([]model.Cell{good()}); err != nil {
		t.Fatalf("the control is refused: %v", err)
	}
	plants := map[string]func(*model.Cell){
		"no binding":           func(c *model.Cell) { c.Binding = "" },
		"no tool version":      func(c *model.Cell) { c.ToolVersion = "" },
		"no result":            func(c *model.Cell) { c.Result = "" },
		"an unknown result":    func(c *model.Cell) { c.Result = "maybe" },
		"no level":             func(c *model.Cell) { c.Level = "" },
		"a rung past 6":        func(c *model.Cell) { c.Rung = 7 },
		"a negative rung":      func(c *model.Cell) { c.Rung = -1 },
		"rung 6 pass":          func(c *model.Cell) { c.Rung = 6 },
		"reviewed below 6":     func(c *model.Cell) { c.Result = model.TruthReviewed },
		"rung 6 fail":          func(c *model.Cell) { c.Rung, c.Result = 6, model.TruthFail },
		"not run with no why":  func(c *model.Cell) { c.Result = model.TruthNotRun },
		"a reason on a result": func(c *model.Cell) { c.Reason = model.ReasonNoPair },
	}
	for name, plant := range plants {
		c := good()
		plant(&c)
		if err := proof.Validate([]model.Cell{c}); err == nil {
			t.Errorf("a cell with %s is accepted", name)
		}
		if _, err := read(t, map[string]string{proof.Path: fmt.Sprintf("%s|%s|%s|%d|%s|%s\n", c.Binding, c.ToolVersion, c.Result, c.Rung, c.Level, c.Reason)}, lines); err == nil {
			t.Errorf("Read accepts a grid with a cell with %s", name)
		}
	}
	if err := proof.Validate([]model.Cell{good(), good()}); err == nil {
		t.Error("two cells for one key are accepted")
	}
	r6 := good()
	r6.Rung, r6.Result = 6, model.TruthReviewed
	if err := proof.Validate([]model.Cell{r6}); err != nil {
		t.Errorf("a rung-6 reviewed cell is refused: %v", err)
	}
}

func TestValidateNamesAHostileValueShortAndNeverPanics(t *testing.T) {
	t.Parallel()
	c := good()
	c.Result = model.Truth(strings.Repeat("\xff", 5000))
	err := proof.Validate([]model.Cell{c})
	if err == nil || len(err.Error()) > 400 {
		t.Errorf("Validate = %v, want one short refusal", err)
	}
}

func book() model.Book {
	b1 := model.Binding{Tool: "lint", Key: "k1", Rung: 3, Pair: 1}
	b2 := model.Binding{Tool: "review", Key: "k2", Rung: 6, Pair: 1}
	return model.Book{Rules: []model.Rule{{ID: "a.b.1", Bindings: []model.Binding{b1, b2}}}}
}

func cellFor(b model.Book, i int, tool string, r model.Truth) model.Cell {
	c := good()
	c.Binding = proof.BindingText(b.Rules[0].Bindings[i].Ref(b.Rules[0].ID))
	c.ToolVersion, c.Result, c.Rung = tool, r, int(b.Rules[0].Bindings[i].Rung)
	if r == model.TruthNotRun {
		c.Reason = model.ReasonToolMissing
	}
	return c
}

func TestTruthsReducesTheCellsOfABindingAndIgnoresWhatTheBookLacks(t *testing.T) {
	t.Parallel()
	b := book()
	ref := func(i int) model.BindingRef { return b.Rules[0].Bindings[i].Ref(b.Rules[0].ID) }
	stray := good()
	stray.Binding = "gone.1#ff"
	for name, tc := range map[string]struct {
		cells []model.Cell
		want  model.Grid
	}{
		"none":                {nil, model.Grid{}},
		"one pass":            {[]model.Cell{cellFor(b, 0, "1", model.TruthPass)}, model.Grid{ref(0): model.TruthPass}},
		"a fail beats a pass": {[]model.Cell{cellFor(b, 0, "1", model.TruthPass), cellFor(b, 0, "2", model.TruthFail)}, model.Grid{ref(0): model.TruthFail}},
		"not run beats pass":  {[]model.Cell{cellFor(b, 0, "1", model.TruthPass), cellFor(b, 0, "2", model.TruthNotRun)}, model.Grid{ref(0): model.TruthNotRun}},
		"fail beats not run":  {[]model.Cell{cellFor(b, 0, "1", model.TruthNotRun), cellFor(b, 0, "2", model.TruthFail)}, model.Grid{ref(0): model.TruthFail}},
		"reviewed":            {[]model.Cell{cellFor(b, 1, "1", model.TruthReviewed)}, model.Grid{ref(1): model.TruthReviewed}},
		"a review beside a pass of a rung-3 binding": {[]model.Cell{cellFor(b, 0, "1", model.TruthPass), {Binding: proof.BindingText(ref(0)), ToolVersion: "2", Result: model.TruthReviewed, Rung: 6, Level: model.RunPair}}, model.Grid{ref(0): model.TruthNotRun}},
		"a stray cell": {[]model.Cell{stray}, model.Grid{}},
	} {
		if got := proof.Truths(b, tc.cells); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: Truths = %v, want %v", name, got, tc.want)
		}
	}
}

// TestTheBindingTextIsFixedAndHoldsNoPath: a change to it moves every cell of
// every grid already written, so it is held to its form.
func TestTheBindingTextIsFixedAndHoldsNoPath(t *testing.T) {
	t.Parallel()
	got := proof.BindingText(model.BindingRef{Rule: "a.b.1", Binding: "x"})
	if got != "a.b.1#2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881" {
		t.Errorf("BindingText = %q", got)
	}
}

// FuzzValidate: a hostile grid is refused or held, and never panics.
func FuzzValidate(f *testing.F) {
	f.Add("a", "1", "pass", 3, "pair", "")
	f.Add("", "", "", -1, "", "x")
	f.Add("a", "1", "reviewed", 6, "fixture", "")
	f.Add("a", "1", "pass", 6, "fixture", "")
	f.Fuzz(func(t *testing.T, b, v, r string, rung int, l, why string) {
		c := model.Cell{Binding: b, ToolVersion: v, Result: model.Truth(r), Rung: rung, Level: model.RunLevel(l), Reason: model.Reason(why)}
		err := proof.Validate([]model.Cell{c})
		if err == nil && (b == "" || v == "" || rung < 0 || rung > 6 || (rung == 6 && (r == "pass" || r == "fail"))) {
			t.Fatalf("Validate accepted %+v", c)
		}
		_ = proof.Truths(book(), []model.Cell{c})
	})
}
