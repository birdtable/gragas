package plugins

// The plan rounds. A read asks the plugin for
// a plan, has host run the plan's steps, and asks again with what the steps
// gave, at most three times; then it makes one reading call with everything
// gathered. The plugin decides which steps are asked for and never runs one.
// Every way a read can go wrong ends it with an error: none is an empty list
// of rules.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// The limits of one read. The wire schema holds the same numbers: a round is 0
// to 2, a step 0 to 15, and have holds at most 48 results.
const (
	// PlanCalls is how many times a read may ask for a plan.
	PlanCalls = 3
	// RoundSteps is how many steps one plan may hold.
	RoundSteps = 16
)

// End is how a read that did not finish ended. The zero End is none of them.
type End int

// The three ends of a read that did not finish.
const (
	// Refused is the plugin saying it will not handle this input, and where.
	Refused End = iota + 1
	// Failed is an error of the plugin's or of host's, an answer that does
	// not fit, a step that could not run, or a limit.
	Failed
	// NotReady is a tool or a sandbox level a step needs that is missing on
	// this machine.
	NotReady
)

// The three ends, for errors.Is. An *EndError matches exactly one, by its End
// and by nothing in its cause.
var (
	// ErrRefused is what a read's error matches when the plugin refused the
	// input at a plan call.
	ErrRefused = errors.New("the plugin refused the input")
	// ErrFailed is what a read's error matches for every end that is neither
	// of the other two.
	ErrFailed = errors.New("the read failed")
	// ErrNotReady is what a read's error matches when a tool or a sandbox
	// level is missing here: a Runner says so with port.ErrStepNotReady.
	ErrNotReady = errors.New("a tool or a sandbox level is missing here")
)

// atCall is the Step of an EndError that ended at a call and at no step.
const atCall = -1

// EndError is the error of every read that did not finish.
type EndError struct {
	// End is which of the three ends it is.
	End End
	// Round is the round the read ended in, 0 to 2.
	Round int
	// Step is the step it ended at, or -1 when it ended at a call.
	Step int
	// Cause is what ended it: the plugin's error, host's, or the engine's own.
	Cause error
}

// Error says the end, where the read stood, and the cause. A plugin's own
// text in the cause is escaped where it starts, by plugin.CallError's Error,
// so it is never escaped twice here; a screen that lays the parts out itself
// finds the raw cause by errors.As.
func (e *EndError) Error() string {
	at := fmt.Sprintf("round %d", e.Round)
	if e.Step >= 0 {
		at += fmt.Sprintf(", step %d", e.Step)
	}
	cause := "no cause"
	if e.Cause != nil {
		cause = e.Cause.Error()
	}
	return fmt.Sprintf("%s (%s): %s", e.sentinel(), at, cause)
}

func (e *EndError) sentinel() error {
	switch e.End {
	case Refused:
		return ErrRefused
	case NotReady:
		return ErrNotReady
	case Failed:
		return ErrFailed
	default:
		// An End nobody set is a failure, never a finish.
		return ErrFailed
	}
}

// Is matches the one sentinel of e's End.
func (e *EndError) Is(target error) bool { return target == e.sentinel() }

// Unwrap answers the cause with the three ends hidden in it, so that a cause
// which happens to carry one cannot say the read ended another way.
func (e *EndError) Unwrap() error {
	if e.Cause == nil {
		return nil
	}
	return sealed{e.Cause}
}

// sealed is a cause that answers errors.Is and errors.As for everything in
// its chain but the three ends.
type sealed struct{ err error }

func (s sealed) Error() string { return s.err.Error() }

func (s sealed) Is(target error) bool {
	if target == ErrRefused || target == ErrFailed || target == ErrNotReady {
		return false
	}
	return errors.Is(s.err, target)
}

func (s sealed) As(target any) bool { return errors.As(s.err, target) }

// Runner is what a read needs of host, bound to one read: one manifest, one
// root and one scratch folder. The context declares it on its own side, and
// core/api binds it to host's step runner.
type Runner interface {
	// Granted answers nil when the step's program is one of the manifest's
	// grants, and an error matching port.ErrGrant when it is not. It looks
	// for nothing and starts nothing.
	Granted(s plugin.Step) error
	// Run runs one step and answers its result. An error matching
	// port.ErrStepNotReady says a tool or a sandbox level is missing on this
	// machine; any other error says the step could not run.
	Run(ctx context.Context, round, step int, s plugin.Step) (plugin.Result, error)
}

