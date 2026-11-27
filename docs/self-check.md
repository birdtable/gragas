---
type: register
status: current
as-of: 2026-11-09
for: The Hard rules of the go, cli, code, tests, arch, gates, supply, agents and ts parts that gragas does not meet yet
---

# Self-check

To see what `gragas check` finds in gragas, with the row that excuses each, run `go test -v ./cmd/gragas -run TestGragasAuditsItself`.

The Hard rules of gragas's own parts (go, cli, code, tests, arch, gates, supply,
agents and ts) that gragas does not meet yet: what is unmet, and what closes it. In go, code,
tests and arch a rule not listed is met, or does not apply to a one-binary CLI with no daemon, store, network or
secret; for gates, supply, agents, ts and cli, "The cycle's parts" below says what a row means. Where a rule names "the shared look module", "the shared runner" and
so on, it means the shared kit's package; "the shared kit, not placed" means only a change
to the shared kit meets it, and gragas does not edit the shared kit. "Not placed" means the
operator has not given the work a milestone yet.

Read on 6 Nov 2026 against the rulebook at this commit, after the layout
moved to the Go program template (go.layout.1), the layer table became one
file, `layers.yaml`, and a review's holes in its enforcement were closed:
the pure layers take an allow list of standard packages, every recipe is
read for build tags and cgo, the operations hand out no domain type, and
every exception is listed in `suppressions.txt`.

## Where only the shared kit, or the rule, can move

