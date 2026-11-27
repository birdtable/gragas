---
type: board
status: current
as-of: 2026-11-09
---

# Findings

A listing of findings is a table, one row each, the worst first. A notice is listed apart and never changes the word. Every finding names what it is about and the fix that is offered.

```text
check

reading .gragas/pin.yaml in the project
pinned to the base rulebook at 1.2.0

Findings
  rule          about            fix
  go.errors.1   src/a.go         wrap the error with its context
  cli.look.5    src/b.go         end the run on a closing word

Notices
  1 preset is older than the pin. gragas upgrade shows what changes.

  findings   2 findings
    gragas check --json
```

Ends: findings, exit 1.
Answered by: nothing. `--json` gives the same listing as one document.

`--sarif`, on a command that lists findings, writes every finding and notice as one SARIF document on stdout instead, a notice at level note. The closing word goes to stderr, and stdout stays empty when there is nothing to list from, such as a rulebook that cannot be read. `--json` and `--sarif` together are a usage error.

Each result of `gragas validate --sarif` carries the id `gragas.validate.1`. It is the validate check's own id, not a rulebook rule: no part of the rulebook is named gragas, so no rule's id can be it.
