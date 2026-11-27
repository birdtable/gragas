package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/birdtable/go-proc/proc"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// TestTaskEndSaysWhyATaskGaveNoCodeOfItsOwn holds taskEnd (gragas#2263,
// gragas#2408): a task's own code passes through and says nothing, and a run
// that did not reach its end, or a task ended by a signal, is exitFailed and
// says which. Before, a task ended by a signal failed the hook with no word.
func TestTaskEndSaysWhyATaskGaveNoCodeOfItsOwn(t *testing.T) {
	for _, c := range []struct {
		name string
		code int
		err  error
		want int
		says string
	}{
		{"done", 0, nil, exitDone, ""},
		{"its own code", 3, nil, 3, ""},
		{"a signal", -1, nil, exitFailed, "was ended by a signal"},
		{"its timeout", -1, proc.Ended{Why: proc.WhyDeadline}, exitFailed, "did not run to its end"},
		{"refused", -1, proc.ErrRefused, exitFailed, "did not run to its end"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, says := taskEnd(c.code, c.err)
			if got != c.want || (c.says == "") != (says == "") || !strings.Contains(says, c.says) {
				t.Errorf("taskEnd(%d, %v) = %d, %q; want %d and words holding %q", c.code, c.err, got, says, c.want, c.says)
			}
		})
	}
}

// hangingHook is a hook over a clone whose stand-in task hangs on pre-commit,
// and the buffer it prints to.
func hangingHook(t *testing.T) (*hook, *bytes.Buffer) {
	t.Helper()
	dir, _, _, _ := hookClone(t, fakeTaskConfig{Hang: []string{"pre-commit"}}, "")
	var out bytes.Buffer
	h, err := newHook(t.Context(), dir, configtest.Inherited(), &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	return h, &out
}

// withinAMinute is the code run gives, failing the test if run has not ended a
// minute after it started: the runs below end in seconds, and a hung one would
// otherwise hold the test to its own deadline. It waits on a timer rather than
// reading the clock, which only the Clock adapter does (arch.ports.1).
func withinAMinute(t *testing.T, what string, run func() int) int {
	t.Helper()
	done := make(chan int, 1)
	go func() { done <- run() }()
	select {
	case code := <-done:
		return code
	case <-time.After(time.Minute):
		t.Fatalf("%s: the run had not ended a minute after it started", what)
		return 0
	}
}

// TestATaskThatHangsIsEndedAtItsTimeoutAndSaysSo (gragas#2263): a task past
// the run's timeout is ended, its tree killed, and the hook fails saying why,
// long before the task's own hour.
func TestATaskThatHangsIsEndedAtItsTimeoutAndSaysSo(t *testing.T) {
	h, out := hangingHook(t)
	code := withinAMinute(t, "a hung task with a timeout of 2s", func() int {
		return h.runTask(t.Context(), h.root, h.env, 2*time.Second, "pre-commit")
	})
	if code != exitFailed || !strings.Contains(out.String(), "task pre-commit did not run to its end") || !strings.Contains(out.String(), "reached its timeout") {
		t.Errorf("a hung task: code %d, output %q; want %d and the words naming its deadline", code, out, exitFailed)
	}
}

// TestACancelledHookEndsItsTask (gragas#2263): Ctrl+C cancels the context
// term.Main gives the hook, and the task's tree is ended with it, not left to
// run on with no one waiting for it.
func TestACancelledHookEndsItsTask(t *testing.T) {
	h, out := hangingHook(t)
	ctx, cancel := context.WithTimeoutCause(t.Context(), 2*time.Second, errors.New("the test's Ctrl+C"))
	defer cancel()
	code := withinAMinute(t, "a hung task whose context ends after 2s", func() int {
		return h.runTask(ctx, h.root, h.env, time.Hour, "pre-commit")
	})
	if code != exitFailed || !strings.Contains(out.String(), "task pre-commit did not run to its end") || !strings.Contains(out.String(), "context ended") {
		t.Errorf("a cancelled task: code %d, output %q; want %d and the words naming the cancel", code, out, exitFailed)
	}
}
