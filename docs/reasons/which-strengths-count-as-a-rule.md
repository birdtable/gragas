---
type: reason
status: current
as-of: 2026-11-16
---

# Which strengths count as a rule

A rule has a strength: Hard, Review, Lens and the like. What counts as a rule depends on whether an instrument can check it and whether that instrument can fail. The rules below show the line.

- **agents.review.3** (Hard, active): An instrument that cannot fail carries no information, and a lens is a prompt that can drift with a model change.
- **react.client-state.1** (Lens, active): Two stores for client state drift apart, and state in context re-renders every consumer. Half of the rule holds by package ban; the context half is a model lens.
- **ts.compiler.6** (Hard, later): Running both compilers and failing on a disagreement is the safety; a window that depends on a scheduled tier cannot be met when gates run only at the push and the release. Until the native release is stable nothing can fail, so the rule waits.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
