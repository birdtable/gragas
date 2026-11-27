package root

import (
	"errors"
	"io/fs"

	"github.com/birdtable/gragas/internal/port"
)

// Place is what the working folder is to gragas: a rulebook's repository, a
// project, both at once, or neither (spec 15.1, S19). It decides which group
// the bare menu leads with. It is not the Context of a command, which is where
// a command may run.
type Place string

// The four places.
const (
	PlaceRulebook Place = "rulebook"
	PlaceProject  Place = "project"
	PlaceBoth     Place = "both"
	PlaceNeither  Place = "neither"
)

// What marks each place, relative to the working folder: a rulebook has its
// identity file, or until M6 the parts table of schema v2 the owner's rulebook
// is still in; a project has its pin.
func rulebookMarks() []string { return []string{"rulebook.yaml", "rulebook/parts.yaml"} }
func projectMarks() []string  { return []string{".gragas/pin.yaml"} }

// placeOf reads the working folder through the Tree port. A folder that cannot
// be opened or read is neither: the menu then leads with init.
func placeOf(tree port.Tree) Place {
	root, err := tree.Open("")
	if err != nil {
		return PlaceNeither
	}
	has := func(marks []string) bool {
		for _, m := range marks {
			if _, err := tree.Stat(root, m); err == nil {
				return true
			} else if !errors.Is(err, fs.ErrNotExist) {
				continue
			}
		}
		return false
	}
	rb, pr := has(rulebookMarks()), has(projectMarks())
	switch {
	case rb && pr:
		return PlaceBoth
	case rb:
		return PlaceRulebook
	case pr:
		return PlaceProject
	}
	return PlaceNeither
}
