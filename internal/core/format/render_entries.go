package format

import (
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// The entries a delta adds to sources.yaml, decisions.yaml and terms.yaml:
//
//   - source:<slug>, decision:<slug> and term:<slug>, with no Field and no
//     Before: After is the new entry as the YAML mapping of its fields, added
//     at the end of the file's list. A file that is not there yet is made from
//     its head.
//   - term:<slug> with a Field of literal or status: the scalar goes from
//     Before to After in the term's entry ("" for a field that is not there,
//     or is to be taken out), and no other byte of the file is touched.
//
// Beyond adding, a source's title, locator and attribution and a decision's
// superseded_by are set from their Before value: the source family's edit and the
// decision family's supersede (S59). Any other field of one is refused here.

// entryFile is where an entity kind's entries live: the file below the book's
// folder, the schema that holds it and the list in it.
func entryFile(l Loaded, kind string) (file, schemaName string, k entryKind, ok bool) {
	switch kind {
	case "source":
		return join(l.Dir, SourcesFile), sourcesSchema, entryKind{what: "source", list: "sources", key: "slug"}, true
	case "decision":
		return join(l.Dir, DecisionsFile), decisionsSchema, entryKind{what: "decision", list: "decisions", key: "slug"}, true
	case "term":
		return join(l.Dir, TermsFile), termsSchema, entryKind{what: "term", list: "terms", key: "slug"}, true
	}
	return "", "", entryKind{}, false
}

// entryTarget is the file an entry is added to, the schema it is checked by and
// the kind of entry it holds.
type entryTarget struct {
	file, schema string
	kind         entryKind
}

// renderEntries adds the entries of changes to one sources or decisions file
// and checks the result still passes the file's schema.
func renderEntries(schema port.Schema, target entryTarget, old []byte, exists bool, changes []model.Change) ([]byte, error) {
	file, schemaName, k := target.file, target.schema, target.kind
	if !exists {
		old = fmt.Appendf(nil, "format: %d\n%s:\n", model.FormatVersion, k.list)
	}
	t, err := newText(file, old)
	if err != nil {
		return nil, err
	}
	for _, c := range changes {
		if err := applyEntry(t, file, k, c); err != nil {
			return nil, err
		}
	}
	next := t.bytes()
	if _, bad, err := parse(schema, file, next, schemaName, migrations()); err != nil || len(bad) > 0 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("the edit would leave %s invalid: %s", file, bad[0])
	}
	return next, nil
}

// editable are the fields of an entry a plan sets, by kind of entry.
func editable(what string) []string {
	switch what {
	case "term":
		return []string{"literal", "status"}
	case "source":
		return []string{"title", "locator", "attribution"}
	case "decision":
		return []string{"superseded_by"}
	}
	return nil
}

// applyEntry makes one change to the entries of a file: a new entry added at the
// end of the list, or a field set from its before value: a term's literal or
// status, a source's title, locator or attribution.
func applyEntry(t *text, file string, k entryKind, c model.Change) error {
	_, slug, _ := strings.Cut(c.Entity, ":")
	if c.Field != "" || c.Before != "" {
		if !slices.Contains(editable(k.what), c.Field) {
			return unsupported(file, 1, fmt.Sprintf("a change to %s adds an entry only; editing %s of one is not a %s command's", c.Entity, c.Field, k.what))
		}
		return t.setEntryField(k, slug, c.Field, c.Before, c.After)
	}
	doc, err := t.doc()
	if err != nil {
		return err
	}
	has := false
	for _, m := range ordered.Maps(doc.List(k.list)) {
		has = has || m.Str(k.key) == slug
	}
	return t.addEntry(k, slug, c.After, has)
}
