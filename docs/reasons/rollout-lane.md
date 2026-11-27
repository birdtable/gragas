---
type: reason
status: current
as-of: 2026-11-16
---

# Shared modules are few, owned and released together

Shared code lives in a small number of modules, each with a named owner, and a module's tags are cut together. The rules below follow from that. Each states its own reason, with the strength and status it has in the rulebook.

- **arch.shared.1** (Hard, active): Every extra shared module is another place a copy can hide and another owner to find.
- **arch.shared.14** (Hard, active): Two shared UI sources have no owner file; a shared source with no one to ask is abandoned the first time it needs a change.
- **supply.bumps.3** (Hard, active): The sub-module tags came about 64 minutes and two commits after the root tag, so the pair was not atomic.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
