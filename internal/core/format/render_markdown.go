package format

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/markdown"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// gragasBlocks are the closed fenced blocks of a guide whose info string is
// gragas. The loader and the renderer both find a guide's blocks here, so
// they cannot disagree about where a block is.
func gragasBlocks(doc *markdown.Doc) []markdown.Block {
	var out []markdown.Block
	for _, b := range doc.Find(markdown.GragasInfo) {
		if b.Closed {
			out = append(out, b)
		}
	}
	return out
}

// The changes renderGuide applies, besides a set or an append on a rule that
// is in a block (entity rule:<id>, as in a concern file):
//   - block:<guide path>#<anchor> with field rules[]: After is the YAML mapping
//     of a new rule, added to the block under that anchor.
//   - block:<guide path>#<anchor> with no field and no Before: After is the
//     YAML of a block, anchor and rules, added at the end of the guide under
//     that anchor.
//
// Anything that would change prose, or remove a rule, is refused with the
// section named. Bytes outside the body of the one block edited are not
// touched, so line endings, a BOM and a missing final newline stay as they
// were.

// renderGuide applies the changes to a guide's gragas blocks.
func renderGuide(schema port.Schema, file string, old []byte, exists bool, changes []model.Change) ([]byte, error) {
	if !exists {
		return nil, unsupported(file, 1, "the guide is prose its author writes; the engine does not create it")
	}
	src := old
	for _, c := range changes {
		var err error
		if src, err = guideChange(schema, file, src, c); err != nil {
			return nil, err
		}
	}
	return src, nil
}

func guideChange(schema port.Schema, file string, src []byte, c model.Change) ([]byte, error) {
	doc, err := markdown.Parse(src)
	if err != nil {
		return nil, unsupported(file, 1, "a fenced block is never closed; close it by hand first")
	}
	blocks := gragasBlocks(doc)
	kind, key, _ := strings.Cut(c.Entity, ":")
	switch kind {
	case "prose":
		_, anchor, _ := strings.Cut(key, "#")
		return nil, unsupported(file, 1, fmt.Sprintf("the change is to the prose of the section %q; prose is edited as a file, never by the engine", anchor))
	case "block":
		_, anchor, _ := strings.Cut(key, "#")
		return blockChange(schema, file, src, blocks, anchor, c)
	}
	id := key
	b, ok := blockOf(blocks, id)
	switch {
	case !ok:
		return nil, moved(file, 1, "the rule %s is not in a block of the file", id)
	case c.Field == "" && c.Before != "":
		return nil, unsupported(file, b.Line, fmt.Sprintf("%s would be removed from the section %q; the engine removes nothing from a guide", id, sectionOf(b)))
	case c.Field == "":
		return nil, unsupported(file, b.Line, "a new rule goes in a block of the guide: name it as block:<file>#<anchor> with the field rules[]")
	}
	return editBlock(schema, file, src, b, func(t *text) error { return t.apply(c) })
}

func blockChange(schema port.Schema, file string, src []byte, blocks []markdown.Block, anchor string, c model.Change) ([]byte, error) {
	at := -1
	for i, b := range blocks {
		if a, _ := blockContent(b); a == anchor {
			at = i
		}
	}
	switch {
	case c.Field == "" && c.Before == "":
		if at >= 0 {
			return nil, moved(file, blocks[at].Line, "the guide has a block under %q already", anchor)
		}
		return appendBlock(schema, file, src, anchor, c.After)
	case at < 0:
		return nil, moved(file, 1, "the guide has no block under %q", anchor)
	case c.Field == "rules[]" && c.Before == "":
		v, err := ordered.ParseYAML([]byte(c.After))
		m, ok := v.(*ordered.Map)
		if err != nil || !ok || m.Str("id") == "" {
			return nil, fmt.Errorf("the new rule for %q is not a YAML mapping with an id", anchor)
		}
		return editBlock(schema, file, src, blocks[at], func(t *text) error { return t.addRule(m.Str("id"), c.After) })
	}
	return nil, unsupported(file, blocks[at].Line, fmt.Sprintf("the change to the block under %q is not one the engine makes: it adds rules and blocks, and removes nothing", anchor))
}

// blockContent is the anchor of a block and the ids of its rules.
func blockContent(b markdown.Block) (string, []string) {
	v, err := ordered.ParseYAML([]byte(b.Body))
	m, ok := v.(*ordered.Map)
	if err != nil || !ok {
		return "", nil
	}
	var ids []string
	for _, rm := range ordered.Maps(m.List("rules")) {
		ids = append(ids, rm.Str("id"))
	}
	return m.Str("anchor"), ids
}

func blockOf(blocks []markdown.Block, id string) (markdown.Block, bool) {
	for _, b := range blocks {
		_, ids := blockContent(b)
		if slices.Contains(ids, id) {
			return b, true
		}
	}
	return markdown.Block{}, false
}

func sectionOf(b markdown.Block) string {
	if b.Section == "" {
		return "before the first heading"
	}
	return b.Section
}

// editBlock runs edit on the body of one block and puts the result back
// between its fences, checked against the block schema.
func editBlock(schema port.Schema, file string, src []byte, b markdown.Block, edit func(*text) error) ([]byte, error) {
	t, err := newText(file, []byte(b.Body))
	if err != nil {
		return nil, shift(err, b.Line)
	}
	if err := edit(t); err != nil {
		return nil, shift(err, b.Line)
	}
	body := t.bytes()
	if _, bad, err := parseAt(schema, file, body, blockSchema, b.Line); err != nil || len(bad) > 0 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("the edit would leave the block at %s:%d invalid: %s", file, b.Line, bad[0])
	}
	out := make([]byte, 0, len(src)+len(body))
	out = append(out, src[:b.BodyStart]...)
	out = append(out, body...)
	return append(out, src[b.BodyEnd:]...), nil
}

// appendBlock adds a block under a new anchor at the end of the guide, in the
// file's own line ending.
func appendBlock(schema port.Schema, file string, src []byte, anchor, yaml string) ([]byte, error) {
	v, err := ordered.ParseYAML([]byte(yaml))
	m, ok := v.(*ordered.Map)
	if err != nil || !ok || m.Str("anchor") != anchor {
		return nil, fmt.Errorf("the new block %q is not a YAML mapping with that anchor", anchor)
	}
	nl := "\n"
	if bytes.Contains(src, []byte("\r\n")) {
		nl = "\r\n"
	}
	body := strings.Join(strings.Split(strings.TrimRight(yaml, "\r\n"), "\n"), nl) + nl
	if _, bad, err := parseAt(schema, file, []byte(body), blockSchema, 0); err != nil || len(bad) > 0 {
		if err == nil {
			err = errors.New(bad[0].String())
		}
		return nil, fmt.Errorf("the new block %q is invalid: %w", anchor, err)
	}
	out := append([]byte{}, src...)
	if len(out) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, nl...)
	}
	if len(out) > 0 && !bytes.HasSuffix(out, []byte(nl+nl)) {
		out = append(out, nl...)
	}
	out = append(out, "```gragas"+nl+body+"```"+nl...)
	return out, nil
}
