---
type: reason
status: current
as-of: 2026-11-16
---

# Why releases come from a tag and not from scheduled or push-triggered runs

Three rules (supply.release.1, supply.workflows.1 and tests.selection.13) rest on one practice: the tag is the release and the local gate is the gate, with no nightly runs and no hosted minutes on every push. Each rule keeps its status.

- **supply.release.1** (Hard, active): A shared package can otherwise publish from a laptop with whatever the laptop holds, and an install from an untagged tree is a version nobody can name. The invariant three target repositories share is one recipe from a clean tagged main after the full gate; CI is the only place that holds the publish token.
- **supply.workflows.1** (Hard, active): Hosted minutes on every push are refused: the tag is the release and the local gate is the gate. Neither the workflow linter nor the security audit checks the trigger set, so a push trigger went unseen.
- **tests.selection.13** (Hard, active): A release is a claim about one tree, so it either proves that tree or shows a verdict that already did. Where selection is on, the release is exactly as safe as the map is complete, so the plan is recorded with the tag.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
