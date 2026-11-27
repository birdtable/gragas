---
type: reason
status: current
as-of: 2026-11-16
---

# Why the gates rules are justified in one page

Twenty-nine rules of the gates part carry their reason in this one page. Each rule keeps its status, and each states below the measurement or incident that justifies it.

- **gates.command.16** (Lens, retired): A build that compiles for a platform proves nothing of what runs there; one gate ran on a single platform while the program declared two.
- **gates.command.17** (Lens, active): The release tier took 27 minutes 47 seconds, most of it fuzz and heavy race runs that found two bugs in 260 commits; cut back to the push tier plus crash injection and the dry run it is about 1 to 1.5 minutes.
- **gates.command.18** (Lens, active): The pre-push tier ran fourteen members one after another and kept 1.7 of 15 CPUs busy; a hidden ordering (one test needing a file another member writes) is what a parallel runner would break first.
- **gates.command.19** (Lens, active): Every push installed the tools again and checked all eleven twice, 5 to 13 seconds of a tier whose budget is a minute, and the second check changed nothing.
- **gates.command.20** (Lens, active): The gate's own report said it kept 1.7 of 15 CPUs busy while the tier broke its budget; the number was printed and nothing acted on it.
- **gates.command.21** (Lens, active): A second machine hit two reds on its first run: the release dry run failed until a cross-platform module graph was primed, and a docs test failed until an ignored file a different member writes existed.
- **gates.command.24** (Lens, active): One project's gate pruned browsers from the shared cache on every run, and another reinstalled its packages every push for 6 to 8 seconds; in a third set of projects, installs of 106 to 180 seconds sat in no budget and a stale install after a lockfile change gave 170 false lint errors.
- **gates.command.25** (Lens, active): One lint command took 394 seconds cold in one run mode, about 128 seconds in the other, and 8.8 seconds warm with four threads; the same command, three speeds, and a push that was 65 percent that one member.
- **gates.command.26** (Lens, active): One project's daily race run stopped silently and its main branch failed the race detector with nobody knowing; another set of projects ran integration, characterisation and mutation checks by hand with no request path, no log and no issue on red.
- **gates.command.27** (Lens, active): One web project went red with every test passing when uncapped test workers ran beside other gates: worker timeouts rose 1, 6, 12 over three runs; capped, 9,428 of 9,428 passed. Three gate runs at once on 15 cores gave a load of 26 with no gain in total work.
- **gates.command.28** (Lens, active): One project's generate check rewrote the live tree, so the tests beside it failed 6 of 6 in a parallel trial; two commit hooks in another set of projects ran the formatter in write mode and restaged.
- **gates.command.29** (Lens, active): In one set of projects a type check, a lint prune, a git push and one unit-test file held the heavy-job capacity; experiments held 360 minutes of it in two days; and agents waited at least 2,846 minutes for heavy-job capacity over two days.
- **gates.command.31** (Lens, active): One set of projects ran 198 bare full gates in one day as its hand-in, 25.3 hours of heavy-job capacity, because its agent rules still named the full tier; the one push-sized run that day took 0.9 minutes against a full median of 8.7.
- **gates.exemptions.2** (Hard, active): A directive that no longer suppresses anything is dead debt and hides the next real violation at the same line.
- **gates.hooks.5** (Hard, active): Heavy hooks invited the bypass at a reference team; a bypass that nobody can see makes the hook a suggestion.
- **gates.hooks.8** (Answered, proposed): After the concurrency and parallelism rules a push is about 50 seconds and the hook is no longer the bottleneck of the day; a lighter hook would cut the builder's wait to about 15 seconds at the price of a red branch for a few minutes.
- **gates.hooks.9** (Lens, active): One hook with a quadratic shell substitution cost 374 seconds of a 391 second push and 210 seconds of every commit; four publishes ran the gates twice, 403 seconds, because the tag push re-fired the hook; and a deny written after a bypass came 9 hours 15 minutes later with a hole in it.
- **gates.land.11** (Hard, retired): A cache keyed on less lets a result from a different platform or lockfile stand in.
- **gates.land.14** (Hard, retired): At a reference team 98 of 201 heavy runs were a bare full gate repeated until the command refused.
- **gates.land.15** (Lens, retired): Two projects kept a stamp keyed to a 24-hour age; another set of projects read a stamp bound to the main branch's tip, so every landing staled every waiting green and 37 of 44 land commands in 8 hours landed nothing.
- **gates.land.16** (Lens, active): One project ran 44 checks for 13 landings, 18 of them on a verdict commit that never lands, and two landing agents threw away 9 of each other's proofs; in another set of projects 44 land commands in 8 hours landed 8 changes, 5.1 runs per change.
- **gates.land.17** (Lens, active): A head that changed the format of a file the judge reads was judged by code that could not read it and went red at landing; five projects with no judge needed the rule only where one exists.
- **gates.land.18** (Lens, active): Six engineers of one team waited 333 agent-minutes after a red post-land proof for someone to name a main branch commit, and four hand-offs with no notice idled 106 minutes in one day.
- **gates.measures.2** (Hard, active): A failed measurement that prints 0 reads as perfect.
- **gates.measures.7** (Lens, active): One set of projects counted 11 minutes of queue as gate time against a 1.5 times limit and landed 0 of 5; its agents waited at least 2,846 minutes for heavy-job capacity in two days, a floor because the wait was logged only at its start.
- **gates.own-rules.3** (Hard, active): A hand-edit is overwritten by the next regeneration, or worse, kept and drifts.
- **gates.ratchets.9** (Answered, active): Mixed moves hide the real change from a reader. This is argued from practice, with no fault of the project's own behind it.
- **gates.registry.1** (Hard, active): A reference team learned this the hard way, and the ledger idea comes from the same team. There is no fault in the project's own record; the schema uses a standard validator and no program was built.
- **gates.registry.3** (Hard, active): The first review found 52 "claim false" lines; the guides test in one repository is the model.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
