package model_test

import (
	"errors"
	"os"
	"slices"
	"testing"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/model"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

func TestTheEightWordsAreTheClosedSet(t *testing.T) {
	t.Parallel()
	want := []string{"done", "failed", "findings", "usage", "busy", "not ready", "refused", "declined"}
	var got []string
	for _, w := range model.Words() {
		got = append(got, string(w))
	}
	if !slices.Equal(got, want) {
		t.Errorf("Words() = %q, want %q", got, want)
	}
	if len(got) != 8 {
		t.Errorf("%d words, want 8", len(got))
	}
}

func TestTheSetCannotBeChangedThroughWhatWordsReturns(t *testing.T) {
	t.Parallel()
	model.Words()[0] = "changed"
	if model.Words()[0] != model.Done {
		t.Error("a caller changed the closed set")
	}
}

func TestKnownRefusesAWordOutsideTheSet(t *testing.T) {
	t.Parallel()
	for _, w := range model.Words() {
		if !model.Known(w) {
			t.Errorf("Known(%q) = false", w)
		}
	}
	for _, w := range []model.Word{"", "ok", "Done", "needs you", "stopped at"} {
		if model.Known(w) {
			t.Errorf("Known(%q) = true", w)
		}
	}
}

func TestARefusalWithoutAHintIsNotBuilt(t *testing.T) {
	t.Parallel()
	for _, w := range []model.Word{model.Usage, model.Busy, model.NotReady, model.Refused, model.Declined} {
		if _, err := model.NewRefusal(w, "saw x", nil, ""); !errors.Is(err, model.ErrNoHint) {
			t.Errorf("%q without a hint: err = %v, want ErrNoHint", w, err)
		}
	}
}

func TestARefusalNeedsAWordThatRefuses(t *testing.T) {
	t.Parallel()
	for _, w := range []model.Word{model.Done, model.Failed, model.Findings, "", "nonsense"} {
		if _, err := model.NewRefusal(w, "saw x", nil, "gragas help"); !errors.Is(err, model.ErrNotRefusing) {
			t.Errorf("%q: err = %v, want ErrNotRefusing", w, err)
		}
	}
}

func TestARefusalKeepsWhatItWasGiven(t *testing.T) {
	t.Parallel()
	valid := []string{"a", "b"}
	r, err := model.NewRefusal(model.Declined, "no yes was given", valid, "gragas gen --apply")
	if err != nil {
		t.Fatal(err)
	}
	valid[0] = "changed"
	if r.Word() != model.Declined || r.Observed() != "no yes was given" || r.Hint() != "gragas gen --apply" {
		t.Errorf("refusal = %v %q %q", r.Word(), r.Observed(), r.Hint())
	}
	if !slices.Equal(r.Valid(), []string{"a", "b"}) {
		t.Errorf("Valid() = %q, a later change to the argument leaked in", r.Valid())
	}
}

func TestAFailureNeedsItsKindAndTheRunID(t *testing.T) {
	t.Parallel()
	if _, err := model.NewFailure("", "r1"); !errors.Is(err, model.ErrNoKind) {
		t.Errorf("no kind: err = %v, want ErrNoKind", err)
	}
	if _, err := model.NewFailure("settings", ""); !errors.Is(err, model.ErrNoRun) {
		t.Errorf("no run: err = %v, want ErrNoRun", err)
	}
	f, err := model.NewFailure("settings", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if f.Word() != model.Failed || f.Kind() != "settings" || f.Run() != "r1" {
		t.Errorf("failure = %v %q %q", f.Word(), f.Kind(), f.Run())
	}
}
