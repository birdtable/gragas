# The fixture's documents

Hand-authored goldens of the contract (v1.0), written from the input in
`internal/host/assets/fixture/` (the YAML of `base/`, `other/` and
`projects/*`; every name in it is a stand-in from `gate/standins.txt`). They
are the spec the encoders (M4, M5) reproduce byte for byte. Nothing here is
generated: the meaning of each field was written by hand, and `jq -S --indent
2` only sorted keys and laid the bytes out.

| File | Document | What it is |
|---|---|---|
| `bundle.json` | bundle | `fixture-base` 1.2.0: its rules, bindings, sources, decisions, history, the release with its tested set and health, the proof grid, the rule map and the rendering manifest |
| `changelog.json` | changelog | the three releases, 1.0.0, 1.1.0 and 1.2.0, each rule and term entry with its consumer action and remedy |
| `report-clean.json` | report | the clean project: pinned to 1.2.0, every entry held, one live waiver and one baseline, no drift |
| `report-weakened.json` | report | the weakened project: a weaker, an absent and an unmapped entry, an edited copy and an edited rendering, an expired waiver, a grown baseline, a withdrawn release |
| `plan.json` | plan | the plan of the clean project's `init` (the digest its `yes` row names) |
| `sarif.json` | SARIF | the weakened project's findings, with the engine's extension |
| `proposal.json` | proposal | one open proposal to bind `go.errors.1` more tightly |
| `pages/` | renderings | the sample pages the bundle's rendering manifest hashes (see Renderings) |

`internal/host/jsonschema/contractschema_test.go` validates each against its real schema; `internal/core/contract/fixture_test.go` holds them to this page: each
is its own canonical bytes (2-space indent,
RFC 8785 key order, LF, one trailing LF), every field of the v1 schemas is in
at least one document (the one exemption is named in the test and below), and
the health numbers and the bump are recomputed from the bundle's own data.

## Standing and marks (S32)

Six standings, three marks beside one. Each rule's standing is derived from
its status, binding, rung and proof; a mark never replaces a standing.

| Rule | Status, strength | Standing | Marks | Why |
|---|---|---|---|---|
| `go.errors.2` | proposed, should | proposed | | status `proposed` |
| `base.naming.2` | retired | retired | | status `retired`, `replaced_by` `base.naming.1` |
| `base.naming.1` | active, may | advisory | unproven | a `may` with a binding; its proof has not run (no pair) |
| `go.layout.1` | active, must | reviewed | | its binding is rung 6, a person's review, and its result is `reviewed` |
| `go.errors.1` | active, must | enforced | blocked | both bindings pass (rung 1 `wrapcheck`, rung 0 `errorlint`) with a pair; it conflicts with `go.layout.2` |
| `web.forms.1` | active, must | enforced | | its binding (rung 2) passes with a pair |
| `go.errors.3` | trial, must | planned | trial, unproven | a `zero_by` date, and its one proof fails |
| `base.errors.2` | trial, should | planned | trial, unproven | trial, and no pair to prove it |
| `base.errors.1`, `base.targets.1`, `base.targets.2`, `go.layout.3` | active | planned | unproven | a binding with no proof (no pair) |
| `go.layout.2` | active, must | planned | blocked, unproven | no proof, and it conflicts with `go.errors.1` |
| `web.forms.2`, `web.forms.3` | active, should | planned | | no binding, so nothing is unproven |

## Health (S29)

Seven numbers, in S29's order, for release 1.2.0. A ratio is the exact
decimal of num/den to 4 places, rounded half to even; no denominator is empty,
because the fixture holds an item of every set a ratio divides by.

*Live rules* are those with status `active` or `trial` (13: 11 active and 2
trial). A `proposed` rule is not yet a rule of the release and a `retired`
one no longer is.

| Name | Derivation | num | den | Value |
|---|---|---|---|---|
| `rules-held` | live rules with at least one entry in the coverage's `rules_to_bindings`, over live rules | 11 | 13 | 0.8462 |
| `entries-explained` | entries of `bindings_to_rules` that map to a rule, over those entries | 16 | 16 | 1 |
| `proof-pass-rate` | proof results `pass` over those that ran (`pass` and `fail`); `not run` and `reviewed` did not run | 3 | 4 | 0.75 |
| `completeness` | active rules with a source, a pair and a decision (a history entry naming one), over active rules | 3 | 11 | 0.2727 |
| `review-currency` | live rules with a `review_by` on or after the release date, over live rules that have one | 11 | 11 | 1 |
| `attention-size` | rules with at least one attention item of seven of S13's eight kinds (an integer, no ratio; the high-override-rate kind is left out: it needs two report batches and S29 keeps signals out of health) | | | 11 |
| `days-to-active-median` | whole days from a rule's `proposed` to its `active` in the history, the median with an even count taken at the lower middle | | | 1 |

Lines, where a derivation takes more than a count:

- `proof-pass-rate`: `go.errors.1/errorlint` pass, `go.errors.1/wrapcheck`
  pass, `web.forms.1/field-label` pass, `go.errors.3/staticcheck.SA4006`
  fail: 3 of 4. Each pair's planted case fails and its clean control passes in
  the pass rows; the fail row is a pair authored failing, so no number rests
  on a placeholder. The review result of `go.layout.1` is a person's, not a
  tool's run, and is outside the denominator.
