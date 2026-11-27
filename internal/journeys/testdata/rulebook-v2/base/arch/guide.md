# Shared files in a repository built by parallel agents

This is the guide of the architecture part. It answers one question for any repository that several builders, human or agent, change at once: which files will they all want to edit, and how do you lay those files out so that two builders on two features never touch the same line? The rules it backs are arch.structure.9, arch.structure.10, arch.layers.14, arch.decisions.5, docs.freshness.8, gates.ratchets.11 and gates.command.15. The merge and the queue, the two layers that sit after the files, are in the guide of the gates part. A source key in parentheses names a program or tool that does the same thing; the keys are in `sources.yaml`, and the glossary is at the end.

The second part of this guide, on code that several programs share, starts at "Shared code: one owner, one export, a ban on the primitive" below the glossary.

## 1. What to measure first

A file that every feature edits is a lock on the queue. Count commits that touch the file per landing: anything over about one in four landings is a lock. Over three days in one repository of this kind, 69 of 143 commits touched one of nine hot files and 61 touched a generated file, and the commits that were only reviewers' verdict records were a third of the total. Two mechanics multiply every such count: a proof of a change (a full run of the checks) is keyed to the exact tree it ran on, so any rebase, with or without a conflict, is a new tree and a new proof; and a landing step that wants the head rebased on the base moves the base under everything queued. The file layout below cuts the conflicts and the re-reviews. Only the queue layer cuts the re-proofs. Fix the file, not the process around it.

## 2. The eleven principles

1. **A file every feature edits is a lock on the queue.** Measure it (section 1) and fix the file.
2. **One owner per file; a folder of entries instead of a list.** A list that grows by one row per feature becomes one file per entry, joined by a generator or read as a folder (arch.structure.9). Towncrier's news fragments (towncrier-fragments), changesets' per-change files (changesets-fragments), Go's per-proposal files (go-api-next, go-doc-next), Bazel's build file per package (bazel-build-files) and Kubernetes' owners file per directory (kubernetes-owners) are the same move.
3. **A fact about a thing lives with the thing** (arch.structure.10). What a package holds belongs in its doc comment, the reason it imports what it imports belongs beside the package, a stability mark beside the export. The central table is rendered from them.
4. **Generated outputs are committed, and a conflict in one is resolved by a rule, never by a person.** The rule is "run the generator on the merged tree and take the result". Kubernetes commits generated files and holds them with a verification script (kubernetes-verify-codegen); Gazelle holds build files by failing when they are stale (bazel-gazelle). Where there is no hosted CI, the land check and the tests read the outputs from the tree, so they stay committed.
5. **No shared counters** (arch.decisions.5). "The next free number" is a lock as surely as a shared line: two branches both take it.
6. **No digest chains in authored files** (docs.freshness.8). A chain whose newest line must equal the digest of the current state makes every parallel change wrong after a rebase.
7. **A fingerprint covers the fact that was answered, not the bytes of the file that holds it** (gates.ratchets.11).
8. **Lists git can merge: one entry per line, sorted, with a blank line between unrelated lists.** Two insertions at different points of a sorted list merge cleanly; two insertions into one flow list `[a, b, c]` on a single line never do. This is the form for lists that stay lists, not a substitute for a folder of entries.
9. **Protection follows the entry.** A protected folder named by pattern means a reviewer's verdict covers one entry file’s bytes, and a rebase that leaves them identical keeps the verdict (gates.protection.3).
10. **Merge mechanics are the last resort, and only inside the trusted step.** No union merge on anything a check reads, no merge driver in a builder’s clone, no recorded-resolution replay. The one place a conflict is resolved by machine is the landing step, which runs the base’s code and proves the result again (the gates guide, section 2).
11. **Batch what is proved anyway, and prove ahead only with a spare runner.** A train of N changes is one proof for N. Speculation buys wall time only when a second runner is idle (the gates guide, section 4).

## 3. A taxonomy of shared files

