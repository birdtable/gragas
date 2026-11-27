// The relations view's selectors over the fixture bundle, in node:
// the graph one hop around an entity, one more hop around an expanded node, the
// list that carries the same edges as the graph, the cap on a huge
// neighbourhood, and the one route form the palette and this view share.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  BundleDocument,
  FieldListSchema,
  parse,
  recordReads,
  unlisted,
} from "@gragas/contract";
import { describe, expect, it } from "vitest";
import {
  buildSearchIndex,
  destinationPath,
  parseDestination,
  relationKinds,
  searchBundle,
  type RelationKind,
} from "../../data/index.ts";
import { documents, type Bundle } from "../../test/support.ts";
import {
  defaultCap,
  relationsView,
  type RelationsGraph,
  type RelationsOptions,
} from "./selectors.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const bundle = fixture();

function graph(
  kind: RelationKind,
  id: string,
  options: RelationsOptions = {},
  from: Bundle = bundle,
): RelationsGraph {
  const result = relationsView(from, kind, id, options);
  if (result.kind !== "found") throw new Error(`${kind}:${id} is missing`);
  return result.graph;
}

const edgeSet = (g: RelationsGraph): string[] =>
  g.edges.map((e) => `${e.from} ${e.label} ${e.to}`).sort();
const listSet = (g: RelationsGraph): string[] =>
  g.list.map((r) => `${r.from.key} ${r.label} ${r.to.key}`).sort();

describe("a decision", () => {
  it("lists the rules it excludes, and the sources it cites", () => {
    const excluded = graph("decision", "house-call");
    expect(
      excluded.edges.filter((e) => e.label === "excludes").map((e) => e.to),
    ).toEqual(["rule:std.errors.2", "rule:std.errors.3"]);
    expect(excluded.nodes.find((n) => n.id === "std.errors.2")?.known).toBe(
      false,
    );
    const cites = graph("decision", "wrap-errors");
    expect(
      cites.edges
        .filter((e) => e.from === "decision:wrap-errors")
        .map((e) => `${e.label} ${e.to}`),
    ).toEqual([
      "cites source:effective-go",
      "cites source:error-study",
      "cites rule:go.errors.1",
      "cites tool-entry:golangci-lint/wrapcheck",
    ]);
  });
});

describe("expanding a node", () => {
  it("adds one hop around it and keeps what was shown", () => {
    const first = graph("source", "effective-go");
    expect(first.nodes.map((n) => n.key)).toEqual([
      "source:effective-go",
      "rule:go.errors.1",
      "decision:wrap-errors",
    ]);
    const next = graph("source", "effective-go", {
      expanded: ["decision:wrap-errors"],
    });
    expect(next.nodes.map((n) => n.key)).toEqual(
      expect.arrayContaining([
        ...first.nodes.map((n) => n.key),
        "source:error-study",
        "tool-entry:golangci-lint/wrapcheck",
      ]),
    );
    expect(edgeSet(next)).toEqual(expect.arrayContaining(edgeSet(first)));
    expect(next.edges.length).toBeGreaterThan(first.edges.length);
  });

  it("ignores a key that is not on the graph, so a stale link opens nothing new", () => {
    const first = graph("source", "effective-go");
    expect(
      edgeSet(
        graph("source", "effective-go", { expanded: ["rule:nowhere.x.1"] }),
      ),
    ).toEqual(edgeSet(first));
  });
});

describe("the list for assistive technology", () => {
  it("carries the same edges as the graph, for every entity of the bundle", () => {
    const ends: [RelationKind, string][] = [
      ["rule", "go.errors.1"],
      ["decision", "wrap-errors"],
      ["decision", "house-call"],
      ["source", "effective-go"],
      ["binding", "go.errors.1/golangci-lint/wrapcheck"],
      ["tool-entry", "golangci-lint/wrapcheck"],
      ["tool-entry", "golangci-lint"],
      ["part", "go"],
      ["term", "wrap"],
    ];
    for (const [kind, id] of ends) {
      const g = graph(kind, id, { expanded: [`${kind}:${id}`] });
      expect({ at: `${kind}:${id}`, list: listSet(g) }).toEqual({
        at: `${kind}:${id}`,
        list: edgeSet(g),
      });
      expect(g.edges.length).toBeGreaterThan(0);
    }
  });
});

