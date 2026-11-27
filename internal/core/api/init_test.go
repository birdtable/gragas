package coreapi_test

import (
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func initForm() model.InitForm {
	return model.InitForm{
		Name: "demo", Kind: model.KindBase, Version: "1.0.0", Language: "en",
		Licences: model.Licences{Rules: "CC-BY-4.0", Presets: "MIT"}, Owners: []string{"team-a"},
		Governance: model.Governance{Contribution: model.ContributionProposals, DecisionRequired: true, TrialReleases: 3, ReviewDays: 90},
		Layer:      model.Named{Slug: "core", Name: "Core"}, Part: model.Named{Slug: "core", Name: "Core"},
	}
}

func initWorld(t *testing.T) (*porttest.World, model.Root) {
	t.Helper()
	w := porttest.NewWorld(t)
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/history.schema.json")
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{})
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "A Person", Email: "a@example.test"}})
	return w, root
}

func newInit(t *testing.T, w *porttest.World, root model.Root, f model.InitForm) *coreapi.Change {
	t.Helper()
	c, err := coreapi.InitRulebook(w.Ports(), root, f, coreapi.Lines{Command: "gragas init --kind rulebook", Applies: "gragas init --kind rulebook --apply", Who: "gragas init"}, "run-1", "v0.0.0-test")
	if err != nil {
		t.Fatalf("InitRulebook: %v", err)
	}
	return c
}

// TestInitWritesIdentityStructureAndTheFirstHistoryLine: an empty folder gets
// the two files and a first history line that is a literal expected one, the
// agreed command in it.
func TestInitWritesIdentityStructureAndTheFirstHistoryLine(t *testing.T) {
	t.Parallel()
	w, root := initWorld(t)
	out := newInit(t, w, root, initForm()).Apply(t.Context(), "")
	if out.Word != model.Done || out.Nothing || len(out.Changes) != 3 {
		t.Fatalf("outcome = %+v", out)
	}
	plan := string(out.Plan.Digest)
	want := `{"after":{"owner":"team-a"},"command":"gragas init --kind rulebook","entity":"rulebook:demo@1.0.0","format":3,"kind":"init","number":1,"plan":"` + plan +
		`","when":"2026-11-08T10:00:00Z","who":"gragas init"}`
	if got, _ := w.File(root, "history.jsonl"); strings.TrimSuffix(got, "\n") != want {
		t.Fatalf("history.jsonl = %q\nwant       %q", got, want)
	}
	if got, _ := w.File(root, "rulebook.yaml"); !strings.Contains(got, "version: 1.0.0\n") || !strings.Contains(got, "kind: base\n") {
		t.Fatalf("rulebook.yaml = %q", got)
	}
	l, err := format.Load(w.Ports().Tree, w.Ports().Schema, root)
	if err != nil || len(l.Findings) != 0 {
		t.Fatalf("Load = %+v, %v", l, err)
	}
}

// TestInitTwiceChangesNothingTheSecondTime: the plan of a rulebook that holds
// what the form says is empty.
func TestInitTwiceChangesNothingTheSecondTime(t *testing.T) {
	t.Parallel()
	w, root := initWorld(t)
	newInit(t, w, root, initForm()).Apply(t.Context(), "")
	out, ok := newInit(t, w, root, initForm()).Preview(t.Context())
	if ok || out.Word != model.Done || !out.Nothing {
		t.Fatalf("second preview = %+v, ok %v", out, ok)
	}
}

// TestInitDryRunWritesNothing: the preview shows the plan and leaves the
// folder empty.
func TestInitDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	w, root := initWorld(t)
	out, ok := newInit(t, w, root, initForm()).Preview(t.Context())
	if !ok || len(out.Plan.Files) != 2 || out.Plan.Digest == "" {
		t.Fatalf("preview = %+v, ok %v", out, ok)
	}
	if _, there := w.File(root, "rulebook.yaml"); there {
		t.Fatal("the dry run wrote rulebook.yaml")
	}
}

// TestInitRefusesAContentHashThatIsEmptyOrMalformed (S67): an overlay
// without its parent's hash, a hash that is no sha256 digest, and an import
// without one or with a bad one are each refused, naming the flag.
func TestInitRefusesAContentHashThatIsEmptyOrMalformed(t *testing.T) {
	t.Parallel()
	w, root := initWorld(t)
	good := model.Digest("sha256:" + strings.Repeat("a", 64))
	overlay := func(h model.Digest) model.InitForm {
		f := initForm()
		f.Kind = model.KindOverlay
		f.Extends = &model.Extends{Rulebook: "base", Version: "1.0.0", ContentHash: h}
		return f
	}
	imported := func(h model.Digest) model.InitForm {
		f := initForm()
		f.Imports = []model.Import{{Rulebook: "other", Version: "1.0.0", Parts: []string{"go"}, ContentHash: h}}
		return f
	}
	for _, tc := range []struct {
		name string
		form model.InitForm
		flag string // "" for a form that is accepted
	}{
		{"an overlay with its hash", overlay(good), ""},
		{"an overlay with no hash", overlay(""), "--parent-hash"},
		{"a hash with no sha256 prefix", overlay(model.Digest(strings.Repeat("a", 64))), "--parent-hash"},
		{"a hash too short", overlay("sha256:abc"), "--parent-hash"},
		{"a hash in capitals", overlay(model.Digest("sha256:" + strings.Repeat("A", 64))), "--parent-hash"},
		{"an import with its hash", imported(good), ""},
		{"an import with no hash", imported(""), "--import"},
		{"an import with a bad hash", imported("sha256:zz"), "--import"},
	} {
		_, err := coreapi.InitRulebook(w.Ports(), root, tc.form, coreapi.Lines{Command: "c", Applies: "c --apply", Who: "c"}, "r", "e")
		switch {
		case tc.flag == "" && err != nil:
			t.Errorf("%s: err = %v, want none", tc.name, err)
		case tc.flag != "" && (err == nil || !strings.Contains(err.Error(), tc.flag)):
			t.Errorf("%s: err = %v, want it to name %s", tc.name, err, tc.flag)
		}
	}
}

