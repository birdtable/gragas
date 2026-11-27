# The contract's field reference, v1

A skeleton written from the schemas under `internal/host/assets/contract/schema/v1/` by `gragas-generate`; the handbook completes the derivation column. Edit the schemas and run `make gen`; a hand edit is lost.

## document-bundle

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| adapters_used | array | 1.0 | `[]` | to be written |
| adapters_used[].adapter_version | text | 1.0 | required | to be written |
| adapters_used[].tool | text | 1.0 | required | to be written |
| adapters_used[].waiver_syntax |  | 1.3 | `null` | to be written |
| bindings | array | 1.0 | required | to be written |
| changelog | array | 1.0 | required | to be written |
| checker_code | array | 1.0 | `[]` | to be written |
| conflicts | array | 1.0 | `[]` | to be written |
| contract | contract | 1.0 | required | to be written |
| coverage | object | 1.0 | `{}` | to be written |
| coverage.bindings_to_rules | array | 1.0 | `[]` | to be written |
| coverage.rules_to_bindings | array | 1.0 | `[]` | to be written |
| decisions | array | 1.0 | required | to be written |
| exclusions | array | 1.0 | `[]` | to be written |
| health | entity-health | 1.0 | required | to be written |
| history | array | 1.0 | required | to be written |
| identity | entity-rulebook | 1.0 | required | to be written |
| presets | array | 1.0 | `[]` | to be written |
| proof_grid | array | 1.0 | `[]` | to be written |
| release | entity-release | 1.0 | required | to be written |
| released | boolean | 1.0 | required | to be written |
| rendering_manifest | array | 1.0 | `[]` | to be written |
| rule_map | array | 1.0 | `[]` | to be written |
| rules | array | 1.0 | required | to be written |
| sources | array | 1.0 | required | to be written |
| structure | entity-structure | 1.0 | required | to be written |
| terms | array | 1.0 | required | to be written |
| tested_set | entity-tested-set | 1.0 | required | to be written |
| versions | versions | 1.0 | required | to be written |

## document-changelog

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| contract | contract | 1.0 | required | to be written |
| releases | array | 1.0 | required | to be written |
| releases[].bump |  | 1.1 | `null` | to be written |
| releases[].date | date | 1.0 | required | to be written |
| releases[].entries | array | 1.0 | required | to be written |
| releases[].release_note | textOrEmpty | 1.0 | `""` | to be written |
| releases[].version | semver | 1.0 | required | to be written |
| rulebook | slug | 1.0 | required | to be written |

## document-grid

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| cells | array | 1.0 | required | to be written |
| contract | contract | 1.0 | required | to be written |
| tested_set |  | 1.0 | `null` | to be written |

## document-plan

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| contract | contract | 1.0 | required | to be written |
| plan | entity-plan | 1.0 | required | to be written |

## document-proposal

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| contract | contract | 1.0 | required | to be written |
| proposal | entity-proposal | 1.0 | required | to be written |

