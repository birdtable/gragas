// The standing words and symbols equal spec 17 (web.content.1, docs.glossary).
// The Go messages module's half of this comes with it.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  createSay,
  markSymbols,
  observationSymbols,
  proofSymbols,
  runLevelSymbols,
  standingSymbols,
  statusSymbols,
} from "../messages/index.ts";

const spec = readFileSync(
  fileURLToPath(new URL("../../../../docs/spec.md", import.meta.url)),
  "utf8",
);

// The rows of the first table under a heading of section 17, as cell lists.
function rows(heading: string): string[][] {
  const start = spec.indexOf(`### ${heading}`);
  expect(start).toBeGreaterThan(-1);
  const body = spec.slice(start).split("\n").slice(1);
  const table: string[][] = [];
  for (const line of body) {
    if (line.startsWith("|")) {
      table.push(
        line
          .split("|")
          .slice(1, -1)
          .map((c) => c.trim()),
      );
    } else if (table.length > 0) {
      break;
    }
  }
  return table.slice(2);
}

const msg = createSay("en-GB");

describe("spec 17", () => {
  it("17.1 status: word and symbol", () => {
    const table = rows("17.1");
    expect(table).toHaveLength(4);
    for (const [name = "", word = "", symbol = ""] of table) {
      expect(msg(`status.${name}`)).toBe(word);
      expect(Object.entries(statusSymbols)).toContainEqual([name, symbol]);
    }
  });

  it("17.2 standing: word and symbol", () => {
    const table = rows("17.2");
    expect(table).toHaveLength(6);
    for (const row of table) {
      const [name = ""] = row;
      expect(msg(`standing.${name}`)).toBe(name);
      expect(Object.entries(standingSymbols)).toContainEqual([name, row[3]]);
    }
  });

  it("17.3 marks: word and symbol, and the example readings", () => {
    const table = rows("17.3");
    expect(table).toHaveLength(3);
    for (const row of table) {
      const [name = ""] = row;
      expect(msg(`mark.${name}`)).toBe(name);
      expect(Object.entries(markSymbols)).toContainEqual([name, row[3]]);
    }
    expect(
      msg("standing.marked", {
        standing: msg("standing.planned"),
        mark: msg("mark.blocked"),
      }),
    ).toBe("planned, blocked");
  });

  it("17.4 observations: word and symbol", () => {
    const table = rows("17.4");
    expect(table).toHaveLength(6);
    for (const row of table) {
      const [name = ""] = row;
      expect(msg(`observation.${name}`)).toBe(name);
      expect(Object.entries(observationSymbols)).toContainEqual([name, row[2]]);
    }
  });

  it("17.5 proof cells: pass +, fail !, not run ., reviewed ~, isolated i, sandboxed s", () => {
    expect(proofSymbols).toEqual({
      pass: "+",
      fail: "!",
      "not-run": ".",
      reviewed: "~",
    });
    expect(runLevelSymbols).toEqual({ isolated: "i", sandboxed: "s" });
    expect(spec).toContain("pass (+), fail (!), not run (.), reviewed (~)");
    expect([
      msg("proof.pass"),
      msg("proof.fail"),
      msg("proof.not-run"),
      msg("proof.reviewed"),
    ]).toEqual(["pass", "fail", "not run", "reviewed"]);
  });

  it("names the six states web.states.1 lists", () => {
    expect(
      ["ready", "loading", "empty", "error", "stale", "disconnected"].map((s) =>
        msg(`state.${s}`),
      ),
    ).toEqual(["ready", "loading", "empty", "error", "stale", "disconnected"]);
  });
});
