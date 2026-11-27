// Package generate writes the rulebook's generated files from a loaded book: the
// rules.json, bindings.json, coverage.json and ids.lock. Output
// is deterministic: the same book gives the same bytes.
package generate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// GeneratedBy is the value of generated_by in the JSON outputs.
const GeneratedBy = "gragas gen"

// jsonIndent is the indent of the JSON outputs, in spaces.
const jsonIndent = 1

// schemaVersion is the rulebook schema rules.json is written in.
const schemaVersion = 2

// RulesJSON is the file name of rules.json, under the rulebook's folder.
const RulesJSON = "rules.json"

const lockHeader = `# Every rule id ever issued, one per line. gragas gen writes it
# as this list and the rulebook's ids together; gragas check fails when an id
# here is no longer in the rulebook. A retired rule keeps its id.
`

// Output is one generated file.
type Output struct {
	// Path is relative to the repository root, with forward slashes.
	Path string
	Data []byte
}

// Outputs returns every generated file for the book, in a fixed order:
// rules.json, bindings.json, coverage.json and ids.lock.
func Outputs(b Book) []Output {
	out := []Output{
		{Path: path.Join(b.Root(), RulesJSON), Data: []byte(rulesJSON(b))},
		{Path: path.Join(b.Root(), "bindings.json"), Data: []byte(bindingsJSON(b))},
		{Path: path.Join(b.Root(), "coverage.json"), Data: []byte(coverageJSON(b))},
		{Path: path.Join(b.Root(), "ids.lock"), Data: []byte(lockFile(b))},
	}
	return out
}

// Stale returns the outputs whose file in fsys is missing or differs.
func Stale(src legacy.Source, outs []Output) ([]Output, error) {
	var stale []Output
	for _, o := range outs {
		old, err := fs.ReadFile(src, o.Path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			stale = append(stale, o)
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", o.Path, err)
		case !bytes.Equal(old, o.Data):
			stale = append(stale, o)
		}
	}
	return stale, nil
}

// rulesJSON is every part, kind, source and rule, each rule with its part,
// concern and derived standing; rulebook/schema/rules-json.schema.json is
// its shape.
func rulesJSON(b Book) string {
	parts := []any{}
	for _, p := range b.Parts() {
		parts = append(parts, partRow(p))
	}
	all := b.Rules()
	rules := make([]any, 0, len(all))
	for _, r := range all {
		m := ordered.NewMap(0)
		m.Set("id", r.ID, 0)
		m.Set("part", r.Part, 0)
		m.Set("concern", r.Concern, 0)
		for _, k := range r.Data.Keys() {
			if k != "id" {
				m.Set(k, r.Data.Val(k), 0)
			}
		}
		m.Set("standing", b.Standing(r.ID), 0)
		rules = append(rules, m)
	}
	top := ordered.NewMap(0)
	top.Set("generated_by", GeneratedBy, 0)
	top.Set("schema", int64(schemaVersion), 0)
	top.Set("parts", parts, 0)
	top.Set("kinds", anyMaps(b.Registry().Kinds), 0)
	top.Set("sources", b.Registry().Sources, 0)
	top.Set("rules", rules, 0)
	return ordered.JSON(top, jsonIndent) + "\n"
}

// partRow is a part as the parts table writes it, rebuilt from its fields
// when the book kept no row.
func partRow(p Part) *ordered.Map {
	if p.Data != nil {
		return p.Data
	}
	m := ordered.NewMap(0)
	m.Set("short", p.Short, 0)
	m.Set("layer", p.Layer, 0)
	m.Set("name", p.Name, 0)
	m.Set("description", p.Description, 0)
	builds := make([]any, len(p.BuildsOn))
	for i, x := range p.BuildsOn {
		builds[i] = x
	}
	m.Set("builds_on", builds, 0)
	return m
}

func anyMaps(ms []*ordered.Map) []any {
	out := make([]any, len(ms))
	for i, m := range ms {
		out[i] = m
	}
	return out
}

// bindingsJSON lists the enforced rules only, each with the checks that
// hold it: what a project's gate runs (R-12).
func bindingsJSON(b Book) string {
	bindings := ordered.NewMap(0)
	for _, r := range b.Rules() {
		if b.Standing(r.ID) != "enforced" {
			continue
		}
		checks := []any{}
		for _, c := range ordered.Maps(r.Data.List("checks")) {
			if !b.Check(c).Holds {
				continue
			}
			m := ordered.NewMap(0)
			for _, k := range []string{"binding", "class", "tool", "key", "test", "tier", "fix"} {
				if v, ok := c.Get(k); ok {
					m.Set(k, v, 0)
				}
			}
			checks = append(checks, m)
		}
		bindings.Set(r.ID, checks, 0)
	}
	top := ordered.NewMap(0)
	top.Set("generated_by", GeneratedBy, 0)
	top.Set("bindings", bindings, 0)
	return ordered.JSON(top, jsonIndent) + "\n"
}

// coverageKeys are the counts coverage.json gives each part.
func coverageKeys() []string {
	return []string{"rules", "enforced", "planned", "advisory", "later", "retired", "proposed", "unsourced",
		"checks", "unresolved", "external_equal", "external_stricter"}
}

// coverageJSON counts each part's rules by standing, the proposed and the
// unsourced ones, its checks and those whose tool or key does not resolve,
// and the citations of external rules this rulebook covers.
func coverageJSON(b Book) string {
	byPart := map[string]map[string]int{}
	for _, r := range b.Rules() {
		c := byPart[r.Part]
		if c == nil {
			c = map[string]int{}
			byPart[r.Part] = c
		}
		countRule(b, r, c)
	}
	parts := ordered.NewMap(0)
	total := map[string]int{}
	for _, p := range b.Parts() {
		m := ordered.NewMap(0)
		for _, k := range coverageKeys() {
			m.Set(k, int64(byPart[p.Short][k]), 0)
			total[k] += byPart[p.Short][k]
		}
		parts.Set(p.Short, m, 0)
	}
	all := ordered.NewMap(0)
	for _, k := range coverageKeys() {
		all.Set(k, int64(total[k]), 0)
	}
	top := ordered.NewMap(0)
	top.Set("generated_by", GeneratedBy, 0)
	top.Set("total", all, 0)
	top.Set("parts", parts, 0)
	return ordered.JSON(top, jsonIndent) + "\n"
}

func countRule(b Book, r Rule, c map[string]int) {
	c["rules"]++
	c[b.Standing(r.ID)]++
	if r.Data.Str("status") == "proposed" {
		c["proposed"]++
	}
	if r.Data.Str("status") != "retired" && !r.Data.Truthy("sources") {
		c["unsourced"]++
	}
	for _, ch := range ordered.Maps(r.Data.List("checks")) {
		c["checks"]++
		if !b.Check(ch).Resolves {
			c["unresolved"]++
		}
	}
	for _, s := range ordered.Maps(r.Data.List("sources")) {
		if src := b.Registry().Sources.Sub(s.Str("ref")); src == nil || src.Str("kind") != "external-rule" {
			continue
		}
		switch s.Str("covers") {
		case "equal":
			c["external_equal"]++
		case "stricter":
			c["external_stricter"]++
		}
	}
}

func lockFile(b Book) string {
	return lockHeader + strings.Join(b.LockIDs(), "\n") + "\n"
}
