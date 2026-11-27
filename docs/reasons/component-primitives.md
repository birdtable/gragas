---
type: reason
status: current
as-of: 2026-11-16
---

# React widgets come from a primitives library

Interactive widgets such as comboboxes and date pickers come from a component primitives library, not from hand-built code.

- **react.components.2** (Hard, active): Hand-built widgets drift from the accessibility contract: a hand-built combobox was 416 lines and a date picker 371 lines. The chosen library gave zero violations under the script and style policy.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
