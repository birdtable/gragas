# Laying out a command line program so parallel builders never collide

This is the chapter of the cli part that the rules cannot carry: how to arrange a command line program's files so that several builders, human or agent, can each add a command in the same week and never edit the same line. It is written for any project. The samples are Go, because the rules it backs are checked in Go; what carries over to any language with packages is the shape (a folder per entry, a generated join, a test that walks the joined result), while the type names and the cobra call are Go's. The rules are cli.commands.9, cli.commands.20, cli.commands.29, cli.output.3 and cli.output.11, with arch.structure.9 and gates.protection.3 to .5 behind them. A source key in parentheses names the surveyed program a claim comes from; the keys are in `sources.yaml`.

**Which source backs which claim.** gh-root-join, kubectl-cmd-join, cargo-commands-mod and terraform-commands show a hand-written join of per-command packages or files. hugo-commands shows one file per command. git-command-list shows the command list kept as data. docker-commands-register shows registration from init functions. kubectl-cmd-sanity and gh-help-test show cross-cutting checks that walk the command tree. helm-golden-output shows one golden per command. gh-gen-docs and kubectl-verify-docs show generated documents, left uncommitted or committed and compared. fleet-run-log-2026-11-08 shows the cost of shared files in a queue of builders. Words a cold reader may not know are in the glossary at the end.

## 1. Why a layout can be a rule

Most programs grow a command by adding a row to a table, a line to a root file, an entry to a menu, a paragraph to a help page and a line to a golden that lists everything. With one builder that is a short walk. With ten builders in a merge queue, every one of those shared files is a place two changes meet, and a meeting costs a rebase, a full run of the checks and often a new review. The surveyed programs mostly escape this by accident: a few humans resolve a few merges a day (gh-root-join, kubectl-cmd-join, cargo-commands-mod). A queue of builders does not. The layout below removes the meeting places instead of resolving them.

## 2. The eight principles

1. **One owner per file.** A change to one feature touches only files that feature owns. Two builders on two features never edit the same line.
2. **A folder of entries instead of a list.** Any list that grows by one row per feature (commands, gate members, planned items, marks) is one file per entry. Adding is a new file, never a shared line (arch.structure.9).
3. **Generated joins over hand-kept tables.** Where the program needs one table, a generator renders it from the entries and `gen --check` holds it. A conflict in a generated file is settled by a rule: run the generator and take the result, with no judgement and no second review (gates.protection.5).
4. **Cross-cutting rules are tests that walk the joined result.** "Every command has two examples" is one test over the tree, not a column every builder fills (kubectl-cmd-sanity, gh-help-test).
5. **Per-feature tests, goldens and docs.** A golden lives beside the code it records. No surveyed program keeps one global help snapshot (helm-golden-output, cli.output.11).
6. **Protect per feature.** A protected pattern with one `*` segment means a verdict covers only the changed feature's files, and a rebase that leaves them byte-identical keeps the verdict (gates.protection.3).
7. **A static join, no plug-ins, no `init`.** The join is compiled in. Only the entry point wires things, and the set of commands never depends on an import side effect (go.layout.2).
8. **Indexes are generated.** A person reads an index; nobody edits it (docs.freshness.1).

## 3. The general tree

```
cmd/<program>/main.go            wiring only: ports filled, root run, exit
internal/cli/kit/                the row type, the registry and its rules, the base verb list,
                                 the session and host interfaces, shared flags, the one --json encoder
internal/cli/cmd/<family>/       one package per command family: its row, its command,
                                 its tests, its goldens (testdata/), its board or notes
internal/cli/planned/<name>.yaml one file per command decided and not built
internal/cli/root/               the root, the menu, help rendering, and the generated join
internal/core/api/               the operations every command calls, and the result types
internal/core/<context>/         one package per bounded context
internal/port/ and its fake      interfaces only, and the in-memory stand-in
internal/host/<tool>/            one adapter per outside tool
internal/journeys/               tests of the built binary; testdata/<command>/ per command
internal/dev/<generator>/        the repository's own tools (gen, gate, layers, surface)
layers.yaml                      the layer table, with folder patterns so a new package adds no line
gate/exemptions.yaml             one exception format: path, rule, reason, owner, expiry
gate/members/<id>.yaml           one gate member per file, once the single file gets in the way
docs/                            hand-written typed docs, plus generated indexes and tables
```

