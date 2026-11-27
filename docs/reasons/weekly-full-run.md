---
type: reason
status: current
as-of: 2026-11-16
---

# A release proves one tree

A release is a claim about one tree. It either proves that tree in full or shows a verdict that already did, and where selection is on, the plan is recorded with the tag.

- **supply.release.1** (Hard, active): A shared package can otherwise publish from a laptop with whatever the laptop holds, and an install from an untagged tree is a version nobody can name. The invariant three target repositories share is one recipe from a clean tagged main after the full gate; CI is the only place that holds the publish token.
- **supply.workflows.1** (Hard, active): The project does not spend hosted minutes on every push: the tag is the release and the local gate is the gate. Neither the workflow linter nor the security audit checks the trigger set, so a push trigger went unseen.
- **tests.selection.12** (Hard, active): Selection is exactly as safe as its map is complete; the backstop is how a miss is found and turned into a fixture.
- **tests.selection.13** (Hard, active): A release is a claim about one tree, so it either proves that tree or shows a verdict that already did. Where selection is on, the release is exactly as safe as the map is complete, so the plan is recorded with the tag.
- **tests.selection.9** (Hard, active): Each skip must be proved; without a per-skip record the claim is unproved. One repo records only counts of packages run, skipped and cached.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
