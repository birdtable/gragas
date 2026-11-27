package format

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/birdtable/gragas/internal/canon"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Site is where an entity is written.
type Site struct {
	File string
	Line int
}

// The kinds of change Render applies. A change with an entity kind not here
// is refused, naming the kind.
//
//   - rule:<id> with a Field: the scalar at the field, a dotted path such as
//     status or reasoning.definition, goes from Before to After. A field that
//     is not there has Before ""; one to be taken out has After "".
//   - rule:<id> with a Field ending in []: After is added to that list, as in
//     aliases[]. Before is "".
//   - rule:<id> with a Field ending in {}: After is a record, as the YAML
//     mapping of its keys (a text, or a list of texts, for each), added to that
//     list of records, as in sources{}. Before is "". A record the list holds
//     already, and one with a key the schema does not name, are refused.
//     With Before the record and After "", the record is taken out of the
//     list, and a list left empty goes with its key; a record the list does
//     not hold is refused with ErrMoved.
//   - rule:<id> with no Field, Before "": After is the new rule as the YAML
//     mapping of its fields, added to its concern file.
//   - proposal:<slug> with a Field of state or link: the scalar in
//     proposals/<slug>.yaml and its twin in document.proposal go from Before
//     to After together; a state change the lifecycle does not give is
//     refused with ErrNoSuchMove. With no Field and Before "", After is the
//     new proposal's YAML, written whole (renderProposal).
//   - exclusion:<overlay>/<rule>/<number>: a record of exclusions.yaml is
//     added, or marked lifted (renderExclusions).
//   - record:<path>: the bindings/ record at path, below the book's folder,
//     is written whole (S34, S70). Before is the record's text now ("" for
//     none) and After its whole new text; a record whose text is not Before
//     is refused with ErrMoved. A path outside bindings/ is refused.
//   - file:<path>: the generated file at path is written whole. Before is the
//     digest of the file now ("" for none) and After the JSON it holds, which
//     is written as canonical JSON.
//
// A scalar After is written as text; a number or a boolean is therefore
// written quoted, and the fields the engine sets today are all strings.

// Render turns a delta into the files it changes. It reads each file through
// the Tree, changes the lines the delta owns and leaves every other byte as it
// was, comments and order included. It returns one Target for each file,
// holding the whole new content and the digest of the bytes it replaces. A
// delta whose before value is not what the file holds is refused with
// ErrMoved naming the file and line; an empty delta returns no target.
func Render(tree port.Tree, schema port.Schema, root model.Root, l Loaded, d model.Delta) ([]model.Target, error) {
	files, order, err := plan(l, d)
	if err != nil {
		return nil, err
	}
	var out []model.Target
	for _, f := range order {
		old, exists, err := readOptional(tree, root, f)
		if err != nil {
			return nil, err
		}
		var next []byte
		kind, _, _ := strings.Cut(files[f][0].Entity, ":")
		efile, eschema, ek, isEntry := entryFile(l, kind)
		switch {
		case strings.HasPrefix(files[f][0].Entity, "file:"):
			next, err = renderWhole(f, old, exists, files[f])
		case strings.HasPrefix(files[f][0].Entity, "record:"):
			next, err = renderRecord(f, old, files[f])
		case strings.HasPrefix(files[f][0].Entity, "exclusion:"):
			next, err = renderExclusions(schema, f, old, exists, files[f])
		case strings.HasPrefix(files[f][0].Entity, "proposal:"):
			next, err = renderProposal(schema, f, strings.TrimPrefix(files[f][0].Entity, "proposal:"), old, exists, files[f])
		case isEntry:
			next, err = renderEntries(schema, entryTarget{efile, eschema, ek}, old, exists, files[f])
		case path.Ext(f) == ".md" && len(clashPins(l, files[f])) > 0:
			err = unsupported(f, 1, "a rule written in a guide that took an id another rule holds is renumbered by hand: give it an id of its own in the guide")
		case path.Ext(f) == ".md":
			next, err = renderGuide(schema, f, old, exists, files[f])
		default:
			next, err = renderRules(schema, f, old, exists, l, files[f])
		}
		if err != nil {
			return nil, err
		}
		t := model.Target{Path: f, Data: next}
		if exists {
			t.Before = digestOf(old)
		}
		out = append(out, t)
	}
	return out, nil
}

