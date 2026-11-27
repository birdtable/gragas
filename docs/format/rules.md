---
type: spec
status: current
as-of: 2026-11-09
---

# Rules: a concern file, a rule and a guide's block

A rule is one sentence in the imperative with the reasons behind it, where it applies, what it cites, and the bindings that check it. A rule is written in one of two forms, and both are the same rule: as an item of a concern file in YAML, or inside a fenced block of a part's guide in Markdown. The index of [the format](README.md) says how to read a table.

## The concern file

A concern file is `<layer>/<part>/rules/<concern>.yaml`. It holds the rules of one concern of one part.

Schema: [rules.schema.json](../../internal/host/assets/format/schema/v3/rules.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here |
| `part` | yes | The part's slug, equal to the part's folder |
| `concern` | yes | The concern's slug, equal to the file's name |
| `rules` | yes | The rules of the concern, each as the next table gives it. May be empty |

## The guide

A part's guide is a Markdown file of prose. The engine reads only the fenced blocks whose info string is `gragas` and leaves every other byte of the guide alone, so a guide keeps its prose, its links and its order. A block holds the YAML of one or more rules under the anchor of the section it sits in. The guide has no `format` field and no version of its own: its version is the rulebook's.

Schema: [block.schema.json](../../internal/host/assets/format/schema/v3/block.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `anchor` | yes | The section's anchor, written once in the guide. It may change without a rule's id changing |
| `rules` | yes | The rules of the section, at least one, each as the next table gives it |

## The rule

The same table holds a rule in a concern file and a rule in a block.

Schema: [rule.schema.json](../../internal/host/assets/format/schema/v3/rule.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | `part.concern.number`. Frozen at birth and never reused: a rule that moves or goes away is retired with `replaced_by`, and the id stays. Never starts with `gragas`, the word the engine's own check ids start with, whatever the rule's status (see [structure](structure.md)) |
| `aliases` | no | Ids the rule had before it was renumbered. Never starts with `gragas`, as for `id` |
| `statement` | yes | One sentence in the imperative. It uses terms as [terms](terms.md) defines them |
| `strength` | yes | `must`, `should` or `may` |
| `status` | yes | `proposed`, `trial`, `active` or `retired`. Typed by people. Standing is derived from the status, the bindings and their proof, and is never a field |
| `scope` | no | Where the rule applies. A field given must hold, and an absent field holds |
| `scope.parts` | no | The part slugs the rule applies to |
| `scope.paths` | no | Slash-separated globs from the project root |
| `scope.kinds` | no | Kind slugs of the rulebook's structure |
| `exemptions` | no | Exceptions to the scope, each by path or kind, each with a reason |
| `exemptions[].paths` | no | Slash-separated globs from the project root |
| `exemptions[].kinds` | no | Kind slugs of the rulebook's structure |
| `exemptions[].reason` | yes | Prose a reader acts on. An exemption names `paths`, `kinds` or both |
| `reasoning` | no | Why the rule exists, at least one of its four parts |
| `reasoning.definition` | no | What the rule means |
| `reasoning.pros` | no | The reasons for it, at least one |
| `reasoning.cons` | no | The costs of it, at least one |
| `reasoning.decision` | no | The slug of the decision that settles the rule, in [decisions](decisions.md) |
| `pairs` | no | The example pairs of the rule |
| `pairs[].number` | yes | The pair's folder, `pairs/<rule id>/<number>/`, from 1 |
| `pairs[].target` | yes | The target type the pair is written in |
| `owner` | no | Who answers for the rule |
| `review_by` | no | A date. An active rule carries one |
| `zero_by` | no | A date, optional, for a planned rule |
| `relations` | no | How the rule stands to other rules, at least one of the four |
| `relations.relates` | no | Ids of related rules |
| `relations.conflicts` | no | Ids of rules it conflicts with |
| `relations.depends` | no | Ids of rules it depends on |
| `relations.covers` | no | Ids of rules it covers |
| `origin` | yes | `authored`, `imported` or `migrated` |
| `sources` | no | What the rule cites. A rule leaves proposed only with at least one |
| `sources[].ref` | yes | A slug in [sources](sources.md) |
| `sources[].where` | no | The place inside the source |
| `sources[].says` | no | What the source says there, at most 40 words |
| `sources[].backs` | yes | What the source backs: `rule`, `threshold` or `check` |
| `bindings` | no | The checks that hold the rule, one per tool entry |
| `bindings[].tool` | yes | The tool's slug |
| `bindings[].target` | yes | The target type the tool is run on |
| `bindings[].key` | yes | The tool's entry. At rung 6 it is the review question |
| `bindings[].value` | no | The entry's value. At rung 6, when the question applies. It is any YAML value |
| `bindings[].options` | no | A mapping of the entry's options |
| `bindings[].severity` | yes | `error`, `warning` or `info` |
| `bindings[].rung` | yes | A whole number from 0 to 6: 0 an upstream preset, 1 a configured built-in, 2 a pattern, 3 an own plugin or analyzer, 4 a fact held by a policy tool, 5 a test or kit, 6 a review |
| `bindings[].run_point` | yes | When it runs: `edit`, `commit`, `push`, `land`, `review`, `release` or `deploy` |
| `bindings[].remedy` | no | The tool's fixer, named and never run |
| `bindings[].pair` | no | The number of the pair that proves the binding |
| `bindings[].pair_path` | no | The path the pair stands at in the fixture it is run in |
| `bindings[].tightens` | no | In an overlay or an importer, the id of the parent's or imported rule this binding tightens |
| `bindings[].checker` | no | The name of the rulebook's checker-code declaration, `checkers/<name>/`, the binding's entry comes from: the rulebook's own plugin or analyzer, or an import's that the rulebook declares ([checkers](checkers.md)) |
| `replaced_by` | no | The id of the rule that replaced this one, on a retired rule |

Authoring guidance, not held by the schema (a statement is a text of at least one character): S24 asks for a statement that is one sentence in the imperative, within the rulebook's `budgets.statement_words`, that uses defined terms as defined, and that names no tool, organisation or project except through the rulebook's own terms and no "we". The prose lint that holds this is designed.

## Target types

A `target` is one of `source`, `config`, `migration`, `container`, `workflow`, `iac`, `api-spec`, `prose`, `commit`, `telemetry-registry`, `dependency` or `package`.

## What a rule needs to leave proposed (S24, designed)

The schema takes a rule with only `id`, `statement`, `strength`, `status` and `origin`. The rest is what S24 asks of a promotion, not of the file's shape. It is designed: `gragas validate` does not yet refuse a promotion for it, and the list says what it will refuse:

- Every rule needs at least one source before it leaves proposed. The owner's own recorded decision counts as a source of kind `own-decision`.
- A `must` or `should` becomes trial or active only with a source, a decision and an example pair. A `may` becomes active with a source and a decision, and needs a pair once it is bound. Without them the rule stays proposed and validation will refuse the promotion.
- One exception: a grandfathered import enters active with its pair on the attention list.
- A `must` or `should` enters as trial. A trial lasts at least one release and at most `governance.trial_releases`.
- Every active rule carries a `review_by` date. An overdue date will be a finding.

## Standing and marks

Standing is never written. It is derived from `status`, the bindings and their proof: `proposed` and `retired` mirror the status, and `enforced`, `reviewed`, `planned` and `advisory` come from the effective bindings. Three marks sit beside a standing and never replace it: trial, blocked and unproven. S32 gives the whole table; the fixture's [README](../../internal/host/assets/fixture/README.md) lists the input of each.

## Examples

A concern file of the base fixture, with a rule of each kind of binding:

```yaml rules.schema.json
format: 3
part: go
concern: layout
rules:
  - id: go.layout.1
    statement: Keep the program under cmd and everything else under internal.
    strength: must
    status: active
    owner: alex
    review_by: "2027-03-01"
    origin: authored
    sources:
      - {ref: house-call, backs: rule}
    bindings:
      - tool: review
        target: prose
        key: layout-reviewed
        value: "a person looks"
        severity: warning
        rung: 6
        run_point: review
        pair: 1
        pair_path: README.md
    pairs:
      - {number: 1, target: prose}
  - id: go.layout.2
    statement: Put every package in one folder of its own.
    strength: must
    status: active
    owner: alex
    review_by: "2027-03-01"
    origin: authored
    bindings:
      - tool: golangci-lint
        target: config
        key: wrapcheck
        value: false
        severity: error
        rung: 2
        run_point: push
      - tool: golangci-lint
        target: dependency
        key: depguard
        value: {list-mode: lax}
        severity: warning
        rung: 4
        run_point: deploy
  - id: go.layout.3
    statement: Keep a container image to one program.
    strength: should
    status: active
    owner: alex
    review_by: "2027-03-01"
    origin: authored
    bindings:
      - tool: container-lint
        target: container
        key: one-process
        severity: info
        rung: 5
        run_point: release
```

The block of a guide, from the web guide of the base fixture (the guide itself is `internal/host/assets/fixture/base/language/web/guide.md`; the block is the YAML inside its `gragas` fence):

```yaml block.schema.json
anchor: forms
rules:
  - id: web.forms.1
    statement: Give every form field a label.
    strength: must
    status: active
    owner: alex
    review_by: "2027-03-01"
    origin: authored
    sources:
      - {ref: house-call, backs: rule}
    bindings:
      - tool: stylelint
        target: source
        key: field-label
        value: true
        severity: error
        rung: 2
        run_point: commit
        pair: 1
        pair_path: src/form.html
    pairs:
      - {number: 1, target: source}
  - id: web.forms.2
    statement: Say what a field is for in its label.
    strength: should
    status: active
    owner: alex
    review_by: "2027-03-01"
    origin: authored
```

## A rebind record

An overlay that changes a binding of its parent's rule writes a record under `bindings/`, named for the rule. The record is designed: no schema takes it yet (M3), so this page gives the one the base fixture has, in a `text` fence, and the fixture is the conformance suite for it until its schema lands. The record's fields are `subject` (the id of the parent's rule), `tool`, `key`, `value` and `severity`, the entry as the overlay sets it. It may only tighten, as a binding's `tightens` field says. The file is `internal/host/assets/fixture/base/bindings/std-errors-1.yaml`:

```text
subject: std.errors.1
tool: golangci-lint
key: wrapcheck
value: true
severity: error
```
