---
type: reason
status: current
as-of: 2026-11-16
---

# What runs at land

At land, the whole gate runs where it fits. Where it does not fit, a selector chooses a subset from declared rules, and a backstop catches what selection missed. A main that is sometimes red is not accepted.

- **gates.command.4** (Hard, active): Three target repos run the whole gate on every push, and a selector is a second program that can be wrong. Where the full gate does not fit, a hand-picked subset lets an author choose what proves their own change, so the subset comes from declared rules.
- **gates.command.5** (Hard, active): A selector that guesses on a path it cannot place proves a subset and calls it green; a lead once reported 14 chosen tests green where the package would have failed 4.
- **gates.land.7** (Hard, active): A full suite at every land misses the budget; selection without a backstop misses what selection missed.
- **gates.land.8** (Hard, active): A red nobody stops compounds; the miss that got through selection teaches the planner only if it is kept.
- **gates.quality.3** (Review, active): A main that is sometimes red teaches everyone to retry, and a retry hides a real fault.
- **tests.journeys.1** (Hard, active): End-to-end tests must be scoped. One repo's browser reach map selected 7 of 41 journey files for a one-card change; none of the repos declares inputs per journey, and the land step runs the whole acceptance suite.
- **tests.selection.1** (Hard, active): Selection is a tool for a suite too slow to run whole, and its only failure mode is a missed dependency. A full uncached run has no such failure and is the claim every target makes, so it is the default, and the selection rules are kept for the day a suite outgrows its budget.
- **tests.selection.10** (Hard, active): Selection has one failure mode, a missed dependency, and the oracle is the only check on it. The promise that each skip is proved stays unproved until the oracle runs; it learns dependencies from deletions only.
- **tests.selection.2** (Hard, active): Held today in two repos (6 of 6 unplaced paths select everything in one, 15 of 15 and 87 of 87 in the trialled selector); every other path is a guess.
- **tests.selection.9** (Hard, active): Each skip must be proved; without a per-skip record the claim is unproved. One repo records only counts of packages run, skipped and cached.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
