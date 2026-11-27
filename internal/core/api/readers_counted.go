package coreapi

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/core/audit"
	"github.com/birdtable/gragas/internal/ordered"
)

// Counting is what the count check reads of one tool, as its readers row
// gives it until the tool's plugin does: the names its config
// may have, the key paths of its exclusion lists and the word of its Go
// directive, "" for none.
type Counting struct {
	Configs    []string
	Exclusions [][]string
	Loosening  []Loosening
	Directive  string
	FoldKeys   bool
}

// Loosening is a scalar of a tool's config, at a key path, whose Values
// exclude more than the tool's default: each such value is one
// exclusion row of the count.
type Loosening struct {
	At     []string
	Values []string
	// Unless is the tool's default, for a value that loosens whenever it is
	// anything else; nil where Values name the loosening ones.
	Unless *string
}

// countings is the audit's view of each tool's counting.
func countings(cs []Counting) []audit.Counting {
	out := make([]audit.Counting, len(cs))
	for i, c := range cs {
		var l []audit.Loosening
		for _, s := range c.Loosening {
			l = append(l, audit.Loosening{At: s.At, Values: s.Values, Unless: s.Unless})
		}
		out[i] = audit.Counting{Configs: c.Configs, Exclusions: c.Exclusions, Loosening: l, Directive: c.Directive, FoldKeys: c.FoldKeys}
	}
	return out
}

// The limits of a row's counted, the same as the manifest's counted
// (contract/plugin/v1/manifest.schema.json), so a row and the
// plugin that will give it hold one shape: a key is lower-case letters,
// digits, - and _, starting with a letter or a digit; a word is lower-case
// letters, digits and -, starting with a letter.
const (
	maxExclusions = 64
	maxSegments   = 16
	maxSegment    = 128
	maxDirective  = 32
	maxLoosening  = 16
	maxValues     = 16
)

// isExclusionKey is whether s is a key of an exclusion path.
func isExclusionKey(s string) bool {
	return s != "" && len(s) <= maxSegment && lowerOrDigit(rune(s[0])) && !strings.ContainsFunc(s, func(r rune) bool {
		return !lowerOrDigit(r) && r != '-' && r != '_'
	})
}

// isDirectiveWord is whether s is a directive's word.
func isDirectiveWord(s string) bool {
	return s != "" && len(s) <= maxDirective && s[0] >= 'a' && s[0] <= 'z' && !strings.ContainsFunc(s, func(r rune) bool {
		return !lowerOrDigit(r) && r != '-'
	})
}

