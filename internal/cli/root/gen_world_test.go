package root

import (
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

const genIdentity = `format: 3
name: example
version: 1.0.0
kind: base
licences:
  rules: CC-BY-4.0
language: en
owners: [team-a]
governance:
  contribution: open
  decision_required: false
  trial_releases: 1
  review_days: 30
`

const genStructure = `format: 3
layers:
  - slug: language
    name: Language
    parts:
      - slug: go
        name: Go
        concerns:
          - slug: errors
            name: Errors
kinds: []
`

const genRules = `format: 3
part: go
concern: errors
rules:
  - id: go.errors.1
    statement: Wrap an error.
    strength: must
    status: proposed
    origin: authored
`

// seedGenBook is a format-3 rulebook in memory with no rules.json, at each of the
// folders a test names with -C and at the working folder, so gen finds the one
// generated file stale. gen reads the book through this world and writes
// through the legacy fake.
func seedGenBook(w *porttest.World, dirs ...string) {
	w.SeedSchemas("v3/identity.schema.json", "v3/structure.schema.json", "v3/rules.schema.json")
	for _, d := range append([]string{"."}, dirs...) {
		w.Seed(d, map[string]string{"rulebook/rulebook.yaml": genIdentity, "rulebook/structure.yaml": genStructure, "rulebook/language/go/rules/errors.yaml": genRules})
	}
}
