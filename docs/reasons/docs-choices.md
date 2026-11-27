---
type: reason
status: current
as-of: 2026-11-16
---

# Why the docs rules exist

Fourteen rules of the docs part (freshness, glossary, links, samples and voice) rest on the reasons below. Each rule keeps its status. The reason for each is stated here with the measurement or failure behind it.

- **docs.freshness.1** (Hard, active): No pinned repository had front matter; without a type, a status and an owner nobody can tell a current record from an old one or who keeps it true. The invariants are the rule; the container is the repository's choice, so a repository that already writes a header line or a status paragraph is held to the same fields without a rewrite.
- **docs.freshness.2** (Hard, active): Notes mixed into the docs folder read as current fact and go stale unseen. A record that is marked is not that risk: deleting it loses the reasons behind the current design, and marking it keeps them without letting them pass for current fact.
- **docs.freshness.3** (Hard, active): A date bumped with no content change makes an old claim look fresh, and a section replaced without a note reads as current.
- **docs.freshness.5** (Hard, active): A decision that supersedes another without the back link leaves two live answers, and an edit in place loses what was decided and why. The invariants are the rule; the container is the repository's choice.
- **docs.freshness.7** (Hard, later): A command described once and rendered everywhere cannot drift: the agent's skill, the tool list and the README are the program's own words, not a second copy of them.
- **docs.glossary.4** (Hard, active): A person reading help or a guide cannot follow an internal name or id anywhere, so it is noise that also leaks how the code is built; the id belongs where the people who can follow it read.
- **docs.links.1** (Hard, active): One broken file link in one repository and 36 fragment failures in one file of another, on 5,800 links of which 4,200 are remote. A site that is down for an hour is not the repository's fault, so it must not stop a push, but a dead source must still be seen before a release goes out.
- **docs.links.3** (Hard, active): A comment that points at a path that is gone sends a reader and an agent to nothing; a version or a CI claim in prose is a second copy of a fact whose home is the lockfile or the workflow, and it goes stale first.
- **docs.samples.1** (Hard, active): Docs rot; 41 fenced Go blocks counted in six clones showed no known fault from them yet, so the Go half is practice only.
- **docs.samples.2** (Hard, active): A README with a known shape is read in one pass by a stranger and can be partly generated; a README that does not point to the rules sends an agent and a person to guess.
- **docs.samples.3** (Hard, active): A visible choice about docs, though it buys visibility and not truth.
- **docs.voice.1** (Hard, active): Punctuation style is checkable and drifts without a check. An em dash is the mark that most often survives from generated text, and a semicolon in a terminal line splits one fact into two half-sentences.
- **docs.voice.2** (Hard, active): Printed text is read in a hurry and parsed by scripts, so personality there is noise; the filler patterns are the ones a model reaches for and a reader stops trusting. Tone, honesty and order cannot be checked by a pattern, so a reader or a model lens judges them.
- **docs.voice.3** (Review, active): A number without its date and machine reads as a property of the product when it is one reading on one day, and a figure with more decimals than its source is a claim nobody measured.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
