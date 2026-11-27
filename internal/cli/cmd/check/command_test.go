package check_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/birdtable/gragas/internal/cli/cmd/check"
	"github.com/birdtable/gragas/internal/cli/kit"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// TestMain starts from the shared test setup.
func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

// A session that is only the kit's offers no old check.
type bare struct{ kit.Session }

// An old session hands over the command it holds.
type old struct {
	bare
	made *cobra.Command
}

func (o old) OldCheck() *cobra.Command { return o.made }

func TestTheCommandIsTheOneTheSessionHolds(t *testing.T) {
	t.Parallel()
	want := &cobra.Command{Use: "check"}
	if got := check.New(old{made: want}); got != want {
		t.Fatalf("New made %p, the session holds %p", got, want)
	}
}

func TestASessionWithNoOldCheckIsRefusedLoudly(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("New on a session with no old check did not panic")
		}
	}()
	check.New(bare{})
}

func TestTheRowIsTheOneOfTheSpecTable(t *testing.T) {
	t.Parallel()
	r := check.Row()
	if r.Name != "check" || r.Order != 410 || r.State != kit.Legacy || !r.Menu.Offered {
		t.Fatalf("the row is %+v", r)
	}
}

// inBook is a session in a folder whose check is the rulebook's: it answers
// CheckRulebook with what the test set, and keeps how the run ended.
type inBook struct {
	bare
	made    *cobra.Command
	oldRan  *bool
	asked   *bool
	json    bool
	res     model.RulebookCheck
	ref     *model.Refusal
	err     error
	ended   *kit.Ending
	written *any
	// the project check: where the run is, what it answers, and whether it was asked
	here, pinned bool
	project      model.ProjectCheck
	projectRef   *model.Refusal
	projectErr   error
	projectAsked *bool
}

func (s inBook) OldCheck() *cobra.Command                    { return s.made }
func (s inBook) JSON() bool                                  { return s.json }
func (s inBook) Document(v any)                              { *s.written = v }
func (s inBook) Exit(int) error                              { return nil }
func (s inBook) End(e kit.Ending) int                        { *s.ended = e; return 0 }
func (s inBook) HoldsRulebook(context.Context) (bool, error) { return s.here, nil }
func (s inBook) HoldsPin(context.Context) (bool, error)      { return s.pinned, nil }
func (s inBook) CheckProject(context.Context) (model.ProjectCheck, *model.Refusal, error) {
	*s.projectAsked = true
	return s.project, s.projectRef, s.projectErr
}
func (s inBook) CheckRulebook(context.Context) (model.RulebookCheck, *model.Refusal, error) {
	*s.asked = true
	return s.res, s.ref, s.err
}

// newInBook is an inBook whose old check has the -C flag and says it ran.
func newInBook() inBook {
	s := inBook{oldRan: new(bool), asked: new(bool), ended: &kit.Ending{}, written: new(any), projectAsked: new(bool)}
	var dir string
	s.made = &cobra.Command{Use: "check", RunE: func(*cobra.Command, []string) error { *s.oldRan = true; return nil }}
	s.made.Flags().StringVarP(&dir, "dir", "C", ".", "")
	return s
}

// runCheck runs check over s with args.
func runCheck(t *testing.T, s inBook, args ...string) {
	t.Helper()
	c := check.New(s)
	c.SetArgs(append([]string{}, args...)) // never nil: cobra reads os.Args for a nil list, the test binary's flags
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	if err := c.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// TestAFolderWithNoOneRulebookIsTheOldCheck: no rulebook.yaml, or more than
// one, as gragas's own repository holds, is the old check's; -C is the old
// check's flag, and the rulebook's is never asked.
func TestAFolderWithNoOneRulebookIsTheOldCheck(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{"none": coreapi.ErrNoBook, "two": coreapi.ErrTwoBooks} {
		s := newInBook()
		s.err = err
		runCheck(t, s)
		if !*s.oldRan || !*s.asked {
			t.Errorf("%s: old ran %t, rulebook asked %t; want both", name, *s.oldRan, *s.asked)
		}
	}
	s := newInBook()
	runCheck(t, s, "-C", "../other")
	if !*s.oldRan || *s.asked {
		t.Errorf("-C: old ran %t, rulebook asked %t; want only the old", *s.oldRan, *s.asked)
	}
}

func TestAGateFindingEndsOnFindingsAndIsListedWithWhatIsNotAvailable(t *testing.T) {
	t.Parallel()
	s := newInBook()
	s.res = model.RulebookCheck{BaseRef: "origin/main", Commits: 1,
		Gate:        []model.GateFinding{{Kind: model.GateUnrecorded, Entity: "rule:go.errors.1", Msg: "commit 2222 makes status active", Fix: "gragas rule promote go.errors.1"}},
		Unavailable: []model.Unavailable{{Name: "proof marks", Reason: "check takes them in a later release"}}}
	runCheck(t, s)
	e := *s.ended
	if *s.oldRan || e.Word != model.Findings || !strings.Contains(e.What, "1 gate finding") || !strings.Contains(e.What, "origin/main") {
		t.Fatalf("ended %+v, old ran %t; want findings naming the gate finding and the base ref", e, *s.oldRan)
	}
	text := fmt.Sprint(e.Lead)
	for _, want := range []string{"Gate", string(model.GateUnrecorded), "commit 2222", "Fix: gragas rule promote", "Not available yet", "a later release"} {
		if !strings.Contains(text, want) {
			t.Errorf("the listing does not say %q: %s", want, text)
		}
	}
}

func TestACleanRulebookEndsDone(t *testing.T) {
	t.Parallel()
	s := newInBook()
	s.res = model.RulebookCheck{BaseRef: "origin/main", Validation: model.Validation{Rules: 3}}
	runCheck(t, s)
	if e := *s.ended; e.Word != model.Done || !strings.HasPrefix(e.What, "0 findings, 0 gate findings") {
		t.Errorf("ended %+v; want done", e)
	}
}

func TestARefusalEndsOnItsWordWithItsLine(t *testing.T) {
	t.Parallel()
	s := newInBook()
	ref, err := model.NewRefusal(model.NotReady, "no base ref is named", nil, "base-ref: origin/main")
	if err != nil {
		t.Fatal(err)
	}
	s.ref = &ref
	runCheck(t, s)
	if e := *s.ended; e.Word != model.NotReady || e.What != "no base ref is named" || e.Next != ref.Hint() {
		t.Errorf("ended %+v; want the refusal's word, observation and line", e)
	}
}

func TestUnderJSONTheDocumentIsChecks(t *testing.T) {
	t.Parallel()
	s := newInBook()
	s.json = true
	s.res = model.RulebookCheck{BaseRef: "origin/main", Base: "1111", Gate: []model.GateFinding{{Kind: model.GatePin}}}
	runCheck(t, s)
	doc, ok := (*s.written).(coreapi.Check)
	if !ok || doc.Command != "check" || doc.Word != "findings" || doc.Base != "1111" || len(doc.Gate) != 1 {
		t.Errorf("the document %+v; want check's, on findings", *s.written)
	}
	if e := *s.ended; e.Word != model.Findings || len(e.Lead) != 0 {
		t.Errorf("ended %+v; want findings, and nothing drawn", e)
	}
}
