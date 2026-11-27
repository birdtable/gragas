# The fixture rulebooks

This folder is the conformance suite of the format and the contract (S25). It is
never for use: nothing here is a rulebook to install, and every name in it is a
stand-in from `gate/standins.txt`. Every encoder, renderer and the explorer is
tested against it; a second implementation runs the same suite.

| Folder | What it is |
|---|---|
| `base/` | The base fixture: kind `overlay` (it extends a parent and imports a part), YAML and Markdown forms, every field of the rulebook types exercised, 15 rules over the four statuses, a translation (one fresh, one stale), exclusions (one lifted), four proposals, a binding record, example pairs, and a history of 13 entries over three releases |
| `other/` | The second fixture, kind `base`: the rulebook the first is mapped to, and the part `base` imports |
| `projects/clean/` | A project that follows `fixture-base` 1.2.0 and holds to it: its configuration is as the rules set it or tighter |
| `projects/weakened/` | The same project after it let go: a weaker setting, a missing one, an entry no rule maps, an expired waiver, a baseline that grew, a copied preset and a rendering edited by hand |

Each folder under `base/`, `other/` and `projects/*` is a root of its own: `format.Load` is given one root at a time and a root holds one `rulebook.yaml`.

## Releases

The history holds three releases of `fixture-base`, told apart by date.

| Release | Dates | What changed in the rules |
|---|---|---|
| 1.0.0 | 2026-09-01 to 2026-09-30 | `go.errors.1` proposed and put on trial; `go.layout.1` proposed and activated |
| 1.1.0 | 2026-10-01 to 2026-10-31 | `go.errors.1` activated; `base.naming.1` activated; `base.naming.2` retired in favour of it |
| 1.2.0 | 2026-11-01 on | `go.errors.3` put on trial with a zero-by date; `web.forms.1` activated |

The release records themselves (the tested set, the health numbers, the bundle) are documents the encoders produce from this input; they are not written here (M1-20, M5).

## Standings, marks and observations

Standing and the marks are derived, never written, so the fixture holds the inputs that derive each of them. The proof that completes "enforced" and "unproven" is added with the proof grids (M4).

| Standing or mark | Input | Where |
|---|---|---|
| proposed | a rule with status `proposed` | `go.errors.2` |
| retired | a rule with status `retired` and `replaced_by` | `base.naming.2` |
| planned | an active `must` whose binding has no proof yet; one with a `zero_by` date | `go.errors.3`, `go.layout.2` |
| advisory | an active `may` with a binding | `base.naming.1` |
| reviewed | a rung-6 binding with its pair | `go.layout.1` |
| enforced | an active rule with a binding at rung 0 to 5 and its pair | `go.errors.1`, `web.forms.1` |
| trial (mark) | status `trial` | `base.errors.2`, `go.errors.3` |
| blocked (mark) | two bindings on one tool entry that disagree | `go.errors.1` and `go.layout.2` on `golangci-lint wrapcheck` |
| unproven (mark) | a binding whose pair is not proven | added with M4 |

| Observation | Where it shows |
|---|---|
| as-set | `projects/clean/.golangci.yml` sets `wrapcheck` as the rule does |
| tighter | `projects/clean/.golangci.yml` enables `nilerr`, which `base.errors.2` asks only as a warning |
| weaker | `projects/weakened/.golangci.yml` disables `errorlint`, which `base.errors.1` and `go.errors.1` ask |
| absent | `projects/weakened/.golangci.yml` does not enable `nilerr` or `revive` |
| unmapped | `projects/weakened/.golangci.yml` enables `gocritic`, which no binding names |
| excluded | `std.errors.2` and the lifted `std.errors.3` in `base/exclusions.yaml`: a project sees the rule as excluded, with the decision `house-call` |

## Not exercised

The model lists these members of closed sets, and no fixture can hold them, because the suite has two rulebooks and each has one value: the identity kinds `companion`, `fork` and `archived`, the contribution `closed`, and the proof level `isolated`. A test holds this list to the model: a new member of any set is a decision here.

## Field coverage

Every field of the model's rulebook types is a row: the field, and the file that exercises it. A test loads the fixtures and fails for a field that no loaded value uses, for a field with no row, and for a row whose file does not carry the field.

