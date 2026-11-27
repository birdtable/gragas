// The test-only entry `@gragas/contract/fixtures`: request
// handlers built from the fixture bundle's own documents, so no test writes a
// network stub by hand (react.testing.2, tests.doubles.6). The library build
// and the published exports leave this folder out, and a test proves it.
//
// Use it with `msw/node`'s `setupServer(...fixtureHandlers())` until browser
// mode is on, then with the browser worker beside it.
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { http, HttpResponse, type RequestHandler } from "msw";
import * as z from "zod";

export type Json = z.infer<ReturnType<typeof z.json>>;

const fixtureFolder = join("internal", "host", "assets", "contract", "fixture");

/** The engine's fixture folder, found by walking up from this file. */
function findFixtures(from: string): string {
  let at = from;
  for (;;) {
    const candidate = join(at, fixtureFolder);
    if (existsSync(candidate)) return candidate;
    const parent = dirname(at);
    if (parent === at) {
      throw new Error(`no ${fixtureFolder} above ${from}`);
    }
    at = parent;
  }
}

/** Each document of the fixture bundle, by file name without its extension. */
export function fixtureDocuments(
  folder: string = findFixtures(dirname(fileURLToPath(import.meta.url))),
): ReadonlyMap<string, Json> {
  const documents = new Map<string, Json>();
  for (const file of readdirSync(folder).sort()) {
    if (!file.endsWith(".json")) continue;
    const text: string = readFileSync(join(folder, file), "utf8");
    const document = z.json().parse(JSON.parse(text));
    documents.set(file.slice(0, -".json".length), document);
  }
  return documents;
}

/** One GET handler per fixture document, at `<base><name>.json`. */
export function fixtureHandlers(
  base = "https://explorer.test/",
  documents: ReadonlyMap<string, Json> = fixtureDocuments(),
): readonly RequestHandler[] {
  return [...documents].map(([name, document]) =>
    http.get(`${base}${name}.json`, () => HttpResponse.json(document)),
  );
}
