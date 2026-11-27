package model_test

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// TestACausedFailureNamesItsRunAndKeepsItsCause (A39): a Caused says what was
// being done and the run id, closes on the failed word, and unwraps to its
// cause; one with no run is not built, and its error still carries the
// cause and why. Risk: an adapter's failure that loses its cause or its id.
func TestACausedFailureNamesItsRunAndKeepsItsCause(t *testing.T) {
	t.Parallel()
	c, err := model.NewCaused("git", "run-1", "git could not be read", fs.ErrPermission)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.Error(), "git could not be read (run run-1): permission denied"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if c.Word() != model.Failed || c.Kind() != "git" || !errors.Is(c, fs.ErrPermission) {
		t.Errorf("word %v, kind %q, is ErrPermission %v; want failed, git, true", c.Word(), c.Kind(), errors.Is(c, fs.ErrPermission))
	}
	if _, err := model.NewCaused("git", "", "git could not be read", fs.ErrPermission); !errors.Is(err, model.ErrNoRun) || !errors.Is(err, fs.ErrPermission) {
		t.Errorf("no run: %v; want ErrNoRun and the cause", err)
	}
}
