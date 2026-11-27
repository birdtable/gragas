package guard_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/cli/cmd/guard"
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

// session is a run that keeps what the command said, and answers GuardEdit
// with what the test set.
type session struct {
	kit.Session
	json   bool
	guard  model.Guard
	ref    *model.Refusal
	err    error
	doc    any
	aside  []string
	ending *kit.Ending
	// mergeHook is what guard staged was asked: --merge-hook given or not
	mergeHook bool
}

func (s *session) JSON() bool     { return s.json }
func (s *session) Document(v any) { s.doc = v }
func (s *session) Aside(l string) { s.aside = append(s.aside, l) }
func (s *session) GuardEdit(context.Context) (model.Guard, *model.Refusal, error) {
	return s.guard, s.ref, s.err
}
func (s *session) GuardStaged(_ context.Context, mergeHook bool) (model.Guard, *model.Refusal, error) {
	s.mergeHook = mergeHook
	return s.guard, s.ref, s.err
}
func (s *session) End(e kit.Ending) int {
	s.ending = &e
	c, _ := exit.Code(e.Word)
	return c
}

// status is a refusal the core makes of an edit that changes a rule's status.
func status() (model.Guard, *model.Refusal) {
	g := model.Guard{Path: "rules/ts.yaml", Changes: []model.Guarded{{
		Entity: "ts.style.1", Field: "status", Before: "trial", After: "active", Command: "gragas rule promote ts.style.1",
	}}}
	r, err := model.NewRefusal(model.Refused, "rules/ts.yaml changes status by hand", nil, "gragas rule promote ts.style.1")
	if err != nil {
		panic(err)
	}
	return g, &r
}

func TestAnEditThatChangesNoLifecycleFieldEndsDone(t *testing.T) {
	t.Parallel()
	s := &session{guard: model.Guard{Path: "rules/ts.yaml"}}
	if code := guard.Edit(t.Context(), s); code != exit.Done || s.ending.Word != model.Done {
		t.Fatalf("code %d, ending %+v", code, s.ending)
	}
	if len(s.aside) != 0 {
		t.Fatalf("an allowed edit wrote %q on standard error", s.aside)
	}
	if !strings.Contains(s.ending.What, "rules/ts.yaml") {
		t.Fatalf("done does not name the file: %q", s.ending.What)
	}
}

func TestARefusalNamesTheFieldAndTheCommandOnScreenAndOnStandardError(t *testing.T) {
	t.Parallel()
	g, ref := status()
	s := &session{guard: g, ref: ref}
	if code := guard.Edit(t.Context(), s); code != exit.Refused || s.ending.Word != model.Refused {
		t.Fatalf("code %d, ending %+v", code, s.ending)
	}
	if s.ending.Next != "gragas rule promote ts.style.1" {
		t.Fatalf("the line to type is %q", s.ending.Next)
	}
	if len(s.ending.Lead) != 2 {
		t.Fatalf("the screen has %d blocks above the word, want the heading and the table", len(s.ending.Lead))
	}
	table, ok := s.ending.Lead[1].(model.Table)
	if !ok || len(table.Rows) != 1 || !slices.Contains(table.Rows[0], "status") || !slices.Contains(table.Rows[0], "gragas rule promote ts.style.1") {
		t.Fatalf("the table is %+v", s.ending.Lead[1])
	}
	want := []string{"refused: " + ref.Observed(), "instead: gragas rule promote ts.style.1"}
	if !slices.Equal(s.aside, want) {
		t.Fatalf("standard error has %q, want %q", s.aside, want)
	}
}

func TestUnderJSONARefusalIsOneDocumentAndStillGoesToStandardError(t *testing.T) {
	t.Parallel()
	g, ref := status()
	s := &session{json: true, guard: g, ref: ref}
	if code := guard.Edit(t.Context(), s); code != exit.Refused {
		t.Fatalf("code %d", code)
	}
	doc, ok := s.doc.(coreapi.GuardDocument)
	if !ok || doc.Word != string(model.Refused) || doc.Next != "gragas rule promote ts.style.1" || len(doc.Changes) != 1 {
		t.Fatalf("the document is %+v", s.doc)
	}
	if len(s.ending.Lead) != 0 {
		t.Fatal("under --json the screen draws a table")
	}
	if len(s.aside) != 2 {
		t.Fatalf("under --json standard error has %q", s.aside)
	}
}

func TestUnderJSONAnAllowedEditIsOneDocument(t *testing.T) {
	t.Parallel()
	s := &session{json: true, guard: model.Guard{Path: "rules/ts.yaml"}}
	guard.Edit(t.Context(), s)
	doc, ok := s.doc.(coreapi.GuardDocument)
	if !ok || doc.Word != string(model.Done) || doc.Why != "" || doc.Path != "rules/ts.yaml" {
		t.Fatalf("the document is %+v", s.doc)
	}
}

func TestAnErrorTheCoreCannotPlaceIsRefused(t *testing.T) {
	t.Parallel()
	s := &session{err: errors.New("the disk went away")}
	if code := guard.Edit(t.Context(), s); code != exit.Refused {
		t.Fatalf("code %d", code)
	}
}

