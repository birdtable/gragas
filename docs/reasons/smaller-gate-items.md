---
type: reason
status: current
as-of: 2026-11-16
---

# Two smaller gate items

Two rules cover smaller items of the gate: how the repository is searched, and which toolchain the gate runs on. Each states its own reason, with the strength and status it has in the rulebook.

- **agents.discovery.3** (Hard, active): A language server found 28 of 28 callers of one function where grep found the same plus 4 fixture strings; limited views of the repository cause reinvention.
- **supply.install.9** (Hard, active): A gate that ran on a different toolchain than the lockfile names proves nothing about the shipped build. One tool version per estate removes a class of drift.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
