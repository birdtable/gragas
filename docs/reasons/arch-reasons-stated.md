---
type: reason
status: current
as-of: 2026-11-16
---

# Reasons for thirteen rules of the arch part

Thirteen rules of the arch part carry their reason in this one page. Each keeps its status and cites this record. Each reason is a measurement or an incident, stated without naming the project it came from.

- **arch.layers.1** (Hard, active): A layer that imports upward makes the layers unreplaceable and unreadable one at a time. In Go the layer test exists in two repositories and the table counts breaches in four (7, 1, 1 and 1 at baseline). In the web stack the rule existed as a text rule (a layer-check script) in two repositories, with no fault traced to a missed breach.
- **arch.layers.2** (Hard, active): A rule that only a reader knows is not held; the lint rule is the part that keeps working when the author is an agent. Only one Go repository enables the lint rule today.
- **arch.layers.5** (Hard, active): A deep import turns every internal file into a public API, so no module can change its inside. Boundary plugins fired on 10 of 10 Astro plants but have never been run on React.
- **arch.ports.11** (Hard, active): One wiring library is archived and no repository uses one; hand wiring in one place is what makes the composition readable.
- **arch.ports.13** (Hard, active): One composition root is the only place that knows every module, so a module can be added or removed in one place.
- **arch.ports.21** (Lens, proposed): A gate grew operating-system calls in its entry point, and no check fired, because the ports rule held the core and said nothing of the file that wires it.
- **arch.ports.22** (Lens, proposed): A gate grew shell with platform literals and no rule applied, because tooling was not counted as a program.
- **arch.shared.2** (Hard, active): A shared module with product words in it is a program in disguise; one shared style file carried photo words. The shared kit has no repository names today.
- **arch.shared.3** (Hard, active): One census found 42 hand copies (223,170 bytes) of what the shared UI module holds, and named a shared package for 31 of the 51 steps. Copies diverge silently.
- **arch.shared.5** (Hard, proposed): 12 of 20 UI steps were independent copies in one census, and 14 independent steps in a Go census; without a reason list a divergence cannot be told from drift.
- **arch.shared.6** (Hard, proposed): One repository's drift check fails and sits outside the gate (7 of 25 whole-file entries fail); five versions of a pre-push hook exist in five repositories; three byte-equal things were found in one census.
- **arch.shared.7** (Hard, active): Only one program imports the shared UI module (30 non-test files); the others copy. Three byte-equal things and 12 independent steps were found in one census.
- **arch.shared.8** (Hard, proposed): 13 hand-written replace-a-file bodies sit outside the owner and 14 production files call flock directly; a name search found none of the copies and a clone detector found 0 clones; the shape rule flagged 1 of 6 planted forms of the atomic write and missed all five restructured copies, so per-export bans are the gate.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