func readOptional(tree port.Tree, root model.Root, file string) ([]byte, bool, error) {
	data, err := tree.Read(root, file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

// plan groups the changes by the file they land in, in the order a file is
// first named.
func plan(l Loaded, d model.Delta) (map[string][]model.Change, []string, error) {
	files := map[string][]model.Change{}
	var order []string
	table := NewTable(l.Book.Rules)
	pins := clashPins(l, d.Changes)
	for _, c := range d.Changes {
		kind, key, _ := strings.Cut(c.Entity, ":")
		var file string
		switch kind {
		case "file":
			file = key
		case "rule":
			f, err := ruleChangeFile(l, table, pins, &c, key)
			if err != nil {
				return nil, nil, err
			}
			file = f
		case "source", "decision", "term":
			file, _, _, _ = entryFile(l, kind)
		case "exclusion":
			file = join(l.Dir, ExclusionsFile)
		case "record":
			if !recordPath(key) {
				return nil, nil, fmt.Errorf("%w: the record %q is not a file of %s/, named <name>.yaml", ErrUnsupported, key, BindingsDir)
			}
			file = join(l.Dir, key)
		case "block", "prose":
			file, _, _ = strings.Cut(key, "#")
		case "proposal":
			f, err := proposalFile(l, key)
			if err != nil {
				return nil, nil, err
			}
			file = f
		default:
			return nil, nil, fmt.Errorf("%w: the entity kind %q is not rendered", ErrUnsupported, kind)
		}
		if _, seen := files[file]; !seen {
			order = append(order, file)
		}
		files[file] = append(files[file], c)
	}
	return files, order, nil
}

// clashPins are the rules a delta renumbers that the book holds twice: for
// each id a change of the delta sets (the field id) while the load put a later
// rule with that id aside, the place of the later rule. Every change of the
// delta to that id lands there, on that line, and not on the rule that kept
// the id. The place is read from the load and is written nowhere.
// ruleChangeFile is the file the change c of the rule key lands in: the
// rule's own file by its site, a clash's pinned file, or, for a new rule,
// the rules file of its concern. An alias in key is resolved, and c then
// names the rule by its id.
func ruleChangeFile(l Loaded, table *Table, pins map[string]Site, c *model.Change, key string) (string, error) {
	if r, ok := table.Resolve(model.RuleID(key)); ok {
		c.Entity = "rule:" + string(r.ID)
	}
	if pin, pinned := pins[key]; pinned {
		return pin.File, nil
	}
	if site, known := l.Sites[c.Entity]; known {
		return site.File, nil
	}
	if c.Field == "" && c.Before == "" {
		return rulesPath(l, key)
	}
	return "", moved(IdentityFile, 1, "the rule %s is not in the book", key)
}

func clashPins(l Loaded, changes []model.Change) map[string]Site {
	var pins map[string]Site
	for _, c := range changes {
		kind, key, _ := strings.Cut(c.Entity, ":")
		if kind != "rule" || c.Field != "id" {
			continue
		}
		if s, ok := l.Clashed[model.RuleID(key)]; ok {
			if pins == nil {
				pins = map[string]Site{}
			}
			pins[key] = s
		}
	}
	return pins
}

// rulesPath is the concern file a new rule belongs in: its id names the part
// and the concern.
func rulesPath(l Loaded, id string) (string, error) {
	part, rest, _ := strings.Cut(id, ".")
	concern, _, _ := strings.Cut(rest, ".")
	for _, ly := range l.Book.Structure.Layers {
		for _, p := range ly.Parts {
			if p.Slug != part {
				continue
			}
			for _, c := range p.Concerns {
				if c.Slug == concern {
					return join(l.Dir, ly.Slug+"/"+p.Slug+"/rules/"+c.Slug+".yaml"), nil
				}
			}
		}
	}
	return "", fmt.Errorf("the rule %s names part %q and concern %q, which structure.yaml does not hold", id, part, concern)
}

// renderRules applies the changes to one concern file and checks the result
// still passes the rules schema.
func renderRules(schema port.Schema, file string, old []byte, exists bool, l Loaded, changes []model.Change) ([]byte, error) {
	if !exists {
		old = []byte(newConcernFile(l, file))
	}
	t, err := newText(file, old)
	if err != nil {
		return nil, err
	}
	for id, s := range clashPins(l, changes) {
		if s.File == file {
			if t.pin == nil {
				t.pin = map[string]int{}
			}
			t.pin[id] = s.Line
		}
	}
	for _, c := range changes {
		if err := t.apply(c); err != nil {
			return nil, err
		}
	}
	next := t.bytes()
	if _, bad, err := parse(schema, file, next, rulesSchema, migrations()); err != nil || len(bad) > 0 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("the edit would leave %s invalid: %s", file, bad[0])
	}
	return next, nil
}

// apply makes one change to the rules in the text.
func (t *text) apply(c model.Change) error {
	_, id, _ := strings.Cut(c.Entity, ":")
	switch {
	case c.Field == "" && c.Before == "":
		return t.addRule(id, c.After)
	case strings.HasSuffix(c.Field, "[]"):
		if c.Before != "" {
			return moved(t.file, 1, "a list item is added with no before value, and %s has %q", c.Field, c.Before)
		}
		return t.appendItem(id, strings.TrimSuffix(c.Field, "[]"), c.After)
	case strings.HasSuffix(c.Field, "{}"):
		switch {
		case c.Before != "" && c.After != "":
			return t.replaceRecord(id, strings.TrimSuffix(c.Field, "{}"), c.Before, c.After)
		case c.Before != "":
			return t.removeRecord(id, strings.TrimSuffix(c.Field, "{}"), c.Before)
		}
		return t.appendRecord(id, strings.TrimSuffix(c.Field, "{}"), c.After)
	case c.Field == "":
		return moved(t.file, 1, "the rule %s is in the file and the plan adds it", id)
	}
	return t.setField(id, c.Field, c.Before, c.After)
}

// newConcernFile is the head of a concern file that does not exist yet.
func newConcernFile(l Loaded, file string) string {
	parts := strings.Split(strings.TrimPrefix(file, join(l.Dir, "")), "/")
	part, concern := parts[len(parts)-3], strings.TrimSuffix(parts[len(parts)-1], ".yaml")
	return fmt.Sprintf("format: %d\npart: %s\nconcern: %s\nrules:\n", model.FormatVersion, part, concern)
}

// renderWhole writes a generated file whole, as canonical JSON.
func renderWhole(file string, old []byte, exists bool, changes []model.Change) ([]byte, error) {
	if len(changes) != 1 {
		return nil, fmt.Errorf("the file %s is written whole, once; the delta names it %d times", file, len(changes))
	}
	c := changes[0]
	have := model.Digest("")
	if exists {
		have = digestOf(old)
	}
	if string(have) != c.Before {
		return nil, moved(file, 1, "the file is %q now and the plan expected %q", have, c.Before)
	}
	v, err := canon.Decode([]byte(c.After))
	if err != nil {
		return nil, fmt.Errorf("the content of %s is not JSON: %w", file, err)
	}
	data, err := canon.Encode(v, canon.Canonical)
	if err != nil {
		return nil, fmt.Errorf("the content of %s cannot be written canonically: %w", file, err)
	}
	if bytes.Equal(data, old) && exists {
		return old, nil
	}
	return data, nil
}

// recordForm is a bindings/ record's path below the book's folder: one file
// of the folder, never below it, its name a slug.
var recordForm = regexp.MustCompile(`^` + BindingsDir + `/[a-z0-9]+(?:-[a-z0-9]+)*\.yaml$`)

func recordPath(p string) bool { return recordForm.MatchString(p) }

// renderRecord writes a bindings/ record whole: the text it has now is the
// change's Before, else the record moved since the plan was made.
func renderRecord(file string, old []byte, changes []model.Change) ([]byte, error) {
	if len(changes) != 1 {
		return nil, fmt.Errorf("the record %s is written whole, once; the delta names it %d times", file, len(changes))
	}
	c := changes[0]
	if string(old) != c.Before {
		return nil, moved(file, 1, "the record is not what the plan read; plan again")
	}
	if c.After == "" {
		return nil, unsupported(file, 1, "a record is written, never taken out, by a plan")
	}
	return []byte(c.After), nil
}

// RuleFile is the file the rule with id is written in: the file it was read
// from, or, for an id the book does not hold, the file its concern is written
// in: a guide when another rule of the concern stands in one, else the
// concern file. An id whose part or concern the structure lacks is an error.
func RuleFile(l Loaded, id string) (string, error) {
	if site, ok := l.Sites["rule:"+id]; ok {
		return site.File, nil
	}
	part, rest, _ := strings.Cut(id, ".")
	concern, _, _ := strings.Cut(rest, ".")
	for _, r := range l.Book.Rules {
		if p, c := ruleConcern(r.ID); p == part && c == concern {
			if s, ok := l.Sites["rule:"+string(r.ID)]; ok && path.Ext(s.File) == ".md" {
				return s.File, nil
			}
		}
	}
	return rulesPath(l, id)
}

func ruleConcern(id model.RuleID) (part, concern string) {
	part, rest, _ := strings.Cut(string(id), ".")
	concern, _, _ = strings.Cut(rest, ".")
	return part, concern
}
