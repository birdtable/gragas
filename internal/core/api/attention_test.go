package coreapi_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// threeActive is a rules file of three active rules, each confirmed to a date
// a half year or more apart.
const threeActive = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: active
    origin: authored
    owner: team-a
    review_by: "2026-12-01"
  - id: go.errors.2
    statement: Return an error, never panic.
    strength: should
    status: active
    origin: authored
    owner: team-a
    review_by: "2027-06-01"
  - id: go.errors.3
    statement: Name a sentinel error with the prefix Err.
    strength: may
    status: active
    origin: authored
    owner: team-a
    review_by: "2027-12-31"
`

func overdueOn(t *testing.T, w *porttest.World, root model.Root, day time.Time) []string {
	t.Helper()
	w.Instant(day)
	got, err := coreapi.Attention(t.Context(), w.Ports(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Today != day.Format(time.DateOnly) {
		t.Fatalf("today = %s, want the clock's day %s", got.Today, day.Format(time.DateOnly))
	}
	var out []string
	for _, i := range got.Items {
		if i.Kind == model.AttentionOverdue {
			out = append(out, string(i.Rule))
		}
	}
	return out
}

// TestAttentionJudgesDatesByTheClockPortAlone: the same book, the Clock moved
// 400 days on, and the overdue list grows from none to the two rules whose
// review-by the new day passes. Risk: a date judged against the machine's day,
// which this test cannot move.
func TestAttentionJudgesDatesByTheClockPortAlone(t *testing.T) {
	t.Parallel()
	w, root := historyWorld(t, threeActive, "")
	start := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	if got := overdueOn(t, w, root, start); len(got) != 0 {
		t.Fatalf("on the first day overdue = %v, want none", got)
	}
	if got := overdueOn(t, w, root, start.AddDate(0, 0, 400)); !slices.Equal(got, []string{"go.errors.1", "go.errors.2"}) {
		t.Fatalf("400 days on overdue = %v, want go.errors.1 and go.errors.2", got)
	}
}

// TestAttentionOfAFolderWithNoRulebookIsNoBook: the error the command ends not
// ready on.
func TestAttentionOfAFolderWithNoRulebookIsNoBook(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{"README.md": "x"})
	if _, err := coreapi.Attention(t.Context(), w.Ports(), root); !errors.Is(err, coreapi.ErrNoBook) {
		t.Fatalf("err = %v, want no book", err)
	}
}

// TestTheAttentionDocumentSaysWhyTheOverrideRateIsNull: no batches are given,
// so the value is null and the reason is in the document (cli.output.9).
func TestTheAttentionDocumentSaysWhyTheOverrideRateIsNull(t *testing.T) {
	t.Parallel()
	d := coreapi.AttentionDoc(model.AttentionListing{})
	if d.Schema != coreapi.AttentionSchema || d.OverrideRate.Value != nil || d.OverrideRate.Reason != "no batches given" {
		t.Fatalf("doc = %+v", d)
	}
}
