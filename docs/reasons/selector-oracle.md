---
type: reason
status: current
as-of: 2026-11-16
---

# Test selection is checked by an oracle

When only the tests a change can affect are run, an oracle checks that selection. The rule below follows from that. It states its own reason, with the strength and status it has in the rulebook.

- **tests.selection.10** (Hard, active): Selection has one failure mode, a missed dependency, and the oracle is the only check on it. The claim that each skip is proved is unproved until the oracle runs; it learns dependencies from deletions only.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
