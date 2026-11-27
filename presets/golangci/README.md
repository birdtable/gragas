# golangci-lint base

`base.yml` is the shared golangci-lint v2 config for Go projects, pinned at golangci-lint 2.15.0. It is written by hand; every linter and setting in it carries a comment naming the rule ids it holds, and each of those rules names this preset in its check.

## What it holds

| Linter or setting | Rule ids |
| --- | --- |
| `standard` set (errcheck, govet, ineffassign, staticcheck, unused) | go.errors.1, go.errors.3 (errcheck with check-type-assertions and check-blank, tests excluded); code.format.2 (govet defaults, staticcheck all but the non-default ST checks); code.naming.1 (ST1003); go.errors.5 (ST1005, pending its ruling); go.concurrency.2 (govet waitgroup) |
| `issues`: both caps 0, `uniq-by-line: false` | go.errors.4, gates.plants.10 |
| bodyclose, noctx | go.style.4, go.concurrency.1 |
| contextcheck | go.concurrency.1 |
| depguard | cli.config.7, cli.process.1, cli.look.1, arch.shared.7, arch.ports.11, arch.ports.24, tests.doubles.2, go.style.5 (no assertion library in a test) |
| dupl | code.duplication.1 |
| errorlint (wrapcheck off) | go.errors.2 |
| exhaustive | code.idioms.3 |
| exptostd, modernize, usestdlibvars | code.idioms.2 |
| forbidigo (analyze-types) | go.errors.3, go.errors.4, go.concurrency.1, cli.config.3, cli.config.5, cli.errors.3, cli.output.2, cli.process.1, arch.ports.1, arch.ports.24, tests.admission.5, tests.isolation.6 |
| gochecknoglobals | arch.ports.12 |
| gochecknoinits | go.layout.2 |
| gocognit 20 | code.size.3 |
| gocritic commentedOutCode | code.comments.3 |
| gocyclo 30 | code.idioms.4 |
| godoclint | code.comments.4 |
| gomoddirectives (`replace-local` refused, `go-version-pattern` and `toolchain-pattern` name a release to the patch) | go.layout.3, go.cycle.3 |
| gomodguard_v2 (`blocked`: each module a chosen row of the register of libraries replaces and bans, with the row's why as `reason` and its module as `recommendations`; a repo's own rows go in its own config) | supply.intake.6 |
| gosec, every rule with no `includes` or `excludes` list, so G201, G202, G204 and G404 run; each exclusion names its rule ids in `text` with its paths and reason; every `#nosec` and `gosec:disable` comment ignored (`config.global.nosec`), and a generated-file header silencing nothing (`exclusions.generated: disable`) | cli.process.1, gates.exemptions.1, supply.scans.3 |
| iface | arch.ports.10 |
| interfacebloat 10 | go.style.1 |
| nolintlint | gates.exemptions.1, gates.exemptions.2 |
| paralleltest (`ignore-missing`, `ignore-missing-subtests` off) | go.cycle.2, tests.runners.4 (a serial test says `//nolint:paralleltest // serial: <reason>`; nolintlint cannot hold the text, so the project's suppression list does) |
| revive | code.idioms.1, code.naming.1, code.comments.4, code.size.4, go.errors.4, go.concurrency.2, arch.structure.7 |
| sloglint | go.logging.1, go.logging.2 |
| thelper (begin only) | go.style.5 |
| tparallel | go.cycle.2, tests.runners.4 |

## Left to the project

Lines starting `# per-project:` hold what only a project knows: its runner, shared UI module, paths helper and adapter paths (as exclusions); the layer and port rows rendered from its layer table (arch.layers.2, .5, .7, .9, arch.ports.1, .2, .8); the shared UI module's literals and constructors (cli.errors.2, cli.look.4, cli.look.5, cli.output.5); the `log` ban for a daemon (go.logging.1); sqlclosecheck and rowserrcheck where `database/sql` is imported (go.style.4).

Not expressible in golangci-lint: the table-case names of go.style.5, the shrink-only allow lists and ratchets (go.errors.3, code.size.3, tests.admission.5, tests.isolation.6), argument-limit for new code only (code.size.4; the setting is repo-wide), the module plugins (go.style.3, tests.isolation.1), and the ast-grep halves of go.concurrency.2 and arch.ports.6.

## How a project uses it

The project's `.golangci.yml` is generated from `base.yml`, unchanged, and the project's own file; it is never edited by hand.

1. Copy `base.yml` to `lint/base.yml`, unchanged. The places a project may change are marked in it by a pair of lines, `# slot <name> begin` and `# slot <name> end`; what lies between them is the default.
2. Write `lint/project.yml`: a YAML mapping from a slot's name to the text that takes the default's place, as a literal block. A slot the file does not name keeps the default. Write the text without its indent: the generator indents it to the slot's marker. The one exception is a slot whose marker is at column 0 (`overrides`), which keeps the text's own indentation, so a rule added there must match the indent of the list it joins. In the text, `{{programs}}` in it is the exclusions of the programs the project's layer table names. Give each override a comment with the rule id and the reason.
3. Run the generator (`gragas-dev layers` in gragas), which writes `.golangci.yml` whole and drops the slot lines. The `layer-rows` and `exclusions-generated` slots are always filled from the layer table.
4. Run `golangci-lint config verify` and `golangci-lint run ./...` with the pinned release binary, and add the same version to CI.
5. Run the lint once, as `golangci-lint run ./...`. A product holds no file tagged for an operating system (arch.ports.4, arch.ports.24), so one run reads every file it ships, and the preset sets no `run.build-tags`.
6. Leave the `os-host` slot at `["$all"]`: a product has no host layer, so depguard spares no folder, and `gragas check` lists every Go file whose name or build line names a system (arch.ports.24, `os-files`) unless a row of the project's exemptions file names it. A project that has not moved a folder of operating-system code to a platform library yet spares that folder as `"!**/<folder>/**"` and gives each file in it a row of the exemptions file; the rows only fall.

A diff of `lint/base.yml` against `base.yml` shows drift: they must match byte for byte.
