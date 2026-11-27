---
type: spec
status: current
as-of: 2026-11-07
for: What the product does, and why
---

# gragas: the product spec

Written 7 Nov 2026, refined the same evening. This file is a record of the spec as of that date. Where it names decisions.md, decisions-ideal.md, product-frame.md, the product README and its rulings, or a page or canvas, those are documents of the working folder it was written in (companion 8e9aacc), not of this repository, and are named here in plain text. It holds no technology choice; [the architecture](architecture.md) holds those and the phasing, and says for each part whether it is designed or built.

The one canonical product spec. It supersedes decisions.md (P1–P29) and decisions-ideal.md (I1–I20); where an older document, page, canvas or wireframe differs, this one wins. It is designed inside the operator's frame (product-frame.md): the rulebook is the core; everything gragas does is a domain around it. It describes the ideal, complete product from the user's side. It holds no technology choices and no release staging or cost words; those belong to the architecture and the plan, which are built from this document.

Two words are kept apart in every sentence: **the engine** is gragas, one program for everyone; **a rulebook** is content, one repo per owner. The engine ships no rulebook for use; the one rulebook inside it is the fixture rulebook, marked as the conformance suite and nothing more. A rulebook contains no engine.

Each decision S-n names the earlier decision it keeps or changes. New decisions carry a reason and the options rejected.

## 0. Rulings folded in

| Ruling (the operator, 7 Nov) | Where it lands |
|---|---|
| Overlays may exclude a base rule, with a recorded decision | S4; "exclusion", one of the five loosening words |
| Config-fidelity facts in check, as plain code; code facts as rulebook presets the project's tools run | S14; section 2 "placed or dropped" |
| The explorer stays a gragas product, embeddable | domain 7; S16; S38 |
| Three output layers | section 9; S16 |
| The ideal product, with no release-stage or cost words | S22; this whole document |
| Subdomains are a product concern; the architecture maps them to bounded contexts | section 3; S21 |

The twenty answers the operator accepted on the product review (7 Nov, evening), and where each lands:

| # | Answer | Lands in |
|---|---|---|
| 1 | A command table by domain and context, before any board | section 15 |
| 2 | A bind family: bind, rebind, unbind, conflicts | S34; section 15 |
| 3 | Trial is a modifier on enforced and reviewed; the six standings stay | S32; section 17 |
| 4 | Observation word "excluded" with provenance; an entry conflict shows as "blocked" | S32, S33; section 17 |
| 5 | The fifth status (decided, not yet bound) is dropped; "planned, no date" covers it | S32 |
| 6 | Import grandfathers a rule whose binding already existed | S35; flow 2 |
| 7 | One Markdown block may hold several rules under one anchor | S36 |
| 8 | "Every diagnostic opens its reason" softens to what tools allow; an editor relay is an architecture option | S37; section 8 |
| 9 | The explorer adds a "relations" and a "rulebooks" view | S38; domain 7 |
| 10 | Pin: rulebook, version, source, kinds, parts, content hash. Manifest: parts, kinds | S39; section 16 |
| 11 | check in a project exits 0 with findings listed; fail-on opts in; in a rulebook findings exit 1 | S40; section 15 |
| 12 | "A MUST without an id" is a prose-tool preset the portal runs; gragas never reads the portal | S41 |
| 13 | A copied preset is owned by the project, its digest in the lock, a three-way diff in the upgrade plan | S42 |
| 14 | "Narrow" is dropped from the overlay contract | S4 |
| 15 | Two named sandbox levels, isolated and sandboxed, recorded in the proof grid | S23 |
| 16 | No translate lifecycle command; the authoritative language and the stale hash stay | S27 |
| 17 | Withdrawal lives in the rulebook's own repo, read unsigned, not in the index | S11 |
| 18 | The bare menu detects rulebook, project or neither and leads with it | S19; section 15 |
| 19 | The here view is fed by a local report file; the explorer itself stays embeddable. Narrowed by the night ruling 6 below: the here view is standalone only, and the iframe shows the rulebook views | S38 |
| 20 | init, import, upgrade, switch, the rule and bind families, release and fix end in a plan; a yes is a history entry in a rulebook and a lock entry in a project; a yes flag for scripts | S43; section 15 |

Three notes bound these answers and are decisions here: rewording a rule's prose is a plain file edit and only lifecycle fields go through the CLI (S59); the CLI also corrects, by edit, fix, restore and supersede, each kept in history (S59); every rule needs a source, and the owner's own decision counts as one (S24).

The eight answers the operator accepted on the spec review (7 Nov, night), and where each lands:

| # | Answer | Lands in |
|---|---|---|
| 1 | A rulebook upgrades its own pins (parent, imports, upstream presets) through gragas; upgrade runs in both contexts | S64; flow 5 |
| 2 | propose runs from a project too, in both contexts | S65; S5 |
| 3 | The explorer builds only and ends "done"; no local server; the handbook shows how to open the bundle | S38; domain 7 |
| 4 | A stale translation is informational, with a rendered mark; it never fails the gate | S27; "notice" in section 6 |
| 5 | Per-machine facts live in a local state file outside version control; the lock keeps only what a yes decided and what the release fixed | S66; section 16 |
| 6 | The here view is standalone only; the iframe packaging shows the rulebook views; a portal that wants a project's report hosts it by its own choice | S38 |
| 7 | The engine ships the fixture rulebook as its conformance suite and still ships no rulebook to use | S25; section 1 |
| 8 | init in the "neither" context asks project or rulebook; the rulebook form writes identity and structure and prints the guard's wiring | S67; domain 1; flow 5 |

