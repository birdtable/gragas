---
type: reason
status: current
as-of: 2026-11-16
---

# Test rules and the choices behind them

The tests part of the rulebook covers what a test must prove, how doubles are made, how tests stay isolated, how long a suite may run and how tests are selected. The default is a full, uncached run at every claim; selection is a tool kept for the day a suite outgrows its budget. This page gives the reason for each rule.

## The rules

- **tests.admission.2** (Hard, active): Agents are the likeliest authors of weak assertions, and an empty loop is the second way a suite goes vacuous: it asserts nothing and passes. A guard with no negative test is an opinion. The cost line says what the signature cannot: why the test exists. Trial on the Go side of the assertion check: 11 hits in 5,767 tests, 0 to 1 true; the false ones trace to contract suites, table values, helper processes and skip-only tests. Low yield, kept because it costs nothing.
- **tests.admission.5** (Hard, active): A skip in a default tier is a test that silently stopped existing. Skips per test across the measured repositories, raw and including environment skips: 102 of 1,645, 74 of 1,731, 68 of 1,020, 38 of 1,301, 25 of 271, 13 of 465.
- **tests.doubles.3** (Hard, active): The usual line from the testing literature: mock only the edge you do not control, and do not assert on calls that are queries. The file turns a ban of named paths into a check per dependency.
- **tests.doubles.4** (Hard, active): A fake that drifts from the engine hides the bug; a contract test per fake proves it against the real one. A fake that answers an unseeded call with a default makes a test pass for a reason the author never chose.
- **tests.doubles.5** (Hard, active): A hand-written stub encodes what the author thought the server said, and recorded bytes freeze what it said once; the real program on a fake account says what it says today. In the reference practice, one fake source serves the whole repo.
- **tests.doubles.6** (Hard, active): Fixtures generated from samples run the wrong direction: goldens are the handler's bytes and the schema is a second check. Generators' defaults broke "null, never 0". Faults of this kind: 11, 6 and 6 across three repositories. A single example set hides what one platform answers differently.
- **tests.doubles.7** (Hard, active): Real names recurred in fixtures twice, and a real host, address or person in a public file cannot be taken back. A stand-in list catches the name nobody thought to put on a roster; the roster catches the known ones in places the stand-in list does not cover.
- **tests.isolation.13** (Hard, active): A test that imports across modules passes for a module nobody could extract, a suite that never loads its subject proves nothing, and a test that writes another module's rows encodes that module's schema in a place its owner cannot see.
- **tests.isolation.8** (Hard, active): A test that registers a service, edits the real git config, drives the person's browser or kills a process by name damages the host it runs on; one repo's integration tests need root and write under a system folder. An opt-in tier that must touch the host is held to names it owns and a cleanup it proves, so a failed run leaves nothing behind.
- **tests.measures.2** (Hard, active): A suite written by agents cannot be allowed to grow unnoticed to hours. One repo's browser journeys alone take 35 minutes serial for one family of them (not re-timed).
- **tests.runners.1** (Hard, active): A test that is not in the gate does not exist for the agent; unit tests are the fastest tier.
- **tests.selection.1** (Hard, active): Selection is a tool for a suite too slow to run whole, and its only failure mode is a missed dependency. A full uncached run has no such failure and is the claim every target makes, so it is the default, and the selection rules are kept for the day a suite outgrows its budget.
- **tests.selection.10** (Hard, active): Selection has one failure mode, a missed dependency, and the oracle is the only check on it. The promise that each skip is proved stays unproved until the oracle runs; it learns dependencies from deletions only.
- **tests.selection.12** (Hard, active): Selection is exactly as safe as its map is complete; the backstop is how a miss is found and turned into a fixture.
- **tests.selection.13** (Hard, active): A release is a claim about one tree, so it either proves that tree or shows a verdict that already did. Where selection is on, the release is exactly as safe as the map is complete, so the plan is recorded with the tag.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