The names are a sample. What the layout fixes is the shape: a kit that every command imports and nothing imports back, one package per command family, a planned folder, a root that only joins, and generators under the repository's own tools.

The kit imports the shared module for the mechanisms several programs need (writing a file, taking a lock, drawing a word); nothing under a command's own folder does. A command calls the kit, and the kit calls the shared export. The part on shared code in [the architecture guide](../../base/arch/guide.md) says how that module is chosen and how its use is held.

## 4. Where a command's parts live

Code, metadata (the row), help text, examples, flags, in-process goldens and the command's notes all sit in `internal/cli/cmd/<family>/`. The built binary's journey goldens sit in `internal/journeys/testdata/<command>/`. Every surveyed program keeps a command's code and help in the command's own package or file (gh-root-join, kubectl-cmd-join, cargo-commands-mod, hugo-commands, helm-golden-output, terraform-commands).

A command package never sets a run hook that the root owns: the persistent pre-run and post-run, with or without the error return. A package that sets its own replaces the root's hook rather than adding to it, so the shared refusals and the write claim are skipped without any error, and with one join per package no shared line is left to catch it. The registry test walks the joined tree and fails on any command that sets one (cli.commands.29).

A family is one package, not one package per verb: verbs that share a context and a plan screen live together, one file per verb. A root-level true journey (a mistyped command, no command, the settings file) is the only golden that is not a command's.

## 5. The row

A command declares itself once, in a typed value next to the command that builds it (cli.commands.29):

```go
package status

// Row is the one declaration of status.
var Row = kit.Command{
	Name: "status", Order: 41,
	Context: kit.InProject, Ends: kit.EndsListing,
	Summary:  "Say what applies to this project and which versions are in play",
	Examples: [2]kit.Example{{"tool status", "say it"}, {"tool status --json", "as one JSON document"}},
	Exits:    kit.Words("done; not ready"),
	JSONKind: "status",
	Menu:     kit.Menu{Lead: kit.LeadProject, Rank: 1},
}

// New is the command, drawn through the terminal port.
func New(s kit.Session) *cobra.Command { /* ... */ }
```

The fields, in plain words (the type names belong to the program's own kit package, a small package every command imports and that imports no command):

| Field | Type | What it is for | Allowed values |
|---|---|---|---|
| `Name` | string | the word a person types | unique among commands |
| `Order` | integer | the command's place in the program's own command table, so two commands cannot claim one slot | unique; a command the table does not name takes a high number such as 900 and sorts by name |
| `Context` | enum | where the command may run | the program's list, for example `anywhere` and `project` (the sample's `InProject`) |
| `Ends` | enum | what the command's last screen is | the program's list, for example `listing` (it reports), `plan` (it shows what it would do and waits for `--apply`) |
| `Summary` | string | one line of the command list and the root help | one line |
| `Examples` | two `Example` values | a line to type and what it does | exactly two |
| `Exits` | list of words | the closing words the command may end on, each tied to an exit code | the program's closing words |
| `JSONKind` | string | the name of the document `--json` prints | unique per document shape |
| `Menu` | struct | which screen leads with the command (`Lead`) and at what `Rank` | the program's screens |

`kit.Session` is what a command is given to do its work: the terminal, the settings and the ports it needs, passed in and never reached for as globals. `kit.Package` is the pair `{Row, New}` that the join lists for one command. `Example` is a pair of strings, `{line, does}`. A row is Go because it is type-checked and sits next to the code, as in every surveyed program.

**The registry and its test.** The kit's registry is built from the built packages and the planned files and refuses a bad set. One test over it, run in the program's ordinary tests, walks every entry and fails on: a missing summary; fewer or more than two examples; an order used twice, or a generated table not in order; a name that is both built and planned, or neither; a verb that is not in the shared base list or the program's own domain verbs (cli.commands.26). It names the entry at fault, so a builder fixes their own row.

## 6. The planned entry

A command that is decided and not built has no code to carry a row, so it is a file of the planned folder (cli.commands.9):

```yaml
name: waive
order: 43
context: project
ends: plan
summary: Waive a rule for a path, with a reason and an end date
examples:
  - {line: "tool waive <id>", does: show the plan}
  - {line: "tool waive <id> --apply", does: carry it out}
writes: [.tool/waivers.yaml]
exits: [done, declined]
json: plan
```

