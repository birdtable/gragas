// Spec section 17: the status words, standings, marks, observations and proof
// cells of the package equal the tables of docs/spec.md, and every colour role
// they name is a colour of tokens.json (web.tokens.*).
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  marks,
  observations,
  proofCells,
  standings,
  statuses,
} from "../src/index.ts";

const root = (path: string): string =>
  fileURLToPath(new URL(`../../../../${path}`, import.meta.url));
const spec = readFileSync(root("docs/spec.md"), "utf8");
const parsed: unknown = JSON.parse(
  readFileSync(new URL("../tokens.json", import.meta.url), "utf8"),
);
const tokens: Record<string, Record<string, unknown>> = {};
if (typeof parsed === "object" && parsed !== null) {
  for (const [name, group] of Object.entries(parsed)) {
    if (typeof group === "object" && group !== null)
      tokens[name] = { ...group };
  }
}

// section is the text of one "### 17.n" part of the spec, up to the next heading.
export function section(markdown: string, number: string): string {
  const lines = markdown.split("\n");
  const from = lines.findIndex((l) => l.startsWith(`### ${number} `));
  if (from < 0) throw new Error(`docs/spec.md has no section ${number}`);
  const rest = lines.slice(from + 1);
  const to = rest.findIndex((l) => l.startsWith("#"));
  return (to < 0 ? rest : rest.slice(0, to)).join("\n");
}

// rows is the body rows of the one table in a section: header and rule skipped,
// each row split on its bars.
export function rows(text: string): string[][] {
  const table = text
    .split("\n")
    .filter((l) => l.startsWith("|"))
    .map((l) =>
      l
        .slice(1, l.lastIndexOf("|"))
        .split("|")
        .map((c) => c.trim()),
    );
  if (table.length < 3) throw new Error("the section holds no table");
  return table.slice(2);
}

describe("spec section 17 against the package", () => {
  it("17.1: each status has its word and its symbol", () => {
    expect(
      rows(section(spec, "17.1")).map(([s, w, y]) => ({
        status: s,
        word: w,
        symbol: y,
      })),
    ).toEqual(statuses);
  });

  it("17.2: each standing has its colour role and its symbol", () => {
    expect(
      rows(section(spec, "17.2")).map(([s, , r, y]) => ({
        standing: s,
        role: r,
        symbol: y,
      })),
    ).toEqual(standings);
  });

  it("17.3: each mark has its symbol", () => {
    expect(
      rows(section(spec, "17.3")).map(([m, , , y]) => ({ mark: m, symbol: y })),
    ).toEqual(marks);
  });

  it("17.4: each observation has its symbol", () => {
    expect(
      rows(section(spec, "17.4")).map(([o, , y]) => ({
        observation: o,
        symbol: y,
      })),
    ).toEqual(observations);
  });

  it("17.5: each proof cell has its symbol", () => {
    // "pass (+), fail (!), not run (.), reviewed (~); each with its run level..."
    const list = section(spec, "17.5").trim().split(";")[0] ?? "";
    const found = list.split(", ").map((part) => {
      const m = /^(.+) \((.)\)$/.exec(part);
      return { cell: m?.[1], symbol: m?.[2] };
    });
    expect(found).toEqual(proofCells);
  });
});

describe("the colour roles the tables name", () => {
  it("every standing's role is a standing colour, and the six are all there", () => {
    const defined = Object.keys(tokens["standing"] ?? {}).sort();
    expect(standings.map((s) => s.role).sort()).toEqual(defined);
  });

  it("every mark and observation is a colour of its own name", () => {
    expect(marks.map((m) => m.mark).sort()).toEqual(
      Object.keys(tokens["marks"] ?? {}).sort(),
    );
    expect(observations.map((o) => o.observation).sort()).toEqual(
      Object.keys(tokens["observations"] ?? {}).sort(),
    );
  });

  it("the table check fails on a table that changed (the guard's plant)", () => {
    const moved = spec.replace(
      "| enforced | an effective binding at rung 0–5 | green | + |",
      "| enforced | an effective binding at rung 0–5 | green | # |",
    );
    expect(moved).not.toBe(spec);
    expect(
      rows(section(moved, "17.2")).map(([s, , r, y]) => ({
        standing: s,
        role: r,
        symbol: y,
      })),
    ).not.toEqual(standings);
  });
});
