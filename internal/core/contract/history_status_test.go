package contract

import (
	"testing"

	"github.com/birdtable/gragas/internal/canon"
)

// TestAHistoryStepsStatusIsItsAfterRecords: contract 1.3 names the status a
// step moved a rule to as history[].status, beside after, which stays an
// untyped record because the generator types no key of an open object.
// The two are one fact written twice, so the fixture is
// held to them being equal: every step whose after record holds a status
// carries the same status, a step whose after holds none carries none, and the
// fixture has such steps to hold.
//
// What it costs when this fails: the explorer's history shows a status the
// step did not move the rule to, and nothing says the two copies disagree.
func TestAHistoryStepsStatusIsItsAfterRecords(t *testing.T) {
	t.Parallel()
	v, err := canon.Decode(fixtureBytes(t, "bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := v.(canon.Object)
	steps, _ := doc["history"].(canon.Array)
	held := 0
	for i, s := range steps {
		step, _ := s.(canon.Object)
		after, _ := step["after"].(canon.Object)
		moved, recorded := after["status"].(canon.String)
		named, has := step["status"].(canon.String)
		switch {
		case recorded && (!has || named != moved):
			t.Errorf("history[%d] (%v): status is %q, %v, and after.status is %q", i, step["entity"], named, has, moved)
		case !recorded && has:
			t.Errorf("history[%d] (%v): status is %q, and after records no status", i, step["entity"], named)
		case recorded:
			held++
		}
	}
	if held == 0 {
		t.Fatalf("no step of the fixture's %d records a status, so nothing was held", len(steps))
	}
}
