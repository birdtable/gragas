// The relations view's selectors: the graph one hop around a rule,
// decision, source, binding, tool entry, part or term, built from ids already in
// the bundle and nothing else. Pure, so a node test judges it. The page draws
// these nodes and edges as its own SVG and, for assistive technology, the same
// edges as a list; both come from the one `edges` below, so they cannot differ.
//
// The edges, each from the entity that says it to the entity it names:
//   cites       decision -> source, rule or tool entry it cites; rule -> source it rests on
//   excludes    decision -> rule an overlay excluded by it
//   covers      part -> rule whose scope names the part
//   holds       rule -> binding it holds; tool -> each of its entries
//   defined-in  binding -> the tool entry it is set in; term -> rule that uses the term
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import type { RelationKind } from "../../data/index.ts";

type Bundle = z.output<typeof BundleDocument>;

export type EdgeLabel =
  "cites" | "excludes" | "covers" | "holds" | "defined-in";

export interface RelationNode {
  readonly kind: RelationKind;
  readonly id: string;
  /** `kind:id`, the node's identity in `edges` and in `expanded`. */
  readonly key: string;
  readonly label: string;
  /** False when the bundle names the entity but does not hold it (a parent's rule). */
  readonly known: boolean;
}

export interface RelationEdge {
  readonly from: string;
  readonly to: string;
  readonly label: EdgeLabel;
}

/** One row of the list that stands for the graph: the same edge, in words' place of lines. */
export interface RelationListRow {
  readonly from: RelationNode;
  readonly label: EdgeLabel;
  readonly to: RelationNode;
}

export interface RelationsGraph {
  readonly center: RelationNode;
  readonly nodes: readonly RelationNode[];
  readonly edges: readonly RelationEdge[];
  readonly list: readonly RelationListRow[];
  /** Neighbours of a node that the cap left out, by node key: the "N more" control. */
  readonly more: Readonly<Record<string, number>>;
}

export type RelationsResult =
  | { readonly kind: "found"; readonly graph: RelationsGraph }
  | {
      readonly kind: "missing";
      readonly entity: RelationKind;
      readonly id: string;
    };

export interface RelationsOptions {
  /** Node keys that were expanded: one more hop around each. */
  readonly expanded?: readonly string[];
  /** The most neighbours one node shows; the rest are `more`. */
  readonly cap?: number;
}

export const defaultCap = 12;

export const nodeKey = (kind: RelationKind, id: string): string =>
  `${kind}:${id}`;

interface Universe {
  readonly nodes: ReadonlyMap<string, RelationNode>;
  readonly edges: readonly RelationEdge[];
}

function wordsOf(slug: string): RegExp {
  const escaped = slug
    .replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
    .replace(/-/g, "[- ]");
  return new RegExp(`(?<![A-Za-z0-9])${escaped}(?![A-Za-z0-9])`, "i");
}

