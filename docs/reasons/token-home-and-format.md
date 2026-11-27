---
type: reason
status: current
as-of: 2026-11-16
---

# One shared module for design tokens

Design tokens live in one shared module. A rule of the architecture part holds the line on how many shared modules there are.

- **arch.shared.1** (Hard, active): Every extra shared module is another place a copy can hide and another owner to find. The shared module for tokens exists.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
