---
type: spec
status: current
as-of: 2026-11-09
---

# The rulebook format, version 3

This is the format spec of S25: the files of a rulebook, written in prose beside the schemas that hold them and the fixture rulebook that exercises them. A second implementation needs this folder, the schemas and the fixture, and nothing of the engine's code. Where the engine does something the prose and the fixture do not say, that is a gap in this spec and is added here.

The files of a rulebook are listed in [section 7.1 of the architecture](../architecture.md#71-a-rulebooks-files-the-format-spec). This folder has one page per file kind, and each page keeps one section per schema, so a later milestone adds rows to a table and never rewrites a page.

## What is built and what is only designed

| Part of the format | State | Where |
|---|---|---|
| The v3 schemas of identity, structure, terms, rule files, the Markdown block, sources, decisions, exclusions, translations, history and proposals | built (M1-01) | `internal/host/assets/format/schema/v3/` |
| The fixture rulebooks, the conformance suite | built (M1-02) | `internal/host/assets/fixture/` |
| These pages and the checks that hold them to the schemas | built (M1-22) | this folder |
| The overlay's rebind records under `bindings/` | designed, M3: no schema takes it yet | [rules](rules.md) shows the one record the fixture has |
| The adapter and docs-profile overrides under `adapters/` and `profiles/` | designed, M3 and M6: the adapter schema exists and is not described here yet | [section 4.1 of the architecture](../architecture.md) |
| the preset, checker, proof and release folders | designed, M3 to M5: files the engine writes, not files an author writes | [section 7.1 of the architecture](../architecture.md#71-a-rulebooks-files-the-format-spec) |
| `.gragas/state.yaml` | designed: local state, kept out of version control | [section 7.1 of the architecture](../architecture.md#71-a-rulebooks-files-the-format-spec) |
| `migrate`, which moves a rulebook from one format version to the next | designed, M6 | [section 7.1 of the architecture](../architecture.md#71-a-rulebooks-files-the-format-spec) |

A page says nothing about a file that is only designed beyond the row above. The prose grows with every milestone.

## The pages

| Page | File kind |
|---|---|
| [identity](identity.md) | `rulebook.yaml`: name, version, kind, licences, language, extends, imports, owners, governance, the tested set, budgets |
| [structure](structure.md) | `structure.yaml`: layers, parts, concerns, kinds |
| [rules](rules.md) | a concern file of a part, the rule, and the fenced block of a guide |
| [terms](terms.md) | `terms.yaml`: the words a rulebook defines once, and how a statement uses them |
| [sources](sources.md) | `sources.yaml`: what the rules cite |
| [decisions](decisions.md) | `decisions.yaml`: what was decided and why |
| [exclusions](exclusions.md) | `exclusions.yaml`: the parent rules an overlay drops |
| [translations](translations.md) | `translations/<language>/<name>.yaml`: text in another language |
| [history](history.md) | `history.jsonl`: the append-only history |
| [proposals](proposals.md) | `proposals/<slug>.yaml`: a proposal an overlay carries to a parent |
| [checkers](checkers.md) | `checkers/<name>/checker.yaml`: the declaration of an artifact of checker code, by name and publisher |

## How a page reads

Every file but a guide opens with `format: 3`. A page gives, for each schema it covers, the line `Schema:` with a link to the schema file, then one table. The table has a row for every field the schema declares and for no other. A field is written as the path to it: `a.b` is the field `b` of the object `a`, and `a[].b` is the field `b` of each item of the list `a`. The Required column says **yes** when the schema requires the field in its object, and says when an absent field means something. A field that a page does not name is not in the format, and a file that carries one is refused: every object of every schema is closed.

Each page ends its sections with an example. An example is a whole document that its schema accepts, taken from the fixture rulebook `fixture-base` or `fixture-other` (every name in the fixture is a stand-in). A fence marked `text` is not a document. The checks that hold this folder are in `internal/repo/docs/format_docs_test.go`: every schema field has a row, every row names a field, the Required column agrees with the schema, and every fenced example is accepted by the schema it names.

## Shapes the schemas share

These shapes appear in many files, and a page uses them by name.

| Shape | What it is |
|---|---|
| slug | lowercase letters and digits in words joined by a hyphen, such as `go-errors` |
| part | lowercase letters only, the first word of a rule id |
| id | `part.concern.number`: a part, a concern slug and a number from 1, frozen at birth and never reused |
| text | a string of at least one character |
| date | `yyyy-mm-dd` |
| timestamp | UTC to the second, `yyyy-mm-ddThh:mm:ssZ` |
| semver | `major.minor.patch`, three numbers with no leading zero |
| digest | `sha256:` and 64 lowercase hexadecimal characters |
| language | a language tag such as `en` or `fr` |
| url | an `https://` address with no space |

## Versions

The format has its own version, apart from the engine's. The engine reads the current and the previous major version and migrates forward (S25); a file that names a newer version stops the reader, and the message names both.

| Version | Shape | State |
|---|---|---|
| 2 | One repository holds one rulebook as it was first written: `rulebook/parts.yaml` for layers and parts, a rules file per part, `sources.yaml` and `tools.yaml` beside it, `rulebook/ids.lock` for the ids ever issued, and generated files (`rulebook/rules.json`, the part READMEs) committed with their source. A rule had a strength of Hard, Answered, Lens, Record, Review or Session, a status that could be `later`, and checks with a class and a tier. See [the record of the move to v2](../schema-v2.md) | read by the old tooling; the owner's rulebook is v2 until M6 |
| 3 | One folder per rulebook with a file per kind, each opening `format: 3`: identity, structure, terms, a rules file per concern, sources, decisions, exclusions, translations and a history. Strength is must, should or may; status is proposed, trial, active or retired, and standing is derived and never written; a rule's checks are bindings with a tool, a key and a rung; ids are frozen and a renamed id is an alias; history is append-only and chained by digest | this spec |

What moves between them, by file:

| v2 | v3 | Note |
|---|---|---|
| `rulebook/parts.yaml` | `structure.yaml` | layers, parts and concerns, with aliases for what was renamed; product kinds become kinds |
| a rules file of a part | `<layer>/<part>/rules/<concern>.yaml` | one concern per file; the Markdown form is a guide with fenced blocks |
| `sources.yaml` | `sources.yaml` | source kinds are renamed to the seven of [sources](sources.md); a locator is one text |
| `rulebook/ids.lock` | the ids in the rules and their aliases | an id is frozen at birth and never reused |
| a tool's `checks` | a rule's `bindings` | a binding names its tool, target, key, severity, rung and run point |
| none | `terms.yaml`, `decisions.yaml`, `exclusions.yaml`, the translations folder, `history.jsonl` | new in v3 |

The move is a plan the engine makes (`migrate`, designed, M6). The table says what moves and is not the algorithm.

## The conformance suite

The fixture rulebooks under `internal/host/assets/fixture/` are the suite; its [README](../../internal/host/assets/fixture/README.md) lists what each folder exercises. The base fixture uses every field of the rulebook files, four statuses, both source forms and a translation. A second implementation reads each fixture root as a rulebook and agrees with the schemas on every file.

## Changing the format

A change to the format arrives as a proposal under the [template](../process/proposal-template.md): motivation, change, compatibility, migration.
