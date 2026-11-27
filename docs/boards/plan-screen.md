---
type: board
status: current
as-of: 2026-11-08
---

# Plan screen

A plan has the same sections in every command, in this order: what changes, consumer actions, what is written where, what is re-keyed or orphaned, renderings rewritten, checker-code changes, the question. A section with nothing to say is left out, never printed empty. The first screen is a plan at a terminal where the person says yes. The second is the same plan with no terminal and no flag.

## The plan at a terminal

```text
upgrade

reading .gragas/pin.yaml in the project
pinned 1.1.0, newest 1.2.0

What changes
  rule          before    after
  go.errors.1   proven    unproven
  cli.look.5    proven    proven

Consumer actions
  Run gragas check once the pin has moved.

Written
  .gragas/pin.yaml    the pin moves to 1.2.0
  .gragas/lock.yaml   one entry added with this plan's digest

Re-keyed or orphaned
  waiver old-code   re-keyed to go.errors.2

Renderings rewritten
  AGENTS.md   3 lines change

Checker code
  one artifact changes, diff at https://example.org/diff/1.1.0...1.2.0

Plan digest sha256:3f2a9c41
Apply this plan? [y/N]
```

Ends: done, exit 0. This is the ending after a yes.
Answered by: `--apply` answers the question with yes. A person at a terminal types y.

## The same plan with no terminal and no flag

```text
upgrade

reading .gragas/pin.yaml in the project
pinned 1.1.0, newest 1.2.0

What changes
  rule          before    after
  go.errors.1   proven    unproven
  cli.look.5    proven    proven

Written
  .gragas/pin.yaml    the pin moves to 1.2.0

This was a dry run and nothing was changed. Add --apply to make these changes.

  declined   a plan was shown and not applied
    gragas upgrade --apply
```

Ends: declined, exit 6.
Answered by: `--apply`.
