# Tiers and tests: the handbook for a project starting out

This is the guide of the tests part for a project that is setting up its gate. It says which tier holds what and why, with the numbers that settled it, and which of the practices are for every project and which are for a project that has the need. It backs tests.runners.4 to .12, tests.generated.6, tests.journeys.7 and .8, tests.isolation.14, tests.flakes.6, tests.measures.9 to .12 and tests.selection.16 and .17, and rests on gates.command.17 to .31 in the gates guide. Sections 6 to 10 are the handbook of the development cycle: the six moments, the core rules by moment, how a stack adds lines, the mistakes the rules prevent, and the checklist for a new project. Words a cold reader may not know are in the glossary at the end.

## 1. The six moments

A gate is run at six moments, and each moment has one question to answer. A check belongs to the earliest moment where it can answer its question inside that moment's budget. Review has no members: its budget is rounds, and section 6 holds it with the others.

| Moment | The question | What runs | How long |
|---|---|---|---|
| Commit | Is what I staged well formed? | The check-only members: format, generated files, supply-chain config. They read and never rewrite. | Seconds |
| Push | Does the pushed tree pass everything a push must? | Every member a pushed tree must pass: build, light tests with the race detector, heavy tests, lint, the compile for each platform, the narrow-width properties. | Within the push budget, about a minute |
| Review | Would an independent reader accept it? | A reviewer who did not write it reads the diff and breaks each new guard by hand; a fix round reads only the delta. | One round, two at most |
| Landing | Does the merged candidate pass, once? | The same tier, on the merged tree of a train, not again for a tree that already has a pass. | One proof for the whole train |
| Release | Does it prove a release? | The push tier plus what only a release proves: crash injection and the reproducible-build dry run. | Push plus about half a minute each |
| On demand | What do the deep runs say? | The long fuzz run and the heavy race run. | Minutes, and they block nothing |

(gates.command.17.) Two things keep this honest. A check that cannot meet the budget of its moment moves to a later one, and it never moves to a timer or out of the gate (gates.command.3). And the deep runs are on demand and nothing waits for them: the owner asks, or the hub asks the owner first.

### Why fuzz and the heavy race run are on demand

In the measured history of this project's gate, the release tier took 27 minutes 47 seconds, run member after member. The long fuzz run was 14 minutes 38 seconds of it, and it found two bugs in 260 commits, both while a parser was new. The heavy race run took 6 minutes 38 seconds and has found none. A run that long, at a moment that cannot wait, found the least. The seeds checked in for each fuzz target still run in every tier as ordinary tests, and a pull request that adds a parser asks for one long run (tests.generated.6). The light packages keep the race detector at push: it costs 17 seconds of wall time and keeps every race visible at the earliest moment, and turning it off at push saves about nothing once members run together, so that is not done (tests.runners.6).

## 2. For every project

These nine hold whatever the project is. Each has a number behind it, from a measured study of this project's own gate (tier-study-2026-11-09).

1. **Tiers by moment.** The table above. The first thing to write down is which moment each member belongs to (gates.command.17).
2. **Members run at the same time.** A member runs alone only when it declares the member it needs, and that order is shown in the report. The measured push ran fourteen members one after another and kept 1.7 of 15 CPUs busy. Run together, the longest member sets the time (gates.command.18).
3. **Tests are parallel.** A serial test says why in its own code. A test that needs a variable for a child sets it on the child, and stays parallel (tests.runners.4).
4. **Expensive setup runs once per package.** One test built the landing judge and ran the landing script four times, 38 to 47 seconds of every push, to prove four things one build could show (tests.runners.5).
5. **Tools are installed when a pin changes.** The check is the guard and runs once; the install is the fix and runs when the check fails or a pin file changed. Doing both on every push cost 5 to 13 seconds (gates.command.19).
6. **A budget comes from a measured floor.** The budget is the floor on a quiet host times a stated margin; this project states 1.5. A tier that passes only because the host was busy is a finding, not a pass, and the report says so in its first lines. Every measured push broke its 180 second budget and was excused by that clause (tests.measures.2, tests.measures.9).
7. **The gate says how busy it kept the machine.** A tier that keeps a small share of the CPUs busy is a finding that names what holds the rest back (gates.command.20).
8. **A fresh clone passes.** After the documented setup, every member passes on a clone made from the remote. Two reds came on a second machine's first run, a module graph that only an earlier run had primed and a test that needed an ignored file another member writes (gates.command.21).
9. **The tier runs once, on the one system the project is on.** A product holds no code of one system, so no per-platform run is asked of it; what differs by system is the platform libraries' own proof (gates.command.16 is retired).