// Entries reads the entries of a config: the rounds with for "config", then
// one entries call with everything gathered. in is the first plan's request:
// its For, Round and Have are the engine's to write, whatever in holds there.
func Entries(ctx context.Context, p port.Plugin, r Runner, in plugin.PlanRequest) (plugin.EntriesResponse, error) {
	in.For = plugin.ForConfig
	return read(ctx, p, r, in, p.Entries, entriesRequest)
}

// Findings reads the findings of a run over fixtures: the rounds with for
// "run", then one findings call with everything gathered. in is the first
// plan's request, as Entries takes it.
func Findings(ctx context.Context, p port.Plugin, r Runner, in plugin.PlanRequest) (plugin.FindingsResponse, error) {
	in.For = plugin.ForRun
	return read(ctx, p, r, in, p.Findings, findingsRequest)
}

// entriesRequest is the entries call of a read: what the plans were asked
// with, and everything they gathered. The files a read returned are left out of
// its Config, so a file counts once against the entries cap (gragas#2375); a
// read that returned none leaves the whole config.
func entriesRequest(in plugin.PlanRequest, have []plugin.Result) plugin.EntriesRequest {
	return plugin.EntriesRequest{ToolVersion: in.ToolVersion, Scratch: in.Scratch, Config: unread(in.Config, have), Have: have, Target: in.Target}
}

// unread is cfg without the files a read result of have returned, by path. It
// answers cfg itself when no read returned a file.
func unread(cfg []plugin.Config, have []plugin.Result) []plugin.Config {
	read := map[string]bool{}
	for _, h := range have {
		if h.Kind != plugin.KindRead {
			continue
		}
		for _, f := range h.Files {
			read[f.Path] = true
		}
	}
	if len(read) == 0 {
		return cfg
	}
	out := make([]plugin.Config, 0, len(cfg))
	for _, c := range cfg {
		if !read[c.Path] {
			out = append(out, c)
		}
	}
	return out
}

// findingsRequest is the findings call of a read, as entriesRequest is the
// entries call: it holds the fixtures where that one holds the target.
func findingsRequest(in plugin.PlanRequest, have []plugin.Result) plugin.FindingsRequest {
	return plugin.FindingsRequest{ToolVersion: in.ToolVersion, Scratch: in.Scratch, Config: in.Config, Have: have, Fixtures: in.Fixtures}
}

// read is one read: the rounds, then the one reading call, made with the
// request that request builds from everything the rounds gathered. A config
// that holds a control character ends it Failed before the first plan call
// (controlFree), so nothing is asked and nothing runs. A context
// done by then ends the read before the call, as before every other call and
// step. Any error of the reading call ends the read Failed, the plugin's
// refused among them: the plugin had its say at the plans, and what it answers
// here is kept as the cause, its code and path with it.
func read[Q, A any](ctx context.Context, p port.Plugin, r Runner, in plugin.PlanRequest, call func(context.Context, Q) (A, error), request func(plugin.PlanRequest, []plugin.Result) Q) (A, error) {
	var none A
	if err := controlFree(in.Config); err != nil {
		return none, &EndError{End: Failed, Round: 0, Step: atCall, Cause: err}
	}
	have, last, err := gather(ctx, p, r, in)
	if err != nil {
		return none, err
	}
	if err := ctx.Err(); err != nil {
		return none, &EndError{End: Failed, Round: last, Step: atCall, Cause: err}
	}
	got, err := call(ctx, request(in, have))
	if err != nil {
		return none, &EndError{End: Failed, Round: last, Step: atCall, Cause: err}
	}
	return got, nil
}

// controlFree is nil when no config of a read holds a control character in its
// text, and otherwise the engine's refusal, naming the config's data by its
// pointer and the character by its code point, before the plugin is asked
// anything. A control character is Unicode's Cc, C0, DEL
// and C1, all but tab, line feed and carriage return. No data format a tool
// reads holds one raw (JSON, YAML and TOML refuse them), and a key that holds
// one may be read by the tool as another key, extends among them. The check is
// of the text alone, whatever its format: whether a key reaches beyond its file,
// and what an escape such as YAML's \e reads as, is the tool's reader, which
// the plugin's plan answers refused. A config sent as base64
// is not text, and is the plugin's to read.
func controlFree(configs []plugin.Config) error {
	for i, c := range configs {
		if c.Data == nil || c.Data.Text == nil {
			continue
		}
		text := *c.Data.Text
		if at := strings.IndexFunc(text, isControl); at >= 0 {
			r, _ := utf8.DecodeRuneInString(text[at:])
			return fmt.Errorf("the config at /config/%d/data holds the control character U+%04X at byte %d, which the tool may read as another key, so it is not handed to the plugin", i, r, at)
		}
	}
	return nil
}

