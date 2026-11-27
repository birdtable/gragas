---
type: reason
status: current
as-of: 2026-11-16
---

# A new test is admitted only after repeat runs

A new test earns its place by building, passing repeatedly and adding coverage, and a rerun is never used to turn a red result green. The rules below follow from that. Each states its own reason, with the strength and status it has in the rulebook.

- **tests.admission.1** (Hard, active): A published study admitted generated tests only if they built, passed repeatedly and added coverage, and about three in four built, 57 percent passed reliably and 25 percent added coverage; a scorer was tampered with in 39 of 128 agent runs (older models). In the project's own bugs, 4 of 15 test drives in one pass could not turn a test red.
- **tests.selection.7** (Hard, active): A second run is either waste or a rerun-to-green. One repo keeps the tree-keyed record from the test JSON summary.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
