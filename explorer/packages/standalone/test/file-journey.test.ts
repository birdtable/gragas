// The built page opened from a file (web.speed.4, web.speed.7):
// the one script and the policy run in a real engine from file://, with the
// fixture documents written into the blocks as the engine does. Every route
// renders, no request leaves the file, no policy violation and no console
// message are raised (gate/baselines.yaml allows none), and a script injected
// from another origin is refused by the policy.
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { pathToFileURL, fileURLToPath } from "node:url";
import { chromium, webkit, type BrowserType, type Page } from "playwright";
import { afterAll, beforeAll, describe, expect, test } from "vitest";
import { buildPage, tags } from "./page-checks.ts";

const fixture = (name: string): string =>
  readFileSync(
    fileURLToPath(
      new URL(
        `../../../../internal/host/assets/contract/fixture/${name}.json`,
        import.meta.url,
      ),
    ),
    "utf8",
  );
const baselines = readFileSync(
  fileURLToPath(new URL("../../../../gate/baselines.yaml", import.meta.url)),
  "utf8",
);

/** A block's text, as the engine writes it: nothing can close the tag early. */
const safe = (json: string): string => json.replaceAll("<", "\\u003c");

function filled(html: string, docs: Record<string, string>): string {
  let page = html;
  for (const block of tags(html, "script")) {
    const file = block.attrs["data-file"];
    const text = file === undefined ? undefined : docs[file];
    if (text === undefined) continue;
    page = page.replace(
      `<script type="application/json" data-file="${file}"></script>`,
      `<script type="application/json" data-file="${file}">${safe(text)}</script>`,
    );
  }
  return page;
}

const routes = [
  "rules",
  "rulebooks",
  "tools",
  "coverage",
  "standing",
  "health",
  "changelog",
  "terms",
  "attention",
] as const;

interface Seen {
  requests: string[];
  responses: string[];
  console: string[];
  said: string[];
}

async function open(page: Page, url: string): Promise<Seen> {
  const seen: Seen = { requests: [], responses: [], console: [], said: [] };
  page.on("request", (r) => seen.requests.push(r.url()));
  page.on("response", (r) => seen.responses.push(r.url()));
  page.on("console", (m) => {
    seen.said.push(m.text());
    if (m.type() === "error" || m.type() === "warning") {
      seen.console.push(`${m.type()}: ${m.text()}`);
    }
  });
  page.on("pageerror", (e) => seen.console.push(`pageerror: ${e.message}`));
  await page.addInitScript(() => {
    const held: string[] = [];
    Object.defineProperty(window, "__violations", { value: held });
    document.addEventListener("securitypolicyviolation", (e) => {
      held.push(`${e.violatedDirective} ${e.blockedURI}`);
    });
  });
  await page.goto(url);
  return seen;
}

async function violationsOf(page: Page): Promise<string[]> {
  const text = await page.evaluate(() =>
    JSON.stringify(Reflect.get(window, "__violations") ?? []),
  );
  const held: unknown = JSON.parse(text);
  return Array.isArray(held) ? held.map(String) : [];
}

let url = "";
beforeAll(async () => {
  const out = await buildPage();
  const index = join(out, "index.html");
  writeFileSync(
    index,
    filled(readFileSync(index, "utf8"), {
      "bundle.json": fixture("bundle"),
      "report.json": fixture("report-clean"),
    }),
  );
  url = pathToFileURL(index).href;
}, 120_000);

const engines: [string, BrowserType][] = [
  ["chromium", chromium],
  ["webkit", webkit],
];

describe.each(engines)("the page from a file in %s", (name, type) => {
  let close: () => Promise<void> = () => Promise.resolve();
  let launch: () => Promise<Page> = () =>
    Promise.reject(new Error("not launched"));
  beforeAll(async () => {
    const browser = await type.launch();
    close = () => browser.close();
    launch = () => browser.newPage();
  });
  afterAll(() => close());

  test("the console allow list is empty", () => {
    expect(baselines).toMatch(/^console_allow: \[\]$/m);
  });

  test.each(routes)(
    "%s renders with no request, message or violation",
    async (route) => {
      const page = await launch();
      const seen = await open(page, `${url}#/${route}`);
      await page.waitForSelector("main h1");
      expect((await page.textContent("main h1"))?.trim()).not.toBe("");
      await page.waitForLoadState("networkidle");
      expect(seen.requests.filter((r) => !r.startsWith("file:"))).toEqual([]);
      expect(seen.console).toEqual([]);
      expect(await violationsOf(page)).toEqual([]);
      await page.close();
    },
  );

  test("a script assigned from another origin is refused by trusted types", async () => {
    const page = await launch();
    await open(page, `${url}#/rules`);
    await page.waitForSelector("main h1");
    const outcome = await page.evaluate(() => {
      try {
        const s = document.createElement("script");
        s.src = "https://scripts.example.test/x.js";
        document.head.append(s);
        return "assigned";
      } catch (e) {
        return e instanceof TypeError ? "refused" : "other";
      }
    });
    expect(outcome).toBe("refused");
    await page.close();
  });

  // WebKit on file:// does not match 'self' against the scheme as Chromium does:
  // from a file page it requests an https script the policy does not allow.
  // It does refuse an inline script, which both engines check.
  async function planted(tag: string): Promise<{ seen: Seen; page: Page }> {
    const file = join(
      dirname(fileURLToPath(url)),
      `planted-${Math.random().toString(36).slice(2)}.html`,
    );
    writeFileSync(
      file,
      readFileSync(fileURLToPath(url), "utf8").replace(
        "</body>",
        `${tag}</body>`,
      ),
    );
    const page = await launch();
    const seen = await open(page, pathToFileURL(file).href);
    await page.waitForSelector("main h1");
    await page.waitForLoadState("networkidle");
    return { seen, page };
  }

  test("a request to another origin is seen by the zero-request check", async () => {
    const { seen, page } = await planted(
      '<link rel="stylesheet" href="https://styles.example.test/a.css">',
    );
    expect(seen.requests.filter((r) => !r.startsWith("file:"))).not.toEqual([]);
    await page.close();
  });

  test("a console error is seen by the empty allow list", async () => {
    const { seen, page } = await planted('<img src="./missing.png" alt="">');
    expect(seen.console).not.toEqual([]);
    await page.close();
  });

  test("an inline script in the markup is refused by the policy", async () => {
    const { seen, page } = await planted(
      '<script>document.title = "ran"</script>',
    );
    expect(await page.title()).not.toBe("ran");
    const said = [...(await violationsOf(page)), ...seen.said].join("\n");
    expect(said).toMatch(/script-src|Refused to execute|violates/);
    await page.close();
  });

  test.runIf(name === "chromium")(
    "a script tag in the markup from another origin is blocked by script-src",
    async () => {
      const { seen, page } = await planted(
        '<script src="https://scripts.example.test/x.js"></script>',
      );
      expect((await violationsOf(page)).join("\n")).toMatch(/script-src/);
      expect(seen.responses.filter((r) => r.startsWith("https://"))).toEqual(
        [],
      );
      await page.close();
    },
  );
});
