---
type: reason
status: current
as-of: 2026-11-16
---

# Why selection is proved and backed by a full run

Fourteen rules (gates, quality, routing, tests and selection) rest on one principle: each test the gate skips is proved safe to skip, and a full run stands behind any selection. Each rule keeps its status.

- **gates.command.4** (Hard, active): Three target repositories run the whole gate on every push, and a selector is a second program that can be wrong. Where the full gate does not fit, a hand-picked subset lets an author choose what proves their own change, so the subset comes from declared rules.
- **gates.command.5** (Hard, active): A selector that guesses on a path it cannot place proves a subset and calls it green; a lead once reported 14 chosen tests green where the package would have failed 4.
- **gates.land.7** (Hard, active): A full suite at every land misses the budget; selection without a backstop misses what selection missed.
- **gates.land.8** (Hard, active): A red nobody stops compounds; the miss that got through selection teaches the planner only if it is kept.
- **gates.quality.3** (Review, active): A main that is sometimes red teaches everyone to retry, and a retry hides a real fault.
- **react.routing.10** (Review, active): A rendering mode that is not yet needed adds a server, an adapter and a failure surface nobody has measured.
- **tests.admission.9** (Review, active): A switch only ever exercised in its default state is dead code on one side.
- **tests.generated.1** (Hard, active): Practice with fuzz targets already in the repositories: 25 in one, 2 and 1 in two others.
- **tests.generated.2** (Review, active): Generated inputs find the case the author did not write; a property is the stated invariant. The web catalogue has no row for pure UI logic such as formatters and parsers; the rule fits them.
- **tests.isolation.2** (Hard, active): Shared data couples tests through order and timing; one helper is the only place isolation is proved once. The reference practice has one helper per store kind; the Go side gets the same effect from a per-test home and temp folder plus shuffled order (tests.isolation.10).
- **tests.selection.1** (Hard, active): Selection is a tool for a suite too slow to run whole, and its only failure mode is a missed dependency. A full uncached run has no such failure and is the claim every target makes, so it is the default, and the selection rules are kept for the day a suite outgrows its budget.
- **tests.selection.12** (Hard, active): Selection is exactly as safe as its map is complete; the backstop is how a miss is found and turned into a fixture.
- **tests.selection.13** (Hard, active): A release is a claim about one tree, so it either proves that tree or shows a verdict that already did. Where selection is on, the release is exactly as safe as the map is complete, so the plan is recorded with the tag.
- **tests.selection.9** (Hard, active): Each skip must be proved; without a per-skip record the claim is unproved. One repository records only counts of packages run, skipped and cached.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
