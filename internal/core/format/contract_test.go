package format

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
)

// treeAt is every file of the fixture rulebook under testdata/dir, by its
// path below it.
func treeAt(t *testing.T, dir string) map[string]string {
	t.Helper()
	base := filepath.Join("testdata", dir)
	out := map[string]string{}
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, p)
		out[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil || len(out) == 0 {
		t.Fatalf("the fixture %s: %d files, %v", dir, len(out), err)
	}
	return out
}

// foreignAt is the fixture rulebook under testdata/dir, loaded as the format
// loads any book, as coreapi would hand it in: the second fixture rulebook,
// base, is the parent, and other the import.
func foreignAt(t *testing.T, dir string) Foreign {
	t.Helper()
	w, root := world(t, treeAt(t, dir))
	l, err := load(t, w, root)
	if err != nil || len(l.Findings) != 0 {
		t.Fatalf("the fixture %s: %v, %v", dir, l.Findings, err)
	}
	f := Foreign{Rulebook: l.Book.Identity.Name, Version: l.Book.Identity.Version, Rules: l.Book.Rules}
	for _, layer := range l.Book.Structure.Layers {
		for _, p := range layer.Parts {
			f.Parts = append(f.Parts, p.Slug)
		}
	}
	return f
}

// composed is the fixture overlay's parent and import, both handed in.
func composed(t *testing.T) Composed {
	t.Helper()
	parent := foreignAt(t, "base")
	return Composed{Parent: &parent, Imports: []Foreign{foreignAt(t, "other")}}
}

// overlayWith is the fixture overlay with plant applied to its files, loaded
// with no finding.
func overlayWith(t *testing.T, plant func(*testing.T, map[string]string)) Loaded {
	t.Helper()
	files := fullBook(t)
	if plant != nil {
		plant(t, files)
	}
	w, root := world(t, files)
	l, err := load(t, w, root)
	if err != nil || len(l.Findings) != 0 {
		t.Fatalf("Load = %v, %v", l.Findings, err)
	}
	return l
}

// replaceIn is a plant that replaces old with new in the file at path, which
// must hold it.
func replaceIn(path, old, new string) func(*testing.T, map[string]string) {
	return func(t *testing.T, files map[string]string) {
		t.Helper()
		if !strings.Contains(files[path], old) {
			t.Fatalf("%s does not hold %q", path, old)
		}
		files[path] = strings.Replace(files[path], old, new, 1)
	}
}

// withRule is a plant that adds a rule to the fixture's go errors concern.
func withRule(rule string) func(*testing.T, map[string]string) {
	return func(_ *testing.T, files map[string]string) {
		files["language/go/rules/errors.yaml"] += rule
	}
}

// TestTheFixtureOverlayHoldsItsContractsWithItsParentAndImportHandedIn: the
// fixture overlay, its parent base and its import other handed in, is no
// finding and no notice, though its own part go shares the id go.errors.1
// with the import's: an import is namespaced by its rulebook's name.
// Tightening an imported rule passes as tightening the parent's does.
func TestTheFixtureOverlayHoldsItsContractsWithItsParentAndImportHandedIn(t *testing.T) {
	t.Parallel()
	for name, plant := range map[string]func(*testing.T, map[string]string){
		"the fixture":             nil,
		"tightening an import's":  replaceIn(errorsFile, "tightens: base.errors.1", "tightens: cli.flags.1"),
		"tightening by an alias":  replaceIn(errorsFile, "tightens: base.errors.1", "tightens: base.errs.1"),
		"a record of an import's": replaceIn("bindings/tighten-errors.yaml", "subject: base.errors.1", "subject: go.errors.5"),
		"a rule in its own part":  withRule("  - id: go.errors.3\n    statement: x.\n    strength: must\n    status: active\n"),
		"a record ended by no newline": func(_ *testing.T, files map[string]string) {
			files["bindings/tighten-errors.yaml"] = "subject: base.errors.2"
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			found, notices, err := Contracts(overlayWith(t, plant), composed(t))
			if err != nil || len(found) != 0 || len(notices) != 0 {
				t.Errorf("Contracts = %v, %v, %v; want nothing", found, notices, err)
			}
		})
	}
}

