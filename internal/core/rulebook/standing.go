package rulebook

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port/legacy"
)

// The standings a rule can have. Standing is derived,
// never written: it follows from status, strength, kind and whether a check
// holds the rule through a tool and a key that resolve.
const (
	// Enforced is an active rule that binds and that a check holds, its tool
	// chosen and its key found in the tool's preset.
	Enforced = "enforced"
	// Planned is a rule that binds or is proposed, with no check that holds it yet.
	Planned = "planned"
	// Advisory is an instruction, a measure, a Review rule, or a Lens or
	// Record rule not yet promoted: read, not held by a tool.
	Advisory = "advisory"
	// Later is a rule that waits for its when.
	Later = "later"
	// Retired is a rule that is no longer in force; its id stays.
	Retired = "retired"
)

// Standings are the standings, in report order.
func Standings() []string { return []string{Enforced, Planned, Advisory, Later, Retired} }

// testedClasses are the check classes whose check is our own code, so they
// hold a rule only once their test exists.
func testedClasses() []string { return []string{"pattern", "analyzer", "policy", "fact"} }

// standardLinters are the linters golangci-lint runs when a preset says
// default: standard.
func standardLinters() []string {
	return []string{"errcheck", "govet", "ineffassign", "staticcheck", "unused"}
}

// tool is one tools.yaml entry, and its preset when it names one.
type tool struct {
	part   string
	data   *ordered.Map
	preset *ordered.Map
}

// resolves reports whether key names something the tool's preset holds: a
// linter it enables, or a setting by its dotted path.
func (t *tool) resolves(key string) bool {
	if t.preset == nil || key == "" {
		return false
	}
	if l := t.preset.Sub("linters"); l != nil {
		if slices.Contains(ordered.Strings(l.List("enable")), key) {
			return true
		}
		if l.Str("default") == "standard" && slices.Contains(standardLinters(), key) {
			return true
		}
	}
	var cur any = t.preset
	for seg := range strings.SplitSeq(key, ".") {
		m, ok := cur.(*ordered.Map)
		if !ok {
			return false
		}
		if cur, ok = m.Get(seg); !ok {
			return false
		}
	}
	return true
}

// loadPresets reads the preset of each tool that names one.
func (b *Book) loadPresets(src legacy.Source) {
	for _, short := range b.toolOrder() {
		t := b.tools[short]
		p := t.data.Str("preset")
		if p == "" {
			continue
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			b.add(path.Join(Dir, "tools", t.data.Str("key")), finding{msg: "preset " + p + ": " + err.Error()})
			continue
		}
		v, err := ordered.ParseYAML(data)
		if m, ok := v.(*ordered.Map); err == nil && ok {
			t.preset = m
			continue
		}
		b.add(p, finding{msg: "preset is not a YAML mapping"})
	}
}

func (b *Book) toolOrder() []string {
	keys := make([]string, 0, len(b.tools))
	for k := range b.tools {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Binds reports whether a rule binds: an active rule of a binding strength,
// or of a measured strength once promoted. Other statuses never bind.
func Binds(r *ordered.Map) bool {
	if r.Str("kind") != "rule" || r.Str("status") != "active" {
		return false
	}
	s := r.Str("strength")
	return slices.Contains(bindingStrengths(), s) || (slices.Contains(measuredStrengths(), s) && r.Truthy("promoted"))
}

// advisory is whether a rule is read rather than held by a tool.
func advisory(r *ordered.Map) bool {
	s := r.Str("strength")
	return r.Str("kind") != "rule" || s == "Review" || (slices.Contains(measuredStrengths(), s) && !r.Truthy("promoted"))
}

// Standing is a rule's derived standing.
func (b *Book) Standing(id string) string { return b.standing[id] }

func (b *Book) standingOf(r *ordered.Map) string {
	switch r.Str("status") {
	case "retired":
		return Retired
	case "later":
		return Later
	}
	if advisory(r) {
		return Advisory
	}
	if Binds(r) && slices.ContainsFunc(ordered.Maps(r.List("checks")), b.Holds) {
		return Enforced
	}
	return Planned
}

// Holds reports whether a check holds its rule: its tool is in the rulebook
// and chosen, its key resolves in the tool's preset, and a check of our own
// code has its test.
func (b *Book) Holds(c *ordered.Map) bool {
	if !b.Resolves(c) {
		return false
	}
	if slices.Contains(testedClasses(), c.Str("class")) {
		return b.testExists(c)
	}
	return true
}

// Resolves is whether a check's tool and key both resolve.
func (b *Book) Resolves(c *ordered.Map) bool {
	t, ok := b.tools[c.Str("tool")]
	return ok && t.data.Str("state") == "chosen" && t.resolves(c.Str("key"))
}

func (b *Book) testExists(c *ordered.Map) bool {
	p := c.Str("test")
	if p == "" {
		return false
	}
	_, err := fs.Stat(b.src, p)
	return err == nil
}

// derive computes every rule's standing and the baseline counts.
func (b *Book) derive() {
	for _, inv := range Invariants() {
		b.Counts[inv.Name] = 0
	}
	for _, id := range b.IDs() {
		r := b.Rules[id].Data
		b.standing[id] = b.standingOf(r)
		for _, inv := range Invariants() {
			b.Counts[inv.Name] += inv.count(b, r)
		}
	}
}

// loadLock reads ids.lock, when there is one, and reports a locked id that
// is no longer in the rulebook: ids are frozen.
func (b *Book) loadLock(src legacy.Source) error {
	p := path.Join(Dir, LockFile)
	data, err := fs.ReadFile(src, p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("read %s: %w", p, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		id := strings.TrimSpace(line)
		if id == "" || strings.HasPrefix(id, "#") {
			continue
		}
		b.Lock = append(b.Lock, id)
		if _, ok := b.Rules[id]; !ok {
			b.add(p, finding{line: i + 1, msg: fmt.Sprintf("%s was issued and is gone: an id is retired with replaced_by, never removed", id)})
		}
	}
	slices.Sort(b.Lock)
	return nil
}

// LockIDs is every id ever issued: the lock and the ids in the rulebook now.
func (b *Book) LockIDs() []string {
	out := slices.Concat(b.Lock, b.IDs())
	slices.Sort(out)
	return slices.Compact(out)
}
