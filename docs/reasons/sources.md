---
type: reason
status: current
as-of: 2026-11-16
---

# Privilege is the boundary that splits commands and helpers

Two rules, both still proposed, rest on privilege as the boundary. Each states its own reason, with the strength and status it has in the rulebook.

- **cli.commands.27** (Hard, proposed): Privilege is the boundary that matters to a person deciding what to run; an operating-system split puts a root action beside an owner one on the next platform.
- **daemon.access.8** (Hard, proposed): A privileged helper is the one place a bug becomes root. The shape is settled, but the design it encodes is still proposed, and a rule ahead of its own design is premature.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