describe("a huge neighbourhood", () => {
  const rule = bundle.rules[0];
  if (rule === undefined) throw new Error("the fixture has rules");
  const crowded: Bundle = {
    ...bundle,
    rules: Array.from({ length: 40 }, (_, i) => ({
      ...rule,
      id: `crowd.many.${String(i + 1)}`,
      aliases: [],
      sources: [],
      bindings: [],
      scope: { ...rule.scope, parts: ["crowd"] },
    })),
    bindings: [],
  };

  it("is capped, and says how many it left out", () => {
    const g = graph("part", "crowd", {}, crowded);
    expect(g.edges).toHaveLength(defaultCap);
    expect(g.more).toEqual({ "part:crowd": 40 - defaultCap });
    expect(graph("part", "crowd", { cap: 5 }, crowded).edges).toHaveLength(5);
  });

  it("shows every neighbour of a node that is expanded past the cap", () => {
    const g = graph("part", "crowd", { expanded: ["part:crowd"] }, crowded);
    expect(g.edges).toHaveLength(40);
    expect(g.more).toEqual({});
  });

  it("shows the rest when the control is used with a larger cap", () => {
    const g = graph("part", "crowd", { cap: 100 }, crowded);
    expect(g.edges).toHaveLength(40);
    expect(g.more).toEqual({});
  });
});

describe("a tool, a binding and a term", () => {
  it("a tool lists its entries; an entry its binding and the rules that hold it", () => {
    const tool = graph("tool-entry", "golangci-lint");
    expect(tool.edges.every((e) => e.label === "holds")).toBe(true);
    expect(tool.nodes.map((n) => n.id)).toContain("golangci-lint/errorlint");
    const entry = graph("tool-entry", "golangci-lint/errorlint");
    expect(entry.nodes.map((n) => n.key)).toEqual(
      expect.arrayContaining([
        "binding:go.errors.1/golangci-lint/errorlint",
        "binding:base.errors.1/golangci-lint/errorlint",
      ]),
    );
  });

  it("a term lists the rules that use it, as the bundle's prose does", () => {
    const g = graph("term", "wrap");
    const using = bundle.rules
      .filter((r) =>
        /(?<![A-Za-z0-9])wrap(?![A-Za-z0-9])/i.test(
          `${r.statement} ${r.reasoning}`,
        ),
      )
      .map((r) => `rule:${r.id}`);
    expect(using.length).toBeGreaterThan(0);
    expect(
      g.edges.filter((e) => e.label === "defined-in").map((e) => e.to),
    ).toEqual(using);
  });

  it("an entity the bundle does not name is missing", () => {
    expect(relationsView(bundle, "rule", "no.such.1")).toEqual({
      kind: "missing",
      entity: "rule",
      id: "no.such.1",
    });
  });
});

describe("the route the palette writes and this view reads", () => {
  it("opens a graph for every relations link a search can give", () => {
    const index = buildSearchIndex(bundle);
    const links = [
      "errorlint",
      "wrap",
      "stylelint",
      "wrapcheck-plugin",
      "gate",
      "golangci",
    ].flatMap((query) =>
      searchBundle(index, query).groups.flatMap((g) =>
        g.hits.map((hit) => hit.destination),
      ),
    );
    const read = links.map((d) => parseDestination(destinationPath(d)));
    expect(read).toEqual(links);
    const opened = links.flatMap((d) =>
      d.route === "relations" ? [relationsView(bundle, d.kind, d.id).kind] : [],
    );
    expect(opened.length).toBeGreaterThan(3);
    expect(new Set(opened)).toEqual(new Set(["found"]));
  });

  it("names the seven kinds the issue lists, and no other", () => {
    expect(relationKinds).toEqual([
      "rule",
      "decision",
      "source",
      "binding",
      "tool-entry",
      "part",
      "term",
    ]);
  });
});

