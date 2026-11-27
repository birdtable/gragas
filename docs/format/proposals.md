---
type: spec
status: current
as-of: 2026-11-09
---

# Proposals: the proposals folder

A proposal is one change an overlay carries to its parent, kept as a file of the overlay so the overlay's own history shows what it asked and what came of it. The proposal's content is a contract document, and its shape belongs to the contract, not to the format. The index of [the format](README.md) says how to read a table.

Schema: [proposals.schema.json](../../internal/host/assets/format/schema/v3/proposals.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here |
| `slug` | yes | The proposal's slug, equal to its file name |
| `target` | yes | The rulebook whose prefix owns the rule, or the parent |
| `state` | yes | One of `open`, `accepted`, `declined`, `withdrawn` |
| `link` | no | The pull request or issue that carries it, as an `https://` address |
| `opened` | yes | The day it was opened, as a date |
| `document` | yes | The proposal as a contract document. This spec does not describe its fields: the contract's schema does (designed, M1-16 and the contract page at M10) |

## Example

```yaml proposals.schema.json
format: 3
slug: tighten-open
target: fixture-parent
state: open
link: https://example.com/fixture-parent/pull/1
opened: "2026-10-11"
document:
  kind: rule-change
  rule: std.errors.1
  change: A wrapped error names its step.
```
