package coreapi_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// loadCall is a call of the book load that takes the statement digests.
var loadCall = regexp.MustCompile(`\bloadBook(With|Composed)\(([^()]*)\)`)

// TestOnlyMigratesReadBackHandsTheLoaderDigestsItBuilt: no load turns the
// prose lint off. Every caller of the book load hands it nil, so the digests
// are read from the history, but migrate's read-back, which hands in the
// ones its conversion has just built. Risk: a command that loads a rulebook
// without the lint.
func TestOnlyMigratesReadBackHandsTheLoaderDigestsItBuilt(t *testing.T) {
	t.Parallel()
	src := os.DirFS(filepath.Join("..", ".."))
	var found []string
	err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "SkipProse") {
			found = append(found, p+": SkipProse")
		}
		for _, m := range loadCall.FindAllStringSubmatch(string(b), -1) {
			args := strings.Split(m[2], ",")
			if strings.Contains(m[2], "p port.Ports") {
				continue // the declaration
			}
			last := strings.TrimSpace(args[len(args)-1])
			if m[1] == "Composed" {
				last = strings.TrimSpace(args[len(args)-2])
			}
			if last != "nil" {
				found = append(found, p+": loadBook"+m[1]+"(..., "+last+")")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"core/api/book_load.go: loadBookComposed(..., statements)", "core/api/migrate.go: loadBookWith(..., m.Statements)"}
	if !slices.Equal(found, want) {
		t.Errorf("the loads handing in digests are %q, want only %q", found, want)
	}
}

// TestMigrateHoldsAMigratedRuleUntilAHandEditChangesIt: the written rulebook
// holds its migrated rules by the history until a hand edit changes one, which
// is then linted. The planted hand edit: arch.layers.2's statement is not in
// the imperative, and is held until it is reworded. Risk: a hold that outlives
// the text it was given for.
func TestMigrateHoldsAMigratedRuleUntilAHandEditChangesIt(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{"parts.yaml": v2Parts, "sources.yaml": v2Sources, "products.yaml": v2Products, "base/arch/rules/layers.yaml": v2Layers})
	w.SeedRepo(root, porttest.Repo{Who: &model.Identity{Name: "Ada", Email: "ada@example.org"}})
	ctx := context.Background()
	var doc coreapi.Migration
	c, err := coreapi.Migrate(w.Ports(), root, migrateForm(), coreapi.MigrateRun{Command: "gragas migrate", Applies: "gragas migrate --apply", Who: "gragas migrate", Run: "run-1", Engine: "v0.0.0-test"},
		func(m coreapi.Migration) { doc = m })
	if err != nil {
		t.Fatal(err)
	}
	pre, ok := c.Preview(ctx)
	if !ok {
		t.Fatalf("the plan ended %q (%v)", pre.Word, pre.Cause)
	}
	if doc.Statements != 2 || doc.Prose != 0 {
		t.Errorf("the plan records %d digests and %d prose findings, want 2 and 0", doc.Statements, doc.Prose)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	files := map[string]string{}
	listed, err := w.Ports().Tree.List(root, "", model.Every)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range listed {
		if !e.Dir {
			files[e.Path] = read(t, w, root, e.Path)
		}
	}
	rep, err := coreapi.LoadBook(t.Context(), w.Ports(), root)
	if err != nil || len(rep.Findings) != 0 || rep.Held != 2 {
		t.Fatalf("the written rulebook: findings %v, held %d, err %v; want none, 2 held", rep.Findings, rep.Held, err)
	}
	edited := 0
	for p, text := range files {
		if strings.Contains(text, "statement: A reader reads the record.") {
			files[p] = strings.Replace(text, "statement: A reader reads the record.", "statement: A reader reads the whole record.", 1)
			edited++
		}
	}
	if edited != 1 {
		t.Fatalf("the statement is in %d files, want 1", edited)
	}
	again := w.Seed("edited", files)
	rep, err = coreapi.LoadBook(t.Context(), w.Ports(), again)
	if err != nil || rep.Held != 1 || len(rep.Findings) != 1 || !strings.Contains(rep.Findings[0].Msg, "which is not the imperative") {
		t.Errorf("after the hand edit: findings %v, held %d, err %v; want the one prose finding, 1 held", rep.Findings, rep.Held, err)
	}
}

const v2Parts = `layers:
  - key: base
    name: Base
parts:
  - short: arch
    layer: base
    name: Architecture
    owner: operator
`

const v2Products = `kinds:
  - key: cli
    name: Command-line program
    description: A program run from a terminal.
`

const v2Sources = `sources:
  one-book:
    kind: book
    title: A Book
    locator: isbn:9780321293534
`

const v2Layers = `part: arch
concern: layers
order: 1
title: Layers
rules:
  - id: arch.layers.1
    rule: Keep packages free of cycles.
    strength: Hard
    status: active
    sources:
      - ref: one-book
        where: chapter 2
        backs: rule
  - id: arch.layers.2
    rule: A reader reads the record.
    strength: Record
    status: proposed
`

func migrateForm() model.InitForm {
	return model.InitForm{
		Name: "demo", Kind: model.KindBase, Version: "1.0.0", Language: "en",
		Licences: model.Licences{Rules: "CC-BY-4.0", Presets: "MIT"}, Owners: []string{"team-a"},
		Governance: model.Governance{Contribution: model.ContributionProposals, DecisionRequired: true, TrialReleases: 3, ReviewDays: 90},
	}
}

