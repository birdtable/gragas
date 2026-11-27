---
type: reason
status: current
as-of: 2026-11-16
---

# A workflow's app token must not write everywhere

The app token a workflow job uses must not be able to write to every repository.

- **supply.workflows.3** (Hard, active): A token that can write to every repository turns a compromised job into a compromise of the estate.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