## document-report

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| baselines | array | 1.0 | `[]` | to be written |
| contract | contract | 1.0 | required | to be written |
| drift | array | 1.0 | `[]` | to be written |
| drift[].entry | text | 1.0 | required | to be written |
| drift[].kind | enum | 1.0 | required | to be written |
| last_change |  | 1.0 | `null` | to be written |
| lock_digest | digest | 1.0 | required | to be written |
| mismatches | array | 1.0 | `[]` | to be written |
| mismatches[].expected | text | 1.0 | required | to be written |
| mismatches[].found | text | 1.0 | required | to be written |
| mismatches[].what | enum | 1.0 | required | to be written |
| pin | object | 1.0 | required | to be written |
| pin.rulebook | text | 1.0 | required | to be written |
| pin.version | semver | 1.0 | required | to be written |
| release | entity-release | 1.0 | required | to be written |
| renderings | array | 1.0 | `[]` | to be written |
| renderings[].path | text | 1.0 | required | to be written |
| renderings[].state | enum | 1.0 | required | to be written |
| run_time | timestamp | 1.0 | required | to be written |
| sections | array | 1.0 | required | to be written |
| sections[].manifest | digest | 1.0 | required | to be written |
| sections[].name | textOrEmpty | 1.0 | `""` | to be written |
| sections[].per_rule | array | 1.0 | required | to be written |
| sections[].per_rule[].bindings | array | 1.0 | `[]` | to be written |
| sections[].per_rule[].marks | array | 1.0 | `[]` | to be written |
| sections[].per_rule[].observation | enum | 1.0 | `"held"` | to be written |
| sections[].per_rule[].provenance | object | 1.0 | `{}` | to be written |
| sections[].per_rule[].provenance.decision |  | 1.0 | `null` | to be written |
| sections[].per_rule[].provenance.layer |  | 1.0 | `null` | to be written |
| sections[].per_rule[].provenance.switch |  | 1.0 | `null` | to be written |
| sections[].per_rule[].rule | id | 1.0 | required | to be written |
| sections[].per_rule[].standing | standing | 1.0 | required | to be written |
| trust | object | 1.0 | required | to be written |
| trust.digests_matched | boolean | 1.0 | required | to be written |
| trust.release_state | enum | 1.0 | required | to be written |
| trust.signature | enum | 1.0 | required | to be written |
| versions | versions | 1.0 | required | to be written |
| waivers | array | 1.0 | `[]` | to be written |
| wiring | array | 1.0 | `[]` | to be written |
| wiring[].line | text | 1.0 | required | to be written |
| wiring[].present | boolean | 1.0 | required | to be written |

## document-sarif

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| $schema |  | 1.0 | `null` | to be written |
| contract | contract | 1.0 | required | to be written |
| runs | array | 1.0 | required | to be written |
| runs[].results | array | 1.0 | required | to be written |
| runs[].results[].level | enum | 1.0 | `"error"` | to be written |
| runs[].results[].locations | array | 1.0 | `[]` | to be written |
| runs[].results[].locations[].physicalLocation | object | 1.0 | required | to be written |
| runs[].results[].locations[].physicalLocation.artifactLocation | object | 1.0 | required | to be written |
| runs[].results[].locations[].physicalLocation.artifactLocation.uri | text | 1.0 | required | to be written |
| runs[].results[].locations[].physicalLocation.region | object | 1.0 | `{}` | to be written |
| runs[].results[].locations[].physicalLocation.region.startLine |  | 1.0 | `null` | to be written |
| runs[].results[].message | object | 1.0 | required | to be written |
| runs[].results[].message.text | text | 1.0 | required | to be written |
| runs[].results[].properties | object | 1.0 | `{}` | to be written |
| runs[].results[].properties.gragas | object | 1.0 | `{}` | to be written |
| runs[].results[].properties.gragas.fix | textOrEmpty | 1.0 | `""` | to be written |
| runs[].results[].properties.gragas.notice | boolean | 1.0 | `false` | to be written |
| runs[].results[].properties.gragas.standing |  | 1.0 | `null` | to be written |
| runs[].results[].ruleId | id | 1.0 | required | to be written |
| runs[].tool | object | 1.0 | required | to be written |
| runs[].tool.driver | object | 1.0 | required | to be written |
| runs[].tool.driver.name | text | 1.0 | required | to be written |
| runs[].tool.driver.rules | array | 1.0 | `[]` | to be written |
| runs[].tool.driver.rules[].id | id | 1.0 | required | to be written |
| runs[].tool.driver.rules[].shortDescription |  | 1.0 | `null` | to be written |
| runs[].tool.driver.version |  | 1.0 | `null` | to be written |
| version |  | 1.0 | required | to be written |

## entity-baseline

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| age_days | integer | 1.0 | `0` | to be written |
| count | integer | 1.0 | required | to be written |
| owner | text | 1.0 | required | to be written |
| rule | id | 1.0 | required | to be written |
| slug | slug | 1.0 | required | to be written |
| tool | text | 1.0 | required | to be written |
| zero_by | date | 1.0 | required | to be written |

