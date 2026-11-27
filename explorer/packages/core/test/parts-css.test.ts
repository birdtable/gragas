// The parts' stylesheets: no 100vh (dvh instead), reduced motion
// honoured, a 44 px target where the pointer is coarse, and one focus ring token
// that no part restates. Core imports no here view.
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { keyedFiles, read, sourceFiles } from "./guard.ts";

const core = fileURLToPath(new URL("../", import.meta.url));
const keyed = keyedFiles(`${core}parts`, [".css"]);
const sheets = [...keyed.values()];
const strip = (s: string): string => s.replace(/\/\*[\s\S]*?\*\//g, "");

describe("the parts' stylesheets", () => {
  it("hold a sheet for every part and the shared ones", () => {
    expect(sheets.length).toBeGreaterThanOrEqual(19);
  });

  it("never size by 100vh", () => {
    expect(sheets.filter((f) => /\b100vh\b/.test(strip(read(f))))).toEqual([]);
  });

  it("honour reduced motion and the coarse pointer in the shared sheet", () => {
    const base = strip(read(`${core}parts/base.css`));
    expect(base).toMatch(/prefers-reduced-motion:\s*reduce/);
    expect(base).toMatch(/pointer:\s*coarse/);
    expect(base).toMatch(/44px/);
  });

  it("restate no focus ring outside the shared sheet", () => {
    const outside = [...keyed]
      .filter(([key]) => key !== "base.css")
      .filter(([, f]) => /:focus-visible/.test(strip(read(f))));
    expect(outside).toEqual([]);
  });
});

describe("core", () => {
  it("imports no here view", () => {
    const code = sourceFiles(core, [".ts", ".tsx"]).filter(
      (f) => !f.includes("node_modules"),
    );
    expect(
      code.filter((f) => /standalone\/here|views\/here/.test(read(f))),
    ).toEqual([]);
  });
});
