package format

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// ruleItem is one rule of a concern file: the lines it takes and the columns
// of its dash and of its keys.
type ruleItem struct {
	m          *ordered.Map
	start, end int
	dash, key  int
}

var dashLine = regexp.MustCompile(`^( *)-( +)\S`)

// ruleItem finds the rule with id in the file as it is now.
func (t *text) ruleItem(id string) (ruleItem, error) {
	return t.entryItem(entryKind{what: "rule", list: "rules", key: "id"}, id)
}

// entryItem finds the entry of kind k keyed id in the file as it is now: the
// one at the line a pin names when id is pinned, else the one keyed id.
func (t *text) entryItem(k entryKind, id string) (ruleItem, error) {
	line, pinned := t.pin[id]
	return t.listItem(k, id, `"- `+k.key+`:"`, func(m *ordered.Map) bool {
		if pinned {
			return m.Line == line
		}
		return m.Str(k.key) == id
	})
}

// listItem finds the first item of k's list that match holds for, in the
// file as it is now; key names it in a message, and first is how its first
// line starts.
func (t *text) listItem(k entryKind, key, first string, match func(*ordered.Map) bool) (ruleItem, error) {
	doc, err := t.doc()
	if err != nil {
		return ruleItem{}, err
	}
	for _, rm := range ordered.Maps(doc.List(k.list)) {
		if !match(rm) {
			continue
		}
		start := rm.Line - 1
		if start < 0 || start >= len(t.lines) {
			return ruleItem{}, unsupported(t.file, max(len(t.lines), 1), "the parser read a line break the file does not split at (a bare carriage return); make the line endings LF or CRLF and run again")
		}
		mm := dashLine.FindStringSubmatch(t.lines[start])
		if mm == nil {
			return ruleItem{}, unsupported(t.file, rm.Line, "the "+k.what+" does not start with "+first+" on one line")
		}
		dash := len(mm[1])
		last := start
		for j := start + 1; j < len(t.lines); j++ {
			if blank(t.lines[j]) {
				continue
			}
			if indentOf(t.lines[j]) <= dash {
				break
			}
			last = j
		}
		return ruleItem{m: rm, start: start, end: last + 1, dash: dash, key: dash + 1 + len(mm[2])}, nil
	}
	return ruleItem{}, moved(t.file, 1, "the %s %s is not in the file", k.what, key)
}

// view is line i with the dash of a rule's first line blanked, so a key is
// found at its column on every line of the rule alike.
func (t *text) view(it ruleItem, i int) string {
	if i == it.start {
		return strings.Repeat(" ", it.key) + t.lines[i][it.key:]
	}
	return t.lines[i]
}

// spot is where a path of keys is in a rule, or where it would go.
type spot struct {
	found    bool
	idx, ind int // the key's line and column, when found
	depth    int // how many segments of the path exist, when not
	at       int // the line a missing key is added at
	childInd int // the column the missing key is written at
}

// locate walks path inside the rule. Not found, it says how far the path
// exists and where the rest goes.
func (t *text) locate(it ruleItem, path []string) spot {
	lo, hi, ind := it.start, it.end, it.key
	for depth, seg := range path {
		idx := -1
		for i := lo; i < hi; i++ {
			s := t.view(it, i)
			if indentOf(s) == ind && strings.HasPrefix(s[ind:], seg+":") && (len(s) == ind+len(seg)+1 || s[ind+len(seg)+1] == ' ') {
				idx = i
				break
			}
		}
		if idx < 0 {
			return spot{depth: depth, at: hi, childInd: ind}
		}
		if depth == len(path)-1 {
			return spot{found: true, idx: idx, ind: ind, depth: depth}
		}
		lo, hi = idx+1, t.valueEnd(idx, ind)
		ind = t.childIndent(lo, hi, ind)
	}
	return spot{}
}

func (t *text) childIndent(lo, hi, parent int) int {
	for i := lo; i < hi; i++ {
		if !blank(t.lines[i]) && !comment(t.lines[i]) {
			return indentOf(t.lines[i])
		}
	}
	return parent + 2
}

// valueAt reads path from a parsed rule.
func valueAt(m *ordered.Map, path []string) (any, bool) {
	for _, seg := range path[:len(path)-1] {
		if m = m.Sub(seg); m == nil {
			return nil, false
		}
	}
	return m.Get(path[len(path)-1])
}

