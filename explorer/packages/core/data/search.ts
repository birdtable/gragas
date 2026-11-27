// The search index of the loaded bundle (spec S38): rules, tools,
// entries, checks and terms, built with MiniSearch from the one parsed bundle
// and nothing else. Pure: it reads no network, no storage and no document, so
// a search can reach nothing the bundle does not hold.
//
// An entry is a tool key, "tool/key", the form a rule's bindings use; a rule is
// found by the keys it binds as well as by its own words, so typing an entry
// finds the entry and the rules that hold it. A check is a named checker
// artifact of the bundle.
import type { BundleDocument } from "@gragas/contract";
import MiniSearch from "minisearch";
import type * as z from "zod";
import type { Destination } from "./destination.ts";

type Bundle = z.output<typeof BundleDocument>;

export type SearchKind = "rule" | "tool" | "entry" | "check" | "term";

/** The order the groups come in, and so the order the palette shows them. */
export const searchKinds: readonly SearchKind[] = [
  "rule",
  "tool",
  "entry",
  "check",
  "term",
];

interface Indexed {
  /** Unique across kinds: the kind and the key. */
  readonly uid: string;
  readonly kind: SearchKind;
  readonly key: string;
  /** The id the link names: the key, but a check links to its tool. */
  readonly target: string;
  /** What a result is called: ids and names, matched with the most weight. */
  readonly names: string;
  /** Prose and the names of what it is joined to. */
  readonly text: string;
}

export interface SearchHit {
  readonly kind: SearchKind;
  readonly key: string;
  readonly destination: Destination;
  readonly score: number;
}

export interface SearchGroup {
  readonly kind: SearchKind;
  /** Every match of the kind, whatever `hits` shows. */
  readonly count: number;
  readonly hits: readonly SearchHit[];
}

export interface SearchResults {
  readonly query: string;
  /** Every match, over all kinds: the number the field says (web.states.14). */
  readonly total: number;
  /** One group per kind that has a match, in `searchKinds` order. */
  readonly groups: readonly SearchGroup[];
}

export interface SearchIndex {
  readonly size: number;
  readonly index: MiniSearch<Indexed>;
}

function destinationOf(kind: SearchKind, target: string): Destination {
  switch (kind) {
    case "rule":
      return { route: "rule", id: target };
    case "term":
      return { route: "relations", kind: "term", id: target };
    case "tool":
    case "check":
    case "entry":
      return { route: "relations", kind: "tool-entry", id: target };
  }
}

function distinct(values: readonly string[]): string[] {
  return [...new Set(values)];
}

function documentsOf(bundle: Bundle): Indexed[] {
  const rules: Indexed[] = bundle.rules.map((rule) => ({
    uid: `rule:${rule.id}`,
    kind: "rule",
    key: rule.id,
    target: rule.id,
    names: [rule.id, ...rule.aliases].join(" "),
    text: [rule.statement, rule.reasoning, ...rule.bindings].join(" "),
  }));
  const terms: Indexed[] = bundle.terms.map((term) => ({
    uid: `term:${term.slug}`,
    kind: "term",
    key: term.slug,
    target: term.slug,
    names: term.slug,
    text: [term.definition, term.literal].join(" "),
  }));
  const tools = distinct([
    ...bundle.bindings.map((b) => b.tool),
    ...bundle.presets.map((p) => p.tool),
    ...bundle.checker_code.map((c) => c.tool),
  ]).map((tool): Indexed => ({
    uid: `tool:${tool}`,
    kind: "tool",
    key: tool,
    target: tool,
    names: tool,
    text: "",
  }));
  const entries = distinct(
    bundle.bindings.map((b) => `${b.tool}/${b.key}`),
  ).map((entry): Indexed => ({
    uid: `entry:${entry}`,
    kind: "entry",
    key: entry,
    target: entry,
    names: entry,
    text: bundle.bindings
      .filter((b) => `${b.tool}/${b.key}` === entry)
      .map((b) => b.rule)
      .join(" "),
  }));
  const checks: Indexed[] = bundle.checker_code.map((check) => ({
    uid: `check:${check.name}`,
    kind: "check",
    key: check.name,
    target: check.tool,
    names: check.name,
    text: [check.publisher, check.tool].join(" "),
  }));
  return [...rules, ...tools, ...entries, ...checks, ...terms];
}

export function buildSearchIndex(bundle: Bundle): SearchIndex {
  const index = new MiniSearch<Indexed>({
    idField: "uid",
    fields: ["names", "text"],
    storeFields: ["kind", "key", "target"],
  });
  const documents = documentsOf(bundle);
  index.addAll(documents);
  return { size: documents.length, index };
}

/**
 * The matches of a query, grouped by kind. Every word must match (a prefix of a
 * word does, and so does a word a letter off); a blank query matches nothing,
 * so an empty field lists nothing and says zero. `perGroup` caps the hits a
 * group carries and not its count.
 */
export function searchBundle(
  found: SearchIndex,
  query: string,
  perGroup = Number.POSITIVE_INFINITY,
): SearchResults {
  if (query.trim() === "") return { query, total: 0, groups: [] };
  const matches = found.index.search(query, {
    prefix: true,
    fuzzy: 0.15,
    combineWith: "AND",
    boost: { names: 3 },
  });
  const hits: SearchHit[] = [];
  for (const match of matches) {
    const kind: unknown = match["kind"];
    const key: unknown = match["key"];
    const target: unknown = match["target"];
    if (typeof key !== "string" || typeof target !== "string") continue;
    const known = searchKinds.find((k) => k === kind);
    if (known === undefined) continue;
    hits.push({
      kind: known,
      key,
      destination: destinationOf(known, target),
      score: match.score,
    });
  }
  const groups = searchKinds.flatMap((kind): SearchGroup[] => {
    const of = hits.filter((hit) => hit.kind === kind);
    return of.length === 0
      ? []
      : [{ kind, count: of.length, hits: of.slice(0, perGroup) }];
  });
  return { query, total: hits.length, groups };
}