- `completeness`: `go.errors.1` (3 sources, 2 pairs, decisions `wrap-errors`
  and `wrap-errors-again`), `go.layout.1` and `web.forms.1` (decision
  `house-call`) hold all three; the other eight active rules lack a source,
  a pair or a decision.
- `attention-size`: a rule counts once however many kinds apply to it; a `proposed` rule counts only if a kind applies. A missing pair, source or decision is asked only of an active or trial rule. A trial past its window and a rule a linter holds that still sits in review text apply to no rule (the fixture holds no data for them); no review-by or zero-by is overdue at 2026-11-07. Kinds per rule:

  | Rule | Kinds |
  |---|---|
  | `base.errors.1` | unproven binding; missing pair, decision |
  | `base.errors.2` | unproven binding; missing pair, decision |
  | `base.naming.1` | unproven binding; missing pair, decision |
  | `base.targets.1` | unproven binding; missing source, decision |
  | `base.targets.2` | unproven binding; missing pair, source, decision |
  | `go.errors.1` | blocked binding |
  | `go.errors.3` | unproven binding; missing source, decision |
  | `go.layout.2` | unproven binding; blocked binding; missing pair, source, decision |
  | `go.layout.3` | unproven binding; missing pair, source, decision |
  | `web.forms.2` | missing pair, source, decision |
  | `web.forms.3` | missing pair, source, decision |

  Eleven rules. `go.errors.2` (proposed), `base.naming.2` (retired), `go.layout.1` and `web.forms.1` have no item: 11.
- `days-to-active-median`: `go.layout.1` 2026-09-04 to 2026-09-05 is 1 day;
  `go.errors.1` 2026-09-01 to 2026-10-03 is 32; `base.naming.1` 2026-10-05
  to 2026-10-05 is 0; `web.forms.1` 2026-11-04 to 2026-11-05 is 1. Sorted: 0,
  1, 1, 32. The lower middle of four is the second: 1.

## Bump (S11)

Release dates: 1.0.0 on 2026-09-30, 1.1.0 on 2026-10-31, 1.2.0 on 2026-11-07.
The bump of a release is the largest row of S11's table over its entries:
a rule added, a rule replaced with a successor, or a tightened binding is a
minor; an explanation, a translation, a remedy or a term is a patch; a rule
removed, retired with no successor or weakened, or a preset entry removed, is
a major.

| Release | Entries | Largest | Bump |
|---|---|---|---|
| 1.2.0 | `go.errors.3` added (minor), `web.forms.1` added (minor), term `gate` changed (patch) | minor | minor |

## How the fixture's YAML maps to the contract's entities

- A rule's `reasoning` is the definition only; its relations map by kind
  (`see-also`, `conflicts-with`, `refines`); source kinds are written with
  spaces.
- A translation's field is `reasoning`. A history entry drops `format` and
  `previous`.
- The fixture's own fields (`tested`, `budgets`, `verified`, and a source's
  `where`, `says` and `backs`, and the `aliases` of a layer or a concern) are not fields of the contract and are not
  carried.
- The bundle is the 1.2.0 set of `fixture-base` alone. The imported
  `ui.forms.1` and the parent's `std.*` rules are not rules of this bundle;
  `std.*` appears only in its exclusions. Rule map rows: `go.errors.1` to
  `go.errors.1` weaker, `go.errors.2` to `go.errors.2` tighter, `go.layout.1`
  to nothing (`none`), `web.forms.1` to `ui.forms.1` same.
- A tool outside the tested set has `tool_version` `none`.

## Renderings

The bundle's manifest for release 1.2.0, target `pages`, names two pages, each
hashed by its source's bytes: `pages/go/errors.md` (English) by
`language/go/rules/errors.yaml`, and `pages/fr/go/errors.md` by
`translations/fr/errors.yaml`. The French page records one omission, the
stale translation of the `go.errors.1` reasoning, with the plan it was made
under.

## The two reports

Both pin `fixture-base` 1.2.0 and the same release. `lock_digest` is the
sha256 of each project's `.gragas/lock.yaml` as it stands in the fixture, and
the section's `manifest` is the digest of the bundle's rendering manifest.
Age is whole days from a waiver's or baseline's creation to the report's run
time (2026-11-10 for the clean project, 2026-11-13 for the weakened one): `w-legacy-a`
and `b-wrap` are 5 days old in the clean report, `w-old-b` and the grown
`b-wrap` 104 days old in the weakened one (2026-08-01 to 2026-11-13). The
closed waiver `w-closed-c` is not live and is not in the report.

The weakened report's release carries `withdrawn`, and its `trust.release_state`
says so, so the field is present in a document; `switch` is present, `null`,
on the provenance of `go.errors.1`.

## What is not here

- Of the schemas' fields only `deprecation.since` and `removed_in` are in no
  document: v1.0 deprecates nothing. The test names them.
- The `bundle` digest in each project's `.gragas/lock.yaml` still names the
  earlier bundle. The lock rows are fixture input, outside this change; the
  report's `lock_digest` is the sum of the lock as it is.