// setField makes the scalar at field the after value, where before is what the
// file holds now; "" for a field that is not there, or no longer to be. The
// bytes outside the field's own lines are not touched.
func (t *text) setField(id, field, before, after string) error {
	it, err := t.ruleItem(id)
	if err != nil {
		return err
	}
	path := strings.Split(field, ".")
	if numeric(path[0]) {
		return unsupported(t.file, t.lineOf(id), field+" is a number or a list of records in the schema; Render sets text only")
	}
	return t.setIn(it, id, path, before, after)
}

// setEntryField is setField for an entry of kind k: a term's literal or status.
func (t *text) setEntryField(k entryKind, id, field, before, after string) error {
	it, err := t.entryItem(k, id)
	if err != nil {
		return err
	}
	return t.setIn(it, id, strings.Split(field, "."), before, after)
}

// setIn is setField inside the item it, which a message calls name.
func (t *text) setIn(it ruleItem, name string, path []string, before, after string) error {
	field := strings.Join(path, ".")
	cur, present := valueAt(it.m, path)
	now, scalar := scalarText(cur)
	line := max(ordered.LineAt(it.m, path), it.m.Line)
	if present && !scalar {
		return unsupported(t.file, line, field+" is not a single value")
	}
	if now != before {
		return moved(t.file, line, "%s of %s is %q now and the plan expected %q", field, name, now, before)
	}
	if before == after {
		return nil
	}
	sp := t.locate(it, path)
	switch {
	case present && !sp.found:
		return unsupported(t.file, line, field+" is written in flow style")
	case sp.found && after == "":
		t.splice(sp.idx, t.valueEnd(sp.idx, sp.ind), nil)
	case sp.found:
		return t.replaceValue(sp, path[len(path)-1], after)
	default:
		t.insertChain(sp, path, []string{}, after)
	}
	return nil
}

// replaceValue puts a new value on the key at sp, keeping the key's own text,
// the spacing after the colon and the comment at the end of its line.
func (t *text) replaceValue(sp spot, key, after string) error {
	line := t.lines[sp.idx]
	cut := sp.ind + len(key) + 1
	prefix, rest := line[:cut], line[cut:]
	val := strings.TrimLeft(rest, " ")
	space := rest[:len(rest)-len(val)]
	if space == "" {
		space = " "
	}
	value, tail := splitComment(val)
	if strings.HasPrefix(value, "&") || strings.HasPrefix(value, "*") || strings.HasPrefix(value, "!") {
		return unsupported(t.file, sp.idx+1, "the value uses an anchor, alias or tag; edit it by hand")
	}
	vl := valueLines(after, sp.ind)
	t.splice(sp.idx, t.valueEnd(sp.idx, sp.ind), append([]string{prefix + space + vl[0] + tail}, vl[1:]...))
	return nil
}

// insertChain adds the missing keys of path below the part that exists, the
// last with either the scalar after or the list items.
func (t *text) insertChain(sp spot, path, items []string, after string) {
	var out []string
	for k, seg := range path[sp.depth:] {
		pad := strings.Repeat(" ", sp.childInd+2*k)
		if k < len(path[sp.depth:])-1 {
			out = append(out, pad+seg+":")
			continue
		}
		if len(items) == 0 {
			vl := valueLines(after, sp.childInd+2*k)
			out = append(append(out, pad+seg+": "+vl[0]), vl[1:]...)
			continue
		}
		out = append(out, pad+seg+":")
		for _, it := range items {
			out = append(out, pad+"  - "+yamlScalar(it))
		}
	}
	t.splice(sp.at, sp.at, out)
}

// appendItem adds item to the list at field of the rule, flow or block style
// as the file has it.
func (t *text) appendItem(id, field, item string) error {
	it, err := t.ruleItem(id)
	if err != nil {
		return err
	}
	path := strings.Split(field, ".")
	cur, present := valueAt(it.m, path)
	line := max(ordered.LineAt(it.m, path), it.m.Line)
	list, isList := cur.([]any)
	if present && cur != nil && !isList {
		return unsupported(t.file, line, field+" is not a list")
	}
	if slices.Contains(ordered.Strings(list), item) {
		return moved(t.file, line, "%s of %s already holds %q", field, id, item)
	}
	sp := t.locate(it, path)
	if !sp.found {
		if present {
			return unsupported(t.file, line, field+" is written in flow style")
		}
		t.insertChain(sp, path, []string{item}, "")
		return nil
	}
	return t.appendTo(sp, path[len(path)-1], yamlScalar(item), func(dash, _ int) []string {
		return []string{strings.Repeat(" ", dash) + "- " + yamlScalar(item)}
	})
}