// lowerOrDigit is whether r is an ASCII lower-case letter or digit.
func lowerOrDigit(r rune) bool { return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' }

// countedOf reads the row's counted, when it has one, into r: a mapping with
// exclusions, a list of key paths each a list of keys, loosening, a list of
// a key path (at) and one of its loosening texts (values) or the default
// (unless), directive, a word, and fold_keys, true or false, and nothing
// else, held to the manifest's limits. It counts something: one of
// exclusions, loosening and directive is there.
func countedOf(r *Reader, m *ordered.Map) error {
	v, ok := m.Get("counted")
	if !ok {
		return nil
	}
	c, isMap := v.(*ordered.Map)
	if !isMap || c.Len() == 0 {
		return errors.New("counted is not a mapping with exclusions, loosening, a directive or fold_keys")
	}
	if err := countedKeys(c); err != nil {
		return err
	}
	if f, has := c.Get("fold_keys"); has {
		fold, isBool := f.(bool)
		if !isBool {
			return errors.New("counted.fold_keys is not true or false")
		}
		r.FoldKeys = fold
	}
	if _, has := c.Get("loosening"); has {
		l, err := looseningOf(c.List("loosening"))
		if err != nil {
			return err
		}
		r.Loosening = l
	}
	if d, has := c.Get("directive"); has {
		word, ok := d.(string)
		if !isDirectiveWord(word) || !ok {
			return fmt.Errorf("counted.directive is not a word: lower-case letters, digits and -, starting with a letter, at most %d bytes", maxDirective)
		}
		r.Directive = word
	}
	if _, has := c.Get("exclusions"); has {
		paths, err := exclusionsOf(c.List("exclusions"))
		if err != nil {
			return err
		}
		r.Exclusions = paths
	}
	return nil
}

// countedKeys refuses a counted that counts nothing, and a key it does not hold.
func countedKeys(c *ordered.Map) error {
	counts := false
	for _, k := range c.Keys() {
		switch k {
		case "exclusions", "loosening", "directive":
			counts = true
		case "fold_keys":
		default:
			return fmt.Errorf("counted has the key %q, and holds only exclusions, loosening, directive and fold_keys", k)
		}
	}
	if !counts {
		return errors.New("counted holds none of exclusions, loosening and directive, so it counts nothing")
	}
	return nil
}

// looseningOf is the scalars of counted.loosening: one to 16 of them, each a
// mapping of at, a key path of one to 16 keys (isExclusionKey), and values,
// one to 16 words (isDirectiveWord), and nothing else, each path once.
func looseningOf(items []any) ([]Loosening, error) {
	if len(items) == 0 || len(items) > maxLoosening {
		return nil, fmt.Errorf("counted.loosening holds %d scalars, and holds one to %d", len(items), maxLoosening)
	}
	var out []Loosening
	for i, it := range items {
		m, isMap := it.(*ordered.Map)
		if !isMap {
			return nil, fmt.Errorf("counted.loosening[%d] is not a mapping of at and one of values and unless", i)
		}
		_, hasValues := m.Get("values")
		_, hasUnless := m.Get("unless")
		if m.Len() != 2 || hasValues == hasUnless {
			return nil, fmt.Errorf("counted.loosening[%d] is not a mapping of at and one of values and unless", i)
		}
		at, err := keysOf(m.List("at"), fmt.Sprintf("counted.loosening[%d].at", i))
		if err != nil {
			return nil, err
		}
		l := Loosening{At: at}
		if hasUnless {
			if l.Unless, err = unlessOf(m, i); err != nil {
				return nil, err
			}
		} else if l.Values, err = valuesOf(m.List("values"), i); err != nil {
			return nil, err
		}
		if slices.ContainsFunc(out, func(o Loosening) bool { return slices.Equal(o.At, at) }) {
			return nil, fmt.Errorf("counted.loosening[%d] is %s a second time", i, strings.Join(at, "."))
		}
		out = append(out, l)
	}
	return out, nil
}

// valuesOf is a loosening's values: one to 16 texts (isValueText), each once.
func valuesOf(texts []any, i int) ([]string, error) {
	if len(texts) == 0 || len(texts) > maxValues {
		return nil, fmt.Errorf("counted.loosening[%d].values holds %d words, and holds one to %d", i, len(texts), maxValues)
	}
	var values []string
	for _, w := range texts {
		text, ok := w.(string)
		if !ok || !isValueText(text) || slices.Contains(values, text) {
			return nil, fmt.Errorf("counted.loosening[%d].values has a value that is not a word given once: lower-case letters, digits and -, starting with a letter or a digit, at most %d bytes", i, maxDirective)
		}
		values = append(values, text)
	}
	return values, nil
}

// unlessOf is a loosening's unless: the default's text, a value text, empty,
// or [] for an empty list.
func unlessOf(m *ordered.Map, i int) (*string, error) {
	v, _ := m.Get("unless")
	text, ok := v.(string)
	if !ok || (text != "" && text != "[]" && !isValueText(text)) {
		return nil, fmt.Errorf("counted.loosening[%d].unless is not a default's text: empty, [], or lower-case letters, digits and -, starting with a letter or a digit, at most %d bytes", i, maxDirective)
	}
	return &text, nil
}

// isValueText is whether s is a loosening value's text: lower-case letters,
// digits and -, starting with a letter or a digit, so false and 0 are texts.
func isValueText(s string) bool {
	return s != "" && len(s) <= maxDirective && lowerOrDigit(rune(s[0])) && !strings.ContainsFunc(s, func(r rune) bool {
		return !lowerOrDigit(r) && r != '-'
	})
}

// keysOf is a key path: one to 16 keys (isExclusionKey), where names it.
func keysOf(keys []any, where string) ([]string, error) {
	if len(keys) == 0 || len(keys) > maxSegments {
		return nil, fmt.Errorf("%s is not a list of one to %d keys", where, maxSegments)
	}
	var path []string
	for _, k := range keys {
		key, ok := k.(string)
		if !ok || !isExclusionKey(key) {
			return nil, fmt.Errorf("%s has a key that is not lower-case letters, digits, - and _, of at most %d bytes", where, maxSegment)
		}
		path = append(path, key)
	}
	return path, nil
}

// exclusionsOf is the key paths of counted.exclusions: one to 64 of them, each
// once, each one to 16 keys (isExclusionKey).
func exclusionsOf(items []any) ([][]string, error) {
	if len(items) == 0 || len(items) > maxExclusions {
		return nil, fmt.Errorf("counted.exclusions holds %d paths, and holds one to %d", len(items), maxExclusions)
	}
	var out [][]string
	for i, it := range items {
		keys, isList := it.([]any)
		if !isList || len(keys) == 0 || len(keys) > maxSegments {
			return nil, fmt.Errorf("counted.exclusions[%d] is not a list of one to %d keys", i, maxSegments)
		}
		var path []string
		for _, k := range keys {
			key, ok := k.(string)
			if !ok || !isExclusionKey(key) {
				return nil, fmt.Errorf("counted.exclusions[%d] has a key that is not lower-case letters, digits, - and _, of at most %d bytes", i, maxSegment)
			}
			path = append(path, key)
		}
		if slices.ContainsFunc(out, func(p []string) bool { return slices.Equal(p, path) }) {
			return nil, fmt.Errorf("counted.exclusions[%d] is %s a second time", i, strings.Join(path, "."))
		}
		out = append(out, path)
	}
	return out, nil
}
