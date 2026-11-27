package format

import (
	"errors"
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// recField is one key of a record: a text, or a list of texts.
type recField struct {
	key  string
	vals []string
	list bool
	// bare marks a value the file wrote as a boolean or a number, which is
	// written back bare: quoted it would read back as a text.
	bare []bool
	// sub is a nested mapping, one record of keys, as a binding's options are;
	// a field with sub holds no vals.
	sub []recField
}

// one is the value i as YAML.
func (f recField) one(i int) string {
	if i < len(f.bare) && f.bare[i] {
		return f.vals[i]
	}
	return yamlScalar(f.vals[i])
}

func isBare(v any) bool {
	switch v.(type) {
	case bool, int64, float64:
		return true
	}
	return false
}

// parseRecord reads the YAML mapping of a record to append. A key holds a
// single text or a list of texts; which keys a list of records may carry is
// the schema's to say, and is checked on the result.
func parseRecord(after string) ([]recField, error) {
	v, err := ordered.ParseYAML([]byte(after))
	if err != nil {
		return nil, fmt.Errorf("it is not YAML: %w", err)
	}
	m, ok := v.(*ordered.Map)
	if !ok || m.Len() == 0 {
		return nil, errors.New("it is not a mapping with a key")
	}
	var out []recField
	for _, k := range m.Keys() {
		f, err := recValue(k, m.Val(k))
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func recValue(key string, v any) (recField, error) {
	if m, ok := v.(*ordered.Map); ok {
		if m.Len() == 0 {
			return recField{}, fmt.Errorf("%s holds an empty mapping", key)
		}
		f := recField{key: key}
		for _, k := range m.Keys() {
			sub, err := recValue(k, m.Val(k))
			if err != nil {
				return recField{}, fmt.Errorf("%s: %w", key, err)
			}
			f.sub = append(f.sub, sub)
		}
		return f, nil
	}
	if l, ok := v.([]any); ok {
		f := recField{key: key, list: true}
		for _, x := range l {
			s, ok := scalarText(x)
			if !ok || s == "" {
				return recField{}, fmt.Errorf("%s holds an item that is not a text", key)
			}
			f.vals = append(f.vals, s)
			f.bare = append(f.bare, isBare(x))
		}
		if len(f.vals) == 0 {
			return recField{}, fmt.Errorf("%s holds an empty list", key)
		}
		return f, nil
	}
	s, ok := scalarText(v)
	if !ok || s == "" {
		return recField{}, fmt.Errorf("%s is not a text", key)
	}
	return recField{key: key, vals: []string{s}, bare: []bool{isBare(v)}}, nil
}

func (f recField) text() string {
	if f.sub != nil {
		return flowRecord(f.sub)
	}
	if !f.list {
		return f.one(0)
	}
	parts := make([]string, len(f.vals))
	for i := range f.vals {
		parts[i] = f.one(i)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// flow is the record on one line, as {key: value, ...}.
func flowRecord(r []recField) string {
	parts := make([]string, len(r))
	for i, f := range r {
		parts[i] = yamlScalar(f.key) + ": " + f.text()
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// blockRecord is the record as the lines of a list item: its dash at dash and
// its keys at key.
func blockRecord(r []recField, dash, key int) []string {
	var out []string
	for i, f := range r {
		pad := strings.Repeat(" ", key)
		if i == 0 {
			pad = strings.Repeat(" ", dash) + "-" + strings.Repeat(" ", key-dash-1)
		}
		out = append(out, fieldLines(f, pad, key)...)
	}
	return out
}

// fieldLines is the field as block lines: its key after pad, which stands at
// column col, and the keys of a nested mapping two columns in, each under it.
func fieldLines(f recField, pad string, col int) []string {
	if f.sub == nil {
		return []string{pad + yamlScalar(f.key) + ": " + f.text()}
	}
	out := []string{pad + yamlScalar(f.key) + ":"}
	for _, s := range f.sub {
		out = append(out, fieldLines(s, strings.Repeat(" ", col+2), col+2)...)
	}
	return out
}

// same says the parsed record m holds exactly the keys and texts of r.
func sameRecord(m *ordered.Map, r []recField) bool {
	if m.Len() != len(r) {
		return false
	}
	for _, f := range r {
		if f.sub != nil {
			if nested, ok := m.Val(f.key).(*ordered.Map); !ok || !sameRecord(nested, f.sub) {
				return false
			}
			continue
		}
		got, err := recValue(f.key, m.Val(f.key))
		if err != nil || got.sub != nil || got.list != f.list || strings.Join(got.vals, "\x00") != strings.Join(f.vals, "\x00") {
			return false
		}
	}
	return true
}

// appendRecord adds the record, a YAML mapping, to the list of records at
// field of the rule, flow or block style as the file has it. A record the list
// holds already is refused; a key the schema does not name is refused when the
// result is checked against it.
func (t *text) appendRecord(id, field, after string) error {
	it, err := t.ruleItem(id)
	if err != nil {
		return err
	}
	rec, err := parseRecord(after)
	if err != nil {
		return fmt.Errorf("the new record of %s in %s: %w", field, id, err)
	}
	path := strings.Split(field, ".")
	cur, present := valueAt(it.m, path)
	line := max(ordered.LineAt(it.m, path), it.m.Line)
	list, isList := cur.([]any)
	if present && cur != nil && !isList {
		return unsupported(t.file, line, field+" is not a list")
	}
	for _, m := range ordered.Maps(list) {
		if sameRecord(m, rec) {
			return moved(t.file, line, "%s of %s already holds %s", field, id, flowRecord(rec))
		}
	}
	sp := t.locate(it, path)
	if sp.found {
		return t.appendTo(sp, path[len(path)-1], flowRecord(rec), func(dash, key int) []string { return blockRecord(rec, dash, key) })
	}
	if present {
		return unsupported(t.file, line, field+" is written in flow style")
	}
	var out []string
	for k, seg := range path[sp.depth:] {
		out = append(out, strings.Repeat(" ", sp.childInd+2*k)+seg+":")
	}
	dash := sp.childInd + 2*(len(path)-sp.depth)
	t.splice(sp.at, sp.at, append(out, blockRecord(rec, dash, dash+2)...))
	return nil
}

// recordKeyColumn is the column the keys of a list item stand at: the one the
// first item of the list uses, or two past the dash.
func (t *text) recordKeyColumn(from, end, dash int) int {
	for i := from; i < end; i++ {
		if mm := dashLine.FindStringSubmatch(t.lines[i]); mm != nil && len(mm[1]) == dash {
			return dash + 1 + len(mm[2])
		}
	}
	return dash + 2
}
