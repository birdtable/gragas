package apply_test

import (
	"testing"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/model"
)

// TestDigestBytesIsTheSHA256OfTheBytes: two published vectors, the empty
// file and "abc". Expected values are the standard's, not computed here.
func TestDigestBytesIsTheSHA256OfTheBytes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, in string
		want     model.Digest
	}{
		{"the empty file", "", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"the three letters abc", "abc", "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	} {
		if got := apply.DigestBytes([]byte(tc.in)); got != tc.want {
			t.Errorf("%s: DigestBytes(%q) = %s, want %s", tc.name, tc.in, got, tc.want)
		}
	}
}

// basePlan is a small plan with every part filled.
func basePlan() model.Plan {
	return model.Plan{
		Command:  "gragas rule retire x --apply",
		Delta:    model.Delta{Changes: []model.Change{{Entity: "rule:x", Field: "status", Before: "active", After: "retired"}}},
		Files:    []model.Target{{Path: "rules/x.yaml", Before: apply.DigestBytes([]byte("old")), Data: []byte("new")}},
		Sections: []model.Section{{Title: "what changes", Lines: []string{"x: active -> retired"}}},
		Inputs:   []model.Input{{Path: "identity.yaml", Digest: apply.DigestBytes([]byte("id"))}},
		Goal:     model.Goal{Files: []model.Input{{Path: "rules/x.yaml", Digest: apply.DigestBytes([]byte("new"))}}},
		Yes:      &model.Yes{Who: "A Person <a@example.test>", At: 1794132000, Command: "gragas rule retire x --apply"},
		Record:   &model.Target{Path: "history.jsonl", Data: []byte(`{"plan":"later"}`), Append: true},
	}
}

// TestThePlanDigestIsPinned: a golden. The digest of basePlan is the form
// that plans, histories and releases store, so a change to it is a change to
// a compatibility surface and a person reads the diff. The literal was
// checked against an independent RFC 8785 encoding (node, sorted keys, SHA-256)
// before the time of the yes left the digest; the literal now is the
// program's own and differs from the old one by that field alone.
func TestThePlanDigestIsPinned(t *testing.T) {
	t.Parallel()
	got, err := apply.Digest(basePlan())
	if err != nil {
		t.Fatal(err)
	}
	const want = "sha256:63433e2722c6e686cfb3aa00f612487339c32299ffb015714f8cce4ec13f0d9e"
	if got != want {
		t.Errorf("Digest = %s, want %s", got, want)
	}
}

// TestThePlanDigestCoversEveryPartButItsOwnFieldsAndTheRecordBytes: each edit
// below moves the digest, and the four that cannot (the digest field, the
// yes's plan, the record's bytes and the order a map was built in) do not.
// Risk: a part of the plan a person saw that the digest does not cover, or a
// digest that depends on itself.
func TestThePlanDigestCoversEveryPartButItsOwnFieldsAndTheRecordBytes(t *testing.T) {
	t.Parallel()
	base, err := apply.Digest(basePlan())
	if err != nil {
		t.Fatal(err)
	}
	moves := map[string]func(*model.Plan){
		"command":         func(p *model.Plan) { p.Command += "x" },
		"delta":           func(p *model.Plan) { p.Delta.Changes[0].After = "trial" },
		"a file's bytes":  func(p *model.Plan) { p.Files[0].Data = []byte("newer") },
		"a file's path":   func(p *model.Plan) { p.Files[0].Path = "rules/y.yaml" },
		"a file's before": func(p *model.Plan) { p.Files[0].Before = "" },
		"append":          func(p *model.Plan) { p.Files[0].Append = true },
		"a section":       func(p *model.Plan) { p.Sections[0].Lines[0] += "." },
		"an input":        func(p *model.Plan) { p.Inputs[0].Digest = apply.DigestBytes([]byte("id2")) },
		"an extra input":  func(p *model.Plan) { p.Inputs = append(p.Inputs, model.Input{Path: "pin.yaml"}) },
		"the goal":        func(p *model.Plan) { p.Goal.Files[0].Digest = "" },
		"who":             func(p *model.Plan) { p.Yes.Who = "B" },
		"no yes":          func(p *model.Plan) { p.Yes = nil },
		"the record path": func(p *model.Plan) { p.Record.Path = "other.jsonl" },
		"no record":       func(p *model.Plan) { p.Record = nil },
	}
	for name, edit := range moves {
		p := basePlan()
		edit(&p)
		got, err := apply.Digest(p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got == base {
			t.Errorf("changing %s leaves the digest %s", name, base)
		}
	}
	for name, edit := range map[string]func(*model.Plan){
		"the digest field":    func(p *model.Plan) { p.Digest = "sha256:x" },
		"the yes's plan":      func(p *model.Plan) { p.Yes.Plan = "sha256:y" },
		"the record bytes":    func(p *model.Plan) { p.Record.Data = []byte("another line") },
		"the time of the yes": func(p *model.Plan) { p.Yes.At += 2 },
	} {
		p := basePlan()
		edit(&p)
		if got, _ := apply.Digest(p); got != base {
			t.Errorf("changing %s moves the digest to %s", name, got)
		}
	}
}

// TestAPlanWithTextThatIsNotUTF8HasNoDigest: canon refuses it, so a plan
// cannot be sealed from bytes two readers would read two ways.
func TestAPlanWithTextThatIsNotUTF8HasNoDigest(t *testing.T) {
	t.Parallel()
	p := basePlan()
	p.Sections[0].Title = "\xff"
	if got, err := apply.Digest(p); err == nil {
		t.Errorf("Digest = %s, want an error", got)
	}
}
