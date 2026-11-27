package coreapi_test

import (
	"errors"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/format"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// TestALoadsEndingIsMaskedWhileAListIsHeld plants the stand-in name in the
// folder of a rulebook written in a newer format, which the load refuses with
// an error that names the file. With no list the error says the path whole,
// so the plant is shown to reach it; under the list the error must not say
// the name, and errors.Is still sees ErrNotReady. Planted failure: maskErr
// returns its error unchanged, and a load's ending reaches the screen holding
// a name the list refuses.
func TestALoadsEndingIsMaskedWhileAListIsHeld(t *testing.T) {
	t.Parallel()
	const plant = "alex"
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	root := w.Seed("repo", map[string]string{plant + "/rulebook.yaml": strings.Replace(bookIdentity, "format: 3", "format: 4", 1)})
	w.SeedOutside("/work/scratch/names.txt", "# stand-in list\n\\b"+plant+"\\b\n")
	_, err := coreapi.LoadBook(t.Context(), w.Ports(), root)
	if !errors.Is(err, coreapi.ErrNotReady) || !strings.Contains(err.Error(), plant) {
		t.Fatalf("with no list the ending must say the planted path, or masking is not shown: %v", err)
	}
	_, err = coreapi.LoadBookNamed(t.Context(), w.Ports(), root, "/work/scratch/names.txt")
	if err == nil {
		t.Fatal("LoadBookNamed ended with no error")
	}
	if strings.Contains(err.Error(), plant) {
		t.Errorf("the load's ending says the name: %v", err)
	}
	if !errors.Is(err, coreapi.ErrNotReady) {
		t.Errorf("err = %v; want ErrNotReady still seen through the mask", err)
	}
}

// TestTheWordsOfAFindingAndANoticeAreMaskedWhileAListIsHeld: a finding that
// is not the names check's, and a notice, whose Msg or Fix quote a path that
// holds the stand-in. The run with no list shows each one saying it, and the
// run under the list shows none: Msg and Fix of the finding, Msg of the
// notice. Planted failure: masked leaves the Msg and Fix of a finding that
// is not the names check's (book_names.go:63), or the Msg of a notice (:66).
func TestTheWordsOfAFindingAndANoticeAreMaskedWhileAListIsHeld(t *testing.T) {
	t.Parallel()
	const plant = "alex"
	identity := bookIdentity + "imports:\n  - rulebook: " + plant + "-go\n    version: 2.0.0\n    parts: [ui]\n"
	w, root := checkersWorld(t, identity, map[string]string{
		"checkers/" + plant + "-plugin/checker.yaml": "format: 3\nname: " + plant + "-plugin\ntool: lint-a\npublisher: team-a\n",
	})
	w.SeedOutside("/work/scratch/names.txt", "# stand-in list\n\\b"+plant+"\\b\n")
	open, err := coreapi.LoadBook(t.Context(), w.Ports(), root)
	if err != nil {
		t.Fatal(err)
	}
	var findingWords, noticeWords int
	for _, f := range open.Findings {
		if strings.Contains(f.Msg+f.Fix, plant) {
			findingWords++
		}
	}
	for _, n := range open.Notices {
		if strings.Contains(n.Msg, plant) {
			noticeWords++
		}
	}
	if findingWords == 0 || noticeWords == 0 {
		t.Fatalf("with no list, %d findings say the name in Msg or Fix and %d notices in Msg: masking is not shown", findingWords, noticeWords)
	}
	r, err := coreapi.LoadBookNamed(t.Context(), w.Ports(), root, "/work/scratch/names.txt")
	if err != nil || r.NamesOff != "" {
		t.Fatalf("LoadBookNamed: %v, names off %q", err, r.NamesOff)
	}
	for _, f := range r.Findings {
		if f.Msg != format.NamesMsg && f.Msg != format.NamesPathMsg && f.Msg != format.NamesValueMsg && strings.Contains(f.Msg+f.Fix, plant) {
			t.Errorf("a finding's words say the name: %+v", f)
		}
	}
	for _, n := range r.Notices {
		if strings.Contains(n.Msg, plant) {
			t.Errorf("a notice's words say the name: %+v", n)
		}
	}
}