| Field | File |
|---|---|
| RulebookIdentity.format | `base/rulebook.yaml` |
| RulebookIdentity.name | `base/rulebook.yaml` |
| RulebookIdentity.version | `base/rulebook.yaml` |
| RulebookIdentity.kind | `base/rulebook.yaml` |
| RulebookIdentity.licences | `base/rulebook.yaml` |
| RulebookIdentity.language | `base/rulebook.yaml` |
| RulebookIdentity.docs_url | `base/rulebook.yaml` |
| RulebookIdentity.explorer_url | `base/rulebook.yaml` |
| RulebookIdentity.extends | `base/rulebook.yaml` |
| RulebookIdentity.imports | `base/rulebook.yaml` |
| RulebookIdentity.owners | `base/rulebook.yaml` |
| RulebookIdentity.governance | `base/rulebook.yaml` |
| RulebookIdentity.tested | `base/rulebook.yaml` |
| RulebookIdentity.budgets | `base/rulebook.yaml` |
| RulebookIdentity.hand_written_presets | `base/rulebook.yaml` |
| Licences.rules | `base/rulebook.yaml` |
| Licences.checker_code | `base/rulebook.yaml` |
| Licences.presets | `base/rulebook.yaml` |
| Extends.rulebook | `base/rulebook.yaml` |
| Extends.version | `base/rulebook.yaml` |
| Extends.source | `base/rulebook.yaml` |
| Extends.content_hash | `base/rulebook.yaml` |
| Import.rulebook | `base/rulebook.yaml` |
| Import.version | `base/rulebook.yaml` |
| Import.source | `base/rulebook.yaml` |
| Import.content_hash | `base/rulebook.yaml` |
| Import.parts | `base/rulebook.yaml` |
| Governance.contribution | `base/rulebook.yaml` |
| Governance.checker_publishers | `base/rulebook.yaml` |
| Governance.checker_second_reviewer | `base/rulebook.yaml` |
| Governance.checker_proof | `base/rulebook.yaml` |
| Governance.decision_required | `base/rulebook.yaml` |
| Governance.trial_releases | `base/rulebook.yaml` |
| Governance.review_days | `base/rulebook.yaml` |
| Tested.tools | `base/rulebook.yaml` |
| Tested.plugins | `base/rulebook.yaml` |
| Tested.kits | `base/rulebook.yaml` |
| Tested.adapters | `base/rulebook.yaml` |
| Tested.min_engine | `base/rulebook.yaml` |
| Version.name | `base/rulebook.yaml` |
| Version.version | `base/rulebook.yaml` |
| Budgets.statement_words | `base/rulebook.yaml` |
| Budgets.reasoning_words | `base/rulebook.yaml` |
| Structure.format | `base/structure.yaml` |
| Structure.layers | `base/structure.yaml` |
| Structure.kinds | `base/structure.yaml` |
| Layer.slug | `base/structure.yaml` |
| Layer.name | `base/structure.yaml` |
| Layer.aliases | `base/structure.yaml` |
| Layer.parts | `base/structure.yaml` |
| Part.slug | `base/structure.yaml` |
| Part.name | `base/structure.yaml` |
| Part.aliases | `base/structure.yaml` |
| Part.owners | `base/structure.yaml` |
| Part.concerns | `base/structure.yaml` |
| Concern.slug | `base/structure.yaml` |
| Concern.name | `base/structure.yaml` |
| Concern.aliases | `base/structure.yaml` |
| ProgramKind.slug | `base/structure.yaml` |
| ProgramKind.name | `base/structure.yaml` |
| ProgramKind.description | `base/structure.yaml` |
| Rule.id | `base/language/go/rules/errors.yaml` |
| Rule.aliases | `base/language/go/rules/errors.yaml` |
| Rule.statement | `base/language/go/rules/errors.yaml` |
| Rule.strength | `base/language/go/rules/errors.yaml` |
| Rule.status | `base/language/go/rules/errors.yaml` |
| Rule.scope | `base/language/go/rules/errors.yaml` |
| Rule.exemptions | `base/language/go/rules/errors.yaml` |
| Rule.reasoning | `base/language/go/rules/errors.yaml` |
| Rule.pairs | `base/language/go/rules/errors.yaml` |
| Rule.owner | `base/language/go/rules/errors.yaml` |
| Rule.review_by | `base/language/go/rules/errors.yaml` |
| Rule.zero_by | `base/language/go/rules/errors.yaml` |
| Rule.relations | `base/language/go/rules/errors.yaml` |
| Rule.origin | `base/language/go/rules/errors.yaml` |
| Rule.sources | `base/language/go/rules/errors.yaml` |
| Rule.bindings | `base/language/go/rules/errors.yaml` |
| Rule.replaced_by | `base/core/base/rules/naming.yaml` |
| RuleScope.parts | `base/language/go/rules/errors.yaml` |
| RuleScope.paths | `base/language/go/rules/errors.yaml` |
| RuleScope.kinds | `base/language/go/rules/errors.yaml` |
| Exemption.paths | `base/language/go/rules/errors.yaml` |
| Exemption.kinds | `base/language/go/rules/errors.yaml` |
| Exemption.reason | `base/language/go/rules/errors.yaml` |
| Reasoning.definition | `base/language/go/rules/errors.yaml` |
| Reasoning.pros | `base/language/go/rules/errors.yaml` |
| Reasoning.cons | `base/language/go/rules/errors.yaml` |
| Reasoning.decision | `base/language/go/rules/errors.yaml` |
| Pair.number | `base/language/go/rules/errors.yaml` |
| Pair.target | `base/language/go/rules/errors.yaml` |
| Relations.relates | `base/language/go/rules/errors.yaml` |
| Relations.conflicts | `base/language/go/rules/errors.yaml` |
| Relations.depends | `base/language/go/rules/errors.yaml` |
| Relations.covers | `base/language/go/rules/errors.yaml` |
| Citation.ref | `base/language/go/rules/errors.yaml` |
| Citation.where | `base/language/go/rules/errors.yaml` |
| Citation.says | `base/language/go/rules/errors.yaml` |
| Citation.backs | `base/language/go/rules/errors.yaml` |
| Binding.tool | `base/language/go/rules/errors.yaml` |
| Binding.target | `base/language/go/rules/errors.yaml` |
| Binding.key | `base/language/go/rules/errors.yaml` |
| Binding.value | `base/language/go/rules/errors.yaml` |
| Binding.options | `base/language/go/rules/errors.yaml` |
| Binding.severity | `base/language/go/rules/errors.yaml` |
| Binding.rung | `base/language/go/rules/errors.yaml` |
| Binding.run_point | `base/language/go/rules/errors.yaml` |
| Binding.remedy | `base/language/go/rules/errors.yaml` |
| Binding.pair | `base/language/go/rules/errors.yaml` |
| Binding.pair_path | `base/language/go/rules/errors.yaml` |
| Binding.tightens | `base/language/go/rules/errors.yaml` |
| Binding.checker | `base/language/go/rules/errors.yaml` |
| Term.slug | `base/terms.yaml` |
| Term.definition | `base/terms.yaml` |
| Term.literal | `base/terms.yaml` |
| Term.status | `base/terms.yaml` |
| Source.slug | `base/sources.yaml` |
| Source.kind | `base/sources.yaml` |
| Source.title | `base/sources.yaml` |
| Source.locator | `base/sources.yaml` |
| Source.attribution | `base/sources.yaml` |
| Source.verified | `base/sources.yaml` |
| Decision.slug | `base/decisions.yaml` |
| Decision.title | `base/decisions.yaml` |
| Decision.text | `base/decisions.yaml` |
| Decision.rejected | `base/decisions.yaml` |
| Decision.cites | `base/decisions.yaml` |
| Decision.who | `base/decisions.yaml` |
| Decision.date | `base/decisions.yaml` |
| Decision.supersedes | `base/decisions.yaml` |
| Decision.superseded_by | `base/decisions.yaml` |
| Cites.sources | `base/decisions.yaml` |
| Cites.rules | `base/decisions.yaml` |
| Cites.bindings | `base/decisions.yaml` |
| Exclusion.overlay | `base/exclusions.yaml` |
| Exclusion.rule | `base/exclusions.yaml` |
| Exclusion.number | `base/exclusions.yaml` |
| Exclusion.decision | `base/exclusions.yaml` |
| Exclusion.who | `base/exclusions.yaml` |
| Exclusion.when | `base/exclusions.yaml` |
| Exclusion.lifted | `base/exclusions.yaml` |
| Lifted.decision | `base/exclusions.yaml` |
| Lifted.who | `base/exclusions.yaml` |
| Lifted.when | `base/exclusions.yaml` |
| Translation.entity | `base/translations/fr/errors.yaml` |
| Translation.field | `base/translations/fr/errors.yaml` |
| Translation.language | `base/translations/fr/errors.yaml` |
| Translation.text | `base/translations/fr/errors.yaml` |
| Translation.hash | `base/translations/fr/errors.yaml` |
| Proposal.format | `base/proposals/tighten-open.yaml` |
| Proposal.slug | `base/proposals/tighten-open.yaml` |
| Proposal.target | `base/proposals/tighten-open.yaml` |
| Proposal.state | `base/proposals/tighten-open.yaml` |
| Proposal.link | `base/proposals/tighten-open.yaml` |
| Proposal.opened | `base/proposals/tighten-open.yaml` |
| Proposal.document | `base/proposals/tighten-open.yaml` |
| HistoryEntry.format | `base/history.jsonl` |
| HistoryEntry.number | `base/history.jsonl` |
| HistoryEntry.previous | `base/history.jsonl` |
| HistoryEntry.kind | `base/history.jsonl` |
| HistoryEntry.entity | `base/history.jsonl` |
| HistoryEntry.before | `base/history.jsonl` |
| HistoryEntry.after | `base/history.jsonl` |
| HistoryEntry.who | `base/history.jsonl` |
| HistoryEntry.when | `base/history.jsonl` |
| HistoryEntry.plan | `base/history.jsonl` |
| HistoryEntry.decision | `base/history.jsonl` |
| HistoryEntry.command | `base/history.jsonl` |
| HistoryEntry.reverts | `base/history.jsonl` |
| HistoryEntry.statements | `other/history.jsonl` |
| Lifecycle.status | `base/history.jsonl` |
| Lifecycle.owner | `base/history.jsonl` |
| Lifecycle.review_by | `base/history.jsonl` |
| Lifecycle.zero_by | `base/history.jsonl` |
| Lifecycle.replaced_by | `base/history.jsonl` |
| Lifecycle.literal | `base/history.jsonl` |
| Book.anchors | `base/language/web/guide.md` |
| Book.binding_records | `base/bindings/std-errors-1.yaml` |
