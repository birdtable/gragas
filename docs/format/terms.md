---
type: spec
status: current
as-of: 2026-11-09
---

# Terms: terms.yaml

A term is a word the rulebook defines once, so a statement can use it and mean one thing. Another language's definition is a translation, never a field here. The index of [the format](README.md) says how to read a table.

Schema: [terms.schema.json](../../internal/host/assets/format/schema/v3/terms.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here |
| `terms` | yes | The terms. May be empty |
| `terms[].slug` | yes | The term's slug, the name a statement uses it by |
| `terms[].definition` | yes | What the term means, in the rulebook's authoritative language |
| `terms[].literal` | no | The name the term stands for in the world, such as a header name or a package, so a statement can stay neutral. The literal is the one name a rulebook may use bare |
| `terms[].status` | no | `active` or `retired`; absent is active. A retired term stays in the file, no statement may use it, and its literal allows no bare name. `gragas term retire` sets it, and refuses while a rule uses the term |

## How a statement uses a term

A statement uses a term by writing its slug in braces, `{slug}`. A slug that terms.yaml does not define is a finding (the undefined-term check). A term's literal written bare is the one name a rulebook may use, so the names check takes literals and `{slug}` references out of the text before it looks. The names check reads every line of every file the load reads, comments included, and each file's path, not the rule text alone. A finding says the file and line, and never the name it found (gragas#1741).

## Example

```yaml terms.schema.json
format: 3
terms:
  - slug: gate
    definition: A check that stops a push when it fails.
    literal: task pre-push
  - slug: wrap
    definition: Add what was being done to an error before it is returned.
  - slug: stale-word
    definition: A word no statement uses any more.
    status: retired
```
