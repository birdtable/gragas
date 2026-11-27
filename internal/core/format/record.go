package format

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"time"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Entry is what one history line says about the change, besides who made it,
// when and the plan the yes accepted: those three come with the yes.
type Entry struct {
	// Kind is what the command did, a slug such as init or promote.
	Kind string
	// Entity is the entity kind and its key, such as rule:go.errors.1.
	Entity string
	// Before is the lifecycle fields before the change; nil when the entity is
	// new.
	Before *model.Lifecycle
	// After is the lifecycle fields after the change; at least one is set.
	After model.Lifecycle
	// Decision is the slug of the decision the entry records, "" for none.
	Decision string
	// Reverts is the digest of the entry an undo takes back, "" for none.
	Reverts model.Digest
	// Statements is the digest of each rule's statement, by rule id: a
	// migrate entry holds it, empty when the rulebook held no
	// rule, and every other entry nil, which writes no field.
	Statements map[model.RuleID]model.Digest
}

// noPlan is the plan digest of a line made before the plan has one: apply
// asks for the record once to learn its path and again with the digest set,
// and only the second line is written.
const noPlan = model.Digest("sha256:0000000000000000000000000000000000000000000000000000000000000000")

var kindSlug = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// HistoryRecord is the record function of a plan that ends in a yes: given the
// yes, it makes the one line to append to history.jsonl. The line is read
// against the file as it is when asked: its number is the highest number in
// the file plus one (two branches may each take the same one, and that is not
// a finding), and it names the digest of the last line before it, which is
// the canonical digest of that line. The first line of a file has no previous
// digest and says so by having none. The same file, entry and yes give the
// same bytes.
func HistoryRecord(tree port.Tree, schema port.Schema, root model.Root, l Loaded, entries ...Entry) func(model.Yes) (model.Target, error) {
	file := join(l.Dir, HistoryFile)
	return func(y model.Yes) (model.Target, error) {
		if len(entries) == 0 {
			return model.Target{}, fmt.Errorf("%s: a plan that records itself has an entry to record", file)
		}
		old, err := tree.Read(root, file)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return model.Target{}, err
		}
		number, previous, err := nextInChain(file, old)
		if err != nil {
			return model.Target{}, err
		}
		// A plan's entries are one write: each is numbered after the one before
		// and names its digest, so the chain runs through them as it would if
		// they were appended one by one, and they share the yes and its plan.
		lines := make([][]byte, 0, len(entries))
		for _, e := range entries {
			line, next, err := chained(schema, file, e, y, number, previous)
			if err != nil {
				return model.Target{}, err
			}
			number, previous = number+1, next
			lines = append(lines, line)
		}
		t := model.Target{Path: file, Data: bytes.Join(lines, []byte("\n")), Append: true}
		if len(old) > 0 {
			t.Before = digestOf(old)
		}
		return t, nil
	}
}

// chained is the line of one entry numbered number after the line whose digest
// is previous, checked against the schema, and the digest of the line itself,
// which the next line names.
func chained(schema port.Schema, file string, e Entry, y model.Yes, number int, previous model.Digest) ([]byte, model.Digest, error) {
	line, err := historyLine(file, e, y, number, previous)
	if err != nil {
		return nil, "", err
	}
	if _, bad, err := parse(schema, file, line, historySchema, migrations()); err != nil || len(bad) > 0 {
		if err != nil {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("the history line would be invalid: %s", bad[0])
	}
	v, err := canon.Decode(line)
	if err != nil {
		return nil, "", err
	}
	d, err := canon.DigestOf(v)
	if err != nil {
		return nil, "", err
	}
	return line, model.Digest(d.String()), nil
}

// nextInChain reads the lines already in history.jsonl and answers the number
// of the next and the digest of the last, "" when there is none.
func nextInChain(file string, data []byte) (int, model.Digest, error) {
	highest := 0
	var last model.Digest
	for i, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		v, err := canon.Decode([]byte(line))
		if err != nil {
			return 0, "", fmt.Errorf("%s:%d: the line is not JSON, so the next one cannot name it: %w", file, i+1, err)
		}
		n, ok := numberOf(v)
		if !ok {
			return 0, "", fmt.Errorf("%s:%d: the line has no number, so the chain cannot go on from it", file, i+1)
		}
		highest = max(highest, n)
		d, err := canon.DigestOf(v)
		if err != nil {
			return 0, "", fmt.Errorf("%s:%d: %w", file, i+1, err)
		}
		last = model.Digest(d.String())
	}
	return highest + 1, last, nil
}

func numberOf(v canon.Value) (int, bool) {
	o, ok := v.(canon.Object)
	if !ok {
		return 0, false
	}
	n, ok := o["number"].(canon.Int)
	return int(n), ok && n > 0
}

// historyLine is the canonical bytes of one entry, without a newline.
func historyLine(file string, e Entry, y model.Yes, number int, previous model.Digest) ([]byte, error) {
	if !kindSlug.MatchString(e.Kind) {
		return nil, fmt.Errorf("%s: the kind %q is not a slug such as promote", file, e.Kind)
	}
	if e.Entity == "" || y.Who == "" {
		return nil, fmt.Errorf("%s: an entry names its entity and who made it", file)
	}
	after := lifecycleValue(e.After)
	if len(after) == 0 {
		return nil, fmt.Errorf("%s: the entry for %s holds no lifecycle field after the change", file, e.Entity)
	}
	plan := y.Plan
	if plan == "" {
		plan = noPlan
	}
	v := canon.Object{
		"format": canon.Int(model.FormatVersion), "number": canon.Int(number), "kind": canon.String(e.Kind),
		"entity": canon.String(e.Entity), "after": after, "who": canon.String(y.Who),
		"when": canon.String(time.Unix(y.At, 0).UTC().Format("2006-01-02T15:04:05Z")), "plan": canon.String(string(plan)),
	}
	if previous != "" {
		v["previous"] = canon.String(string(previous))
	}
	if e.Before != nil {
		v["before"] = lifecycleValue(*e.Before)
	}
	if e.Decision != "" {
		v["decision"] = canon.String(e.Decision)
	}
	if e.Reverts != "" {
		v["reverts"] = canon.String(string(e.Reverts))
	}
	if y.Command != "" {
		v["command"] = canon.String(y.Command)
	}
	if e.Statements != nil {
		s := canon.Object{}
		for id, d := range e.Statements {
			s[string(id)] = canon.String(string(d))
		}
		v["statements"] = s
	}
	return canon.Encode(v, canon.Canonical)
}

func lifecycleValue(l model.Lifecycle) canon.Object {
	o := canon.Object{}
	for k, v := range map[string]string{
		"status": string(l.Status), "owner": l.Owner, "review_by": l.ReviewBy, "zero_by": l.ZeroBy, "replaced_by": string(l.ReplacedBy), "literal": l.Literal,
	} {
		if v != "" {
			o[k] = canon.String(v)
		}
	}
	return o
}
