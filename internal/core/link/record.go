package link

import (
	"regexp"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
)

// RecordsDir is the folder of a rulebook's binding records, below its folder.
const RecordsDir = "bindings"

// The keys of a binding record: its subject first, then a binding's.
// tightens is not one, since the subject says what the record tightens.
func recordKeys() []string {
	return []string{"subject", "tool", "target", "key", "value", "options", "severity", "rung", "run_point", "remedy", "pair", "pair_path", "checker"}
}

// subjectForm is a rule id as the format schema holds one (common.schema.json,
// id), the same the overlay contract reads a record's first line with.
var subjectForm = regexp.MustCompile(`^[a-z]+\.[a-z0-9]+(?:-[a-z0-9]+)*\.[1-9][0-9]*$`)

// ReadRecord reads the one binding record data, which stands at path below
// the rulebook's folder (S34, S70): its subject on the first line, then the
// binding the rulebook holds the borrowed rule to, in the keys of a rule's
// binding. Every key that names the entry and where it runs is written out,
// so the record is compared with the borrowed binding at that entry and never
// another. It answers the record, or every reason it is refused, and never
// both: a record decides what a borrowed rule is held to, so one that cannot
// be read is refused, never guessed at.
func ReadRecord(path string, data []byte) (model.Tightening, []Refusal) {
	r := &reader{path: path, kind: "a binding record", kinds: "binding records"}
	if len(data) > MaxDocument {
		r.refuse(0, CheckSize, "the document is %d bytes, over the cap of %d", len(data), MaxDocument)
		return model.Tightening{}, r.out
	}
	v, err := ordered.ParseStrict(data)
	if err != nil {
		r.yaml(err)
		return model.Tightening{}, r.out
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		r.refuse(1, CheckShape, "the document must be a mapping of a subject and a binding's fields")
		return model.Tightening{}, r.out
	}
	r.keys(m, "", recordKeys()...)
	if ks := m.Keys(); len(ks) == 0 || ks[0] != "subject" {
		r.refuse(1, CheckShape, "the first line is subject: <rule id>, the rule the record tightens")
	}
	subject := r.word(m, "", "subject", subjectForm, "a rule id, part.concern.number", true)
	b := model.Binding{
		Tool:     r.word(m, "", "tool", slugForm, "a lowercase tool name", true),
		Target:   oneOf(r, m, "", "target", model.TargetTypes(), true),
		Key:      r.word(m, "", "key", wordForm, "an entry key", true),
		Severity: oneOf(r, m, "", "severity", model.Severities(), true),
		Rung:     r.rung(m),
		RunPoint: oneOf(r, m, "", "run_point", model.RunPoints(), true),
		Pair:     r.whole(m, "pair"),
	}
	// the free text a listing prints is held as a declaration's is, so a
	// remedy shows as written
	b.Remedy = r.plain(m, "remedy", false)
	b.PairPath = r.plain(m, "pair_path", false)
	b.Checker = r.plain(m, "checker", false)
	if x, has := m.Get("value"); has {
		b.Value = ordered.Plain(x)
	}
	if o := r.sub(m, "", "options", false); o != nil {
		// an option's name is printed in a refusal as written, so it is a word
		for _, k := range o.Keys() {
			if !wordForm.MatchString(k) {
				r.refuse(o.KeyLine(k), CheckShape, "the option %q is not a word of letters, digits and . _ / -, and an option's name is one", k)
			}
		}
		if opts, ok := ordered.Plain(o).(map[string]any); ok && len(opts) > 0 {
			b.Options = opts
		}
	}
	if len(r.out) > 0 {
		return model.Tightening{}, r.out
	}
	return model.Tightening{Path: path, Subject: model.RuleID(subject), Binding: b, Text: string(data)}, nil
}

// ReadRecords reads each record, in the order given, and answers those read
// and the refusals of those not.
func ReadRecords(records []model.Opaque) ([]model.Tightening, []Refusal) {
	var out []model.Tightening
	var bad []Refusal
	for _, o := range records {
		t, refused := ReadRecord(o.Path, o.Data)
		if len(refused) > 0 {
			bad = append(bad, refused...)
			continue
		}
		out = append(out, t)
	}
	return out, bad
}

// rung reads the rung, a whole number from 0 to model.MaxRung.
func (r *reader) rung(m *ordered.Map) model.Rung {
	v, has := m.Get("rung")
	if !has {
		r.refuse(m.Line, CheckShape, "rung is missing")
		return 0
	}
	n, ok := v.(int64)
	if !ok || n < 0 || n > int64(model.MaxRung) {
		r.refuse(m.KeyLine("rung"), CheckShape, "rung is %s, and it must be a whole number from 0 to %d", ordered.Repr(v), model.MaxRung)
		return 0
	}
	return model.Rung(n)
}

// whole reads an optional whole number of at least 1.
func (r *reader) whole(m *ordered.Map, k string) int {
	v, has := m.Get(k)
	if !has {
		return 0
	}
	n, ok := v.(int64)
	if !ok || n < 1 {
		r.refuse(m.KeyLine(k), CheckShape, "%s is %s, and it must be a whole number from 1", k, ordered.Repr(v))
		return 0
	}
	return int(n)
}
