// Package proof is the proof context (domain 4): the proof grid, the result of
// each binding under each tool version, as the engine holds it. It reads the
// grid's file through the Tree port and hands the standing derivation the
// truth of each binding. It decodes nothing itself: the grid document's bytes
// are read and written by the contract context, and arrive here as a decoder
// argument, since no context imports another (arch.ports.8).
package proof

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Path is the proof grid's file, relative to a rulebook's root. The grid is a
// generated document the owner commits (S57); release copies it.
const Path = "proof/grid.json"

// Decode reads the bytes of a proof grid document into its cells and tested set. It is the
// contract context's reader, handed in: its *NotReady for a grid in a newer
// version, and any other refusal, pass through Read unchanged.
type Decode func(data []byte) (model.ProofGrid, error)

// Read is the cells of the proof grid at Path below root. A file that is not
// there is the empty grid, not an error: a rulebook that has proved nothing has
// no grid. A file that is there is decoded, and then held to the invariant of
// Validate, so a grid with a blank cell is refused whole, never used in part.
// An error from decode, such as a grid in a newer contract version, is
// returned wrapped, and errors.As finds it.
func Read(tree port.Tree, root model.Root, decode Decode) (model.ProofGrid, error) {
	data, err := tree.Read(root, Path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.ProofGrid{}, nil
	}
	if err != nil {
		return model.ProofGrid{}, fmt.Errorf("read %s: %w", Path, err)
	}
	grid, err := decode(data)
	if err != nil {
		return model.ProofGrid{}, fmt.Errorf("read %s: %w", Path, err)
	}
	if err := Validate(grid.Cells); err != nil {
		return model.ProofGrid{}, fmt.Errorf("read %s: %w", Path, err)
	}
	return grid, nil
}

// Validate is the invariant of a grid: no cell is blank, and no key has two
// cells. A cell names its binding, its tool version and a result of the four,
// carries a run level and a rung from 0 to 6, never holds pass or fail at
// rung 6 (S9), and has a reason when it was not run and none otherwise. An
// adapter version may be empty, for a run no adapter made. Every breach is
// named, in the order of the cells; none is a panic.
func Validate(cells []model.Cell) error {
	var bad []error
	seen := map[model.CellKey]int{}
	for i, c := range cells {
		at := fmt.Sprintf("cell %d", i+1)
		for _, why := range breaches(c) {
			bad = append(bad, fmt.Errorf("%s: %s", at, why))
		}
		if first, dup := seen[c.Key()]; dup {
			bad = append(bad, fmt.Errorf("%s: the same binding and tool version as cell %d", at, first))
		}
		seen[c.Key()] = i + 1
	}
	return errors.Join(bad...)
}

// breaches is what is wrong with c alone, each in a sentence.
func breaches(c model.Cell) []string {
	var out []string
	if c.Binding == "" || c.ToolVersion == "" {
		out = append(out, "a cell names its binding and its tool version")
	}
	if !slices.Contains(model.Truths(), c.Result) {
		out = append(out, fmt.Sprintf("the result %q is none of pass, fail, not run or reviewed", clip(string(c.Result))))
	}
	if !slices.Contains(model.RunLevels(), c.Level) {
		out = append(out, fmt.Sprintf("the run level %q is none of fixture, pair or project", clip(string(c.Level))))
	}
	if c.Rung < 0 || c.Rung > 6 {
		out = append(out, fmt.Sprintf("the rung %d is not 0 to 6", c.Rung))
	}
	if c.Rung == 6 && (c.Result == model.TruthPass || c.Result == model.TruthFail) {
		out = append(out, fmt.Sprintf("a rung-6 cell is never %s: a review is never pass or fail", c.Result))
	}
	if c.Result == model.TruthReviewed && c.Rung != 6 {
		out = append(out, "a reviewed cell is rung 6: only a review is reviewed")
	}
	switch {
	case c.Result == model.TruthNotRun && !slices.Contains(model.Reasons(), c.Reason):
		out = append(out, fmt.Sprintf("a cell that was not run says why, and %q is none of no pair, tool missing, limit hit or sandbox missing", clip(string(c.Reason))))
	case c.Result != model.TruthNotRun && c.Reason != "":
		out = append(out, "a cell that has a result gives no reason")
	}
	return out
}

// BindingText is the text a cell names a binding by: the rule's id and the
// digest of what decides the binding (model.Binding.Ref), so a binding since
// changed or unbound matches no cell, and the text carries no path or name of
// the host (S57).
func BindingText(ref model.BindingRef) string {
	return string(ref.Rule) + "#" + digest.Hex([]byte(ref.Binding))
}

// Truths is the grid the standing derivation reads: for each binding of the
// book that has a cell, the truth the cells give it across every tool version
// they name. One failing cell makes the binding fail, else one not run makes
// it not run, else its cells agree: pass, or reviewed. A cell whose rung is not
// the binding's reads as not run, so a cell cannot state a rung to be taken
// for a review. The tool versions are taken as the cells name them: pass does
// not say that every version of a tested set has a cell (release's). A binding with no cell
// is not in the grid, and reads as not run. A cell for a binding the book no
// longer holds matches none, and changes nothing.
func Truths(book model.Book, cells []model.Cell) model.Grid {
	byText := map[string][]model.Cell{}
	for _, c := range cells {
		byText[c.Binding] = append(byText[c.Binding], c)
	}
	grid := model.Grid{}
	for _, r := range book.Rules {
		for _, b := range r.Bindings {
			ref := b.Ref(r.ID)
			got, ok := byText[BindingText(ref)]
			if !ok {
				continue
			}
			results := make([]model.Truth, len(got))
			for i, c := range got {
				results[i] = c.Result
				if c.Rung != int(b.Rung) {
					results[i] = model.TruthNotRun // a cell that states another rung proves nothing here
				}
			}
			grid[ref] = reduce(results)
		}
	}
	return grid
}

// reduce is the one truth of the results one binding has under its tool
// versions: fail beats not run, and not run beats the rest.
func reduce(results []model.Truth) model.Truth {
	switch {
	case slices.Contains(results, model.TruthFail):
		return model.TruthFail
	case slices.Contains(results, model.TruthNotRun):
		return model.TruthNotRun
	case slices.Contains(results, model.TruthPass):
		return model.TruthPass
	}
	return model.TruthReviewed
}

// clip is s cut for a message, so a hostile grid cannot make an error long.
func clip(s string) string {
	const most = 40
	s = strings.ToValidUTF8(s, "?")
	if runes := []rune(s); len(runes) > most {
		return string(runes[:most]) + "..."
	}
	return s
}
