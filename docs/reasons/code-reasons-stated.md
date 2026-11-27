---
type: reason
status: current
as-of: 2026-11-16
---

# Reasons for seven rules of the code part

Seven rules of the code part carry their reason in this one page. Each keeps its status and cites this record. Each reason is a measurement or an incident, stated without naming the project it came from.

- **code.comments.5** (Hard, active): Long, decorated and restating comments rot first and bury the few that carry a contract.
- **code.duplication.1** (Hard, proposed): One Go repository has 82 source clones (1.25% of tokens); two web repositories had 2 clones (0.26%) and 5 (0.15%). The lint runner's detector misses cross-package copies, so a cross-package detector is needed. The two reports' figures are not reconciled.
- **code.format.1** (Hard, active): Unformatted code makes every diff noisy and every review slower; the floor items are cheap and run in every Go lint config. The one-time format commit has no blame-ignore row in Go today.
- **code.format.2** (Hard, active): These are the cheapest finders of real mistakes (a wrong print verb, a deprecated call); they run in six repositories today.
- **code.naming.1** (Hard, active): A fixed table lets a reader and an agent guess a name correctly; a stutter and a free spelling make the same thing two things.
- **code.naming.2** (Hard, active): A second spelling of a file name makes every glob and every search miss half the files.
- **code.naming.4** (Hard, active): Two spellings of one concept make clients and docs guess which one is real.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