| Category | Examples | The pattern that removes the conflict | Held by |
|---|---|---|---|
| **A. Generated outputs** | the layer document, the import rows of the linter config, a surface listing, a command document, a rulebook’s generated JSON and READMEs, a judge file | Stay committed. Mark them generated so a reviewer reads the source (the attribute only hides the diff on a hosting site and changes nothing about merging, github-linguist-generated). A conflict confined to them is regenerated by the landing step. A generated path that is also protected keeps needing a verdict on the regenerated bytes | the generated-file check; the landing step; the gate check for protected ones |
| **B. Registries, one row per feature** | a layer table’s package lists, a gate’s member list, an answers file, an exemptions list, a planned-items list | Folder patterns first, so most additions are no row at all (arch.layers.14). What remains a row becomes a file per entry where the file is hot, and stays a sorted one-entry-per-line list where it is not | the joining generator; a schema per entry; the layer tests |
| **C. Facts about a thing kept centrally** | a prose column in a layer table, fan-out answers, stability marks, usage text per command in one switch file | Move the fact next to the thing: the package doc comment, a file in the package’s folder, a per-package marks file, a usage constant in the command’s own file (arch.structure.10) | the renderer reads the tree; the generated-file check |
| **D. Config with a generated part** | a lint config that is mostly generated with hand parts above and below; a build file whose generate and check recipes everyone edits | Make the file wholly generated from a hand template plus the data, or split the hand part into its own file the generator includes. A tool that reads one config file and has no include or extends (golangci-lint-config-file) forces the split to be done by generation | the generated-file check; a test that no exclusion is added outside the list |
| **E. Prose indexes and tables kept by hand** | the decisions index, a table of planned packages in an architecture document, an agent instruction file’s table of documents, a list of rules not yet met | Generated from front matter or from the data they index (docs.freshness.1). The prose around the table stays hand-written and cold | the generated-file check; the docs test |
| **F. Digest chains and ledgers** | a surface file whose newest line must equal the listing’s digest; shrink-only counts | The chain goes: the digest lives in the generated listing and a note per change carries the reason (docs.freshness.8). Counts that are ratchets by design stay, because the conflict is the point | the surface check; the gate check |
| **G. Single shared lines** | one line listing every phony target; a flow list `may: [a, b, c]` | One entry per line, sorted, or one declaration beside each target (gates.command.15) | a test that a target is phony exactly once; the layer table’s parser |
| **H. Sequential ids** | decision numbers, rule ids, a lock file of issued ids | A slug or a content-derived id where the id has no public grammar; an id assigned by the landing step where it does (section 5). A number the host allocates, such as a pull request’s, is already safe | the rulebook check at landing; the docs test |
| **I. Wiring and join files** | the entry point that fills the ports; the root that joins commands | One owner by design. Keep hand-written where the set is small and nearly fixed; generate where it grows per feature. Move per-item text into the item’s own file | the compiler; the generated-file check for generated joins |

## 4. Applying the principles to the files that were hot

**A config that is mostly generated.** When 87% of a file is a generated block and the rest is hand-written, every conflict is in the "mixed" file, and a rule that says "regenerate and take the result" cannot be applied to a file with hand hunks in it. Make the file wholly generated: a hand template for the shared part, a hand file for the project’s overrides, and the generator writes the result from those and the layer table. Protect the small cold override file instead of the large generated one.

**A layer table.** Keep it one file (arch.layers.9), restructured so that a new package in an existing family is no new line: folder patterns on the rows and in the lists of what may import what (arch.layers.14), the prose a package says it holds taken from its doc comment (arch.structure.10), and every list one entry per line, sorted. A pattern is a path with one trailing ellipsis, not an expression. If the file still changes in more than one landing in four after that, split it into one file per layer joined by a generator, and write a note on arch.layers.9 first.

**An architecture document.** Its two hot regions become generated blocks and the prose stays: the table of planned packages is rendered from the layer table’s planned list into a marked block, and the decisions section becomes two sentences of prose plus a generated index of one line per record, built from the records’ front matter. Each record stays a file of its own.

