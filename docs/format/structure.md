---
type: spec
status: current
as-of: 2026-11-09
---

# Structure: structure.yaml

The structure names the layers a rulebook is cut into, the parts of each layer, the concerns of each part, and the kinds of project the rules are switched by. A rule's id, its folder and its scope all come from it. There is one at the root of a rulebook. The index of [the format](README.md) says how to read a table.

Schema: [structure.schema.json](../../internal/host/assets/format/schema/v3/structure.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here |
| `layers` | yes | The layers, at least one, in the order a reader sees them. The order of each list in this file is the order of the layers, parts and concerns |
| `layers[].slug` | yes | The layer's slug, and its folder name |
| `layers[].name` | yes | The layer's name for a reader |
| `layers[].aliases` | no | Slugs the layer had before it was renamed. A reference to an alias still resolves |
| `layers[].parts` | yes | The parts of the layer, at least one |
| `layers[].parts[].slug` | yes | The part's slug: lowercase letters only, the first word of every rule id in it, and the folder under the layer. Never `gragas`, which is reserved |
| `layers[].parts[].name` | yes | The part's name for a reader |
| `layers[].parts[].aliases` | no | Part slugs it had before it was renamed |
| `layers[].parts[].owners` | no | Who owns the part, at least one. |
| `layers[].parts[].concerns` | yes | The concerns of the part. May be empty when the part holds no rule yet |
| `layers[].parts[].concerns[].slug` | yes | The concern's slug: the middle word of a rule id, and the name of its rules file |
| `layers[].parts[].concerns[].name` | yes | The concern's name for a reader |
| `layers[].parts[].concerns[].aliases` | no | Concern slugs it had before it was renamed |
| `kinds` | yes | The kinds of project a rule's scope and exemptions may name. May be empty |
| `kinds[].slug` | yes | The kind's slug. A trait a rulebook uses to switch rules off, such as having no database, is a kind like any other (S73) |
| `kinds[].name` | yes | The kind's name for a reader |
| `kinds[].description` | no | One sentence on what the kind is |

## Rules the fields come with

- A rule id is `part.concern.number`. The part and the concern of an id are slugs this file declares, and the rule lives in `<layer>/<part>/rules/<concern>.yaml`.
- Renaming a slug never changes an id a rule already has: the old slug is written in `aliases`, and the rule keeps its id (see [rules](rules.md)).
- A slug names one thing. A layer slug, a part slug and a kind slug is each written once in the file, and a part's alias never takes a part slug in use. A concern slug is written once within its part (two parts may each have an `errors`).
- The part name `gragas` is reserved, as a slug and as an alias. It is the first word of the engine's own check ids, such as `gragas.validate.1`, which SARIF lists beside a rulebook's rules. A part of that name could hold a rule whose id is the engine check's, and a reader keying on the rule id would merge the two. The loader reports the word as a finding at its line and never renames the part. Since a rule's id is not held to its part, the loader also refuses a rule whose id or alias starts with `gragas`, whatever its status, as a finding at the rule's line, and leaves the rule out of the book. This narrows what format 3 holds: a rulebook that names the part renames it, and the old slug cannot stay on as an alias, since an alias is held to the same word. The rulebook's own name is not reserved (see [identity](identity.md)).

## Example

```yaml structure.schema.json
format: 3
layers:
  - slug: core
    name: Core
    aliases: [foundation]
    parts:
      - slug: base
        name: Base
        aliases: [root]
        owners: [alex]
        concerns:
          - slug: errors
            name: Errors
            aliases: [faults]
          - slug: naming
            name: Naming
          - slug: targets
            name: Targets
  - slug: language
    name: Language
    parts:
      - slug: go
        name: Go
        owners: [alex]
        concerns:
          - slug: errors
            name: Errors
          - slug: layout
            name: Layout
      - slug: web
        name: Web
        concerns:
          - slug: forms
            name: Forms
kinds:
  - slug: service
    name: Service
    description: A program that runs until it is stopped.
  - slug: library
    name: Library
    description: Code other programs import.
```
