---
type: reason
status: current
as-of: 2026-11-16
---

# Why the gate rules are shaped as they are

Fifteen rules of the gates part (command, hooks, land, own-rules, plants and shared) rest on the reasons below. Each rule keeps its status. There is no CI on a push or a pull request: the gate runs locally, and a check too slow for the hook moves to the release, so a push that skips the gate moves its failure straight to main.

- **gates.command.12** (Hard, active): One script per repository drifts. A gate that stops at the first red shows one failure per push, so a change with three faults takes three pushes; accumulating shows all of them at once, and the summary still puts the cheap failure in front.
- **gates.command.4** (Hard, active): Three target repositories run the whole gate on every push, and a selector is a second program that can be wrong. Where the full gate does not fit, a hand-picked subset lets an author choose what proves their own change, so the subset comes from declared rules.
- **gates.command.8** (Hard, active): A release gate that can run without bound is an outage that nobody sees until it ends.
- **gates.hooks.1** (Hard, active): With no push CI, a push that skips the gate moves the failure straight to main. A hook that runs a subset leaves the rest of the gate with no tier at all.
- **gates.hooks.4** (Hard, active): Heavy hooks invited `--no-verify` at a reference team, so the time is measured, not guessed. With no push CI, the only place a slow check can go and still bind is the release.
- **gates.land.1** (Hard, active): A merge path that skips the check is the path that gets used. With no hosted land job, the binding between the tree that was merged and the run that passed it is that the run is made on that tree, just before the merge; a pass on one tree must never be claimed for another.
- **gates.land.12** (Hard, active): Two semantic conflicts passed every file-overlap tool and only train builds caught them.
- **gates.land.3** (Hard, retired): A self-reported proof was the fault: a lead reported 14 chosen tests green where the whole package would have failed 4. Agents declare done early and prove subsets; the known way round is re-running a chosen subset through the gate, so the record lists the selection and its reason.
- **gates.land.4** (Hard, active): A plan that is only a file is the author's claim; a recomputed hash makes it the gate's.
- **gates.own-rules.6** (Hard, active): A gate that imports its subject dies with it when the subject breaks, and judges the subject with the subject's own logic.
- **gates.plants.11** (Hard, active): A typo in the config silently disables nothing until a run proves it. A setting read from text says what somebody wrote, not what the running program does.
- **gates.plants.12** (Hard, active): A reference team went from 10 errors to 0 once its own scripts were gated; this project's scripts had no gate. Hosted minutes on every push are refused, so the local gate is the gate.
- **gates.plants.5** (Hard, active): A pending check exited 0 at a reference team; a secrets test skips when its scanner is missing; scanners that read nothing read green; a spec that cannot fail ran against nothing. A softening flag is the switch somebody turns on the day the gate is red.
- **gates.shared.7** (Hard, active): Three adoption issues were open with no end date.
- **gates.shared.8** (Hard, active): A drift check that is not in the gate fails 13 of 35 entries unseen. The scan is a `gatescan` rule. A gate list typed apart from the gate files loses the gate that was added last.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
