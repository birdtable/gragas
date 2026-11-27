---
type: spec
status: current
as-of: 2026-11-09
---

# Identity: rulebook.yaml

The identity says what a rulebook is: its name, its version, its kind, who owns it, how it is governed, what it extends and imports, and the set it was tested with. There is one at the root of a rulebook. The index of [the format](README.md) says how to read a table.

Schema: [identity.schema.json](../../internal/host/assets/format/schema/v3/identity.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here. A reader that finds an older one migrates it forward; one that finds a newer one stops and names both |
| `name` | yes | The rulebook's slug. Its releases, pins and the overlay field of an exclusion refer to it |
| `version` | yes | A semver, declared once at init for the first release. Every later release computes its own |
| `kind` | yes | One of `base`, `overlay`, `companion`, `fork`, `archived`. A base stands alone; an overlay extends a parent at a pin; a companion is a base that cites a guide its owner does not control, by anchor, and never copies it; a fork records its origin; an archived rulebook is frozen, with a successor and a rule map |
| `licences` | yes | The licence of each thing the rulebook ships |
| `licences.rules` | yes | An SPDX expression for the rules and their prose |
| `licences.checker_code` | no | An SPDX expression for the hand-written checkers. Absent when the rulebook ships none |
| `licences.presets` | no | An SPDX expression for the generated presets |
| `language` | yes | The authoritative language. A translation names its own, and a reader with none gets this language's text |
| `docs_url` | no | The base URL of the published pages |
| `explorer_url` | no | The base URL of an explorer the owner hosts |
| `extends` | no | The parent of an overlay, at a pin. Absent in a base |
| `extends.rulebook` | yes | The parent's short name or a full location |
| `extends.version` | yes | The pin: the parent's version |
| `extends.source` | no | Where to fetch the parent from |
| `extends.content_hash` | no | The content hash of the pinned parent release, `sha256:` and 64 lower-case hexadecimal digits. You give it: `gragas init` asks for it (`--parent-hash`) and refuses an overlay without one. A fetched release that does not hash to it is refused. Absent only until that check is built |
| `imports` | no | Parts taken from other rulebooks, each at a pin. Absent when nothing is imported |
| `imports[].rulebook` | yes | The imported rulebook |
| `imports[].version` | yes | The pin |
| `imports[].source` | no | Where to fetch it from |
| `imports[].content_hash` | no | The content hash of the pinned release, as `extends.content_hash` is. `gragas init` takes it with each `--import`; an import you add later is an edit of this file with its hash written in (S69) |
| `imports[].parts` | yes | The part slugs taken, at least one |
| `owners` | yes | The people or teams that own the rulebook, at least one. |
| `governance` | yes | How the rulebook takes change |
| `governance.contribution` | yes | `open`, `proposals` or `closed`: who may propose a change |
| `governance.checker_publishers` | no | Who may publish checker code |
| `governance.checker_second_reviewer` | no | Whether checker code needs a second reviewer |
| `governance.checker_proof` | no | `isolated` or `sandboxed`: the proof level checker code from a third party must reach |
| `governance.decision_required` | yes | Whether a promotion to trial or active needs a recorded decision |
| `governance.trial_releases` | yes | The longest a trial lasts, in releases, at least 1 |
| `governance.review_days` | yes | The default cadence of a rule's `review_by`, in days, at least 1 |
| `tested` | no | The set a release was proven with. Proposed by the proof and fixed at the release's yes |
| `tested.tools` | no | The tools and the version of each |
| `tested.tools[].name` | yes | A tool's slug |
| `tested.tools[].version` | yes | The version, as the tool prints it |
| `tested.plugins` | no | The plugins and the version of each |
| `tested.plugins[].name` | yes | A plugin's slug |
| `tested.plugins[].version` | yes | The version |
| `tested.kits` | no | The test kits and the version of each |
| `tested.kits[].name` | yes | A kit's slug |
| `tested.kits[].version` | yes | The version |
| `tested.adapters` | no | The plugins and the version of each (the field keeps the name `adapters` in release 1, gragas#2071) |
| `tested.adapters[].name` | yes | A plugin's slug |
| `tested.adapters[].version` | yes | The version |
| `tested.min_engine` | yes | The oldest engine, as a semver, that reads the rulebook |
| `budgets` | no | Word limits the prose lint holds the rules to |
| `budgets.statement_words` | no | The most words in a statement |
| `budgets.reasoning_words` | no | The most words in a rule's reasoning |
| `hand_written_presets` | no | The tools whose presets the owners write by hand, by slug, each once |

## Rules the fields come with

- `extends` is for an overlay, and an imported rule or a parent's rule can only be tightened by a binding that names it in `tightens` (see [rules](rules.md)).
- An `imports` entry names parts by slug; ids stay namespaced by the prefix of the rulebook that owns them.
- A rulebook may be named `gragas`. That word is reserved only as a part name (see [structure](structure.md)): an imported id is namespaced as `name.part.concern.number`, four words, and an engine check's id, such as `gragas.validate.1`, has three, so the two never meet.
- `budgets` sets the limits of the prose lint (S24): a statement is one sentence in the imperative within `statement_words`.
- `hand_written_presets` lists the tools whose presets the owners write by hand (gragas#2071). No plugin is asked to rank a value of a listed tool, and the conflicts listing does not end not ready for want of one; two values of it that differ as text are a conflict. A tool a rule binds that no installed plugin serves and the list does not name still ends it not ready, so the list is never the default. A listed tool no active or trial rule binds is a finding, and so is a listed tool an installed plugin serves.

## Example

The base fixture is an overlay of a stand-in parent, with one import:

```yaml identity.schema.json
format: 3
name: fixture-base
version: 1.2.0
kind: overlay
licences:
  rules: CC-BY-4.0
  checker_code: Apache-2.0
  presets: MIT
language: en
docs_url: https://example.com/fixture-base/
explorer_url: https://example.org/fixture-base/explorer/
extends:
  rulebook: fixture-parent
  version: 0.9.0
  source: https://example.com/fixture-parent.git
  content_hash: sha256:ab78351089603a19341bd2b875a5f7348d516b1d60335727c5e54c1e62131640
imports:
  - rulebook: fixture-other
    version: 0.3.0
    source: https://example.org/fixture-other.git
    content_hash: sha256:b77b6a5859c584ff1f1acf5fc4f59e3d19d9a213e8bcf5a944a130fb0b3fa6c2
    parts: [ui]
owners: [alex]
governance:
  contribution: proposals
  checker_publishers: [alex]
  checker_second_reviewer: true
  checker_proof: sandboxed
  decision_required: true
  trial_releases: 2
  review_days: 180
tested:
  tools:
    - {name: golangci-lint, version: 2.1.0}
    - {name: stylelint, version: 16.0.0}
  plugins:
    - {name: wrapcheck-plugin, version: 1.4.0}
  kits:
    - {name: fixture-kit, version: 0.2.0}
  adapters:
    - {name: golangci-lint, version: 1.0.0}
  min_engine: 0.1.0
budgets:
  statement_words: 30
  reasoning_words: 80
hand_written_presets: [sqlfluff]
```
