---
type: reason
status: current
as-of: 2026-11-16
---

# Reasons for test rules that carry them on this page

These rules of the tests part carry their reason in this one page. Each keeps its own status.

- **tests.admission.10** (Lens, active): Names are how a failure is read; a naming claim cannot be machine judged, only the presence of a case name can.
- **tests.flakes.6** (Lens, active): Reruns hide what load exposes: one project's journeys flaked at load 76 to 202 and only under load; 13 red journeys of another were 75 of 75 green alone, and each had a cause and a fix.
- **tests.generated.7** (Lens, active): One project's alias-bomb proof took 23 seconds under the race detector and about 1 second once shrunk to the smallest input that crosses the cap.
- **tests.isolation.11** (Hard, active): 17 of 60 sampled bugs were machine-dependent. A product holds no code of one system (the os-code check), so no row asks for a run on Linux; a verdict still names the machine it ran on.
- **tests.isolation.14** (Lens, active): Two tests of one project needed the checkout's own git folder, so every exported tree carried a failure that had to be explained away; four tests of another group needed sibling checkouts and failed rather than skipped, so a fresh clone was red.
- **tests.isolation.5** (Hard, active): The operating system limits socket paths to about 104 bytes and a deep temp folder passes it; one program failed at 153 bytes.
- **tests.isolation.7** (Hard, active): A read of the machine is an input the cache and the selector cannot see, and the reason a test passes on one host only. One repo has the scan: 7 files, 121 listed reads.
- **tests.journeys.10** (Lens, active): Three projects had journeys that need a virtual machine, a container runtime or a device, and each needed somewhere to say the host lacked it without a quiet pass.
- **tests.journeys.7** (Lens, active): A torn write is found only by the kill that lands in it, and a run long enough to find them all does not belong in a push; the release already runs everything a release alone proves.
- **tests.journeys.8** (Lens, active): Three goldens of the same screen at three widths triple the review of every change and say the same thing three times; the properties catch the same breakage and never need a hand read.
- **tests.journeys.9** (Lens, active): The crash run of one project is set at 400 and 100; run in one child it is a serial member that sets the release time.
- **tests.measures.10** (Lens, active): Every measured push of one project broke its 180 second budget and passed through the loaded-host clause printed at the bottom where nobody read it; another group had no budget at all, and a commit hook took 218 to 241 seconds unseen.
- **tests.measures.11** (Lens, active): One test took 9.5 seconds idle against a 30 second limit and went red at load 27; another's module took 565 milliseconds idle against a 60 second budget and still timed out nine times, and the tickets stayed open 21 to 23 days.
- **tests.measures.12** (Lens, active): Timing specs in one group of projects put 11 of 44 land logs red in one day and locked landings twice, and a rule that turned an all-inconclusive budget into a lock held landing for 8 hours 2 minutes.
- **tests.measures.5** (Hard, active): Practice; one repo reports without a threshold. Low value, kept so the cheap scan closes the row.
- **tests.measures.9** (Lens, active): Every measured push run broke its 180 second budget (206 to 301 seconds) and passed only through the clause that excuses a busy host, so the budget never pushed the heaviest member out of the tier.
- **tests.runners.10** (Lens, active): It was the slowest package in every project of one group, 7 to 40 times slower under the race detector: 47 seconds to 334 seconds in one, and one test from 4.5 seconds to 72.8 seconds in another.
- **tests.runners.11** (Lens, active): Self-tests set the push time in three repositories of one group, 126 of 189 seconds in one service and 221 of 254 seconds in another; the gate's tests were one project's slowest package, 126 seconds, for the same reason.
- **tests.runners.12** (Lens, active): One group set isolation off on a shared pool; four specs went red only there, one at 31 seconds against 6 seconds alone, and two changes were held off the main branch for 4 hours 23 minutes.
- **tests.runners.9** (Lens, active): One project ran its tests as two batches, 135 seconds, where one batch takes 51 seconds; another ran its Go tests then its web tests in series; a third ran the test tool twice, the second time for a cover profile sent nowhere.
- **tests.selection.16** (Answered, proposed): Selection buys about 20 seconds on a 50 second push and trades a whole run at every push for a whole run at every train and release; the study recommends measuring the cheaper changes first.
- **tests.selection.17** (Answered, proposed): A second switch beside selection would be a second way to skip a test, with its own gaps; as a row of the map it is held by the oracle already.
- **tests.selection.8** (Hard, active): Green for a skipped step is a lie the next reader believes. Today one repo prints its plan and loses it; the tools already fail closed on empty globs (verified) but custom checks and name filters need the runner to close the gap.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