## entity-binding

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| checker |  | 1.0 | `null` | to be written |
| key | text | 1.0 | required | to be written |
| options | object | 1.0 | `{}` | to be written |
| pair |  | 1.0 | `null` | to be written |
| pair_path | textOrEmpty | 1.0 | `""` | to be written |
| proof |  | 1.0 | `null` | to be written |
| remedy | textOrEmpty | 1.0 | `""` | to be written |
| rule | id | 1.0 | required | to be written |
| run_point | runPoint | 1.0 | required | to be written |
| rung | rung | 1.0 | required | to be written |
| severity | severity | 1.0 | required | to be written |
| target_type | targetType | 1.0 | required | to be written |
| tightens |  | 1.0 | `null` | to be written |
| tool | text | 1.0 | required | to be written |
| value |  | 1.0 | `null` | to be written |

## entity-changelog-entry

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| bump |  | 1.1 | `null` | to be written |
| change | enum | 1.0 | required | to be written |
| consumer_action | textOrEmpty | 1.0 | `""` | to be written |
| date |  | 1.1 | `null` | to be written |
| diff |  | 1.0 | `null` | to be written |
| entity | text | 1.0 | required | to be written |
| release | semver | 1.0 | required | to be written |
| remedy | textOrEmpty | 1.0 | `""` | to be written |
| sentence | textOrEmpty | 1.0 | `""` | to be written |

## entity-checker-code

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| by_reference | boolean | 1.0 | `false` | to be written |
| digest | digest | 1.0 | required | to be written |
| licence | textOrEmpty | 1.0 | `""` | to be written |
| name | slug | 1.0 | required | to be written |
| publisher | text | 1.0 | required | to be written |
| reviewed_by | array | 1.0 | `[]` | to be written |
| tests_present | boolean | 1.0 | required | to be written |
| tool | text | 1.0 | required | to be written |

## entity-decision

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| cites | object | 1.0 | `{}` | to be written |
| cites.bindings | array | 1.0 | `[]` | to be written |
| cites.rules | array | 1.0 | `[]` | to be written |
| cites.sources | array | 1.0 | `[]` | to be written |
| date | date | 1.0 | required | to be written |
| rejected | array | 1.0 | `[]` | to be written |
| slug | slug | 1.0 | required | to be written |
| superseded_by |  | 1.0 | `null` | to be written |
| supersedes |  | 1.0 | `null` | to be written |
| text | text | 1.0 | required | to be written |
| title | text | 1.0 | required | to be written |
| who | text | 1.0 | required | to be written |

## entity-exclusion

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| decision | slug | 1.0 | required | to be written |
| lifted |  | 1.0 | `null` | to be written |
| number | integer | 1.0 | required | to be written |
| overlay | slug | 1.0 | required | to be written |
| rule | id | 1.0 | required | to be written |
| when | timestamp | 1.0 | required | to be written |
| who | text | 1.0 | required | to be written |

## entity-finding

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| entity | text | 1.0 | required | to be written |
| fix | textOrEmpty | 1.0 | `""` | to be written |
| kind | slug | 1.0 | required | to be written |
| location | textOrEmpty | 1.0 | `""` | to be written |
| message | text | 1.0 | required | to be written |
| notice | boolean | 1.0 | `false` | to be written |
| refusal | boolean | 1.0 | `false` | to be written |
| rule |  | 1.0 | `null` | to be written |

## entity-health

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| numbers | array | 1.0 | required | to be written |
| numbers[].name | slug | 1.0 | required | to be written |
| numbers[].value | number | 1.0 | required | to be written |
| resolved | boolean | 1.0 | `false` | to be written |

## entity-history-entry

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| after | object | 1.0 | required | to be written |
| before | object | 1.0 | `{}` | to be written |
| command |  | 1.0 | `null` | to be written |
| decision |  | 1.0 | `null` | to be written |
| entity | text | 1.0 | required | to be written |
| kind | slug | 1.0 | required | to be written |
| number | integer | 1.0 | required | to be written |
| plan | digest | 1.0 | required | to be written |
| status |  | 1.3 | `null` | to be written |
| when | timestamp | 1.0 | required | to be written |
| who | text | 1.0 | required | to be written |

