// The rules view's selectors and filters, in node over the fixture
// bundle: counts agree with the Markdown part page, the eight filter names are
// the spec's eight, and the selectors read fields and compute nothing.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { BundleDocument, parse } from "@gragas/contract";
import { describe, expect, it } from "vitest";
import {
  applied,
  filterNames,
  filtersSchema,
  searchNames,
  without,
} from "../views/rules/filters.ts";
import {
  countsFor,
  factsFor,
  filterRules,
  groupByPart,
  partOf,
} from "../views/rules/selectors.ts";
import { documents, type Bundle } from "./support.js";

function bundle(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const here = (relative: string): string =>
  fileURLToPath(new URL(relative, import.meta.url));

/** The spec's eight `find` filters, read from the spec's own sentence. */
function specFilters(): readonly string[] {
  const text = readFileSync(here("../../../../docs/spec.md"), "utf8");
  const found = /rules by filter: ([a-z, ]+)\)/.exec(text);
  if (found?.[1] === undefined) throw new Error("the spec names no filters");
  return found[1].split(", ");
}

/** The rule rows of a part page's table: id, strength. */
function pageRows(page: string): readonly { id: string; strength: string }[] {
  const text = readFileSync(
    here(`../../../../internal/host/assets/contract/fixture/pages/${page}.md`),
    "utf8",
  );
  return text
    .split("\n")
    .filter((line) => /^\| [a-z]+\.[a-z]+\.\d+ \|/.test(line))
    .map((line) => line.split("|").map((cell) => cell.trim()))
    .map((cells) => ({ id: cells[1] ?? "", strength: cells[3] ?? "" }));
}

describe("the filter names", () => {
  it("are the spec's eight, in the spec's order", () => {
    expect([...filterNames]).toEqual(specFilters());
  });

  it("go red when a filter is renamed", () => {
    const planted = filterNames.map((n) => (n === "owner" ? "maintainer" : n));
    expect(planted).not.toEqual(specFilters());
  });

  it("are the keys of the search schema, with q", () => {
    expect(Object.keys(filtersSchema.shape)).toEqual([...searchNames]);
    expect(searchNames).toEqual([...specFilters(), "q"]);
  });

  it("refuse a value out of an enum and a key the view does not name", () => {
    expect(filtersSchema.safeParse({ strength: "always" }).success).toBe(false);
    expect(filtersSchema.safeParse({ colour: "red" }).success).toBe(false);
    expect(filtersSchema.safeParse({ q: "" }).success).toBe(false);
  });

  it("list what is applied and drop one by name", () => {
    const f = filtersSchema.parse({ q: "x", part: "go", strength: "must" });
    expect(applied(f).map((a) => a.name)).toEqual(["part", "strength", "q"]);
    expect(without(f, "part")).toEqual({ q: "x", strength: "must" });
  });
});

describe("filterRules and countsFor", () => {
  const b = bundle();
  const parts = b.structure.parts;

  it("shows every rule under no filter", () => {
    expect(filterRules(b.rules, {}, parts)).toHaveLength(b.rules.length);
    expect(countsFor(b.rules, {}, parts).shown).toBe(b.rules.length);
  });

  it("agrees with the Markdown page's rows for the same filter", () => {
    const rows = pageRows("go/errors");
    expect(rows.length).toBeGreaterThan(0);
    const filters = { part: "go", q: "go.errors." };
    const got = filterRules(b.rules, filters, parts).map((r) => r.id);
    expect(got).toEqual(rows.map((r) => r.id));
    expect(countsFor(b.rules, filters, parts).shown).toBe(rows.length);
    const must = filterRules(
      b.rules,
      { ...filters, strength: "must" as const },
      parts,
    );
    expect(must).toHaveLength(rows.filter((r) => r.strength === "must").length);
  });

  it("counts each facet over the other filters, not its own", () => {
    const c = countsFor(b.rules, { part: "go" }, parts);
    const all = countsFor(b.rules, {}, parts);
    expect(c.shown).toBe(c.facets.part.get("go"));
    expect(c.facets.part.get("go")).toBe(all.facets.part.get("go"));
    const strengths = [...c.facets.strength.values()].reduce(
      (a, n) => a + n,
      0,
    );
    expect(strengths).toBe(c.shown);
  });

  it("keeps a value with no match in the list, as a zero", () => {
    const c = countsFor(b.rules, { part: "go", strength: "may" }, parts);
    expect(c.facets.part.size).toBe(all(b).facets.part.size);
    expect([...c.facets.part.values()].some((n) => n === 0)).toBe(true);
  });

  it("matches a mark, a standing and a tool as the rule states them", () => {
    for (const r of filterRules(b.rules, { mark: "blocked" }, parts)) {
      expect(r.marks).toContain("blocked");
    }
    for (const r of filterRules(b.rules, { standing: "enforced" }, parts)) {
      expect(r.standing).toBe("enforced");
    }
    const tool = b.rules.flatMap((r) => r.bindings)[0]?.split("/")[0];
    if (tool === undefined) throw new Error("the fixture binds nothing");
    for (const r of filterRules(b.rules, { tool }, parts)) {
      expect(r.bindings.some((id) => id.startsWith(`${tool}/`))).toBe(true);
    }
  });
});

function all(b: Bundle): ReturnType<typeof countsFor> {
  return countsFor(b.rules, {}, b.structure.parts);
}

describe("groupByPart and factsFor", () => {
  const b = bundle();

  it("groups in the structure's order and loses no rule", () => {
    const groups = groupByPart(b.rules, b.structure.parts);
    expect(groups.flatMap((g) => g.rules)).toHaveLength(b.rules.length);
    const order = b.structure.parts.map((p) => p.slug);
    const seen = groups.map((g) => g.slug).filter((s) => order.includes(s));
    expect(seen).toEqual(order.filter((s) => seen.includes(s)));
    for (const g of groups) {
      for (const r of g.rules)
        expect(partOf(r, b.structure.parts)).toBe(g.slug);
    }
  });

  it("names the layer from the structure and reads the tier from bindings", () => {
    const rule = b.rules.find((r) => r.bindings.length > 0);
    if (rule === undefined) throw new Error("the fixture binds nothing");
    const facts = factsFor(rule, b.bindings, b.structure);
    expect(facts.tools.length).toBeGreaterThan(0);
    const held = b.bindings.filter((x) => x.rule === rule.id);
    const strongest =
      held.length === 0 ? null : Math.max(...held.map((x) => x.rung));
    expect(facts.tier).toBe(strongest);
    expect(facts.layer).not.toBeNull();
  });
});
