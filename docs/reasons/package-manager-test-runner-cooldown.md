---
type: reason
status: current
as-of: 2026-11-16
---

# Why one package manager, one test runner and a checked cooldown

Four rules (supply, tests and ts) rest on the reasons below. Each rule keeps its status.

- **supply.age.2** (Hard, active): A setting that is absent, non-numeric or overridden by a later line looks present and does nothing.
- **tests.runners.3** (Hard, active): One preset is the only place a flag is set once; the reference practice has one preset for every program.
- **tests.selection.3** (Hard, active): 44 of 87 test files in one repository read the file system or run a program, so a selector that looks only at imports misses them.
- **ts.runtime.3** (Hard, active): Two runners double the setup and split the test selection. In a reference project, 87 of the 103 test files used the built-in runner, and those are the files that must migrate.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