// TestInitRefusesAFormWithAFlagToChange: each problem names the flag that
// answers it.
func TestInitRefusesAFormWithAFlagToChange(t *testing.T) {
	t.Parallel()
	w, root := initWorld(t)
	for flag, mutate := range map[string]func(*model.InitForm){
		"--name":    func(f *model.InitForm) { f.Name = "Bad Name" },
		"--version": func(f *model.InitForm) { f.Version = "1.0" },
		"--parent":  func(f *model.InitForm) { f.Kind = model.KindOverlay },
		"--owner":   func(f *model.InitForm) { f.Owners = nil },
	} {
		f := initForm()
		mutate(&f)
		if _, err := coreapi.InitRulebook(w.Ports(), root, f, coreapi.Lines{Command: "c", Applies: "c --apply", Who: "c"}, "r", "e"); err == nil || !strings.Contains(err.Error(), flag) {
			t.Errorf("%s: err = %v", flag, err)
		}
	}
}

// TestEveryFormInitAcceptsIsARulebookTheSchemaAccepts: a form the init
// takes writes files that load with no finding, and a form with a version, a
// part or a name the schema would refuse is refused by init, naming
// the flag, so no rulebook is made that validate then rejects. Planted
// failure: a version written unquoted that holds a newline, which injects a
// key.
func TestEveryFormInitAcceptsIsARulebookTheSchemaAccepts(t *testing.T) {
	t.Parallel()
	hash := model.Digest("sha256:" + strings.Repeat("c", 64))
	overlay := func(edit func(*model.Extends)) model.InitForm {
		f := initForm()
		f.Kind = model.KindOverlay
		f.Extends = &model.Extends{Rulebook: "base", Version: "1.0.0", ContentHash: hash}
		edit(f.Extends)
		return f
	}
	imported := func(edit func(*model.Import)) model.InitForm {
		f := initForm()
		i := model.Import{Rulebook: "other", Version: "1.0.0", Parts: []string{"go", "ts"}, ContentHash: hash}
		edit(&i)
		f.Imports = []model.Import{i}
		return f
	}
	for _, tc := range []struct {
		name string
		form model.InitForm
		flag string // "" for a form that is taken
	}{
		{"a base", initForm(), ""},
		{"an overlay", overlay(func(*model.Extends) {}), ""},
		{"an overlay with a location for a name", overlay(func(e *model.Extends) { e.Rulebook = "example.test/rules/base" }), ""},
		{"an import of two parts", imported(func(*model.Import) {}), ""},
		{"a parent version that is two numbers", overlay(func(e *model.Extends) { e.Version = "1.0" }), "--parent-version"},
		{"a parent version with a newline", overlay(func(e *model.Extends) { e.Version = "1.0.0\nowners: []" }), "--parent-version"},
		{"a parent name with a newline", overlay(func(e *model.Extends) { e.Rulebook = "base\nx: y" }), "--parent"},
		{"an import version that is two numbers", imported(func(i *model.Import) { i.Version = "1.0" }), "--import"},
		{"an import version with a newline", imported(func(i *model.Import) { i.Version = "1.0.0\nparts: []" }), "--import"},
		{"an import part in capitals", imported(func(i *model.Import) { i.Parts = []string{"Go"} }), "--import"},
		{"an import part that is empty", imported(func(i *model.Import) { i.Parts = []string{"go", ""} }), "--import"},
		{"an import part named twice", imported(func(i *model.Import) { i.Parts = []string{"go", "go"} }), "--import"},
		{"an import name with a control character", imported(func(i *model.Import) { i.Rulebook = "ot\x00her" }), "--import"},
	} {
		w := porttest.NewWorld(t)
		w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json", "v3/history.schema.json")
		w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
		root := w.Seed("repo", map[string]string{})
		w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "A Person", Email: "a@example.test"}})
		c, err := coreapi.InitRulebook(w.Ports(), root, tc.form, coreapi.Lines{Command: "gragas init --kind rulebook", Applies: "gragas init --kind rulebook --apply", Who: "gragas init"}, "run-1", "v0.0.0-test")
		if tc.flag != "" {
			if err == nil || !strings.Contains(err.Error(), tc.flag) {
				t.Errorf("%s: err = %v, want it to name %s", tc.name, err, tc.flag)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: refused: %v", tc.name, err)
			continue
		}
		if out := c.Apply(t.Context(), ""); out.Word != model.Done {
			t.Errorf("%s: apply = %+v", tc.name, out)
			continue
		}
		loaded, err := format.Load(w.Ports().Tree, w.Ports().Schema, root)
		if err != nil || len(loaded.Findings) != 0 {
			t.Errorf("%s: the files init wrote load with err %v and findings %+v", tc.name, err, loaded.Findings)
		}
	}
}
