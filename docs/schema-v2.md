---
type: record
status: current
as-of: 2026-11-06
---

# Rulebook schema v2

The rulebook moved from schema v1 to schema v2 in milestone M3, following
R-12 section 2 and decisions 42 to 66. This file records what the move
counted, what it could not map honestly, and what stays open. The loader,
the validator and the generators read v2 only.

## Counts before and after

Total: 1003 rules before, 1003 after. No id was added, removed or renamed
(decision 42); `rulebook/ids.lock` lists every id issued.

| Part | Before | After |
|---|---|---|
| agents | 41 | 41 |
| arch | 70 | 70 |
| astro | 4 | 4 |
| aws | 5 | 5 |
| cli | 86 | 86 |
| code | 30 | 30 |
| daemon | 40 | 40 |
| docs | 27 | 27 |
| dynamodb | 10 | 10 |
| gates | 111 | 111 |
| go | 27 | 27 |
| lambda | 4 | 4 |
| postgres | 25 | 25 |
| react | 34 | 34 |
| service | 96 | 96 |
| sst | 8 | 8 |
| store | 26 | 26 |
| supply | 72 | 72 |
| tests | 72 | 72 |
| ts | 45 | 45 |
| web | 170 | 170 |

| Status | Before | After |
|---|---|---|
| active | 984 | 595 |
| later | 12 | 10 |
| proposed | 0 | 391 |
| retired | 7 | 7 |

| Strength | Before | After |
|---|---|---|
| Hard | 869 | 881 |
| Later | 12 | 0 |
| Answered | 26 | 26 |
| Lens | 25 | 25 |
| Record | 15 | 15 |
| Review | 48 | 48 |
| Session | 8 | 8 |

Every change in status and strength, v1 to v2:

| v1 strength, status | v2 strength, status | Rules |
|---|---|---|
| Hard active | Hard active | 523 |
| Hard active | Hard proposed | 339 |
| Hard retired | Hard retired | 7 |
| Later later | Hard later | 10 |
| Later later | Hard proposed | 2 |
| Answered active | Answered active | 20 |
| Answered active | Answered proposed | 6 |
| Lens active | Lens active | 17 |
| Lens active | Lens proposed | 8 |
| Record active | Record active | 6 |
| Record active | Record proposed | 9 |
| Review active | Review active | 24 |
| Review active | Review proposed | 24 |
| Session active | Session active | 5 |
| Session active | Session proposed | 3 |

Why each move:

- **active to proposed, 389 rules.** Decision 47: an active rule cites at
  least one source. Their v1 evidence was a grade, our own measure, our own
  bug record or practice, with nothing a reader can open. No source was
  invented; the v1 evidence text is kept in the rule's `open` as "v1
  evidence, not yet a source".
- **active to proposed, 2 rules.** D-T62: `cli.commands.27` and
  `daemon.access.8` keep their sources and their promotion condition in
  `open`.
- **strength Later, 12 rules.** v2 has no Later strength. Each became
  `status: later` with a `when`, and its strength Hard. Two of them had no
  source, so they are proposed rather than later.

Standing is derived (decision 48), never written. After the move: 26
enforced, 842 planned, 118 advisory, 10 later, 7 retired.

## What changed in the files

- `rules/*.yaml`: `evidence` and `refs` became `sources: [{ref, where?,
  says?, backs}]`; the evidence grade left `why`. `exempt` and `Later`
  became `when` and `unless: [{kinds?, facets?, paths?, reason}]`. Checks
  became `{binding, class, tool?, key?, test?, tier, fix?, note?}`. Rule
  text changed only where v2 splits a field.
- `rulebook/parts.yaml`: each part has `layer`, `builds_on`, `bindings`, `facets`,
  `owner` and `when`; the file has a default waiver per strength. The
  comment block on dormant parts is gone: astro, dynamodb, aws, lambda and
  sst are taken only by the kinds `site` and `cloud-service`, which no
  product has (D-T00, D-T01).
- `rulebook/products.yaml` (new): the product kinds `cli`, `daemon`, `service`,
  `web-app`, `library`, `site` and `cloud-service`, each with the products
  that are of it, cited by source.
