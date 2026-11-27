// The rule view's selectors: what one rule says, why, how it is
// held, how it is proven and how it got here, read from the parsed bundle and
// nothing else. Pure, so a node test judges it: no network, no document, no
// clock. A value the bundle does not state is a dash with its reason, never a
// blank or a zero (web.states.5). The proof cells are the bindings' own proof
// summaries; the proof grid is never read.
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import type { Destination } from "../../data/index.ts";
import { dash, dashable, type Dashable } from "../../messages/index.ts";

type Bundle = z.output<typeof BundleDocument>;
type Rule = Bundle["rules"][number];
type Binding = Bundle["bindings"][number];

/** A binding's id is the one its proof names: rule, tool and key. */
export function bindingId(binding: Binding): string {
  return `${binding.rule}/${binding.tool}/${binding.key}`;
}

export interface ProofCell {
  readonly result: "pass" | "fail" | "not run" | "reviewed";
  readonly level: "fixture" | "pair" | "project";
  readonly toolVersion: string;
}

export interface BindingRow {
  readonly id: string;
  readonly tool: string;
  readonly key: string;
  /** `tool/key`, the entry the relations view is asked about. */
  readonly entry: string;
  readonly rung: number;
  readonly runPoint: string;
  readonly remedy: Dashable<string>;
  readonly pairPath: Dashable<string>;
  /**
   * The cells of the binding's proof summary. The contract has one ProofResult per binding, so one cell, and not
   * one per tested tool version (contract gap 3).
   */
  readonly proof: Dashable<readonly ProofCell[]>;
  /** For a rung-6 binding (S51): the review question and when it applies. */
  readonly review: {
    readonly question: Dashable<string>;
    readonly applies: string;
  } | null;
  /** The waiver syntax the bundle records for the binding's adapter. */
  readonly waiver: Dashable<string>;
  readonly destination: Destination;
}

export interface DecisionRow {
  readonly slug: string;
  readonly title: string;
  readonly who: string;
  readonly date: string;
  readonly destination: Destination;
}

export interface SourceRow {
  readonly slug: string;
  readonly title: Dashable<string>;
  readonly kind: Dashable<string>;
  readonly locator: Dashable<string>;
  readonly attribution: Dashable<string>;
  readonly destination: Destination;
}

export interface HistoryRow {
  readonly number: number;
  readonly kind: string;
  readonly when: string;
  readonly who: string;
  readonly plan: string;
  readonly status: Dashable<string>;
  readonly decision: Dashable<string>;
}

export interface RelationRow {
  readonly kind: string;
  readonly rule: string;
  readonly destination: Destination;
}

export interface RuleView {
  readonly id: string;
  readonly statement: string;
  readonly strength: Rule["strength"];
  readonly status: Rule["status"];
  readonly standing: Rule["standing"];
  readonly marks: Rule["marks"];
  readonly aliases: readonly string[];
  /** The anatomy: the contract states a definition and a reasoning; the rest is a gap of it. */
  readonly anatomy: {
    readonly definition: string;
    readonly reasoning: Dashable<string>;
    readonly pros: Dashable<string>;
    readonly cons: Dashable<string>;
    readonly example: Dashable<string>;
  };
  readonly pairs: readonly {
    readonly number: number;
    readonly targetType: string;
  }[];
  readonly bindings: readonly BindingRow[];
  readonly decisions: readonly DecisionRow[];
  readonly sources: readonly SourceRow[];
  readonly history: readonly HistoryRow[];
  readonly translations: readonly {
    readonly language: string;
    readonly field: string;
    readonly stale: boolean;
  }[];
  readonly relations: readonly RelationRow[];
  /** Set for a retired rule: the panel and the link to its replacement. */
  readonly retired: {
    readonly replacement: Dashable<Destination>;
  } | null;
  /** The relations view of this rule. */
  readonly relationsLink: Destination;
}

export type RuleViewResult =
  | { readonly kind: "found"; readonly view: RuleView }
  | { readonly kind: "redirect"; readonly to: string }
  | { readonly kind: "missing"; readonly id: string };

const notRun = {
  "no pair": "proof.reason.no-pair",
  "tool missing": "proof.reason.tool-missing",
  "limit hit": "proof.reason.limit-hit",
  "sandbox missing": "proof.reason.sandbox-missing",
} as const;

function proofOf(binding: Binding): Dashable<readonly ProofCell[]> {
  const proof = binding.proof;
  if (proof === null) return dash("absent.not-run");
  if (proof.result === "not run") {
    return dash(notRun[proof.reason]);
  }
  return {
    kind: "value",
    value: [
      {
        result: proof.result,
        level: proof.run_level,
        toolVersion: proof.tool_version,
      },
    ],
  };
}

