// The rules view's selectors: pure functions over the parsed
// bundle. They filter, count and group the rules the bundle states; they never
// recompute a standing, a mark or a coverage figure, only read the fields the
// contract carries (web.structure.5: no document, window or storage).
//
// Two readings are fixed here and `find` takes the same ones:
// a rule with no kind in its scope applies to every kind, so it matches any
// `kind`; and a rule's part is the structure part whose slug, with its slashes
// as dots, leads the rule's id (the longest such slug), else the id's first
// segment.
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import {
  filterNames,
  searchNames,
  type FilterName,
  type Filters,
  type SearchName,
} from "./filters.ts";

type Bundle = z.output<typeof BundleDocument>;
export type Rule = Bundle["rules"][number];
export type Binding = Bundle["bindings"][number];
export type Part = Bundle["structure"]["parts"][number];
export type Structure = Bundle["structure"];

/** The tool a binding id (`tool/key`) names. */
export function toolOf(bindingId: string): string {
  return bindingId.split("/")[0] ?? bindingId;
}

const dotted = (slug: string): string => slug.replaceAll("/", ".");

/** The slug of the part a rule belongs to. */
export function partOf(rule: Rule, parts: readonly Part[]): string {
  const led = parts
    .filter((part) => rule.id.startsWith(`${dotted(part.slug)}.`))
    .sort((a, b) => b.slug.length - a.slug.length)[0];
  return led?.slug ?? rule.id.split(".")[0] ?? rule.id;
}

function valuesOf(
  rule: Rule,
  name: FilterName,
  parts: readonly Part[],
): readonly string[] {
  switch (name) {
    case "part":
      return [partOf(rule, parts)];
    case "kind":
      return rule.scope.kinds;
    case "strength":
      return [rule.strength];
    case "standing":
      return [rule.standing];
    case "mark":
      return rule.marks;
    case "tool":
      return [...new Set(rule.bindings.map(toolOf))];
    case "owner":
      return rule.owner === "" ? [] : [rule.owner];
    case "status":
      return [rule.status];
  }
}

function holds(
  rule: Rule,
  name: FilterName,
  wanted: string,
  parts: readonly Part[],
): boolean {
  // A rule scoped to no kind applies to all of them.
  if (name === "kind" && rule.scope.kinds.length === 0) return true;
  return valuesOf(rule, name, parts).includes(wanted);
}

function mentions(rule: Rule, text: string): boolean {
  const needle = text.toLowerCase();
  return [rule.id, rule.statement, ...rule.aliases].some((field) =>
    field.toLowerCase().includes(needle),
  );
}

function matches(
  rule: Rule,
  filters: Filters,
  parts: readonly Part[],
  skip?: SearchName,
): boolean {
  for (const name of filterNames) {
    const wanted = filters[name];
    if (name === skip || wanted === undefined) continue;
    if (!holds(rule, name, wanted, parts)) return false;
  }
  const text = filters.q;
  return skip === "q" || text === undefined || mentions(rule, text);
}

/** The rules that meet every filter in force, in the bundle's order. */
export function filterRules(
  rules: readonly Rule[],
  filters: Filters,
  parts: readonly Part[],
): readonly Rule[] {
  return rules.filter((rule) => matches(rule, filters, parts));
}

export interface Counts {
  /** Every rule the bundle holds. */
  readonly total: number;
  /** The rules that meet every filter: "41 of 76" is shown over total. */
  readonly shown: number;
  /**
   * For each filter, how many rules each value would show were that filter
   * the only one set differently: the other filters hold. Values with none
   * are kept with a zero so the list does not move under the reader's hand.
   */
  readonly facets: Readonly<Record<FilterName, ReadonlyMap<string, number>>>;
}

function facetValues(
  name: FilterName,
  rules: readonly Rule[],
  parts: readonly Part[],
): readonly string[] {
  const seen = new Set<string>(
    name === "part" ? parts.map((part) => part.slug) : [],
  );
  for (const rule of rules) {
    for (const value of valuesOf(rule, name, parts)) seen.add(value);
  }
  return name === "part" ? [...seen] : [...seen].sort();
}

/** The live counts of the view: the match total and each facet's values. */
export function countsFor(
  rules: readonly Rule[],
  filters: Filters,
  parts: readonly Part[],
): Counts {
  const facet = (name: FilterName): ReadonlyMap<string, number> => {
    const others = rules.filter((rule) => matches(rule, filters, parts, name));
    return new Map(
      facetValues(name, rules, parts).map((value) => [
        value,
        others.filter((rule) => holds(rule, name, value, parts)).length,
      ]),
    );
  };
  return {
    total: rules.length,
    shown: filterRules(rules, filters, parts).length,
    facets: {
      part: facet("part"),
      kind: facet("kind"),
      strength: facet("strength"),
      standing: facet("standing"),
      mark: facet("mark"),
      tool: facet("tool"),
      owner: facet("owner"),
      status: facet("status"),
    },
  };
}

export interface Group {
  readonly slug: string;
  readonly name: string;
  readonly rules: readonly Rule[];
}

/** The rules grouped by part in the structure's order; a part with none is left out. */
export function groupByPart(
  rules: readonly Rule[],
  parts: readonly Part[],
): readonly Group[] {
  const bySlug = new Map<string, Rule[]>();
  for (const rule of rules) {
    const slug = partOf(rule, parts);
    bySlug.set(slug, [...(bySlug.get(slug) ?? []), rule]);
  }
  const known = [...parts]
    .sort((a, b) => a.order - b.order)
    .flatMap((part) => {
      const held = bySlug.get(part.slug);
      bySlug.delete(part.slug);
      return held === undefined
        ? []
        : [{ slug: part.slug, name: part.name, rules: held }];
    });
  const unknown = [...bySlug].map(([slug, held]) => ({
    slug,
    name: slug,
    rules: held,
  }));
  return [...known, ...unknown];
}

/** What a row says about the tools and the layer behind its rule. */
export interface RuleFacts {
  readonly tools: readonly string[];
  /** The rung of the strongest binding, or null when nothing holds the rule. */
  readonly tier: number | null;
  /** A binding of the rule carries a remedy. */
  readonly fix: boolean;
  /** The layer the rule's part sits in, or null when the structure lacks the part. */
  readonly layer: string | null;
}

export function factsFor(
  rule: Rule,
  bindings: readonly Binding[],
  structure: Structure,
): RuleFacts {
  const held = bindings.filter((binding) => binding.rule === rule.id);
  const part = structure.parts.find(
    (p) => p.slug === partOf(rule, structure.parts),
  );
  const layer =
    part === undefined
      ? null
      : (structure.layers.find((l) => l.slug === part.layer)?.name ?? null);
  return {
    tools: [...new Set(rule.bindings.map(toolOf))],
    tier: held.length === 0 ? null : Math.max(...held.map((b) => b.rung)),
    fix: held.some((binding) => binding.remedy !== ""),
    layer,
  };
}

export { filterNames, searchNames };