describe("what the view reads", () => {
  it("is listed in the consumer-driven field list", () => {
    const recorder = recordReads(bundle);
    for (const [kind, id] of [
      ["rule", "go.errors.1"],
      ["decision", "house-call"],
      ["source", "effective-go"],
      ["term", "wrap"],
      ["part", "go"],
    ] as const) {
      relationsView(recorder.value, kind, id, { expanded: [`${kind}:${id}`] });
    }
    const list = FieldListSchema.parse(
      JSON.parse(
        readFileSync(
          fileURLToPath(
            new URL("../../../../contract/fields.json", import.meta.url),
          ),
          "utf8",
        ),
      ),
    );
    expect(unlisted(list, "relations", "bundle", recorder.reads())).toEqual([]);
  });
});

describe("the direction of each edge label", () => {
  // Every edge of the bundle, by one big expansion around each kind of entity.
  const all = (): Set<string> => {
    const seen = new Set<string>();
    for (const [kind, id] of [
      ["rule", "go.errors.1"],
      ["rule", "base.errors.1"],
      ["decision", "wrap-errors"],
      ["decision", "house-call"],
      ["tool-entry", "golangci-lint"],
      ["tool-entry", "golangci-lint/wrapcheck"],
      ["binding", "go.errors.1/golangci-lint/wrapcheck"],
      ["part", "go"],
      ["term", "wrap"],
    ] as const) {
      const g = graph(kind, id, { cap: 1000 });
      for (const e of g.edges) seen.add(`${e.from} ${e.label} ${e.to}`);
    }
    return seen;
  };
  const edges = all();

  it("cites runs from a decision to a source, a rule or a tool entry, and from a rule to a source", () => {
    expect(edges).toContain("decision:wrap-errors cites source:effective-go");
    expect(edges).toContain("decision:wrap-errors cites rule:go.errors.1");
    expect(edges).toContain(
      "decision:wrap-errors cites tool-entry:golangci-lint/wrapcheck",
    );
    expect(edges).toContain("rule:go.errors.1 cites source:effective-go");
    expect(edges).not.toContain(
      "source:effective-go cites decision:wrap-errors",
    );
    expect(edges).not.toContain("rule:go.errors.1 cites decision:wrap-errors");
  });

  it("excludes runs from a decision to a rule", () => {
    expect(edges).toContain("decision:house-call excludes rule:std.errors.2");
    expect(edges).not.toContain(
      "rule:std.errors.2 excludes decision:house-call",
    );
  });

  it("covers runs from a part to a rule", () => {
    expect(edges).toContain("part:go covers rule:go.errors.1");
    expect(edges).not.toContain("rule:go.errors.1 covers part:go");
  });

  it("holds runs from a rule to a binding, and from a tool to its entries", () => {
    expect(edges).toContain(
      "rule:go.errors.1 holds binding:go.errors.1/golangci-lint/wrapcheck",
    );
    expect(edges).toContain(
      "tool-entry:golangci-lint holds tool-entry:golangci-lint/wrapcheck",
    );
    expect(edges).not.toContain(
      "binding:go.errors.1/golangci-lint/wrapcheck holds rule:go.errors.1",
    );
  });

  it("defined-in runs from a binding to its tool entry, and from a term to a rule that uses it", () => {
    expect(edges).toContain(
      "binding:go.errors.1/golangci-lint/wrapcheck defined-in tool-entry:golangci-lint/wrapcheck",
    );
    expect(edges).toContain("term:wrap defined-in rule:go.errors.1");
    expect(edges).not.toContain("rule:go.errors.1 defined-in term:wrap");
  });

  it("uses no label but the five of the issue", () => {
    const labels = new Set([...edges].map((e) => e.split(" ")[1]));
    expect([...labels].sort()).toEqual([
      "cites",
      "covers",
      "defined-in",
      "excludes",
      "holds",
    ]);
  });
});
