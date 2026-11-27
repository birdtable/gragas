---
type: reason
status: current
as-of: 2026-11-16
---

# Project templates stay in sync with one command and one gate

Projects start from a template and keep it up to date through one sync command, and the gate runs the same way on a developer's machine and at landing. The rules below follow from that choice. Each states its own reason, with the strength and status it has in the rulebook.

- **agents.instructions.5** (Hard, active): Agents run in directories that are not sibling clones.
- **arch.templates.1** (Hard, active): A template that does not record its version cannot be updated, and a project that has drifted from it cannot be told apart from one that chose to.
- **gates.command.1** (Hard, active): Separate entry points drift: a check is added to one and not the other, and a hook that runs only a fast subset reports green for what the land job would refuse.
- **gates.command.2** (Hard, active): A gate that needs a helper nobody installed is skipped silently on a fresh machine or in a clean clone. Five of seven Go repos have a tracked pre-push hook, two have none.
- **gates.exemptions.1** (Hard, active): 2,435 live nolint lines in Go with no exit record; the shared nolintlint is enabled nowhere. One repo has 662 suppression lines, 318 outside tests. A reference trial counted 548 and 624 nolintlint findings in two repos, mostly test files.
- **gates.hooks.1** (Hard, active): With no push CI, a push that skips the gate moves the failure straight to main. A hook that runs a subset leaves the rest of the gate with no tier at all.
- **gates.hooks.2** (Hard, active): Five Go repos have the hook and five different versions of it. An unarmed repo looks the same as an armed one until a bad push goes through.
- **gates.shared.1** (Hard, active): 5 distinct lint configs among 6 repos.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
