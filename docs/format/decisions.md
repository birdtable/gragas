---
type: spec
status: current
as-of: 2026-11-09
---

# Decisions: decisions.yaml

A decision records what was decided and why, with the options that lost. A rule's reasoning points at the decision that settles it, a promotion that needs a decision names one, and the owner's own call is a source of kind `own-decision` whose locator is a decision's slug. The index of [the format](README.md) says how to read a table.

Schema: [decisions.schema.json](../../internal/host/assets/format/schema/v3/decisions.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here |
| `decisions` | yes | The decisions. May be empty |
| `decisions[].slug` | yes | The decision's slug. A rule's `reasoning.decision`, a history entry and an exclusion refer to it |
| `decisions[].title` | yes | A short title |
| `decisions[].text` | yes | What was decided and why |
| `decisions[].rejected` | no | The options that lost, at least one, each in a sentence |
| `decisions[].cites` | no | What the decision rests on, at least one of the three lists |
| `decisions[].cites.sources` | no | Source slugs from [sources](sources.md) |
| `decisions[].cites.rules` | no | Rule ids |
| `decisions[].cites.bindings` | no | Bindings, each written `<rule id> <tool> <key>` |
| `decisions[].who` | yes | Who decided |
| `decisions[].date` | yes | The day it was decided, as a date |
| `decisions[].supersedes` | no | The slug of the decision this one replaces |
| `decisions[].superseded_by` | no | The slug of the decision that replaced this one |

## Rules the fields come with

- Nothing is deleted. A decision that no longer holds is superseded by a new one, and the base fixture writes both sides: the old decision names its successor in `superseded_by` and the new one names its predecessor in `supersedes`.
- A decision log keeps the rejected options (S25), so a later reader does not propose them again.

## Example

```yaml decisions.schema.json
format: 3
decisions:
  - slug: wrap-errors
    title: Wrap every error
    text: Wrapping keeps the cause and adds the step that failed.
    rejected: [return errors bare, log and return]
    cites:
      sources: [effective-go, error-study]
      rules: [go.errors.1]
      bindings: ["go.errors.1 golangci-lint wrapcheck"]
    who: alex
    date: "2026-09-02"
    superseded_by: wrap-errors-again
  - slug: wrap-errors-again
    title: Wrap every error, with the step named
    text: The step is named in the message, not only the call.
    cites:
      rules: [go.errors.1]
    who: alex
    date: "2026-10-03"
    supersedes: wrap-errors
  - slug: house-call
    title: Forms are labelled
    text: Every form field has a label the reader can see.
    who: alex
    date: "2026-10-20"
```