/** Every entity the bundle names and every edge between them, in the bundle's order. */
function universe(bundle: Bundle): Universe {
  const nodes = new Map<string, RelationNode>();
  const edges: RelationEdge[] = [];
  const add = (
    kind: RelationKind,
    id: string,
    label: string,
    known: boolean,
  ): string => {
    const key = nodeKey(kind, id);
    const held = nodes.get(key);
    if (held === undefined || (!held.known && known)) {
      nodes.set(key, { kind, id, key, label, known });
    }
    return key;
  };
  const edge = (from: string, label: EdgeLabel, to: string): void => {
    edges.push({ from, label, to });
  };
  for (const rule of bundle.rules) add("rule", rule.id, rule.id, true);
  for (const decision of bundle.decisions) {
    add("decision", decision.slug, decision.title, true);
  }
  for (const source of bundle.sources) {
    add("source", source.slug, source.title, true);
  }
  for (const part of bundle.structure.parts) {
    add("part", part.slug, part.name, true);
  }
  for (const term of bundle.terms) add("term", term.slug, term.slug, true);
  for (const b of bundle.bindings) {
    const entry = `${b.tool}/${b.key}`;
    const binding = add("binding", `${b.rule}/${entry}`, entry, true);
    const toolKey = add("tool-entry", b.tool, b.tool, true);
    const entryKey = add("tool-entry", entry, entry, true);
    edge(nodeKey("rule", b.rule), "holds", binding);
    edge(binding, "defined-in", entryKey);
    if (!edges.some((e) => e.from === toolKey && e.to === entryKey)) {
      edge(toolKey, "holds", entryKey);
    }
  }
  for (const tool of [
    ...bundle.presets.map((p) => p.tool),
    ...bundle.checker_code.map((c) => c.tool),
  ]) {
    add("tool-entry", tool, tool, true);
  }
  for (const rule of bundle.rules) {
    const from = nodeKey("rule", rule.id);
    for (const slug of rule.sources) {
      edge(from, "cites", add("source", slug, slug, false));
    }
    for (const part of rule.scope.parts) {
      edge(add("part", part, part, false), "covers", from);
    }
    const prose = `${rule.statement} ${rule.reasoning}`;
    for (const term of bundle.terms) {
      if (wordsOf(term.slug).test(prose)) {
        edge(nodeKey("term", term.slug), "defined-in", from);
      }
    }
  }
  for (const decision of bundle.decisions) {
    const from = nodeKey("decision", decision.slug);
    for (const slug of decision.cites.sources) {
      edge(from, "cites", add("source", slug, slug, false));
    }
    for (const id of decision.cites.rules) {
      edge(from, "cites", add("rule", id, id, false));
    }
    for (const entry of decision.cites.bindings) {
      edge(from, "cites", add("tool-entry", entry, entry, false));
    }
  }
  for (const exclusion of bundle.exclusions) {
    edge(
      add("decision", exclusion.decision, exclusion.decision, false),
      "excludes",
      add("rule", exclusion.rule, exclusion.rule, false),
    );
  }
  return { nodes, edges };
}

/**
 * The graph one hop around `kind` and `id`, with one more hop around each key in
 * `expanded`. A node shows at most `cap` neighbours, and says how many it left
 * out, so a part with many rules is a control and never a frozen page.
 */
export function relationsView(
  bundle: Bundle,
  kind: RelationKind,
  id: string,
  options: RelationsOptions = {},
): RelationsResult {
  const { nodes, edges } = universe(bundle);
  const centerKey = nodeKey(kind, id);
  const center = nodes.get(centerKey);
  if (center === undefined) return { kind: "missing", entity: kind, id };
  const cap = options.cap ?? defaultCap;
  const shown = new Map<string, RelationNode>([[centerKey, center]]);
  const kept = new Map<string, RelationEdge>();
  const left = new Map<string, number>();
  // The first hop keeps `cap` neighbours; expanding a node lifts its cap, so the
  // "more" control shows the rest instead of finding no room left.
  const reach = (key: string, capped: boolean): void => {
    const incident = edges.filter((e) => e.from === key || e.to === key);
    const taken = incident.filter((e) => {
      const at = `${e.from}\u0000${e.label}\u0000${e.to}`;
      return kept.has(at);
    });
    const fresh = incident.filter((e) => !taken.includes(e));
    const room = capped ? Math.max(0, cap - taken.length) : fresh.length;
    for (const e of fresh.slice(0, room)) {
      kept.set(`${e.from}\u0000${e.label}\u0000${e.to}`, e);
      for (const end of [e.from, e.to]) {
        const node = nodes.get(end);
        if (node !== undefined) shown.set(end, node);
      }
    }
    if (fresh.length > room) left.set(key, fresh.length - room);
    else left.delete(key);
  };
  reach(centerKey, true);
  for (const key of options.expanded ?? []) {
    if (shown.has(key)) reach(key, false);
  }
  const keptEdges = [...kept.values()];
  const list = keptEdges.flatMap((e): RelationListRow[] => {
    const from = shown.get(e.from);
    const to = shown.get(e.to);
    return from === undefined || to === undefined
      ? []
      : [{ from, label: e.label, to }];
  });
  return {
    kind: "found",
    graph: {
      center,
      nodes: [...shown.values()],
      edges: keptEdges,
      list,
      more: Object.fromEntries(left),
    },
  };
}
