package coreapi_test

import (
	"slices"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
)

// TestEveryLineOfTheReportIsMaskedWhileAListIsHeld plants the stand-in name
// in checkers/: in a declaration's folder and its words, and in the path of a
// stray file. The names check reads what the load read of checkers/ too, so
// each path is a path finding and the declaration's line a line finding; and
// no line of the report says the name, whichever check made it, though the
// run with no list shows that link's own findings quote those paths. Planted
// failure: a path, or a finding's words, reach --json and SARIF holding a
// name the list refuses.
func TestEveryLineOfTheReportIsMaskedWhileAListIsHeld(t *testing.T) {
	t.Parallel()
	const plant = "alex"
	decl := "checkers/" + plant + "-plugin/checker.yaml"
	w, root := checkersWorld(t, bookIdentity, map[string]string{
		decl:                               "format: 3\nname: " + plant + "-plugin\ntool: lint-a\npublisher: team-a\n",
		"checkers/notes-" + plant + ".txt": "a stray file\n",
	})
	w.SeedOutside("/work/scratch/names.txt", "# stand-in list\n\\b"+plant+"\\b\n")
	open, err := coreapi.LoadBook(t.Context(), w.Ports(), root)
	if err != nil || !slices.ContainsFunc(open.Findings, func(f model.Finding) bool { return strings.Contains(f.Entity+f.Msg, plant) }) {
		t.Fatalf("with no list, no finding quotes a planted path, so masking is not shown: %+v, %v", open.Findings, err)
	}
	r, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, "/work/scratch/names.txt")
	if err != nil || r.NamesOff != "" {
		t.Fatalf("LoadBookNamed: %v, names off %q", err, r.NamesOff)
	}
	var paths, lines []string
	for _, f := range r.Findings {
		if strings.Contains(f.Entity+f.File+f.Msg+f.Fix, plant) {
			t.Errorf("a finding says the name: %+v", f)
		}
		switch f.Msg {
		case format.NamesPathMsg:
			paths = append(paths, f.Entity)
		case format.NamesMsg:
			lines = append(lines, f.Entity)
		}
	}
	for _, n := range r.Notices {
		if strings.Contains(n.Entity+n.File+n.Msg, plant) {
			t.Errorf("a notice says the name: %+v", n)
		}
	}
	slices.Sort(paths)
	masked := "book/checkers/" + format.MaskedName + "-plugin/checker.yaml"
	if want := []string{masked, "book/checkers/notes-" + format.MaskedName + ".txt"}; !slices.Equal(paths, want) {
		t.Errorf("path findings = %q, want %q", paths, want)
	}
	if want := []string{masked + ":2"}; !slices.Equal(lines, want) {
		t.Errorf("line findings = %q, want %q", lines, want)
	}
}

// TestWhyTheNamesCheckDidNotRunIsNamesOff: each way the check is off sets
// NamesOff to the notice's words, and a list that holds a name leaves it
// empty, so a caller whose whole job is the check can end not ready on it.
func TestWhyTheNamesCheckDidNotRunIsNamesOff(t *testing.T) {
	t.Parallel()
	w, root := checkersWorld(t, bookIdentity, nil)
	w.SeedOutside("/work/scratch/empty.txt", "# no name\n")
	w.SeedOutside("/work/scratch/names.txt", "\\bnobody-here\\b\n")
	for _, c := range []struct{ name, file, want string }{
		{"no list is named", "", "no names list was given"},
		{"the list is not there", "/work/scratch/absent.txt", "does not exist"},
		{"the list holds no name", "/work/scratch/empty.txt", "holds no name"},
		{"the list holds a name", "/work/scratch/names.txt", ""},
	} {
		r, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, c.file)
		if err != nil || (c.want == "") != (r.NamesOff == "") || !strings.Contains(r.NamesOff, c.want) {
			t.Errorf("%s, list %q: NamesOff = %q, %v; want it to hold %q", c.name, c.file, r.NamesOff, err, c.want)
		}
	}
}

// TestANamesFindingIsToldByItsKindUnderAListThatHoldsItsWords: a list that
// holds a word of the names finding's own phrase, by pattern or by hash,
// leaves that finding's words whole and NamesFound lists it, so a caller
// never takes a rulebook with a name for a clean one. The control, the list
// of the plant alone, finds the same one finding. Planted failure: the report
// masks a names finding's words, and a caller that tells it by them finds
// none and exits clean.
func TestANamesFindingIsToldByItsKindUnderAListThatHoldsItsWords(t *testing.T) {
	t.Parallel()
	const plant = "alex"
	phrase := strings.Fields(format.NamesMsg)[1] // "line", which no file here holds
	w, root := checkersWorld(t, bookIdentity, map[string]string{
		"checkers/lint-a/checker.yaml": "format: 3\nname: lint-a\ntool: lint-a\npublisher: " + plant + "\n",
	})
	lists := map[string]string{
		"the control": "\\b" + plant + "\\b\n",
		"a pattern":   "\\b" + plant + "\\b\n\\b" + phrase + "\\b\n",
		"a hash":      digest.Hex([]byte(plant)) + "\n" + digest.Hex([]byte(phrase)) + "\n",
	}
	for name, list := range lists {
		file := "/work/scratch/" + strings.ReplaceAll(name, " ", "-") + ".txt"
		w.SeedOutside(file, list)
		r, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, file)
		if err != nil || r.NamesOff != "" {
			t.Fatalf("%s: LoadBookNamed: %v, names off %q", name, err, r.NamesOff)
		}
		found := r.NamesFound()
		if len(found) != 1 || found[0].Msg != format.NamesMsg || found[0].Fix == "" {
			t.Errorf("%s: NamesFound = %+v; want the one line finding, in its own words", name, found)
		}
	}
}
