// The request handlers: built from the fixture bundle's
// documents, serving them over the network seam, and kept out of the
// published entry.
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { setupServer } from "msw/node";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import * as z from "zod";
import { fixtureDocuments, fixtureHandlers } from "../src/fixtures/index.js";
import { BundleDocument, ReportDocument, parse } from "../src/main.js";

const base = "https://explorer.test/";
const server = setupServer(...fixtureHandlers(base));
beforeAll(() => server.listen({ onUnhandledFrame: "error" }));
afterAll(() => server.close());

describe("the fixture handlers", () => {
  // The documents are read once and fetched in parallel, and the test names its
  // own 30 s timeout: on the shared Linux host the serial run took over vitest's
  // 5 s default.
  it(
    "serve every fixture document, which parses as it does from disk",
    { timeout: 30_000 },
    async () => {
      const documents = fixtureDocuments();
      await Promise.all(
        [...documents.keys()].map(async (name) => {
          const response = await fetch(`${base}${name}.json`);
          expect(response.status).toBe(200);
          expect(await response.json()).toEqual(documents.get(name));
        }),
      );
      const bundle = parse(
        BundleDocument,
        await (await fetch(`${base}bundle.json`)).json(),
      );
      const report = parse(
        ReportDocument,
        await (await fetch(`${base}report-weakened.json`)).json(),
      );
      expect([bundle.status, report.status]).toEqual(["ok", "ok"]);
    },
  );

  it("answer a document that is not a fixture with an error, never a hand stub", async () => {
    await expect(fetch(`${base}nothing.json`)).rejects.toThrow();
  });
});

const here = (path: string): string =>
  fileURLToPath(new URL(path, import.meta.url));
const ManifestSchema = z.object({
  exports: z.record(z.string(), z.unknown()),
  files: z.array(z.string()),
  publishConfig: z.object({ exports: z.record(z.string(), z.unknown()) }),
  dependencies: z.record(z.string(), z.string()),
  devDependencies: z.record(z.string(), z.string()),
});

describe("the published entry", () => {
  const manifest = ManifestSchema.parse(
    JSON.parse(readFileSync(here("../package.json"), "utf8")),
  );

  it("leaves the fixtures out: of the published exports and of the tarball", () => {
    expect(Object.keys(manifest.publishConfig.exports)).toEqual(["."]);
    expect(Object.keys(manifest.exports)).toEqual([".", "./fixtures"]);
    expect(manifest.files).toContain("!dist/fixtures");
  });

  it("does not reach MSW: no file the library build compiles imports it, and it is no dependency", () => {
    const src = here("../src");
    const reached: string[] = [];
    const reaches =
      /(?:from|import)\s*\(?\s*["'](?:msw|(?:\.\.?\/)+(?:[\w./-]*\/)?fixtures)/;
    for (const name of readdirSync(src, {
      recursive: true,
      encoding: "utf8",
    })) {
      if (!name.endsWith(".ts") || name.startsWith("fixtures")) continue;
      if (reaches.test(readFileSync(join(src, name), "utf8"))) {
        reached.push(name);
      }
    }
    expect(reached).toEqual([]);
    expect(Object.keys(manifest.dependencies)).not.toContain("msw");
    expect(manifest.devDependencies["msw"]).toMatch(/^\d+\.\d+\.\d+$/);
  });
});