## 3. Optional, and when each applies

| Practice | Apply it when | Where it runs |
|---|---|---|
| Fuzz | The program reads input it did not write. The seeds always run; the long run is for a new parser. | Seeds in every tier, the long run on demand (tests.generated.1, .6) |
| Race detector | The program has goroutines or threads. | Light packages at push, the heavy run on demand (tests.runners.6) |
| Crash injection | The program writes a user's files. | A short run at push, the long run at release (tests.journeys.7) |
| Narrow widths | The program draws a screen. | One golden width read by a person, properties at the narrow widths (tests.journeys.8) |
| Test selection | The whole push still misses its budget on a quiet host after items 2 to 6. The owner decides. | Proposed (tests.selection.16) |
| A split by language | Selection is on. It is one row of the selection map. | Proposed (tests.selection.17) |
| A lighter builder hook | Selection is decided and a push is no longer the bottleneck. The owner decides. | Proposed (gates.hooks.8) |

The last three are proposed and not active. The study that measured them recommends taking the nine items first and measuring, since selection buys about 20 seconds on a 50 second push and trades a whole run at every push for a whole run at every train and release. They become active, or are retired, when the owner decides after the nine are in and measured.

## 4. The numbers that ruled it

| Measure | Value | Why it matters |
|---|---|---|
| CPUs the push kept busy | 1.7 of 15 | The tier was about 89 percent idle, so the fix is structure, not a bigger machine |
| Push, serial members | 4 minutes 5 seconds to 4 minutes 20 seconds | Above its 180 second budget in every measured run |
| Push, after items 2 to 6, estimated | About 50 seconds | The target |
| Release, as it was | 27 minutes 47 seconds | Fuzz and the heavy race run were most of it |
| Release, as ruled, estimated | About 1 minute 20 seconds | Push plus crash injection and the dry run, in parallel |
| Fuzz history | 2 bugs in 260 commits, both in a new parser | Why it is on demand |
| Heavy race history | None found | Why it is on demand |
| Race detector at push, light packages | 17 seconds of wall time | Why it stays at push |
| Setup repeated in one test | 38 to 47 seconds of every push | Why setup runs once per package |
| Tool install and second check | 5 to 13 seconds | Why a pin change is the trigger |

All of these are numbers from a busy host. A quiet host is faster, and the budget is set from a quiet run.

Which rule holds which proposal of the tier study:

| Proposal | Rule | State |
|---|---|---|
| P1 members at the same time | gates.command.18 | active |
| P2 and P4 parallel tests | tests.runners.4, with tests.generated.7 for the smallest crossing input | active |
| P3 setup once | tests.runners.5 | active |
| P5 tools by pin | gates.command.19 | active |
| P6 release members in parallel | tests.journeys.9, supply.release.11 | active |
| P7 fresh-clone fixes | gates.command.21, tests.isolation.14 | active |
| P8 budgets at the floor | tests.measures.9, tests.measures.10 | active |
| P9 test selection | tests.selection.16 | proposed, the owner decides |
| P10 the Go and TypeScript split | tests.selection.17 | proposed, the owner decides |
| P11 race detector off at push | tests.runners.6 | not taken |
| P12 a lighter builder hook | gates.hooks.8 | proposed, the owner decides |

## 5. A project starting out, in short