**A build file.** A `.PHONY` declaration on the line above each target, no shared line. Each generator adds its generate and check commands to two lists from its own block, and the two recipes that run every generator loop over those lists, so a new generator is one new block at its place and no edit to a shared recipe. If blocks appended at the end of the file keep colliding, split by target family with an include, as Bazel splits by package; hold that until the first two changes are measured.

**An answers file.** A folder of entries, one answer per file, and the fingerprint keyed to the fact (gates.ratchets.11): for an answer that a package imports something, the sorted import list of the package’s non-test files. An unrelated edit then renews nothing, and a change to the fact fails the answer until a reason is recorded.

**A surface listing.** The listing carries its digest in its header. A change to the public surface adds a note, a file with a date and a reason of a few words, and the check is that when the listing differs from the base’s, the change adds a note. Marks, when there are any, live per package.

**Gate registries that stay as they are.** A members list, a protected-path list, a baselines file and an exemptions list change a few times in three days, each change deserved its verdict, and each is protected whether it is one file or a folder. Leave them. The count in a baselines file is a ratchet: two changes lowering the same count meet on purpose, and the answer is to add them. Turn one into a folder only when it gets in the way, and protect the folder by pattern when you do.

**Wiring files.** An entry point that fills the ports is one owner by design. Keep it hand-written while the set is small, with each port’s fill on its own line, sorted by port, so two fills merge. Move the per-command usage text out of the switch file into each command’s file, joined by a constant.

## 5. Shared counters

Two branches that both take "the next free number" collide as surely as two edits of one line. Three patterns, and where each fits:

- **Ids assigned at landing.** The author writes a provisional id; the landing step allocates the number, rewrites the provisional id everywhere in the change, and records the allocation in its own commit. This fits an id with a public grammar that must stay numeric and frozen, such as a rule id. A reference from one branch to an unlanded id is refused by the check, so a half-assigned id cannot land. The rewrite touches generated outputs, which landing regenerates anyway. Build it when two rule changes first collide; until then whoever dispatches the work assigns the number in the ticket, which is the same thing done by hand.
- **Slug or content-derived ids.** This fits an id with no public grammar: a decision, a note, a planned item. A slug is a permanent id and one home (arch.decisions.1). Records already numbered keep their numbers. Two authors choosing one slug is the only collision left, and it is a real one worth a conversation.
- **Per-author ranges.** Each builder gets a block of numbers. This fits nothing: it puts the roster into permanent ids and wastes numbers. Rejected.

Numbers allocated by the host system, such as a pull request’s, are safe. Counts that are ratchets are not ids; see section 4.

## 6. What stays as it is, and why

- **Generated outputs stay committed.** There is no hosted CI to build them, and the land check, the import linter and the docs test read them from the tree. A repository with CI can leave them out (the cli guide names the tradeoff).
- **One layer table stays one file**, until it fails the one-in-four measure.
- **The shrink-only ratchets stay.** They are hand edits on purpose.
- **A large shared registry kept by one integrator is affordable for a few people and not for a queue of builders** (linux-maintainers). The counter-example is the reason to lay files out as above, not a reason to avoid it for a small team.
- **A change to the stamp’s key or to when a landing is allowed is not a file layout question.** It is in the gates guide.

## 7. Adding a list to a repository

1. Ask whether the list grows by one entry per feature. If it changes a few times a month, leave it one file, one entry per line, sorted.
2. If it grows per feature, make the entry a file in a folder, with a name that cannot collide (a slug, a package path, a pull request number).
3. Put each fact in the entry that it describes. If a column of prose in the old table was written by many authors, it was a fact kept away from its thing.
4. Write the generator that joins the folder into the one table a reader needs, sorted by a key in the entries so no order list exists. The generated file is committed and marked generated.
5. Hold it with the generated-file check, and write one test that walks the joined result for the rules that were columns ("every entry has an example").
6. If entries have goldens or fixtures that must be protected, protect the folder by pattern, so a reviewer’s verdict covers one entry.
7. Never put a digest of the current state in a hand-written file, and never number entries from a shared counter.

