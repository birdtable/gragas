// Package jsonschema is the adapter of legacy.Schemas: rulebook documents
// checked against the JSON Schemas in rulebook/schema, through
// santhosh-tekuri/jsonschema, the one package that imports it. Nothing is
// fetched, and a schema is read from the repository.
package jsonschema

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// maxSchemaErrors is how many schema errors one file reports.
const maxSchemaErrors = 10

// maxMessage is the longest schema message kept, in characters.
const maxMessage = 200

// schemaBase is the URL the schemas are registered under. Nothing is fetched.
const schemaBase = "file:///rulebook/schema/"

// validator checks documents against one schema.
type validator struct {
	schema *jsonschema.Schema
	// raw is the schema as written, to order errors at one place by the
	// order of the keywords in the schema.
	raw any
}

// schemaFinding is one schema error at a place in the document.
type schemaFinding struct {
	path    []string
	rank    int
	seq     int
	message string
}

// Set is the compiled schemas, by file name. It fills legacy.Checker.
type Set struct {
	byName map[string]*validator
}

// Validate checks doc against the schema file named schema.
func (s *Set) Validate(schema string, doc any) []legacy.Problem {
	v, ok := s.byName[schema]
	if !ok {
		return []legacy.Problem{{Msg: "schema " + schema + " was not compiled"}}
	}
	return v.validate(doc)
}

// Schemas fills legacy.Schemas.
type Schemas struct{}

// Compile is the package's Compile, as the port asks for it: a schema
// that does not compile is an error and no checker.
func (Schemas) Compile(src legacy.Source, dir string, names ...string) (legacy.Checker, error) {
	s, err := Compile(src, dir, names...)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Compile reads each named schema from dir in src and compiles it. A schema
// may refer to another only by a name in the same list.
func Compile(src fs.FS, dir string, names ...string) (*Set, error) {
	files := make([]model.File, 0, len(names))
	for _, name := range names {
		data, err := fs.ReadFile(src, path.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", name, err)
		}
		files = append(files, model.File{Path: name, Data: data})
	}
	return compile(files)
}

// compile compiles the schemas in files, each named by its Path.
func compile(files []model.File) (*Set, error) {
	c := jsonschema.NewCompiler()
	raws := map[string]any{}
	names := make([]string, 0, len(files))
	for _, f := range files {
		name, data := f.Path, f.Data
		names = append(names, name)
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("parse schema %s: %w", name, err)
		}
		if err := c.AddResource(schemaBase+name, doc); err != nil {
			return nil, fmt.Errorf("add schema %s: %w", name, err)
		}
		raw, err := ordered.ParseYAML(data)
		if err != nil {
			return nil, fmt.Errorf("read schema %s for keyword order: %w", name, err)
		}
		raws[name] = raw
	}
	out := &Set{byName: map[string]*validator{}}
	for _, name := range names {
		s, err := c.Compile(schemaBase + name)
		if err != nil {
			return nil, fmt.Errorf("compile schema %s: %w", name, err)
		}
		out.byName[name] = &validator{schema: s, raw: raws[name]}
	}
	return out, nil
}

// validate returns the schema errors in doc, sorted by place, as
// "schema at <place>: <message>" with the source line of the place.
func (v *validator) validate(doc any) []legacy.Problem {
	err := v.schema.Validate(ordered.Plain(doc))
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []legacy.Problem{{Msg: "schema: " + err.Error()}}
	}
	var found []schemaFinding
	v.collect(ve, doc, &found)
	slices.SortStableFunc(found, func(a, b schemaFinding) int {
		if c := comparePath(a.path, b.path); c != 0 {
			return c
		}
		if c := cmp.Compare(a.rank, b.rank); c != 0 {
			return c
		}
		return cmp.Compare(a.seq, b.seq)
	})
	out := make([]legacy.Problem, 0, min(len(found), maxSchemaErrors))
	for _, f := range found[:min(len(found), maxSchemaErrors)] {
		place := strings.Join(f.path, "/")
		if place == "" {
			place = "top"
		}
		out = append(out, legacy.Problem{
			Line: ordered.LineAt(doc, f.path),
			Msg:  "schema at " + place + ": " + truncate(f.message, maxMessage),
		})
	}
	return out
}

func (v *validator) collect(e *jsonschema.ValidationError, doc any, out *[]schemaFinding) {
	if len(e.Causes) > 0 {
		for _, c := range e.Causes {
			v.collect(c, doc, out)
		}
		return
	}
	inst := valueAt(doc, e.InstanceLocation)
	node := v.schemaNode(e)
	for _, msg := range messages(e.ErrorKind, inst) {
		*out = append(*out, schemaFinding{path: e.InstanceLocation, rank: rank(node, e.ErrorKind.KeywordPath()), seq: len(*out), message: msg})
	}
	// The validator stops at a type error, where the rulebook has always
	// gone on to report an enum the value is not in as well.
	if _, isType := e.ErrorKind.(*kind.Type); isType && node != nil {
		if want, ok := node.Get("enum"); ok {
			if l, ok := want.([]any); ok && !slices.ContainsFunc(l, func(x any) bool { return ordered.Repr(x) == ordered.Repr(inst) }) {
				*out = append(*out, schemaFinding{path: e.InstanceLocation, rank: rank(node, []string{"enum"}), seq: len(*out), message: ordered.Repr(inst) + " is not one of " + ordered.Repr(l)})
			}
		}
	}
}