1. Write the member table with a tier for each member, and keep the six moments above.
2. Let members run at the same time, with `after` where one reads what another writes.
3. Write tests parallel from the first test, and build any shared setup once per package.
4. Put the tools behind a pin and a single check.
5. Measure the push on a quiet host, write the floor, and set the budget at the stated margin.
6. Run the tier in a clone from the remote after the setup and see it pass.
7. Add the optional practices as the program grows into them, and ask for the deep runs when a parser is new.

## 6. The development cycle: the six moments

This section and the four after it are the handbook of the Development Cycle Standard, adopted and written into the rulebook. One cycle for any software: commit, push, review, land, release, and the runs asked for on demand. The core is the same for every project, whatever the language. A stack adds its own members to the same moments and never a cycle of its own (section 8). Each moment answers one question inside its budget, a check runs at the earliest moment where it can answer in time, and a check too slow for its moment moves to a later moment and never out of the cycle (gates.command.17, gates.command.3). Each moment has an owner: the builder runs commit and push on their own machine, the hub lands and releases, and an on-demand run is asked for (gates.command.31).

| Moment | The question | What it holds | The budget rule |
|---|---|---|---|
| Commit | Is what I staged well formed? | A format check that reads and never rewrites, the generated files against their source, a secrets scan of the staged diff against a reviewed allowlist, the supply config (pins, lockfile), and the protected-path check where the project has protected paths. | Seconds. The measured commit of one project is 3.0 to 3.3 seconds. |
| Push | Does this tree pass everything a push must? | Every member at once in a pool capped at cores divided by slots: the build, every test in parallel with the report counting what ran, lint with its workers and cache, the vulnerability and licence scan, the gate registry and protected paths. Every member says where it can run, and one that cannot run here prints SKIPPED and why. | From the project's measured floor times 1.5 (tests.measures.10). About one minute is a target, never a budget. |
| Review | Would an independent reader accept it? | A reviewer who did not write it. Fix rounds review only the delta. Each new guard is broken by hand. A second round of the same kind asks for the fix to the kind. A verdict on every protected path, recorded where the project says. A change to the runner's pool, isolation or workers is proved on the whole suite first. | One round, two at most. Rounds are counted (gates.measures.5). |
| Land | Does the merged candidate pass, once? | Approved, disjoint changes go as a train, and a train of one is a train. The push tier on the merged tree, keyed to the candidate tree. The base's code judges the head. One push per landing, and the next train is built on this train's squash. A red names the last green main. A hosted job here is dispatch-only or run by the hub. | One proof per train. |
| Release | Does it prove a release? | The push tier plus the release members, all at once, on a fresh clone of the tag. Every claimed platform runs the tier. A secrets scan of all history. The clock runs from the last fix merged to published, and for a library to its consumers' main. A tag push from a hosted workflow never runs the local hook. | Push plus about 30 seconds, from the measured floor. |
| On demand | What do the deep runs find? | Runs asked for by the owner or asked of the owner, named in the members file, logged, with a red opening an issue the same day. The heavy race run, daily on the main branch, run by the hub. Timing and budget specs on a shared host, after a landing on the main branch's tip. | Minutes. Blocks nothing. |