// appendTo adds one item to the list at the key of sp: flow is the item as it
// stands in a flow list, block its lines in a block list, given the dash
// column and the column of the keys of a record.
func (t *text) appendTo(sp spot, key, flow string, block func(dash, key int) []string) error {
	line := t.lines[sp.idx]
	cut := sp.ind + len(key) + 1
	prefix, rest := line[:cut], line[cut:]
	val := strings.TrimLeft(rest, " ")
	space := rest[:len(rest)-len(val)]
	value, tail := splitComment(val)
	switch {
	case strings.HasPrefix(value, "["):
		if !strings.HasSuffix(value, "]") {
			return unsupported(t.file, sp.idx+1, "the list is written over several lines in flow style")
		}
		inner := strings.TrimSpace(value[1 : len(value)-1])
		joined := flow
		if inner != "" {
			joined = strings.TrimRight(value[:len(value)-1], " ") + ", " + joined
		} else {
			joined = "[" + joined
		}
		t.lines[sp.idx] = prefix + space + joined + "]" + tail
	case value == "":
		end, dash := t.listEnd(sp.idx, sp.ind)
		t.splice(end, end, block(dash, t.recordKeyColumn(sp.idx+1, end, dash)))
	default:
		return unsupported(t.file, sp.idx+1, "the value is not a list")
	}
	return nil
}

// addRule adds the rule written as the mapping body to the end of the rules
// list, in the dash column and the blank-line habit the file already has.
func (t *text) addRule(id, body string) error {
	_, err := t.ruleItem(id)
	return t.addEntry(entryKind{what: "rule", list: "rules", key: "id"}, id, body, err == nil)
}

// entryKind names a list of entries a file holds and the field that keys them.
type entryKind struct{ what, list, key string }

// addEntry adds the entry body, a mapping keyed by id, at the end of the list
// of its kind, in the layout the list already has; has says the id is there.
func (t *text) addEntry(k entryKind, id, body string, has bool) error {
	v, err := ordered.ParseYAML([]byte(body))
	if m, ok := v.(*ordered.Map); err != nil || !ok || m.Str(k.key) != id {
		if err == nil {
			err = errors.New("it is not a mapping with that " + k.key)
		}
		return fmt.Errorf("the new %s %s: %w", k.what, id, err)
	}
	if has {
		return moved(t.file, 1, "the %s %s is in the file already", k.what, id)
	}
	at := -1
	for i, s := range t.lines {
		if strings.HasPrefix(s, k.list+":") {
			at = i
		}
	}
	if at < 0 {
		return unsupported(t.file, 1, "the file has no "+k.list+": list to add to")
	}
	if strings.TrimSpace(strings.TrimPrefix(t.lines[at], k.list+":")) == "[]" {
		t.lines[at] = k.list + ":"
	}
	end, dash := t.listEnd(at, 0)
	sep := false
	for i := end - 1; i > at; i-- {
		if indentOf(t.lines[i]) == dash && strings.HasPrefix(strings.TrimSpace(t.lines[i]), "- ") {
			sep = i-1 > at && blank(t.lines[i-1])
			break
		}
	}
	var out []string
	if sep {
		out = append(out, "")
	}
	for k, l := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		pad := strings.Repeat(" ", dash)
		if k == 0 {
			out = append(out, pad+"- "+l)
		} else {
			out = append(out, pad+"  "+l)
		}
	}
	t.splice(end, end, out)
	return nil
}

// numeric are the fields of a rule that hold a number, a bool or records of
// them (pairs[].number, bindings[].rung and .pair, in rule.schema.json). A
// scalar written for one would be a string the schema then refuses.
func numeric(field string) bool { return field == "pairs" || field == "bindings" }

// lineOf is the line of the rule's id, 1 when the rule is not found.
func (t *text) lineOf(id string) int {
	if it, err := t.ruleItem(id); err == nil {
		return it.m.Line
	}
	return 1
}