// TestEachPlantedBreachOfTheContractsIsOneFinding: with the parent and the
// import handed in, each breach is exactly one finding, at the file and line
// of what breaks it, saying so. Planted failures: each check of Contracts
// taken out in turn.
func TestEachPlantedBreachOfTheContractsIsOneFinding(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		plant  func(*testing.T, map[string]string)
		parent func(*Foreign)
		at     string
		says   string
	}{
		{name: "a rule under a parent's id", plant: withRule("  - id: base.errors.1\n    statement: x.\n    strength: must\n    status: active\n"),
			at: errorsFile + ":54", says: "base.errors.1 is the parent base 2.1.0's rule base.errors.1, and an overlay never edits a parent's rule in place"},
		{name: "an alias that is a parent's alias", plant: withRule("  - id: go.errors.3\n    aliases: [base.errs.1]\n    statement: x.\n    strength: must\n    status: active\n"),
			at: errorsFile + ":54", says: "go.errors.3 is the parent base 2.1.0's rule base.errs.1"},
		{name: "a rule added in the parent's part", plant: withRule("  - id: base.errors.9\n    statement: x.\n    strength: must\n    status: active\n"),
			at: errorsFile + ":54", says: "base.errors.9 is added in the part base, which the parent base 2.1.0 holds"},
		{name: "an import of a part not there", plant: replaceIn(IdentityFile, "parts: [go, cli]", "parts: [go, ui]"),
			at: IdentityFile + ":16", says: "imports the part ui, which other 0.3.0 does not hold"},
		{name: "a tightening of its own rule", plant: replaceIn(errorsFile, "tightens: base.errors.1", "tightens: go.errors.2"),
			at: errorsFile + ":48", says: "tightens names go.errors.2, this rulebook's own rule"},
		{name: "a tightening of its own alias", plant: replaceIn(errorsFile, "tightens: base.errors.1", "tightens: go.errs.1"),
			at: errorsFile + ":48", says: "tightens names go.errs.1, this rulebook's own rule"},
		{name: "a tightening of a rule no one holds", plant: replaceIn(errorsFile, "tightens: base.errors.1", "tightens: base.errors.9"),
			at: errorsFile + ":48", says: "tightens names base.errors.9, which neither the parent nor an imported part holds"},
		{name: "a tightening of a part not imported", plant: replaceIn(errorsFile, "tightens: base.errors.1", "tightens: web.forms.1"),
			at: errorsFile + ":48", says: "tightens names web.forms.1, which neither the parent nor an imported part holds"},
		{name: "a tightening two books hold", plant: replaceIn(errorsFile, "tightens: base.errors.1", "tightens: cli.flags.1"),
			parent: func(p *Foreign) { p.Rules = append(p.Rules, model.Rule{ID: "cli.flags.1"}) },
			at:     errorsFile + ":48", says: "tightens names cli.flags.1, which the parent base 2.1.0 and the import other 0.3.0 all hold"},
		{name: "a record whose first line is no subject", plant: func(_ *testing.T, files map[string]string) {
			files["bindings/tighten-errors.yaml"] = "not: a subject\nsubject: base.errors.1\n"
		},
			at: "bindings/tighten-errors.yaml:1", says: "a binding record names its subject on its first line"},
		{name: "a record of its own rule", plant: replaceIn("bindings/tighten-errors.yaml", "subject: base.errors.1", "subject: go.errors.1"),
			at: "bindings/tighten-errors.yaml:1", says: "subject names go.errors.1, this rulebook's own rule"},
		{name: "a record of a rule no one holds", plant: replaceIn("bindings/tighten-errors.yaml", "subject: base.errors.1", "subject: base.errors.3"),
			at: "bindings/tighten-errors.yaml:1", says: "subject names base.errors.3, which neither the parent nor an imported part holds"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			comp := composed(t)
			if c.parent != nil {
				c.parent(comp.Parent)
			}
			found, notices, err := Contracts(overlayWith(t, c.plant), comp)
			if err != nil || len(notices) != 0 {
				t.Fatalf("Contracts: notices %v, %v", notices, err)
			}
			if len(found) != 1 {
				t.Fatalf("Contracts found %d: %v; want one", len(found), found)
			}
			if got := fmt.Sprintf("%s:%d", found[0].File, found[0].Line); got != c.at || !strings.Contains(found[0].Msg, c.says) {
				t.Errorf("the finding is %s, want at %s saying %q", found[0], c.at, c.says)
			}
		})
	}
}

