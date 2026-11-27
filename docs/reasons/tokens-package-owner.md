---
type: reason
status: current
as-of: 2026-11-16
---

# A shared package needs a named owner

A shared package, such as the tokens package, has a named owner, and a change to it is approved by that owner. The rules below hold that line.

- **arch.decisions.3** (Review, active): A change settled by the wrong person is not settled; naming who settled it in the record is what lets a later reader trust it.
- **arch.shared.1** (Hard, active): Every extra shared module is another place a copy can hide and another owner to find. The shared module exists.
- **arch.shared.14** (Hard, active): Two shared UI sources have no owner file; a shared source with no one to ask is abandoned the first time it needs a change.
- **supply.bumps.3** (Hard, active): The sub-module tags came about 64 minutes and two commits after the root tag, so the pair was not atomic.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
