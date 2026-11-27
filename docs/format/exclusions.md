---
type: spec
status: current
as-of: 2026-11-09
---

# Exclusions: exclusions.yaml

An overlay may drop a rule of its parent. Each drop is an exclusion: it names the rule, the decision that allows it, and who did it and when. An exclusion is lifted by a later decision, and the lifted record stays, marked lifted (S4). The index of [the format](README.md) says how to read a table.

Schema: [exclusions.schema.json](../../internal/host/assets/format/schema/v3/exclusions.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here |
| `exclusions` | yes | The exclusions. May be empty |
| `exclusions[].overlay` | yes | The overlay's name |
| `exclusions[].rule` | yes | The parent's rule id |
| `exclusions[].number` | yes | The sequence number, from 1: a second exclusion of the same rule gets the next one |
| `exclusions[].decision` | yes | The slug of the decision that allows the exclusion, in [decisions](decisions.md) |
| `exclusions[].who` | yes | Who excluded the rule |
| `exclusions[].when` | yes | When, as a timestamp |
| `exclusions[].lifted` | no | Present when the exclusion was lifted. The record stays |
| `exclusions[].lifted.decision` | yes | The decision that lifted it |
| `exclusions[].lifted.who` | yes | Who lifted it |
| `exclusions[].lifted.when` | yes | When, as a timestamp |

## Rules the fields come with

- An exclusion is keyed by the overlay, the parent's rule id and the sequence number.
- Lifting writes `lifted` on the record and never removes the record. A rule excluded again takes the next sequence number.

## Example

```yaml exclusions.schema.json
format: 3
exclusions:
  - overlay: fixture-base
    rule: std.errors.2
    number: 1
    decision: house-call
    who: alex
    when: "2026-10-21T09:00:00Z"
  - overlay: fixture-base
    rule: std.errors.3
    number: 1
    decision: house-call
    who: alex
    when: "2026-10-22T09:00:00Z"
    lifted:
      decision: wrap-errors-again
      who: alex
      when: "2026-11-02T09:00:00Z"
```