The values of `context`, `ends`, `exits` and `json` are the same words as in the Go row, written as text; `json: plan` says the command prints a plan document. A name is built or planned, never both and never neither: the registry test fails on a name in both and on a name in neither. Building the command deletes its file and adds its package in the same change. A command generator can turn the one shape into the other, so the two never drift.

## 7. The generated join

The root holds no list of commands. A generator reads the folders under `internal/cli/cmd/`, sorts them by name, and writes the join:

```go
// Code generated by the repository's gen tool from internal/cli/cmd; DO NOT EDIT.
package root

import (
	"example.com/tool/internal/cli/cmd/check"
	"example.com/tool/internal/cli/cmd/status"
	"example.com/tool/internal/cli/kit"
)

var built = []kit.Package{
	{Row: check.Row, New: check.New},
	{Row: status.Row, New: status.New},
}
```

**What the generator does.** It lists the subfolders of `internal/cli/cmd/`, sorts them by name, and writes one import and one `{Row, New}` line for each, so the same folders always give the same file. It does not read the rows. A folder that does not export both `Row` and `New` is not skipped: the generated file names them, the compiler refuses it, and the builder sees the missing name. A folder with no Go files is an error the generator reports by name. The generator takes no flags that change the output, so a rebase conflict in the file is settled by running it again.

The root builds the registry from `built` and the planned folder, and the command tree from `built`. A package must export `Row` and `New` or the join does not compile, and `gen --check` fails when the file is stale.

**Why this beats a hand-written join.** A hand-written join conflicts whenever two changes add neighbouring lines, and each conflict costs a rebase, a rerun of the checks and often a new review. A generated join conflicts the same way as text, since two packages whose names sit side by side still change adjacent lines, but the fix is a rule with no judgement: run the generator, take the output. A landing step can do that itself for paths a generator owns, and send anything hand-written back (gates.protection.5).

**Why it beats `init`.** Self-registration through blank imports removes the file, and one surveyed program does it (docker-commands-register). It makes the command set depend on import side effects, lets any package register, and breaks the rule that only the entry point wires things and the rule against `init` outside main (go.layout.2). A generated join keeps the effect of a registry and the property of a static one.

## 8. What is generated, by what, and held by what

| Output | From | Written by | Held by |
|---|---|---|---|
| the join in `internal/cli/root/` | the folders of `internal/cli/cmd/` | the gen tool's commands step | `gen --check`, the compiler |
| the command document, the command tree file, the README table | the joined registry | the docs generator | `gen --check` |
| the root help (`docs/help.txt`) | the joined registry | the docs generator | `gen --check`, and a journey that compares the binary to it |
| the layer table's document and the linter's import rows | `layers.yaml` | the layers generator | `gen --check`, the layer tests |
| the surface document | code and per-package marks | the surface generator | `gen --check` |
| the landing check's closure list | the package graph | the gate's judge step | the gate |
| the agent index block | front matter of the per-area notes | the agents generator | `gen --check` |

This table is the command line program's share of a wider question: which files in a whole repository every feature edits, and how each kind is laid out. The shared-files chapter, [guide.md of the architecture part](../../base/arch/guide.md), has the taxonomy of every kind of shared file and the principles behind this table, and [guide.md of the gates part](../../base/gates/guide.md) has what happens when two changes meet in a generated file and how a queue of changes is proved; they are not repeated here.

Documents generated and not committed are an alternative (gh-gen-docs); committed and checked is the choice when a person must read the file in review or an outside reader cites it (kubectl-verify-docs).

## 9. Per-feature protection

