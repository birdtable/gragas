package audit

import (
	"cmp"
	"slices"
)

// The checks of this file's neighbours read a repository's tracked files, its
// history and its modules, and serve one rule each. Each is a
// function over data the layer above reads: none runs a program, touches the
// network or writes a file, and each reports a Finding in the shape of the
// config checker's, with the program gragas and the check's name as its tool
// and key.

// The keys of the gragas preset these checks compute: each is the key a rule's
// check row with tool gragas names, and the key a finding is reported under.
const (
	KeyLink      = "link"
	KeyFold      = "fold"
	KeyNames     = "names"
	KeyReplace   = "replace"
	KeyDebt      = "debt"
	KeyDirective = "directive"
)

// The names a check is called by when it could not be made: not a rule's key,
// for one check judges several rules (tracked: link and fold; counts: debt and
// directive).
const (
	// KeyTracked and KeyCounts name the tracked-files check and the count check.
	KeyTracked = "tracked"
	KeyCounts  = "counts"
)

// Computed says whether key is one of the gragas preset's keys these checks
// compute: no config file holds it, so the config readers leave it alone.
func Computed(key string) bool {
	return slices.Contains(ComputedKeys(), key)
}

// ComputedKeys are the keys Computed says yes to, in the order they are
// declared: the list the gragas preset's computes keys are held equal to.
func ComputedKeys() []string {
	return []string{KeyLink, KeyFold, KeyNames, KeyReplace, KeyDebt, KeyDirective}
}

// RepoRules are the rule ids the checks report under, by the key of the gragas
// preset each computes. The rulebook says them (a rule's check row with tool
// gragas and the key); the core names no rule.
type RepoRules map[string]string

// Has says whether every key has a rule.
func (r RepoRules) Has(keys ...string) bool {
	for _, k := range keys {
		if r[k] == "" {
			return false
		}
	}
	return true
}

// Program is what a finding of these checks names as its tool.
const Program = "gragas"

// found is one finding of a repository check.
func found(rules RepoRules, key, file string, line int, missing string) Finding {
	return Finding{Rule: rules[key], Tool: Program, Key: key, File: file, Line: line, Missing: missing}
}

// Unjudged says a check could not be made, and why: not a finding and not a
// pass. Name is the check's key.
func Unjudged(key, why string) NotChecked {
	return NotChecked{Tool: Program + " " + key, Why: why}
}

// sorted sorts findings by file, line, rule and message, so a run says the
// same findings in the same order whatever order its reads came in.
func sorted(fs []Finding) []Finding {
	slices.SortStableFunc(fs, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Rule, b.Rule), cmp.Compare(a.Missing, b.Missing))
	})
	return fs
}
