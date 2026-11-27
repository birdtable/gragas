package tool

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// NotFound is a tool that is at none of the places it is looked for at the
// version asked. It is the value the callers turn into "not ready" or "not
// run", naming the version the tested set wants, and matches fs.ErrNotExist;
// another version is never taken in its place.
type NotFound struct {
	// Tool is the tool's name, and Version the version asked for.
	Tool, Version string
	// Tried is each place looked, in order: the step and the path, and the
	// version the tool there answered when it was another.
	Tried []string
	// Reason is why no place was looked at, when one stops the find before
	// any place is tried; "" when the places were looked at.
	Reason string
}

func (e *NotFound) Error() string {
	if e.Reason != "" {
		return e.Tool + " " + e.Version + " is not found: " + e.Reason
	}
	if len(e.Tried) == 0 {
		return e.Tool + " " + e.Version + " is named by no setting, no adapter path and no line of the tools file, so it is not found"
	}
	return e.Tool + " " + e.Version + " is not found; tried " + strings.Join(e.Tried, ", ")
}

// Is makes a NotFound match fs.ErrNotExist, as the port says.
func (e *NotFound) Is(target error) bool { return target == fs.ErrNotExist }

// Refused is a call or a tool the adapter will not run: a call that does
// not fit its shape, or a tool at a path the trust check does not pass. It
// matches port.ErrRefused, and nothing was started. A run held to a memory
// limit this system neither holds nor reads (unheld) matches
// port.ErrNotHeld too.
type Refused struct {
	model.Refusal
	notHeld bool
}

func (e *Refused) Error() string { return e.Observed() }

// Is makes a Refused match port.ErrRefused, and an unheld one
// port.ErrNotHeld.
func (e *Refused) Is(target error) bool {
	return target == port.ErrRefused || e.notHeld && target == port.ErrNotHeld
}

// refused is a *Refused with the line that fixes it.
func refused(observed, fix string) error {
	r, err := model.NewRefusal(model.Refused, observed+", so it is not run", nil, fix)
	if err != nil {
		return errors.Join(port.ErrRefused, err)
	}
	return &Refused{Refusal: r}
}

// StepNotReady is a step whose program, at a version its grant serves, or
// the level it asks for, is missing on this machine: not a failure and not a refusal, and nothing
// was started. It matches port.ErrStepNotReady, and never port.ErrRefused;
// a program that is there and is not what it says it is is a refusal.
type StepNotReady struct {
	// Says is what is missing, and Fix the line that makes it ready.
	Says, Fix string
}

func (e *StepNotReady) Error() string { return e.Says + ", so the step is not ready; " + e.Fix }

// Is makes a StepNotReady match port.ErrStepNotReady.
func (e *StepNotReady) Is(target error) bool { return target == port.ErrStepNotReady }

// notReady is a *StepNotReady with the line that makes it ready.
func notReady(says, fix string) error { return &StepNotReady{Says: says, Fix: fix} }

// Ended is a run the adapter ended or did not count: it printed past its
// cap or ran past its deadline, and its whole tree was ended; or it held
// more memory at its peak, in its largest single process, than its limit,
// or may have left a process of its tree running whose peak was never
// read. The Output that comes with it holds what it printed up to the cap,
// with Cut set when the cap ended it.
type Ended struct {
	// Tool is the tool's name.
	Tool string
	// Why is "cap", "deadline" or "memory".
	Why string
	// Says is the sentence for a person.
	Says string
	// Limit names the limit a deadline or memory hit passed, such as "the
	// memory limit of 16 MiB"; "" for a cap.
	Limit string
}

func (e *Ended) Error() string {
	if e.Why == EndedMemory {
		return e.Tool + " " + e.Says + ", so its result is not counted"
	}
	return e.Tool + " " + e.Says + ", so its process tree was ended"
}

// NotRun is the cell a limit hit makes, "not run" with the limit named, and
// whether e is one: a run past its deadline or its memory limit (spec, the
// states of a cell). A run past its cap is not: its output was cut.
func (e *Ended) NotRun() (string, bool) {
	if e.Why != EndedDeadline && e.Why != EndedMemory {
		return "", false
	}
	return "not run: " + e.Limit + " was hit", true
}

// The three reasons a run is ended or not counted.
const (
	EndedCap      = "cap"
	EndedDeadline = "deadline"
	EndedMemory   = "memory"
)
