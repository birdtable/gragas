---
type: reason
status: current
as-of: 2026-11-16
---

# How the shared module is released and bumped

The shared module is released on its own, and consumers bump to a new version in a change of their own. The rules below follow from that. Each states its own reason, with the strength and status it has in the rulebook.

- **arch.shared.15** (Hard, proposed): Measured: the atomic write exists six times in one Go program and the digest of a file twelve times, each added by a builder who did not see the earlier one; the shared module's own instruction file states the second-copy rule, and nothing in this program enforced it.
- **cli.exit.1** (Hard, active): Measured today: one program uses 0, 1, 2, 64, 69, 75 and 78; the shared module uses 0 to 4 with 1 for both refused and failed; three others use 1 for everything; one TypeScript program uses three codes with refusal and bad usage both 1.
- **supply.bumps.1** (Hard, active): A bump mixed with source edits hides the change a reader should see, and a bump can ride inside a large refactor unseen. The known way round is hiding the bump in a refactor, which the same scan catches.
- **supply.bumps.2** (Hard, active): A consumer that lags keeps running code the owner has already replaced, so the lag is shown. A fixed window forces every consumer to the newest version on the owner's schedule, which arch.measures.6 itself names as a cause of coupling; fixing callers in the breaking change is what keeps a lagging consumer safe to move later. A pair that must always release together signals a misplaced boundary.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