## Glossary

- **Judge, judge file:** the judge is the trusted program, built from the base, that decides whether a change may land; a file it is built from is a judge file and is protected like a golden.
- **Generated output:** a file a program writes from other files; it says so at the top and is refused when edited by hand.
- **Generated-file check:** the check that runs every generator in memory and fails when a committed output differs from what it would write.
- **Landing step:** the step that moves a reviewed change onto the main branch, run by the repository’s trusted code from the base, never from the change.
- **Base:** the main branch at the point the change is landed on; the change is the head.
- **Verdict:** a record, kept in the repository, that an independent reviewer read a protected file at one exact content and accepted it.
- **Protected path:** a file or folder, or a pattern with one star segment, whose change needs a verdict because it can weaken a check.
- **Ratchet:** a count that may only fall; a change that would raise it fails.
- **Surface listing:** a generated list of the public names a package exports, used to see that a change to the public surface was meant.
- **Fan-out answer:** a recorded reason why one package imports many others, so a rise in the count is a reviewed decision.
- **Front matter:** the block of fields at the top of a document, read by the generators that index documents.

# Shared code: one owner, one export, a ban on the primitive

The first part of this guide is about files that many builders edit. This part is about code that many programs need. A mechanism that two programs both write (an atomic file write, a lock, a digest of a file, a walk of a tree, a judgment of whether a program is trustworthy, a golden-file helper) gets written a third time unless something stops it. Builders do not copy on purpose. They do not see the earlier copy, because a search by name finds none of them: copies are called `Put`, `replaceOnce` or `enqueue`. The rules this part backs are arch.shared.2, arch.shared.5, arch.shared.6, arch.shared.8, arch.shared.10, arch.shared.12, arch.shared.15, code.duplication.1, code.duplication.2, agents.discovery.1, agents.discovery.6 and supply.bumps.2. The examples are Go. What carries over to any language is the shape: one owner for each capability, one list that a person and the gate both read, and a ban on the primitive outside the owner.

## 1. What counts as shared

The test is one of mechanism, not of policy (arch.shared.2): would a fix to this code have to be made in every program that has it? A write that must survive a crash, a lock that must be released when a process dies and a digest that must match across tools are shared. The look of a screen, the name of a command and the text of an error are policy: they differ on purpose. Keep one list of what is already shared, with the owner of each item. In a repository that has it, the list is the registry of section 2, and the listing is rendered from it rather than kept by hand.

## 2. The registry is the table you read and the table the gate reads

A capability that more than one package or program needs is one row in a registry file (arch.shared.15). The file is one row per line, sorted, so two builders adding two rows do not conflict. A row has seven columns.

| Column | What it holds |
|---|---|
| capability | a short name, such as "replace a file atomically" |
| owner | the package in this repository that owns it, or the shared module's package once it has moved |
| export | the one function or type to call |
| shared | the shared module's package and export, if it has one |
| primitives | the low-level calls the capability owns, such as renaming a file or hashing bytes |
| status | local (owned here), shared (owned by the shared module) or later (named, not yet built) |
| since | the tag of the shared module from which the row is shared |

Three things are rendered from the registry by the generator, so no one writes them by hand. The first is the lint bans: one ban per primitive outside its owner, each with the export in its message. The second is the instruction block that tells an agent what to read before writing a helper. The third is the shared column of the export listing. A generated-file check holds all three, so a row without a ban, or a ban without a row, fails the build. A repository test also fails a row whose owner package does not exist and a row whose export is not in the shared module's listing at the pinned tag. Removing a row narrows a gate, so the registry is a protected path: a change to it needs an independent reviewer's verdict.

## 3. The thresholds

Across programs the number is two (arch.shared.10). The second program that needs a mechanism is the trigger to move it into the shared module, since a third copy is a defect and not a local choice. Inside one repository the second copy of a mechanism moves to a leaf package at once, because a leaf is free to create. The rule of three is a rule for an abstraction whose shape is not yet known, where waiting for a third example stops you from guessing the wrong interface. The shape of plumbing is known, so it is not waiting for a third. A copy that a boundary forces, such as two contexts that may not import each other, is a baseline row with a date by which it is zero, not an exception without an end.

