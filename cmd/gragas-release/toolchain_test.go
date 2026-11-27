package main

import (
	"errors"
	"slices"
	"testing"

	"github.com/birdtable/go-proc/proc"
)

// over sets each extra variable once, at extra's value, whatever environ held:
// go-proc hands a child its list as given, and a child reads the first of two
// settings. Risk: the person's GIT_TERMINAL_PROMPT=1 reaching the guard's git
// ahead of the 0 the toolchain sets, so a remote that asks for a password
// waits on a terminal nobody reads (gragas#2263).
func TestOverSetsEachExtraVariableOnceAtItsValue(t *testing.T) {
	t.Parallel()
	environ := []string{"KEPT=a", "GIT_TERMINAL_PROMPT=1", "ALSO_KEPT=b", "GIT_OPTIONAL_LOCKS=1", "GIT_TERMINAL_PROMPT=2"}
	got := over(environ, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	want := []string{"KEPT=a", "ALSO_KEPT=b", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0"}
	if !slices.Equal(got, want) {
		t.Errorf("over = %q, want %q", got, want)
	}
	if got := over(environ); !slices.Equal(got, environ) {
		t.Errorf("over with nothing extra = %q, want environ as it was", got)
	}
}

// ended is a run's error in os/exec's words: nil only for an exit of 0.
// Risk: a start test or a goreleaser that exited 1, or was killed, read as
// passing now that go-proc answers an exit as a code and no error.
func TestEndedIsNilOnlyForAnExitOfZero(t *testing.T) {
	t.Parallel()
	ran := errors.New("did not start")
	for _, c := range []struct {
		code int
		err  error
		want string
	}{
		{0, nil, ""},
		{1, nil, "exit status 1"},
		{-1, nil, "ended by a signal"},
		{0, ran, "did not start"},
	} {
		got := ended(proc.Result{Code: c.code}, c.err)
		switch {
		case c.want == "" && got != nil:
			t.Errorf("ended(%d, %v) = %v, want nil", c.code, c.err, got)
		case c.want != "" && (got == nil || got.Error() != c.want):
			t.Errorf("ended(%d, %v) = %v, want %q", c.code, c.err, got, c.want)
		}
	}
}
