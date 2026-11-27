package newcommand

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func words() Words {
	return Words{
		Base:    []string{"clear", "edit", "explain", "get", "list", "new", "restore"},
		Groups:  []string{"rule", "term"},
		Refused: []string{"get", "make", "destroy"},
	}
}

func taken() map[string][]string {
	return map[string][]string{"rule": {"new", "edit"}, "term": {"new"}, "explain": nil}
}

// TestANameIsRefusedByTheRegistrysRules: each refusal says why, and a name the
// registry would take is not refused (the clean control).
func TestANameIsRefusedByTheRegistrysRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		n    Name
		says string
	}{
		{"a refused verb", Name{Verb: "get"}, "is refused"},
		{"a word off the base list", Name{Verb: "frobnicate"}, "not on the base list"},
		{"a name already declared", Name{Verb: "explain"}, "already declared"},
		{"a group typed alone", Name{Verb: "rule"}, "is a group and takes a verb"},
		{"a verb the group has", Name{Group: "rule", Verb: "edit"}, "already declared"},
		{"a group that is not one", Name{Group: "explain", Verb: "list"}, "is not a group"},
		{"no verb", Name{Group: "rule"}, "a command has a verb"},
		{"a word that is not letters", Name{Verb: "Edit1"}, "lower-case letters"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Refuse(words(), tc.n, taken())
			if err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("got %v, want a refusal saying %q", err, tc.says)
			}
		})
	}
	for _, ok := range []Name{{Verb: "clear"}, {Group: "term", Verb: "restore"}, {Group: "rule", Verb: "list"}} {
		t.Run(ok.Verb, func(t *testing.T) {
			t.Parallel()
			if err := Refuse(words(), ok, taken()); err != nil {
				t.Errorf("%s was refused: %v", ok, err)
			}
		})
	}
}

// plannedGroup is a planned row of a group, as the seed of its package.
func plannedGroup() *Seed {
	s := stub(Name{Group: "term", Verb: "new"})
	s.Order, s.Then = 70, []string{"new", "retire"}
	return &s
}

const termPlanned = plannedDir + "term.yaml"

func templates() fstest.MapFS {
	fs := fstest.MapFS{}
	for _, name := range []string{"command.go.tmpl", "verb.go.tmpl", "golden_test.go.tmpl", "api.go.tmpl", "unit_test.go.tmpl", "journey_test.go.tmpl"} {
		fs["files/"+name] = &fstest.MapFile{Data: []byte("package p\n\n// {{.Name}} {{.File}} {{.Camel}} {{.Pascal}} {{.Family}}\n")}
	}
	return fs
}

func TestAPlannedGroupBecomesItsPackageAndItsRowGoes(t *testing.T) {
	t.Parallel()
	tree := fstest.MapFS{termPlanned: &fstest.MapFile{Data: []byte("x")}}
	p, err := Make(templates(), tree, Name{Group: "term", Verb: "new"}, plannedGroup())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"internal/cli/cmd/term/command.go", "internal/cli/cmd/term/new.go", "internal/cli/cmd/term/new_test.go",
		"internal/core/api/term_new.go", "internal/core/api/term_new_test.go", "internal/journeys/term_new_test.go"}
	if got := slices.Sorted(maps.Keys(p.Files)); !slices.Equal(got, want) {
		t.Errorf("files %v, want %v", got, want)
	}
	if !slices.Equal(p.Removes, []string{termPlanned}) {
		t.Errorf("removes %v, want the planned row", p.Removes)
	}
	if got := p.Files["internal/cli/cmd/term/command.go"]; !strings.Contains(got, "// term new term_new termNew TermNew term\n") {
		t.Errorf("rendered %q", got)
	}
	if !slices.IsSorted(p.Paths()) {
		t.Error("paths are not sorted")
	}
}

func TestAFlatCommandIsAPackageOfItsOwnAndAFileThatExistsIsNeverTaken(t *testing.T) {
	t.Parallel()
	tree := fstest.MapFS{}
	p, err := Make(templates(), tree, Name{Verb: "clear"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"internal/cli/cmd/clear/command.go", "internal/cli/cmd/clear/command_test.go", "internal/core/api/clear.go"} {
		if _, ok := p.Files[path]; !ok {
			t.Errorf("%s is not in the plan %v", path, p.Paths())
		}
	}
	if len(p.Removes) != 0 || len(p.Files) != 5 {
		t.Errorf("a flat command with no planned row writes 5 files and removes none: %v", p.Paths())
	}
	tree["internal/core/api/clear.go"] = &fstest.MapFile{Data: []byte("mine\n")}
	if _, err := Make(templates(), tree, Name{Verb: "clear"}, nil); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Errorf("a file that exists was taken: %v", err)
	}
}