## 4. The order of a move

A mechanism moves in five steps, each its own change.

1. Make it a leaf in the program that has it, with a test.
2. Write a design note in the shared module's repository, with a section on what is left out and the libraries looked at, and read by someone other than the author.
3. Change the shared module, with tests, a line in its changelog and a tag.
4. In each consumer, a change that bumps the tag and deletes the local copy or lowers its baseline row.
5. Set the registry row's status to shared.

Never use a `replace` directive or a branch pin to reach a change that is not tagged. The consumer builds from the tag, so a reader of the consumer's history sees what it ran.

## 5. Versioning

Tags only. Below 1.0, a new capability is a minor version and a fix is a patch. A break is said first in the changelog, and every caller that the owner keeps moves in the same change. A separately versioned module in the same repository, such as one for the interface code, has its own tag line. The consumer's release report prints how far it lags, in minors, per module, and lists the waivers that wait on a bump (supply.bumps.2). It reads, for example, "one module lags three minors; three waivers wait on it".

## 6. Deprecating the local copy

A `// Deprecated:` comment is not a deprecation, because nothing fails when it is ignored. A deprecation is two things the gate can read: a baseline row for the copy (path, lines, a ticket and the date by which it is zero), and a ban whose message names the export to use. The change that takes the shared export deletes the copy. A copy that has to stay for a while is a row that only falls.

## 7. How a bump is reviewed

A bump is its own change, never mixed with a feature. The body names the tag range and quotes the changelog lines. The reviewer reads the diff of the shared module over that range and says in the verdict that they read it. The release tier runs a module-verification command so that the download matches its checksum. When the package that changed is one that confines paths, judges a program, takes a lock, runs a child or writes a file, the reviewer is a security reviewer, because every consumer inherits the change.

## 8. What builders read before writing a helper

Before adding a function that writes, locks, sums, walks, trusts a program, isolates a test or compares a golden, a builder does three things: reads the registry file, lists the shared module's package at the pinned tag, and searches the repository for the primitive. If they still write the function, they say what they read, in a `Reuse:` trailer on the commit (agents.discovery.6):

```
Reuse: read the registry rows for atomic replace and digest and the shared module's write package; its write keeps no owner of the file, and this one has to.
```

The ban catches a call to the primitive. The trailer catches the copy that was restructured so the ban cannot see it, by making the builder say what they looked at. A trailer can be written without reading anything, so it is weak evidence, but it puts the omission in front of the reviewer, who asks: which row did you read, and what in it did not fit? The same three reads are rendered into the agents' instruction file from the registry (agents.discovery.1).

## 9. Measuring the copies that remain

Two measures keep the count honest. A clone detector over every non-test source file, at 60 tokens, sees copies across packages; its count of clone groups goes into a baseline row that only falls (code.duplication.1). A detector that works on tokens ignores identifier names, so a copy with every name changed is found too (code.duplication.2). A deliberate copy taken from another repository is a row in a list of copies, with the original, what may differ and why, and a test fails if the copy differs in any other way (arch.shared.6).

## Glossary

- **Capability:** one mechanism that more than one package or program needs, such as an atomic write.
- **Owner:** the package that holds the one implementation of a capability.
- **Export:** the function or type that callers use, so that no one calls the primitive.
- **Primitive:** a low-level call a capability is built from, such as a rename or a hash of bytes; the ban is on this call outside the owner.
- **Leaf:** a package with no imports from the rest of the program, so any package may import it without a cycle.
- **Copy:** a second implementation of a capability, listed with a reason and a date by which it is zero.
- **Drift list:** the list of copies taken from another repository, each compared with its original by a test.
- **Bump:** a change that moves a consumer to a new tag of the shared module.
- **Lag:** how many minor versions a consumer is behind the latest tag of the shared module.