The budgets of the moments are targets until the floors are measured on a quiet host (#648). A budget is a measured floor times 1.5, and a run that passes only because the host was busy is unjudged, never green, and says so in its first lines (tests.measures.9).

## 7. The core rules, by moment

Forty-four rules hold for every project. Each is here because one of three fleets made the mistake it names: this project, a fleet of five Go projects, and a fleet of 39 repositories. The standard's own ids (C for the cycle, T for tests, R for review and landing) are kept beside the rulebook's, so the two can be read together. A rulebook id marked new was written for the standard; the others held the rule already.

### The cycle as a whole

| Standard | Rule | Rulebook |
|---|---|---|
| C1 | One cycle, one runner, one members file. A project differs only in which commands fill each kind. | gates.command.23 (new), gates.command.1, gates.shared.8 |
| C2 | Members run at the same time. A red never stops the others. The report prints the cheapest first. | gates.command.18, gates.command.12 |
| C3 | A check belongs to the earliest moment where it fits the budget, and never moves to a timer or out of the cycle. There is no moment below push. | gates.command.17, gates.command.3, gates.hooks.4 |
| C4 | A budget comes from a measured floor, times 1.5. A run that passes only under load is unjudged. | tests.measures.10 (new), tests.measures.2, tests.measures.9, gates.command.7 |
| C5 | The gate reports how busy it kept the machine. | gates.command.20 |
| C6 | Tools are installed when a pin changes and checked once. Browsers and images are tools. | gates.command.19, gates.command.24 (new), supply.provenance.7 |
| C7 | A fresh clone passes after the documented setup. | gates.command.21, gates.hooks.2 |
| C8 | Every claimed platform runs the tier, or the claim is narrowed. | gates.command.16 (now active), supply.release.7 |
| C9 | Lint allows parallel runners, and gets its workers and cache in every tier. | gates.command.22, gates.command.25 (new) |
| C10 | The stamp is a claim, keyed to the exact tree. Landing and release never take it as proof. | gates.land.15 (new), gates.land.11, gates.land.2 |
| C11 | The hook is the gate and nothing skips it. The hook does nothing but call the runner. | gates.hooks.9 (new), gates.hooks.1, gates.hooks.4, gates.hooks.5, agents.guards.2 |
| C12 | A run off the cycle exists only if someone watches it. | gates.command.26 (new), tests.runners.7 |
| C13 | Times come from the gate's own report, never from prose. | gates.measures.1, docs.freshness.4 |
| C14 | The pool's capacity is cores divided by slots, set on the child; a weight is a promise and a cap. | gates.command.27 (new), tests.runners.2 |
| C15 | A green carries its counts and the delta against the last green. | gates.measures.6 (new), gates.plants.8, gates.plants.1 |
| C16 | A check never writes the tree it reads. | gates.command.28 (new), gates.hooks.3, gates.own-rules.3 |
| C17 | A gate's wall time excludes queue time. | gates.measures.7 (new), tests.measures.1 |
| C18 | Heavy runs take a slot; light commands run bare. | gates.command.29 (new) |
| C19 | A hosted workflow never triggers itself. | supply.workflows.1, supply.workflows.5 |
| C20 | Every member says where it can run, and a skip is red at land and release. | gates.command.30 (new), tests.admission.5 |
| C21 | Each moment has an owner. | gates.command.31 (new) |
| C22 | A rule adopted on a tree that already breaks it starts from a reviewed, shrink-only baseline. | gates.ratchets.1, gates.ratchets.4 |
| C23 | A token never goes on a command line or in a brief. | agents.guards.12 (new), agents.guards.8 |
| C24 | A library's release clock runs to its consumers' main. | supply.bumps.8 (new), supply.bumps.2 |

### Writing tests

| Standard | Rule | Rulebook |
|---|---|---|
| T1 | Tests are parallel from the first test. | tests.runners.4 |
| T2 | Expensive setup runs once per package. | tests.runners.5, tests.isolation.3 |
| T3 | No test waits out real time. | tests.runners.8, tests.isolation.6 |
| T4 | A test owns its fixtures. | tests.isolation.14 (new), tests.isolation.1 |
| T5 | A flaky test is a bug ticket, never a retry. One diagnostic rerun, logged; the first red opens the ticket; the second quarantines. | tests.flakes.6 (new), tests.flakes.1, tests.flakes.2 |
| T6 | One test run per tier, per module. | tests.runners.9 (new) |
| T7 | The package that reads the whole tree is parallel and lean. | tests.runners.10 (new) |
| T8 | Goldens are read by a person; widths are properties. | tests.journeys.8 |
| T9 | The gate's own tests are ordinary tests. | tests.runners.11 (new), gates.plants.12 |
| T10 | A test over a third of its timeout when idle is a finding. | tests.measures.11 (new) |
| T11 | A list of accepted failures has an owner and an expiry, and the gate prints its length. | gates.exemptions.4, tests.flakes.2 |
| T12 | A timing assertion is off the blocking path on a shared host. | tests.measures.12 (new), tests.flakes.4 |

### Review and landing

| Standard | Rule | Rulebook |
|---|---|---|
| R1 | On a second round, ask for the fix to the kind. | agents.review.6 |
| R2 | Fix rounds review the delta, and the reviewer breaks each new guard by hand. | agents.review.7 (new), agents.review.2 |
| R3 | Approved changes land as trains, proved once, keyed to the candidate tree. | gates.land.16 (new), gates.land.12 |
| R4 | The base judges the head. | gates.land.17 (new), gates.protection.1 to .3 |
| R5 | Prove the merged tree, not each change alone. | gates.land.12 |
| R6 | Count rounds, refused pushes and hand-offs. | gates.measures.5, agents.delivery.1 |
| R7 | A change to the runner's pool, isolation or workers is proved on the whole suite first. | tests.runners.12 (new) |
| R8 | A hand-off carries a notice, and a red at landing names the last green main. | gates.land.18 (new) |

The six owner decisions of the standard are in the rules: no retry closes a red (tests.flakes.6); the shared runner ships as one command built from a pinned tag (gates.command.23); the tree-reading package leaves the push race run (tests.runners.10); the release runs the light race set and the heavy race is the hub's daily on-demand run (tests.runners.6, gates.command.26); a post-land run of the whole tree is retired where landing proves the merged tree (gates.land.16); and no project takes selection until the floors are measured (tests.selection.16).

## 8. How a stack adds lines

A stack is a language, a framework or a property of the program: Go, TypeScript, a React app built with Vite, Storybook stories or design tokens, browser end-to-end tests, a service with containers, Python, a Liquid theme, static HTML or a docs-only repository, a terminal or screen UI, a program that writes a user's files, one that reads untrusted input, one that ships a binary, and journeys that need a real machine. A project takes every stack whose condition holds: a Go CLI with a terminal UI takes the Go lines and the screen lines; a React app with stories and browser tests takes four stacks.

A stack adds members to the moments above. It never adds a moment and never moves a core check. Each line names its moment: commit, push, land, release or on demand, plus the tools it pins and the rules it reads in review. A stack's lines live in its own part of the rulebook, in that part's cycle concern, and that part's README says in one short section what it adds to the cycle. The core rules of this guide hold whatever the stack.

Three things a stack line may not do. It may not put a check that misses its moment's budget on a timer or outside every hook: the check moves to the next blocking moment (gates.command.17). It may not set a tool's workers or cache by hand: they come from the pool (gates.command.25). And it may not pass quietly on a host that cannot run it: it prints SKIPPED with the cause (gates.command.30).

## 9. The mistakes the rules prevent, by cost

What cost time or let a red through, across the three fleets, ranked by the largest measured figure in each row's own unit: time lost first, blocks second, counts last. Every number is from a loaded host. The rule column is the rulebook id that stops it.

| # | What was done | What it cost | Rule |
|---|---|---|---|
| 1 | Waited for heavy slots, first come first served, with the wait logged only at its start | at least 2,846 agent-minutes in two days, a floor | gates.measures.7, gates.command.29 |
| 2 | Ran the full tier as the hand-in, bare, one per change | 198 full gates, 25.3 slot-hours, in one day | gates.command.31, gates.command.17 |
| 3 | Re-proved every waiting green each time the main branch moved | 44 land commands for 8 landings; 470 land-wall minutes landed nothing | gates.land.16, gates.land.15 |
| 4 | Ran light commands and experiments in heavy slots | 360 slot-minutes of experiments; 3.9 slot-hours of light calls | gates.command.29 |
| 5 | Waited for someone to name a main after a red post-land proof | 333 agent-minutes, six engineers | gates.land.18 |
| 6 | Asserted wall time on a shared host | 11 of 44 land logs red in a day; about 120 slot-minutes | tests.measures.12 |
| 7 | Kept fuzz and the heavy race run in the release tier | release 27 minutes 47 seconds for 2 bugs in 260 commits | gates.command.17, tests.generated.6 |
| 8 | Raced the whole tree at release, the tree-reading package 48 percent of it | release 13 minutes 3 seconds, race job 11 minutes 46 seconds | tests.runners.10, gates.command.17 |
| 9 | Pushed through a serial gate with one cold, single-threaded lint member | push 607 seconds, lint 394 of it (8.8 seconds warm and threaded) | gates.command.18, gates.command.25 |
| 10 | Put a quadratic shell substitution in a hook | 374 seconds of a 391 second push; 210 seconds of every commit | gates.hooks.9, tests.measures.10 |
| 11 | Ran the gate's members one after another | push 4 minutes 5 seconds to 4 minutes 20 seconds, 1.7 of 15 CPUs busy | gates.command.18 |
| 12 | Ran the gate's self-tests on every push, one sweep running each gate twice | 126 of 189 seconds in one service; 221 of 254 seconds in another | tests.runners.11 |
| 13 | Ran a serial check with two test batches | push 164 seconds; 135 seconds of tests where one batch takes 51 | gates.command.18, tests.runners.9 |
| 14 | Ran a hosted backstop after every landing on a loaded host | 633 seconds median, 22 minutes at peak, holding a slot | gates.land.16 |
| 15 | Published a design system with no test script | 4 days 20 hours with 20 spec files unrun; two versions shipped | gates.measures.6 |
| 16 | Let an inconclusive budget reading lock landing | landing locked 8 hours 2 minutes | tests.measures.12 |
| 17 | Changed a shared pool's isolation without a whole-suite proof | four specs red only there; 4 hours 23 minutes; two refs held | tests.runners.12 |
| 18 | Let workflows trigger themselves | 5,000 hosted minutes gone in about 5 days; 18 workflows lost | supply.workflows.1 |
| 19 | Let a fresh clone depend on a step the setup did not run | 14 of 38 repositories red or refused on a fresh clone | gates.command.21 |
| 20 | Built the same setup four times in one test | 38 to 47 seconds of every push | tests.runners.5 |
| 21 | Wrote tests serial by default | 152 serial tests, 136 seconds; 0 parallel tests in four projects | tests.runners.4 |
| 22 | Let tests wait out real time | waits of 16.6, 12.1, 7.1, 5.1 and 5 seconds; 3 second backoffs; fixed browser budgets | tests.runners.8 |
| 23 | Let the loaded-host clause excuse every run | a 180 second budget broken in every run, unseen | tests.measures.9, tests.measures.10 |
| 24 | Let the tag push from a hosted workflow re-run the local hook | 403 seconds over four publishes | gates.hooks.9 |
| 25 | Released a library with no owner on the consumer side | 2 hours 45 minutes, 5 hours 30 minutes, and once over 5 days to reach its consumer | supply.bumps.8 |
| 26 | Ran lint with its single-runner lock | seven refused pushes across four projects in one week, each a full rerun | gates.command.22 |
| 27 | Reinstalled tools on every push, or never checked the pin | 5 to 13 seconds; 6 to 8 seconds; installs of 180, 123 and 106 seconds; 170 false lint errors | gates.command.19, gates.command.24 |
| 28 | Ran uncapped test workers beside other gates | red with every test passing; worker timeouts 1, 6, 12 | gates.command.27, tests.runners.2 |
| 29 | Ran browser gates in a no-network sandbox | 49 of 49 blocks failed with a network error | gates.command.30 |
| 30 | Scheduled a race run nobody watched | main red on race, unknown | gates.command.26, tests.runners.7 |
| 31 | Checked a protected path at push, not commit | at least 5 refs red, 17.7 minutes per round | gates.command.17 |
| 32 | Claimed a platform without running the tier there | main red on one platform, every tool refused | gates.command.16 |
| 33 | Hand-edited a derived file in a repository with no hook | main red for about a day | gates.hooks.9, gates.own-rules.3 |
| 34 | Proved each landing alone, and the verdict commit too | 44 checks for 13 landings, 9 proofs thrown away | gates.land.16 |
| 35 | Fixed one instance per review round | five rounds on one class; seven contrast tickets in 8 days | agents.review.6 |
| 36 | Started a container per spec file inside a fixed 10 second bind budget | 19 seconds against 7 for four files; three tier failures on the timeout | tests.runners.5, tests.isolation.3 |
| 37 | Reported green with no counts | 18 plays unrun; 13 of 14 mutation runs tested nothing; 11 instruments never red | gates.measures.6 |
| 38 | Kept goldens at three widths | stale 60 and 40 column goldens on the main branch | tests.journeys.8 |
| 39 | Wrote times into docs by hand | 17 seconds said, 26 to 30 measured; about 30 said, 73 to 111 measured | gates.measures.1 |
| 40 | Put a token expression in a brief | at least 10 installs by that form, one print into a transcript | agents.guards.12 |
| 41 | Pushed past the hook | 9 hours 15 minutes from the event to the deny; one push in a history | gates.hooks.5, agents.guards.2 |

## 10. Starting a new project

1. Pin the shared tier runner, and write the members file with the core members in their tiers, each with its kind, cost, weight, platform and network need. A repository with nothing to run says so (gates.command.23, gates.command.30).
2. Add the stack lines whose condition holds. Nothing else (section 8).
3. Make the documented setup install the hook and run every generator the type check needs. The hook runs the tier and nothing else (gates.command.21, gates.hooks.9).
4. Write tests parallel from the first one, with fixtures they own, setup built once per package, and fake clocks for time (tests.runners.4, tests.isolation.14, tests.runners.5, tests.runners.8).
5. Put the tools behind pins and one check; a tool that is not a module is pinned by checksum (gates.command.19, gates.command.24).
6. Install the hooks and the deny. The pre-push hook runs the push tier, a push from a hosted workflow never runs it, and nothing skips it (gates.hooks.1, gates.hooks.5, gates.hooks.9).
7. Run the tier on a fresh clone on every claimed platform, and see it pass with its counts printed (gates.command.21, gates.command.16, gates.measures.6).
8. Measure the push on a quiet host, write the floor, and set each budget at 1.5 times it. Nothing asserts wall time on a shared host (tests.measures.10, tests.measures.12).
9. Name every off-cycle run, log it, and open an issue on any red (gates.command.26).
10. Where a rule meets existing breakage, freeze a reviewed baseline with an owner, and shrink it (gates.ratchets.1, gates.ratchets.4).

## Glossary

- **Moment:** one of the six points of the cycle (commit, push, review, land, release, on demand), each with one question and one budget.
- **Train:** several approved, disjoint changes landed together and proved once on their merged tree; a train of one is still a train.
- **Slot:** a share of a shared host that a heavy run takes before it starts; the pool's capacity is the cores divided by the slots.
- **Tier:** a named set of checks run at one moment, with a time budget.
- **Member:** one check or run of the gate that a tier lists.
- **Floor:** the best time measured for a tier on a quiet host, with the date, machine and load beside it.
- **Margin:** the most a budget may exceed the floor, as a multiple of it.
- **Loaded-host clause:** the rule that excuses a run over its budget when the host was busy at the start. It makes the run unjudged, not green.
- **Seed:** an input checked in for a fuzz target; it runs as an ordinary test.
- **Crash injection:** killing a program at chosen points of a write, then checking that recovery leaves the old file or the new one.
- **Property check:** a test of a rule an output keeps (no line wider than the screen) instead of a comparison with a file.
- **Selection:** choosing the tests to run from what changed, instead of the whole suite.