This section assumes a regime some projects do not have: a protected-path list, kept in the repository, of files that can weaken a check (test oracles, goldens, the gate's own code), where a change to a listed file needs a stored verdict, a signed-off record from a reviewer who did not write the change, before it lands. A project without that regime can skip this section; the rest of the layout stands without it.

A golden is a test oracle, so it is protected: a change to it needs a stored verdict from a reviewer who did not make the change. A protected list that names a whole `testdata/` folder buys one verdict for everything in it, and every new feature re-buys it. With a pattern (`internal/cli/cmd/*/testdata/`), a verdict covers one command's goldens. When a rebase leaves those files byte-identical, the verdict stays valid, so another command landing first costs nothing. The list's grammar gains one case, a `*` that stands for one path segment and never crosses a slash, and the list's schema test gets one more case (gates.protection.3).

The root help stops being protected: it becomes a generated output of a protected generator, and its oracle is the per-command goldens (cli.output.3). Cross-cutting goldens that list every command retire, because the same facts are generator outputs held by `gen --check` (cli.output.11).

## 10. Trade-offs

- **Lost: one glance at every command.** Got back by the generated command document and the generated root help.
- **Lost: the command table's order as a visible list.** Got back by `Order` in each row, a test that the numbers are unique, and a generated table in that order.
- **More packages, more `go list` and lint time.** About one small package per family. The fan-out number of the root rises once, because the join imports them all, and the measure's answers file needs that one answer.
- **Generated files still conflict textually.** Two changes that add alphabetical neighbours conflict in the join. The resolution is the generator, run by the builder or by the landing step.
- **The root help is derived text.** A wrong summary in one package shows in the help document's diff, which `gen --check` forces into the change, so it is still read; nobody hand-approves the whole page.
- **Two shapes for one thing.** A planned file and a Go row describe the same command. Both are validated by the registry, and a command generator turns one into the other.
- **A weaker ratchet where a chain was.** A digest chain of surface changes becomes one note per change; the second of two parallel changes regenerates, and edits only its own note.
- **A big move, once.** Moving the table into planned files and the goldens into folders is a large mechanical diff. `gen --check` plus identical rendered output is the proof.

## 11. Adding a command

1. Pick the family. A new verb of an existing family is a new file in its package; a new family is a new folder under `internal/cli/cmd/`.
2. If the command was planned, delete `internal/cli/planned/<name>.yaml` in this change. The registry test fails on a name in both places.
3. Write the row next to the command: name, order, summary, two examples, exit words, JSON kind, files written, menu place. Write `New`, which draws through the terminal port.
4. Write the command's tests and its goldens in the package's `testdata/`, and its journey goldens in `internal/journeys/testdata/<command>/`. Read the goldens: they are the hand-read oracle.
5. Run the generator. It rewrites the join, the command document, the command tree file, the README table and the root help. Commit them with the command.
6. Run the checks. A new package needs a line in the layer table only when no folder pattern covers it.
7. On a rebase conflict in a generated file, run the generator and take the result. A conflict in anything you wrote is yours to resolve.

Touching a file outside the command's folder, the planned folder and the generated outputs is the sign that something still has a shared owner: raise it as a rule gap rather than editing the shared file.

## 12. The CLI in the development cycle

A CLI takes the Go lines where it is written in Go (go.cycle.1 to .4), and where it draws a screen it takes the screen lines (cli.cycle.1 and .2): one golden per output at 80 columns, read by a person, with the narrow widths as properties, and pty journeys that read until their final frame. Where it writes a user's files it takes the short crash run at push and the long one at release (tests.journeys.7, tests.journeys.9), with the lock and run record before the first write (cli.commands.3). Where it parses input it did not write it has fuzz seeds in every tier and caps proved by the smallest crossing input (tests.generated.1, tests.generated.7). Where it ships a binary its release runs from a fresh clone of the tag with the dry run as a member and the reproducible build per platform (supply.release.10, supply.release.11). The moments themselves, their budgets and the core rules are the tests guide's sections 6 to 10.

## 13. Glossary

- **Golden:** a file holding the exact output a command printed when a person last read and accepted it; a test compares later runs with it.
- **Journey:** a test that runs the built program as a user would and compares what it prints.
- **Plan screen:** the answer of a command that changes something: it shows what it would do and writes nothing until it is run again with the apply flag.
- **Terminal port:** the one interface through which commands draw to the terminal, so no command prints on its own and a fake can stand in for tests.
- **Port:** an interface to something outside the program (the terminal, the file system, a tool), with an adapter per real thing and a fake for tests.
- **`gen --check`:** the check that runs every generator in memory and fails when a committed generated file differs from what it would write.
- **Verdict (stored verdict):** a record, kept in the repository, that an independent reviewer read a protected file at one exact content and accepted it; a change to the content needs a new one.
- **Closure list, judge step:** in a project with that regime, the landing check is itself a program; its closure list names the files that program is built from, and the judge step writes the list. A change to one of those files is protected like a golden.
- **Measure's answers file:** a project-kept file of accepted answers to a size or fan-out measure, so a deliberate rise (the root importing every command package) is recorded once instead of failing every run.
- **Command table:** the program's own numbered list of its commands, in its specification or its docs, the source of `Order`.
