---
type: reason
status: current
as-of: 2026-11-16
---

# The command-line choices behind the cli rules

These fifty-four rules of the cli part cover commands, config, environment, errors, exit codes, look, output and processes. Each keeps its status and carries its reason on this page.

## Commands

- **cli.commands.1** (Hard, active): A dry run that wrote caused 9 of 60 sampled bugs in the output-truth class. The goal is that nothing changes without a person seeing the change first: for a tool that edits a machine the dry run is the only way to show it, and for a store with change sets the draft is the dry run, so a second mode there would be code with no reader. Declaring the form per command keeps the matrix test and the help honest.
- **cli.commands.11** (Hard, active): The check proves name and type consistency; same meaning is judgement. A unit on every duration and size stops the oldest config bug (is 15 seconds or minutes), and a short-form list keeps an agent's flags readable.
- **cli.commands.12** (Hard, active): Off a terminal a bare call did not do what was asked, so it is a usage outcome and a script can see it. One piped half ran in an earlier trial; the pty half needs a pseudo-terminal helper.
- **cli.commands.13** (Hard, active): A self-ticking redraw wakes the machine for nothing and hides a stall behind motion. Slow steps in the real program are found only if fixtures can slow them.
- **cli.commands.14** (Hard, active): Whether the run reads well is review (cli.commands.16). A banner on every run contradicts the target programs, which show it in the menu only.
- **cli.commands.15** (Hard, active): Scoped by the journey rules of the tests part. A phone over ssh is 40 to 60 columns.
- **cli.commands.17** (Hard, active): Two code paths for one plan drift: the dry run says one thing and the page another. One function makes the preview a person sees in the terminal and on a page the same fact.
- **cli.commands.18** (Hard, later): A tool that changes somebody's machine and cannot say what it found, why it acts, how to undo it and where the claim comes from is a tool a person stops trusting. Fixed sections make the lesson generatable and testable, and the sources are the one defence against an agent inventing a tuning.
- **cli.commands.19** (Hard, later): A feature that fails quietly is found by its owner months later; a doctor line per piece is the cheapest place to see it, and the fix line turns a finding into a step.
- **cli.commands.20** (Hard, later): A command described once and rendered everywhere cannot drift: the agent's skill, the MCP tool and the README are the program because they come from it. All three target programs already generate at least two surfaces from one source.
- **cli.commands.21** (Hard, active): The one change a person cannot undo from outside is the one that removes their way in; a refusal before the write is the only guard that works after it.
- **cli.commands.22** (Hard, active): Exit 0 is a claim about a command, not about the machine, and a tool that says done when the state is wrong is worse than one that says nothing. Reading back is cheap, and it is the same check the lesson prints for a person (cli.commands.18).
- **cli.commands.23** (Hard, active): A half-written config after a power cut and a vendor file overwritten by an update are the two oldest ways a tool breaks a machine; two target programs wrote the same guard. The generated-file half stops a merge from silently un-generating a file.
- **cli.commands.24** (Hard, active): A versioned record with fixed fields is what lets a record written by one backend be read after that backend is replaced.
- **cli.commands.25** (Hard, active): The digest check is the difference between an undo and a second accident, and per-unit records keep one bad item from blocking or tangling the rest.
- **cli.commands.26** (Hard, active): A shared verb list lets an agent that learned one program drive another without reading its help; the base plus one declared table keeps the list extendable without each program inventing synonyms.
- **cli.commands.27** (Hard, proposed): Privilege is the boundary that matters to a person deciding what to run; an operating-system split puts a root action beside an owner one on the next platform.
- **cli.commands.28** (Hard, active): The fresh-folder default is what makes a restore safe to run by reflex; a restore over live data is the one backup action that can destroy what it was meant to save.
- **cli.commands.3** (Hard, active): State and concurrency caused 5 of 60 sampled bugs. The invariant is that two writers never interleave, and a database transaction holds it better than a file lock beside the database, so the mechanism is the store's. A typed name is the one confirmation an agent cannot answer by reflex; two target programs chose the same shape independently.
- **cli.commands.7** (Hard, active): Which of the three outcomes a function has is semantic, but what is undone after a failure is a fact a test can see, and it is what keeps a program safe to interrupt for its whole life.
- **cli.commands.8** (Hard, active): Help that lies caused 9 of 60 sampled bugs. Help with the same parts across every program lets a person or an agent learn one tool and read all of them, and the resolving pointer test keeps help from pointing at a topic that moved.
- **cli.commands.9** (Lens, active): 9 of 60 sampled bugs say what the code does not do. The list in data keeps the design and the program in step without a stub in shipped code, and naming the built alternatives turns a dead end into a next step. Wording truth cannot be judged by a program.

## Config

