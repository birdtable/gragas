// Package link is the enforcement link's context: how the book's bindings
// reach the tools. It settles the entries each plugin is given to write a
// tool's preset (Shippable), names the mark a generated preset carries, and
// reads the two documents of the link that come from outside: a checker's
// declaration and a binding record. Nothing in this package names a tool: a
// tool's own reading is its plugin's.
//
// A document read here comes from a rulebook, a project or a plugin, which the
// engine does not control (docs/architecture.md section 4.3), so the reader
// fails closed: a document is read whole or not at all, every key it does not
// name is refused, every value must fit its shape, and size is capped before
// anything is parsed. The YAML is read by ordered.ParseStrict, so an anchor,
// an alias or a tag is refused before it can expand. The document is held to
// the Schema port as well, so the Go reader and the schema each refuse what
// the other would miss. Each refusal names the file, the line and the check
// that refused it. The reader reads no file: its caller hands it the bytes.
package link

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// The caps. A document over MaxDocument bytes is refused unread.
const (
	MaxDocument = 64 << 10
	maxText     = 256
	maxRefusals = 20
)

// Check names the check that refused a document. The set is closed.
type Check string

// The checks.
const (
	// CheckSize is a document over its cap.
	CheckSize Check = "size"
	// CheckPlace is a file that does not stand where its kind stands, or
	// whose name differs from its place.
	CheckPlace Check = "place"
	// CheckYAML is a document that is not plain YAML: unreadable, or holding
	// an anchor, an alias, a tag or a key written twice.
	CheckYAML Check = "yaml"
	// CheckFormat is a document of another format version.
	CheckFormat Check = "format"
	// CheckKey is a key the document's shape does not name.
	CheckKey Check = "key"
	// CheckConditional is a key that would make a value depend on something.
	CheckConditional Check = "conditional"
	// CheckReference is a key that would make one place stand for another.
	CheckReference Check = "reference"
	// CheckExpression is a value holding the characters an expression is
	// written with.
	CheckExpression Check = "expression"
	// CheckShape is a value of the wrong type, outside its set, or not in
	// its form.
	CheckShape Check = "shape"
	// CheckSchema is a document the Schema port refuses.
	CheckSchema Check = "schema"
)

// Refusal is one reason a document is not read: the file, slash separated
// from the root its caller reads from, the line (0 when the whole file is
// meant), the check and a sentence for a person.
type Refusal struct {
	Path  string
	Line  int
	Check Check
	Msg   string
}

// String is the refusal as a person reads it: path:line: check: sentence.
func (r Refusal) String() string {
	if r.Line > 0 {
		return fmt.Sprintf("%s:%d: %s: %s", r.Path, r.Line, r.Check, r.Msg)
	}
	return fmt.Sprintf("%s: %s: %s", r.Path, r.Check, r.Msg)
}

// yaml refuses a document ParseStrict could not read, at the line it names.
func (r *reader) yaml(err error) {
	var se *ordered.SyntaxError
	if !errors.As(err, &se) {
		r.refuse(0, CheckYAML, "the document could not be read: %v", err)
		return
	}
	r.refuse(se.Line, CheckYAML, "%s", strings.TrimPrefix(se.Detail, fmt.Sprintf("line %d: ", se.Line)))
}

// reader gathers the refusals of one document. kind names the document in
// its refusals ("a checker declaration"), and kinds names its kind in the
// plural.
type reader struct {
	path        string
	out         []Refusal
	kind, kinds string
}

// an is the document as a refusal names it.
func (r *reader) an() string { return r.kind }

// many is the document's kind in the plural.
func (r *reader) many() string { return r.kinds }

// refuse records a refusal, up to the cap.
func (r *reader) refuse(line int, c Check, format string, args ...any) {
	if len(r.out) >= maxRefusals {
		return
	}
	r.out = append(r.out, Refusal{Path: r.path, Line: line, Check: c, Msg: fmt.Sprintf(format, args...)})
}

// schema holds the document to its kind's schema, name, through the port. The Go
// reader has passed it, so a problem here is a drift between the two, and it
// refuses the document all the same. Each problem's line, which is the
// line of the JSON the port was handed, is mapped back to the YAML's.
func (r *reader) schema(s port.Schema, name string, v any) {
	doc := ordered.JSON(v, 1)
	problems, err := s.Check(name, []byte(doc))
	if err != nil {
		r.refuse(0, CheckSchema, "the schema could not check it: %v", err)
		return
	}
	if len(problems) == 0 {
		return
	}
	back := lineMap(v, doc)
	for _, p := range problems {
		r.refuse(back[p.Line], CheckSchema, "%s", p.Msg)
	}
}

// lineMap maps each line of doc, the JSON encoding of v, that holds a value
// of v to the line of that value in v's YAML.
func lineMap(v any, doc string) map[int]int {
	out := map[int]int{}
	j, err := ordered.ParseYAML([]byte(doc))
	if err != nil {
		return out
	}
	var walk func(x any, at []string)
	walk = func(x any, at []string) {
		out[ordered.LineAt(j, at)] = ordered.LineAt(v, at)
		switch n := x.(type) {
		case *ordered.Map:
			for _, k := range n.Keys() {
				walk(n.Val(k), append(slices.Clip(at), k))
			}
		case []any:
			for i, e := range n {
				walk(e, append(slices.Clip(at), fmt.Sprint(i)))
			}
		}
	}
	walk(v, nil)
	return out
}