// TestABookNotHandedInIsANoticeNamingWhatWasNotChecked: with the parent or
// an import left out, Contracts gives one notice for each, naming the pin
// and the checks it skipped, and no finding, so the closing word is what
// Load's findings make it. A breach only the parent could show is not
// judged, and a tightening of the book's own rule still is. Planted
// failure: the notice taken out, or a subject held by none judged while the
// parent or an import is unread.
func TestABookNotHandedInIsANoticeNamingWhatWasNotChecked(t *testing.T) {
	t.Parallel()
	parentNotice := "parent not fetched: base 2.1.0, so the overlay contract against it was not checked"
	importNotice := "import not fetched: other 0.3.0, so the import contract against it was not checked"
	full := composed(t)
	cases := []struct {
		name  string
		plant func(*testing.T, map[string]string)
		comp  Composed
		want  []string
		finds int
	}{
		{name: "no parent", comp: Composed{Imports: full.Imports}, want: []string{parentNotice}},
		{name: "no import", comp: Composed{Parent: full.Parent}, want: []string{importNotice}},
		{name: "neither", want: []string{parentNotice, importNotice}},
		{name: "no parent, and a rule under a parent's id", comp: Composed{Imports: full.Imports},
			plant: withRule("  - id: base.errors.2\n    statement: x.\n    strength: must\n    status: active\n"), want: []string{parentNotice}},
		{name: "no parent, and a tightening of a rule only it could hold", comp: Composed{Imports: full.Imports},
			plant: replaceIn(errorsFile, "tightens: base.errors.1", "tightens: base.errors.9"), want: []string{parentNotice}},
		{name: "no import, and a tightening of a rule only it could hold", comp: Composed{Parent: full.Parent},
			plant: replaceIn(errorsFile, "tightens: base.errors.1", "tightens: cli.flags.1"), want: []string{importNotice}},
		{name: "neither, and a tightening of its own rule", plant: replaceIn(errorsFile, "tightens: base.errors.1", "tightens: go.errors.2"),
			want: []string{parentNotice, importNotice}, finds: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			l := overlayWith(t, c.plant)
			found, notices, err := Contracts(l, c.comp)
			if err != nil || len(found) != c.finds {
				t.Fatalf("Contracts = %v, %v; want %d findings", found, err, c.finds)
			}
			if len(notices) != len(c.want) {
				t.Fatalf("notices %v, want %d", notices, len(c.want))
			}
			for i, n := range notices {
				if n.File != IdentityFile || !strings.HasPrefix(n.Msg, c.want[i]) {
					t.Errorf("notice %d is %s: %s, want %s: %s", i, n.File, n.Msg, IdentityFile, c.want[i])
				}
			}
			if len(l.Findings) != 0 {
				t.Errorf("Load's findings moved: %v", l.Findings)
			}
		})
	}
}

// TestABookHandedInThatIsNotThePinIsAnError: a parent or an import of
// another name or release than the identity pins, a parent for a book that
// extends none, and the same import twice are ErrNotThePin, and nothing is
// checked. Planted failure: the match on name or on version taken out.
func TestABookHandedInThatIsNotThePinIsAnError(t *testing.T) {
	t.Parallel()
	full := composed(t)
	other := full.Imports[0]
	moved := *full.Parent
	moved.Version = "2.2.0"
	renamed := other
	renamed.Rulebook = "another"
	cases := map[string]struct {
		plant func(*testing.T, map[string]string)
		comp  Composed
	}{
		"a parent at another release": {comp: Composed{Parent: &moved, Imports: full.Imports}},
		"an import of another name":   {comp: Composed{Parent: full.Parent, Imports: []Foreign{renamed}}},
		"an import twice":             {comp: Composed{Parent: full.Parent, Imports: []Foreign{other, other}}},
		"a parent where none is pinned": {comp: Composed{Parent: full.Parent, Imports: full.Imports},
			plant: func(_ *testing.T, files map[string]string) {
				id := files[IdentityFile]
				start, end := strings.Index(id, "extends:"), strings.Index(id, "imports:")
				files[IdentityFile] = strings.Replace(id[:start]+id[end:], "kind: overlay", "kind: base", 1)
			}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			found, notices, err := Contracts(overlayWith(t, c.plant), c.comp)
			if !errors.Is(err, ErrNotThePin) || found != nil || notices != nil {
				t.Errorf("Contracts = %v, %v, %v; want ErrNotThePin alone", found, notices, err)
			}
		})
	}
}

