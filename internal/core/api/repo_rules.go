package coreapi

import (
	"github.com/birdtable/gragas/internal/core/audit"
)

// RepoBinding is one active preset-rule check: the rule it serves and the tool
// and key it names.
type RepoBinding struct{ Rule, Tool, Key string }

// RepoRulesOf is the rule each key of the gragas preset is checked under, read
// from the active preset-rule bindings of the parts the repository names: the
// rulebook says which rule a repository check serves, and the core names none.
// It reads no file, so a caller that has the bindings of any format hands them
// in. A repository whose rulebook binds no such key has no rules, and its
// checks that need one do not run.
func RepoRulesOf(bs []RepoBinding) map[string]string {
	rules := map[string]string{}
	for _, b := range bs {
		if b.Tool == audit.Program {
			rules[b.Key] = b.Rule
		}
	}
	return rules
}

// Countings are what the count check reads of each tool whose readers row
// says it: the names of its config, at any depth, the key paths
// of its exclusion lists and its directive word. The row holds them until the
// tool's plugin gives them in its manifest's counted.
func Countings(rows []Reader) []Counting {
	var out []Counting
	for _, r := range rows {
		if len(r.Exclusions) > 0 || len(r.Loosening) > 0 || r.Directive != "" {
			out = append(out, Counting{Configs: r.Files, Exclusions: r.Exclusions, Loosening: r.Loosening, Directive: r.Directive, FoldKeys: r.FoldKeys})
		}
	}
	return out
}