// TestMigrateRefusesAFormWithoutAnOwner: a form the format cannot hold ends
// before any plan is made. Risk: a plan written for a rulebook with no owner.
func TestMigrateRefusesAFormWithoutAnOwner(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("r", map[string]string{})
	if _, err := coreapi.Migrate(w.Ports(), root, model.InitForm{}, coreapi.MigrateRun{Command: "gragas migrate", Applies: "gragas migrate --apply", Who: "gragas migrate", Run: "run", Engine: "v"}, nil); err == nil {
		t.Error("migrate accepted a form with no name and no owner")
	}
}

// TestTheMigratePlanSaysHowManyStatementDigestsItRecords: the plan names the
// digests the history entry records, and says so when the rulebook held no
// rule. Risk: a migrate whose hold on the prose lint the owner never sees.
func TestTheMigratePlanSaysHowManyStatementDigestsItRecords(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{
		0: "the history records no statement digest: the rulebook holds no rule",
		2: "the history records 2 statement digests: the prose lint holds each migrated rule until its statement changes",
	} {
		lines := coreapi.MigrateLines(coreapi.Migration{Statements: n})
		if !slices.Contains(lines, want) {
			t.Errorf("with %d digests the lines are %q, want %q among them", n, lines, want)
		}
	}
}

// TestTheMigratePlanPrintsTheStrengthMappingOnceAndTheParkedFile: one line
// maps the old strength words to the format's, with the count of each, and one
// names the parked file with its line count, before the remainder in no file.
// Risk: a fold the owner never sees, or values moved to a file the plan does
// not name.
func TestTheMigratePlanPrintsTheStrengthMappingOnceAndTheParkedFile(t *testing.T) {
	t.Parallel()
	lines := coreapi.MigrateLines(coreapi.Migration{
		Parked:     coreapi.Parked{Path: "rulebook/parked.yaml", Lines: 1234},
		NotCarried: []coreapi.NotCarried{{Item: "parts.yaml waivers", Reason: "r"}},
		Strengths: []coreapi.Strength{
			{From: []string{"Hard"}, To: "must", Rules: 895},
			{From: []string{"Answered", "Session", "Record"}, To: "should", Rules: 55},
			{From: []string{"Lens", "Review"}, To: "may", Rules: 1},
		},
	})
	strengths := "strengths: Hard to must, 895 rules; Answered, Session and Record to should, 55 rules; Lens and Review to may, 1 rule; the parked file keeps each word folded with another"
	parked := "parked: 1,234 lines in rulebook/parked.yaml, each value with no field in the format, by rule id, then field; no program reads it"
	at := map[string]int{}
	for i, l := range lines {
		for _, p := range []string{"strengths:", "parked:", "not carried:"} {
			if strings.HasPrefix(l, p) {
				at[p]++
				at[p+"at"] = i
			}
		}
	}
	if !slices.Contains(lines, strengths) || !slices.Contains(lines, parked) || at["strengths:"] != 1 || at["parked:"] != 1 || at["parked:at"] > at["not carried:at"] {
		t.Errorf("the lines are %q, want once each %q and %q, before the remainder", lines, strengths, parked)
	}
}

// TestTheMigratePlanSaysWhereEachOldRelationWent: one line, after the
// strengths, names the relation each old how word went to with its count, and
// that the parked file keeps each word that went to relates and each winner
// named; a rulebook with no relation has no such line. Risk: a declared
// conflict, or a word, moved by a fold the owner never sees.
func TestTheMigratePlanSaysWhereEachOldRelationWent(t *testing.T) {
	t.Parallel()
	lines := coreapi.MigrateLines(coreapi.Migration{
		Parked:    coreapi.Parked{Path: "rulebook/parked.yaml", Lines: 3},
		Strengths: []coreapi.Strength{{From: []string{"Hard"}, To: "must", Rules: 1}},
		Relations: []coreapi.Relation{{How: "see", To: "relates", Relations: 649}, {How: "refines", To: "relates", Relations: 7}, {How: "conflicts", To: "conflicts", Relations: 2}},
		Wins:      2,
	})
	want := "relations: see to relates, 649 relations; refines to relates, 7 relations; conflicts to conflicts, 2 relations; the parked file keeps each word that went to relates, and the 2 winners named, since at format 3 a winner is a decision"
	at := slices.Index(lines, want)
	if at < 1 || !strings.HasPrefix(lines[at-1], "strengths:") || !strings.HasPrefix(lines[at+1], "parked:") {
		t.Errorf("the lines are %q, want %q between the strengths and the parked file", lines, want)
	}
	for _, l := range coreapi.MigrateLines(coreapi.Migration{Strengths: []coreapi.Strength{{From: []string{"Hard"}, To: "must"}}}) {
		if strings.HasPrefix(l, "relations:") {
			t.Errorf("with no relation the plan prints %q", l)
		}
	}
}

// TestMigrateRecordsItsCommandAsWhoAndReadsNoIdentity: in a
// repository that can name nobody the migrate plans and applies, and the line
// it writes names the command. Planted break: migrate reading History.Who
// again, which is an error here, or recording nothing.
func TestMigrateRecordsItsCommandAsWhoAndReadsNoIdentity(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.SeedSchemas(allSchemas()...)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{"parts.yaml": v2Parts, "sources.yaml": v2Sources, "products.yaml": v2Products, "base/arch/rules/layers.yaml": v2Layers})
	w.SeedRepo(root, porttest.Repo{})
	c, err := coreapi.Migrate(w.Ports(), root, migrateForm(), coreapi.MigrateRun{Command: "gragas migrate", Applies: "gragas migrate --apply", Who: "gragas migrate", Run: "run-1", Engine: "v0.0.0-test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out := c.Apply(t.Context(), ""); out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if h := read(t, w, root, "history.jsonl"); !strings.Contains(h, `"who":"gragas migrate"`) {
		t.Errorf("the history line = %s, want who gragas migrate", h)
	}
}
