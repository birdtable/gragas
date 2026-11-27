package format

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// The changes a delta makes to an overlay's exclusions.yaml (S4). The entity
// is exclusion:<overlay>/<rule>/<number>, the record's key:
//
//   - with no Field and no Before, After is the new record as the YAML mapping
//     of its fields, added at the end of the list. A file that is not there yet
//     is made from its head. A record the file holds under the key already is
//     refused with ErrMoved.
//   - with a Field of lifted.decision, lifted.who or lifted.when and Before "",
//     the scalar is written into the record's lifted mapping. A record that is
//     lifted already is refused with ErrMoved.
//
// A record is never removed, and no other field of one changes: a lifted
// record stays (S4).

// exclusionKind is the list of an exclusions file, keyed by the parent's rule.
func exclusionKind() entryKind { return entryKind{what: "exclusion", list: "exclusions", key: "rule"} }

// exclusionKey is an exclusion's key as its entity names it.
type exclusionKey struct {
	overlay string
	rule    string
	number  int
}

func (k exclusionKey) String() string { return fmt.Sprintf("%s/%s/%d", k.overlay, k.rule, k.number) }

func (k exclusionKey) holds(m *ordered.Map) bool {
	return m.Str("overlay") == k.overlay && m.Str("rule") == k.rule && intOf(m.Val("number")) == k.number
}

func parseExclusionKey(file, entity string) (exclusionKey, error) {
	_, key, _ := strings.Cut(entity, ":")
	parts := strings.Split(key, "/")
	if len(parts) == 3 && parts[0] != "" && parts[1] != "" {
		if n, err := strconv.Atoi(parts[2]); err == nil && n >= 1 && strconv.Itoa(n) == parts[2] {
			return exclusionKey{overlay: parts[0], rule: parts[1], number: n}, nil
		}
	}
	return exclusionKey{}, unsupported(file, 1, fmt.Sprintf("the entity %q is not exclusion:<overlay>/<rule>/<number>", entity))
}

// renderExclusions applies the changes to the exclusions file and checks the
// result still passes its schema.
func renderExclusions(schema port.Schema, file string, old []byte, exists bool, changes []model.Change) ([]byte, error) {
	if !exists {
		old = fmt.Appendf(nil, "format: %d\n%s:\n", model.FormatVersion, exclusionKind().list)
	}
	t, err := newText(file, old)
	if err != nil {
		return nil, err
	}
	for _, c := range changes {
		k, err := parseExclusionKey(file, c.Entity)
		if err != nil {
			return nil, err
		}
		if err := t.exclusion(k, c); err != nil {
			return nil, err
		}
	}
	next := t.bytes()
	if _, bad, err := parse(schema, file, next, exclusionsSchema, migrations()); err != nil || len(bad) > 0 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("the edit would leave %s invalid: %s", file, bad[0])
	}
	return next, nil
}

// exclusion makes one change to the record k names.
func (t *text) exclusion(k exclusionKey, c model.Change) error {
	doc, err := t.doc()
	if err != nil {
		return err
	}
	held := 0
	for _, m := range ordered.Maps(doc.List(exclusionKind().list)) {
		if k.holds(m) {
			held++
		}
	}
	switch field, lifted := strings.CutPrefix(c.Field, "lifted."); {
	case c.Field == "" && c.Before == "":
		v, err := ordered.ParseYAML([]byte(c.After))
		if m, ok := v.(*ordered.Map); err != nil || !ok || !k.holds(m) {
			return fmt.Errorf("the new exclusion %s is not a mapping with that overlay, rule and number", k)
		}
		return t.addEntry(exclusionKind(), k.rule, c.After, held > 0)
	case held > 1:
		return unsupported(t.file, 1, fmt.Sprintf("%d records share the key %s; gragas validate lists them, and one takes the next number by hand", held, k))
	case lifted && (field == "decision" || field == "who" || field == "when") && c.Before == "":
		it, err := t.listItem(exclusionKind(), k.String(), "a dash", k.holds)
		if err != nil {
			return err
		}
		return t.setIn(it, "the exclusion "+k.String(), []string{"lifted", field}, "", c.After)
	}
	return unsupported(t.file, 1, fmt.Sprintf("a change to %s adds the record or sets its lifted fields; %q from %q is neither", k, c.Field, c.Before))
}
