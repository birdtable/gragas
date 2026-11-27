package contract

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/canon"
)

// TestReadBundleRefusesAHistoryStepWhoseStatusDiffersFromItsAfterRecord:
// contract 1.3's history[].status equals history[].after.status where the step
// records one. The schema cannot say so, since after is an
// untyped record, so ReadBundle holds it: a bundle in which a step carries
// a status and no equal after.status is refused, naming the step's number and nothing else
// of it. A step with no status, or two that agree, is read.
//
// What it costs when this fails: the explorer's history view shows a status
// the step did not record.
func TestReadBundleRefusesAHistoryStepWhoseStatusDiffersFromItsAfterRecord(t *testing.T) {
	t.Parallel()
	_, s := world(t, schemaFiles(t))
	v, err := canon.Decode(fixtureBytes(t, "bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := v.(canon.Object)
	steps, _ := doc["history"].(canon.Array)
	at := -1
	for i, st := range steps {
		step, _ := st.(canon.Object)
		after, _ := step["after"].(canon.Object)
		if _, ok := after["status"].(canon.String); ok {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatal("no step of the fixture records a status in after, so none could be planted")
	}
	step, _ := steps[at].(canon.Object)
	after, _ := step["after"].(canon.Object)
	moved, _ := after["status"].(canon.String)
	n, _ := step["number"].(canon.Int)
	number := int(n)
	read := func(named canon.Value) error {
		step["status"] = named
		body, err := canon.Encode(doc, canon.Document)
		if err != nil {
			t.Fatal(err)
		}
		_, err = ReadBundle(body, s)
		return err
	}
	other := canon.String("retired")
	if moved == other {
		other = "active"
	}
	err = read(other)
	if _, ok := errors.AsType[*Refused](err); !ok {
		t.Fatalf("ReadBundle with step %d's status %q and after.status %q = %v, want refused", number, other, moved, err)
	}
	if !strings.Contains(err.Error(), "step "+strconv.Itoa(number)) {
		t.Errorf("the refusal = %q, want it to name step %d", err, number)
	}
	for _, leak := range []string{string(other), string(moved)} {
		if strings.Contains(err.Error(), `"`+leak+`"`) || strings.Contains(err.Error(), "status "+leak) {
			t.Errorf("the refusal = %q, want no status value in it (%q)", err, leak)
		}
	}
	if err := read(moved); err != nil {
		t.Errorf("ReadBundle with the two statuses equal = %v, want read", err)
	}
	if err := read(canon.Null{}); err != nil {
		t.Errorf("ReadBundle with a null status beside after.status = %v, want read", err)
	}
}

// TestReadBundleHoldsTheHistoryStatusRuleOnEveryStep: the rule of
// TestReadBundleRefusesAHistoryStepWhoseStatusDiffersFromItsAfterRecord holds
// at every step of the history, not the first that records a status, and for
// every shape of the after copy: a status beside an after.status that is a
// number or a list, or beside none, is refused as a differing one is (a
// status that is absent or null is not compared). Each case is planted on
// each step in turn.
func TestReadBundleHoldsTheHistoryStatusRuleOnEveryStep(t *testing.T) {
	t.Parallel()
	_, s := world(t, schemaFiles(t))
	cases := []struct {
		name   string
		status canon.Value // the step's status; nil removes the key
		after  canon.Value // after.status; nil removes the key
		refuse bool
	}{
		{"equal", canon.String("active"), canon.String("active"), false},
		{"differing", canon.String("retired"), canon.String("active"), true},
		{"number", canon.String("active"), canon.Int(3), true},
		{"list", canon.String("active"), canon.Array{canon.String("active")}, true},
		{"none beside a status", canon.String("active"), nil, true},
		{"null status beside after", canon.Null{}, canon.String("active"), false},
		{"neither", nil, nil, false},
	}
	fixture := fixtureBytes(t, "bundle.json")
	v, err := canon.Decode(fixture)
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := v.(canon.Object)
	steps, _ := doc["history"].(canon.Array)
	if len(steps) < 2 {
		t.Fatalf("the fixture has %d history steps, want several to plant on", len(steps))
	}
	for at := range steps {
		for _, c := range cases {
			v, err := canon.Decode(fixture)
			if err != nil {
				t.Fatal(err)
			}
			doc, _ := v.(canon.Object)
			steps, _ := doc["history"].(canon.Array)
			step, _ := steps[at].(canon.Object)
			after, _ := step["after"].(canon.Object)
			for _, val := range map[string]struct {
				into canon.Object
				v    canon.Value
			}{"status": {step, c.status}, "after": {after, c.after}} {
				if val.v == nil {
					delete(val.into, "status")
				} else {
					val.into["status"] = val.v
				}
			}
			body, err := canon.Encode(doc, canon.Document)
			if err != nil {
				t.Fatal(err)
			}
			_, err = ReadBundle(body, s)
			_, refused := errors.AsType[*Refused](err)
			switch {
			case c.refuse && !refused:
				t.Errorf("step %d, %s: ReadBundle = %v, want refused", at+1, c.name, err)
			case !c.refuse && err != nil:
				t.Errorf("step %d, %s: ReadBundle = %v, want read", at+1, c.name, err)
			}
		}
	}
}
