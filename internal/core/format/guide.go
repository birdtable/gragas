package format

import (
	"errors"
	"fmt"

	"github.com/birdtable/gragas/internal/markdown"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// GuideFile is a part's guide, next to its rules folder.
const GuideFile = "guide.md"

const blockSchema = "v3/block.schema.json"

// guide reads <layer>/<part>/guide.md, when the part has one: each fenced
// block whose info string is gragas holds the YAML of one section, its anchor
// once and one or more rules. Prose outside the blocks is never touched. A
// problem in a block is a finding at its line in the Markdown file; the other
// blocks are still read.
func (r reader) guide(layer string, p model.Part) error {
	file := join(r.dir, layer+"/"+p.Slug+"/"+GuideFile)
	if !r.exists(file) {
		return nil
	}
	src, err := r.tree.Read(r.root, file)
	if err != nil {
		return err
	}
	doc, err := markdown.Parse(src)
	if me, ok := errors.AsType[*markdown.Error](err); ok {
		r.add(file, me.Line, "the fenced block opened here is never closed; close it with a fence of its own")
	}
	anchors := map[string]int{}
	for _, b := range gragasBlocks(doc) {
		if err := r.block(file, b, anchors); err != nil {
			return err
		}
	}
	return nil
}

// block reads one closed gragas block; anchors holds the line of the block
// that already stands under each anchor of the guide.
func (r reader) block(file string, b markdown.Block, anchors map[string]int) error {
	m, bad, err := parseAt(r.schema, file, []byte(b.Body), blockSchema, b.Line)
	if err != nil {
		return err
	}
	r.out.Findings = append(r.out.Findings, bad...)
	if m == nil {
		return nil
	}
	anchor := m.Str("anchor")
	if first, dup := anchors[anchor]; dup {
		r.add(file, b.Line, fmt.Sprintf("two blocks under the anchor %q (the first is at line %d); keep the rules of a section in one block", anchor, first))
		return nil
	}
	anchors[anchor] = b.Line
	for _, rm := range ordered.Maps(m.List("rules")) {
		rule := ruleOf(rm)
		if !r.addRule(rule, rm, file, b.Line) {
			continue
		}
		if r.out.Book.Anchors == nil {
			r.out.Book.Anchors = map[model.RuleID]string{}
		}
		r.out.Book.Anchors[rule.ID] = anchor
	}
	return nil
}

// parseAt is parse for text that starts below line off of its file: the lines
// of the findings are the file's, and a problem at no line is at off, the
// block's opening fence.
func parseAt(schema port.Schema, file string, data []byte, name string, off int) (*ordered.Map, []Finding, error) {
	m, bad, err := parse(schema, file, data, name, migrations())
	for i := range bad {
		if bad[i].Line > 0 {
			bad[i].Line += off
		} else {
			bad[i].Line = off
		}
	}
	return m, bad, err
}

// addRule adds a rule to the book unless its id is already used by another
// rule or alias in the book, which is a finding at the second place, or its
// id or an alias starts with the word the engine's checks start with, a
// finding for each. It tells whether the rule was added.
func (r reader) addRule(rule model.Rule, rm *ordered.Map, file string, off int) bool {
	line := off + rm.Line
	here := fmt.Sprintf("%s:%d", file, line)
	if bad := reservedIDs(rule); len(bad) > 0 {
		for _, id := range bad {
			r.add(file, line, fmt.Sprintf(reservedID, id))
		}
		return false
	}
	seen := map[model.RuleID]bool{}
	for _, id := range append([]model.RuleID{rule.ID}, rule.Aliases...) {
		if at, dup := r.ids[id]; dup || seen[id] {
			if !dup {
				r.add(file, line, fmt.Sprintf("the id %s is listed twice in this rule; an id is used once", id))
				return false
			}
			r.add(file, line, fmt.Sprintf("the id %s is also at %s; an id is used once", id, at))
			if id == rule.ID {
				r.out.Book.Clashed = append(r.out.Book.Clashed, rule)
				if _, kept := r.out.Clashed[id]; !kept {
					r.out.Clashed[id] = Site{File: file, Line: line}
				}
			}
			return false
		}
		seen[id] = true
	}
	for id := range seen {
		r.ids[id] = here
		r.out.at.rules[id] = ref{id: id, file: file, line: line, what: "rule"}
	}
	r.tightens(rule, rm, file, off)
	*r.refs = append(*r.refs, ruleRefs(rule, rm, file, off)...)
	if rule.Reasoning != nil && rule.Reasoning.Decision != "" {
		*r.slugs = append(*r.slugs, slugRef{id: rule.Reasoning.Decision, file: file, line: off + rm.Sub("reasoning").KeyLine("decision")})
	}
	r.out.Sites["rule:"+string(rule.ID)] = Site{File: file, Line: line}
	*r.prose = append(*r.prose, proseItems(rule, rm, file, off)...)
	r.out.Book.Rules = append(r.out.Book.Rules, rule)
	return true
}

// tightens keeps each binding of rule that names a rule it tightens, at the
// line of its tightens, for the contract checks.
func (r reader) tightens(rule model.Rule, rm *ordered.Map, file string, off int) {
	maps := ordered.Maps(rm.List("bindings"))
	for i, b := range rule.Bindings {
		if b.Tightens == "" {
			continue
		}
		line := off + rm.Line
		if i < len(maps) {
			line = off + maps[i].KeyLine("tightens")
		}
		r.out.at.tightens = append(r.out.at.tightens, ref{id: b.Tightens, file: file, line: line, what: "tightens"})
	}
}
