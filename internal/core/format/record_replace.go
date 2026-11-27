package format

import (
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// holdsRecord says the parsed record m holds each key of r at the text r gives
// it. It may hold more keys: hand-written ones, such as nested options, that
// a record in a delta cannot carry and a replacement leaves where they are.
func holdsRecord(m *ordered.Map, r []recField) bool {
	for _, f := range r {
		got, err := recValue(f.key, m.Val(f.key))
		if err != nil || got.list != f.list || strings.Join(got.vals, "\x00") != strings.Join(f.vals, "\x00") {
			return false
		}
	}
	return true
}

// replaceRecord changes one record of the block list at field of the rule in
// place: the record that holds the keys of before takes the texts after gives
// its keys, on the lines they stand on, and every other line of the record, its
// hand-written keys and comments among them, is not touched. The keys of after
// are the keys of before: a key added or taken out is not made here.
func (t *text) replaceRecord(id, field, before, after string) error {
	it, err := t.ruleItem(id)
	if err != nil {
		return err
	}
	was, err := parseRecord(before)
	if err != nil {
		return fmt.Errorf("the record of %s to change in %s: %w", field, id, err)
	}
	now, err := parseRecord(after)
	if err != nil {
		return fmt.Errorf("the new record of %s in %s: %w", field, id, err)
	}
	path := strings.Split(field, ".")
	cur, present := valueAt(it.m, path)
	line := max(ordered.LineAt(it.m, path), it.m.Line)
	list, isList := cur.([]any)
	if !present || !isList {
		return moved(t.file, line, "%s of %s holds no list to change a record in", field, id)
	}
	k := -1
	for i, v := range list {
		if m, ok := v.(*ordered.Map); ok && holdsRecord(m, was) {
			k = i
			break
		}
	}
	if k < 0 {
		return moved(t.file, line, "%s of %s does not hold %s", field, id, flowRecord(was))
	}
	lo, hi, col, err := t.recordLines(it, path, k, len(list))
	if err != nil {
		return err
	}
	for _, f := range now {
		if err := t.replaceKey(was, f, lo, hi, col); err != nil {
			return err
		}
	}
	return nil
}

// recordLines are the lines [lo, hi) of item k of the n items of the block list
// at path, and the column its keys stand at.
func (t *text) recordLines(it ruleItem, path []string, k, n int) (lo, hi, col int, err error) {
	sp := t.locate(it, path)
	key := path[len(path)-1]
	if !sp.found {
		return 0, 0, 0, unsupported(t.file, it.m.Line, strings.Join(path, ".")+" is written in flow style")
	}
	if value, _ := splitComment(strings.TrimLeft(t.lines[sp.idx][sp.ind+len(key)+1:], " ")); value != "" {
		return 0, 0, 0, unsupported(t.file, sp.idx+1, strings.Join(path, ".")+" is written in flow style")
	}
	end, dash := t.listEnd(sp.idx, sp.ind)
	var starts []int
	for j := sp.idx + 1; j < end; j++ {
		s := t.lines[j]
		if !blank(s) && indentOf(s) == dash && strings.HasPrefix(strings.TrimSpace(s), "-") {
			starts = append(starts, j)
		}
	}
	if len(starts) != n {
		return 0, 0, 0, unsupported(t.file, sp.idx+1, "the list is not written one item to a dash")
	}
	lo, hi = starts[k], end
	if k+1 < n {
		hi = starts[k+1]
	}
	col = dash + 1
	for col < len(t.lines[lo]) && t.lines[lo][col] == ' ' {
		col++
	}
	return lo, hi, col, nil
}

// replaceKey gives the key of f, which the record was written with as was has
// it, its new text on the line it stands on in lines [lo, hi) at column col.
func (t *text) replaceKey(was []recField, f recField, lo, hi, col int) error {
	i := -1
	for j := range was {
		if was[j].key == f.key {
			i = j
		}
	}
	if i < 0 || was[i].list || f.list {
		return unsupported(t.file, lo+1, "the key "+f.key+" of the record is added or is a list; change it by hand")
	}
	if was[i].text() == f.text() {
		return nil
	}
	for j := lo; j < hi; j++ {
		s := t.lines[j]
		if len(s) > col && (j == lo || indentOf(s) == col) && strings.HasPrefix(s[col:], f.key+":") {
			return t.replaceValue(spot{found: true, idx: j, ind: col}, f.key, f.text())
		}
	}
	return unsupported(t.file, lo+1, "the key "+f.key+" is not written on a line of its own in the record")
}