// isControl is whether r is a control character a config may not hold: one of
// Unicode's Cc but tab, line feed and carriage return.
func isControl(r rune) bool {
	switch {
	case r == '\t', r == '\n', r == '\r':
		return false
	case r < 0x20, r == 0x7f:
		return true
	}
	return r >= 0x80 && r <= 0x9f
}

// gather runs the rounds and answers every result, in order, and the round
// the last plan was made in. It ends: the loop makes at most PlanCalls passes,
// each pass runs at most RoundSteps steps, and no pass goes back.
//
// A request over the request cap is not weighed here: the cap is the codec's
// (wire.MaxRequest), which refuses the request before the plugin is asked,
// and the port's limit error of "request" ends the read Failed like any other
// error of a call.
func gather(ctx context.Context, p port.Plugin, r Runner, in plugin.PlanRequest) ([]plugin.Result, int, error) {
	var have []plugin.Result
	for round := range PlanCalls {
		in.Round, in.Have = round, have
		plan, err := ask(ctx, p, in)
		if err != nil {
			return nil, round, err
		}
		got, err := runAll(ctx, r, round, plan.Steps)
		if err != nil {
			return nil, round, err
		}
		have = append(have, got...)
		if plan.Final {
			return have, round, nil
		}
	}
	// Not reached: the last pass ends at its plan's answer. A change that
	// makes it reachable finds a failure here, not a finish.
	return nil, PlanCalls - 1, &EndError{End: Failed, Round: PlanCalls - 1, Step: atCall, Cause: &plugin.LimitError{What: plugin.LimitRounds}}
}

// ask makes one plan call and answers a plan the round can go on from. A plan
// that is not final in the last round ends the read at the answer, so none of
// that round's steps runs.
func ask(ctx context.Context, p port.Plugin, in plugin.PlanRequest) (plugin.PlanResponse, error) {
	fail := func(end End, cause error) (plugin.PlanResponse, error) {
		return plugin.PlanResponse{}, &EndError{End: end, Round: in.Round, Step: atCall, Cause: cause}
	}
	if err := ctx.Err(); err != nil {
		return fail(Failed, err)
	}
	plan, err := p.Plan(ctx, in)
	if err != nil {
		if ce, ok := errors.AsType[*plugin.CallError](err); ok && ce.Code == plugin.CodeRefused {
			return fail(Refused, err)
		}
		return fail(Failed, err)
	}
	switch {
	case len(plan.Steps) > RoundSteps:
		return fail(Failed, &plugin.AnswerError{Path: "/steps", Why: fmt.Sprintf("%d steps, and a round holds at most %d", len(plan.Steps), RoundSteps)})
	case len(plan.Steps) == 0 && !plan.Final:
		return fail(Failed, &plugin.AnswerError{Path: "/final", Why: "no steps and not final, so the next plan would be asked the same"})
	case !plan.Final && in.Round == PlanCalls-1:
		return fail(Failed, &plugin.LimitError{What: plugin.LimitRounds})
	}
	for i, s := range plan.Steps {
		if (s.Run == nil) == (s.Read == nil) {
			return fail(Failed, &plugin.AnswerError{Path: fmt.Sprintf("/steps/%d", i), Why: "a step is one run or one read"})
		}
	}
	return plan, nil
}

// runAll runs the steps of one round and answers their results, in order. It
// asks every grant of the round before its first step: a round that names one
// program outside the grants starts nothing.
func runAll(ctx context.Context, r Runner, round int, steps []plugin.Step) ([]plugin.Result, error) {
	fail := func(end End, step int, cause error) ([]plugin.Result, error) {
		return nil, &EndError{End: end, Round: round, Step: step, Cause: cause}
	}
	for i, s := range steps {
		if err := r.Granted(s); err != nil {
			return fail(Failed, i, err)
		}
	}
	got := make([]plugin.Result, 0, len(steps))
	for i, s := range steps {
		if err := ctx.Err(); err != nil {
			return fail(Failed, i, err)
		}
		res, err := r.Run(ctx, round, i, s)
		if err != nil {
			if errors.Is(err, port.ErrStepNotReady) {
				return fail(NotReady, i, err)
			}
			return fail(Failed, i, err)
		}
		// The place of a result is the engine's to say, never the runner's.
		res.Round, res.Step, res.Kind = round, i, kindOf(s)
		got = append(got, res)
	}
	return got, nil
}

// kindOf is the kind of a step ask let through: a run, or else a read.
func kindOf(s plugin.Step) plugin.Kind {
	if s.Run != nil {
		return plugin.KindRun
	}
	return plugin.KindRead
}
