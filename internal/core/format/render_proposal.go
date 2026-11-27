package format

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// ErrNoSuchMove is a proposal's state change the lifecycle does not give: a
// proposal moves once, from open, to accepted, declined or withdrawn.
var ErrNoSuchMove = errors.New("a proposal moves once, from open to accepted, declined or withdrawn")

// slugForm is common.schema.json's slug: a proposal's key names its file, so
// it is held to the form before it is joined to a path.
var slugForm = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// proposalFile is the file of the proposal slug, or an error naming a key that
// is not a slug.
func proposalFile(l Loaded, slug string) (string, error) {
	if !slugForm.MatchString(slug) {
		return "", fmt.Errorf("%w: the proposal %q is not a slug: lower-case letters and digits, in words joined by one dash", ErrUnsupported, slug)
	}
	return join(l.Dir, ProposalsDir+"/"+slug+".yaml"), nil
}

// renderProposal changes the file of one proposal (proposal:<slug>):
//
//   - with a Field of state or link, the scalar in the file and its twin in
//     document.proposal go from Before to After together, and every other
//     byte is left as it was. A state change CanBecome does not give is
//     refused with ErrNoSuchMove. The rest of a proposal is written once,
//     when it is opened.
//   - with no Field and Before "", After is the new proposal's YAML, written
//     whole: its file is not there yet, and its state is open.
//
// The result is read back with the proposals schema, and a file whose
// document says other than the file is refused.
func renderProposal(schema port.Schema, file, slug string, old []byte, exists bool, changes []model.Change) ([]byte, error) {
	for _, c := range changes {
		if c.Entity != "proposal:"+slug {
			return nil, fmt.Errorf("the file %s is the proposal %s's alone; the delta also names it as %s", file, slug, c.Entity)
		}
	}
	next, err := proposalBytes(file, slug, old, exists, changes)
	if err != nil {
		return nil, err
	}
	if err := readBack(schema, file, slug, next, !exists); err != nil {
		return nil, err
	}
	return next, nil
}

// proposalBytes is the file after the changes: the new proposal's YAML, or
// the file with each move made.
func proposalBytes(file, slug string, old []byte, exists bool, changes []model.Change) ([]byte, error) {
	if c := changes[0]; c.Field == "" && c.Before == "" {
		if len(changes) != 1 {
			return nil, fmt.Errorf("the proposal %s is written whole, once; the delta names it %d times", slug, len(changes))
		}
		if exists {
			return nil, moved(file, 1, "the proposal %s is in the tree and the plan adds it", slug)
		}
		return []byte(c.After), nil
	}
	if !exists {
		return nil, moved(file, 1, "the proposal %s is not in the tree", slug)
	}
	t, err := newText(file, old)
	if err != nil {
		return nil, err
	}
	for _, c := range changes {
		if err := t.moveProposal(slug, c); err != nil {
			return nil, err
		}
	}
	return t.bytes(), nil
}

// readBack refuses a proposal file the schema refuses, whose document says
// other than the file, whose slug does not name the file, or, when it is
// new, that is not open.
func readBack(schema port.Schema, file, slug string, data []byte, isNew bool) error {
	doc, bad, err := parse(schema, file, data, proposalsSchema, migrations())
	if err != nil {
		return err
	}
	if len(bad) > 0 || doc == nil {
		return fmt.Errorf("the edit would leave %s invalid: %s", file, append(bad, Finding{File: file, Line: 1, Msg: "the file is no proposal"})[0])
	}
	bad = disagreements(file, doc)
	if doc.Str("slug") != slug {
		bad = append(bad, Finding{File: file, Line: lineOfKey(doc, "slug"), Msg: "slug is " + doc.Str("slug") + " and the file is named " + slug})
	}
	if isNew && doc.Str("state") != string(model.ProposalOpen) {
		bad = append(bad, Finding{File: file, Line: lineOfKey(doc, "state"), Msg: fmt.Sprintf("a new proposal is open, and this one is %q", doc.Str("state"))})
	}
	if len(bad) > 0 {
		return fmt.Errorf("the edit would leave %s invalid: %s", file, bad[0])
	}
	return nil
}

// moveProposal makes one state or link change, in the file and in its
// document.
func (t *text) moveProposal(slug string, c model.Change) error {
	if c.Field == "" {
		return moved(t.file, 1, "the proposal %s is in the tree and the plan adds it", slug)
	}
	if c.Field != "state" && c.Field != "link" {
		return unsupported(t.file, 1, fmt.Sprintf("Render moves a proposal's state or link; %s is written once, when the proposal is opened", c.Field))
	}
	if c.Field == "state" && !model.ProposalState(c.Before).CanBecome(model.ProposalState(c.After)) {
		return fmt.Errorf("%w: the proposal %s cannot go from %q to %q", ErrNoSuchMove, slug, c.Before, c.After)
	}
	for _, path := range [][]string{{c.Field}, {"document", "proposal", c.Field}} {
		it, err := t.whole()
		if err != nil {
			return err
		}
		if err := t.setIn(it, "the proposal "+slug, path, c.Before, c.After); err != nil {
			return err
		}
	}
	return nil
}

// whole is the file as one item: its top-level keys at column 0, through its
// last line that is not blank.
func (t *text) whole() (ruleItem, error) {
	doc, err := t.doc()
	if err != nil {
		return ruleItem{}, err
	}
	end := len(t.lines)
	for end > 0 && blank(t.lines[end-1]) {
		end--
	}
	return ruleItem{m: doc, start: 0, end: end}, nil
}
