// The attention view's selectors: which rules need a person now, and
// why, in eight kinds. They read the one parsed bundle and the day the caller
// gives (S55: gragas keeps no timer, so the view reads the clock once and hands
// the day down), and nothing else (web.structure.5). The same bundle and day give
// the same list. "High override rate" needs two batches of reports, which the
// bundle never carries, so that kind is always absent and the view says so.
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import { dashable, type Dashable } from "../../messages/index.ts";

type Bundle = z.output<typeof BundleDocument>;

/** The eight kinds, in the order the page groups them. */
export const attentionKinds = [
  "unproven",
  "missing",
  "overdue",
  "past-zero-by",
  "trial-overrun",
  "blocked",
  "override-rate",
  "review-held",
] as const;
export type AttentionKind = (typeof attentionKinds)[number];

/** What a rule in the "missing" kind lacks. */
export type Missing = "pair" | "source";

export interface Filters {
  readonly kind?: AttentionKind | undefined;
}

export interface AttentionItem {
  readonly rule: string;
  readonly kind: AttentionKind;
  readonly status: string;
  /** The date the item turns on: review-by, zero-by; a dash for the others. */
  readonly date: Dashable<string>;
  /** Who answers for the rule; a dash with its reason when nobody does. */
  readonly owner: Dashable<string>;
  /** For "missing": what the rule lacks, in the order pair, source. */
  readonly missing: readonly Missing[];
  /** For "missing": whether the rule has a decision; the bundle has no field for it yet. */
  readonly decision: Extract<Dashable<never>, { kind: "dash" }> | null;
  /** For "trial-overrun": releases shipped in trial against the window. */
  readonly trial: { readonly shipped: number; readonly allowed: number } | null;
  /** For "review-held": the standing the shipped rendering still shows. */
  readonly shown: string | null;
  /** For "review-held": the rendering's path. */
  readonly path: string | null;
}

export interface AttentionGroup {
  readonly kind: AttentionKind;
  readonly items: readonly AttentionItem[];
  /** The kind cannot be computed from the bundle (override rate). */
  readonly needsBatches: boolean;
}

export interface StaleRendering {
  readonly rule: string;
  readonly path: string;
  readonly target: string;
  readonly shown: string;
  readonly now: string;
}

export interface OpenConflict {
  readonly rule: Dashable<string>;
  readonly entity: string;
  readonly message: string;
  readonly fix: Dashable<string>;
}

export interface AttentionView {
  /** The day the dates were judged against (`YYYY-MM-DD`). */
  readonly today: string;
  /** One group per kind, in order, a kind with no item kept with `items: []`. */
  readonly groups: readonly AttentionGroup[];
  /** The groups the filter leaves. */
  readonly shownGroups: readonly AttentionGroup[];
  /** Rules with at least one item; a rule counts once. */
  readonly ruleCount: number;
  readonly itemCount: number;
  /** Every live rule is proposed, so the whole book is listed (S13). */
  readonly allProposed: boolean;
  readonly staleRenderings: readonly StaleRendering[];
  readonly conflicts: readonly OpenConflict[];
}

const reviewTargets: ReadonlySet<string> = new Set(["agent", "review"]);

const isLive = (status: string): boolean => status !== "retired";

function ownerOf(owner: string): Dashable<string> {
  return dashable(owner === "" ? null : owner, "absent.none-recorded");
}

function blank(
  rule: Bundle["rules"][number],
  kind: AttentionKind,
  date: string | null,
): AttentionItem {
  return {
    rule: rule.id,
    kind,
    status: rule.status,
    date: dashable(date, "absent.none-recorded"),
    owner: ownerOf(rule.owner),
    missing: [],
    decision: null,
    trial: null,
    shown: null,
    path: null,
  };
}

function missingOf(rule: Bundle["rules"][number]): readonly Missing[] {
  const out: Missing[] = [];
  // A may needs a pair only once it is bound (S24).
  if (
    rule.pairs.length === 0 &&
    (rule.strength !== "may" || rule.bindings.length > 0)
  ) {
    out.push("pair");
  }
  if (rule.sources.length === 0) out.push("source");
  return out;
}

