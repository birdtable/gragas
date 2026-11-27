---
type: reason
status: current
as-of: 2026-11-16
---

# Why a gate must not pass on no input

Three rules (gates.plants.8, supply.licences.2 and supply.provenance.4) rest on one practice: a check that has nothing to look at reports a failure or a skip, never a pass. Each keeps its status.

- **gates.plants.8** (Hard, active): A reference gate did not run its vulnerability scan; a gate skipping a step was a regret.
- **supply.licences.2** (Hard, active): A licence check that passes on nothing found passes on a broken install.
- **supply.provenance.4** (Hard, active): A stale generated file passes review because nobody regenerates it. None of the seven Go repositories checked used go:generate, which the count shows, with the build-constraint lines as the positive.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
