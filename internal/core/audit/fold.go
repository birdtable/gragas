package audit

import (
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// folded is a config read the way the tools read it. Both tools take their
// keys through a reader that ignores the case of a key and splits it on a dot,
// so `Linters: {Settings: {Custom: ...}}` and
// `linters.settings.custom:` are one key to the tool and must be one to this
// check. Each path is the lower-cased segments joined by a dot; a mapping with
// entries is a node only, anything else (a scalar, a list, an empty mapping,
// a null) is a leaf.
type folded map[string]any

// foldYAML is the folded view of a mapping, or the reason it is not read: two
// keys fold to one path. A mapping read as a whole is never merged from two
// spellings, so a file written twice over is not checked.
func foldYAML(m *ordered.Map) (folded, error) {
	out := folded{}
	return out, foldInto(out, m, nil, map[string]bool{})
}

func foldInto(out folded, m *ordered.Map, prefix []string, written map[string]bool) error {
	for _, k := range m.Keys() {
		ks, err := keySegments(k)
		if err != nil {
			return err
		}
		segs := append(append([]string{}, prefix...), ks...)
		p := strings.Join(segs, ".")
		if written[p] {
			return fmt.Errorf("two keys of the file are one key to the tool (%s)", p)
		}
		written[p] = true
		v, _ := m.Get(k)
		if sub, ok := v.(*ordered.Map); ok && sub.Len() > 0 {
			out[p] = nil
			if err := foldInto(out, sub, segs, written); err != nil {
				return err
			}
			continue
		}
		out[p] = v
	}
	return nil
}

// FoldConfig reads data as one YAML or JSON mapping, the way the tool reads
// its config's keys: each path the lower-cased segments joined by a dot, a
// mapping with entries a node (nil), anything else a leaf (folded). mapping is
// false when data is not one mapping, and err is then the parse's; with a
// mapping, err is the fold's refusal (two keys that are one key to the tool, a
// key outside ASCII). The os-files read takes its host layer from it, and the
// operations' adapter-backed entries their settings blocks.
func FoldConfig(data []byte) (paths map[string]any, mapping bool, err error) {
	v, perr := ordered.ParseStrict(data)
	doc, ok := v.(*ordered.Map)
	if perr != nil || !ok {
		return nil, false, perr
	}
	f, err := foldYAML(doc)
	return f, true, err
}

// configPaths is the folded view of a config file, or the reason it is not
// read: it is no mapping, or two keys of it are one key to the tool.
func configPaths(file string, data []byte) (folded, string) {
	f, mapping, err := FoldConfig(data)
	switch {
	case !mapping:
		return nil, fmt.Sprintf("%s cannot be read as a mapping: %v", file, err)
	case err != nil:
		return nil, fmt.Sprintf("%s: %v", file, err)
	}
	return f, ""
}

// configFormat is whether ext is the extension of a config whose document
// this check parses as the tool does: YAML, and JSON, which the YAML parser
// reads.
func configFormat(ext string) bool { return ext == ".yml" || ext == ".yaml" || ext == ".json" }

// keySegments is a key as the tools read it: lower-cased and split on a dot.
// A key outside ASCII is refused: how each tool version folds one (the long s
// is an s to a reader that folds simply) is not a table kept here, so the
// file is not checked.
func keySegments(k string) ([]string, error) {
	for i := 0; i < len(k); i++ {
		if k[i] >= 0x80 {
			return nil, fmt.Errorf("the key %q is not ASCII, and the tool's fold of it is not known", k)
		}
	}
	return strings.Split(strings.ToLower(k), "."), nil
}

// foldTOML is the folded view of a TOML document, read the way the tool reads
// a key: folded and split on a dot, so `"allowlist.paths"` at the
// top is `[allowlist] paths`. A path written by two keys, or one that is a
// value for one key and a table for another, is refused. The tables of one
// list are each a document of their own.
func foldTOML(doc map[string]any) (map[string]any, error) {
	out := map[string]any{}
	return out, foldTable(out, doc, nil, map[string]bool{})
}

func twoKeys(p string) error {
	return fmt.Errorf("two keys of the file are one key to the tool (%s)", p)
}

func foldTable(root, in map[string]any, prefix []string, written map[string]bool) error {
	for k, e := range in {
		ks, err := keySegments(k)
		if err != nil {
			return err
		}
		segs := append(append([]string{}, prefix...), ks...)
		p := strings.Join(segs, ".")
		if written[p] {
			return twoKeys(p)
		}
		written[p] = true
		at, err := tableAt(root, segs[:len(segs)-1], p)
		if err != nil {
			return err
		}
		last := segs[len(segs)-1]
		if tbl, isTable := e.(map[string]any); isTable {
			if err := foldNode(root, at, last, tbl, segs, written); err != nil {
				return err
			}
			continue
		}
		if _, exists := at[last]; exists {
			return twoKeys(p)
		}
		v, err := foldLeaf(e)
		if err != nil {
			return err
		}
		at[last] = v
	}
	return nil
}

// tableAt is the table at a path below root, its missing tables made, or the
// error that a part of the path is a value.
func tableAt(root map[string]any, path []string, p string) (map[string]any, error) {
	at := root
	for _, s := range path {
		next, ok := at[s]
		if !ok {
			next = map[string]any{}
			at[s] = next
		}
		m, isTable := next.(map[string]any)
		if !isTable {
			return nil, twoKeys(p)
		}
		at = m
	}
	return at, nil
}

// foldLeaf is a value that is no table, each table of a list folded as a
// document of its own.
func foldLeaf(e any) (any, error) {
	l, ok := e.([]any)
	if !ok {
		return e, nil
	}
	list := make([]any, len(l))
	for i, el := range l {
		m, isTable := el.(map[string]any)
		if !isTable {
			list[i] = el
			continue
		}
		sub := map[string]any{}
		if err := foldTable(sub, m, nil, map[string]bool{}); err != nil {
			return nil, err
		}
		list[i] = sub
	}
	return list, nil
}

// foldNode makes the table a node of its parent: the keys below it land by
// their own path.
func foldNode(root, at map[string]any, last string, tbl map[string]any, segs []string, written map[string]bool) error {
	if old, exists := at[last]; exists {
		if _, ok := old.(map[string]any); !ok {
			return twoKeys(strings.Join(segs, "."))
		}
	} else {
		at[last] = map[string]any{}
	}
	return foldTable(root, tbl, segs, written)
}
