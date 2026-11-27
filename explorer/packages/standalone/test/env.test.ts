// No variable is read at run time (react.config.1): the build's names are read
// in env.ts, once and typed, and nowhere else.
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { expect, test } from "vitest";
import { readBuild } from "../src/env.ts";

const src = fileURLToPath(new URL("../src", import.meta.url));

test("only env.ts reads import.meta.env or process.env", () => {
  const offenders = readdirSync(src, { recursive: true })
    .map(String)
    .filter((f) => /\.tsx?$/.test(f) && f !== "env.ts")
    .filter((f) =>
      /import\.meta\.env|process\.env/.test(readFileSync(join(src, f), "utf8")),
    );
  expect(offenders).toEqual([]);
});

test("the schema reads the one name, and refuses a value that is not a URL", () => {
  expect(readBuild({}).dataBase).toBeUndefined();
  expect(
    readBuild({ VITE_GRAGAS_DATA_BASE: "https://x.test/d/" }).dataBase,
  ).toBe("https://x.test/d/");
  expect(() => readBuild({ VITE_GRAGAS_DATA_BASE: "not a url" })).toThrow();
});