## entity-plan

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| changes | array | 1.0 | required | to be written |
| changes[].entity | text | 1.0 | required | to be written |
| changes[].standing_after |  | 1.0 | `null` | to be written |
| changes[].standing_before |  | 1.0 | `null` | to be written |
| changes[].what | text | 1.0 | required | to be written |
| checker_code | array | 1.0 | `[]` | to be written |
| checker_code[].diff |  | 1.0 | `null` | to be written |
| checker_code[].name | slug | 1.0 | required | to be written |
| command | text | 1.0 | required | to be written |
| consumer_actions | array | 1.0 | `[]` | to be written |
| digest | digest | 1.0 | required | to be written |
| question | text | 1.0 | required | to be written |
| rekeyed | array | 1.0 | `[]` | to be written |
| rekeyed[].entity | enum | 1.0 | required | to be written |
| rekeyed[].slug | slug | 1.0 | required | to be written |
| rekeyed[].state | enum | 1.0 | required | to be written |
| renderings | array | 1.0 | `[]` | to be written |
| renderings[].path | text | 1.0 | required | to be written |
| renderings[].what | enum | 1.0 | required | to be written |
| writes | array | 1.0 | required | to be written |
| writes[].path | text | 1.0 | required | to be written |
| writes[].what | text | 1.0 | required | to be written |

## entity-preset

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| digest | digest | 1.0 | required | to be written |
| entries | integer | 1.0 | `0` | to be written |
| extends | array | 1.0 | `[]` | to be written |
| extends[].rulebook | text | 1.0 | required | to be written |
| extends[].version | semver | 1.0 | required | to be written |
| licence | textOrEmpty | 1.0 | `""` | to be written |
| name | slug | 1.0 | required | to be written |
| release | semver | 1.0 | required | to be written |
| tool | text | 1.0 | required | to be written |

## entity-proof-result

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| adapter_version | textOrEmpty | 1.0 | `""` | to be written |
| binding | text | 1.0 | required | to be written |
| inputs | digestOrEmpty | 1.0 | `""` | to be written |
| reason | enum | 1.0 | `"no pair"` | to be written |
| result | result | 1.0 | required | to be written |
| run_level | enum | 1.0 | `"fixture"` | to be written |
| rung | rung | 1.0 | `0` | to be written |
| tool | textOrEmpty | 1.0 | `""` | to be written |
| tool_version | text | 1.0 | required | to be written |

## entity-proposal

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| change | object | 1.0 | required | to be written |
| change.anatomy | text | 1.0 | required | to be written |
| change.binding | textOrEmpty | 1.0 | `""` | to be written |
| change.pair | textOrEmpty | 1.0 | `""` | to be written |
| change.sources | array | 1.0 | `[]` | to be written |
| evidence | array | 1.0 | `[]` | to be written |
| evidence[].kind | enum | 1.0 | required | to be written |
| evidence[].text | text | 1.0 | required | to be written |
| link |  | 1.0 | `null` | to be written |
| opened | date | 1.0 | required | to be written |
| slug | slug | 1.0 | required | to be written |
| state | enum | 1.0 | required | to be written |
| target | text | 1.0 | required | to be written |

## entity-release

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| assets | array | 1.0 | `[]` | to be written |
| assets[].digest | digest | 1.0 | required | to be written |
| assets[].path | text | 1.0 | required | to be written |
| bump | enum | 1.0 | required | to be written |
| checker_code | array | 1.0 | `[]` | to be written |
| checker_code[].digest | digest | 1.0 | required | to be written |
| checker_code[].name | slug | 1.0 | required | to be written |
| checker_code[].publisher | text | 1.0 | required | to be written |
| contract | contract | 1.0 | required | to be written |
| date | date | 1.0 | required | to be written |
| format | integer | 1.0 | required | to be written |
| health |  | 1.0 | `null` | to be written |
| release_note | textOrEmpty | 1.0 | `""` | to be written |
| tested_set |  | 1.0 | `null` | to be written |
| version | semver | 1.0 | required | to be written |
| withdrawn |  | 1.0 | `null` | to be written |