| Rule | What is unmet | Closes |
|---|---|---|
| cli.exit.1 | gragas holds the seven codes 0 to 6 from the shared kit's exit package (gragas#731), but no test parses a documented table against the constants and runs the built binary through the refused, failed and missing-port paths | not placed |
| cli.exit.2 | the seven codes are gragas's own, declared once in `internal/exit` and built there into go-run/exit's table (gragas#1452), and every command takes them from there since the landing judge's three literals went (gragas#912). The rule's main text is met, and its detail names the program's own exit leaf as the codes' owner. No preset tool key checks it | not placed |
| cli.errors.2, cli.errors.6, cli.look.5 | `ui.Refused(why)` and `ui.Failed(what)` take no next line or observed value, so gragas's own ending carries the line (`said` when the sentence already holds it in backticks; the answer never reads it back out of the text, gragas#917); no constructor-only check | the shared kit, not placed |
| cli.output.5, cli.errors.3 | the shared kit's ui package registers the four output flags only inside `ui.Execute`, and gragas's help under `--json` and the menu's second run read the tree outside it, so `internal/cli/root` registers `--json`, `--plain`, `--no-color` and `--accessible` itself; no variable is set for them, as main reads the environment once before the root runs (gragas#917); `internal/cli/root/answer.go` writes the document to stdout outside the module | the shared kit, not placed |
| cli.environment.1 to .7, .11 to .14 | the shared kit's ui/uitest package has no matrix or conformance function; gragas's terminal rows run through a pipe only, force colour with `CLICOLOR_FORCE` alone and never compare stripped forced output | the shared kit, not placed |
| cli.look.3, .4, .6, .9 | none of the literal checks exist (format strings, escapes, footer forms, emoji, spacing calls); `version` passes a column of 8; the shared kit has no dry-run footer, so a dry run ends on its next line alone | the shared kit, not placed |
| cli.look.7 | the shared kit's ui package's `Table` drops columns at 40 columns without marking them (`check-40.golden`) | the shared kit, not placed |
| cli.look.8, cli.output.3 | the goldens are plain only, with no role-tagged ones; `-update` is not refused under the gate, and check, init and version have no `--help` golden | the shared kit, not placed |
| cli.output.7 | no test splits stdout and stderr per command, and `ui.Change` takes no required path | the shared kit, not placed |
| cli.commands.3, .4, .25 | the lock and run record go through ports now, but no analyser holds lock-before-write, the way back is never exercised, the record keeps no copy or digest, and there is no scratch-root sandbox test | the shared kit, not placed |
| cli.errors.1 | only a run that writes has a run record, so only its errors carry an id; a warning names no pointer and the JSON error has `kind` and `hint`, not a code and `next` | rule and the shared kit, not placed |
| arch.ports.12 | gragas declares no mutable state of its own but the two strings the linker stamps into main, yet each run drives and resets the shared kit's ui package's variables (`DryRun`, `JSON`, `Ended`...) | the shared kit, not placed |
| go.logging.2 | no line of a run carries a run id from one handler; the shared logger is the shared kit's to build | the shared kit, not placed |
| go.errors.8 | `internal/cli/root/errors.go` reads cobra's error text (`unknown command`, `unknown flag: --`) with `strings.HasPrefix`, because cobra types neither error; it is the one line of `suppressions.txt` of its kind, held by a repository test that finds error text read in any file | rule, not placed |
| arch.shared.6, .7 | `.golangci.yml` is joined from the generated `presets/golangci-lint/.golangci.yml` and `lint/project.yml` (gragas#1744), not imported, because golangci-lint cannot import a config; what decides whether a finding is silenced is compared (internal/lintconfig_test.go): the default and enabled linters, gosec and nolintlint, the generated, preset, path and rule exclusions, the issue caps and whether tests are linted. The rest is not | M4, with the presets |

## Not built yet

| Rules | What is unmet | Closes |
|---|---|---|
| arch.decisions.1, .2, .4 | gragas holds no decision records of its own; the decisions it follows live in the operator's standards repository | not placed |
| arch.layers.9 | `layers.yaml` renders the depguard rows and the layer doc, but the layer check is gragas's own test in `internal/`, not one shared program every site imports, and no protected-path list holds the table (gragas#912 retired the list) | not placed |
| arch.ports.2 | gragas names no provider in shipped source, but no word-list test holds it | not placed |
| arch.ports.6 | the owner list in `layers.yaml` is per import path: the shared kit's root module is reached from four packages (write, lock and run each from its adapter, version from the command tree), and the list has no shrink-only ratchet | not placed |
| arch.ports.7 | `--json`, `rulebook/rules.json` and `rulebook/bindings.json` cross a program boundary with no consumer-driven contract | not placed |
| arch.ports.15 | the ports are named in `layers.yaml`, not in a target architecture document with a section per port; yaml.v3 enters through the leaf `internal/ordered`, and cobra, pflag and the shared kit's ui package through the command tree, not as adapters on a port | not placed |
| arch.structure.1, .2 | no classification table or door trailer, and no land check for a complete extension | not placed |
| arch.structure.8 | this restructure was proved by the journeys' goldens and the in-process tests; no transcript of every command was committed first, and no gate reruns one | not placed |
| arch.templates.1 to .4 | no template or generator a program comes from | M4, with `init` |
| arch.shared.1, .5, .9, .10, .14 | gragas copies steps from the tools (a sibling tool's isolation, now `internal/config/testing`, the Taskfile, hook and `.gitleaks.toml`; a sibling tool's JSON answer) with no census or byte check; as the source of presets it has no named owner file, surface listing or place to ask | M4, with the presets release |
| arch.shared.8, .15 | the shared kit's write package replaces a hand copy, but no ban holds `os.WriteFile`, `os.Create` or a rename outside its adapter, and no shape rule points at it (also cli.commands.23); there is no registry of the shared capabilities for the bans and the instruction block to be rendered from (arch.shared.15) | gragas#513 (the registry and the rendered bans); the digest and dev-write bans come with the steps that add their owners |
| cli.commands.23 | a replace keeping a group a new file would not be given is read back from the file only where this account may give a second group; inside the gate's network block, a user namespace that maps only its own group, that half of `TestAReplaceKeepsTheModeAndOwnerOfTheFileItReplaces` does not run and says so in the test log, since a skip needs a suppression and that list only shrinks; a seam that sees the group the replace gives stands in for it | not placed |
| go.errors.4 | endings map to exit codes in one function (`exitFor`), but no count check holds it to one | not placed |
| go.errors.6 | no nil-flow analysis (nilaway is not run) | not placed |
| cli.output.1, cli.commands.10 | a mistyped command under `--json` exits 2 with a document on stdout, where both rules ask for an empty stdout; the JSON test lists its commands by hand | not placed |
| cli.output.2 | the encoder ban now covers `cmd/` and `internal/cli/`, sparing only the root's renderer, but the commands still build their `--json` data as maps, not as `internal/core/api` types | not placed |
| cli.output.4 | the command tree is not written to a committed file and compared with the last tag; there is no tag | M4, with the first release |
| cli.output.6, cli.commands.17 | no test decodes every command's `--json` strictly, and none compares the printed plan with the answer's changes | not placed |
| cli.output.1 | the old `check` and `gen` name a `schema` number in their `--json` documents and no top-level `contract` field, because their recorded owner transcripts hold that shape; `version` and `help` name `contract` since M0a-24 | M6-21, with the legacy mount |
| cli.output.9 | `check --json` does not mark that the names check did not run (`version` is met since M0a-24: a tag or a commit it cannot establish is null, with its reason under `notes`) | not placed |
| cli.exit.3 | `version --json` names the toolchain `go`, not `toolchain`, and no test compares it with the build info | not placed |
| cli.commands.1, .8, .11, .26 | the writes annotation names no form; init's help has no example and version and init do not say what they write; `-C` is not on a shared short list; there is no verb table or grammar test | not placed |
| cli.commands.6, .22 | no double-release test of the lock, and an apply does not regenerate to prove the files are current | not placed |
| cli.commands.12 | a bare `gragas` off a terminal prints help and exits 0, not 2 | not placed |
| cli.commands.13 | no progress: a rulebook of thousands of files is read in silence | not placed |
| cli.commands.14 | the banner draws at a terminal for check, gen and help, no test checks the last line (`version` ends on done since M0a-24) | not placed |
| cli.commands.15 | the journeys run through a pipe, not a pty, and there is no first-run or menu journey | not placed |
| cli.config.3, .4, .5, .6 | the environment exclusion now spares only `internal/config`'s own files and main, but forbidigo misses `os.ExpandEnv` and `syscall.Getenv`; the state folder is `~/.local/state` on macOS too; a relative `XDG_STATE_HOME` is ignored, not refused; there is no planted-secret test and gitleaks scans the tree, not staged diffs | not placed |
| cli.look.1 | no similarity test against the shared kit's ui package's files and no adapters repo-check | not placed |
| code.comments.2, .5 | no check refuses a bare TODO or a decorated or long comment block | not placed |
| code.config.1 | no test scans non-config source for one installation's values | not placed |
| code.deadcode.1 | deadcode is not run (`go mod tidy` is checked for a diff by the tidy member) | not placed |
| code.deadcode.2 | no scan for not-implemented bodies or placeholder returns | not placed |
| code.duplication.1, .2 | the cross-package clone pass (deleted with `gragas-dev measure`, #911) skips test files, and no pass ignores identifiers, so a renamed copy is not seen | not placed |
| code.format.1 | no import-order tool next to gofmt | not placed |
| code.naming.2, .4 | no lint of file names, and no test holds flags, paths and JSON fields to a table of terms | not placed |
| code.size.1 | no file-length report or baseline | not placed |
| tests.admission.1 to .4, .6 | no admission ladder at land: no seeded repeats, budget step, mutants, assertion or tautology analyser, cost line or edited-test scan | not placed |
| tests.doubles.3 to .7 | no file marking each dependency managed or unmanaged; the one fake in `internal/port/testing` is hand-written with no contract suite run against both it and the adapters; no schema walk of `--json` samples; no stand-in-value scan | not placed |
| tests.flakes.1 | the push runs the light packages unshuffled and from Go's test cache, so a push re-runs only the packages whose inputs changed: the common practice, over the stricter rule (gragas#970, RULED). The release runs them shuffled and uncached | not placed |
| tests.flakes.4, .5; tests.isolation.4, .10 | no retry closes a red, but no gate scan holds the test flags, no typed hygiene check exists, and an infrastructure fault is not told apart | not placed |
| tests.generated.3, .5 | no anti-vacuity test at the end of a property file, and no diff check over the goldens | not placed |
| tests.isolation.1, .7, .8, .9, .11, .12 | isolation keeps agent-session variables and the journeys' build runs with a nil environment; the tests that read the machine are not listed; no gate with a symlinked temp folder; no verdict line naming the OS; the in-process command tests share the shared kit's ui package's state and are not listed | not placed |
| tests.journeys.1 to .4, .6 | no journey map or per-command journey test; journeys share one state folder and no scan holds the binary to the contract package; a stale or unstamped binary is not refused and no leftover scan follows a run | not placed |
| tests.measures.2, .6 | no tier budgets, floors or comparison helper | not placed |
| tests.runners.2, .3 | the test flags live in gragas's Taskfile, not a shared preset, and no `-p` comes from a slot variable | M4, with the presets |
| supply.workflows.5 | gragas's one workflow, `test.yml`, runs on a pull request, where the rule asks for a workflow on a tag and by hand only; the rule is the rulebook owner's to amend | not placed |

## The cycle's parts: held by no check yet

Read on 9 Nov 2026 against the gates, supply, agents, ts and cli parts. The rules the
three stack and cycle PRs (671, 672, 674) added are all Lens: none is Hard, so none is listed
for them. A row here is an active Hard rule of one of these parts that no resolved check
holds (its standing is planned in `rulebook/rules.json`). It does not say gragas breaks the
rule: gragas may meet it by hand, and a check that holds it is what is missing. A Hard rule
with a resolved check is in no row, and a test (`internal/repo/rulebook/selfcheck_test.go`) fails an active
Hard rule of these parts that is neither held by a check nor a row of this page.

| Rules | What is unmet | Closes |
|---|---|---|
| gates.command.1, .2, .3, .4, .5, .8, .12, .13 | no resolved check holds them: one gate command; runs without helper tool; home tier and budget; and 5 more | not placed |
| gates.exemptions.4 | no resolved check holds it: one exit record | #684 |
| gates.hooks.1, .2, .4, .5 | no resolved check holds them: pre push runs the gate; hooks armed; hooks stay light; and 1 more | not placed |
| gates.land.1, .2, .3, .4, .7, .8, .11, .12, .14 | no resolved check holds them: main moves only on accepted commit; verdict bound to tree; lander recomputes verdict; and 6 more | not placed |
| gates.measures.1, .2 | no resolved check holds them: numbers by script; null not zero | #708 |
| gates.own-rules.3, .6 | no resolved check holds them: generated not edited; gate never imports its subject | not placed |
| gates.plants.1, .4, .5, .8, .11, .12 | no resolved check holds them: every check has a plant; two sided pair run; fail closed; and 3 more | #695 |
| gates.protection.1, .2, .3 | no resolved check holds them: weakening needs a non author; one list of gate paths; agent cannot widen in own change | not placed |
| gates.quality.2 | no resolved check holds it: failure message names fix | not placed |
| gates.ratchets.1, .4 | no resolved check holds them: debt may not grow; debt registered | #684 |
| gates.registry.1, .3, .7, .8, .9 | no resolved check holds them: active only with a check; rule ids and claims resolve; base and packs; and 2 more | not placed |
| gates.shared.1, .7, .8, .9 | no resolved check holds them: gates shared by version; migration swept by deadline; check in gate or deleted; and 1 more | not placed |
| supply.age.3, .4 | no resolved check holds them: skips and waivers name what they pass; and 1 more | not placed |
| supply.bumps.1, .2, .3, .4 | no resolved check holds them: bump is its own change; currency window; related modules tagged together; and 1 more | not placed |
| supply.install.1, .2, .4, .6, .8, .9, .10 | no resolved check holds them: install from the lockfile only; build and check offline; exact versions; and 4 more | not placed |
| supply.intake.1, .2 | no resolved check holds them: one owner per concern; allow file and falling count | not placed |
| supply.licences.1, .2, .5, .6 | no resolved check holds them: licence on the allowlist; licence check fails closed; shared modules have licence file; and 1 more | not placed |
| supply.provenance.2, .4, .7 | no resolved check holds them: copied part records origin; regenerating leaves no diff; fetched program pinned and verified | not placed |
| supply.release.1, .2, .5, .6, .7, .9 | no resolved check holds them: publish only from the release job; undeclared api break refused; tag with changelog and no push; and 3 more | not placed |
| supply.scans.3, .4, .5, .6, .7 | no resolved check holds them: security analyser with named exclusions; sql parameterised; child process argument list; and 2 more | not placed |
| supply.secrets.1, .2, .4 | no resolved check holds them: no secret committed; config holds secret reference; private names never in the repo | not placed |
| supply.workflows.1, .2, .3, .5 | no resolved check holds them: no workflow triggers itself; workflows linted and actions pinned; ci token read only and scoped; and 1 more | not placed |
| agents.delivery.5 | no resolved check holds it: commit message shape | not placed |
| agents.discovery.3 | no resolved check holds it: type checked lookup ready | not placed |
| agents.guards.1, .2, .3, .4, .5, .6, .7 | no resolved check holds them: protected paths refused; bypass by command refused; no sweeping or discarding; and 4 more | not placed |
| agents.instructions.1, .2, .3, .5 | no resolved check holds them: short instruction map; instruction commands resolve; rules load by path; and 1 more | not placed |
| agents.review.1, .3 | no resolved check holds them: reviewer verdict blocks land; lens has its own plant | not placed |
| agents.tickets.1, .3, .4, .7, .8 | no resolved check holds them: ticket names exist; done when check ran; purpose and proof; and 2 more | not placed |
| ts.compiler.1, .2, .3, .4, .5 | no resolved check holds them: zero type errors; strictness ratchet; shared compiler base; and 2 more | not placed |
| ts.errors.1, .2, .3, .4 | no resolved check holds them: no empty catch; rethrow keeps cause; no pointless rethrow; and 1 more | not placed |
| ts.gates.1 | no resolved check holds it: node gate process discipline | not placed |
| ts.lint.1, .2, .3, .4 | no resolved check holds them: type aware correctness; type only imports explicit; lint rules are errors; and 1 more | not placed |
| ts.lint-config.1, .2, .3, .4, .5, .6, .7, .8, .9 | no resolved check holds them: bulk suppressions with line count; new rule ships as error; typed lint uncached; and 6 more | not placed |
| ts.modules.1, .2, .3, .4, .5 | no resolved check holds them: named exports only; no enum or namespace; esm with extensions; and 2 more | not placed |
| ts.packages.1 | no resolved check holds it: exports map per unit | not placed |
| ts.runtime.1, .2, .3 | no resolved check holds them: one node major; no deprecated core modules; one unit test runner | not placed |
| ts.schema.1 | no resolved check holds it: model generates every shape | not placed |
| ts.type-safety.1, .2, .3, .4, .5, .8, .9, .10, .11 | no resolved check holds them: no explicit any; no type assertions; suppression comments need a reason; and 6 more | not placed |
| cli.commands.2, .5, .7, .21, .24, .28 | no resolved check holds them: safe to run twice; install by rename; multi step operations say which; and 3 more | not placed |
| cli.config.1, .2, .8, .9, .10, .11, .12 | no resolved check holds them: one precedence order; unknown key refused; migration proves equal meaning; and 4 more | not placed |
| cli.environment.8, .9, .10 | no resolved check holds them: no question nobody can answer; nothing animates unwatched; pager rules | not placed |
| cli.errors.4, .5 | no resolved check holds them: untrusted text sanitised; nothing throws past the executor | not placed |
| cli.look.2 | no resolved check holds it: one token file | not placed |
| cli.output.10 | no resolved check holds it: numbers no finer than their source | not placed |
| cli.process.2, .3 | no resolved check holds them: how an adapter runs a program; plug in child protocol | not placed |

## Not proved on any machine

What the agent's own tools do, which no gragas test runs. None is a finding of `gragas check`.

| Where | What is not proved | Why it is not fixed |
|---|---|---|
| gragas#1761 | what the host's Write and Edit write for a path that is not clean, such as `<dir>/l/../t.txt` with `l` a folder link, and whether the hook is handed that path as typed or cleaned. A probe on the Mac: the host's Read returned the file the spelling names, and `cat` the one the kernel names | nothing to fix in gragas: `Tree.Within` answers `port.ErrNotClean` for a path that is not clean and places it nowhere (`TestWithinRefusesAPathThatIsNotClean`), and the guard refuses it (`TestTheGuardRefusesAPathThatIsNotClean`), so it never judges a file the host might not write |

## Not proved off the Mac

A change is proved on one machine, the Mac, and nothing waits for a Windows or a Linux run. These
are what that leaves unproved, each with the ticket that holds it. None is a finding of `gragas check`, so
`TestGragasAuditsItself` does not read this table.

| Where | What is not proved | Why it is not fixed |
|---|---|---|
| gragas#1687 (b) | two `cmd/gragas` linterset tests pass 2 times in 3 on Windows | no recipe came |
| gragas#1639 | a handle is held under `checkLinterSetOf`'s TempDir (`cmd/gragas/main_test.go`) on Windows | no recipe came |
| gragas#1689 (b) | `warm_test`'s base-name count | proved on the Mac only |
| gragas#1715 (M2) | containment: the Linux-only `namespace` and `namespace-caps` proof, which went with the two tasks | Linux only; nothing runs off the Mac |
| gragas#1711 | `confine.MkdirPrivate` and `confine.MkdirFresh` (`internal/host/git/remote.go`'s cache and work folders, `internal/host/fs/lock.go`'s private folders, the sandbox's and the version run's folders) on a volume with no access list (FAT, exFAT) | no ticket: go-files refuses with `ErrNoAccessList` at the first folder, by design (go-files#86, #254), and gragas passes the refusal on; nothing is made without an access list, so it is not a hole |
| gragas#1721 | `internal/host/tool/locate.go`'s `pathFolder` leaves out a PATH folder holding a double quote, because Windows' `SplitList` reads a quoted element as one folder, and that split is not run here | nothing to fix on the Mac: the check fails closed on every system, and B1's follow-up on #1703 adds a test PATH that meets it |

## Not applicable today

arch.layers.11 (no package is declared types-only), arch.layers.13 (no
design names a separable part), arch.ports.16 (no plug-in registry),
and arch.ports.18 (no capability list to read) hold only because gragas has
nothing they govern; none has a check of its own here.

## Rule conflicts met by a choice

- go.layout.1 and cli.output.2 put the operations and result types in
  `internal/core/api`, but revive's package-naming with check-extra-bad-name,
  arch.structure.7's own check, refuses a package named `api`. The folder is
  the template's and the package is named `coreapi`; `layers.yaml` says so.

## What `gragas check` finds in gragas

gragas takes its own parts and builds for the five systems of the Taskfile's `CROSS`: the two files
`internal/testdata/self-audit/parts` and `systems`, which are the input the audit is run with (there is no
`.gragas` folder at the root, so `gragas check` over gragas stays the rulebook check, and a change to either
file takes the gate team's review: narrowing them would hide a finding). `TestGragasAuditsItself` reads
gragas's own Taskfile, hooks and tool configs with them, as the check reads a project's. Each finding it still makes is a row below, with the rule, the finding's key, how many
there are, and the open issue that closes it. `TestGragasAuditsItself` (`cmd/gragas`) runs the check
in process and is red when a finding has no row, when a row's count is not the finding's, and when a
row has no finding: a new gap is red, and a fixed gap whose row stays is red. The release tier reads
each `#N` and asks GitHub whether it is open (`gragas-dev self-check issues`): a row of a closed or a
missing issue is red there. A row is an excuse, so a change to this file takes the gate team's review, who read
every new row. Where the audit misread gragas, the audit was fixed and no row was made: a `status:`
that is the declared CLI_FORCE guard is a skip at the commit and the push, and a `timeout:` on a `cmds`
entry or a `deps` entry holds it (go-task ignores one on the task itself).

| Rule | Key | Count | Issue | What is unmet | Exemption |
|---|---|---|---|---|---|
| gates.command.1 | pre-push build linux/amd64 | 1 | #1376 | the push has no build member for this system | self-gates-command-1-build-linux-amd64 |
| gates.command.1 | pre-push build linux/arm64 | 1 | #1376 | the push has no build member for this system | self-gates-command-1-build-linux-arm64 |
| gates.command.1 | pre-push build darwin/arm64 | 1 | #1376 | the push has no build member for this system | self-gates-command-1-build-darwin-arm64 |
| gates.command.1 | pre-push build windows/amd64 | 1 | #1376 | the push has no build member for this system | self-gates-command-1-build-windows-amd64 |
| gates.command.1 | pre-push build freebsd/amd64 | 1 | #1376 | the push has no build member for this system | self-gates-command-1-build-freebsd-amd64 |
| go.cycle.2 | pre-push lint linux/amd64 | 1 | #1377 | the push has no lint member for this system | self-go-cycle-2-lint-linux-amd64 |
| go.cycle.2 | pre-push lint linux/arm64 | 1 | #1377 | the push has no lint member for this system | self-go-cycle-2-lint-linux-arm64 |
| go.cycle.2 | pre-push lint darwin/arm64 | 1 | #1377 | the push has no lint member for this system | self-go-cycle-2-lint-darwin-arm64 |
| go.cycle.2 | pre-push lint windows/amd64 | 1 | #1377 | the push has no lint member for this system | self-go-cycle-2-lint-windows-amd64 |
| go.cycle.2 | pre-push lint freebsd/amd64 | 1 | #1377 | the push has no lint member for this system | self-go-cycle-2-lint-freebsd-amd64 |

## Releases

gragas releases with GoReleaser and changie, both pinned in `tools.yaml`, from
`.goreleaser.yaml`, run by the Taskfile (gragas#914). There is no release workflow:
the release runs on a person's machine, like every gate.

- `task release` is the tier a tree must pass before a tag: `hatch tag`,
  then the push tier, the heavy tests, the tests again uncached with `-race`, the
  secret scan over every commit, and the pty tests. The guard refuses a tree that is
  not clean, a HEAD that is not main, a main behind, ahead of or apart from the
  remote's, and a version, the one changie batched last, that is a tag here or on the
  remote.
- `task release-recipe` runs the one fetch (`go mod download` for each system, the
  only step with the network open), the release tier, then its own steps: the cross
  build, crash injection and `release-build`. `SNAPSHOT=1` makes it a dry release at
  any sha: the guard then holds only a clean tree and a branch that is its remote's.
- `release-build` is `cmd/gragas-release build` (gragas#1655): before anything,
  it holds the goreleaser it is given to the sha256 `tools.yaml` pins for the
  host and starts no other (a planted one ran once, and at a
  publish would have had the token). It then empties the release folder under
  the tools folder, refusing one that is the repository or holds it, and clones
  HEAD twice into it.
  In each clone, before GoReleaser runs, `cmd/gragas-release sources` reads every
  source `.goreleaser.yaml` names under `files:` and its like, and refuses one that
  is not a plain file (mode 100644) in `git ls-tree` of HEAD. GoReleaser follows a
  source that is a symbolic link and ships what it points to, which the unpack step
  never sees (gragas#1206). It then runs GoReleaser in each clone
  with the module proxy off, the second with a cold build cache, and runs
  `cmd/gragas-release`: `compare` holds the two checksum files to each other
  (supply.release.11), and `verify` holds the first folder to its checksums both
  ways, unpacks each archive, refuses a name that leaves its folder, holds the
  licence and notices to the tree's, and runs the host's binary, whose `version`
  must print the tag (supply.release.6).
- `task release-publish`, from the annotated tag a person made and pushed, runs the
  fetch, `tools-check`, the guard in `--tagged` mode (HEAD is the tag, and the remote's tag is the
  same commit), `release-build` with the first build a draft release, and then
  publishes the draft only when it holds every archive and the checksum file.
- The final run (`docs/checks.md`, The final run; gragas#2044): before the final
  pull request of release 1 lands, each machine starts the shipped binary from a
  home with no plugin approved, adds golangci-lint's plugin by path, approves it
  once by digest on the command line and runs `gragas check`, under the gragas#2007
  freeze, and the run's record names the approved digest.

Each archive holds the binary, `LICENSE`, `NOTICE` and `notices/gragas.txt`, one
notice per linked module. The systems are the Taskfile's `CROSS`, windows among
them, and `internal/release_test.go` holds `.goreleaser.yaml` to it. The release
writes no SBOM and no provenance, and signs nothing: a release is pinned by its
content hash (release-pin-by-hash), and gragas has no signing step (gragas#1556).
A source test of `cmd/gragas-release` finds no such code. `release-publish` has not
run: there is no release tag yet.