The six product-review questions (pages/product-review.md) are answered in S21 (domains), S9 (proof for review-only rules), S30 (the page's figures and phone layout) and S31 (the boundary page retired).

## 1. The product

**Thesis.** A style guide is a promise about code that nobody can check. gragas turns it into a rulebook: every rule keeps a stable id and its reasoning, is bound to the exact tool entries that hold it, is proven by its own examples on every supported tool version, is released as a dependency with a computed version, and is explained wherever people and agents already read. Tools enforce; your gate decides; the engine makes the rulebook true, proven and visible and keeps it so as tools, teams and rules change.

**Four verbs.** gragas defines (rules as tool-neutral knowledge), configures (presets a project imports, extends and overrides), explains (in the portal, the editor where a tool allows a docs link, the agent's context, the terminal) and evolves (a reviewed, versioned lifecycle that learns from waivers, overrides and incidents). It runs no linter on a project's code and blocks nothing in a project.

**Scope test.** gragas does a thing only if it makes a rulebook more true (held by tools), more proven (its examples pass) or more visible (explained where people and agents read). It never judges a project's code, never runs a project's checks, and never knows which projects exist. Any tool gragas invokes takes the rulebook's fixtures as input, or prints its own effective config over no source; never a project's source. The one place it refuses is a rulebook's own repo, where its gate and write-time hook protect the rulebook's lifecycle data; that is the rulebook's gate, set up by the rulebook's owner, and it blocks nothing in any project.

**Content test.** Text belongs in a rulebook only if it exists to explain one or more rules. Everything else is the portal's.

**Positioning.** "A style guide that knows how it is enforced, and explains it to people and agents." Public guides are rulebooks that never joined their halves: Google keeps the prose half (definition, pros, cons, decision) beside an approximate config no section names; Vercel kept the config half only, and the guide is archived. gragas joins them; the join survives tool churn because the rule, not the tool entry, is the unit. gragas uses linters, analyzers and reviewers; writes into docs platforms and portals; feeds scorecards and code scanning; generates what agent rule syncers copy by hand; cites what decision-record tools record. One engine, many rulebooks, Apache-2.0 for the engine and for the rulebook, offline, no service. Success test: a well-known public style guide adopts gragas. The runnable step before it: a companion rulebook over a public guide its owner does not control (S50).

## 2. Core and domains

**The core** is content only, one repo per owner: rules; structure (layers → parts → concerns, one guide per part); terms; bindings; presets (generated data plus hand-written checker code); knowledge (guides, the rulebook's own chapters, sources, decisions); identity (name, version, licences, authoritative language, extends and imports each with the content hash it is pinned by, owners, governance, tested set); releases. The engine ships no rulebook for use.

| Domain | Purpose | Features | Value to the rulebook |
|---|---|---|---|
| 1 Format | How a rulebook is written and validated | YAML rule files, or a guide's own Markdown with one structured block per section holding one or more rules, prose and anchors untouched; the anatomy of section 6; terms; frozen ids with aliases; exemptions as a rule's scope exceptions; the overlay and import contracts; the rulebook door of init; translations with an authoritative language and a stale hash; the names check and the prose lint; the fields an active rule must have; validation; the format spec, with a schema per version and forward migration | The record sits beside the prose, and a change to either is in the changelog; a guide joins without a rewrite |
| 2 Authoring and lifecycle | The only path by which a rule's life changes | Rule, term, source and decision commands; trial before active; promotion; the re-check at review-by; corrections (edit, fix, restore, supersede, undo) each kept in history; append-only history; the guard; import from configs, headings and prose MUST lines, with grandfathering; incident-to-rule; propose, from an overlay or a project; the rulebook's own upgrade of its pins; signals; attention; the fix menu | Every change has a decision, a reason and a person's merge; agents maintain a guide safely |
| 3 Enforcement link | Join prose to config, and read it back | Plugins, one per tool and versioned, installed and approved on a machine, with gragas knowing no tool; the bind family; bindings with rung, run point, remedy, proof; coverage both ways; standing derived; entry conflicts detected; presets generated from bindings, checker code hand-written, declared by name and publisher and digested at release; copy-once where a tool has no extends, the copy owned by the project; upstream presets pinned and flattened; kits as package bindings; non-code targets; reviewers at rung 6; the chain resolved into one effective rulebook with provenance; a project's effective config explained line by line; verify | "Which rules are held, by what, which entries explain nothing, and what this project really runs" |
| 4 Proof | Examples are tests | An example pair per binding, standing at a declared path, run by the tool's harness or the generic runner on every tool version in the tested set, in an isolated or sandboxed run; unproven never counts as enforced; a failing pair marks the binding unproven and standing falls; rung-6 pairs are eval cases, never pass/fail; incremental re-runs; the proof grid is published | A tool upgrade that drops a check fails in the guide first |
| 5 Releases | A version that means something | Bump computed from rule, binding and preset diffs; per-rule changelog with the consumer action; tested set; flattened upstream sets so drift is a diff; assets pinned by content hash, with checker-code digests; health computed by the engine and claimed by the release; a rule map toward other rulebooks; diff by rule; archive with a successor; a withdrawn list read unsigned; the index | Upgrades stop being scary; a sunset guide points somewhere |
| 6 Explain and publish | One explanation, many renderings | A page per rule, part, guide, decision, term; in-place blocks for Markdown-form guides; the engine's handbook and the rulebook's chapters; front-matter profiles with a placement per field, and nav fragments per docs platform; catalog rows for portals; lint docs links to the rule page or guide anchor where the tool allows; SARIF help-link rewrite; a rendering manifest so an edited or stale page is a finding; a language per rendering; attribution | The reader finds the rule where they already read |
| 7 The explorer | The views no docs platform renders | Filtered rules with filters and live counts; rule view with anatomy, bindings, proof grid, history, relations, translations; relations view; rulebooks view; tools and unmapped entries; coverage; standing bars; health; changelog; terms; attention; the "here" view over a local report file, standalone only. Standalone, web component, docs and portal plugins, iframe; built as a static bundle; state in the URL; host theme tokens; accessible | The aggregate truth, filterable and embeddable |
| 8 In a project, optional | Presets work with or without gragas | init (pin, lock, copied presets, printed wiring, content-hash and digest checks); status; check with fidelity findings, exit 0 by default and fail-on; waive; baselines; the upgrade plan; the switch plan; monorepo manifests; the report. Explain and verify run here too, but belong to domain 3 | A team sees what applies, what its overrides weaken, and what an upgrade or a switch changes |
| 9 Agents and reviewers | The guide reaches the agent and the reviewer | Scoped rule text in each agent's native folder the project opts into; budgets set by the rulebook in bytes; rules for a path; review instructions per reviewer format, the review question as the text; example pairs as eval fixtures; language setting | Agents get only the rules no tool holds; reviewers cite ids |
| 10 The contract | The data every domain emits, documented and versioned | Schemas, field reference, types, compatibility promise, fixture rulebook, validator, changelog; published as a spec beside the format spec; the explorer is its first consumer | Anyone renders the rulebook any way, and stays compatible |
| Across all | The terminal experience | A menu when bare that leads with the detected context; forms that end in a plan; closing words mapped to exits; JSON everywhere and SARIF where there are findings; plain and accessible modes; the standing palette | Maintainer, developer and agent read the same words |

**Placed or dropped.** Every feature of the older drafts is placed above or dropped here. Dropped: run, apply and pull as commands (they were a code runner; a plan is applied by the command that produced it, on a yes); a policy engine, collectors and graph facts in the engine (code facts such as missed reuse, old code beside its replacement, workflow triggers and dead code are rulebook presets the project's tools run); a prose check over a portal's pages (a prose-tool preset the portal runs, S41); search or MCP (the contract is the API); per-project planted fixtures (verify borrows the rulebook's); scorecard definitions, wiki sync, PR comments (report consumers); server-side gate rules (handbook guidance); schedules; telemetry; a translate command (S27); facets (kinds cover them, S61); the fifth rule status (S32); "narrow" in the overlay contract (S4). The older drafts' strength scales, cost-of-breaking and level fold into must, should, may.

## 3. Subdomains: core, supporting, generic

Two words for two things: **the rulebook** (the frame's "core") is the content, the central model; **the core domain** is the capability only gragas has. The central model is the rule, a tool-neutral practice with a frozen id and its reasoning. Classifying subdomains is the product's job; the architecture maps them to bounded contexts.

**Core domain: enforcement truth. Domains 3 and 4.** The reason to adopt. Bindings from rules to exact config entries, many-to-many; rungs and run points; standing derived, never typed; coverage both ways; entry conflicts; proof of every binding across tool versions; resolving a chain into one effective rulebook with provenance; explaining a project's effective config line by line as rule, observation and layer; verify. Why core: no tool surveyed has it (derived standing appears nowhere in the tools the market research covered), and "which rules are enforced, by what" is every persona's question. This is where gragas must be clearly best.

**Supporting subdomains.** Shaped by the rule model and needed for the core to be usable; each has precedents; none is the reason to adopt. Good enough, simple, and free to reuse others' tools.

| Subdomain (domains) | Purpose | Why supporting |
|---|---|---|
| Format and validation (1) | two source forms, terms, translations, names check, prose lint, schema, migration, the format spec | the model could be written many ways; the link is what matters |
| Authoring and lifecycle (2) | commands, history, decisions, trial, review-by, corrections, guard, propose, signals, attention | decision-record tools do lifecycle; ours is shaped by ids and bindings |
| Composition, distribution and the project (1, 8) | the overlay and import contracts (1); pin, lock, init, status, check, waive, baselines, manifests, the report, the upgrade and switch plans (8) | config tools compose and lock; ours keeps provenance and fidelity |
| Releases (5) | computed bump, changelog, consumer actions, tested set, content-hash pinning, health, rule map, archive, withdrawal, the index | semver tooling exists; ours diffs rules |
| Publishing (6, 7, 10) | Markdown export, docs profiles, the explorer, the contract | renderers are replaceable |
| Agents and reviewers (9) | scoped text, review instructions, eval fixtures | syncers distribute prose; ours selects by standing |
| Import (2) | drafts from configs, headings, prose; grandfathering | one-time, replaceable by hand |

Seven supporting subdomains. The terminal belongs to no subdomain: it is the voice of every domain, and the architecture treats it as a shared shell.

**Generic, adopted and never built.** Enforcement by tools; docs rendering, search, MCP, llms.txt; CI, gates, hook runners; git hosting, pull requests, registries, signing infrastructure; dependency bumps; package managers; schema validation libraries; SARIF consumers; compliance evidence; machine translation; the OS sandbox and containers; terminal drawing and colour. Partners gragas integrates, never competes with.

**A separate product line.** The base rulebook and the owner's overlay are content: the core domain's first customer.

The contract is published by a supporting subdomain, and it is the core domain's published language: the derivation rules it documents (standing, coverage, observation, provenance, bump) are the core's. Domain 10 is not split; it documents what the core derives. The chain resolved with provenance is core because explain needs it.

## 4. Owns / uses / never

| Owns | Uses | Never |
|---|---|---|
| The format spec and its change process: schema, ids and aliases, lifecycle, history, anatomy, terms, translations | Enforcers: linters, compilers, analyzers, pattern and policy tools, tests, kits, AI reviewers | Judges a project's code; runs its checks; blocks a build, commit or edit in a project |
| Plugins, bindings, rungs, standing, coverage, entry conflicts, proof and its isolated and sandboxed runs | Upstream presets and published guides, as sources, imports or rulebooks | Writes a tool config a project did not ask it to copy; writes a hook (it prints the line); writes outside the opted-in agent folders and the review file's path (the pointer line is printed, never written); edits a project's own AGENTS.md or CLAUDE.md |
| Generated preset data; binding ↔ entry fidelity; checker-code digests | Registries, release assets, signing infrastructure, static hosts, mirrors | Installs tools; runs a package manager; loads checker code outside an isolated or sandboxed proof or verify run |
| Computed bump, changelog, consumer actions, tested set, release content hash and its verification, health, rule map, withdrawal | The guide's site, a portal, its catalog, search, MCP, llms.txt | Ships a search, an MCP server, a service, a schedule, telemetry |
| Every explanation and rendering; the explorer; the engine's handbook | Agent harnesses and their rule folders; reviewer formats; machine translation outside the engine | Knows which projects use a rulebook; stores results; builds a rule × project grid; counts installs; reads a portal's own pages (it re-reads only the files it wrote, by the manifest) |
| Pin, lock, state file, manifest, waiver, baseline, report and plan formats; the upgrade and switch plans | Dependency bots, hook runners, CI, rulesets: your gate | Agrees a rule; deletes; names an org, tool or project in a rule; applies a plan without a yes |
| The contract; the CLI and its terminal; the rulebook index; the engine's own governance | Trackers, decision logs, control catalogs, incidents, git hosting | Ships a rulebook in the engine for use (the fixture rulebook is the conformance suite only); requires gragas to use a preset; phones home; accepts an unpinned release silently |

## 5. The ecosystem

Count: one · 1/owner · 1/org · 1/project · many, the owner in brackets where not obvious. Relation: **is** (gragas), **reads**, **writes**, **configures**, **links**, **feeds** (gragas output goes there), **out** (touches gragas only through the org). Groups: engine 1–4, rulebooks 5–10, upstream 11–12, projects 13–16, enforcers 17–21, readers 22–25, gate and org 26–34, hosting 35–36. Thirty-six kinds; gragas is the first four. "Project" (row 13) is a role: any repository that pins a rulebook holds it, so the engine's repository, a rulebook's repository and the portal's repository are projects too when they pin one. The kinds overlap by role and are disjoint by nature.

| # | Kind | Count | gragas | Relations |
|---|---|---|---|---|
| 1 | Engine: binary, renderers, explorer, the engine's handbook | one | is | reads 5–9; writes into 22–25; released via 36 |
| 2 | The two specs: the format spec and the contract, each with its own version, schemas, fixtures and validator; stewarded with the engine, versioned apart from it | one of each per version | is | 5–9, 13, 32 conform |
| 3 | Rulebook index: static list of rulebooks and overlays with their format versions and published health; names only, no withdrawal marks; mirrorable | one (gragas, by PR) | is, as data | resolves names |
| 4 | Enforcer plugins, versioned, each naming the tool version range it describes | one per tool and range (installed with `plugin add`, or a person's own; a tool's community may maintain its own) | is | describe 17; read 13's configs |
| 5 | Base rulebook, YAML-form or Markdown-form | 1/owner, many coexist | reads; writes via CLI | parent of 6; imports 7; extends 11; cites 12; releases 8–9 |
| 6 | Overlay: one parent at a pin, own prefix, nestable | 1/org or division, chained | reads; writes via CLI | child of 5 or 6; imports 7; pinned by 13 |
| 7 | Imported rulebook: parts by name, ids namespaced | many per rulebook | reads at release | binding conflicts reported to the importer; checker code only when declared |
| 8 | Preset packages: one per package ecosystem, locked files, dependency-bot preset, agent and review templates | many per rulebook | generates data; verifies fidelity; records digests | imported by 13, with or without gragas; extend 11; on 36 |
| 9 | Rulebook checker code: plugin rules, analyzers, pattern rules, policies, with their own tests | many per rulebook | links by binding; proves in an isolated or sandboxed run; digests | loaded by 17 in the project, never by gragas |
| 10 | Fork of a rulebook; companion rulebook over a guide its owner does not control; archived rulebook with a successor and a rule map | many (anyone) | records origin; cites by anchor; warns at init, offers the switch plan | Vercel's case and Google's case, handled |
| 11 | Upstream presets and guides not yet rulebooks | many (third parties) | configures at rung 0; flattens; imports as drafts | 8 extends; prose is a source |
| 12 | Sources: standards, control catalogs, vendor guides, studies, incidents, decision records, the owner's own decisions | many | links, never copies | cited by 5–7 |
| 13 | Project, a role any repository may hold: service, library, app, infra, docs, monorepo, the portal, every rulebook, the engine | many (its team) | optional; writes pin, lock, state file, waivers, baselines, opted-in agent files, the report | pins one chain; imports 8; runs 17 at 27–30; reports to 26 and 31–32 |
| 14 | Monorepo tooling | per project | reads, walking up | one pin at root; part manifests per package |
| 15 | Templates and scaffolders | 1/org | out; may run init | day 0 for 13 |
| 16 | Shared kits and libraries | many (org teams) | links (package bindings) | held by 17; versions in the tested set |
| 17 | Enforcer tools and plugins: linters, formatters, compilers, analyzers, pattern, policy, graph and architecture checks, secret and supply-chain scanners, prose, commit, API, migration, container, workflow, telemetry | many, own version lines | configures; reads effective config and output; runs them over fixtures only, in an isolated or sandboxed run | run by 13; in the tested set; diagnostics in 21 |
| 18 | Test frameworks and contract tests (rung 5) | per project | configures via bindings | the project runs them |
| 19 | AI reviewers | per org | configures review text; ships eval cases | enforcers at review |
| 20 | Agent harnesses and rule files | per project | writes scoped files into opted-in folders; prints one pointer line | read at edit time in 13 |
| 21 | Editors, language servers, package managers | per developer | out; a diagnostic's docs URL is the rule page where the tool allows it | no gragas extension |
| 22 | The guide's own site, or a docs build over the export | one per rulebook | writes pages, blocks, nav; hosts the explorer | replaced by 23 where one exists |
| 23 | Developer portal: standards, patterns, decision records, catalog, search, MCP, llms.txt | 1/org or none | writes pages, rows, the rulebook bundle; hosts the explorer; findings link there | built on 24; is a 13 |
| 24 | Docs platforms, file-based and API-only | one per site | writes Markdown, front matter, nav; API-only ones via the org's adapter | host 22–23 |
| 25 | Explorer hosts: HTML page, component host, docs plugin, portal plugin, iframe | many (anyone) | ships element, package, plugins | read 2's data |
| 26 | Your gate: merge check, required workflow, ruleset, local gate | 1/org, or none for a lone project | out; consumes exit codes and the report | runs 17 and check |
| 27 | Hook runners; shared CI templates and org defaults | per project; 1/org | out; prints hook lines and one CI line | run 17 at commit, push; carry 26 |
| 28 | Dependency bots | 1/org | configures via the rulebook's preset | the only timer; bumps 13's pin, 8, 17, and a rulebook's own parent, import and upstream pins |
| 29 | Deploy pipelines and admission control | per environment | configures (policy presets) | run 17 at deploy |
| 30 | Code scanning and SARIF consumers | per org | feeds; rewrites tool SARIF links | from 13 |
| 31 | Scorecard portals and IDPs | per org | feeds; the bundle may seed standards | fed by 32 |
| 32 | Org collector and dashboards | one (the org) | out; schema and metric names are gragas's; signals reads the batches it hands over | reads 13's reports; informs 5–6 |
| 33 | Compliance tooling and control catalogs | per org | links control ids as sources; a controls map | never evidence |
| 34 | Tracker, decision log, incident tooling | one each (the org) | links | decisions and waivers cite; incidents become proposals |
| 35 | Git hosting: repos, releases, rulesets, pull requests; may be a private mirror | per org | out; init fetches releases from any host or path; propose opens a change | carries 5–7 releases and 10's withdrawn list, 26 |
| 36 | Registries, static hosts, signing and provenance; may be private mirrors | per ecosystem | out; distribution targets; content hashes verified at init and upgrade | host 1, 8, 22 |

**Still missing?** Localised renderings beyond the rulebook's own translations, training material and notification channels are contract consumers, not kinds gragas must know.

## 6. Vocabulary

One name per concept. The page, the wireframes and the architecture use these words.

- **Rulebook**: a versioned repo of rules and everything that explains and holds them. **Base** (no parent), **overlay** (one parent at a pin, own prefix), **import** (parts by name from another rulebook, ids namespaced), **fork** (origin recorded), **companion** (a base that cites a guide its owner does not control, by anchor, never copying), **archived** (frozen, with a successor and a rule map). **Chain**: base → org → division → project. **Project**: any repository that pins a rulebook; "repo" is used only for a rulebook's own repository.
- **Structure**: layer → part → concern. **Kind**: a named trait of a program a rulebook defines (service, library, app, infra, docs, and any the rulebook adds); a project declares several; rules are scoped by kind. **Guide**: one narrative per part, no MUST lines. **Term**: a word defined once; it may carry a literal (a header name, a package) so statements stay neutral. **Chapter**: a rulebook's own knowledge text beyond the guides.
- **Rule**: id (frozen; aliases kept when renumbered; a Markdown-form rule also records its anchor, which may change), statement, strength, scope (parts, paths, kinds), exemptions (scope exceptions by path or kind, each with a reason), reasoning (definition, pros, cons, decision), example pair, status, owner, review-by, zero-by (optional, for a planned rule), relations (relates, conflicts, depends, covers), origin, sources, bindings, translations, history, replaced-by. **Strength**: must, should, may. **Status** (decided by people, four): proposed, trial, active, retired.
- **Binding**: tool, target type, key or value (at rung 6 the review question and when it applies), options, severity, rung, run point, remedy, the pair it is proven by and the path that pair stands at, proof. **Rung**: 0 upstream preset, 1 configured built-in, 2 pattern, 3 own plugin or analyzer, 4 fact held by a policy tool, 5 test or kit, 6 review. **Run point**: edit, commit, push, land, review, release, deploy. **Remedy**: the tool's fixer, named, never run. **Plugin**: the code that stands for one tool, with its own version and the tool version range it covers; installed by digest and approved on a machine, and not a tool's own plugin, which a tool's config loads (S23). **Tool entry**: one line of a tool's config (tool, target, key). **Entry conflict**: two bindings in one resolved rulebook naming the same tool entry with different values, options or severity; an overlay's tightening of a parent's entry is not a conflict, the tighter value wins with provenance "changed". **Effective binding**: at rung 0–5, a binding that is proven and not blocked; at rung 6, a binding whose pair exists and that is not blocked; standing is derived from effective bindings only.
- **Standing** (derived, never typed, six): proposed (status proposed); enforced (an effective binding at rung 0–5); reviewed (effective bindings at rung 6 only); planned (must or should with no effective binding; a zero-by may be set); advisory (may with no effective binding); retired (status retired). **Marks** (three, shown beside a standing): trial (status trial; on enforced, reviewed or planned, never on advisory or proposed), blocked (a binding in an unresolved entry conflict), unproven (a binding with a missing, failing or not-run proof). **Coverage**: rules → bindings and entries → rules; an upstream preset adopted whole at rung 0 counts as one covered set. **Example pair**: one good and one bad example; the binding that uses it says where it stands. **Proof**: the pair's result per binding per tool version, with the run level; at rung 6 the pair is an **eval case** and the cell reads "reviewed". **Tested set**: the tool, tool-plugin, kit and gragas-plugin versions a release was proven on, plus the minimum engine. **Isolated run**: a scratch directory holding only fixtures and the config under test, a scrubbed environment, a time limit, and a memory limit only where one is set. **Sandboxed run**: an isolated run inside a container or the OS sandbox.
- **Preset**: the rulebook's shipped config, package or locked file. **Copied preset**: a preset a project asked init to copy because its tool has no extends; the project owns the file, the lock holds its digest. **Checker code**: the hand-written plugin rules, analyzers, pattern rules and policies a rulebook ships, declared by name and publisher and digested at release. **Effective config**: what a tool runs for a file after the project's own config. **Observation** (six): as-set, tighter, weaker, absent, unmapped, excluded. **Provenance**: which layer set, changed or excluded an entry, with the decision, or which switch brought it.
- **Override** (free, observed), **waiver** (rule, path, reason, kind, owner, expiry, ticket), **baseline** (counted debt, shrink-only, owner, zero-by; the tool's own baseline file where one exists), **exemption** (rulebook scoping), **exclusion** (an overlay dropping a parent rule with a decision): the five loosening words.
- **Overlay contract**: add in own prefix, tighten (a rebind on a parent's rule that sets a stricter value, options or severity), rebind, annotate (the overlay's own note, source or example placed beside a parent rule, a prose edit in the overlay's files that changes no parent field), exclude with a decision; never edit a parent rule, and never weaken one except by a recorded exclusion. **Governance**: published contribution mode, owners per part, who may publish checker code and whether it needs a second reviewer, the proof level required for third-party checker code, review cadence, decision requirement, trial window. **Proposal**: a structured change carried to a parent.
- **Release**: rules, bindings, presets, changelog, tested set, health and rule map at one tag. **Consumer action**: what a project does about one change. **Health**: the named numbers a release publishes about the rulebook itself, computed by the engine, claimed by the release. **Rule map**: a release's directional mapping between two rulebooks, row by row: the id in the rulebook left, the id in the rulebook arrived at, a mark (same, tighter, weaker or none) and a reason. **Withdrawn list**: a file in the rulebook's repo, read unsigned, naming releases no longer to be used. **Upgrade plan**: what a new release of the same rulebook changes in this project. **Switch plan**: what a different rulebook changes in this project. **Plan**: the preview every changing command shows before a yes. **Yes**: the recorded acceptance of a plan, a history entry in a rulebook and a lock entry in a project. **State file**: a project's per-machine facts (tool versions found, last fetch, withdrawn list seen), kept outside version control.
- **Authoritative language**: the one language, declared in identity, in which a rulebook's text is the law. **Translation**: the same text in another language, carrying the hash of the authoritative text it translated; stale when the hashes differ; a plain file edit, never a command.
- **Explanation**: statement, reasoning, examples, sources, from one source. **Rendering**: page, block, docs-link target, agent line, review line, badge, catalog row, explorer view. **Handbook**: the engine's book on how rules, bindings, standing, releases and projects work. **Docs profile**: a host's front-matter and catalog mapping, with a placement per field (front matter, body, omitted). **Report**: a project's fidelity state as one document. **Signals**: per-rule aggregates over report batches an org hands over. **Attention list**: rules needing a maintainer. **Finding**: one thing a check, validation or proof says is wrong, with the entity it names and the fix it offers. **Notice**: one thing a check or validation lists as information; it never changes the closing word (a stale translation is one). **Index**: the list of rulebooks. **Contract**: the documented data. **Format spec**: the public specification of how a rulebook is written, versioned apart from the engine.

## 7. The lifecycle

| Stage | Who executes | Verb | The rulebook gives | gragas does | What flows back |
|---|---|---|---|---|---|
| Plan | people, decision records, tracker | explains | the rules for this kind of program | status lists them; a proposal cites the decision | |
| Document | guide authors, docs platform | defines | rules, guides, terms, decisions, translations | export, blocks, chapters, catalog rows; a stale translation is a notice, rendered with a mark | the portal's prose tool, with the rulebook's preset, flags a MUST line without an id |
| Write | developer, agent, editor | explains | scoped rules; example pairs | agent files; rules for a path; docs links in diagnostics where a tool allows | repeated agent misses become a proposal |
| Check on save | linters, compilers, formatters | configures | presets at rung 0–3 | explain a diagnostic's rule on request | |
| Commit | hook runners | configures | presets for hook tools | printed hook lines | |
| Push and land | your gate | explains | presets; fidelity rules | check: report, exit code, SARIF; verify on request | overrides, weakened rules, drift |
| Review | people, AI reviewers | explains | review-held rules with eval cases | review text with ids and the review question | remarks become proposals |
| Release | release tooling, scanners | configures | supply-chain presets | nothing at run time; the presets came from bindings | |
| Deploy | pipelines, admission control | configures | policy presets | nothing at run time; the presets came from bindings | |
| Upgrade | dependency bots | evolves | a new release with its changelog and its content hash | the upgrade plan, or the switch plan | overrides made redundant upstream |
| Operate | operations, monitoring | receives | | nothing; an incident arrives as a proposal with its example | an incident becomes a proposal |
| Learn | owner, org collector | evolves | lifecycle commands; attention; review-by re-checks; trial outcomes | signals; propose; promote, retire, replace, rebind | the next plan |

Twelve stages. Proof failures on new tool versions, expired waivers, overdue review-by dates, trials past their window, entry conflicts and unmapped upstream entries also arrive at learn from the rulebook's own gate. gragas acts at every stage but operate, where it only receives, and executes none; learn ends in a command, so the loop closes.

## 8. People and moments

- **Style-guide maintainer.** Before: two dozen Markdown guides and an approximate config nobody can map to a section; tool releases risk silent loss; nobody can say how an agent should follow the guide. After: each section carries a block with ids, strengths, scopes and bindings; coverage and health are published; a tool upgrade that drops a check fails in the guide's gate; releases carry a computed version, a content hash and consumer actions; agent files generated; URLs and attribution kept.
- **Org platform team.** Before: standards in a portal, configs in a package, rules in a data file, a copied CI script in thirty projects; two searches; nobody can say which rules are held. After: one overlay extends a public base and imports what fits; presets flow through packages and the dependency bot; the portal shows rule pages and the explorer; signals show which rules are overridden everywhere.
- **Developer.** Before: a long standard, a reviewer's memory, an unrecorded override, a scary upgrade. After: a diagnostic opens its reason where the tool allows a docs link, and explain is the door for the rest; explain shows what applies here and which layer set it; overrides are visible; waivers carry owner and expiry; the upgrade is a plan.
- **Coding agent.** Before: a stale pasted block of portal text, no path scoping, rules the linter already holds, free to edit the rulebook's files. After: short, path-scoped rules no tool holds, a URL per id; examples as teaching; a hook the project adds asks for the rules of the file being edited; the lifecycle reachable only through the CLI.
- **AI reviewer.** Before: generic instructions, no ids, findings nobody can trace. After: a review file in its own format with only review-held rules, each with an id, its review question, "cite the rule id" and its example pair as an eval case; a rule a linter comes to hold drops out.
- **Portal owner.** Before: standards with MUST lines and no enforcement field; a second site competing for readers. After: rule pages and catalog rows in the portal's own front matter, each field placed where the host's schema allows; the explorer as a plugin; the portal's search, MCP and llms.txt serve the rules; the portal's own prose tool, running the rulebook's preset, flags a MUST line without an id.
- **Contributor proposing a rule.** Before: a PR to prose; no anatomy, binding or decision; a closed guide has no door. After: propose produces a structured change (anatomy, example pair, draft binding, sources) as a pull request where accepted; the rule enters as trial; the overlay keeps the rule meanwhile and records the outcome.
- **Security reviewer at a consumer.** Before: a config package and plugin code from the internet, trusted because a colleague added it. After: a release pinned by content hash whose checker code is listed by digest and publisher, the rulebook's published governance saying who may publish it and who reviewed it, proof run at a named level (isolated or sandboxed), and an upgrade plan that names every checker-code change before anything applies.
- **Reader in another language.** Before: an English standard, or an unofficial translation that drifted. After: the rule page in their language, marked where the translation is behind the authoritative text; the id, the bindings and the examples the same everywhere.
- **Compliance reader.** Before: a hand-kept spreadsheet of controls to tools. After: rules cite control ids; the controls map renders "supports control X"; evidence stays in the compliance tool.
- **New joiner.** Before: read everything. After: the explorer filtered to this kind of program; each rule with pros, cons and decision; the examples as a first exercise.

Eleven people.

## 9. The three output layers

1. **Our render: the explorer.** The aggregate views no docs platform renders (domain 7); it reads only the contract, which proves the contract sufficient.
2. **Text in their tools.** Markdown pages per rule, part, guide, decision and term, plus the engine's handbook and the rulebook's chapters, with front-matter profiles and nav fragments per host; in-place blocks; agent and review files in each harness's format; docs links in diagnostics where the tool allows. Each in the language the host asks for, with the authoritative language as fallback.
3. **Data with a documented contract.** Everything else derives from it.

**The contract in outline.** Section 16 gives every document at field level.

- **Documents.** The *rulebook bundle*, one document per release. The *project report*, one document per run of check. The *changelog*, per release and rule, with consumer action and remedy. *SARIF* for gragas's findings and rewritten tool output. *Plans*, as documents when asked. Every document carries the contract, format and engine versions.
- **Identity.** Stable ids for rulebook, rule, term, binding, decision, source, history entry, proposal, release, waiver and baseline; URL patterns per rendering.
- **Schemas** per entity, one set per contract version, at a stable URL; **types** generated for the ecosystem's languages from that one schema source per contract version: for TypeScript the published package `@gragas/contract` (types and schema validators), for the engine's own language the types inside the engine.
- **Field reference.** Per field: meaning, type, authored or derived, the derivation rule when derived (standing, marks, coverage, bump, observation, provenance, health), and relations. The entity graph is drawn once and reused everywhere, in the explorer's relations view among others.
- **Compatibility promise.** The contract has its own semantic version. A minor adds fields and enum values with defaults. Before a major removes or changes a meaning, both shapes are emitted for two minors. Consumers may pin; the explorer reads the current and previous major; a validator checks any document.
- **Worked examples.** A fixture rulebook ships with the engine as its conformance suite, marked so and never for use: its bundle, two project reports (clean and weakened), a changelog across three releases, a rule map to a second fixture, and their renderings; renderer authors, the explorer and the export are tested against it.
- **The contract's own changelog** carries a consumer action per change.

## 10. Quality attributes

The product promises these; the architecture says how they are met.

| Attribute | Promise |
|---|---|
| Explain is fast | One diagnostic's explanation returns in well under a tenth of a second once a project is warm; status and explain over one package return in about a second. The consumers of this promise are the agent hook, which runs on every edit, and a person at the terminal. The bundle is indexed once per lock and reused; the effective config is read once per config set and invalidated by a config change. |
| Thousands of rules | A resolved chain of ten thousand rules, their bindings and the proof grid, without the explorer, validation, export or release slowing beyond interactive use. Proof re-runs only the bindings whose fixtures, adapter, checker code or tool version changed. |
| Monorepos | Hundreds of packages under one pin and one lock; one report with a section per package; status per package; the walk up the tree happens once per run. |
| Private and air-gapped | Everything is files and git. Releases come from any git host, a path or a mirror; the index can be mirrored; presets come from private registries; the explorer is a static bundle; nothing needs the network after fetch; no CDN, no phone-home. |
| Accessible | The explorer meets WCAG 2.2 AA in its standalone and iframe packagings: complete by keyboard, standing never shown by colour alone, names for screen readers, reduced motion honoured. The web component, which lives inside a host's own page, states the hosts it was tested in. The terminal's accessible mode uses the same words and symbols (section 17). |
| Deterministic | The same inputs produce byte-identical bundles, pages and reports, the report's run time among its inputs, so a rendering diff is a real change. Derived documents carry no clock reading except the release date and the report's run time; proof timestamps stay in the gate's log. |
| Honest about trust | A release is pinned by its content hash; checker code is digested; proof runs at a named level; nothing applies without a yes. |

## 11. Decisions

S1 to S7 and S10 to S22 keep an I-decision, updated where a ruling or a review answer changed it. S8, S9 and S23 onward are new. Every decision carries its reason and the options rejected.

### Kept from the ideal product, updated

**S1 Thesis, scope test, content test.** Section 1. The boundary is "never judges a project's code, never runs its checks, blocks nothing in a project"; any tool gragas invokes takes the rulebook's fixtures as input, or prints its own effective config over no source. The refusals in this document (the write-time hook, the gate's refusal of a lifecycle change without history, the release's refusal of a conflicting preset entry, init's refusal of a digest mismatch) are all the rulebook's own gate or a project's own acceptance step; none blocks a build, commit or edit in a project. The content test from the 7 Nov session is kept: text belongs in a rulebook only to explain rules. *Keeps I1.* Reason: "blocks nothing" unqualified was false, and a reader who finds one refusal stops trusting the rest. Rejected: dropping the guard (agents would edit lifecycle fields freely).

**S2 Two source forms, one model.** YAML, or a guide's own Markdown with one structured block per section. A block holds one or more rules under the section's anchor (S36); each rule has its own id and one-sentence statement; the section prose is their shared reasoning; a prose change appears in the changelog as "explanation changed". Anatomy as in section 6; terms linked; status includes trial. *Keeps I2.* Rejected: YAML only, the flagship deal-breaker.

**S3 Composition: extend one, import many.** One parent per overlay; any number of imports by part, added at init or afterwards (S69), ids namespaced by rulebook name; two imports binding the same tool entry differently is an entry conflict (S8) the importer resolves; an import brings checker code only when the importer declares it by name and publisher, and the importer's release digests it. *Keeps I3.* Rejected: multiple parents; union by precedence.

**S4 Overlay contract.** Add in own prefix, tighten, rebind, annotate, exclude with a decision; never edit a parent rule in place, and never weaken one except by a recorded exclusion. "Narrow" is dropped (ruling 14): restricting a parent rule to fewer files is a weakening, and exclusion with a decision is the honest form of it; an overlay that wants a parent rule on fewer paths excludes it and adds its own rule in its own prefix with the smaller scope. The exclusion is made with `rule exclude`, which requires a decision and is a history entry; it is lifted with `rule include`, also with a decision and a history entry. include on a rule that is not excluded ends "usage"; on a rule the parent has retired, "not ready", naming the parent's release; a lifted record stays and a second exclusion gets the next sequence number; include's plan lists the overlay's own rules whose "covers" relation names the parent rule, since both would apply. Tighten is a rebind on a parent's rule (S34); annotate is the overlay's own note, source or example beside a parent rule, a prose edit that changes no parent field and renders with the overlay named. An overlay may ship its own checker code under its own prefix. *Keeps I4 and the rulings.* Rejected: narrow defined as a partial exclusion (two words for one act).

**S5 Governance and proposals.** Every rulebook publishes its governance: contribution mode (open, proposals, closed), owners per part, who may publish checker code and whether it needs a second reviewer, the proof level third-party checker code must reach (S23), review cadence, decision requirement, trial window. Propose carries a structured change to the parent: where accepted, it prints the proposal document and the line that opens the pull request or issue with the host's own tool; where closed, a recorded proposal; the engine itself opens nothing on a host; the overlay keeps the rule meanwhile; a project may propose too (S65). The rulebook's own upgrade (S64) flags tightenings and exclusions a parent release made redundant or conflicting. *Keeps I5; adds the checker-code, proof-level and trial fields, and the project's door.*

**S6 Bindings.** Tool, target type (source, config, migration, container, workflow, iac, api-spec, prose, commit, telemetry registry, dependency, package), key or value, options, severity, rung, run point, remedy, the path the pair stands at (S44), proof. At rung 6 the key is the review question and the value is when it applies (S51). Remedy names the tool's fixer, never run by gragas. *Keeps I6.*

**S7 Enforcement truth.** Coverage both ways, with the upstream unit of S47; proof on every tool version in the tested set; unproven never enforced; a failing pair marks the binding unproven and standing is re-derived from the effective bindings (S32); standing derived, never typed; a must or should without an effective binding is planned, with a zero-by where the owner sets one; a rulebook may forbid standings; an adapter may ask a tool to print its config. *Keeps I7.*

**S10 Verify in a project, opt-in.** A project may run the rulebook's example pairs through its own tool invocation, in an isolated or sandboxed run, to prove its effective config still holds each rule; inputs are the rulebook's fixtures, never the project's source. Verify mirrors the project's config set into the run and places each pair at the path its binding declares (S44). Reading config cannot see a plugin version or a custom rule's behaviour; verify can. *Keeps I8.*

**S11 Releases, withdrawal and the index.** Computed bump; per-rule changelog with consumer action and remedy; tested set with plugin versions (S46); flattened upstream sets; assets pinned by content hash and verified at init and upgrade (S23); health (S29); the rule map (S28); archive with a successor. Withdrawal lives in the rulebook's own repo as a withdrawn list (ruling 17): init, upgrade and the dependency-bot flow read it when they fetch; status warns from the state file's last fetch and says when that was. The list is read unsigned, from the rulebook's own repository at the pinned source and over the fetch's own transport. A withdrawal warns at status and blocks an upgrade to the release it names; it never changes anything by itself. The risk: a forged list can fake a withdrawal, and it can also hide one, so a user can stay on a withdrawn release with no warning, which is not a false-alarm-only risk. It is accepted: forging the list takes the same access as replacing a release itself (a push to the rulebook's repository, or the transport), and the operator chose to fix it later if issues show up. The index is a list kept in a repository gragas maintains, changed only by pull request; the engine reads it from that repository or a mirror the handbook or the org names, with no service in between. It resolves short names and records each rulebook's format version and published health; it carries no withdrawal marks and no hosted registry. *Keeps I9; adds content-hash verification, health, rule map, format version, withdrawal in the repo.* Reason for the withdrawal placement: a mark in the index that status must fetch would make the index a service and its maintainers gatekeepers. Rejected: withdrawal in the index; no withdrawal at all.

**S12 In a project.** Presets work through plain extends; init, status, explain, check, waive, baseline, verify, the upgrade plan and the switch plan add on top; a thin pointer, not copies, except a copied preset the project asked for (S42); org-wide enforcement is the org's required workflow; one pin and lock per monorepo, package manifests declare parts and kinds (S39). The upgrade plan applies each changed rule's consumer action to this project's waivers, overrides, baselines, copied presets and agent files, nothing without a yes. Loosening is recorded and rendered; tool-native suppression and baseline files are read, never written (S60). *Keeps I10; adds the switch plan and the copied preset.*

**S13 Report, signals, attention.** The report per the contract, no project identifier, sections keyed by manifest digest (S48); metric names are gragas's, the collector the org's. Signals reads report batches the org hands over, compares two batches, and prints per-rule aggregates (override rate, waiver age, false-positive rate, baseline age), storing nothing (S45). Attention derives per rule, in eight kinds: unproven binding; missing example pair, source or decision; overdue review-by; planned past zero-by; trial past its window; blocked binding; high override rate, only when two batches are given; a rule a linter now holds that still sits in review text. The rulebook knows the last one by comparing each rule's standing with the agent and review renderings its last release shipped (the rendering manifest of 16.9); the item clears at the next release, which rebuilds them, and a project sees the same as a stale rendering in its check until its own refresh. *Keeps I11; adds the new kinds.*

**S14 Check scope and the guard.** In a project, check reports: fidelity of every entry (the observation, overrides among them), versions against the tested set, waiver expiry and missing owner, baseline growth, edited copied presets, edited or stale renderings, content-hash or digest mismatch, a withdrawn release in the pin, a lock behind the pin (upgrade owed), an orphaned waiver (S71). Until upgrade runs, observations are computed against the lock's release, the one the project last said yes to, so a bumped pin changes nothing explain says before the plan is accepted. In a rulebook, check reports: integrity, coverage, proof, entry conflicts, required fields, prose lint, names, budgets, history, review-by, trial windows, bump size, attention, checker code without tests or required review; a stale translation is listed as a notice, never a finding. Config-fidelity facts are plain code in check, with no policy engine; code facts and prose facts are rulebook presets the project's tools run (S41). The rulebook gate refuses a lifecycle change without its CLI history entry; a write-time hook in the rulebook's repo denies direct edits to lifecycle fields; both call the guard command, which lists the lifecycle fields a pending change touches without a history entry and ends "refused", or ends "done" when there are none; none in a project. check runs in both contexts: in a project the fidelity listing above, in a rulebook the gate listing above, where any finding ends "findings". Exit codes per S40. An org may fail its own gate on any finding, a weakened must among them, through fail-on or by reading the report; gragas only reports. *Keeps I12 and the operator's ruling.*

**S15 Explain and publish, and which wins.** Pages, blocks, the engine's handbook, the rulebook's chapters, front-matter profiles with a placement per field (S49), nav fragments, catalog rows, lint docs links where a tool allows, SARIF rewrite, rendering manifest, attribution, a language per rendering. Which wins when a rule is written in several places: the rulebook is the law; agent, review and page text are renderings, and disagreement is drift, reported as a stale rendering; shared CI is your gate and may hold whatever it likes, and gragas's report says only what the rulebook asked. *Keeps I13; adds language and the answer to "which wins".*

**S16 The explorer and the contract.** The explorer is a gragas product: standalone, web component, docs and portal plugins, iframe; built as a static bundle, never served by the engine; state in the URL; host theme tokens; reads only the contract; shares one render model with the export; accessible (S26). Views per S38. The contract is a domain with its own version, promise, fixtures and page. Three output layers, section 9. *Keeps I14 and the operator's rulings.*

**S17 Agents and reviewers.** Scoped rule text into opted-in native folders; one pointer line printed for AGENTS.md, never written; only rules no rung 0–5 effective binding holds, as `id: statement` with at most one pair and a URL; budgets are rulebook settings in bytes; review text per reviewer format carries the review question and the eval cases; text in the authoritative language unless the project sets another. A reviewer format is named by the file it produces: the plain Markdown format writes REVIEW.md at the project root, and the format chosen in the lock is the opt-in for that path; where a file of that name exists and the lock's sums do not list it, the plan names it and the yes replaces it. Agent files go in each harness's own folder, and each file's name carries the rulebook and the part, so two rulebooks or parts never share a file. The line that has a harness ask for a file's rules before an edit is shown once per opted-in harness, printed at init and in the handbook, and never written. The opt-in at init is the yes; a refresh rewrites only inside the opted-in folders and needs no new plan; changing the folders or the reviewer format afterwards is the opt-in form of agents or review, a plan with a lock entry, and an opt-out removes the files at that refresh. *Keeps I15; adds the opt-in form.*

**S18 Import.** Draft rules from configs, guide headings and prose MUST lines; grandfathering per S35; no baselines, since a guide's repo has no debt to count (a project's existing tool baseline file is read at init, S60); why, examples, sources and owner marked to do where missing; proposed rules ship in no preset; attribution recorded. *Keeps I16; adds grandfathering.*

**S19 Terminal and command set.** A menu when bare that detects rulebook, project or neither and leads with it; "neither" offers init and the handbook (ruling 18). Forms that end in a plan (S43). Eight closing words mapped to exits: done 0, findings 1, failed 1, usage 2, busy 3, not ready 4, refused 5, declined 6. JSON everywhere; SARIF where there are findings. Plain and accessible modes. The full set is section 15. *Keeps I17; adds "declined" and "failed".* Reason for "declined": a plan shown and not applied is neither done nor a finding, and a script must be able to tell. Rejected: declined as exit 0 (a script would believe it applied). Reason for "failed": the engine itself can fail (an unreadable settings file, a child process that dies), and that is not a finding; it shares exit 1 with "findings" so a gate fails on either, and the word and the JSON kind tell them apart (rulebook#34). Rejected: a ninth exit code (the estate fixes the codes).

**S20 Licence, attribution, never.** Apache-2.0 for the engine and for the rulebook; a rulebook declares its licences per kind (S52); imports and forks carry attribution into every rendering. No runner for project code, server, telemetry, search, MCP, schedule, project registry, rulebook in the engine for use, deletion or agreement by gragas. *Keeps I18.*

**S21 Subdomains and the domain split.** Section 3. Explain, verify and chain resolution are domain 3's, the enforcement link, where the capability lives; the core domain is domains 3 and 4, whole; domain 8 keeps the conveniences of being in a project and sits with composition and distribution in the subdomain map (S63). *Keeps I19; answers review question 1.* Reason: a domain split across two bounded contexts would force the architecture to carry the product's indecision. Rejected: "part of 8" as core; format or lifecycle as core.

**S22 The ideal product.** This spec describes the complete product. No release-stage mark, no staging word and no cost mark appears in it or on the page; the architecture and the plan carry those. *Keeps I20 and the operator's ruling.*

### New decisions

**S8 Entry conflicts: two rules, one entry, different values.** Detection: at validation and at chain resolution, two bindings in one resolved rulebook that name the same tool entry (tool, target, key) with different values, options or severity are an entry conflict. The same entry with the same value held by two rules is the normal many-to-many case and is not a conflict. Resolution: inside one rulebook the author resolves, by splitting the entry (different options are usually different entries), by rebinding one rule, or by declaring a `conflicts` relation and recording a decision that names the winning binding; across a chain a child's rebind wins only where the overlay contract allows it (tighter), otherwise it is a finding; across imports the importer resolves (S3). Until resolved, both bindings are blocked (S32): the conflict is a finding in the rulebook's check, the release refuses to generate the preset entry, and each rule's standing is derived from its other effective bindings. Reason: a silently generated entry would make one rule's binding a lie, and a lie in a preset is exactly what gragas exists to prevent. Rejected: last binding wins (hides the lie); the most severe wins (a tightening nobody decided); leave it to the tool's own merge (tools differ, and the explanation would not match the entry).

**S9 Proof for review-only rules.** *Answers review question 2.* A rung-6 binding's example pair is an eval case for the AI reviewer: the review file ships the pairs as fixtures, and a reviewer's own eval harness may run them. The proof grid shows "reviewed" in every rung-6 cell, which is the standing, never pass or fail. Where a rulebook runs reviewer evals, the result is published beside the cell as information and changes no standing. Reason: a model's verdict is not deterministic across versions, so pass/fail would be a standing nobody can reproduce; but the pairs are the best teaching a reviewer and an agent get, and dropping them would weaken rung 6 further. Rejected: pass/fail from a reviewer run; review rules without examples.

**S23 Trust and security of what a rulebook runs.** A rulebook is a dependency, and its checker code is code a consumer's tools will load.
- *Who may publish checker code.* Only owners the rulebook's governance names; an overlay only under its own prefix; an import never silently (S3). The release lists every checker-code artifact by digest and by publisher. An overlay's release also lists, under the parent's name and publisher and by the parent's digest, each artifact its resolved presets load from the parent's release; it republishes none of them. The project's lock holds every artifact its tools load, the parent's included (16.3), and the upgrade plan shows a change to any of them as its own action with a diff link.
- *Review of checker code.* Checker code has its own tests in the rulebook, run by the tool's harness in the rulebook's gate; the rulebook's governance says whether it needs a second reviewer, and the release records that it had one. The rulebook gate refuses a release whose checker code lacks tests or the required review.
- *What a consumer trusts beyond the pin.* A release is pinned by the hash of its content, the way go.sum pins a module; the hash in the lock says nothing changed since the user's yes; the tested set says on which tool versions it was proven; the published governance says who reviewed; the health numbers say how complete the rulebook is. init and upgrade verify the hash and the digests and refuse a mismatch; an unpinned release is taken only with the user's explicit yes, recorded in the lock, and an upgrade re-pins a new hash only with a yes. There is no release signing for v1, so nothing says who published a release; the withdrawn list is read unsigned, and its risk is S11's.
- *Two run levels.* (ruling 15) An **isolated run** is a scratch directory holding only the rulebook's fixtures and the config under test, with read-only access to the tool's own dependencies where the gragas plugin's grant names them (a tool's config loads its plugins from there, and nothing else is reachable), a scrubbed environment with no inherited secrets, a time limit, and a memory limit only where one is set (gragas sets none of its own), the tool's output parsed and never executed. A **sandboxed run** is an isolated run inside a container or the OS sandbox, which is what can deny network and file access; where the system gives no network block, a run that needs one is refused by name (sandbox.ErrNoBlock, from sandbox.Check) and never runs unblocked. The proof grid records the level of every cell; the handbook says what each level defends against; a rulebook's governance may require the sandboxed level for third-party checker code, and the release records which level each binding reached. The tool versions of the tested set are supplied by the rulebook's gate (the handbook shows how); gragas looks for each where it is told, never installs one, and a missing version makes the cell "not run", which is a finding.
- *A compromised rulebook.* gragas never loads checker code outside an isolated or sandboxed run; in a project only the project's tools load it, and only after the project accepted the release. The upgrade plan lists every checker-code change as its own consumer action with a diff link, and nothing applies without a yes. Signals show a sudden change in override rate between batches; the rulebook's withdrawn list (read unsigned, S11) names a bad release, and status warns from the last fetch.
Reason: a rulebook that ships code is a supply chain, and the honest defence is the same as for any dependency, made visible rule by rule; two named levels say what is defended instead of promising "a sandbox" a scratch directory cannot deliver. Rejected: a central review authority (gragas would become a registry); gragas executing checker code itself (a runner); accepting an unpinned release with a warning only (a warning nobody reads); one unnamed sandbox level.

**S24 Quality of the rules themselves.**
- *Required fields.* Every rule needs at least one source before it leaves proposed; the owner's own recorded decision is a source of kind "own decision". A must or should becomes trial or active only with a source, a decision and an example pair; a may becomes active with a source and a decision, and needs a pair once it is bound. Without them the rule stays proposed and validation refuses the promotion; the one exception is a grandfathered import (S35), which enters active with the pair on the attention list. Reason: a rule without a source is an opinion, without an example is untestable, without a decision is unexplainable.
- *A prose lint.* Validation checks the statement is one sentence in the imperative, within the rulebook's length budget, uses defined terms as defined, names no tool, org or project except through the rulebook's own terms (S56) and no "we". Richer style is a prose tool's job, bound in the rulebook's own rulebook like any project.
- *Trial before must.* A must or should enters as trial; trial lasts at least one release and at most the governance's window; during trial its binding ships at the tool's warning severity where the tool has one (S53 where it has none), standing derives as usual with the trial mark, and signals count its overrides. Promotion to active is a history entry with the decision; an overlay may tighten a trial rule to active for itself. A trial past its window is an attention item. Trial has two exceptions: a grandfathered import (S35), and restore, which returns a retired rule to active because it had been active and keeps its history; its bindings are re-proven.
- *A re-check at review-by.* Every active rule carries a review-by date (governance sets the default cadence). At review-by the owner confirms (a history entry that sets the next date), changes or retires the rule, looking at its signals and proof; an overdue date is a finding in the rulebook's check and an attention item.
Rejected: warnings only for missing fields; a must without a trial; auto-retire at review-by (a rule dying by calendar); a rule with no source at all.

**S25 Stewardship of the format and the engine.**
- *An open spec.* The rulebook format is a public specification with its own version, written in prose with its schemas and the fixture rulebook as its conformance suite, so a second implementation can exist; the contract is its sibling spec. The engine is the reference implementation and names the format and contract versions it reads.
- *A public change process.* Changes to the format, the contract or the plugin interface's shape arrive as proposals to the engine repo under a published template (motivation, change, compatibility, migration), are decided in public, and land in a decision log with the rejected options. Deprecations keep a window of two minors.
- *A compatibility promise.* The engine reads the current and previous major format version and migrates forward; the contract's promise is section 9's; a release names both versions; the index records each rulebook's format version so a consumer sees a mismatch before fetching.
- *Maintainers.* The engine publishes its maintainers by area (format, plugins, explorer, contract, handbook), with the same governance shape a rulebook publishes; a plugin may be maintained by the tool's own community; the engine's own conventions are a rulebook it pins.
Reason: a flagship guide adopts a format it can leave, not a format that lives only in one binary. Rejected: the format defined by the engine's code; a standards body before adoption; one maintainer.

**S26 Quality attributes.** Section 10 is part of the product: fast explain with its consumers named, ten thousand rules, hundreds of packages, private and air-gapped hosting, an accessible explorer per packaging (S58), determinism with dates handled (S57). Reason: a product that cannot promise these is not adopted by the orgs and guides it is for. Rejected: promises without numbers.

**S27 Rule prose in several languages.** A rulebook declares one authoritative language in its identity; its text fields (statement, reasoning, guide prose, chapters, term definitions, decisions) may carry translations keyed by language. A translation records the hash of the authoritative text it translated; when that text changes the translation is stale: validate lists it as a notice, never a finding, so the gate never fails on it (night ruling 4), and it renders with a mark until re-translated. Ids, keys, bindings, example code and the changelog's consumer actions are never translated. Renderings, agent text and the explorer take a language and fall back to the authoritative language with a mark. A translation change is "translation changed" in the changelog, never a rule change. There is no translate command (ruling 16): a translation is a plain file edit a contributor supplies, like any prose; it is not a health number and not an attention item. Reason: a guide read in one org across countries, or a public guide with a community, needs the same rule in several languages, and the id and binding must stay one thing; but the lifecycle must not grow a dimension the flagships do not ask for. Rejected: one language only; separate translation rulebooks (ids drift); machine translation inside the engine (a generic domain, used by a contributor, not shipped); translation as a lifecycle command.

**S28 Switching rulebooks.** A release may publish a rule map toward another rulebook. A map is directional, read in the direction of the switch: each row names the id in the rulebook being left, the id in the rulebook being arrived at that replaces it, a mark (same, tighter, weaker or none) saying what the arrival rule is against the one left, and a reason. Either side may publish one; an archived rulebook publishes one toward its successor when it is archived. When a project changes its pin to a different rulebook, the switch plan shows per rule what happens, in the plan's words kept (map: same), tighter, weaker, gone (map: none) and new (a target rule no source rule maps to); which tool entries change; which waivers and baselines are re-keyed and which are orphaned; which copied presets and agent files are rewritten. It applies nothing without a yes and records the switch as provenance in the lock and the report. The switch reads the first map it finds, in this order: the one published by the rulebook being left (the author's own record of where each rule went; an archived rulebook's is the successor path), then the one published by the rulebook being arrived at; the map found is used whole, maps are never merged, and the plan names the map and the side that published it (the lock's provenance records both). Without a map, the switch plan is built from the diff of the two resolved presets: rules that share a tool entry are matched and every match is marked a guess for a person to confirm. Reason: without a map, leaving a rulebook means losing every waiver and decision, which is lock-in by neglect. Rejected: cold re-init (loses history); automatic switching; a second matching method beside the preset diff.

**S29 How the product measures itself without knowing projects.** Three sources, none of which names a project.
- *Health*, computed by the engine and claimed by the rulebook in every release: rules held (coverage rules → entries), entries explained (coverage entries → rules), proof pass rate across the tested set, completeness (share of active rules with a source, a pair and a decision), review-by currency, attention list size, median time from proposed to active. Seven numbers. The derivation of each is in the contract; the explorer and the index show them. An overlay publishes two labelled sets (S54). Signals are never folded into health, so health names no project even indirectly.
- *Signals*, computed from report batches an org hands over, printed and stored nowhere by gragas: override rate, waiver age, false-positive rate, baseline age, per rule, and the change between two batches (S45).
- *Adoption*, counted only in public: rulebooks in the index and their health, and public guides that adopted. The success test is a public fact.
No telemetry, no phone-home, no install counting; registries and git hosting count what they count. Reason: the no-tracking promise is part of why a public guide adopts; measurement must come from the artifacts themselves. Rejected: telemetry; a central dashboard; counting downloads; signals inside health.

**S30 The product page.** *Answers review questions 3, 4 and 5.* Section 13. The six-kinds map is section 1's main picture, and Google's and Vercel's halves are a card strip under it. Figures may show one real tool entry per example, the one exception to "no tool syntax", because a stranger needs one concrete entry to see what a binding is; prose never does. Phone layout is a page concern: every figure scrolls sideways above its minimum width, and the six-kinds map stacks on a phone. Reason: the page is read by strangers in ten minutes, and a reader who cannot place the kinds of project cannot place gragas. Rejected: the halves as the main picture (it shows no kinds of project); invented tool entries.

**S31 The boundary page.** pages/boundary-v2.html is superseded by sections 1, 8 and 9 of the product page and retired with a note. Reason: two pages drawing one boundary drift. Rejected: keeping both with a cross-link.

**S32 Statuses, standings and marks.** (rulings 3, 4, 5) Status is typed by people and has four values: proposed, trial, active, retired. Standing is derived from status, the bindings and their proof, and has six values: proposed and retired mirror the status; enforced, reviewed, planned and advisory are derived from the effective bindings, those that are proven and not blocked. A rung-6 binding is effective when its pair exists and it is not blocked, since it is never pass or fail (S9). Three marks sit beside a standing and never replace it: trial (the status), blocked (a binding in an unresolved entry conflict), unproven (a binding whose proof is missing, failing or not run). So a trial must with a proven binding is "enforced, trial"; a trial must with no effective binding yet is "planned, trial"; a must whose only binding failed on a new tool version is "planned, unproven"; a must whose only binding is in a conflict is "planned, blocked"; a may whose binding failed is "advisory, unproven". Trial sits on enforced, reviewed or planned, never on advisory (a may is active at once, S24) or proposed; this reads ruling 3's "on enforced and reviewed" as the common case, with planned added because S24 lets a must enter trial before its binding is proven. Planned needs no date; "planned, no date" replaces the dropped fifth status, and the attention item "planned past zero-by" applies only where a date was set. Reason: the six words stay readable, the screens can paint every state, and demotion has a target that says why. Rejected: a seventh standing for trial (every filter and count would split); a standing "blocked" (it is a property of a binding, and a rule with one blocked and one proven binding is still enforced); keeping the fifth status (it was planned with a flag).

**S33 Excluded, with provenance.** (ruling 4) The observations a project sees are six: as-set, tighter, weaker, absent, unmapped, excluded. Excluded means a rule the chain's parent holds that an overlay below it dropped with a decision; explain names the overlay and the decision; the explorer's here view shows it greyed with the reason. Reason: without the word, explain could only say "absent", which hides a recorded choice. Rejected: excluded as an observation "absent" with a note.

**S34 The bind family.** (ruling 2) The core domain gets its own door: bind (attach a rule to a tool entry, with rung, run point, severity, remedy, pair path), rebind (move a binding to another entry, or change its value, options, severity, rung or run point; in an overlay it may name a parent's rule, and in any rulebook an imported rule, and only tighten, a weakening ends "refused", and the binding record is written under the rulebook's own prefix with the foreign rule's id as its subject; on a rule the rulebook owns it may loosen too, with a decision, S70), unbind (remove a binding, standing re-derived), with one guard for a parent's rule in an overlay and for an imported rule: bind adds a binding and counts as a tightening; unbind and a move to another entry end "refused" (exclude is the door for a parent's rule; for an imported rule it is importing fewer parts); rung and run point do not change (add a binding instead); an options change, or any change that cannot be compared, ends "refused" (options carry no word: the plugin's rank ranks a value at a key and never an options map, and dropping an option can turn a check off, #1554); conflicts (list entry conflicts and offer the three resolutions of S8). bind, rebind and unbind each end in a plan that shows the entry, the standing before and after, the preset diff and the proof to be run; each is a history entry. conflicts is a listing; the resolution it launches is a rebind or a decision, each its own plan. Reason: a maintainer adding a second tool to a rule or resolving a conflict needs a named command, and "edit the rule" hides the core. Rejected: a `rule bind` subcommand (buries the core under lifecycle).

**S35 Import grandfathers.** (ruling 6) When import reads an existing guide and config together, a drafted rule whose binding already existed in the imported config enters active, with origin "imported", the guide itself as its source, and an auto-recorded decision "imported from the guide at its tag"; its missing example pair goes on the attention list and its binding is marked unproven until the pair exists; the preset entry ships because the consumers already ran it. A drafted rule with no existing binding enters proposed and ships in no preset. Trial applies to rules new since import. Reason: the first release of an adopting guide must ship a preset at least as strong as the config it replaces, or adoption is a regression. Rejected: everything proposed (an empty first preset); every imported must through trial (a warning for one release in every consumer).

**S36 Several rules per Markdown block; id apart from anchor.** (ruling 7; C4) A section's block may hold several rules, each with its own id, statement, strength, scope and bindings; the block carries the section's anchor once; the section prose is the shared reasoning. The id is separate from the anchor: a renumbered or renamed section is an anchor change, a history entry and a redirect in renderings, never an id change. Reason: a flagship section's decision paragraph carries several rules, and guides renumber. Rejected: one rule per section (forces a rewrite); id derived from the anchor.

**S37 The editor surface.** (ruling 8) Where a tool lets a preset set a diagnostic's docs URL, it points at the rule page or the guide's anchor. The address comes from the rulebook's published base URL (S72). Where the tool's URL is fixed, the diagnostic keeps the tool's link and explain is the door: explain takes a tool's rule id, a config line or a path. A diagnostic relay or editor extension is an option the architecture may take; the product promises neither. The fast-explain promise has two consumers: the agent hook on every edit and a person at the terminal. Reason: a promise half the tools cannot keep is a lie on the first screen. Rejected: an editor extension in the product (touches the "never" list and binds the product to editors).

**S38 Explorer views and packagings.** (rulings 9 and 19; night rulings 3 and 6) The explorer has these views: rules (filtered, with filters and live counts), rule, relations (the graph around any entity: rule, decision, source, binding, tool entry, part, term), rulebooks (chain, imports, rule maps, archived successors), tools and unmapped entries, coverage, standing bars, health, changelog, terms, attention, here. Twelve views. The here view reads one local report file and exists in the standalone packaging only; the iframe, the web component and the plugins show the rulebook views, and the page says so; the web component takes a view attribute naming any of the eleven rulebook views, so a host mounts one view or several, shows the rules view when none is named, and never offers here, and the plugins mount the same element; a portal that wants a project's report hosts it by its own choice, outside gragas. The explorer is built as a static bundle by one command that ends "done"; the engine serves nothing, and the handbook shows how to open the bundle. In a rulebook before its first release, the build writes a working bundle from the live model, marked unreleased, and every view shows the mark. The explorer stays embeddable: web component, docs and portal plugins, standalone, iframe. Reason: the operator built the first explorer to see entities and their relations, and no view showed the graph; an iframe in a portal cannot reach a developer's local file; and a running server would be the service the Never list refuses. Rejected: relations only inside the rule view; a here view fed by a collector; a here view in the iframe; a local preview server.

**S39 Pin and manifest fields.** (ruling 10) The pin holds: rulebook, version, source (host, path or mirror), kinds, parts, the release's content hash. A package manifest in a monorepo holds: parts, kinds, and an optional name that is off by default (S48). Field meanings are in section 16. Reason: status must list the rules for this kind of program, and init must know whose key to trust. Rejected: kinds only in manifests; per-package pins.

**S40 Exit of check.** (ruling 11) In a project, check ends "done" with exit 0 and lists its findings; `fail-on` names the finding kinds that turn the ending into "findings" with exit 1, and an org puts that in its own gate. In a rulebook, any finding ends "findings" with exit 1, because there the data is gragas's own. "Not ready" (exit 4) covers a project without a pin, an engine below the minimum, a format newer than the engine, and a missing input such as a tool version or a second batch; a plan without a yes is "declined", exit 6 (S43); "refused" (exit 5) covers a content-hash or digest mismatch and a lifecycle change without history. Reason: "overrides are free" must be true for the team that never reads a flag. Rejected: exit 1 by default (adding check to a required workflow would block on any override).

**S41 Prose facts are presets.** (ruling 12) "A MUST line without an id" and every other fact over a portal's or a guide's prose is a rulebook preset for a prose tool, which the portal runs like any project; the rulebook supplies the rule, its id list and the preset; gragas never reads the portal's pages. Reason: the operator's ruling moved code facts out of the engine, and a prose fact over another repo's files is the same thing. Rejected: an engine check over the portal's Markdown (judging a project's files).

**S42 Copied presets are the project's.** (ruling 13) Where a tool has no extends, init copies the preset once because the project asked; from then on the project owns the file, the lock holds the digest of the copy as shipped, explain shows every edited line as an override with provenance "edited copy", check reports "edited copy" as a fidelity fact, the copy lives in a folder gragas owns in the project, one subfolder per tool, and init and upgrade print the flag or setting that points the tool at it (printed, never written), and the upgrade plan shows a three-way diff (shipped before, shipped now, the project's file) and writes nothing without a yes. Reason: it matches "overrides are free" and "a project's config is the effective config". Rejected: a generated file with marked sections gragas owns (two owners in one file).

**S43 Plans and the yes.** (ruling 20; A9) The commands that change something end in a plan: init, import, upgrade, switch, the rule family (exclude and include among them), bind, rebind and unbind, the term, source and decision families, undo, propose, migrate, release, archive, withdraw, waive, baseline, fix, and agents and review in their opt-in form, and docs in its accept form (S74); conflicts is a listing whose resolutions are a rebind or a decision, each a plan of its own. A plan has the same sections everywhere: what changes (rules, bindings, tool entries), consumer actions, what is written where (files named), what is re-keyed or orphaned (waivers, baselines), renderings rewritten (agent files, review files, copied presets), checker-code changes with a diff link, the standing before and after, and the question. A yes is recorded: in a rulebook as a history entry, in a project as a lock entry, each with who, when and the plan's digest. A script passes a yes flag; the flag has one spelling for every command, given in the handbook, and the word on screen and in the record is "yes"; without a terminal and without the flag, the command prints the plan and ends "declined", exit 6. A no ends "declined" and changes nothing. withdraw, archive, switch and the opt-out forms of agents and review also ask for the thing's own name, typed at a terminal or given as a flag, because they are hard to take back. There is no apply command: the command that made the plan applies it. Reason: one plan shape makes every changing screen the same screen, and a recorded yes is what "nothing without a yes" means. Rejected: a separate apply command (a second step nobody records); a silent yes in scripts.

**S44 Verify at the binding's path.** (B6) A binding declares the representative path its example pair stands at, so a tool's path-scoped config applies to it. Proof places the pair at that path inside the isolated or sandboxed run; verify mirrors the project's config set into the run, gives the tool read-only access to the project's own dependencies where the gragas plugin's grant names them (that is where the config's plugins live, S23), and places each pair at the same path, so what verify proves is the project's effective config, not the rulebook's. Nothing is written under the project's tree. Reason: effective config is per path, and a fixture at the wrong path proves nothing. Rejected: fixtures under the project tree (writes into a project); verify over the rulebook's config only (proves the wrong thing).

**S45 Signals over batches.** (B8) The report carries the release it was computed against and its run time, an input to the report (S57). The org hands over batches; signals compares two batches and prints the change per rule; the contract lets the collector attach an opaque key outside the document so one project is not counted twice. A batch is a folder of report files, one per project; any key the collector uses to tell projects apart sits outside the documents, in the file name or beside them. signals and attention take two folder paths and order them by the run times the reports carry (S57), so the order the paths are given in does not matter. gragas stores no batch. Reason: a "sudden change" needs two points in time, and a rate needs one report per project; both can come from the org without gragas knowing a project. Rejected: a store inside gragas; a project identifier in the report.

**S46 Plugins have versions.** (B9) A plugin carries its own version and the tool version range it describes; a rulebook's tested set pins plugin versions beside tool versions; the bundle records the plugins used by version (in release 1 under the name `adapters_used`, gragas#2071); a rulebook may carry a plugin of its own in its plugins/ folder, and a project may install one; the engine's public change process covers the plugin interface, and a tool's community may maintain each plugin. A new tool arrives as a new plugin, with no change to the engine, which names no tool (plugin-interface). Reason: the plugin is where tool churn lands, and an unversioned plugin would make a proof grid unreproducible. Rejected: plugins versioned with the engine binary only. This was S46's "adapters" before plugins were named so; the number stays.

**S47 The unit of coverage for upstream presets.** (B12) Coverage counts an entry the rulebook sets itself as one entry, and an upstream preset adopted whole at rung 0 as one covered set under its rung-0 rule; the coverage view folds the upstream entries under that rule, expandable; an upstream entry the preset adds after the pin shows as unmapped until the rulebook maps or re-adopts it. Reason: a hundred upstream entries would swamp the health number and tell the reader nothing. Rejected: a rule per upstream entry; counting upstream entries as unmapped.

**S48 Report sections keyed by digest.** (B13) In a monorepo the report's package sections are keyed by the manifest's digest; the package name is an optional field, off by default; the here view shows names from the local tree only. Reason: package names identify a monorepo and its teams, and the report promises no identifier. Rejected: names on by default.

**S49 A placement per field in the docs profile.** (B14) A docs profile declares, per exported field, where the host takes it: front matter, body, or omitted; the rendering manifest records every omission as a finding the portal owner sees once and may accept. The profile for a Markdown docs platform with typed front matter (domain 6 lists its placements) omits nothing, so its manifest carries the placement and no omission, and the portal owner accepts it once. Reason: a host whose schema holds no field for strength or sources must not silently lose them. Rejected: refuse to export to a strict host; drop fields silently.

**S50 The companion rulebook, the runnable step.** (C1) A companion rulebook is a base built over a public guide its owner does not control: it cites the guide's sections by anchor, never copies prose, binds the guide's rules to presets, and publishes coverage and a proof grid, with no change to the guide's repo. It is a named kind beside fork and archived, and the step the owner can run before the success test. Reason: the two named flagships are the two least able to accept outside changes; the runnable test must not depend on them. Rejected: no named case (the success test stays unrunnable); copying the guide's prose (a fork with a licence problem).

**S51 The review question is the rung-6 key.** (C3) A rung-6 binding's key is the review question the reviewer must answer, and its value says when it applies. The rule view and the review file render both. Reason: a review-held rule without a question is an instruction nobody can act on. Rejected: a free-text note.

**S52 Licence per kind.** (C5) A rulebook declares its licence per kind of content: prose, presets, checker code; attribution carries the right one into every rendering. Apache-2.0 for the engine; Apache-2.0 for the rulebook's own content unless its owner declares another per kind. Reason: a flagship's prose and code already carry two licences. Rejected: one licence per rulebook.

**S53 Trial where a tool has no warning level.** (C6) Where the bound tool has no warning severity (compilers, policy facts), the trial binding ships at the tool's only severity, and the changelog's consumer action says "trial: this tool has no warning level, so it fails as an error here". Reason: a trial's purpose is signals from real use, and an entry that ships nowhere produces none. Where the tool keeps severity in a table of overrides keyed by entry instead of a level on each entry, it has a warning level for this purpose: the preset puts the trial binding's entry at warning in that table during trial and at its own severity after, and the plugin's manifest says which of the two forms the tool has. Rejected: ship the binding off during trial (no signals, and a surprise at promotion).

**S54 Health for an overlay.** (C7) An overlay publishes two labelled health sets: its own rules, and the resolved chain it presents to its projects. Reason: both questions are asked, by different readers. Rejected: own rules only.

**S55 The clock.** (C8) Attention, review-by, waiver expiry, trial windows and zero-by are computed on demand from the current date when a command runs; gragas keeps no timer. The handbook shows a scheduled run of the rulebook's own check as the org's job, beside the dependency bot. Reason: a rulebook with no pushes still has overdue dates, and a schedule inside gragas is on the "never" list. Rejected: a built-in schedule.

**S56 The names check and terms.** (C9) A term may carry a literal (a header name, a cookie name, a package); a statement uses the term; the names check allows a rulebook's own terms and still refuses a bare tool, org or project name. Reason: an org overlay's rules are about the org's names, and the statement must stay neutral and readable. Rejected: an allow-list in the names check.

**S57 Determinism and dates.** (C10) Derived documents (bundle, pages, report, changelog) carry no clock reading except two inputs: the release date, and the report's run time, which signals needs to order batches (S45); history entries carry their dates as authored inputs; proof timestamps stay in the gate's log. Reason: byte-identical output from identical inputs is the promise, and the run time is one of the report's inputs. Rejected: a generated-at field on the bundle or pages.

**S58 Accessibility per packaging.** (C11) The WCAG 2.2 AA promise is made for the standalone and iframe packagings; the web component states the hosts it was tested in, and the docs and portal plugins inherit the host's promise for chrome and the explorer's for content. Reason: focus and name references inside a host's own page cannot be promised for every host. Rejected: one promise for every packaging.

**S59 Prose is a file edit; lifecycle goes through the CLI; the CLI also corrects.** (the operator's notes) Prose fields are the statement's wording, the reasoning, example text, guide prose, chapters, decision text, term definitions and translations: a maintainer or an agent edits them in the file, and the changelog shows "explanation changed" or "statement reworded" with a consumer action of none. Lifecycle fields are id and aliases, status, strength, scope and its exemptions, owner, review-by, zero-by, relations, origin, replaced-by, the sources list, the bindings, an overlay's exclusions and history: they change only through the CLI (exemptions and zero-by through rule edit), and the guard holds that in the rulebook's repo. A change of a statement's meaning is `rule replace`, never a reword. The CLI also corrects: edit (a lifecycle field), fix (the menu over the check's findings), restore (a retired rule returns, its history intact), supersede (a decision replaced by another, the old one kept), undo (the last history entry reverted by a new entry); every correction is itself a history entry, and nothing is deleted. Reason: prose through commands would make agents useless at the work they are good at, and a correction that erased history would make the history a lie. Rejected: all fields through the CLI; deletion.

**S60 Baselines: the tool's own file where one exists.** Where a tool has its own baseline or bulk-suppression file, that file is the baseline: gragas reads it, counts it, reports its growth and never writes it. gragas's own baseline format exists only for tools with none. Reason: a second baseline beside the tool's would drift. Rejected: a gragas baseline for every tool.

**S61 Kinds only.** A kind is any named trait of a program a rulebook defines, and a project declares as many as apply in its pin or manifest; rules are scoped by part, path and kind. "Facets" as a second axis are dropped. Reason: two scoping axes with one meaning double every filter and form; several kinds per project give the same expressive power. Rejected: facets kept as yes/no traits. A trait that switches rules off is a kind too (S73).

**S62 Renumber keeps aliases.** A renumbered rule gets its new id and keeps every old id as an alias forever, in the rulebook and in the bundle; every rendering, waiver, baseline and rule map resolves aliases; the history entry records the change. Reason: an id that disappears breaks every link, waiver and report outside the rulebook's control. Rejected: renumber as a bare rename (a migration for every consumer).

**S63 Domain 8 in the subdomain map; the terminal across all.** Status, check, waive, baselines, manifests and the report sit with pin, lock, init and the two plans in the subdomain "composition, distribution and the project". The terminal belongs to no subdomain and is a shared shell in the architecture. Reason: every capability needs one home for the bounded-context map. Rejected: a subdomain of its own for the project (it has no model of its own; it reads the core's).

**S64 A rulebook upgrades its own pins.** (night ruling 1) upgrade runs in both contexts; in a repository that is both a rulebook and a project it asks "the project's pin or the rulebook's pins?" or takes the subject as an argument, and the menu lists it in both groups. In a rulebook it reads a changed or added pin where the pin lives (the identity file for the parent and imports, S69; the rung-0 binding or the flattened set for an upstream preset), fetches the release and the withdrawn list, verifies the release against the content hash identity holds for that parent or import, and plans: per changed parent or import rule, the consumer action for this rulebook; the overlay's tightenings now redundant; its exclusions now conflicting (the parent retired or replaced the rule); new upstream entries to map or re-adopt; the proof to re-run; the bump the next release will compute. The yes writes the history entry and the regenerated flattened set; the dependency bot may change the pin where it lives and open the pull request. A pin changed without its upgrade entry is a validation finding, and the rulebook's state is "behind its pins" until upgrade runs. A base with no parent, imports or upstream pins ends "done, nothing to upgrade". Reason: without it an overlay stops at the first parent release, and S5's upstream watch had no command. Rejected: hand-editing identity as the whole act (the edit moves the pin, and upgrade still owes the plan and the history); a separate command (one word for one act in both contexts).

**S65 propose from a project.** (night ruling 2) propose runs in both contexts; in a repository that is both, it asks which subject or takes it as an argument. The proposal is a contract document (section 9). Its target is the rulebook whose prefix owns the rule, or, for a new rule, the parent (from an overlay) or the pinned rulebook (from a project); the document carries the structured change (anatomy, pair, draft binding, sources) and, from a project, the project's evidence (a waiver pattern, an incident's example, an agent's repeated miss). Where the target's governance accepts one, propose prints the proposal document and the line that opens the pull request or issue carrying it with the host's own tool (the engine talks to no host's service), and the target's owner applies it with rule new, bind or import in the target's own source form; where governance is closed it prints the recorded proposal. In an overlay the proposal is also recorded in the proposals file; in a project nothing is written but the plan's lock entry and the printed document. A base that is not a project ends "not ready, no parent and no pin". Reason: a contributor to a public guide, an incident and a repeated agent miss all live in projects, and a public guide's contributors have no overlay. Rejected: proposals only through an overlay.

**S66 The state file.** (night ruling 5) Per-machine facts live in a local state file the handbook says to keep out of version control, and init prints its ignore line with the wiring: tool versions found, last fetch, the withdrawn list as seen. A project has one for its pinned release; a rulebook has one too, for its parent, import and upstream fetches. When the file is missing or older than the age the handbook names, status and check fetch; where they cannot, they print "withdrawn: not checked on this machine" and end done. The lock keeps what a yes decided, what the release fixed, and the sums of the renderings a yes allowed: the resolved release, digests, copied presets, checker code, gragas-plugin versions, opted-in folders and reviewer format, rendering sums, yes entries, provenance. Reason: a lock rewritten by every developer's machine on every run buries the yes entries in noise. Rejected: everything in the lock; everything in the state file (the yes must be shared).

**S67 One door for a new rulebook.** (night ruling 8) init runs in any context. In the "neither" context it asks: a project or a rulebook? In a pinned project it ends "not ready, already pinned" and offers the opt-in forms of agents and review; in an existing rulebook, "not ready, already a rulebook". The project form is S39's. The rulebook form asks name, kind (base; overlay with its parent at a pin and the parent's content hash; companion with the guide it cites; fork with its origin recorded), imports with their content hashes, the starting version (S68), prefix, authoritative language, licences per kind, owners, governance, the first layers and parts, and the source form (structured files or the guide's own text); the plan shows the identity and structure files it will write, the guard's hook line and the gate's rule it will print, and the first commands to run; the yes is the first history entry. Reason: the first screen a flagship maintainer sees must be the product's, not a hand-written file, and the guard needs an install step. Rejected: a separate command for rulebooks; a template repository alone.

**S68 The first release takes the declared version.** init's rulebook form records a starting version in identity (the form offers 1.0.0; a fork, or a guide that already numbers its releases, declares its own), and the yes writes it as part of the first history entry. The first release has no earlier tag to diff against, so it publishes that version unchanged and its changelog reads "first release"; every release after it computes its bump from the diff against the last tag, and nobody types a version again. Reason: "never typed by hand" needs a starting point, and a guide that already ships numbered versions must not restart. Rejected: computing a first bump from nothing (there is no diff, so any answer is invented); a fixed 0.0.0 start (a fork would renumber its past); asking for the version at each release (the bump would be typed).

**S69 Adding an import after init.** An import added after init is an identity edit followed by upgrade, the same path as a moved pin (S64). The maintainer adds the import to identity (rulebook, parts, pin, content hash) as a plain file edit; upgrade in the rulebook fetches the release, verifies it against the hash just written, and plans: the imported rules the rulebook now carries (ids namespaced), entry conflicts with its own bindings and with other imports (S8), checker code only where the importer declares it by name and publisher (S3), the proof to re-run and the bump the next release will compute. The yes writes the history entry. Until it runs, validate lists "pin changed without its upgrade entry" (an added import is a pin changed from nothing) and the rulebook's state is "behind its pins". The command set stays at fifty-four. Reason: S64 already makes upgrade the one door for what a pin brings in, and a pin is the same fact whether new or moved. Rejected: an `import add` subcommand (a new command on a command whose job is drafting rules from configs, guides and prose); editing identity with no plan (the rules would arrive with no history entry and no conflict check).

**S70 Who a rebind may loosen.** The guard of S34 compares a rebind with the rule's owner. On a rule a rulebook borrowed, its parent's or an import's, rebind may only tighten: a stricter value or severity, with the options unchanged. A weakening, a move to another entry and a rung change end "refused"; unbind ends "refused" too, and an importer drops an imported rule by importing fewer parts. The record is written under the borrower's own prefix with the borrowed rule's id as its subject. For an import this is S8's second resolution (rebind one rule) applied across an import (S3): the importer tightens the weaker of two conflicting entries, and where it wants the weaker value it splits the entry or declares a `conflicts` relation and records a decision naming the winner. On a rule the rulebook owns, rebind may loosen as well as tighten, and a loosening needs a decision; the next release computes the bump from the diff as always, major when a must is weakened (S11). Reason: the guard protects what a rulebook borrowed, not what it wrote; an owner who could not loosen its own rule would edit by hand and lose the history. Rejected: refusing every loosening (the owner edits around the guard); letting an importer weaken an import (the import contract would be weaker than the overlay contract).

**S71 Closed, redundant and orphaned waivers.** A waiver is never deleted; its states are active, expired, orphaned and closed. After a switch the plan lists each waiver it re-keys to a target rule and each it cannot; one it cannot re-key stays in the waivers file marked orphaned, and from then on check lists "orphaned waiver" as a finding of its own (exit 0 unless fail-on names it) until a person closes it or re-keys it. Re-keying by hand is an edit of the waivers file, which is the project's own; fix offers to close an orphaned waiver, as a plan. An upgrade lists a waiver as redundant when the release's own diff of the rule makes it so: the rule retired or replaced, its scope or a scope exception now excluding the waived path, or the binding loosened to what the waiver allowed. gragas never runs the tool to find out (S1). The plan carries each redundant waiver as one line, answered inside the yes; closing writes who and when into the waiver, and the record stays, marked closed. Reason: the waiver is the team's evidence of a decision, and deleting it makes any audit afterwards impossible; a waiver with no rule left must show, not vanish. Rejected: deleting redundant waivers on the yes (no record); re-keying orphans by guess (a waiver on the wrong rule is worse than none); judging redundancy by running the tool over the waived path (judging a project's code); a separate command for waiver housekeeping (fix and the plan are the door).

**S72 The docs base URL.** Identity carries a published base URL for the rulebook's pages and, optionally, one for an explorer the owner hosts. From the first, the docs profile's URL pattern gives every rule's page and every guide anchor; the preset sets the docs URL on the rulebook's own checker-code rules to that page where the tool lets a rule carry one (S37); the terminal prints a rule's page link, and its explorer link where the second is set, beside every explain and show. A rulebook with no base URL prints ids only and sets no link. The engine never guesses an address from a hosting service or a repository name, and a base URL promises nothing about serving: the explorer has no address of its own (S38), and the link is only the owner's own copy. Reason: S37 promises a docs link, and a link needs an address only the owner knows; explain is the door with or without one. Rejected: deriving the address from the repository host (wrong for every private mirror and custom domain); a project-level setting (every project would repeat the rulebook's own fact); the engine hosting pages (a service).

**S73 Facets are kinds.** (changes S61 where it was silent) A trait a rulebook uses to switch rules off, such as "has no database", is a kind like any other: the rulebook defines it in its structure, a rule's scope and exemptions name it, and a project lists it in its pin with the rest (16.2). There is no second field. When a rulebook moves from a format that kept a separate list of such traits, migrate turns each one into a kind of the same name, and its plan lists them. Reason: S61 already gives one scoping axis with the full expressive power, and a second field in the pin would bring the second axis back. Rejected: a facets field in the pin (two axes with one meaning, double the filters and forms); a facets file beside the pin (a second place to say what a project is).

**S74 Accepting a host's omissions is a plan.** (S49, S43) The first docs run against a host whose profile omits a field writes the manifest with each omission listed as not accepted and ends "findings". docs in its accept form shows the omitted fields as a plan and asks once. A yes records, beside each accepted omission in the manifest, who accepted, when and the plan's digest, and in a project the yes also enters the lock; the next docs run ends "done". An acceptance holds only for the omissions it listed: a field the profile newly omits after an upgrade is a finding again. A hand edit of the manifest is not an acceptance. Reason: S43 says every change goes through a plan with a recorded yes, and an omission is a fact the host cannot show. Rejected: editing the manifest's accepted list by hand (no yes, no who, no digest, in a file the engine owns); a flag on every docs run (a silent acceptance nobody recorded); a waiver (waivers record a tool finding in a project).

**S75 The release note.** (S11, S68) A release carries one release note in plain words, and each changelog entry carries one plain sentence beside its change kind and consumer action. The plan proposes both from the diff; a person rewrites them in the plan's form, or from a file named by the handbook's flag, before the yes; the yes fixes the text into the plan's digest, the bundle's changelog and the history entry. The words name no file, function or internal id. The bump is still computed and never typed (S11). Reason: a consumer reads the note before upgrading, and a note no person wrote tells them nothing; the engine's own releases follow the same rule. Rejected: text generated with no person to rewrite it; a changelog of change kinds only (no sentence to read); a note written after the tag (it would not be inside the pinned release).

## 12. Positions taken against earlier inputs

1. On the boundary line: "gragas runs no tool in a project" is the wrong sentence; the honest line is "gragas never judges your code, and blocks nothing in a project", and any tool it invokes takes the rulebook's fixtures as input (S1, S10, S23).
2. On composition: one chain with no imports forced a fork on any org wanting parts from two public guides; namespaced imports add no ambiguity (S3).
3. On the site: the explorer is a domain, shares a render model with the export, shows the entity graph and a project's report (S16, S38).
4. On "never aggregates": signals over report batches the org hands over, storing and naming nothing, is not project knowledge; without it the lifecycle has no learn stage (S13, S29, S45).
5. On fixed agent budgets: budgets are policy; the rulebook sets them, gragas holds them (S17).
6. On the frame's nine domains: ten and the terminal across all; the explorer and the contract each need an owner, a version and a page. product-frame.md lists ten.
7. On the coordinator's subdomain map: explain, verify and chain resolution are domain 3's; the core domain is 3 and 4 whole (S21).
8. On the earlier "prose cannot drift from its record": the record sits beside the prose by design, and a change to either is in the changelog; the promise is visibility, not impossibility (S2, S59).
9. On the earlier unqualified sandbox: two named levels, because a scratch directory cannot deny a socket (S23).
10. A caution: the success test depends on S2, S3, S5, S11, S20, S23, S25 and S35 together: a maintainer adopts when the guide keeps its prose, URLs, licences and governance, its first release ships a preset no weaker than its old config, consumers keep their tools and trust what they load, and the format can outlive the engine.

## 13. The product page

Nine sections, each opening with its one-sentence answer in bold and at least one captioned diagram; long tables fold into details.

1. **What gragas is.** "A style guide that knows how it is enforced, and explains it to people and agents." Main picture: the six kinds and how they relate (a public style guide, the rulebook, your project, your tools, the engine, your portal, editor and agent), stacked on a phone. Under it: Google's half and Vercel's half joined by the rule, as a card strip (the only two named guides); the four verbs; "tools enforce, your gate decides".
2. **The core domain and the domains around it.** "The core domain is enforcement truth; supporting domains make it usable; generic is adopted, never built." Diagrams: the three rings, with proof and entry conflicts in the core ring and the base rulebook drawn outside the rings as the first customer; the ten domains around the rulebook, core marked as 3 and 4, with the dependency arrows of section 14. The vocabulary strip, the loosening words included. Stewardship: the open format, the public change process, the compatibility promise, the maintainers.
3. **One rule, many tools.** "Rules and tool entries are many-to-many; standing is derived from proven bindings; the rule outlives the tool." Diagram: the binding web with rungs, one real tool entry per example, and a rung-6 example that no linter also holds; the ladder; coverage both ways with the upstream unit. Entry conflicts and the blocked mark; proof at rung 6 shows "reviewed" and the pair is an eval case; the standing palette with its symbols.
4. **Your config, explained.** "Your own config file stays the source of truth; gragas reads what the tool really runs and explains every line." Diagram: rule → preset → override → explanation, with the copied preset as the one file the project owns outright; the six observations; check reports and your gate decides; explain is fast for the agent hook and the terminal; monorepos.
5. **Rulebooks, overlays and releases.** "One engine, many rulebooks: extend one, import many; a release carries a computed version, a tested set, a content hash and a consumer action per change." Captioned diagram: the chain, imports, the companion, the release, the withdrawn list, the index. Trust: who publishes checker code, how it is reviewed, what a consumer trusts, the two run levels, a compromised rulebook. Switching: the rule map and the switch plan, with the preset-diff fallback.
6. **The lifecycle.** "gragas acts at every stage but operate, executes none, and the loop closes at learn." Diagram: the spine with the verb per stage and every feedback named. Rule quality: required fields with the own decision as a source, the prose lint, trial before must, the re-check at review-by. Import with grandfathering: a guide joins and its first release ships a preset.
7. **People and agents.** "Everyone reads where they already read." Diagram: one explanation, its renderings, the three output layers; the diagnostic's link where the tool allows, explain otherwise. Languages: translations and the authoritative language. The contract, with the history entry among its ids and the deprecation window stated as two minors before the major.
8. **The ecosystem.** "Thirty-six kinds of thing touch a rule; gragas is four of them." Diagram: the grouped map of section 5, with project as a role. Private and air-gapped hosting.
9. **The boundary.** "gragas makes the rulebook shine; it never judges, blocks in a project, stores or knows." Captioned diagram: the boundary as two boxes, engine and rulebook, with the project's acceptance step between. The owns/uses/never table; how the product measures itself; the quality attributes.

**Readability rules.** About 250 words of prose per section outside captions, tables and details: the answer, the diagram, then short paragraphs or cards. Tables for lists over four items. No command names, file names, schema fields or tool syntax, except one real tool entry per example in a figure: the page names capabilities, the architecture names mechanics. Vocabulary defined once, in the strip under section 2. Only Google and Vercel are named among guides. Other products appear as "uses" or "writes into", never compared; no business-model framing. No release-stage or cost words. Ten minutes to read.

**Page contract.** The page's build rules (layout, theme, gutters, figure overflow, the phone breakpoint) live in the page generator's header, not here. Two rules are the product's: every figure reads in light and dark, and the forbidden-name grep is at zero.

## 14. The domains in detail

One card per domain, and one for the terminal. "Context" says where a command runs: a rulebook's repo, a project, or both. Files are named by what they hold, never by path or syntax. Closing words and exit codes follow section 15.4. "Never" lists what the domain refuses to become.

### Domain 1: Format

- **Purpose.** Define how a rulebook is written, and tell its owner when it is not written that way.
- **Owns.** The format spec and its versions; the rulebook's identity (name, version, licences per kind, authoritative language, extends and imports each with a content hash, owners, governance, tested set, format version, the published base URL for pages and optionally for an explorer the owner hosts, S72); structure (layers, parts, concerns, kinds); guides and chapters as prose; the rule's shape and required fields, exemptions among them; the two source forms and the block; ids and aliases; the overlay and import contracts; translations and their stale hash; the names check and the prose lint; schemas per format version; migration forward; the rulebook door of init. Domain 1 owns the format of every rulebook file; it does not own the entities inside them (section 16.1). Files in a rulebook: the identity file, the structure file, the rule files or the guides with blocks, the term file, the translation files.
- **Commands.** validate (rulebook): a listing of findings and notices. migrate (rulebook): a plan that moves the rulebook to the next format version. init (the "neither" context, when the answer is "a rulebook"): a plan that writes identity and structure from a form, prints the guard's hook line and the gate's rule, and makes the first history entry (S67).
- **Reads and hands over.** Reads the rulebook's files. Hands the parsed model (identity, structure, rules, terms, translations, exemptions, contracts, budgets) to 2, 3, 4, 5, 6 and 9; the rest read the bundle. Hands validation findings to the terminal and, as SARIF, to the contract.
- **Invariants.** One id means one rule forever; an alias never points at two rules. Every rule file parses to the same model whichever source form it uses. A statement passes the prose lint before a rule leaves proposed. A translation always names the hash it translated. An overlay never defines a rule under a parent's id; its tightening records, and an importer's, name a parent's or an import's id only as their subject. The format version a rulebook declares is one the engine reads, or the engine says "not ready" and names both versions.
- **States.** Format version: current, previous major (read and migrated forward), newer than the engine (not ready). Translation: current, stale. Rule shape: complete, incomplete (which fields are missing). Pins: current, behind its pins (a pin changed or added without its upgrade entry).
- **Findings.** Unknown field, missing required field, id clash, alias clash, broken link, undefined term, names check hit, prose lint hit, exemption without a reason, overlay contract breach (a parent id edited in place), import of an unknown part, format version mismatch, a pin changed without its upgrade entry (an added import is a pin changed from nothing, S69). Notice: a stale translation.
- **Closing words.** validate: done, or findings (exit 1 in a rulebook); notices never change the word. migrate: done after a yes, declined, or not ready when the engine cannot read the version. init (rulebook form): done after a yes, declined.
- **Empty and error states.** A rulebook with zero rules validates "done, 0 rules, 0 findings" and says what to run next (rule new, or import). A file the engine cannot parse is a finding naming the line. A rulebook whose format is newer than the engine ends "not ready" with both versions and the index's record.
- **Never.** Reads a project's source; judges prose beyond the statement; writes prose; holds tool syntax outside a binding's key and options.

### Domain 2: Authoring and lifecycle

- **Purpose.** Be the only path by which a rule's life changes, and keep every change with its reason.
- **Owns.** The rule family, the term, source and decision families, promote, confirm, restore, renumber, replace, retire, exclude, include, undo; the append-only history; the guard (gate rule and write-time hook, in the rulebook's repo); import with grandfathering; propose; the rulebook side of upgrade (its own pins); signals; the attention list; the fix menu. Entities: the rule, term, source, decision, history entry, proposal, exclusion. Files in a rulebook: the history file, the sources file, the decisions file, the proposals file, the exclusion records of an overlay, the term file (shared with domain 1).
- **Commands.** rule new | edit | replace | retire | restore | renumber | promote | confirm | exclude | include (rulebook; each a plan; exclude and include only from an overlay, each with a decision). term new | edit | retire (rulebook; plan). source new | edit (rulebook; plan). decision new | supersede (rulebook; plan). undo (rulebook; plan). import (rulebook; plan). propose (both; plan, then the proposal document printed with the line that opens it on the target's host, or a recorded proposal; from an overlay to its parent, from a project to its pinned rulebook). guard (rulebook; called by the hook line and the gate rule; a listing of the lifecycle fields a pending change touches without a history entry, ending "refused", or "done" when there are none). check (rulebook side; the gate listing of S14; any finding ends "findings"). upgrade (rulebook side; plan over the parent, import and upstream pins, S64; it is also how an import added after init arrives, S69). fix (both; plan). signals (rulebook; a listing over two batches). attention (rulebook; a listing; takes the two batches as an optional input). history (rulebook; a listing).
- **Reads and hands over.** Reads the model from domain 1 and the derived truth from domain 3 (standing, marks, conflicts; proof through the marks) to build attention and the fix menu; reads report batches the org hands over for signals and, when given, for attention's override rate; for the rulebook's own upgrade, reads the parent's, imports' and upstream releases at their pins and their withdrawn lists (5), verifying each against the content hash identity holds for it. Hands status, trial state, owners, review-by, zero-by, exemptions, exclusions and sources to domain 3 (standing needs status), domain 5 (the changelog needs history) and domain 6 (pages need history and decisions); hands proposals, as contract documents, to the person, with the line that opens them on the target rulebook's host.
- **Invariants.** Every lifecycle change is one history entry, written by the CLI, with who, when, the plan's digest and the decision where one is required. Nothing is deleted; retire, restore, supersede and undo add entries. A rule leaves proposed only with the fields S24 requires. A must or should passes through trial unless grandfathered or restored. History is append-only and the gate refuses a change without its entry. Signals store nothing.
- **States and transitions.** Rule status: proposed → trial (promote, must and should) → active (promote, after at least one release and within the window) → retired (retire, or replace); proposed → active (promote, may; or grandfathered import); retired → active (restore); any active → the same with a new review-by (confirm); a parent rule, in an overlay → excluded for that overlay (exclude, with a decision) → included again (include, with a decision). Decision: made → superseded. Proposal: open → accepted, declined or withdrawn. Term: defined → retired (only when no rule uses it).
- **Findings.** Missing source, decision or pair at promotion; overdue review-by; trial past its window; a lifecycle field edited outside the CLI (the guard); a history gap; a decision required and absent; an import with unresolved to-dos; high override rate from signals; a rule a linter now holds still in review text (known from the last release's renderings, S13).
- **Closing words.** rule and term: done after a yes, declined, refused when the guard would be bypassed. source and decision: done after a yes, declined, refused when the record is not valid or the plan cannot be made. import, propose, fix: done after a yes, declined. undo: done after a yes, declined, refused when the entry has no inverse or a field has changed since, not ready with no history. guard: done, or refused naming the fields. check in a rulebook: done, or findings. upgrade in a rulebook: done after a yes, declined, refused on a content hash mismatch. attention and history: done. signals: done, or not ready when fewer than two batches are given or a folder holds no report.
- **Empty and error states.** attention on a rulebook where every rule is proposed lists the whole book, grouped by what is missing, and says so in one line first; without batches it says "override rate: no batches given". import on an empty config and no guide ends "done, nothing to import". upgrade in a base with no parent, imports or upstream pins ends "done, nothing to upgrade". propose in a base that is not a project ends "not ready, no parent and no pin". undo with no history ends "not ready". propose against a parent whose governance is closed records the proposal and says the parent accepts none.
- **Never.** Edits prose once written (rule new and import seed it; statement wording, reasoning, examples, guide text, chapters, decision text, definitions, translations are file edits from then on); agrees a rule; retires by calendar; counts installs.

### Domain 3: Enforcement link (core)

- **Purpose.** Join each rule to the exact tool entries that hold it, derive what that makes true, and read a project's config back into rules.
- **Owns.** The plugin interface, and plugins as installed and approved code (a plugin is never built into the engine); bindings; rungs and run points; tool entries; entry conflicts; coverage both ways; standing and marks; preset generation from bindings; the declaration of checker code by name and publisher (release computes its digests, domain 5); the copied-preset contract; upstream presets pinned and flattened; kits as package bindings; chain resolution with provenance; the effective config read and its observations; explain; verify. Files in a rulebook: the binding records (inside the rule files or beside them), the plugins of its own, the generated preset data, the flattened upstream sets, the checker-code declarations.
- **Commands.** bind | rebind | unbind (rulebook; plan). conflicts (rulebook; a listing that offers the three resolutions). coverage (rulebook; a listing). tools (both; a listing of tools, entries and unmapped entries). plugin (both; `plugin add` installs by digest, `approve` shows the grants and records the approval on this machine, `list` lists the plugins with versions and whether each is approved, `contract` prints the interface's contract, `test` proves a plugin with 24 conformance checks, `record` runs the real tool through an installed plugin and files the recording and the case in the author's folder). resolve (both; the effective rulebook of a chain with provenance, as a listing or document). explain (both; in a project a config line, a tool's rule id or a path → rules, observations, provenance; in a rulebook a rule → its entries and presets). verify (project; a report of the pairs run over this project's effective config).
- **Reads and hands over.** Reads the model (1), status and trial state (2), proof results (4), the pin, lock, manifests and the project's tool configs (8), and the parent's or the pinned release at its pin (5) when it resolves a chain. Hands standing, marks, coverage, conflicts and the resolved chain to 2 (attention), 5 (release, health, changelog), 6 (renderings), 8 (status, check, report), 9 (which rules no tool holds); the explorer sees all of it through the bundle; hands presets to 5 to ship; hands the pair placement to 4.
- **Invariants.** Standing is derived from effective bindings and never typed. Every generated preset entry traces to exactly one binding set, and every binding at rung 0–5 names an entry a gragas plugin reports. A conflicting entry is never generated. An entry a project edits is observed, never refused. The chain resolves to one effective rulebook, and every entry in it carries the layer that set, changed or excluded it. Checker code is declared by name and publisher, digested at release, and loaded only in an isolated or sandboxed run.
- **States.** Binding: declared, proven, unproven (missing, failing, not run), blocked. Entry (in a project): as-set, tighter, weaker, absent, unmapped, excluded. Plugin: current, outside its tool range, and installed or not approved.
- **Findings.** Entry conflict; a rule with no binding (planned) past its zero-by; an unmapped entry; an entry outside the plugin's tool range; a binding to a tool with no plugin; a plugin that is not approved on this machine; a plugin step outside its grants; checker code without a declaration (the digest finding is release's); a copied preset edited (in a project); an import binding conflict; in an overlay, or on an imported rule, a bind, rebind or unbind on a parent's or an import's rule that weakens, moves or cannot be compared.
- **Closing words.** bind, rebind, unbind: done after a yes, declined, refused when an overlay's rebind would weaken a parent, or any rebind an imported rule. conflicts: done, or findings when any are open. coverage, tools, explain, plugin list and plugin contract: done (the plugin family arrives with its commands). plugin add and plugin approve: done after a yes, or refused when the digest is not the lock's. resolve: done, or not ready when a layer cannot be fetched. verify: done, or findings when a pair fails here, or not ready when a tool version is missing.
- **Empty and error states.** coverage on a rulebook with no bindings reads "0 rules held, 0 entries explained" and offers bind. explain on a line no plugin knows says "unmapped: no rule explains this entry" and names the tool. conflicts with none ends "done, no conflicts". verify without the tool on the path ends "not ready" and names the version the tested set wants. resolve on a broken chain (a parent pin that cannot be fetched) ends "not ready" and names the layer.
- **Never.** Runs a tool over a project's source; writes a project's tool config (it generates the rulebook's presets and reads the project's files); picks a winner in a conflict; installs a tool.

### Domain 4: Proof (core)

- **Purpose.** Make every binding prove itself with the rule's own examples, on every tool version the rulebook claims.
- **Owns.** Example pairs; the proof run, isolated or sandboxed; the proof grid (binding × tool version, with result, level and plugin version); incremental re-runs; eval cases at rung 6; the proposed tested set (the versions prove found, which release's plan shows and the yes fixes). Files in a rulebook: the fixture files of each pair, the proof grid as a generated document.
- **Commands.** prove (rulebook; runs the pairs at the path each binding declares and ends in the proof grid; a show mode prints the last grid).
- **Reads and hands over.** Reads bindings, their pair paths and plugins (3), pairs and the tested set (1), checker code by name and digest (3, 5), tool versions the gate supplies. Hands results to 3 (standing and marks), 5 (the release's grid, health and the proposed tested set), 6 (the grid rendered), 9 (pairs as eval fixtures); the explorer sees the grid through the bundle.
- **Invariants.** A cell is pass, fail, not run or reviewed; never blank. A pass requires the bad example to be flagged and the good example to pass, at the declared path. The level of every run is recorded. Rung 6 is never pass or fail. Nothing from a project enters a run, except that verify reads the project's own dependencies the gragas plugin's grant names, read-only (S23); nothing is executed from them and nothing is written under the project. A run never executes a tool's output.
- **States.** Cell: pass, fail, not run (no pair, tool missing, time or memory limit hit), reviewed. Run level: isolated, sandboxed.
- **Findings.** A failing cell; a not-run cell; a binding with no pair; a pair whose good example also trips the bad check; a level below what governance requires for third-party checker code.
- **Closing words.** prove: done when every cell passes or is reviewed; findings otherwise; not ready when no tool of the tested set is present; busy when another run holds the scratch directory.
- **Empty and error states.** prove on a rulebook with no bindings ends "done, nothing to prove". A tool missing for one version leaves that column "not run" and the run continues. A limit hit is "not run" with the limit named.
- **Never.** Grades a reviewer's verdict; runs over a project's source; installs or updates a tool; counts a cell as passed without the bad example failing.

### Domain 5: Releases

- **Purpose.** Give a rulebook versions that mean something, and ship what a project needs to trust one.
- **Owns.** The computed bump; the changelog with a plain sentence, a consumer action and a remedy per change, and the release note in plain words (S75); the tested set, fixed from prove's proposal at the release's yes; flattened upstream sets; assets pinned by content hash; the checker-code artifacts, whose digests release computes and records, and the parent's, listed by reference under its name and publisher (S23); the bundle; health, computed by the engine and claimed by the release; the rule map; archive and successor; the withdrawn list; the index entry; diff by rule; the release's own commit of the files it names and the annotated tag, made at the yes and never pushed by the engine. Files in a rulebook: the changelog, the bundle, the preset packages and locked files, the rule map file, the withdrawn list, the release's digest list; the tested set lives in identity.
- **Commands.** release (rulebook; plan → the release files committed, the tag, bundle, presets, changelog, health, content hash; the push is the owner's). diff (both; a listing by rule between two releases). changelog (both; a listing with consumer actions). health (both; the seven numbers, own and resolved for an overlay). archive (rulebook; plan: successor and rule map). withdraw (rulebook; plan: an entry in the withdrawn list). index (both; a listing; a mode prints this rulebook's entry for the index pull request).
- **Reads and hands over.** Reads history (2), bindings, presets, coverage and conflicts (3), the proof grid (4), identity and the tested set (1). Hands the bundle to 10, the pages' source to 6, releases to git hosting and registries, the consumer actions to 8 (upgrade and switch plans), the index entry to the index by pull request.
- **Invariants.** The bump is computed from the diff of rules, bindings and presets against the last tag and never typed by hand, and the first release, which has no tag to diff, publishes the version identity declares (S68); a release names the format and contract versions; every asset is listed by digest; checker code is listed by digest and publisher; a release with an unresolved entry conflict is refused; a release whose checker code lacks tests or the required review is refused; health is recomputed at every release and never edited; the withdrawn list is read unsigned (S11).
- **States.** Release: planned → tagged → withdrawn (by the list) or archived (with a successor). Bump: patch (explanations, translations, remedies), minor (new rules, tightened bindings, new presets), major (a must removed, retired or weakened; an id retired without a successor; a preset entry removed).
- **Findings.** Bump size larger than the owner expected (listed, not refused); an unproven binding shipping (listed, release proceeds on a yes); a blocked entry (refused); checker code without tests or review (refused); a tested-set version with no proof column; a rule map entry pointing at a retired id.
- **Closing words.** release: done after a yes, declined, refused on a blocking finding, findings when a dry listing is asked and findings exist. diff, changelog, health, index: done. archive, withdraw: done after a yes, declined.
- **Empty and error states.** release on a rulebook where every rule is proposed shows an empty preset in the plan and says so first. release with no change since the last tag ends "done, nothing to release". health on a rulebook with zero rules shows every number as "no rules" rather than zero. diff between two identical releases ends "done, no change".
- **Never.** Types a version by hand; signs a release; pushes; hosts a registry; marks another rulebook's release withdrawn; folds signals into health.

### Domain 6: Explain and publish

- **Purpose.** Render one explanation into every place people read, in their language, without a second site.
- **Owns.** Pages per rule, part, guide, decision, term; in-place blocks; the engine's handbook; the rulebook's chapters rendered; docs profiles with a placement per field; nav fragments; catalog rows; docs-link targets; the SARIF help-link rewrite; the rendering manifest; attribution; the language of a rendering. Files: the export tree the host asked for, the rendering manifest, the docs profiles (shipped with the engine and overridable by a rulebook or a project).
- **Commands.** docs (both; export pages, blocks, nav, rows and the manifest into the host's tree; in its accept form, a plan that accepts the host's omissions, S74). show (both; one entity, with its relations). find (both; rules by filter: part, kind, strength, standing, mark, tool, owner, status). handbook (both and neither; the engine's handbook in the terminal). sarif (project; rewrite a tool's SARIF help links to rule pages and merge gragas findings).
- **Reads and hands over.** Reads the bundle (10) or the live model (1, 2, 3, 4, 5) in a rulebook; reads the docs profile of the host; re-reads only the files it wrote, by the manifest. Hands pages, blocks, rows and nav to docs platforms and portals; hands the manifest to 8 (check reports a stale or edited rendering); hands the render model it shares with 7.
- **Invariants.** Every rendering derives from one explanation; the manifest records the source hash of each; a rendering edited by hand is a finding, never silently kept; a host's omissions are recorded once and accepted only by a yes (S74); attribution and licence per kind travel with every page; a page in a language falls back to the authoritative text with a mark.
- **States.** Rendering: current, stale (source changed), edited (hash differs from what was written), omitted field (per profile). Language: authoritative, translated, fallback.
- **Findings.** Stale rendering; edited rendering; a field the profile omits; a broken docs-link target; an anchor that moved without an alias.
- **Closing words.** docs: done; findings when the manifest records omissions the host has not accepted; declined in the accept form. show, find, handbook: done; find with no match ends "done, no rule matches" and shows the filters. sarif: done.
- **Empty and error states.** docs on a rulebook with zero rules writes the handbook and the empty part pages and says "0 rules exported". show on an unknown id offers the nearest ids and aliases. docs against a host whose profile is unknown ends "not ready" and lists the profiles it knows.
- **Never.** Builds a second search; renders a project's code; edits the host's own pages beyond the files it wrote; invents a docs URL for a tool that fixes its own, or for a rulebook whose identity carries no base URL (S72).

**Placements in the profile for a Markdown docs platform with typed front matter (docusaurus).** Nothing is omitted.

| Where | Fields |
|---|---|
| Front matter | title, description, type (always standard), status, owner, enforced by |
| Body | rule id, strength, standing, sources, attribution |
| Omitted | none |

The rendering manifest records this placement, and the portal owner accepts it once.

### Domain 7: The explorer

- **Purpose.** Show the aggregate truth no docs platform renders, filterable, embeddable, accessible.
- **Owns.** The twelve views: rules, rule, relations, rulebooks, tools, coverage, standing bars, health, changelog, terms, attention, here. The packagings: standalone, web component, docs plugin, portal plugin, iframe. State in the URL; host theme tokens; the shared render model with domain 6.
- **Commands.** explorer (both; builds the static bundle for a host and ends "done"; in a project with a report file, the standalone build carries the here view; in a rulebook before its first release, a working bundle from the live model, marked unreleased). The engine serves nothing; the handbook shows how to open the bundle.
- **Reads and hands over.** Reads only the contract: one rulebook bundle, and in the standalone packaging one local report file. Hands nothing; it is a reader. Its needs are the proof that the contract is sufficient.
- **Invariants.** Nothing in a view is computed from anything but the bundle and, for here, the report; every filter is in the URL; standing is never shown by colour alone; the here view exists only in the standalone packaging; the iframe, the web component and the plugins show the rulebook views and say so, the web component naming the view it mounts by an attribute (any of the eleven, the rules view when none) and never offering here; an unreleased bundle shows its mark on every view.
- **States.** Bundle: loaded, missing, unreleased, from a previous contract major (read, with a banner). Report: none, loaded, from a different release than the bundle (shown with a mark on every affected row).
- **Findings.** None raised; it shows the rulebook's findings (attention, conflicts, stale renderings) as views.
- **Closing words.** explorer: done; not ready when no bundle and no live model can be found.
- **Empty and error states.** No bundle: an empty frame that names the file it wants. Zero rules: the coverage and health views say "no rules" and the rules view offers the handbook. No report in standalone: the here view says "no report; run report in a project and point here at the file". A report from another release: a banner and a mark per row that differs. A portal that hosts a project's report does so by its own choice, outside gragas.
- **Never.** Stores a report; runs or calls a service; counts visitors; searches outside the bundle; shows a project's name an embedded host did not provide.

### Domain 8: In a project, optional

- **Purpose.** Let a team see what applies, what its overrides weaken, and what an upgrade or a switch changes, while the presets keep working without gragas.
- **Owns.** The pin, lock, state file and manifest formats; init with content-hash and digest checks, copied presets and printed wiring; status; check with fail-on; waive; baseline; the upgrade plan (project side); the switch plan; the report. Files in a project: the pin file, the lock file, the state file (per machine, outside version control), a manifest per package in a monorepo, the waivers file, the baselines file (only for tools with none of their own), the report file, copied presets (owned by the project after the copy).
- **Commands.** init (project form; plan: pin, lock, copied presets, opted-in agent folders and reviewer format, an existing tool baseline file adopted as the project's baseline, wiring printed). status (project; a listing: what applies for these kinds and parts, versions against the tested set, the last fetch from the state file, warnings). check (project side; a listing of fidelity findings, exit 0 unless fail-on; the rulebook side is domain 2's gate listing). waive (project; plan: a waiver record, ending with the suppression text to add in the tool's syntax with the waiver id: the tool's line comment for a line-level finding, and where the weaker entry is a config severity, a path-scoped config block carrying the waiver id as a comment, which replaces the project's global override). baseline (project; plan: record or shrink a baseline). upgrade (project side; plan from the changelog's consumer actions; three-way diff for copied presets). switch (project; plan from the rule map or the preset diff). report (project; writes check's document as the report file).
- **Reads and hands over.** Reads the pin, lock, state file, manifests, the project's tool configs, its hook files and CI files (only to report which printed lines are present), the release and its bundle from git hosting or a mirror, the withdrawn list, the changelog and rule map (5), observations and provenance (3), the rendering manifest (6) and the files it names. Hands the report to the contract (10) and to whoever the org points at; hands the pin, lock, manifests and config set to 3 and 9; hands the yes entries to the lock and the machine facts to the state file.
- **Invariants.** One pin and one lock per repository; manifests narrow, never widen. A release is accepted only after its content hash and digests match the pin, or with the explicit yes that takes an unpinned release, recorded in the lock. Nothing is written outside the pin, lock, state file, manifests, waivers, baselines, report, copied presets the project asked for, opted-in agent folders and the review file at the path its reviewer format names. A waiver is recorded, never refused. Tool-native suppression and baseline files are read, never written. check exits 0 by default. The report names no project. The lock holds what a yes decided, what the release fixed, and the sums of the renderings a yes allowed; machine facts go to the state file. Domain 8 writes nothing outside the files named above; docs writes the export tree the host asked for (domain 6).
- **States.** Project: not pinned (not ready), pinned, pinned to an archived rulebook (warned, switch offered), pinned to a withdrawn release (warned). Waiver: active, expired, orphaned (after a switch), closed. Baseline: open (shrinking), grown (a finding), closed at zero. Lock: current, behind the pin (upgrade owed: a finding in check, exit 0 unless fail-on names it, S14), pinned by a yes.
- **Findings.** Weaker entry without a waiver; absent entry; unmapped entry; edited copied preset; expired waiver; waiver without an owner; baseline grown; tool or preset version outside the tested set; engine below the minimum; stale or edited rendering; content-hash or digest mismatch (refusal); withdrawn release in the pin; archived rulebook in the pin; lock behind the pin (upgrade owed); orphaned waiver (S71).
- **Closing words.** init, upgrade, switch: done after a yes, declined, refused on a mismatch, not ready without network and without a path or mirror. waive, baseline: done after a yes, declined. status, report: done; not ready when unpinned. check: done with findings listed, or findings when fail-on matches.
- **Empty and error states.** Any command in an unpinned project ends "not ready" and offers init. init with no network asks for a path or a mirror. init on a release no pin holds yet shows the explicit yes form and records the yes and the hash in the lock. init on a digest mismatch ends "refused" and names what changed. status on an archived rulebook warns and offers the switch plan toward the successor. check on a clean project ends "done, 0 findings". waive always ends by printing what to add to the tool's file or line.
- **Never.** Blocks a build, commit or edit; writes a hook; writes a tool's suppression file; writes a tool config except the copied preset the project asked for; stores a result anywhere but the project's own report file; phones home.

### Domain 9: Agents and reviewers

- **Purpose.** Give an agent the rules no tool holds for the file it edits, and a reviewer the rules only review can hold, each in its own format.
- **Owns.** Scoped agent files per opted-in folder; the one pointer line; the rules-for-a-path answer; the budgets as rulebook settings; review files per reviewer format with the review question and eval cases; the language setting. Files in a project: the agent files inside opted-in folders, the review file, their sums in the lock.
- **Commands.** agents (project; write or refresh the files in the opted-in folders; a listing of what was written and the budget used; with its opt-in form, a plan that changes the lock's folders and ends in a lock entry, removing files on opt-out). rules (project; print the rules for a path; the hook calls it). review (project; write the review file for the configured reviewer format; with its opt-in form, a plan that changes the format in the lock).
- **Reads and hands over.** Reads standing and marks (3), pairs (4), scope and translations (1), the pin and manifests (8), the budgets from identity. Hands files to the project's opted-in folders and the review tool; hands nothing back but the sums for the lock, which the opt-in yes allowed.
- **Invariants.** Only rules with no effective rung 0–5 binding appear; a rule a linter now holds drops out at the next refresh and is an attention item until it does. Each entry is id, statement, at most one pair, a URL. Budgets in bytes are the rulebook's settings and the rulebook's check holds them. Files are written only in folders the lock lists as opted in, and the review file only at the path of the reviewer format the lock lists; the pointer line and the hook line are printed, never written; AGENTS.md and CLAUDE.md are never written. Text is in the authoritative language unless the project sets another.
- **States.** Agent file: current, stale (standing changed since), over budget (a finding in the rulebook). Opt-in: on, off (files removed at the next refresh after opt-out).
- **Findings.** Over budget (in the rulebook's check); a stale agent file (in the project's check, through the rendering manifest); a review file carrying a rule a linter holds.
- **Closing words.** agents, review: done; in the opt-in form, done after a yes or declined. rules: done; done with "no rules for this path" when none apply.
- **Empty and error states.** agents without any opt-in ends "done, no folder opted in" and names how to opt in at init. rules on a path outside every part answers "no rules for this path". review with no rung-6 rules writes a file that says so, so the reviewer's config does not break.
- **Never.** Writes outside opted-in folders and the review file's path; ships rules a tool already holds; runs an agent or a reviewer; grades a reviewer.

### Domain 10: The contract

- **Purpose.** Document and version the data every domain emits, so anyone renders a rulebook any way and stays compatible.
- **Owns.** The schemas per entity and contract version; the field reference with derivation rules; generated types (for TypeScript the published package `@gragas/contract`, section 9); the compatibility promise; the fixture rulebook and its documents; the validator; the contract's changelog with consumer actions. Documents: the rulebook bundle, the project report, the changelog, SARIF, plans as documents. Domain 10 owns the schema of every emitted document; it does not own the entities the documents carry (section 16.1).
- **Commands.** contract validate (any context; a listing of schema findings for a document). contract fields (any; the field reference for an entity). contract fixture (any; emit the fixture rulebook's documents).
- **Reads and hands over.** Reads documents emitted by 3, 4, 5, 6 and 8. Hands the field reference to the handbook (6) and the explorer (7); hands types to the ecosystem's languages; hands the fixture to tests of every renderer.
- **Invariants.** Every emitted document validates against the schema of the version it names; a minor adds with defaults; both shapes are emitted for two minors before a major removes one; the explorer reads the current and previous major; the contract's own changelog carries a consumer action per change; the fixture rulebook exercises every field.
- **States.** Contract version: current, previous major (read), deprecated shape (emitted beside the new one), removed.
- **Findings.** A document that fails its schema; a document naming an unknown contract version; a deprecated shape past its window.
- **Closing words.** contract validate: done, or findings. fields, fixture: done.
- **Empty and error states.** validate on an unknown document kind ends "usage" and lists the kinds. A document from a newer contract major ends "not ready" and names both versions.
- **Never.** Holds a project identifier; changes a meaning in a minor; emits a document without its versions.

### Across all: the terminal

- **Purpose.** One voice for maintainer, developer and agent.
- **Owns.** The bare menu that detects rulebook, project or neither and leads with it; forms; the plan screen and the plan as an entity (one shape, S43); the finding and the notice as entities (one shape each); the eight closing words and exit codes; the output modes (plain, accessible, JSON, SARIF where a command lists findings); the standing palette and symbols; the language flag; help and version.
- **Commands.** The bare command (menu at a terminal, done; help and "usage" when nobody can answer). help (any). version (any; engine, format and contract versions). journal (any; the runs on this folder, recover an interrupted run, print the lines that restore a run's files). cache (any; what the engine keeps on this machine, and clearing it). init's first question in the "neither" context (project or rulebook), after which domain 8 or domain 1 takes the form.
- **Reads and hands over.** Reads the context (a rulebook's identity file, a project's pin, or neither) and the global flags. Hands every command its context and renders every domain's output.
- **Invariants.** Every command ends in exactly one closing word and its exit code. Every changing command shows the plan screen with the same sections. JSON carries the same words as the screen. The accessible mode uses symbols and words, never colour alone. The menu never runs a changing command without the form.
- **States.** Context: rulebook, project, neither, both (a rulebook that is also a project: the menu shows both groups, rulebook first; upgrade and propose appear in both groups and ask for their subject, the project's pin or the rulebook's pins, unless it is given as an argument).
- **Findings.** None of its own; it renders every domain's.
- **Closing words.** done 0, findings 1, failed 1, usage 2, busy 3, not ready 4, refused 5, declined 6.
- **Empty and error states.** "Neither" shows init (project or rulebook) and the handbook. An unknown command ends "usage" with the nearest names. A lock held by another run ends "busy" with the holder's start time.
- **Never.** Shows a banner outside the menu; hides a finding behind a colour; applies a plan without a yes or the yes flag.

### 14.1 How the domains depend on each other

An arrow A → B means B reads what A hands over. The core (3 enforcement link, 4 proof) sits at the centre: it reads the model (1), the lifecycle state (2), a project's files (8) and, when it resolves a chain, a parent's or a project's pinned release (5); every other domain reads the truth it derives. Nothing outside the core computes standing, coverage, observation or provenance; everything outside renders, ships, records or reads it.

| From | To | What crosses |
|---|---|---|
| 1 Format | 2, 3, 4, 5, 6, 9 | the parsed model: identity, structure, rules, terms, translations, exemptions, contracts, budgets |
| 2 Authoring and lifecycle | 3, 5, 6 | status, trial state, owners, review-by, zero-by, exemptions, exclusions, sources, decisions, history |
| 3 Enforcement link (core) | 2, 5, 6, 8, 9, 4 | standing, marks, coverage, conflicts, the resolved chain with provenance, observations, presets, checker code by name and publisher; to 4 the pair placement |
| 4 Proof (core) | 3, 5, 6, 9 | proof results and levels per binding and tool version; the proposed tested set; pairs as eval fixtures |
| 5 Releases | 8, 10, 6, 3, 2 | releases, changelog, consumer actions, tested set, health, rule map, withdrawn list, the bundle; to 3 a pinned release to resolve; to 2 the parent's, imports' and upstream releases at their pins and the withdrawn list, for the rulebook's own upgrade |
| 6 Explain and publish | 8, hosts | renderings and the rendering manifest |
| 7 The explorer | nobody | a reader of 10 only |
| 8 In a project | 3, 9, 10 | pin, lock, manifests, the project's config set, opted-in folders; the report |
| 9 Agents and reviewers | project folders, reviewers | agent and review files |
| 10 The contract | 7, 6, every renderer | schemas, field reference, types, fixtures |
| Terminal | all | context and flags in; words and exit codes out |

Three readings of the map. First, the core never reads its own renderings or the release it is building; it reads a release only as an input to resolve (a parent at its pin, a project's pinned release), so a change in how pages or releases are made cannot change what is true. Second, the project domain (8) and the agents domain (9) read the core and the releases, never each other's internals; the pin and manifests are the only thing 8 hands to 9. Third, the explorer (7) reads only the contract (10), so if a view needs a field the contract lacks, the contract is incomplete, not the explorer.

## 15. Commands

### 15.1 The full table

Context: R a rulebook's repo, P a project, B both, A any (also "neither"). "Ends in" is what the command leaves on the screen: a plan (shown before a yes), a report (a document that can be written), or a listing (read and gone). Closing words are the normal ones; every command may also end usage, busy or not ready.

| Domain | Command | Context | Ends in | Closing words |
|---|---|---|---|---|
| 1 | validate | R | listing of findings and notices | done; findings |
| 1 | migrate | R | plan | done; declined; not ready |
| 2 | rule new, edit, replace, retire, restore, renumber, promote, confirm, exclude, include | R | plan | done; declined; refused |
| 2 | term new, edit, retire | R | plan | done; declined; refused |
| 2 | source new, edit | R | plan | done; declined; refused |
| 2 | decision new, supersede | R | plan | done; declined; refused |
| 2 | undo | R | plan | done; declined; refused; not ready |
| 2 | guard edit, staged | R | listing of the lifecycle fields a pending change touches without a history entry, or nothing | done; refused |
| 2 | import | R | plan | done; declined |
| 2 | propose | B | plan, then the proposal document printed with the line that opens it on the target's host, or a recorded proposal | done; declined |
| 2 | fix | B | plan (the menu over findings) | done; declined |
| 2 | signals | R | listing | done; not ready |
| 2 | attention | R | listing; two batches as optional input | done |
| 2 | history | R | listing | done |
| 3 | bind, rebind, unbind | R | plan | done; declined; refused; not ready |
| 3 | conflicts | R | listing, offering the three resolutions | done; findings |
| 3 | coverage | R | listing | done |
| 3 | tools | B | listing | done |
| 3 | resolve | B | listing or report | done; not ready |
| 3 | explain | B | listing | done |
| 3 | plugin (test, contract, list, approve, add, upgrade, record) | B | listing, one row to a check | done; findings; not ready; refused; declined |
| 3 | preset | R | plan | done; findings; declined; refused; not ready |
| 3 | verify | P | report | done; findings; not ready |
| 4 | prove | R | report (the proof grid) | done; findings; not ready; busy |
| 5 | release | R | plan | done; declined; refused; findings |
| 5 | diff | B | listing | done |
| 5 | changelog | B | listing | done |
| 5 | health | B | listing | done |
| 5 | archive | R | plan | done; declined |
| 5 | withdraw | R | plan | done; declined |
| 5 | index | B | listing | done |
| 6 | docs | B | report (the export and its manifest); in the accept form, a plan | done; findings; declined; not ready |
| 6 | show | B | listing | done |
| 6 | find | B | listing | done |
| 6 | handbook | A | listing | done |
| 6 | sarif | P | report | done |
| 7 | explorer | B | report (a built bundle; unreleased mark in a rulebook before a release) | done; not ready |
| 8 (or 1) | init | A: a project form, or in "neither" the question project or rulebook | plan | done; declined; refused; not ready |
| 8 | status | P | listing | done; not ready |
| 8, 2 | check | B | in a project, listing of findings, and the report on request; in a rulebook, the gate listing of S14 | done; findings (fail-on in a project; any finding in a rulebook) |
| 8 | waive | P | plan, ending with the suppression text to add | done; declined |
| 8 | baseline | P | plan | done; declined |
| 8 (or 2) | upgrade | B: the project's pin, or a rulebook's own parent, import and upstream pins | plan | done; declined; refused |
| 8 | switch | P | plan | done; declined; refused |
| 8 | report | P | report | done; not ready |
| 9 | agents | P | listing of files written; with the opt-in form, a plan | done; declined |
| 9 | rules | P | listing | done |
| 9 | review | P | listing of the file written; with the opt-in form, a plan | done; declined |
| 10 | contract validate, fields, fixture | A | listing | done; findings; not ready |
| all | (bare) | A | the menu at a terminal; help when nobody can answer | done; usage |
| all | help | A | listing | done |
| all | version | A | listing | done |
| all | journal (list, recover, restore) | A | listing; recover rolls an interrupted run back; restore prints the lines and runs nothing | done; refused |
| all | cache (list, clear) | A | listing | done; refused |

Fifty-five named commands and the bare menu, counting each family once. Twenty-four of them end in a plan: migrate, rule, term, source, decision, undo, import, propose, fix, bind, rebind, unbind, preset, release, archive, withdraw, init, waive, baseline, upgrade, switch, agents and review in their opt-in form, and docs in its accept form (S74). The rule family has ten subcommands; term has three; source and decision have two each; contract has three. Two commands run in both contexts with a different subject: upgrade (a project's pin, or a rulebook's own pins, S64) and propose (to the parent, or to the pinned rulebook, S65); init asks its first question in the "neither" context (S67). check runs in both contexts with a different listing: a project's fidelity findings, or a rulebook's gate listing (S14, S40). guard is the command the rulebook's hook line and gate rule call; it is not for a project.

### 15.2 The five reading commands in a project

These five are the project's reading set; check also runs in a rulebook, where it is the gate's listing (S14), but that is the rulebook's own data and not a project reading. status says what applies here (rules for these kinds and parts, by standing) and which versions are in play against the tested set. explain takes a config line, a tool's rule id or a path and answers with the rules, the observation and the layer that set it. check lists fidelity findings and ends with an exit code. verify runs the rulebook's pairs over this project's effective config. report writes check's document to the report file.

### 15.3 Plans and the yes

Every plan has the same sections, in this order: what changes (rules, bindings, tool entries, with standing before and after); consumer actions; what is written where, file by file; what is re-keyed or orphaned (waivers, baselines); renderings rewritten (agent files, review file, copied presets with a three-way diff, and pages where a rendering manifest exists); checker-code changes with a diff link; the question. A yes writes a history entry in a rulebook and a lock entry in a project, each with who, when and the plan's digest. A script passes the yes flag. Without a terminal and without the flag, the plan prints and the command ends "declined", exit 6. An unpinned release is accepted only through this form, and the yes names it.

### 15.4 Closing words and exit codes

| Word | Exit | Meaning |
|---|---|---|
| done | 0 | the command finished; findings may be listed (check in a project) |
| failed | 1 | the engine itself failed (an unreadable settings file, a child process that died, a torn journal); the run id is printed. Shares exit 1 with findings; the word and the JSON kind tell them apart |
| findings | 1 | a finding the command treats as failing: in a rulebook any finding; in a project a fail-on kind in check, a failing pair in verify, an unaccepted omission in docs; a notice never counts |
| usage | 2 | the command or its arguments were not understood |
| busy | 3 | another run holds the lock or the scratch directory |
| not ready | 4 | missing precondition: no pin, no network and no path, engine below minimum, format or contract newer than the engine, fewer than two batches, no tool of the tested set present |
| refused | 5 | the engine will not do it: content-hash or digest mismatch, a lifecycle change without history, a release with a blocked entry or unreviewed checker code, an overlay weakening a parent outside an exclusion, a rebind weakening an imported rule |
| declined | 6 | a plan was shown and not applied |

### 15.5 Output modes

JSON is available on every command and carries the same words as the screen. SARIF is available wherever a command lists findings: validate, check, conflicts, prove, verify, docs, release, contract validate, and the sarif command's merged output. Plain mode drops decoration; accessible mode adds a word and a symbol beside every colour (section 17). A language flag selects the rendering language where text has translations.

## 16. Entities and formats

### 16.1 The entity model

One rule for ownership: the owning domain is the one whose commands change the entity. Domain 1 owns the format of every rulebook file and domain 10 owns the schema of every emitted document; neither is thereby the owner of the entity written in them. Every entity has exactly one owner.

| Entity | Id | Key fields | Owning domain | Lifecycle |
|---|---|---|---|---|
| Rulebook | name (short name resolved by the index) | version (declared once at init for the first release, S68), licences per kind, authoritative language, extends (parent at a pin, with its content hash), imports (rulebook, parts, content hash), owners, governance, tested set, format version, kind (base, overlay, companion, fork, archived), published base URL for pages, optional base URL for an explorer the owner hosts (S72) | 1 | created → released repeatedly → archived with a successor; releases may be withdrawn |
| Layer, part, concern | slug path; no part slug or alias is `gragas`, the first word of the engine's own check ids (a rulebook's name may be, since an imported id has four words and a check's three) | name, order; a part also has its guide and owners | 1 | added → renamed with an alias → retired when empty |
| Kind | slug | name, description | 1 | added → retired when no rule or project names it |
| Guide, chapter | the part's id; a chapter's slug | prose, anchors, translations | 1 | edited as prose |
| Term | slug | definition, literal (optional); translations live in `translations/<lang>/` (A65) | 2 | defined → retired when unused |
| Rule | frozen id with aliases | statement, strength, scope (parts, paths, kinds), exemptions (path or kind, with a reason), reasoning, pairs, status, owner, review-by, zero-by, relations, origin, sources, bindings, translations, replaced-by, anchor (Markdown form) | 2 | proposed → trial → active → retired; restore; replace |
| Exclusion | overlay, parent rule id and a sequence number | decision, who, when, lifted by (decision, who, when) | 2 | made by exclude → lifted by include; a lifted record stays |
| Example pair | rule id and a pair number | good example, bad example, target type; the files sit in the rulebook's pairs folder, one folder per pair, named by rule id and number | 4 | written → proven per binding and tool version |
| Binding | rule, tool, key | target type, value, options, severity, rung, run point, remedy, pair, the path the pair stands at, proof; in an overlay or an importer, the parent's or imported rule it tightens | 3 | declared → proven or unproven, blocked while in a conflict → removed by unbind |
| Plugin | tool and plugin version | the manifest's facts (tool version range, targets, whether a docs URL can be set), the five operations (plan, entries, findings, rank, preset) and the grants it asks for | 3 | published → installed and pinned by digest → approved on a machine → superseded by a new version |
| Tool entry | tool, target, key | value, options, severity; the bindings that hold it; the layer that set it | 3 (derived) | derived at every resolution |
| Preset | rulebook, tool, name, at a release | entries, extends (upstream pins), digest, licence | 3 | generated from bindings; shipped by a release; copied once into a project on request |
| Checker-code artifact | name and publisher; digest once released | tool, tests present, reviewed by, licence, digest; a parent's or an import's artifact is listed by reference under its own name and publisher | 5 | declared by name (3) → digested and shipped at release → changed (a consumer action) |
| Source | slug | kind (standard, guide, study, incident, decision record, vendor document, own decision), title, locator, attribution | 2 | added → edited |
| Decision | slug | title, text, rejected options, cites (sources, rules, bindings), who, date, supersedes, superseded-by | 2 | made → superseded |
| History entry | sequence number | kind, entity, lifecycle fields before and after, who, when, plan digest, decision | 2 | appended, never changed |
| Proposal | slug | target rulebook, the change (anatomy, pair, draft binding, sources), evidence, state, link; recorded in an overlay's proposals file, or from a project in the lock entry and the printed document | 2 | open → accepted, declined or withdrawn |
| Translation | entity, field, language | text, hash of the authoritative text, stale | 1 | written → stale → re-translated |
| Proof result | binding and tool version | result (pass, fail, not run, reviewed), run level, plugin version, reason when not run (no pair, tool missing, limit hit) | 4 | recomputed when inputs change |
| Tested set | release | tool, tool-plugin, kit and gragas-plugin versions (the last under the name `adapters` in release 1, gragas#2071); minimum engine | 5 | proposed by prove from the versions it found → fixed at the release's yes |
| Release | version | bump kind, date, release note (plain words, S75), content hash, assets by digest, checker code by digest and publisher, tested set, health, rule map, changelog, format and contract versions | 5 | planned → tagged → withdrawn or archived |
| Changelog entry | release and entity | change kind, a plain sentence (S75), consumer action, remedy, link to the diff | 5 | written at release |
| Health | release, and own or resolved for an overlay | the seven numbers | 5 | computed at release |
| Rule map | release, the rulebook left and the rulebook arrived at | per row: id left, id arrived at, mark (same, tighter, weaker, none), reason | 5 | written at release |
| Withdrawn list | rulebook | per entry: release, reason, date | 5 | appended |
| Index entry | short name | location, kind, format version, latest release, health | 5 | added by pull request → updated → marked archived |
| Docs profile | host name | per field: front matter, body or omitted; catalog mapping; nav shape | 6 | shipped → overridden |
| Rendering manifest | release and target | per rendering: source hash, language, omissions accepted (each with who, when and the plan's digest, S74) | 6 | written at export |
| Pin | the project | section 16.2 | 8 | written at init → changed by upgrade or switch |
| Lock | the project | section 16.3 | 8 | written at init → appended by every yes |
| State file | the project on one machine | section 16.4 | 8 | written at init → rewritten by every run that fetches or looks for tools |
| Manifest | package (by digest) | section 16.5 | 8 | written by the team → read |
| Waiver | slug | section 16.6 | 8 | active → expired or closed; re-keyed or orphaned by a switch |
| Baseline | slug | section 16.7 | 8 | open and shrinking → grown (a finding) → closed at zero |
| Report | pin, release, run time, lock digest | section 16.8 | 8 | written by report |
| Agent file, review file | folder and part; the reviewer format | rules carried, sums, language | 9 | written at opt-in → refreshed → removed at opt-out |
| Plan | digest | the sections of 15.3 | the terminal | shown → accepted (yes recorded) or declined |
| Finding, notice | kind, entity, location | message, the fix offered, the rule it concerns, refusal or not; a notice never changes the closing word | the terminal | raised → fixed or accepted |
| Bundle | release | section 16.9 | 5 | written at release |

Thirty-nine rows, forty-four entities: four rows hold kin with one owner and one lifecycle (layer, part and concern; guide and chapter; agent file and review file; finding and notice). Signals are computed from batches and stored nowhere, so they are not an entity.

### 16.2 The pin

| Field | Meaning |
|---|---|
| rulebook | the short name, resolved through the index, or a full location |
| version | the release this project follows |
| source | where releases come from: a git host, a path or a mirror |
| kinds | the kinds this project declares, any trait the rulebook defines as a kind, those that switch rules off included (S73); status and the agent files filter by them |
| parts | the parts in play for this project; a monorepo's manifests narrow them per package |
| content hash | the hash of the release the project pinned for this rulebook; a release whose content hashes to another value is a mismatch |

### 16.3 The lock

| Field | Meaning |
|---|---|
| resolved release | the exact release, with its format and contract versions |
| bundle digest | the digest of the bundle of the resolved release |
| preset digests | each preset the project imports, by digest as shipped |
| copied presets | each copied preset, by path and digest as shipped, so an edit shows |
| checker code | each artifact loaded by the project's tools, by digest and publisher |
| plugin versions | the plugins used when the release was proven (`adapters_used` in release 1) |
| opted-in folders | the agent folders the project allowed; the reviewer format chosen |
| rendering sums | the sums of every agent and review file written, so an edit shows |
| yes entries | every plan accepted: who, when, the plan's digest, the command; the yes that takes an unpinned release among them |
| provenance | the last upgrade or switch: from, to, when, the map used or "preset diff" |

The lock holds what a yes decided, what the release fixed, and the sums of the renderings a yes allowed, so it can be committed beside the pin (S66).

### 16.4 The state file

Per machine, outside version control; the handbook says so and init prints the ignore line. A project has one; a rulebook has one for its parent, import and upstream fetches.

| Field | Meaning |
|---|---|
| tool versions found | what init, check or verify last saw on this machine, against the tested set |
| last fetch | when releases and the withdrawn list were last read |
| withdrawn seen | the withdrawn list as of the last fetch |

### 16.5 The manifest

| Field | Meaning |
|---|---|
| parts | the parts in play for this package; a subset of the pin's |
| kinds | the kinds this package declares; a subset of the pin's, or the same |
| name | optional, off by default; a label for the here view, never in the report unless the team turns it on |

### 16.6 The waiver

| Field | Meaning |
|---|---|
| id | stable, cited in the tool's suppression comment or entry |
| rule | the rule id or alias waived |
| path | the file, folder or pattern it covers |
| reason | one sentence, required |
| kind | false positive, planned fix, accepted risk, legacy |
| owner | a person or team, required |
| expiry | a date, required; an expired waiver is a finding |
| ticket | a tracker reference, optional |
| created | who and when |
| suppression | the exact text the tool needs, printed by waive for the team to add: a line comment, or for a weaker config severity a path-scoped block carrying the waiver id |
| state | active, expired, orphaned (after a switch, S71) or closed |
| closed | who and when, and why: redundant after an upgrade, orphaned after a switch, or by hand; the record stays |

### 16.7 The baseline

| Field | Meaning |
|---|---|
| id | stable |
| rule | the rule id whose debt is counted |
| tool | the tool whose findings are counted |
| count | the current count; may only shrink |
| owner | a person or team, required |
| zero-by | a date; past it, a finding |
| tool file | the tool's own baseline or bulk-suppression file where one exists; then this record only points at it and counts |
| created, last counted | who and when; the last count |

### 16.8 The report

| Field | Meaning |
|---|---|
| versions | contract, format and engine versions |
| pin, release | the pin's fields and the release the report was computed against |
| run time | when check ran; an input to the report, needed by signals to order batches (S57) |
| lock digest | which lock state this report describes |
| sections | one per package, keyed by manifest digest; name only if turned on |
| per rule | standing, marks, observation, provenance (layer, decision, switch), bindings in play |
| waivers | per waiver: rule, kind, age, expiry, owner present or not; never the path |
| baselines | per baseline: rule, size, age, zero-by |
| drift | entries that differ from the preset: weaker, tighter, absent, unmapped, edited copy |
| mismatches | tool, preset and plugin versions outside the tested set; engine below minimum |
| renderings | stale or edited agent, review and page renderings |
| wiring | which printed hook and CI lines are present, read from the project's hook and CI files for this purpose only |
| trust | content hash matched or pinned by a yes; digests matched; withdrawn or archived state |
| last change | the last upgrade or switch, from the lock's provenance |
| no identifier | the report carries no project name, path, remote or author |

### 16.9 The bundle

| Field | Meaning |
|---|---|
| versions | contract, format and engine versions; the release version and date |
| identity | the rulebook's identity, governance and licences per kind |
| structure | layers, parts, concerns, kinds, guides and chapters as rendered text |
| terms | every term with its literal and translations |
| rules | every rule with every field of 16.1, its aliases and translations; retired rules included and marked; an overlay's exclusions with their decisions |
| released | whether this bundle is a release or a working build marked unreleased |
| bindings | every binding with its proof summary |
| adapters used | by tool and plugin version (the field's name stays in release 1, gragas#2071) |
| presets | each preset by name and digest; the flattened upstream sets |
| checker code | each artifact by digest and publisher, with tests and review recorded |
| proof grid | every cell with result and level |
| coverage | both directions, with the upstream unit folded |
| conflicts | open entry conflicts, if the release was made over them on a yes |
| decisions, sources | every decision and source |
| history | the history entries up to this release |
| changelog | this release's entries with consumer actions |
| tested set | as in 16.1 |
| health | the seven numbers; two sets for an overlay |
| rule map | toward each mapped rulebook |
| rendering manifest | the renderings the release ships, with source hashes |
| content hash | the release's content hash |

## 17. The standing and status palette

Every screen, page and view paints the same words. Colour is a role name the host maps to its theme; the symbol and the word are always present in accessible mode and in plain terminals; nothing is told by colour alone.

### 17.1 Status (typed by people)

| Status | Word | Symbol |
|---|---|---|
| proposed | proposed | ? |
| trial | trial | ^ |
| active | active | = |
| retired | retired | x |

### 17.2 Standing (derived)

| Standing | Meaning | Colour role | Symbol |
|---|---|---|---|
| proposed | status proposed; no standing yet | violet | ? |
| enforced | an effective binding at rung 0–5 | green | + |
| reviewed | effective bindings at rung 6 only | blue | ~ |
| planned | must or should with no effective binding; a date may be set | amber | o |
| advisory | may with no effective binding | grey | - |
| retired | status retired | slate | x |

### 17.3 Marks (shown beside a standing)

| Mark | Meaning | Rendering | Symbol |
|---|---|---|---|
| trial | status trial; the binding ships at warning severity where the tool has one; sits on enforced, reviewed or planned, never on advisory or proposed | a hollow version of the standing's symbol in the explorer; the word "trial" after the standing in the terminal | ^ |
| blocked | a binding sits in an unresolved entry conflict | red outline; the word "blocked" | ! |
| unproven | a binding's proof is missing, failing or not run | dotted outline; the word "unproven" | * |

Examples as they read: "enforced, trial" (+^), "planned, trial" (o^), "planned, unproven" (o*), "planned, blocked" (o!), "reviewed, trial" (~^).

### 17.4 Observations (a project's view of an entry)

| Observation | Meaning | Symbol |
|---|---|---|
| as-set | the project runs the entry as the preset ships it | = |
| tighter | the project set a stricter value or severity | > |
| weaker | the project set a looser value or severity, or turned it off | < |
| absent | the preset entry is not in the effective config | _ |
| unmapped | the entry is in the effective config and no rule explains it | . |
| excluded | the rule was dropped by an overlay with a decision; provenance names both | / |

An entry still present in the effective config whose only rule is excluded shows "unmapped, was excluded by (overlay, decision)": the config holds it, no rule in the chain explains it, and the provenance says why.

### 17.5 Proof cells

pass (+), fail (!), not run (.), reviewed (~); each with its run level, isolated (i) or sandboxed (s), as a suffix.

## 18. End-to-end flows

Ten flows. Each step names the command, the domain in brackets, and the file touched. "Yes" means the plan was shown and accepted, and the record written as S43 says. The rulebook's repo is "the rulebook"; the consumer is "the project".

### Flow 1: write a rule, bind it, prove it, release

1. rule new [2]: the form asks statement, strength, scope, reasoning, a source (or "own decision"), a decision; the plan shows the rule as proposed. Yes → a rule file (or a block in the guide), the sources file, the decisions file, the history file.
2. A maintainer or an agent writes the example pair and the prose by plain file edit [1]: the fixture files, the rule's reasoning.
3. validate [1]: the prose lint and required fields pass; a listing, done.
4. bind [3]: the form asks tool, entry, rung, run point, severity, remedy, the pair's path; the plan shows the entry, "preset: ships at promotion" (a proposed rule ships in no preset) and "standing after proof and promotion: enforced, trial". Yes → the binding record, the history file.
5. prove [4]: the pair runs on every tool version the gate supplies at the declared path, at the recorded level; the versions found become the proposed tested set; the proof grid, done.
6. rule promote [2]: proposed → trial; the plan shows "ships at warning severity" and the preset diff; yes → the rule's status, the generated preset data, the history file.
7. release [5]: the plan shows the version (the first release of a rulebook takes the one identity declares, S68; each release after it the computed bump, here minor), the changelog entry with its consumer action and its plain sentence, the release note (S75), the preset, the tested set as proposed by prove, the health numbers, the content hash; yes → the release files committed and the tag made, never pushed; the bundle, the changelog, the presets, the tested set fixed in identity, the index entry text printed.
8. After at least one release and inside the window, rule promote [2] again: trial → active, with the decision; yes → the status, the history file. The next release [5] carries "trial ended, now active" with the consumer action.

### Flow 2: import an existing guide and config, grandfather, first release with a preset

1. import [2]: the form takes the guide's folder and the config files; the plan lists every drafted rule: those whose binding already exists in the config as "active, origin imported, source: the guide, decision auto-recorded, pair to write", the rest as "proposed, ships in no preset"; it names every to-do. Yes → rule files or blocks beside each section (several rules under one anchor where the section holds several), the bindings, the sources file (the guide as a source), the decisions file, the history file. No baseline is written: a guide's repo has no debt to count; its consumers' existing tool baselines are read at their init (flow 3).
2. validate [1]: the listing shows the to-dos as findings; in a rulebook that is "findings", exit 1, and the release will ask for a yes over them.
3. attention [2]: the whole list, grouped: pairs missing first.
4. prove [4]: every grandfathered binding without a pair shows "not run, no pair"; standing reads "planned, unproven" until the pair exists.
5. The maintainer writes pairs by plain file edit [1] and re-runs prove [4]; cells turn to pass; standing turns to enforced.
6. release [5]: the plan shows the version identity declares (S68) and the preset with every grandfathered entry, no weaker than the old config, the unproven ones listed as findings the owner accepts with the yes; health shows the proof pass rate honestly. Yes → the release files committed and the tag made, the bundle, the presets, the changelog.
7. Rules new since import follow flow 1 and pass through trial.

### Flow 3: a project's init, explain, check, verify, report

1. init [8]: in a project the form asks rulebook, version, source, kinds, parts, which agent folders to opt in and which reviewer format; the engine fetches the release and the withdrawn list, computes the release's content hash and verifies its digests, and finds any existing tool baseline file to adopt as the project's baseline; the plan shows the pin with the hash, which the yes takes as the first pin, the lock, the state file, any copied preset (for tools without extends), the agent files, the baseline adopted, and the wiring to add (hook lines, one CI line, one pointer line for AGENTS.md, the ignore line for the state file). Yes → the pin file, the lock file, the state file, the copied presets, the agent files, the baselines file where a tool has no file of its own; the wiring printed, never written.
2. The team adds the preset extends and the printed lines to its own configs and hooks: the project's files, by the team.
3. status [8]: what applies for these kinds and parts, by standing; versions against the tested set and the last fetch, both from the state file; done.
4. explain [3]: one config line → the rule, the observation ("weaker: severity lowered here"), the layer that set the preset entry; done.
5. check [8]: a listing of fidelity findings; "done, 3 findings" exit 0; with fail-on set to "weaker without waiver", "findings" exit 1 for the gate.
6. waive [8]: the form takes rule, path, reason, kind, owner, expiry, ticket; the plan shows the waiver and ends with the suppression text in the tool's syntax carrying the waiver id; yes → the waivers file; the team adds the comment.
7. verify [3]: the project's config set is mirrored into an isolated or sandboxed run and the pairs placed at their paths; a report; done.
8. report [8]: writes the report file, no identifier; the org's collector picks it up; done.

### Flow 4: upgrade and switch

Upgrade:
1. The dependency bot bumps the pin's version in a pull request: the pin file, by the bot.
2. upgrade [8]: the engine fetches the new release and the withdrawn list, verifies its digests and computes its content hash; the plan shows the new hash as a re-pin that only the yes makes, per changed rule the consumer action, the tool entries that change, waivers and baselines affected, the three-way diff for every copied preset, checker-code changes with a diff link, agent files to rewrite. Yes → the lock (new release, digests, yes entry, provenance), the state file (last fetch, withdrawn seen), the copied presets where the team accepted, the agent files.
3. check [8] on the branch: the findings the upgrade left, if any.

Switch:
1. A person changes the pin's rulebook and version: the pin file.
2. switch [8]: the engine fetches the target release; with a rule map the plan shows kept, tighter, weaker, gone and new per rule; without one it builds the plan from the two resolved presets and marks every match a guess; waivers and baselines re-keyed or orphaned; copied presets and agent files rewritten. Yes → the lock (provenance: from, to, map or preset diff), the waivers and baselines files re-keyed, the agent files.
3. status [8]: the new chain; warnings cleared if the old rulebook was archived.

### Flow 5: an overlay over a base, with an import by part

1. In an empty repository, init [terminal, then 1] asks "a project or a rulebook?"; the rulebook form asks name, kind (overlay), the parent at a pin, the own prefix, imports (another rulebook, the parts wanted), the starting version, owners, governance, the authoritative language, licences per kind, the first layers and parts, and the source form; the plan shows the identity and structure files, the guard's hook line and the gate's rule to add, and the first commands to run. Yes → the identity file, the structure file, the first history entry; the guard's wiring printed.
2. validate [1]: the overlay contract and the import are checked; a listing.
3. rule new [2] in the own prefix, as in flow 1. rebind [3] on a parent's rule that sets a stricter value or severity: the plan shows "tighter than parent" and writes the binding record under the overlay's prefix with the parent's rule id as its subject; a rebind that would weaken ends "refused". annotate is a prose edit [1]: the overlay's note or source beside the parent rule, in the overlay's files.
4. rule retire is not how a parent rule is dropped: rule exclude [2] takes the parent's id and a decision (new, or an existing one); the plan shows "excluded, decision cited" and the projects' observation it will produce; yes → the overlay's exclusion record, the decisions file, the history file.
5. conflicts [3]: an imported part binding the same entry as the base differently is listed; the maintainer resolves by rebind [3] (tightening the imported rule's binding, recorded under the overlay's prefix, S70) or a decision naming the winner.
6. release [5]: health shows two labelled sets (own rules; the resolved chain); the bundle carries the resolved chain with provenance.
7. A project pins the overlay (flow 3); explain [3] shows "excluded by the overlay, decision cited" for the dropped parent rule.
8. propose [2] from the overlay to the base: a tightening the overlay keeps becomes a structured change; the plan shows the proposal; yes → the proposals file, the proposal document printed, and the line that opens the pull request on the base's host where its governance accepts one.
9. The base releases. The dependency bot opens a pull request that changes the parent pin in the identity file (an upstream preset pin would change in its rung-0 binding or the flattened set); until upgrade runs, validate lists "pin changed without its upgrade entry". upgrade [2, in the rulebook]: the engine fetches the parent's release and withdrawn list and verifies the release against the parent's content hash in identity; the plan shows, per changed parent rule, the consumer action for the overlay; the overlay's tightenings the parent made redundant; its exclusions now conflicting where the parent retired the rule; new upstream entries to map or re-adopt; the proof to re-run; the bump the next release will compute. Yes → the history file and the regenerated flattened set. prove [4] and release [5] follow; projects pinned to the overlay see it in their own upgrade [8].
10. A lifted exclusion: rule include [2] with a decision when the parent rule is wanted again; yes → the exclusion record marked lifted, the decisions file, the history file.
11. An import added after init: the maintainer adds a second import to identity (rulebook, parts, pin, content hash) by plain file edit; validate [1] lists "pin changed without its upgrade entry"; upgrade [2, in the rulebook] fetches the release, verifies it against the hash, and plans the imported rules, their conflicts with the rulebook's own bindings, any checker code the import brings only if declared, the proof to re-run and the next bump; yes → the history entry, and the state "behind its pins" clears (S69).

### Flow 6: agent and review files

1. init [8] with agent folders opted in and a reviewer format chosen: the lock records both.
2. agents [9]: for each opted-in folder and each part in play, a file with the rules no rung 0–5 effective binding holds for those paths, inside the byte budget; one pointer line printed for AGENTS.md; the sums in the lock.
3. The team adds the hook line the handbook shows; on each edit the hook calls rules [9] with the path and the agent receives the rules for that file.
4. review [9]: the review file in the reviewer's format, with each rung-6 rule's id, review question, when it applies, "cite the rule id", and its pair as an eval case.
5. After an upgrade [8] that bound a review-held rule to a linter, agents and review [9] refresh: the rule drops out of both files; check [8] had listed the files as stale meanwhile; the rulebook's attention [2] listed the rule until the refresh.
6. The team changes its mind about a folder or its reviewer: agents or review [9] with the opt-in form; the plan shows the folders or format before and after and the files to add or remove; yes → the lock's opted-in folders or reviewer format, the files written or removed, a lock entry.

### Flow 7: signals to learn, and back to plan

1. Each project's report [8] goes to the org's collector; the org hands two batches to the rulebook's maintainer: files outside gragas.
2. signals [2]: the listing per rule: override rate, waiver age, false-positive rate, baseline age, and the change between the batches; one rule shows an override rate that jumped.
3. attention [2], given the same two batches: the rule is listed with "high override rate" beside its review-by date; without batches the line would read "override rate: no batches given".
4. rule confirm [2] at review-by, looking at signals and the proof grid: the owner chooses to rebind [3] to a looser entry with a decision new [2] that records why; or retire [2]; each a plan and a history entry.
5. release [5]: the changelog says what changed and what each project does about it; the dependency bot carries it; upgrade [8] in each project shows "your override is now the rule" and lists a redundant waiver as one line, closed in the same yes and kept in the file marked closed (S71): the loop closes at plan.
6. A project team with no overlay has its own door: propose [2, in the project] builds a proposal document with the structured change and the project's evidence (the waiver pattern, an incident's example), addressed to the rulebook whose prefix owns the rule, or to the pinned rulebook for a new rule; the plan shows the document; yes → a lock entry in the project, the document printed, and the line that opens the pull request or issue carrying it with the host's own tool where the target's governance accepts one; the target's owner applies it with rule new, bind or import.

### Flow 8: a correction (edit, fix, restore, supersede)

1. rule edit [2]: a lifecycle field (owner, review-by, scope, a relation, a source added); the plan shows before and after; yes → the rule file, the history file.
2. fix [2] after validate or check: the menu lists each finding with its offered correction (add a source, set a review-by, bind a planned rule, set a waiver's owner); choosing some makes one plan; yes → the files each correction touches, one history entry per correction in a rulebook, one lock entry in a project.
3. rule restore [2]: a retired rule returns as active with its history intact and a new review-by; the plan shows its bindings to re-prove; yes → the status, the history file; prove [4] follows.
4. decision supersede [2]: a new decision replaces an old one; the old stays, marked superseded, and every rule that cited it now cites both; the plan shows the rules affected; yes → the decisions file, the history file.
5. undo [2]: the last history entry is reverted by a new entry; nothing is removed; the plan shows exactly what returns.
6. A reworded statement that keeps its meaning is a plain file edit [1]; the next release's changelog says "statement reworded, no action". A statement whose meaning changes is rule replace [2]: a new id, the old retired with replaced-by, the consumer action in the changelog.

### Flow 9: the terminal, bare

1. A person types the bare command in a rulebook: the menu leads with the rulebook group (validate, rule, bind, prove, release, attention) and offers the project group below if the repo is also pinned.
2. In a project: the menu leads with status, explain, check, waive, upgrade, switch, report, agents.
3. In neither: init, which asks "a project or a rulebook?", and the handbook.
4. Every form ends in the plan screen of 15.3; every ending is one of the eight words. The bare command ends "done" at a terminal; when nobody can answer (a script, a pipe) it prints help and ends "usage".

### Flow 10: publish, and the stale page

1. docs [6] in the rulebook, or in the portal's repository (a project that pins the org's overlay): the docs profile of the host is chosen; the plan is not needed, since the export is a rendering; the command writes the pages, blocks, nav fragments and catalog rows into the host's tree and the rendering manifest beside them, recording each field the profile omits; "done", or "findings" when an omission is not yet accepted.
2. The portal owner accepts the omissions once [6]: docs in its accept form shows the omitted fields as a plan; yes → the manifest records each acceptance with who, when and the plan's digest, and in a project the yes also enters the lock (S74); the next docs run ends "done".
3. explorer [7] in the same place: builds the static bundle for the host's plugin or iframe from the rulebook bundle; done. The handbook shows how the host mounts it.
4. Someone edits an exported page by hand in the portal. check [8] in the portal's repository re-reads only the files the manifest names, finds the hash differs, and lists "edited rendering" as a finding; docs [6] run again rewrites the page from the rulebook and the finding clears.
5. The rulebook releases again; the portal's upgrade [8] plan lists "renderings to rewrite", pages among them where a rendering manifest exists; yes → docs [6] rewrites them and the manifest; the explorer bundle is rebuilt by explorer [7].