// TestABaseBookThatTightensIsAFinding: a book that extends none and imports
// none has no rule to tighten but its own, so a tightening of any other id
// is a finding.
func TestABaseBookThatTightensIsAFinding(t *testing.T) {
	t.Parallel()
	l := Loaded{Book: model.Book{
		Identity:       model.RulebookIdentity{Name: "solo", Version: "1.0.0", Kind: model.KindBase},
		BindingRecords: []model.Opaque{{Path: "bindings/x.yaml", Data: []byte("subject: base.errors.1\n")}},
	}}
	found, notices, err := Contracts(l, Composed{})
	if err != nil || len(notices) != 0 || len(found) != 1 || !strings.Contains(found[0].Msg, "extends none and imports none") {
		t.Errorf("Contracts = %v, %v, %v; want one finding", found, notices, err)
	}
}

// TestABookHandedInThatHoldsAnIDTwiceIsOneHolder: a parent whose rules name
// one id twice still holds it once, so a tightening of it says which rule is
// meant and is no finding. Planted failure: a holder recorded at each name.
func TestABookHandedInThatHoldsAnIDTwiceIsOneHolder(t *testing.T) {
	t.Parallel()
	comp := composed(t)
	comp.Parent.Rules = append(comp.Parent.Rules, model.Rule{ID: "base.other.1", Aliases: []model.RuleID{"base.errors.1"}})
	found, notices, err := Contracts(overlayWith(t, nil), comp)
	if err != nil || len(found) != 0 || len(notices) != 0 {
		t.Errorf("Contracts = %v, %v, %v; want nothing", found, notices, err)
	}
}

// TestABookWhoseIdentityDidNotLoadIsNotChecked: with no identity there is
// no pin to check against, so Contracts checks nothing, and the identity's
// own findings are Load's to give. Planted failure: the check of the
// identity taken out, which finds the record's subject held by none.
func TestABookWhoseIdentityDidNotLoadIsNotChecked(t *testing.T) {
	t.Parallel()
	l := Loaded{Book: model.Book{
		BindingRecords: []model.Opaque{{Path: "bindings/x.yaml", Data: []byte("subject: base.errors.1\n")}},
	}}
	found, notices, err := Contracts(l, Composed{})
	if err != nil || found != nil || notices != nil {
		t.Errorf("Contracts = %v, %v, %v; want nothing", found, notices, err)
	}
}

// overlayOf is a generated overlay and its parent: parts from a short list,
// so ids collide often, each book's ids and aliases used once.
func overlayOf(r *configtest.Draw) (Loaded, Foreign) {
	parts := []string{"a", "b", "c", "d"}
	id := func() model.RuleID {
		return model.RuleID(fmt.Sprintf("%s.%s.%d", parts[r.IntN(len(parts))], []string{"x", "y"}[r.IntN(2)], 1+r.IntN(3)))
	}
	book := func(n int) []model.Rule {
		used := map[model.RuleID]bool{}
		var out []model.Rule
		for range n {
			rule := model.Rule{ID: id()}
			if used[rule.ID] {
				continue
			}
			used[rule.ID] = true
			for range r.IntN(3) {
				if a := id(); !used[a] {
					used[a] = true
					rule.Aliases = append(rule.Aliases, a)
				}
			}
			out = append(out, rule)
		}
		return out
	}
	parent := Foreign{Rulebook: "base", Version: "2.1.0", Rules: book(r.IntN(8))}
	for _, p := range parts {
		if r.IntN(2) == 0 {
			parent.Parts = append(parent.Parts, p)
		}
	}
	l := Loaded{Book: model.Book{
		Identity: model.RulebookIdentity{Name: "example", Version: "1.0.0", Kind: model.KindOverlay, Extends: &model.Extends{Rulebook: "base", Version: "2.1.0"}},
		Rules:    book(r.IntN(10)),
	}}
	return l, parent
}

