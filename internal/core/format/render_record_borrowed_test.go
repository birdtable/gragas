package format

import (
	"errors"
	"testing"

	"github.com/birdtable/gragas/internal/model"
)

// A borrowed rule's record is written whole, at its path below the book's
// folder, against the text the plan read (S34, S70).
func TestABindingRecordIsWrittenWholeAgainstTheTextThePlanRead(t *testing.T) {
	t.Parallel()
	const was = "subject: go.errors.1\ntool: a\ntarget: source\nkey: k\nseverity: warning\nrung: 1\nrun_point: commit\n"
	const next = "subject: go.errors.1\ntool: a\ntarget: source\nkey: k\nseverity: error\nrung: 1\nrun_point: commit\n"
	files := fullBook(t)
	files["bindings/go-errors-1.yaml"] = was
	w, root := world(t, files)
	l, err := load(t, w, root)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Ports()
	for name, c := range map[string]struct {
		change model.Change
		path   string
		before bool
		err    error
	}{
		"a new record":               {model.Change{Entity: "record:bindings/go-errors-1-2.yaml", After: next}, "bindings/go-errors-1-2.yaml", false, nil},
		"a record replaced":          {model.Change{Entity: "record:bindings/go-errors-1.yaml", Before: was, After: next}, "bindings/go-errors-1.yaml", true, nil},
		"a record that moved":        {model.Change{Entity: "record:bindings/go-errors-1.yaml", Before: next, After: was}, "", false, ErrMoved},
		"a new record that is there": {model.Change{Entity: "record:bindings/go-errors-1.yaml", After: next}, "", false, ErrMoved},
		"a record taken out":         {model.Change{Entity: "record:bindings/go-errors-1.yaml", Before: was}, "", false, ErrUnsupported},
		"a path outside bindings/":   {model.Change{Entity: "record:rulebook.yaml", After: next}, "", false, ErrUnsupported},
		"a path that climbs out":     {model.Change{Entity: "record:bindings/../rulebook.yaml", After: next}, "", false, ErrUnsupported},
		"a path below the folder":    {model.Change{Entity: "record:bindings/x/y.yaml", After: next}, "", false, ErrUnsupported},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ts, err := Render(p.Tree, p.Schema, root, l, model.Delta{Changes: []model.Change{c.change}})
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("err = %v, want %v", err, c.err)
				}
				return
			}
			if err != nil || len(ts) != 1 {
				t.Fatalf("targets = %+v, %v", ts, err)
			}
			if ts[0].Path != c.path || string(ts[0].Data) != next || (ts[0].Before != "") != c.before {
				t.Errorf("target = %s %q before %q", ts[0].Path, ts[0].Data, ts[0].Before)
			}
		})
	}
}
