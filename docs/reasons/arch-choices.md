---
type: reason
status: current
as-of: 2026-11-16
---

# The architecture choices behind the arch rules

These twenty-one rules of the arch part cover decision logs, layers, ports, shared code and structure. Each keeps its status and carries its reason on this page.

- **arch.decisions.1** (Hard, active): Decision logs that are prose files in each repository leave a decision impossible to find, supersede or tie to its check. Three target repositories keep decisions in three different containers with the same invariants; the invariants are what a reader relies on, the container is not.
- **arch.decisions.4** (Hard, active): "Design it twice" was written assuming hours per design; for an agent a second design costs minutes, and the first plausible design is what agents settle on. The rejected alternative is the part somebody later proposes again; without its reason the same argument is held twice.
- **arch.layers.10** (Hard, active): A production import of a fixture ships test data, and a test that reaches around the entry hides a coupling. A test tag in a release build ships the stand-in in place of the real adapter.
- **arch.layers.13** (Hard, active): A part declared separable stays separable only while something builds it alone; the first import across the line otherwise goes unseen until somebody tries to split it.
- **arch.layers.3** (Hard, active): A file outside every layer escapes every boundary rule, so the table says nothing about it. A checker that silently parses fewer files than exist passes the files it never read.
- **arch.ports.10** (Hard, active): Three target designs fix their platform ports up front and reject interfaces beside their users, because a port found late is a refactor across every caller. The old limit (an interface only once a second implementation exists) still catches the failure it was written for: one repository has 89 interfaces, 14 with no production implementer, and the true failures were wrappers over pure code.
- **arch.ports.12** (Hard, active): One repository has 26 init functions. Entry files today are 62, 54, 21, 15 and 15 lines; one outside program has a 934-line main and would fail on day one. Two target designs need a registry and a machine value set at start; allowing exactly those, write-once and loud on misuse, keeps the ban on hidden mutable state without forcing every port through every call.
- **arch.ports.14** (Hard, active): Two writers that each open the store each grow their own checks, and the client that skipped one corrupts the data the other trusted. One executor is the one place a write is checked, logged and undone, whichever client asked.
- **arch.ports.15** (Hard, active): A library that enters without a port leaks its types into the core, and a port written straight into code has no design anyone agreed; both make the next swap a refactor.
- **arch.ports.16** (Hard, active): A plug-in the command layer imports cannot be removed without editing the command layer, and a name clash that one registration silently wins is a command that quietly changes meaning.
- **arch.ports.17** (Hard, active): Without a fake the core's tests run only on the platform the developer has, and without a declared required set a new platform learns what it must fill from crashes.
- **arch.ports.18** (Hard, active): A consumer that branches on a platform name has to be edited for every new platform, in every repository that reads the server.
- **arch.ports.19** (Hard, active): A peer that can set a field the core branches on can steer the core. Naming the decision per field makes every new field a choice, not a default.
- **arch.ports.2** (Hard, active): One repository has 13 command files importing the runtime adapter directly and 150 lines in 12 packages that name it. A name ban that counts only imports misses the provider's field names and plan words, which tie the core to one provider as firmly as an import, and a product named in a comment or help line is a product the next reader assumes is supported.
- **arch.ports.20** (Lens, active): Human text changes between releases and locales without notice; a machine format is the part its authors promise to keep. One repository finds a sibling's version by searching its help text.
- **arch.ports.4** (Hard, active): Four core packages of one repository fail on Windows (ten over the whole repository); `go build` showed three of them and hid one. Two other repositories have a machine or file package that fails on other systems.
- **arch.ports.5** (Hard, active): One repository has 18 nil tests on port fields outside its port file, and its macOS port took 151 files. Agents model the first platform they see.
- **arch.ports.6** (Hard, active): One repository has 41 Go files and 7 site files naming its sibling program outside the adapter, and one finds the sibling's version by searching its help text. A rule that counts only literals lets a program name pass through a variable.
- **arch.shared.9** (Hard, active): The shared Go package's surface is 181 lines and 6.9 KB; the list read at the version one repository pins has no UI and no door entry. In the trial, 25 landings in one repository and 11 in another were real port changes. A surface with no stability mark promises everything to every reader; the mark is what lets an experimental export change without a break.
- **arch.structure.6** (Hard, active): A census of 20 UI steps found 3 identical and 12 independent; the same shape across a kind is what lets a check and an agent find things. Two Go target designs name the same folders for core, port, adapters and commands, and a template that does not exist (the rule pointed at one) holds nothing.
- **arch.structure.8** (Hard, active): A refactor without a pinned consumer contract finds its breakage in the consumer, after release. A port extraction touches every command at once, and only a recorded transcript shows that nothing a person reads moved.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
