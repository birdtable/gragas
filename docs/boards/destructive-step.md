---
type: board
status: current
as-of: 2026-11-08
---

# Destructive step

A step that is hard to take back asks twice: the yes, then the name of the thing, typed. Withdrawing a release is one. A script gives both answers as flags: `--apply` for the yes and `--confirm` followed by the name. Without a terminal and without both, the plan prints and the command ends declined.

## At a terminal

```text
withdraw 1.1.0

reading the history of the rulebook
release 1.1.0 is published and 14 projects pin it

What changes
  release   before      after
  1.1.0     published   withdrawn

Written
  the withdrawn list          one entry added, with the reason below

Reason: the release was published with a secret in it

Plan digest sha256:91c07d5e
Apply this plan? [y/N] y
This cannot be taken back. Type the release to go on: 1.1.0

  done   release 1.1.0 is withdrawn
```

Ends: done, exit 0.
Answered by: `--apply` answers the first question and `--confirm 1.1.0` answers the second.

## A name that does not match

```text
withdraw 1.1.0

Plan digest sha256:91c07d5e
Apply this plan? [y/N] y
This cannot be taken back. Type the release to go on: 1.1

  declined   the name typed was not 1.1.0 and nothing was changed
    gragas withdraw 1.1.0 --apply --confirm 1.1.0
```

Ends: declined, exit 6.
Answered by: `--confirm`.
