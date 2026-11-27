---
type: reason
status: current
as-of: 2026-11-16
---

# A person signs off the visual rubric

Automated review lenses find faults, but a person decides. The visual rubric is signed off by a person who looks at what a lens only flags.

- **agents.review.1** (Hard, active): Argued: no study supports AI review as a verdict. The project's own evidence is that review finds faults: one project's designer agent found 31 of 72 and another's reviewers 16, and a spec-against-diff lens found a missing case that tests had not.
- **agents.review.2** (Lens, active): A lens given more than the diff and the card can be steered by the author's account of the change, and a verdict tied to a sha goes stale on a rebase while the content is unchanged.
- **web.delivery.5** (Hard, active): A person looks at what the lens only flags.
- **web.testing.9** (Record, active): A blanket update is the cheap cure for a human-read golden, and only the count catches it. The hosted pixel service tried is archived, and the free tier of another is Chrome only.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