## entity-rendering-manifest

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| release | semver | 1.0 | required | to be written |
| renderings | array | 1.0 | required | to be written |
| renderings[].language | language | 1.0 | required | to be written |
| renderings[].omissions | array | 1.0 | `[]` | to be written |
| renderings[].omissions[].plan | digest | 1.0 | required | to be written |
| renderings[].omissions[].what | text | 1.0 | required | to be written |
| renderings[].omissions[].when | timestamp | 1.0 | required | to be written |
| renderings[].omissions[].who | text | 1.0 | required | to be written |
| renderings[].path | text | 1.0 | required | to be written |
| renderings[].rules | array | 1.1 | `[]` | to be written |
| renderings[].rules[].rule | id | 1.1 | required | to be written |
| renderings[].rules[].standing | standing | 1.1 | required | to be written |
| renderings[].source_hash | digest | 1.0 | required | to be written |
| target | text | 1.0 | required | to be written |

## entity-rule-map

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| rows | array | 1.0 | required | to be written |
| rows[].arrived |  | 1.0 | `null` | to be written |
| rows[].left |  | 1.0 | `null` | to be written |
| rows[].mark | mark | 1.0 | required | to be written |
| rows[].reason |  | 1.0 | `null` | to be written |
| rulebook | text | 1.0 | required | to be written |

## entity-rule

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| aliases | array | 1.0 | `[]` | to be written |
| anchor | textOrEmpty | 1.0 | `""` | to be written |
| bindings | array | 1.0 | `[]` | to be written |
| cons | array | 1.3 | `[]` | to be written |
| exemptions | array | 1.0 | `[]` | to be written |
| exemptions[].kind |  | 1.0 | `null` | to be written |
| exemptions[].path | textOrEmpty | 1.0 | `""` | to be written |
| exemptions[].reason | reason | 1.0 | required | to be written |
| id | id | 1.0 | required | to be written |
| marks | array | 1.0 | `[]` | to be written |
| origin | origin | 1.0 | required | to be written |
| owner | textOrEmpty | 1.0 | `""` | to be written |
| pairs | array | 1.0 | `[]` | to be written |
| pairs[].bad |  | 1.3 | `null` | to be written |
| pairs[].good |  | 1.3 | `null` | to be written |
| pairs[].number | integer | 1.0 | required | to be written |
| pairs[].target_type | targetType | 1.0 | required | to be written |
| pros | array | 1.3 | `[]` | to be written |
| reasoning | textOrEmpty | 1.0 | `""` | to be written |
| reasoning_decision |  | 1.2 | `null` | to be written |
| relations | array | 1.0 | `[]` | to be written |
| relations[].kind | enum | 1.0 | required | to be written |
| relations[].rule | id | 1.0 | required | to be written |
| replaced_by |  | 1.0 | `null` | to be written |
| review_by |  | 1.0 | `null` | to be written |
| scope | object | 1.0 | `{}` | to be written |
| scope.kinds | array | 1.0 | `[]` | to be written |
| scope.parts | array | 1.0 | `[]` | to be written |
| scope.paths | array | 1.0 | `[]` | to be written |
| sources | array | 1.0 | `[]` | to be written |
| standing | standing | 1.0 | required | to be written |
| statement | text | 1.0 | required | to be written |
| status | status | 1.0 | required | to be written |
| strength | strength | 1.0 | required | to be written |
| translations | array | 1.0 | `[]` | to be written |
| trial_shipped | array | 1.1 | `[]` | to be written |
| zero_by |  | 1.0 | `null` | to be written |

## entity-rulebook

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| docs_url |  | 1.0 | `null` | to be written |
| explorer_url |  | 1.0 | `null` | to be written |
| extends |  | 1.0 | `null` | to be written |
| format | integer | 1.0 | required | to be written |
| governance | object | 1.0 | required | to be written |
| governance.contribution | enum | 1.0 | required | to be written |
| governance.decision_required | boolean | 1.0 | required | to be written |
| governance.review_days | integer | 1.0 | required | to be written |
| governance.trial_releases | integer | 1.0 | required | to be written |
| imports | array | 1.0 | `[]` | to be written |
| imports[].content_hash | digestOrEmpty | 1.0 | `""` | to be written |
| imports[].parts | array | 1.0 | required | to be written |
| imports[].rulebook | text | 1.0 | required | to be written |
| imports[].version | semver | 1.0 | required | to be written |
| kind | enum | 1.0 | required | to be written |
| language | language | 1.0 | required | to be written |
| licences | object | 1.0 | required | to be written |
| licences.checker_code | textOrEmpty | 1.0 | `""` | to be written |
| licences.presets | textOrEmpty | 1.0 | `""` | to be written |
| licences.rules | text | 1.0 | required | to be written |
| name | slug | 1.0 | required | to be written |
| owners | array | 1.0 | required | to be written |
| successor |  | 1.3 | `null` | to be written |
| version | semver | 1.0 | required | to be written |