- **cli.config.1** (Hard, active): A setting that is not in the file of a rules-file program is a setting nobody can audit; a service in a container has no file and reads the environment by design. One helper with declared sources serves both and keeps the precedence table where two sources are on.
- **cli.config.10** (Hard, active): A default used because a file was unreadable, and an absent list that means everything, are two of the places a config reader lies to its owner.
- **cli.config.11** (Hard, active): This is the config half of ports and adapters: a second backend is one new table, and value-or-table lets a key grow without breaking a file written years before.
- **cli.config.12** (Hard, active): A config writable by a non-root user that a root process runs commands from is a privilege escalation.
- **cli.config.2** (Hard, active): The planted-key test exists in four repositories. Two spellings read at once is the state two target programs measured as the source of silent misreads, and the migrate command in the refusal makes a transition window unnecessary. Tolerant reading is a courtesy to a peer you trust and a hole for one you do not.
- **cli.config.3** (Hard, active): One reader is what lets precedence and the colour decision be tested once.
- **cli.config.4** (Hard, active): A key in a group-readable file and a token readable from a dump are known leaks; the `_file` suffix makes a secret reference recognisable in any config.
- **cli.config.5** (Hard, active): Hand-built home paths in non-test code today: 19, 4, 1, 3 and 0 across five repositories. The folder depends on who runs the program and on which system, and saying so is what makes the helper right on the next platform; keeping history and generated output out of the config folder keeps it readable after a year of runs.
- **cli.config.6** (Hard, active): One fault on record was a parsed key read by nothing. A silently ignored key and a value that reaches an argv unchecked are two of the places a config reader lies to its owner.
- **cli.config.8** (Hard, active): The equivalence proof is what makes a migration safe to run on an owner's config, and the printed way back is its undo.
- **cli.config.9** (Hard, active): Override chains between files are the config bug that takes an afternoon to find, and refusing duplicates costs one loop at load. One table per thing keeps it deletable in one place.

## Environment

- **cli.environment.14** (Hard, active): A form with no flag blocks an agent and a script; naming it in code is what lets cli.environment.8 refuse with the flag.
- **cli.environment.8** (Hard, active): The refused code is 5. One shared `--yes` is what lets an agent run any command non-interactively without learning a flag per prompt.

## Errors

- **cli.errors.1** (Hard, active): One program prints its version line on stdout after a typo. A warning with no pointer leaves the person where an error would have; this is a marginal assertion on the matrix helper.
- **cli.errors.2** (Hard, active): Held in Go. One value for one refusal makes a new surface (MCP, HTTP, a page) print the right thing with no new code.
- **cli.errors.5** (Hard, active): Typed results through one place are what make a CLI and an API print the same refusal; an exception crossing the executor is an outcome no client renders.
- **cli.errors.6** (Hard, active): Observed plus valid plus the next line is the shape that lets an agent fix its own mistake without a second round trip; the ban on invalid is a one-word lint that forces the message to say what is wrong.

## Exit codes

- **cli.exit.1** (Hard, active): Today one program uses 0, 1, 2, 64, 69, 75 and 78; the shared module uses 0 to 4 with 1 for both refused and failed; three others use 1 for everything; one TypeScript program uses three codes with refusal and bad usage both 1.
- **cli.exit.3** (Hard, active): The four CLIs disagree on `--version`, and it fails in three of them. The version line is the first thing a person pastes into a bug report and the first thing a reader checks against a provider, so it carries the tag, the commit and the API number, the same shape in any language. This rule starts the matrix helper that the output, config, environment and signal rules reuse.

## Look

- **cli.look.1** (Hard, active): Held in one repository with a plant, planned for the others. The rule describes a seam, not a library, so it holds the same in TypeScript. depguard is GPL-3.0 and runs inside golangci-lint as a tool, so there is no linking concern.

## Output

- **cli.output.1** (Hard, active): `--json` exists on 11, 10, 10 and 5 leaves across four CLIs. What a reader needs is one field it can find and compare; whether it holds 3 or launcher/3 is a program's choice stated once. This is a marginal assertion on the matrix helper, whose cost is carried by the colour rule (cli.environment.1).
- **cli.output.10** (Hard, active): A number with two decimals from a whole-watt meter is a lie with a straight face, and a remainder spread over the rest makes every row wrong.
- **cli.output.2** (Hard, active): A command that encodes its own JSON drifts from the schema version rule, and result types in the API package are what make the CLI's `--json` and the HTTP answer one type.
- **cli.output.4** (Hard, active): The target programs disagree because they answer for different readers: a program that owns every reader found that a compatibility layer is where its bugs lived, and a program that ships to readers it never sees must not break them. One rule with a declared reader mode keeps the check mechanical. One program moves every exit code at its next minor release; a tag diff is how that move is seen to be intended.
- **cli.output.5** (Hard, active): One program defines its own colour flag today.
- **cli.output.6** (Hard, active): The shared result type gives every write the same ending, changes, next line and error for an agent to read.
- **cli.output.7** (Hard, active): Stream discipline is what makes a pipe work in any language, a path on every applied line makes a transcript a record, and content rather than a count is the difference between an answer and a boast.
- **cli.output.8** (Hard, later): Parity is what makes a page, an agent and a person see the same facts, and the walk over the route index keeps it true when a route is added years later. Two target programs wrote the same test.
- **cli.output.9** (Hard, active): This is the property that decides whether a status tool can be believed at 3 a.m. One rule across every surface is cheaper than three and keeps a page from showing fine over an API that said unknown.

## Processes

- **cli.process.1** (Hard, active): An effect outside the wrapper is invisible to the record and to the journal that precedes effects (the outward-effect journal of the data part, store.consistency.4). One adapter per dependency is the whole value of ports: replacing a tool is one package, not a search. The resolving check and the closed table close the two ways a program name is smuggled past the import rule.
- **cli.process.2** (Hard, active): Each of these is a known class of bug: PATH hijack, argv injection, a hung child, and a reply that grows without bound. Every program that runs another faces them, in any language.
- **cli.process.3** (Hard, active): A plug-in is code the program did not write, running with the program's authority; a bare environment keeps secrets out of it, and a versioned, capped protocol keeps a new or hostile plug-in from being misread.

## Rules it serves

arch.decisions.1, arch.decisions.4, docs.links.2