function bindingRow(binding: Binding): BindingRow {
  const question =
    typeof binding.value === "string" && binding.value !== ""
      ? binding.value
      : null;
  return {
    id: bindingId(binding),
    tool: binding.tool,
    key: binding.key,
    entry: `${binding.tool}/${binding.key}`,
    rung: binding.rung,
    runPoint: binding.run_point,
    remedy: dashable(
      binding.remedy === "" ? null : binding.remedy,
      "absent.none-recorded",
    ),
    pairPath: dashable(
      binding.pair_path === "" ? null : binding.pair_path,
      "absent.none-recorded",
    ),
    proof: proofOf(binding),
    review:
      binding.rung === 6
        ? {
            question: dashable(question, "absent.not-in-bundle"),
            applies: binding.run_point,
          }
        : null,
    waiver: dash("absent.not-in-bundle"),
    destination: {
      route: "relations",
      kind: "binding",
      id: bindingId(binding),
    },
  };
}

/**
 * The view of the rule `id`: found, a redirect when `id` is an alias of a rule
 * (the URL moves to the current id), or missing. The bundle's order is kept
 * everywhere; nothing is sorted by position.
 */
export function ruleView(bundle: Bundle, id: string): RuleViewResult {
  const rule = bundle.rules.find((r) => r.id === id);
  if (rule === undefined) {
    const aliased = bundle.rules.find((r) => r.aliases.includes(id));
    return aliased === undefined
      ? { kind: "missing", id }
      : { kind: "redirect", to: aliased.id };
  }
  const sources = new Map(bundle.sources.map((s) => [s.slug, s]));
  const entity = `rule:${rule.id}`;
  const view: RuleView = {
    id: rule.id,
    statement: rule.statement,
    strength: rule.strength,
    status: rule.status,
    standing: rule.standing,
    marks: [...rule.marks],
    aliases: rule.aliases,
    anatomy: {
      definition: rule.statement,
      reasoning: dashable(
        rule.reasoning === "" ? null : rule.reasoning,
        "absent.none-recorded",
      ),
      pros: dash("absent.not-in-bundle"),
      cons: dash("absent.not-in-bundle"),
      example: dash("absent.not-in-bundle"),
    },
    pairs: rule.pairs.map((p) => ({
      number: p.number,
      targetType: p.target_type,
    })),
    bindings: bundle.bindings.filter((b) => b.rule === rule.id).map(bindingRow),
    decisions: bundle.decisions
      .filter((d) => d.cites.rules.includes(rule.id))
      .map((d) => ({
        slug: d.slug,
        title: d.title,
        who: d.who,
        date: d.date,
        destination: { route: "relations", kind: "decision", id: d.slug },
      })),
    sources: rule.sources.map((slug): SourceRow => {
      const source = sources.get(slug);
      return {
        slug,
        title: dashable(source?.title, "absent.not-in-bundle"),
        kind: dashable(source?.kind, "absent.not-in-bundle"),
        locator: dashable(
          source === undefined || source.locator === "" ? null : source.locator,
          "absent.none-recorded",
        ),
        attribution: dashable(
          source === undefined || source.attribution === ""
            ? null
            : source.attribution,
          "absent.none-recorded",
        ),
        destination: { route: "relations", kind: "source", id: slug },
      };
    }),
    history: bundle.history
      .filter((h) => h.entity === entity)
      .map((h) => ({
        number: h.number,
        kind: h.kind,
        when: h.when,
        who: h.who,
        plan: h.plan,
        // The status a step moved the rule to is in `after`, an untyped record the field list cannot name (a
        // contract gap, raised on the PR): a dash until the contract types it.
        status: dash("absent.not-in-bundle"),
        decision: dashable(h.decision, "absent.none-recorded"),
      })),
    translations: rule.translations.map((t) => ({
      language: t.language,
      field: t.field,
      stale: t.stale,
    })),
    relations: rule.relations.map((r) => ({
      kind: r.kind,
      rule: r.rule,
      destination: { route: "rule", id: r.rule },
    })),
    retired:
      rule.status === "retired"
        ? {
            replacement: dashable(
              rule.replaced_by === null
                ? null
                : ({ route: "rule", id: rule.replaced_by } as const),
              "absent.none-recorded",
            ),
          }
        : null,
    relationsLink: { route: "relations", kind: "rule", id: rule.id },
  };
  return { kind: "found", view };
}
