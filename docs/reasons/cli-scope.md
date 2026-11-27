---
type: reason
status: current
as-of: 2026-11-16
---

# The land check runs on the tree that is merged

A merge is checked by a run made on the tree being merged, just before the merge. A pass on one tree is never claimed for another.

- **gates.land.1** (Hard, active): A merge path that skips the check is the path that gets used. With no hosted land job, the binding between the tree that was merged and the run that passed it is that the run is made on that tree, just before the merge; a pass on one tree must never be claimed for another.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