func TestOutsideARulebookTheRunIsNotReady(t *testing.T) {
	t.Parallel()
	for _, err := range []error{coreapi.ErrNoBook, fmt.Errorf("two here: %w", coreapi.ErrTwoBooks), fmt.Errorf("%w: bad", coreapi.ErrNotReady)} {
		s := &session{err: err}
		if code := guard.Edit(t.Context(), s); code != exit.NotReady || s.ending.Word != model.NotReady {
			t.Fatalf("%v: code %d, ending %+v", err, code, s.ending)
		}
	}
	s := &session{err: coreapi.ErrNoBook}
	guard.Edit(t.Context(), s)
	if !strings.HasPrefix(s.ending.What, "not a rulebook") {
		t.Fatalf("no rulebook says %q", s.ending.What)
	}
}

func TestTheCommandRunsEditAndEndsWithItsCode(t *testing.T) {
	t.Parallel()
	g, ref := status()
	s := &exiting{session{guard: g, ref: ref}}
	c := guard.New(s)
	c.SetArgs([]string{"edit"})
	c.SilenceUsage, c.SilenceErrors = true, true
	err := c.ExecuteContext(context.Background())
	if code, ok := errors.AsType[exitCode](err); !ok || int(code) != exit.Refused {
		t.Fatalf("the command returned %v", err)
	}
}

func TestTheRowIsTheOneOfTheSpecTable(t *testing.T) {
	t.Parallel()
	r := guard.Row()
	if r.Name != "guard" || r.Order != 80 || r.State != kit.Built || r.Context != kit.InRulebook ||
		!slices.Equal(r.Then, []string{"edit", "staged"}) || len(r.NotYet) != 0 {
		t.Fatalf("the row is %+v", r)
	}
}

func TestAStagedChangeThatRecordsEveryFieldEndsDone(t *testing.T) {
	t.Parallel()
	s := &session{guard: model.Guard{Path: "history.jsonl"}}
	if code := guard.Staged(t.Context(), s, false); code != exit.Done || s.ending.Word != model.Done || !strings.Contains(s.ending.What, "history.jsonl") {
		t.Fatalf("code %d, ending %+v", code, s.ending)
	}
	s = &session{json: true, guard: model.Guard{Path: "history.jsonl"}}
	guard.Staged(t.Context(), s, false)
	if doc, ok := s.doc.(coreapi.GuardDocument); !ok || doc.Command != coreapi.GuardStagedCommand || doc.Word != string(model.Done) {
		t.Fatalf("the document is %+v", s.doc)
	}
}

func TestAStagedRefusalNamesItsTableAndGoesToStandardError(t *testing.T) {
	t.Parallel()
	g, ref := status()
	s := &session{guard: g, ref: ref}
	if code := guard.Staged(t.Context(), s, false); code != exit.Refused {
		t.Fatalf("code %d", code)
	}
	if h, ok := s.ending.Lead[0].(model.Heading); !ok || !strings.Contains(h.Text, "staged") {
		t.Fatalf("the table's heading is %+v", s.ending.Lead[0])
	}
	if len(s.aside) != 2 {
		t.Fatalf("standard error has %q", s.aside)
	}
	s = &session{json: true, guard: g, ref: ref}
	guard.Staged(t.Context(), s, false)
	if doc, ok := s.doc.(coreapi.GuardDocument); !ok || doc.Command != coreapi.GuardStagedCommand || doc.Word != string(model.Refused) {
		t.Fatalf("the document is %+v", s.doc)
	}
}

func TestAStagedChangeTheCoreCannotJudgeIsRefusedAndNoBookIsNotReady(t *testing.T) {
	t.Parallel()
	s := &session{err: errors.New("git went away")}
	if code := guard.Staged(t.Context(), s, false); code != exit.Refused {
		t.Fatalf("code %d", code)
	}
	s = &session{err: coreapi.ErrNoBook}
	if code := guard.Staged(t.Context(), s, false); code != exit.NotReady {
		t.Fatalf("code %d", code)
	}
}

func TestTheStagedCommandHandsOnMergeHook(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"staged"}, {"staged", "--merge-hook"}} {
		s := &exiting{session{guard: model.Guard{Path: "history.jsonl"}}}
		c := guard.New(s)
		c.SetArgs(args)
		c.SilenceUsage, c.SilenceErrors = true, true
		if err := c.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if want := len(args) == 2; s.mergeHook != want {
			t.Fatalf("%v: guard staged was asked mergeHook %v, want %v", args, s.mergeHook, want)
		}
	}
}

type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }

// exiting is a session whose Exit ends with the code, as the root's does.
type exiting struct{ session }

func (s *exiting) Exit(code int) error {
	if code == exit.Done {
		return nil
	}
	return exitCode(code)
}

func (s *session) CheckRulebook(context.Context) (model.RulebookCheck, *model.Refusal, error) {
	return model.RulebookCheck{}, nil, nil
}