// schemaNode is the schema object, as written, that an error came from.
func (v *validator) schemaNode(e *jsonschema.ValidationError) *ordered.Map {
	_, frag, _ := strings.Cut(e.SchemaURL, "#")
	node := v.raw
	for seg := range strings.SplitSeq(strings.TrimPrefix(frag, "/"), "/") {
		if seg == "" {
			continue
		}
		if s, err := url.PathUnescape(seg); err == nil {
			seg = s
		}
		seg = strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
		m, ok := node.(*ordered.Map)
		if !ok {
			return nil
		}
		node, _ = m.Get(seg)
	}
	if m, ok := node.(*ordered.Map); ok {
		return m
	}
	return nil
}

// rank is the position of a keyword in its schema object, -1 when unknown.
func rank(node *ordered.Map, keywordPath []string) int {
	if node == nil || len(keywordPath) == 0 {
		return -1
	}
	return slices.Index(node.Keys(), keywordPath[0])
}

// messages words a schema error of kind k on the value inst as report lines.
func messages(k jsonschema.ErrorKind, inst any) []string {
	r := ordered.Repr(inst)
	switch k := k.(type) {
	case *kind.Required:
		out := make([]string, 0, len(k.Missing))
		for _, p := range k.Missing {
			out = append(out, ordered.Repr(p)+" is a required property")
		}
		return out
	case *kind.AdditionalProperties:
		props := slices.Sorted(slices.Values(k.Properties))
		verb := "were"
		if len(props) == 1 {
			verb = "was"
		}
		quoted := make([]string, len(props))
		for i, p := range props {
			quoted[i] = ordered.Repr(p)
		}
		return []string{fmt.Sprintf("Additional properties are not allowed (%s %s unexpected)", strings.Join(quoted, ", "), verb)}
	case *kind.Type:
		want := make([]string, len(k.Want))
		for i, w := range k.Want {
			want[i] = ordered.Repr(w)
		}
		return []string{r + " is not of type " + strings.Join(want, ", ")}
	case *kind.Enum:
		return []string{r + " is not one of " + ordered.Repr(k.Want)}
	case *kind.Pattern:
		return []string{r + " does not match " + ordered.Repr(k.Want)}
	case *kind.MinLength:
		return []string{r + " " + shortWord(k.Want)}
	case *kind.MinItems:
		return []string{r + " " + shortWord(k.Want)}
	case *kind.Minimum:
		return []string{r + " is less than the minimum of " + k.Want.RatString()}
	default:
		return []string{k.LocalizedString(message.NewPrinter(language.English))}
	}
}

func shortWord(want int) string {
	if want == 1 {
		return "should be non-empty"
	}
	return "is too short"
}

func valueAt(v any, p []string) any {
	for _, seg := range p {
		switch cur := v.(type) {
		case *ordered.Map:
			v, _ = cur.Get(seg)
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(cur) {
				return nil
			}
			v = cur[i]
		default:
			return nil
		}
	}
	return v
}

// comparePath orders places in a document: numbers as numbers, keys as text.
func comparePath(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		ai, aerr := strconv.Atoi(a[i])
		bi, berr := strconv.Atoi(b[i])
		if aerr == nil && berr == nil {
			if c := cmp.Compare(ai, bi); c != 0 {
				return c
			}
			continue
		}
		if c := strings.Compare(a[i], b[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a), len(b))
}

// truncate keeps the first n characters of s.
func truncate(s string, n int) string {
	i := 0
	for j := range s {
		if i == n {
			return s[:j]
		}
		i++
	}
	return s
}

// Schema is the adapter of port.Schema: the schemas main hands it, compiled
// once at start.
type Schema struct {
	set *Set
}

// New compiles the schemas in files, each named by its Path, such as
// "rules.schema.json". A schema may refer to another only by a name in the
// same list. One that does not compile is an error and no Schema.
func New(files []model.File) (*Schema, error) {
	set, err := compile(files)
	if err != nil {
		return nil, err
	}
	return &Schema{set: set}, nil
}

// Check validates doc, one JSON document, against the schema named name. It
// answers at most ten problems sorted by place, and nil when doc is valid.
func (s *Schema) Check(name string, doc []byte) ([]model.Problem, error) {
	if !json.Valid(doc) {
		return nil, fmt.Errorf("check against %s: the document is not JSON", name)
	}
	v, err := ordered.ParseYAML(doc)
	if err != nil {
		return nil, fmt.Errorf("check against %s: read the document: %w", name, err)
	}
	found := s.set.Validate(name, v)
	if len(found) == 0 {
		return nil, nil
	}
	out := make([]model.Problem, len(found))
	for i, p := range found {
		out[i] = model.Problem{Line: p.Line, Msg: p.Msg}
	}
	return out, nil
}
