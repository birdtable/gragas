package format

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// A step moves a document one format version forward: it answers the document
// as the next version writes it, or the reasons it cannot. Forward only: no
// step goes back (store.schemas.2).
type step func(doc *ordered.Map) (*ordered.Map, []string)

// migrations is the steps this engine has, by the version they move from. The
// previous major is read and moved forward by them. Format 3 is the first
// format that has an identity or a structure file, so there is no step from 2
// to write here: gragas migrate owns the move from the owner's schema
// v2, and a format-2 file is a not-ready finding that names it.
func migrations() map[int]step { return map[int]step{} }

// previous is the oldest format version this engine reads and migrates.
const previous = model.FormatVersion - 1

// readFile reads the file at path p below root, moves it forward to the
// current format and checks it against the schema named name.
func readFile(tree port.Tree, schema port.Schema, root model.Root, p, name string, steps map[int]step) (*ordered.Map, []Finding, error) {
	data, err := tree.Read(root, p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, []Finding{{File: p, Msg: "the file is missing"}}, nil
		}
		return nil, nil, fmt.Errorf("read %s: %w", p, err)
	}
	return parse(schema, p, data, name, steps)
}

// parse is readFile past the read. A format newer than the engine is an error
// of type *NotReady and nothing else of the file is read; everything else that
// is wrong is a finding with its line.
func parse(schema port.Schema, file string, data []byte, name string, steps map[int]step) (*ordered.Map, []Finding, error) {
	v, err := ordered.ParseYAML(data)
	if err != nil {
		return nil, []Finding{{File: file, Line: syntaxLine(err), Msg: "not readable YAML: " + detail(err)}}, nil
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return nil, []Finding{{File: file, Line: 1, Msg: "the file is not a mapping of fields"}}, nil
	}
	bad := repeatFindings(file, doc)
	if len(bad) > 0 {
		return nil, bad, nil
	}
	doc, bad, err = forward(doc, file, steps)
	if err != nil || len(bad) > 0 {
		return nil, bad, err
	}
	problems, err := schema.Check(name, []byte(ordered.JSON(plain(doc), -1)))
	if err != nil {
		return nil, nil, fmt.Errorf("check %s: %w", file, err)
	}
	for _, p := range problems {
		bad = append(bad, Finding{File: file, Line: lineOf(doc, p), Msg: p.Msg})
	}
	if len(bad) > 0 {
		return nil, bad, nil
	}
	return doc, nil, nil
}

// forward moves doc to the current format. The version is read before any
// schema is, since a newer file is described by a schema this engine does not
// have.
func forward(doc *ordered.Map, file string, steps map[int]step) (*ordered.Map, []Finding, error) {
	raw, present := doc.Get("format")
	if !present {
		return doc, nil, nil // the schema names the missing field
	}
	n, isInt := raw.(int64)
	if !isInt || n < 1 {
		return nil, []Finding{{File: file, Line: lineOfKey(doc, "format"), Msg: "format is the format version, a whole number from 1"}}, nil
	}
	if n > model.FormatVersion {
		return nil, nil, &NotReady{File: file, Found: int(n), Reads: model.FormatVersion}
	}
	v := int(n)
	if v < previous {
		return nil, []Finding{{File: file, Line: lineOfKey(doc, "format"),
			Msg: fmt.Sprintf("format %d is older than the previous major (%d); migrate it with an engine that reads it, then read it here", v, previous)}}, nil
	}
	for ; v < model.FormatVersion; v++ {
		s, ok := steps[v]
		if !ok {
			return nil, []Finding{{File: file, Line: lineOfKey(doc, "format"),
				Msg: fmt.Sprintf("not ready: format %d is moved forward by gragas migrate, which reads it from the owner's schema v2; run it, then read the result here", v)}}, nil
		}
		next, why := s(doc)
		if len(why) > 0 {
			bad := make([]Finding, len(why))
			for i, w := range why {
				bad[i] = Finding{File: file, Line: doc.Line, Msg: w}
			}
			return nil, bad, nil
		}
		doc = next
	}
	return doc, nil, nil
}

// lineOf is the source line of the place a schema problem names. The adapter
// writes "schema at <place>: <message>" with the place a slash path from the
// top; the line it carries is that of the JSON the engine handed it, so the
// line in the YAML is found again from the place.
func lineOf(doc *ordered.Map, p model.Problem) int {
	rest, ok := strings.CutPrefix(p.Msg, "schema at ")
	if !ok {
		return 0
	}
	place, _, _ := strings.Cut(rest, ": ")
	if place == "top" {
		return doc.Line
	}
	return ordered.LineAt(doc, strings.Split(place, "/"))
}

func lineOfKey(m *ordered.Map, k string) int {
	if l := m.KeyLine(k); l > 0 {
		return l
	}
	return m.Line
}

// syntaxLine is the line a YAML syntax error names ("line 3: ..."), 0 when it
// names none.
func syntaxLine(err error) int {
	_, rest, ok := strings.Cut(detail(err), "line ")
	if !ok {
		return 0
	}
	var n int
	if _, e := fmt.Sscanf(rest, "%d", &n); e != nil {
		return 0
	}
	return n
}

func detail(err error) string {
	if se, ok := errors.AsType[*ordered.SyntaxError](err); ok {
		return se.Detail
	}
	return err.Error()
}

// repeatFindings lists every key written twice in a mapping anywhere in v.
func repeatFindings(file string, v any) []Finding {
	var out []Finding
	switch x := v.(type) {
	case *ordered.Map:
		for _, r := range x.Repeats() {
			out = append(out, Finding{File: file, Line: r.Again, Msg: fmt.Sprintf("%s is written twice (first on line %d); keep one", r.Key, r.First)})
		}
		for _, k := range x.Keys() {
			out = append(out, repeatFindings(file, x.Val(k))...)
		}
	case []any:
		for _, e := range x {
			out = append(out, repeatFindings(file, e)...)
		}
	}
	return out
}

// plain copies v with each bare YAML date or time (a scalar YAML 1.1 reads as
// one) as the one-key object that no string, number or enum schema accepts, as
// ordered.Plain does for the validator in memory: the document goes to the
// Schema port as JSON text, which would otherwise turn it into a plain string
// and pass a date nobody quoted. The order and the lines of the keys are kept.
func plain(v any) any {
	switch x := v.(type) {
	case *ordered.Map:
		out := ordered.NewMap(x.Line)
		for _, k := range x.Keys() {
			out.Set(k, plain(x.Val(k)), x.KeyLine(k))
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = plain(e)
		}
		return out
	case ordered.Timestamp:
		o := ordered.NewMap(0)
		o.Set("$yaml", string(x), 0)
		return o
	}
	return v
}