func TestAVerbOfABuiltPackageAddsItsFilesAndSaysWhatIsLeftByHand(t *testing.T) {
	t.Parallel()
	tree := fstest.MapFS{"internal/cli/cmd/term/command.go": &fstest.MapFile{Data: []byte("package term\n")}}
	p, err := Make(templates(), tree, Name{Group: "term", Verb: "retire"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Files["internal/cli/cmd/term/command.go"]; ok {
		t.Error("the package file of a package that exists was written")
	}
	if len(p.Files) != 5 || !strings.Contains(p.Note, "by hand") {
		t.Errorf("%v, note %q", p.Paths(), p.Note)
	}
}

func TestAGroupStillInTheOldTableIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := Make(templates(), fstest.MapFS{}, Name{Group: "rule", Verb: "list"}, nil); err == nil || !strings.Contains(err.Error(), "old table") {
		t.Errorf("a group with no package and no planned file was taken: %v", err)
	}
}

// TestAPlanThatTouchesAFileOutsideItsPackageIsRefused: a template that lands
// in a hand-written file of the tree, and a plan that removes another command's
// planned row, are each refused naming the path; the plan of the real templates
// is the clean control.
func TestAPlanThatTouchesAFileOutsideItsPackageIsRefused(t *testing.T) {
	t.Parallel()
	n := Name{Verb: "clear"}
	clean, err := Make(templates(), fstest.MapFS{}, n, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Outside(n, clean); err != nil {
		t.Fatalf("the clean control is refused: %v", err)
	}
	for _, bad := range []struct {
		name string
		plan Plan
		says string
	}{
		{"a file of the root", Plan{Files: map[string]string{"internal/cli/root/registry_data.go": "x"}}, "internal/cli/root/registry_data.go"},
		{"a file of another command", Plan{Files: map[string]string{"internal/cli/cmd/check/command.go": "x"}}, "internal/cli/cmd/check/command.go"},
		{"a file of the core that is not its own", Plan{Files: map[string]string{"internal/core/api/validate.go": "x"}}, "internal/core/api/validate.go"},
		{"another command's planned row", Plan{Removes: []string{plannedDir + "explain.yaml"}}, plannedDir + "explain.yaml"},
		{"a path that climbs out of its package", Plan{Files: map[string]string{"internal/cli/cmd/clear/../../root/x.go": "x"}}, "internal/cli/cmd/clear/../../root/x.go"},
		{"an absolute path", Plan{Files: map[string]string{"/internal/cli/cmd/clear/x.go": "x"}}, "/internal/cli/cmd/clear/x.go"},
		{"a removal that climbs", Plan{Removes: []string{plannedDir + "../planned/clear.yaml"}}, plannedDir + "../planned/clear.yaml"},
	} {
		if err := Outside(n, bad.plan); err == nil || !strings.Contains(err.Error(), bad.says) {
			t.Errorf("%s: got %v, want a refusal naming %s", bad.name, err, bad.says)
		}
	}
}

// TestDriftFiresOnAChangedTemplate: the golden files equal the render (the
// control), and a template changed without them is told.
func TestDriftFiresOnAChangedTemplate(t *testing.T) {
	t.Parallel()
	sample := Name{Group: "term", Verb: "restore"}
	files, err := Render(templates(), sample)
	if err != nil {
		t.Fatal(err)
	}
	golden := fstest.MapFS{}
	for path, text := range files {
		golden["golden/"+path+".golden"] = &fstest.MapFile{Data: []byte(text)}
	}
	if d, err := Drift(templates(), golden, sample); err != nil || len(d) != 0 {
		t.Fatalf("the clean control drifts: %v %v", d, err)
	}
	changed := templates()
	changed["files/api.go.tmpl"] = &fstest.MapFile{Data: []byte("// changed {{.Name}}\n")}
	d, err := Drift(changed, golden, sample)
	if err != nil || len(d) != 1 || !strings.Contains(d[0], "differs from what the templates render") {
		t.Errorf("a changed template was not told: %v %v", d, err)
	}
	golden["golden/extra.go.golden"] = &fstest.MapFile{Data: []byte("x")}
	if d, _ := Drift(templates(), golden, sample); len(d) != 1 || !strings.Contains(d[0], "no template renders") {
		t.Errorf("a stray golden file was not told: %v", d)
	}
}
