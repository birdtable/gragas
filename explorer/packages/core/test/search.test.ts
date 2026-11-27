// The search index over the fixture bundle (spec S38), in node. The
// spec's example types `no-explicit`; the fixture bundle has no such entry, so
// these tests use its own: the golangci-lint entry `errorlint` and the rule
// go.errors.1 that binds it.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { BundleDocument, parse } from "@gragas/contract";
import { describe, expect, it, vi } from "vitest";
import {
  buildSearchIndex,
  destinationPath,
  searchBundle,
  searchKinds,
  type SearchResults,
} from "../index.ts";
import { documents, type Bundle } from "./support.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const keys = (results: SearchResults, kind: string): string[] =>
  results.groups.find((g) => g.kind === kind)?.hits.map((h) => h.key) ?? [];

describe("searching the loaded bundle", () => {
  const index = buildSearchIndex(fixture());

  it("finds an entry and the rules that hold it, grouped by kind", () => {
    const found = searchBundle(index, "errorlint");
    expect(keys(found, "entry")).toEqual(["golangci-lint/errorlint"]);
    expect(keys(found, "rule").sort()).toEqual([
      "base.errors.1",
      "go.errors.1",
    ]);
    expect(found.groups.map((g) => g.kind)).toEqual(["rule", "entry"]);
    expect(found.total).toBe(3);
  });

  it("gives each hit a typed destination, the entry's with its slash", () => {
    const found = searchBundle(index, "errorlint");
    const entry = found.groups.find((g) => g.kind === "entry")?.hits[0];
    expect(entry?.destination).toEqual({
      route: "relations",
      kind: "tool-entry",
      id: "golangci-lint/errorlint",
    });
    expect(entry && destinationPath(entry.destination)).toBe(
      "/relations/tool-entry/golangci-lint/errorlint",
    );
    const rule = found.groups.find((g) => g.kind === "rule")?.hits[0];
    expect(rule?.destination).toEqual({ route: "rule", id: rule?.key });
  });

  it("finds a rule by its id, by an alias and by its words", () => {
    expect(keys(searchBundle(index, "go.errors.1"), "rule")).toContain(
      "go.errors.1",
    );
    expect(keys(searchBundle(index, "go.errs.1"), "rule")).toContain(
      "go.errors.1",
    );
    const statement = fixture().rules.find((r) => r.id === "go.errors.1");
    const word = statement?.statement.split(/\s+/)[0] ?? "";
    expect(word).not.toBe("");
    expect(keys(searchBundle(index, word), "rule")).toContain("go.errors.1");
  });

  it("finds a tool, a check and a term, each on its own route", () => {
    const tool = searchBundle(index, "stylelint");
    expect(keys(tool, "tool")).toEqual(["stylelint"]);
    expect(
      tool.groups.find((g) => g.kind === "tool")?.hits[0]?.destination,
    ).toEqual({
      route: "relations",
      kind: "tool-entry",
      id: "stylelint",
    });
    const check = searchBundle(index, "wrapcheck-plugin");
    expect(keys(check, "check")).toEqual(["wrapcheck-plugin"]);
    expect(
      check.groups.find((g) => g.kind === "check")?.hits[0]?.destination,
    ).toEqual({ route: "relations", kind: "tool-entry", id: "golangci-lint" });
    const term = searchBundle(index, "gate");
    expect(keys(term, "term")).toEqual(["gate"]);
    expect(
      term.groups.find((g) => g.kind === "term")?.hits[0]?.destination,
    ).toEqual({ route: "relations", kind: "term", id: "gate" });
  });

  it("matches a prefix, and every word of the query", () => {
    expect(keys(searchBundle(index, "errorli"), "entry")).toEqual([
      "golangci-lint/errorlint",
    ]);
    expect(searchBundle(index, "errorlint nosuchword").total).toBe(0);
  });

  it("lists nothing for a blank query and says zero", () => {
    for (const blank of ["", "   "]) {
      expect(searchBundle(index, blank)).toEqual({
        query: blank,
        total: 0,
        groups: [],
      });
    }
    expect(searchBundle(index, "zzzzqqqq").total).toBe(0);
  });

  it("keeps the groups in the order of searchKinds and the counts honest", () => {
    const found = searchBundle(index, "wrap");
    const order = found.groups.map((g) => searchKinds.indexOf(g.kind));
    expect(order).toEqual([...order].sort((a, b) => a - b));
    expect(found.groups.reduce((sum, g) => sum + g.count, 0)).toBe(found.total);
    const capped = searchBundle(index, "wrap", 1);
    expect(capped.total).toBe(found.total);
    expect(capped.groups.every((g) => g.hits.length <= 1)).toBe(true);
    expect(capped.groups.map((g) => g.count)).toEqual(
      found.groups.map((g) => g.count),
    );
  });

  it("is the same index for the same bundle", () => {
    const again = buildSearchIndex(fixture());
    expect(again.size).toBe(index.size);
    expect(searchBundle(again, "wrap")).toEqual(searchBundle(index, "wrap"));
  });

  it("indexes an empty bundle without a hit", () => {
    const empty = buildSearchIndex({
      ...fixture(),
      rules: [],
      terms: [],
      bindings: [],
      presets: [],
      checker_code: [],
    });
    expect(empty.size).toBe(0);
    expect(searchBundle(empty, "wrap").total).toBe(0);
  });

  it("makes no network request: a search only reads the index", () => {
    const requests = vi.fn(() => {
      throw new Error("a search reached the network");
    });
    vi.stubGlobal("fetch", requests);
    try {
      const fresh = buildSearchIndex(fixture());
      searchBundle(fresh, "errorlint");
      expect(requests).not.toHaveBeenCalled();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("names no way to the network, a document or storage in its source", () => {
    const here = fileURLToPath(new URL("../data/", import.meta.url));
    for (const file of ["search.ts", "destination.ts"]) {
      const text = readFileSync(`${here}${file}`, "utf8")
        .replace(/\/\*[\s\S]*?\*\//g, "")
        .replace(/\/\/.*$/gm, "");
      expect(text).not.toMatch(
        /\b(fetch|XMLHttpRequest|WebSocket|sendBeacon|EventSource|document|window|localStorage|sessionStorage|indexedDB|navigator|location)\b/,
      );
    }
  });
});
