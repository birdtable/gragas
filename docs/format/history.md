---
type: spec
status: current
as-of: 2026-11-09
---

# History: history.jsonl

The history is the append-only record of every change to a rule's lifecycle. Every such change is one line, written by the engine with who, when, the digest of the plan the yes accepted and, where one is required, the decision. Nothing is deleted: retire, restore, supersede and undo add lines. The index of [the format](README.md) says how to read a table.

The file holds one JSON object per line, and each line is a document of the schema below.

Schema: [history.schema.json](../../internal/host/assets/format/schema/v3/history.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here |
| `number` | yes | The sequence number, from 1. Two branches may each append the next one, and that is not a finding |
| `previous` | no | The digest of the entry before it on its branch. Required from number 2 on and absent on the first entry |
| `kind` | yes | What the command did, as a slug, such as `init`, `promote` or `retire` |
| `entity` | yes | The entity kind and its key, such as `rule:store.schemas.2` |
| `before` | no | The lifecycle fields before the change. Absent when the entity is new |
| `before.status` | no | The status before, one of `proposed`, `trial`, `active`, `retired` |
| `before.owner` | no | The owner before |
| `before.review_by` | no | The review date before |
| `before.zero_by` | no | The zero-by date before |
| `before.replaced_by` | no | The replacing rule's id before |
| `before.literal` | no | A term's literal before |
| `after` | yes | The lifecycle fields after the change |
| `after.status` | no | The status after |
| `after.owner` | no | The owner after |
| `after.review_by` | no | The review date after |
| `after.zero_by` | no | The zero-by date after |
| `after.replaced_by` | no | The replacing rule's id after |
| `after.literal` | no | A term's literal after |
| `who` | yes | Who made the change |
| `when` | yes | When, as a timestamp |
| `plan` | yes | The digest of the plan the yes accepted |
| `decision` | no | The slug of the decision the entry records, when the command takes one |
| `command` | no | The command line the yes agreed to, every answer as a flag and never `--apply`, so `guard staged` can make the plan again from the line |
| `reverts` | no | The digest of the entry an undo takes back, as `previous` names one; written on undo entries only |
| `statements` | no | The digest of each rule's statement as the conversion wrote it, by rule id; written on migrate entries only, and an empty object when the rulebook held no rule. The prose lint holds a migrated rule whose statement still has its recorded digest |

## Rules the fields come with

- The lifecycle fields are the ones a history entry guards: `status`, `owner`, `review_by`, `zero_by` and `replaced_by` of a rule, and `status` and `literal` of a term. A change to one of them is made only with its entry, and the object holds at least one.
- The lines form a chain. Each line after the first names the canon digest of the line before it in `previous`, so a line that is edited or removed breaks every line after it.
- The first line of a file has number 1 and no `previous`. The number of a new line is the highest number in the file plus one.
- The same file, entry and yes give the same bytes.

## Example

The first two lines of the base fixture's history, each a document of the schema:

```json history.schema.json
{"after":{"status":"proposed"},"entity":"rule:go.errors.1","format":3,"kind":"init","number":1,"plan":"sha256:6988e53c48a5ec3528cfdf8d4676191340f7f07e0855d2e3fb31fd6c66f15a71","when":"2026-09-01T09:00:00Z","who":"alex"}
```

The second line names the digest of the first:

```json history.schema.json
{"after":{"status":"trial"},"before":{"status":"proposed"},"decision":"wrap-errors","entity":"rule:go.errors.1","format":3,"kind":"promote","number":2,"plan":"sha256:04a75b82a19181e21969b952efa3a5be22f4047dbfb6efc5cf90144b4c12a358","previous":"sha256:532d1b0dc3b7b87958619a30f0548280f6f77c6d12cd2442a561827a5d43edc9","when":"2026-09-03T09:00:00Z","who":"alex"}
```
