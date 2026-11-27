---
type: reason
status: current
as-of: 2026-11-16
---

# Interface widgets come from shadcn, not by hand

Complex widgets such as comboboxes and date pickers come from the shadcn component library, not from hand-built code. The rule below follows from that. It states its own reason, with the strength and status it has in the rulebook.

- **react.components.2** (Hard, active): Hand-built widgets drift from the accessibility contract: in the source a hand-built combobox was 416 lines and a date picker 371 lines. The chosen library gave zero violations under the script and style policy.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
