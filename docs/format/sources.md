---
type: spec
status: current
as-of: 2026-11-09
---

# Sources: sources.yaml

A source is something a rule cites: a standard, a guide, a study, an incident, a decision record, a vendor's document, or the owner's own decision. A rule without a source is an opinion, so a rule leaves proposed only with one (S24). The index of [the format](README.md) says how to read a table.

Schema: [sources.schema.json](../../internal/host/assets/format/schema/v3/sources.schema.json)

| Field | Required | Meaning |
|---|---|---|
| `format` | yes | The format version, always 3 here |
| `sources` | yes | The sources. May be empty |
| `sources[].slug` | yes | The source's slug. A rule cites it by this, in `sources[].ref` |
| `sources[].kind` | yes | One of `standard`, `guide`, `study`, `incident`, `decision-record`, `vendor-document`, `own-decision`. The owner's own recorded decision counts as a source of kind `own-decision` |
| `sources[].title` | yes | The title a reader looks it up by |
| `sources[].locator` | yes | Where to find it: a URL, an ISBN, a DOI, an RFC, or a decision slug |
| `sources[].attribution` | no | Who wrote it |
| `sources[].verified` | no | The day someone opened it, as a date |

## Rules the fields come with

- A rule cites a source by its slug; the place inside the source, and what it says there in at most forty words, are written on the rule's own `sources` entry, not here (see [rules](rules.md)).
- A source of kind `own-decision` has a decision slug for its locator, and the decision is in [decisions](decisions.md).

## Example

```yaml sources.schema.json
format: 3
sources:
  - {slug: effective-go, kind: guide, title: Effective Go, locator: "https://example.com/effective-go", attribution: alex, verified: "2026-09-01"}
  - {slug: rfc-0001, kind: standard, title: A standard of errors, locator: "RFC 0001"}
  - {slug: error-study, kind: study, title: A study of error messages, locator: "doi:10.0000/fixture.1", attribution: alex}
  - {slug: outage-note, kind: incident, title: The outage with no cause in its error, locator: "https://example.org/outage-1", verified: "2026-09-15"}
  - {slug: wrap-record, kind: decision-record, title: The record of wrapping, locator: wrap-errors}
  - {slug: tool-manual, kind: vendor-document, title: The linter manual, locator: "https://example.com/linter/manual"}
  - {slug: house-call, kind: own-decision, title: A call the owners made, locator: house-call}
```