// breaches is, by the contract's own words and not its code, which rules of
// l an overlay may not define: an id or alias the parent uses, or an id in
// a part the parent holds.
func breaches(l Loaded, parent Foreign) []model.RuleID {
	var taken []model.RuleID
	for _, r := range parent.Rules {
		taken = append(taken, r.ID)
		taken = append(taken, r.Aliases...)
	}
	var out []model.RuleID
	for _, r := range l.Book.Rules {
		mine := append([]model.RuleID{r.ID}, r.Aliases...)
		part, _, _ := strings.Cut(string(r.ID), ".")
		if slices.ContainsFunc(mine, func(x model.RuleID) bool { return slices.Contains(taken, x) }) || slices.Contains(parent.Parts, part) {
			out = append(out, r.ID)
		}
	}
	return out
}

// anOverlayNeverDefinesAParentsRule is the property: over a generated
// overlay and parent, the findings are exactly one for each rule that is
// the parent's or in its part, naming it, and none for any other.
func anOverlayNeverDefinesAParentsRule(t *testing.T, seed uint64) {
	t.Helper()
	l, parent := overlayOf(configtest.NewDraw(seed))
	found, _, err := Contracts(l, Composed{Parent: &parent})
	if err != nil {
		t.Fatal(err)
	}
	want := breaches(l, parent)
	var got []model.RuleID
	for _, f := range found {
		id, _, _ := strings.Cut(f.Msg, " ")
		got = append(got, model.RuleID(id))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("seed %d: the findings name %q, want %q (rules %v, parent %v)", seed, got, want, l.Book.Rules, parent)
	}
}

// TestAnOverlayNeverDefinesAParentsRuleOverGeneratedOverlays
// (tests.generated.2): the property over two thousand generated overlays.
// Planted failure: the alias or the part check of overlay taken out.
func TestAnOverlayNeverDefinesAParentsRuleOverGeneratedOverlays(t *testing.T) {
	t.Parallel()
	for seed := range uint64(2000) {
		anOverlayNeverDefinesAParentsRule(t, seed)
	}
}

// FuzzAnOverlayNeverDefinesAParentsRule is the same property over seeds the
// fuzzer picks.
func FuzzAnOverlayNeverDefinesAParentsRule(f *testing.F) {
	for _, s := range []uint64{0, 1, 7, 1 << 40} {
		f.Add(s)
	}
	f.Fuzz(anOverlayNeverDefinesAParentsRule)
}

// schemaID is a rule id as common.schema.json holds one.
var schemaID = regexp.MustCompile(`^[a-z]+\.[a-z0-9]+(-[a-z0-9]+)*\.[1-9][0-9]*$`)

// FuzzSubjectOf (tests.generated.1): whatever bytes a bindings/ record
// holds, the read does not panic, and a subject it answers is a rule id the
// schema holds, written exactly on the first line.
func FuzzSubjectOf(f *testing.F) {
	for _, s := range []string{"subject: base.errors.1\n", "subject: base.errors.1", "subject: base.errors.1\r\n", "subject:  base.errors.1\n", " subject: a.b.1\n",
		"subject: a.b.0\n", "subject: a.b-c.12\nrest", "not: [a schema", "", "\n", "subject: a.b.1 # note\n", "subject: A.b.1\n"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		id, ok := SubjectOf(data)
		if !ok {
			if id != "" {
				t.Fatalf("SubjectOf(%q) = %q and no subject", data, id)
			}
			return
		}
		line := "subject: " + string(id)
		rest, found := strings.CutPrefix(string(data), line)
		rest = strings.TrimPrefix(rest, "\r") // a CR ends the line with its LF
		if !schemaID.MatchString(string(id)) || !found || (rest != "" && rest[0] != '\n') {
			t.Fatalf("SubjectOf(%q) = %q, which is not the first line's rule id", data, id)
		}
	})
}
