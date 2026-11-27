package format

import (
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// removeRecord takes the record before, a YAML mapping, out of the list of
// records at field of the rule, flow or block style as the file has it. Only
// the record's own lines, or its own text and the comma before it in a flow
// list, are taken out. A list the change leaves empty goes with its key, as a
// list of records is never written empty (rule.schema.json, minItems). A
// record the list does not hold is refused as moved.
func (t *text) removeRecord(id, field, before string) error {
	it, err := t.ruleItem(id)
	if err != nil {
		return err
	}
	rec, err := parseRecord(before)
	if err != nil {
		return fmt.Errorf("the record of %s to take out of %s: %w", field, id, err)
	}
	path := strings.Split(field, ".")
	cur, present := valueAt(it.m, path)
	line := max(ordered.LineAt(it.m, path), it.m.Line)
	list, isList := cur.([]any)
	if present && cur != nil && !isList {
		return unsupported(t.file, line, field+" is not a list")
	}
	k := -1
	for i, v := range list {
		if m, ok := v.(*ordered.Map); ok && sameRecord(m, rec) {
			k = i
			break
		}
	}
	if k < 0 {
		return moved(t.file, line, "%s of %s does not hold %s", field, id, flowRecord(rec))
	}
	sp := t.locate(it, path)
	if !sp.found {
		return unsupported(t.file, line, field+" is written in flow style")
	}
	key := path[len(path)-1]
	value, _ := splitComment(strings.TrimLeft(t.lines[sp.idx][sp.ind+len(key)+1:], " "))
	switch {
	case strings.HasPrefix(value, "["):
		return t.cutFlow(sp, key, k, len(list))
	case value == "":
		return t.cutBlock(sp, k, len(list))
	}
	return unsupported(t.file, sp.idx+1, "the value is not a list")
}

// cutBlock takes item k of the n items of the block list at the key of sp out,
// from the comments written right above its dash to the next item's dash, less
// the comments written above the next item, which stay with it. The last item
// goes with its key.
func (t *text) cutBlock(sp spot, k, n int) error {
	end, dash := t.listEnd(sp.idx, sp.ind)
	var starts []int
	for j := sp.idx + 1; j < end; j++ {
		s := t.lines[j]
		if !blank(s) && indentOf(s) == dash && strings.HasPrefix(strings.TrimSpace(s), "-") {
			starts = append(starts, j)
		}
	}
	if len(starts) != n {
		return unsupported(t.file, sp.idx+1, "the list is not written one item to a dash")
	}
	if n == 1 {
		t.splice(sp.idx, end, nil)
		return nil
	}
	lo, hi := starts[k], end
	for lo-1 > sp.idx && comment(t.lines[lo-1]) {
		lo--
	}
	if k+1 < n {
		hi = starts[k+1]
		for hi-1 > lo && comment(t.lines[hi-1]) {
			hi--
		}
	}
	t.splice(lo, hi, nil)
	return nil
}

// cutFlow takes item k of the n items of the one-line flow list at the key of
// sp out, with the comma that joins it to its neighbour. The last item goes
// with its key.
func (t *text) cutFlow(sp spot, key string, k, n int) error {
	ln := t.lines[sp.idx]
	cut := sp.ind + len(key) + 1
	rest := ln[cut:]
	val := strings.TrimLeft(rest, " ")
	value, _ := splitComment(val)
	if !strings.HasSuffix(value, "]") {
		return unsupported(t.file, sp.idx+1, "the list is written over several lines in flow style")
	}
	items, ok := flowItems(value)
	if !ok || len(items) != n {
		return unsupported(t.file, sp.idx+1, "the flow list is not one the engine reads item by item")
	}
	if n == 1 {
		t.splice(sp.idx, sp.idx+1, nil)
		return nil
	}
	lo, hi := items[0][0], items[1][0] // "item, ": up to the start of the next
	if k > 0 {
		lo, hi = items[k-1][1], items[k][1] // ", item": from the end of the one before
	}
	at := cut + len(rest) - len(val) // the column of the opening bracket
	t.lines[sp.idx] = ln[:at+lo] + ln[at+hi:]
	return nil
}

// flowItems are the spans, start and end, of the items of a one-line flow
// list written [a, {b: c}, ...]: the commas that part them are the ones
// outside any bracket and any quote. ok is false for a text that does not
// close as it opens.
func flowItems(list string) (spans [][2]int, ok bool) {
	if list == "" || list[0] != '[' {
		return nil, false
	}
	commas, end, ok := flowCommas(list)
	if !ok {
		return nil, false
	}
	bounds := append(append([]int{0}, commas...), end)
	for k := 0; k+1 < len(bounds); k++ {
		lo, hi := bounds[k]+1, trimEnd(list, bounds[k+1])
		for lo < hi && list[lo] == ' ' {
			lo++
		}
		switch {
		case lo < hi:
			spans = append(spans, [2]int{lo, hi})
		case k+2 < len(bounds):
			return nil, false // an empty item before a comma
		}
	}
	return spans, true
}

// flowCommas are the commas of a flow list that part its own items, outside
// any inner bracket and any quote, and end the bracket that closes it. ok is
// false for a text that does not close, or goes on after it closes.
func flowCommas(list string) (commas []int, end int, ok bool) {
	depth := 0
	for i := 0; i < len(list); i++ {
		switch c := list[i]; {
		case c == '\'' || c == '"':
			if i, ok = quoteEnd(list, i); !ok {
				return nil, 0, false
			}
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			if depth--; depth == 0 {
				return commas, i, i == len(list)-1
			}
		case c == ',' && depth == 1:
			commas = append(commas, i)
		}
	}
	return nil, 0, false
}

// quoteEnd is the index of the quote that closes the text quoted at i: two
// single quotes side by side are a quote inside a single-quoted text, and a
// backslash escapes the next byte of a double-quoted one. closed is false for
// a text that does not close.
func quoteEnd(list string, i int) (end int, closed bool) {
	q := list[i]
	for j := i + 1; j < len(list); j++ {
		switch {
		case q == '\'' && list[j] == '\'' && j+1 < len(list) && list[j+1] == '\'':
			j++
		case q == '"' && list[j] == '\\':
			j++
		case list[j] == q:
			return j, true
		}
	}
	return 0, false
}

// trimEnd is the end of the text before i, the spaces before i left out.
func trimEnd(s string, i int) int {
	for i > 0 && s[i-1] == ' ' {
		i--
	}
	return i
}