- `sources.yaml` (new): 187 sources, keyed, each with `kind`, `title`,
  `locator` and `verified` when someone opened it. By kind: 66 target, 41
  decision, 27 article, 21 vendor-doc, 19 book, 8 standard, 4 paper, 1
  incident. By locator: 66 `repo:`, 40 `decision:`, 40 `url:`, 18
  `archive:`, 17 `isbn:`, 3 `doi:`, 3 `rfc:`. Books cite by ISBN.
- `tools.yaml`: each tool has a `key`, and a `preset` and `key_source` when
  a check's key can be resolved against it. golangci-lint (go part, preset
  `presets/golangci-lint/settings.yml` since gragas#1743) and gragas (gates part) were added.
- Generated: `rulebook/rules.json` (its shape in `rulebook/schema/rules-json.schema.json`,
  validated by `gragas check`), `rulebook/bindings.json` (enforced rules only, with
  the checks that hold them), `rulebook/coverage.json` and `rulebook/ids.lock`.

## Locator and source kinds added

- `repo:<repo>/<path>`: a file of a product repository, for target documents
  (decisions 64 and 65).
- `rule:<rulebook>/<id>`, with source kind `external-rule`: a rule
  of another rulebook. A citation of it may say `covers: equal | stricter`.
  None is populated yet; `rulebook/coverage.json` counts them as `external_equal`
  and `external_stricter`.
- Source kind `target`: a product's documented target state.

## Where this departs from R-12

- `tool`, `key` and `test` are optional in the schema, not required by
  class: requiring them would mean writing keys we do not have. A check
  missing them does not resolve, so its rule stays planned, and the
  baselines count it.
- Tier `R` (read in review) is kept: 30 checks of 29 rules carry it.
- A product instance in `rulebook/products.yaml` cites `{ref, where?}` with no
  `backs`: it backs no rule.
- A CHANGELOG.md is not generated: R-12 says a changelog follows from ids
  and statuses but not when an entry is cut or what dates it, under one
  version (decision 55). It stays open.

## Baselines

`rulebook/baseline.json` holds one count per invariant. `gragas check`
fails when a count rises above its baseline, and also when it falls below,
so the commit that closes a gap lowers the number. A test holds this.

| Invariant | Baseline |
|---|---|
| rules-without-source (not retired) | 393 |
| hard-rules-without-check | 2 |
| checks-unresolved (tool or key does not resolve) | 1086 |
| relates-unknown | 0 |
| checks-without-test (pattern, analyzer, policy, fact) | 266 |
| checks-without-fix | 1087 |
| enforced-without-examples | 26 |
| measures-not-one-fact | 17 |
| advisory-with-checks | 5 |

## What could not be mapped honestly

- **No key.** Most v1 checks named a tool in prose (`enforced_by`) with no
  key in a preset. They keep that text in `note`, and their rules are
  planned. Only golangci-lint has a preset the keys resolve against. Where
  the preset's own comment names a rule that had no v1 check naming the
  linter, a check was added from it: 11 such checks, each saying so in its
  note.
- **Review checks.** 13 v1 checks of class review are no v2 class; their
  text moved to the rule's `note`.
- **Dropped classes and fields.** v1 `state`, `plant`, `control` and
  `enforced_by` have no v2 field; they are kept in the check's `note`.
- **Refs with no source.** 11 v1 refs name a report, a review or a note that
  is not in the archive (report and review numbers, a practitioner note, two
  framework environment guides, two operator points). They are in `open`.
- **Grade-only reasons.** 5 rules gave no reason but their evidence grade;
  their `why` now says so.

Since the move to format 3, `gragas gen` writes `rulebook/rules.json` in schema 3 (see
`docs/format/README.md`, The generated rules.json). It still writes `rulebook/ids.lock`, and no longer
writes `bindings.json` or `coverage.json`. `rulebook/schema/rules-json.schema.json` describes schema 2
only; `rulebook/schema/rules-json-3.schema.json` describes what gen writes now.