## entity-source

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| attribution | textOrEmpty | 1.0 | `""` | to be written |
| kind | enum | 1.0 | required | to be written |
| locator | textOrEmpty | 1.0 | `""` | to be written |
| slug | slug | 1.0 | required | to be written |
| title | text | 1.0 | required | to be written |

## entity-structure

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| kinds | array | 1.0 | required | to be written |
| kinds[].description | textOrEmpty | 1.0 | `""` | to be written |
| kinds[].name | text | 1.0 | required | to be written |
| kinds[].slug | slug | 1.0 | required | to be written |
| layers | array | 1.0 | required | to be written |
| layers[].name | text | 1.0 | required | to be written |
| layers[].order | integer | 1.0 | required | to be written |
| layers[].slug | slug | 1.0 | required | to be written |
| parts | array | 1.0 | required | to be written |
| parts[].aliases | array | 1.0 | `[]` | to be written |
| parts[].concerns | array | 1.0 | `[]` | to be written |
| parts[].concerns[].name | text | 1.0 | required | to be written |
| parts[].concerns[].order | integer | 1.0 | required | to be written |
| parts[].concerns[].slug | slug | 1.0 | required | to be written |
| parts[].guide | textOrEmpty | 1.0 | `""` | to be written |
| parts[].layer | slug | 1.0 | required | to be written |
| parts[].name | text | 1.0 | required | to be written |
| parts[].order | integer | 1.0 | required | to be written |
| parts[].owners | array | 1.0 | `[]` | to be written |
| parts[].slug | slug | 1.0 | required | to be written |

## entity-term

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| definition | text | 1.0 | required | to be written |
| literal | textOrEmpty | 1.0 | `""` | to be written |
| slug | slug | 1.0 | required | to be written |
| translations | array | 1.0 | `[]` | to be written |

## entity-tested-set

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| adapters | array | 1.0 | `[]` | to be written |
| adapters[].adapter | text | 1.0 | required | to be written |
| adapters[].version | text | 1.0 | required | to be written |
| kits | array | 1.0 | `[]` | to be written |
| kits[].kit | text | 1.0 | required | to be written |
| kits[].version | text | 1.0 | required | to be written |
| minimum_engine | semver | 1.0 | required | to be written |
| plugins | array | 1.0 | `[]` | to be written |
| plugins[].plugin | text | 1.0 | required | to be written |
| plugins[].version | text | 1.0 | required | to be written |
| tools | array | 1.0 | required | to be written |
| tools[].tool | text | 1.0 | required | to be written |
| tools[].version | text | 1.0 | required | to be written |

## entity-translation

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| authoritative_hash | digest | 1.0 | required | to be written |
| entity | text | 1.0 | required | to be written |
| field | slug | 1.0 | required | to be written |
| language | language | 1.0 | required | to be written |
| stale | boolean | 1.0 | required | to be written |
| text | text | 1.0 | required | to be written |

## entity-upstream-set

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| observed | array | 1.0 | required | to be written |
| sets | array | 1.0 | required | to be written |
| sets[].entries | array | 1.0 | required | to be written |
| sets[].preset | text | 1.0 | required | to be written |
| tool | slug | 1.0 | required | to be written |
| version | integer | 1.0 | required | to be written |

## entity-waiver

| Field | Type | Since | Default | Derivation |
|---|---|---|---|---|
| age_days | integer | 1.0 | required | to be written |
| expires | date | 1.0 | required | to be written |
| kind | enum | 1.0 | required | to be written |
| owner_present | boolean | 1.0 | required | to be written |
| rule | id | 1.0 | required | to be written |
| slug | slug | 1.0 | required | to be written |
