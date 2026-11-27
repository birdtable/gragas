---
type: reason
status: current
as-of: 2026-11-16
---

# Why someone other than the author signs off a weakened gate

Seven rules (agents, gates and supply) rest on one practice: the author of a change does not alone approve a change that weakens a gate or widens the supply-chain surface. Each rule keeps its status.

- **agents.guards.1** (Hard, active): Scorer tampering was seen in 39 of 128 runs in a published study of agent behaviour; no repository of this project committed a .claude/ file (0 tracked in seven). One Claude Code release treated a Write(...) deny as nothing, found only by a live test.
- **agents.review.1** (Hard, active): Argued: no study supports AI review as a verdict. The project's own evidence is that review finds faults: one project's designer agent found 31 of 72 and another's reviewers 16, and a spec-against-diff lens found a missing case that tests had not.
- **agents.review.2** (Lens, active): A lens given more than the diff and the card can be steered by the author's account of the change, and a verdict tied to a sha goes stale on a rebase while the content is unchanged.
- **gates.protection.1** (Hard, retired): No Gate-Change trailer exists in any of seven Go repositories (0, verified), and nothing stores such a verdict. A sign-off passed as a land argument stops a missing and a self sign-off but not a land-runner who names someone falsely.
- **gates.protection.2** (Hard, retired): 0 mentions in any repository today, no grammar and no list.
- **gates.protection.3** (Hard, active): One web repository's own check runs but its reviewer condition skips any path with no reviewer, and its scripts folder has none. Three reports asked for this with no row in the first merge.
- **supply.intake.4** (Lens, active): Adding a dependency is the largest single change of supply-chain surface; one author should not make it alone.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
