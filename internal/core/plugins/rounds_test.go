package plugins_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The plan rounds, each state and edge of a plan's run planted on the port's
// in-memory stand-in. The plugin is porttest's; the runner is the fake below,
// since running a step is host's and behind no port yet.

// The ways a runner refuses a step.
const (
	cannot    = "cannot"
	notReady  = "notready"
	ungranted = "ungranted"
)

// answer is what the stand-in says at one plan call, and what the runner does
// with that round's steps.
type answer struct {
	name  string
	err   error
	steps int
	final bool
	// bad is the step of this round the runner is to refuse, or -1, and how.
	bad int
	how string
}

func steps(n int, final bool) answer { return answer{name: "steps", steps: n, final: final, bad: -1} }

func fails(name string, err error) answer { return answer{name: name, err: err, bad: -1} }

// trace is one read: every call made on the plugin and the runner, in order,
// and the requests the plugin was asked with.
type trace struct {
	events []string
	plans  []plugin.PlanRequest
	reads  []plugin.EntriesRequest
	found  []plugin.FindingsRequest
}

func (t *trace) add(format string, a ...any) { t.events = append(t.events, fmt.Sprintf(format, a...)) }

func (t *trace) calls(prefix string) int {
	n := 0
	for _, e := range t.events {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

func (t *trace) String() string { return strings.Join(t.events, "; ") }

// place is the name a step carries so that the runner knows where it stands:
// a run's program, a read's glob.
func place(round, step int) string { return fmt.Sprintf("tool-%d-%d", round, step) }

// step is the step at a place: a run where the step is even, a read where it
// is odd, so both kinds pass through every row.
func step(round, i int) plugin.Step {
	if i%2 == 0 {
		return plugin.Step{Run: &plugin.Run{Program: place(round, i), Argv: []string{}}}
	}
	return plugin.Step{Read: &plugin.Read{Glob: place(round, i)}}
}

func kindAt(i int) plugin.Kind {
	if i%2 == 0 {
		return plugin.KindRun
	}
	return plugin.KindRead
}

// scripted is a plugin opened from the stand-in that answers its plan calls
// from script and its reading call with readErr, or with one entry.
func scripted(t *testing.T, tr *trace, script []answer, readErr error) port.Plugin {
	t.Helper()
	p := &porttest.StandinPlugin{
		OnPlan: func(in plugin.PlanRequest) (plugin.PlanResponse, error) {
			tr.add("plan %d", in.Round)
			tr.plans = append(tr.plans, in)
			if len(tr.plans) > len(script) {
				return plugin.PlanResponse{}, errors.New("the script holds no answer for this call")
			}
			a := script[len(tr.plans)-1]
			if a.err != nil {
				return plugin.PlanResponse{}, a.err
			}
			out := plugin.PlanResponse{Final: a.final}
			for i := range a.steps {
				out.Steps = append(out.Steps, step(in.Round, i))
			}
			return out, nil
		},
		OnEntries: func(in plugin.EntriesRequest) (plugin.EntriesResponse, error) {
			tr.add("read")
			tr.reads = append(tr.reads, in)
			if readErr != nil {
				return plugin.EntriesResponse{}, readErr
			}
			return plugin.EntriesResponse{Entries: []plugin.Entry{{Key: "a-rule", On: true}}}, nil
		},
		OnFindings: func(in plugin.FindingsRequest) (plugin.FindingsResponse, error) {
			tr.add("read")
			tr.found = append(tr.found, in)
			if readErr != nil {
				return plugin.FindingsResponse{}, readErr
			}
			return plugin.FindingsResponse{Ran: true}, nil
		},
	}
	return open(t, p)
}

func open(t *testing.T, p *porttest.StandinPlugin) port.Plugin {
	t.Helper()
	ref := model.PluginRef{Name: "standin", Version: "1.0.0"}
	var s porttest.Standin
	s.Install("root", ref, p)
	got, err := s.Open(t.Context(), "root", ref)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// runner is the host of a read: it grants and runs each step but the one the
// script says it refuses.
type runner struct {
	tr     *trace
	script []answer
}

func at(s plugin.Step) (round, i int) {
	name := ""
	switch {
	case s.Run != nil:
		name = s.Run.Program
	case s.Read != nil:
		name = s.Read.Glob
	}
	if _, err := fmt.Sscanf(name, "tool-%d-%d", &round, &i); err != nil {
		panic(err)
	}
	return round, i
}

func (r *runner) Granted(s plugin.Step) error {
	round, i := at(s)
	r.tr.add("granted %d.%d", round, i)
	if a := r.script[round]; a.bad == i && a.how == ungranted {
		return fmt.Errorf("%s: %w", place(round, i), port.ErrGrant)
	}
	return nil
}

func (r *runner) Run(_ context.Context, round, i int, _ plugin.Step) (plugin.Result, error) {
	r.tr.add("run %d.%d", round, i)
	if a := r.script[round]; a.bad == i {
		switch a.how {
		case cannot:
			return plugin.Result{}, errors.New("the step could not be run")
		case notReady:
			return plugin.Result{}, fmt.Errorf("no such tool on this machine: %w", port.ErrStepNotReady)
		}
	}
	// A runner that lies about where its result belongs and what it is: the
	// engine's stamp is the one that counts.
	lie := plugin.KindRun
	if kindAt(i) == plugin.KindRun {
		lie = plugin.KindRead
	}
	return plugin.Result{Round: 2, Step: 15, Kind: lie, Files: []plugin.File{}}, nil
}

// seed is the first plan's request as a caller hands it over, with a For, a
// Round and a Have that are not the caller's to write.
func seed() plugin.PlanRequest {
	return plugin.PlanRequest{
		ToolVersion: "1.2.3",
		Scratch:     new("scratch"),
		Config:      []plugin.Config{{Path: "lintel.config"}},
		Target:      plugin.TargetSource,
		Fixtures:    []string{"a.src"},
		For:         "neither",
		Round:       2,
		Have:        []plugin.Result{{Round: 1, Step: 9}},
	}
}

// read runs one scripted read of entries and answers its trace with what it
// gave.
func read(t *testing.T, script []answer, readErr error) (*trace, plugin.EntriesResponse, error) {
	t.Helper()
	tr := &trace{}
	got, err := plugins.Entries(t.Context(), scripted(t, tr, script, readErr), &runner{tr: tr, script: script}, seed())
	return tr, got, err
}

// ends holds that err is the end want, and matches the one sentinel of it.
func ends(t *testing.T, err error, want plugins.End) *plugins.EndError {
	t.Helper()
	e, ok := errors.AsType[*plugins.EndError](err)
	if !ok {
		t.Fatalf("the read answered %v, and no end", err)
	}
	if e.End != want {
		t.Fatalf("the read ended %d (%v), want %d", e.End, err, want)
	}
	for end, sentinel := range map[plugins.End]error{plugins.Refused: plugins.ErrRefused, plugins.Failed: plugins.ErrFailed, plugins.NotReady: plugins.ErrNotReady} {
		if errors.Is(err, sentinel) != (end == want) {
			t.Fatalf("errors.Is(%v, %v) is %v", err, sentinel, end != want)
		}
	}
	return e
}

func refused() error {
	return &plugin.CallError{Code: plugin.CodeRefused, Message: "the config loads code", Path: "/config/0"}
}

func bug() error { return &plugin.CallError{Code: plugin.CodeBug, Message: "its own fault"} }

// limitOf and answerAt read the typed cause of an end.
func limitOf(err error) plugin.Limit {
	if le, ok := errors.AsType[*plugin.LimitError](err); ok {
		return le.What
	}
	return ""
}

func answerAt(err error) string {
	if ae, ok := errors.AsType[*plugin.AnswerError](err); ok {
		return ae.Path
	}
	return "none"
}

// Row 1: the plugin's refused ends the read Refused, with the plugin's path.
func TestARefusedPlanEndsTheReadRefused(t *testing.T) {
	t.Parallel()
	tr, got, err := read(t, []answer{fails("refused", refused())}, nil)
	e := ends(t, err, plugins.Refused)
	if ce, ok := errors.AsType[*plugin.CallError](err); !ok || ce.Path != "/config/0" {
		t.Fatalf("the plugin's path is lost: %v", err)
	}
	if e.Round != 0 || e.Step != -1 {
		t.Fatalf("it ended at round %d, step %d", e.Round, e.Step)
	}
	if len(got.Entries) != 0 || tr.String() != "plan 0" {
		t.Fatalf("entries %v after %s", got.Entries, tr)
	}
}

// Row 2: any other error of a plan call ends the read Failed, and the cause
// is kept.
func TestAnyOtherPlanErrorEndsTheReadFailed(t *testing.T) {
	t.Parallel()
	for name, cause := range map[string]error{
		"bug":         bug(),
		"unsupported": &plugin.CallError{Code: plugin.CodeUnsupported, Message: "not this target"},
		"unreadable":  &plugin.CallError{Code: plugin.CodeUnreadable, Message: "not its output", Path: "/have/0/stdout"},
		"over time":   &plugin.LimitError{What: plugin.LimitTime},
		"bad answer":  &plugin.AnswerError{Path: "/steps/0", Why: "no kind"},
		"a trap":      fmt.Errorf("the module left with 3: %w", port.ErrTrap),
		"plain":       errors.New("an error of no kind"),
	} {
		tr, got, err := read(t, []answer{fails(name, cause)}, nil)
		ends(t, err, plugins.Failed)
		if !errors.Is(err, cause) {
			t.Errorf("%s: the cause is lost: %v", name, err)
		}
		if len(got.Entries) != 0 || tr.String() != "plan 0" {
			t.Errorf("%s: entries %v after %s", name, got.Entries, tr)
		}
	}
}

// Row 3: no steps and not final is a bad answer.
func TestNoStepsAndNotFinalIsABadAnswer(t *testing.T) {
	t.Parallel()
	tr, _, err := read(t, []answer{steps(0, false)}, nil)
	ends(t, err, plugins.Failed)
	if answerAt(err) != "/final" || tr.String() != "plan 0" {
		t.Fatalf("%v after %s", err, tr)
	}
}

// Row 4: more than sixteen steps in a round is refused, and none of them
// runs or is looked at.
func TestMoreThanSixteenStepsIsRefusedAndNoneRuns(t *testing.T) {
	t.Parallel()
	for _, final := range []bool{true, false} {
		tr, _, err := read(t, []answer{steps(plugins.RoundSteps+1, final)}, nil)
		ends(t, err, plugins.Failed)
		if answerAt(err) != "/steps" || tr.String() != "plan 0" {
			t.Fatalf("final %v: %v after %s", final, err, tr)
		}
	}
	// The control: sixteen is a round.
	if tr, _, err := read(t, []answer{steps(plugins.RoundSteps, true)}, nil); err != nil || tr.calls("run") != plugins.RoundSteps {
		t.Fatalf("sixteen steps: %v after %s", err, tr)
	}
}

// Row 5: final with no steps goes straight to the read.
func TestFinalWithNoStepsGoesStraightToTheRead(t *testing.T) {
	t.Parallel()
	tr, got, err := read(t, []answer{steps(0, true)}, nil)
	if err != nil || len(got.Entries) != 1 {
		t.Fatalf("entries %v, %v", got.Entries, err)
	}
	if tr.String() != "plan 0; read" {
		t.Fatalf("the calls were %s", tr)
	}
}

// Row 6: not final at round 2 is over the rounds limit, at the plan's answer:
// none of that round's steps is looked at or run.
func TestNotFinalAtRoundTwoIsOverTheRoundsLimit(t *testing.T) {
	t.Parallel()
	tr, _, err := read(t, []answer{steps(1, false), steps(1, false), steps(3, false)}, nil)
	e := ends(t, err, plugins.Failed)
	if limitOf(err) != plugin.LimitRounds || e.Round != 2 || e.Step != -1 {
		t.Fatalf("%v", err)
	}
	if want := "plan 0; granted 0.0; run 0.0; plan 1; granted 1.0; run 1.0; plan 2"; tr.String() != want {
		t.Fatalf("the calls were %s, want %s", tr, want)
	}
	// The control: final at round 2 is a read.
	if _, _, err := read(t, []answer{steps(1, false), steps(1, false), steps(3, true)}, nil); err != nil {
		t.Fatalf("final at round 2: %v", err)
	}
}

// Row 7: a step whose program is not granted ends the read Failed, and the
// round starts nothing: every grant is asked before the first step runs.
func TestAnUngrantedStepEndsTheReadBeforeTheRoundStartsAnything(t *testing.T) {
	t.Parallel()
	a := steps(4, true)
	a.bad, a.how = 3, ungranted
	tr, _, err := read(t, []answer{a}, nil)
	e := ends(t, err, plugins.Failed)
	if !errors.Is(err, port.ErrGrant) || e.Step != 3 {
		t.Fatalf("%v", err)
	}
	if want := "plan 0; granted 0.0; granted 0.1; granted 0.2; granted 0.3"; tr.String() != want {
		t.Fatalf("a round with an ungranted step started something: %s", tr)
	}
	// In a later round too: what the earlier rounds ran stays run, and the
	// round with the step starts nothing.
	tr, _, err = read(t, []answer{steps(1, false), a}, nil)
	if e := ends(t, err, plugins.Failed); e.Round != 1 || tr.calls("run 1.") != 0 || tr.calls("read") != 0 {
		t.Fatalf("%v after %s", err, tr)
	}
	// The control: with every step granted, every grant is still asked first.
	tr, _, err = read(t, []answer{steps(2, true)}, nil)
	if want := "plan 0; granted 0.0; granted 0.1; run 0.0; run 0.1; read"; err != nil || tr.String() != want {
		t.Fatalf("%v after %s, want %s", err, tr, want)
	}
}

// Rows 8 and 9: a step that cannot run ends the read Failed, one whose tool
// or level is missing ends it NotReady, and no later step runs.
func TestAStepThatCannotRunEndsTheRead(t *testing.T) {
	t.Parallel()
	for how, want := range map[string]plugins.End{cannot: plugins.Failed, notReady: plugins.NotReady} {
		a := steps(4, true)
		a.bad, a.how = 1, how
		tr, got, err := read(t, []answer{a}, nil)
		e := ends(t, err, want)
		if e.Step != 1 || e.Round != 0 || len(got.Entries) != 0 {
			t.Fatalf("%s: round %d, step %d, entries %v", how, e.Round, e.Step, got.Entries)
		}
		if tr.calls("run") != 2 || tr.calls("read") != 0 {
			t.Fatalf("%s: the calls were %s", how, tr)
		}
	}
}

// Rows 10, 11 and 13: each plan is asked with everything gathered so far, and
// the reading call is made once, with all of it, each result at the place and
// of the kind the engine gave it. What the caller wrote of the round, the have
// and the for is not what the plugin is asked.
func TestTheReadingCallIsMadeOnceWithAllThatWasGathered(t *testing.T) {
	t.Parallel()
	tr, got, err := read(t, []answer{steps(2, false), steps(3, false), steps(1, true)}, nil)
	if err != nil || len(got.Entries) != 1 {
		t.Fatalf("entries %v, %v", got.Entries, err)
	}
	for i, want := range []int{0, 2, 5} {
		if p := tr.plans[i]; p.Round != i || len(p.Have) != want || p.For != plugin.ForConfig {
			t.Fatalf("plan %d was asked round %d for %q with %d results, want %d", i, p.Round, p.For, len(p.Have), want)
		}
	}
	if len(tr.reads) != 1 || tr.events[len(tr.events)-1] != "read" {
		t.Fatalf("the calls were %s", tr)
	}
	in := tr.reads[0]
	var where []string
	for _, r := range in.Have {
		if r.Kind != kindAt(r.Step) {
			t.Fatalf("the result of step %d kept the runner's kind %q", r.Step, r.Kind)
		}
		where = append(where, fmt.Sprintf("%d.%d", r.Round, r.Step))
	}
	if want := "0.0 0.1 1.0 1.1 1.2 2.0"; strings.Join(where, " ") != want {
		t.Fatalf("the results stand at %v, want %s", where, want)
	}
	// The reading call is asked with what the plans were, and nothing else.
	s := seed()
	if in.ToolVersion != s.ToolVersion || in.Scratch == nil || *in.Scratch != *s.Scratch || len(in.Config) != 1 || in.Config[0].Path != s.Config[0].Path || in.Target != s.Target {
		t.Fatalf("the reading call was asked with %+v", in)
	}
}

// Row 12: have over the request cap is over the limit. The cap is the
// codec's, so the port says it, and the read ends Failed at the call the
// request would have gone in: no later step, and no reading call.
func TestHaveOverTheRequestCapIsOverTheLimit(t *testing.T) {
	t.Parallel()
	over := &plugin.LimitError{What: plugin.LimitRequest}
	// At a plan call.
	tr, _, err := read(t, []answer{steps(5, false), fails("over", over), steps(1, true)}, nil)
	e := ends(t, err, plugins.Failed)
	if limitOf(err) != plugin.LimitRequest || e.Round != 1 || e.Step != -1 {
		t.Fatalf("%v", err)
	}
	if tr.calls("plan") != 2 || tr.calls("run") != 5 || tr.calls("read") != 0 {
		t.Fatalf("the calls were %s", tr)
	}
	// At the reading call.
	tr, got, err := read(t, []answer{steps(5, true)}, over)
	ends(t, err, plugins.Failed)
	if limitOf(err) != plugin.LimitRequest || len(got.Entries) != 0 || tr.calls("read") != 1 {
		t.Fatalf("%v after %s", err, tr)
	}
}

// Row 14: any error of the reading call ends the read Failed, refused among
// them, with the plugin's error and path kept.
func TestAnErrorOfTheReadingCallEndsTheReadFailed(t *testing.T) {
	t.Parallel()
	for name, cause := range map[string]error{"refused": refused(), "bug": bug(), "plain": errors.New("an error of no kind")} {
		tr, got, err := read(t, []answer{steps(1, false), steps(1, true)}, cause)
		e := ends(t, err, plugins.Failed)
		if !errors.Is(err, cause) || len(got.Entries) != 0 || tr.calls("read") != 1 {
			t.Errorf("%s: %v, entries %v, after %s", name, err, got.Entries, tr)
		}
		// It ended at a call, after the round of the last plan, and at no step.
		if e.Round != 1 || e.Step != -1 {
			t.Errorf("%s: it ended at round %d, step %d", name, e.Round, e.Step)
		}
	}
	_, _, err := read(t, []answer{steps(0, true)}, refused())
	if ce, ok := errors.AsType[*plugin.CallError](err); !ok || ce.Path != "/config/0" || ce.Code != plugin.CodeRefused {
		t.Fatalf("the plugin's code and path are lost: %v", err)
	}
}

// Row 15: a context that is done ends the read Failed before the next call.
func TestADoneContextEndsTheReadFailed(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tr := &trace{}
	script := []answer{steps(1, true)}
	_, err := plugins.Entries(ctx, scripted(t, tr, script, nil), &runner{tr: tr, script: script}, seed())
	ends(t, err, plugins.Failed)
	if !errors.Is(err, context.Canceled) || len(tr.events) != 0 {
		t.Fatalf("%v after %s", err, tr)
	}
}

// stopping is a runner that ends the read's context as it runs one step.
type stopping struct {
	runner
	stop func()
	at   int
}

func (s *stopping) Run(ctx context.Context, round, i int, st plugin.Step) (plugin.Result, error) {
	if i == s.at {
		s.stop()
	}
	return s.runner.Run(ctx, round, i, st)
}

// A context that is done between two steps ends the read before the next.
func TestAContextDoneBetweenStepsEndsTheReadBeforeTheNext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tr := &trace{}
	script := []answer{steps(4, true)}
	host := &stopping{tr: tr, script: script, stop: cancel, at: 1}
	_, err := plugins.Entries(ctx, scripted(t, tr, script, nil), host, seed())
	e := ends(t, err, plugins.Failed)
	if !errors.Is(err, context.Canceled) || e.Step != 2 || tr.calls("run") != 2 || tr.calls("read") != 0 {
		t.Fatalf("%v after %s", err, tr)
	}
}

// Row 15 at the reading call: a context done as the last step of a final
// round runs ends the read before the reading call is made.
func TestAContextDoneAtTheLastStepEndsTheReadBeforeTheReadingCall(t *testing.T) {
	t.Parallel()
	for _, read := range []string{"entries", "findings"} {
		ctx, cancel := context.WithCancel(t.Context())
		tr := &trace{}
		script := []answer{steps(2, false), steps(3, true)}
		host := &stopping{tr: tr, script: script, stop: cancel, at: 2}
		var err error
		if read == "entries" {
			_, err = plugins.Entries(ctx, scripted(t, tr, script, nil), host, seed())
		} else {
			_, err = plugins.Findings(ctx, scripted(t, tr, script, nil), host, seed())
		}
		cancel()
		e := ends(t, err, plugins.Failed)
		if !errors.Is(err, context.Canceled) || e.Round != 1 || e.Step != -1 || tr.calls("run") != 5 || tr.calls("read") != 0 {
			t.Fatalf("%s: %v at round %d, step %d, after %s", read, err, e.Round, e.Step, tr)
		}
	}
}

// The plugin's own text in an end reaches the terminal only through Shown: a
// refusal whose message and path hold a terminal escape, a C1 control and a
// bidi override prints none of them, while errors.As still finds the cause
// whole.
func TestAnEndPrintsThePluginsTextShown(t *testing.T) {
	t.Parallel()
	const paint = "\x1b]0;owned\x07\x1b[2J\u009b31m\u202efdp.txt"
	cause := &plugin.CallError{Code: plugin.CodeRefused, Message: "no" + paint, Path: "/config" + paint}
	_, _, err := read(t, []answer{fails("refused", cause)}, nil)
	ends(t, err, plugins.Refused)
	got := err.Error()
	for _, r := range []rune{0x1b, 0x07, 0x9b, 0x202e} {
		if strings.ContainsRune(got, r) {
			t.Fatalf("the end prints %U raw: %q", r, got)
		}
	}
	if !strings.Contains(got, plugin.Shown(cause.Message)) || !strings.Contains(got, plugin.Shown(cause.Path)) {
		t.Fatalf("the end does not say the cause shown: %q", got)
	}
	if ce, ok := errors.AsType[*plugin.CallError](err); !ok || ce.Message != cause.Message || ce.Path != cause.Path {
		t.Fatalf("the raw cause is lost: %v", err)
	}
	if got := (&plugins.EndError{End: plugins.Failed}).Error(); strings.Contains(got, "nil") {
		t.Fatalf("an end with no cause says %q", got)
	}
}

// A step that is neither a run nor a read, or both, is a value no schema
// read: a bad answer, and none of the round's steps is looked at or run.
func TestAStepThatIsNoOneKindIsABadAnswer(t *testing.T) {
	t.Parallel()
	for name, odd := range map[string]plugin.Step{
		"neither": {},
		"both":    {Run: &plugin.Run{Program: place(0, 1), Argv: []string{}}, Read: &plugin.Read{Glob: place(0, 1)}},
	} {
		tr := &trace{}
		p := open(t, &porttest.StandinPlugin{OnPlan: func(plugin.PlanRequest) (plugin.PlanResponse, error) {
			tr.add("plan")
			return plugin.PlanResponse{Steps: []plugin.Step{step(0, 0), odd}, Final: true}, nil
		}})
		_, err := plugins.Entries(t.Context(), p, &runner{tr: tr, script: []answer{steps(2, true)}}, seed())
		ends(t, err, plugins.Failed)
		if answerAt(err) != "/steps/1" || tr.String() != "plan" {
			t.Errorf("%s: %v after %s", name, err, tr)
		}
	}
}

// A cause that carries one of the three ends cannot say the read ended
// another way: a plan call whose error wraps ErrNotReady or ErrRefused is
// still Failed, and a runner's error that wraps ErrRefused is too.
func TestACauseCannotSayHowTheReadEnded(t *testing.T) {
	t.Parallel()
	for _, lie := range []error{plugins.ErrNotReady, plugins.ErrRefused} {
		_, _, err := read(t, []answer{fails("liar", fmt.Errorf("a plugin's host said: %w", lie))}, nil)
		ends(t, err, plugins.Failed)
		_, _, err = read(t, []answer{steps(0, true)}, fmt.Errorf("a plugin's host said: %w", lie))
		ends(t, err, plugins.Failed)
	}
	// An end nobody set is a failure.
	if e := (&plugins.EndError{}); !errors.Is(e, plugins.ErrFailed) || errors.Is(e, plugins.ErrRefused) || errors.Is(e, plugins.ErrNotReady) {
		t.Fatalf("the zero end is %v", e)
	}
}

// The findings read takes the same rounds, asked for "run", and its reading
// call carries the fixtures.
func TestTheFindingsReadTakesTheSameRounds(t *testing.T) {
	t.Parallel()
	tr := &trace{}
	script := []answer{steps(2, false), steps(0, true)}
	got, err := plugins.Findings(t.Context(), scripted(t, tr, script, nil), &runner{tr: tr, script: script}, seed())
	if err != nil || !got.Ran {
		t.Fatalf("findings %v, %v", got, err)
	}
	if tr.plans[0].For != plugin.ForRun || tr.plans[0].Round != 0 || len(tr.plans[0].Have) != 0 {
		t.Fatalf("the first plan was asked %+v", tr.plans[0])
	}
	if len(tr.found) != 1 || len(tr.found[0].Have) != 2 || len(tr.found[0].Fixtures) != 1 || tr.found[0].ToolVersion != "1.2.3" {
		t.Fatalf("the reading call was asked %+v", tr.found)
	}
	script = []answer{fails("refused", refused())}
	tr = &trace{}
	_, err = plugins.Findings(t.Context(), scripted(t, tr, script, nil), &runner{tr: tr, script: script}, seed())
	ends(t, err, plugins.Refused)
	script = []answer{steps(0, true)}
	tr = &trace{}
	_, err = plugins.Findings(t.Context(), scripted(t, tr, script, bug()), &runner{tr: tr, script: script}, seed())
	ends(t, err, plugins.Failed)
}

// The proof that a read ends, as a test: every script over the alphabet of
// what a plugin and a runner can answer, to the depth of three rounds. Each
// returns, within three plan calls, forty-eight steps and one reading call;
// the rounds left fall by one at each pass back to the plan and never below
// none; the reading call is the last; and the end is the one the last answer
// asks for.
func TestEveryReadEnds(t *testing.T) {
	t.Parallel()
	alphabet := []answer{
		fails("refused", refused()),
		fails("bug", bug()),
		fails("host", &plugin.LimitError{What: plugin.LimitMemory}),
		steps(0, false),
		steps(0, true),
		steps(plugins.RoundSteps+1, false),
		steps(plugins.RoundSteps+1, true),
	}
	for _, n := range []int{1, 2, plugins.RoundSteps} {
		for _, final := range []bool{true, false} {
			alphabet = append(alphabet, steps(n, final))
			for _, how := range []string{cannot, notReady, ungranted} {
				for _, bad := range []int{0, n - 1} {
					a := steps(n, final)
					a.name, a.bad, a.how = how, bad, how
					alphabet = append(alphabet, a)
				}
			}
		}
	}
	reads := map[string]error{"read": nil, "read fails": bug(), "read refused": refused()}
	walked := 0
	var walk func(script []answer)
	walk = func(script []answer) {
		if len(script) == plugins.PlanCalls {
			for readName, readErr := range reads {
				walked++
				checkOne(t, script, readName, readErr)
			}
			return
		}
		for _, a := range alphabet {
			walk(append(append([]answer{}, script...), a))
		}
	}
	walk(nil)
	if want := len(alphabet) * len(alphabet) * len(alphabet) * len(reads); walked != want {
		t.Fatalf("walked %d scripts, want %d", walked, want)
	}
}

func checkOne(t *testing.T, script []answer, readName string, readErr error) {
	t.Helper()
	tr, got, err := read(t, script, readErr)
	name := func() string { return scriptName(script, readName) }
	if p, s, r := tr.calls("plan"), tr.calls("run"), tr.calls("read"); p > plugins.PlanCalls || s > plugins.PlanCalls*plugins.RoundSteps || r > 1 {
		t.Fatalf("%s: %d plan calls, %d steps, %d reading calls", name(), p, s, r)
	}
	// The measure: the rounds left at each plan call fall by one, from two,
	// and never below none.
	for i, p := range tr.plans {
		if left := plugins.PlanCalls - 1 - p.Round; left != plugins.PlanCalls-1-i || left < 0 {
			t.Fatalf("%s: plan call %d was made with %d rounds left", name(), i, left)
		}
		if len(p.Have) != ranBefore(tr, i) {
			t.Fatalf("%s: plan call %d was asked with %d results of %d", name(), i, len(p.Have), ranBefore(tr, i))
		}
	}
	if tr.calls("read") == 1 && tr.events[len(tr.events)-1] != "read" {
		t.Fatalf("%s: a call followed the reading call: %s", name(), tr)
	}
	want, done := wanted(script, readErr)
	if done {
		if err != nil || len(got.Entries) != 1 || tr.calls("read") != 1 {
			t.Fatalf("%s: want the entries, got %v, %v", name(), got.Entries, err)
		}
		return
	}
	if len(got.Entries) != 0 || err == nil {
		t.Fatalf("%s: want an end, got entries %v and %v", name(), got.Entries, err)
	}
	if e, ok := errors.AsType[*plugins.EndError](err); !ok || e.End != want {
		t.Fatalf("%s: ended %v, want end %d", name(), err, want)
	}
}

// scriptName names one script of the walk, for a failure.
func scriptName(script []answer, readName string) string {
	var s []string
	for _, a := range script {
		s = append(s, fmt.Sprintf("%s(%d,%v,%d)", a.name, a.steps, a.final, a.bad))
	}
	return strings.Join(s, " > ") + " > " + readName
}

// wanted is the end the table asks of a script, worked out from the script
// alone: whether the read finishes, and the end where it does not.
func wanted(script []answer, readErr error) (plugins.End, bool) {
	for round, a := range script {
		switch {
		case a.err != nil:
			if a.name == "refused" {
				return plugins.Refused, false
			}
			return plugins.Failed, false
		case a.steps > plugins.RoundSteps, a.steps == 0 && !a.final, !a.final && round == plugins.PlanCalls-1:
			return plugins.Failed, false
		case a.bad >= 0:
			if a.how == notReady {
				return plugins.NotReady, false
			}
			return plugins.Failed, false
		case a.final:
			return plugins.Failed, readErr == nil
		}
	}
	return plugins.Failed, false
}

// ranBefore counts the steps run before plan call i.
func ranBefore(tr *trace, i int) int {
	n, seen := 0, 0
	for _, e := range tr.events {
		if strings.HasPrefix(e, "plan") {
			if seen == i {
				return n
			}
			seen++
		}
		if strings.HasPrefix(e, "run") {
			n++
		}
	}
	return n
}

// Judge is the judgement the port names: host's opener is made with one.
func TestJudgeIsThePortsManifestJudge(t *testing.T) {
	t.Parallel()
	var judge port.ManifestJudge = plugins.Judge
	if err := judge(good()); err != nil {
		t.Fatalf("the good manifest is refused: %v", err)
	}
	m := good()
	m.Tool = "another"
	if _, ok := errors.AsType[*plugins.ManifestError](judge(m)); !ok {
		t.Fatalf("a manifest with no grant for its tool is taken")
	}
}
