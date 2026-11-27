---
type: reason
status: current
as-of: 2026-11-16
---

# A release has a time limit, and every tier has a budget

The release gate has a bounded run time, and each tier of the gate has a written budget. The rules below follow from that. Each states its own reason, with the strength and status it has in the rulebook.

- **gates.command.3** (Hard, active): Tiers written as prose tables drift from what runs. A check that is too slow for its tier and is run "at most every so often" by a timer is not a gate any more.
- **gates.command.7** (Answered, active): Without written budgets nobody notices a tier creeping; at one reference team the budgets were missed three to six times in the field. The first merge put 2 s on pre-commit, which belongs to on-write.
- **gates.command.8** (Hard, active): A release gate that can run without bound is an outage that nobody sees until it ends.
- **gates.hooks.4** (Hard, active): Heavy hooks invited `--no-verify` at a reference team, so the time is measured, not guessed. With no push CI the only place a slow check can go and still bind is the release.
- **tests.measures.1** (Hard, active): Whole suite 4 to 66 s wall on five repos under load, and the top three packages are 63 to 89 percent of the time in every repo (one sample). In a sample of 60 bugs, 7 were time or load budgets.
- **tests.measures.2** (Hard, active): A suite written by agents cannot grow unnoticed to hours. One repo's browser journeys alone are 35 minutes serial for one family of them (not re-timed).
- **tests.measures.3** (Hard, active): The limit of 20 minutes is a guess until the release is measured step by step; earlier web notes had set a red line of 40 minutes.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
