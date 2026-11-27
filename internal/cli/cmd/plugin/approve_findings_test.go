package plugin_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
)

// TestEveryApproveRefusalEndsOnTheLineToType: a refusal carries a Next, so it
// ends on a closing word. A digest not given points at the
// approve line that shows the digest; the rest at plugin list.
func TestEveryApproveRefusalEndsOnTheLineToType(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]string{
		coreapi.ErrNoSuchPlugin: "gragas plugin list", coreapi.ErrManyVersions: "gragas plugin list",
		coreapi.ErrDigestGiven: "gragas plugin approve alpha",
	} {
		for _, args := range [][]string{{"approve", "alpha"}, {"approve", "alpha", "--digest", string(dig), "--apply"}} {
			s := &session{err: err, shown: shownPlugin()}
			run(t, s, args...)
			if s.ending == nil || s.ending.Word != model.Refused || s.ending.Next != want {
				t.Errorf("%v (%v): ending %+v, want refused with the line %q", err, args, s.ending, want)
			}
		}
	}
	f := &session{err: errors.New("the record cannot be read")}
	run(t, f, "approve", "alpha")
	if f.ending.Word != model.Failed {
		t.Errorf("a record that cannot be read: %+v, want failed", f.ending)
	}
}

// TestTheGrantsAreRowsNotColumns: every grant's versions, version asked and
// command line are shown before the yes, whatever the width: a table drops
// its low-priority columns on a narrow screen, so the screen
// holds no table.
func TestTheGrantsAreRowsNotColumns(t *testing.T) {
	t.Parallel()
	s := &session{shown: shownPlugin()}
	run(t, s, "approve", "alpha")
	for _, b := range s.ending.Lead {
		if _, ok := b.(model.Table); ok {
			t.Errorf("the screen holds a table, whose columns drop at pipe width: %v", s.ending.Lead)
		}
	}
	screen := fmt.Sprint(s.ending.Lead)
	for _, want := range []string{">=1", ">=20", "--version", "the plugin hands it the whole command line", "the script first"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the screen lacks %q: %s", want, screen)
		}
	}
}