/** The renderings an agent or a reviewer reads, whose shipped standing no longer matches the rule's. */
export function staleRenderings(bundle: Bundle): readonly StaleRendering[] {
  const standing = new Map(bundle.rules.map((r) => [r.id, r.standing]));
  const out: StaleRendering[] = [];
  for (const manifest of bundle.rendering_manifest) {
    for (const rendering of manifest.renderings) {
      for (const entry of rendering.rules) {
        const now = standing.get(entry.rule);
        if (now !== undefined && now !== entry.standing) {
          out.push({
            rule: entry.rule,
            path: rendering.path,
            target: manifest.target,
            shown: entry.standing,
            now,
          });
        }
      }
    }
  }
  return out.sort(
    (a, b) => a.rule.localeCompare(b.rule) || a.path.localeCompare(b.path),
  );
}

function itemsOf(bundle: Bundle, today: string): readonly AttentionItem[] {
  const governance = bundle.identity.governance;
  const held = new Map<string, StaleRendering>();
  for (const stale of staleRenderings(bundle)) {
    // A linter now holds the rule: it is enforced, yet a review-text rendering
    // still says it is not (S13). The first such rendering names it.
    if (
      reviewTargets.has(stale.target) &&
      stale.now === "enforced" &&
      !held.has(stale.rule)
    ) {
      held.set(stale.rule, stale);
    }
  }
  const out: AttentionItem[] = [];
  for (const rule of bundle.rules.filter((r) => isLive(r.status))) {
    if (rule.marks.includes("unproven")) {
      out.push(blank(rule, "unproven", null));
    }
    const missing = missingOf(rule);
    if (missing.length > 0)
      out.push({
        ...blank(rule, "missing", null),
        missing,
        decision: { kind: "dash", reason: "absent.decision-gap" },
      });
    if (
      rule.status === "active" &&
      rule.review_by !== null &&
      rule.review_by < today
    ) {
      out.push(blank(rule, "overdue", rule.review_by));
    }
    if (
      rule.standing === "planned" &&
      rule.zero_by !== null &&
      rule.zero_by < today
    ) {
      out.push(blank(rule, "past-zero-by", rule.zero_by));
    }
    if (
      rule.status === "trial" &&
      rule.trial_shipped.length >= governance.trial_releases
    ) {
      out.push({
        ...blank(rule, "trial-overrun", null),
        trial: {
          shipped: rule.trial_shipped.length,
          allowed: governance.trial_releases,
        },
      });
    }
    if (rule.marks.includes("blocked")) out.push(blank(rule, "blocked", null));
    const stale = held.get(rule.id);
    if (stale !== undefined) {
      out.push({
        ...blank(rule, "review-held", null),
        shown: stale.shown,
        path: stale.path,
      });
    }
  }
  return out;
}

/** The rules that need a person on `today`, grouped by why, and the two further lists. */
export function attentionView(
  bundle: Bundle,
  today: string,
  filters: Filters = {},
): AttentionView {
  const items = itemsOf(bundle, today);
  const groups = attentionKinds.map((kind): AttentionGroup => ({
    kind,
    items: items
      .filter((i) => i.kind === kind)
      .sort((a, b) => a.rule.localeCompare(b.rule)),
    needsBatches: kind === "override-rate",
  }));
  const live = bundle.rules.filter((r) => isLive(r.status));
  return {
    today,
    groups,
    shownGroups: groups.filter(
      (g) => filters.kind === undefined || g.kind === filters.kind,
    ),
    ruleCount: new Set(items.map((i) => i.rule)).size,
    itemCount: items.length,
    allProposed: live.length > 0 && live.every((r) => r.status === "proposed"),
    staleRenderings: staleRenderings(bundle),
    conflicts: bundle.conflicts
      .filter((c) => c.kind === "conflict")
      .map((c) => ({
        rule: dashable(c.rule, "absent.none-recorded"),
        entity: c.entity,
        message: c.message,
        fix: dashable(c.fix === "" ? null : c.fix, "absent.none-recorded"),
      })),
  };
}
