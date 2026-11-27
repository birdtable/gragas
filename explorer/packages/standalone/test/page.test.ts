// The built standalone page, read as files (web.speed.4):
// one classic script, no inline script, no dynamic import, a policy with no
// unsafe-inline and no origin, and every kind of file within its byte budget.
// Each check below has a plant: the same check over a fault, which must fail.
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { beforeAll, describe, expect, test } from "vitest";
import {
  budgetProblems,
  buildPage,
  packageDir,
  dynamicImportProblems,
  inlineDataProblems,
  markupProblems,
  policy,
  policyProblems,
  read,
  scriptProblems,
  tagProblems,
  tags,
} from "./page-checks.ts";

let out = "";
let html = "";
beforeAll(async () => {
  out = await buildPage();
  html = read(out, "index.html");
}, 120_000);

const fixtureFile = (name: string): string =>
  fileURLToPath(
    new URL(
      `../../../../internal/host/assets/contract/fixture/${name}.json`,
      import.meta.url,
    ),
  );

describe("the built output", () => {
  test("one classic script from a file, no inline script, no import(", () => {
    expect(scriptProblems(html, out)).toEqual([]);
    expect(html).not.toContain('type="module"');
  });

  test("the three data blocks stand empty, named by data-file", () => {
    const blocks = tags(html, "script").filter(
      (s) => s.attrs["type"] === "application/json",
    );
    expect(blocks.map((b) => b.attrs["data-file"])).toEqual([
      "bundle.json",
      "report.json",
      "proof.json",
    ]);
    expect(blocks.map((b) => b.text)).toEqual(["", "", ""]);
  });

  test("the policy has no unsafe-inline, no origin, and trusted types", () => {
    expect(policyProblems(html)).toEqual([]);
  });

  test("no tag carries crossorigin, and no script is a module", () => {
    expect(tagProblems(html)).toEqual([]);
  });

  test("the source has no dynamic import", () => {
    expect(
      dynamicImportProblems(fileURLToPath(new URL("../src", import.meta.url))),
    ).toEqual([]);
  });

  test("every kind of file is within its budget row", () => {
    expect(budgetProblems(out)).toEqual([]);
  });

  test("the fonts are files and the stylesheet names no origin", () => {
    const css = read(out, "explorer.css");
    expect(css).not.toMatch(/url\((?!\.\/fonts\/)/);
    expect(css).not.toMatch(/https?:\/\//);
  });
});

describe("plants: each check fails on its fault", () => {
  test("an inline script fails the script check", () => {
    const planted = html.replace("</body>", "<script>alert(1)</script></body>");
    expect(scriptProblems(planted, out)).toContain("2 scripts, not one");
  });

  test("a module script fails the script check", () => {
    const planted = html.replace("<script defer", '<script type="module"');
    expect(scriptProblems(planted, out)).toContain("a module script");
  });

  test("a block named by id fails the script check", () => {
    const planted = html.replace('data-file="bundle.json"', 'id="bundle.json"');
    expect(scriptProblems(planted, out)).toEqual(
      expect.arrayContaining([
        "a block with no data-file",
        "a block named by id",
      ]),
    );
  });

  test("unsafe-inline fails the policy check", () => {
    const planted = html.replace(
      "script-src 'self'",
      "script-src 'self' 'unsafe-inline'",
    );
    expect(policyProblems(planted)).toContain("script-src has unsafe-inline");
  });

  test("an external origin fails the policy check", () => {
    const planted = html.replace(
      "connect-src 'self'",
      "connect-src 'self' https://x.test",
    );
    expect(policyProblems(planted)).toContain(
      "connect-src names https://x.test",
    );
  });

  test("a policy without trusted types fails the policy check", () => {
    const planted = html.replace("require-trusted-types-for 'script'; ", "");
    expect(policyProblems(planted)).toContain("trusted types are not required");
  });

  test.each(Object.keys(policy))(
    "a policy without %s fails the policy check",
    (name) => {
      const dropped = html.replace(new RegExp(`${name} [^;"]*(; )?`), "");
      expect(policyProblems(dropped).join("\n")).toContain(
        `${name} is missing`,
      );
    },
  );

  test.each(Object.entries(policy))(
    "a policy that widens %s fails the policy check",
    (name, value) => {
      const widened = html.replace(
        `${name} ${value}`,
        `${name} ${value} 'unsafe-eval'`,
      );
      expect(policyProblems(widened).join("\n")).toContain(
        `${name} is not ${value}`,
      );
    },
  );

  test("crossorigin on the stylesheet and a module script fail the tag check", () => {
    const planted = html
      .replace('<link rel="stylesheet"', '<link rel="stylesheet" crossorigin')
      .replace("<script defer", '<script type="module"');
    expect(tagProblems(planted)).toEqual([
      "crossorigin on a link",
      "a module script",
    ]);
  });

  test("a dynamic import in the source fails the import check", () => {
    const dir = mkdtempSync(join(tmpdir(), "gragas-src-"));
    writeFileSync(
      join(dir, "lazy.ts"),
      'export const view = () => import("./view.ts");\n',
    );
    expect(dynamicImportProblems(dir)).toEqual(["import( in lazy.ts"]);
  });

  test("a large dependency fails the byte budget", async () => {
    const dir = mkdtempSync(join(tmpdir(), "gragas-plant-"));
    const config = join(dir, "vite.config.ts");
    writeFileSync(
      config,
      `import { defineConfig, mergeConfig } from "vite";
import base from ${JSON.stringify(join(packageDir, "vite.config.ts"))};
const filler = Array.from({ length: 40000 }, (_, i) => "k" + i + ":" + i * 7919);
export default mergeConfig(base, defineConfig({
  root: ${JSON.stringify(packageDir)},
  plugins: [{
    name: "plant-large-dependency",
    transform(code, id) {
      if (!id.endsWith("/src/main.tsx")) return null;
      return code + "\\nexport const large = {" + filler.join(",") + "};\\nconsole.debug(large);\\n";
    },
  }],
}));
`,
    );
    const big = await buildPage(config);
    expect(budgetProblems(big).join("\n")).toMatch(
      /page-script: \d+ bytes over a limit of 680000/,
    );
  }, 120_000);
});

describe("the bundle that stays inline", () => {
  const scaled = (rules: number): string => {
    const doc: Record<string, unknown> = JSON.parse(
      readFileSync(fixtureFile("bundle"), "utf8"),
    );
    const first = (key: string): Record<string, unknown> => {
      const list = doc[key];
      const one = Array.isArray(list) ? list[0] : undefined;
      return typeof one === "object" && one !== null
        ? Object.fromEntries(Object.entries(one))
        : {};
    };
    const rule = first("rules");
    const binding = first("bindings");
    delete doc["proof_grid"];
    doc["rules"] = Array.from({ length: rules }, (_, i) => ({
      ...rule,
      id: `part.concern.${i}`,
    }));
    doc["bindings"] = Array.from({ length: rules }, (_, i) => ({
      ...binding,
      rule: `part.concern.${i}`,
    }));
    return JSON.stringify(doc);
  };

  test("ten thousand rules without the proof grid stay inline", () => {
    expect(inlineDataProblems(scaled(10_000))).toEqual([]);
  });

  test("a bundle over the limit is reported, with the fetched form named", () => {
    expect(inlineDataProblems(scaled(30_000)).join("\n")).toMatch(
      /page-inline-data: \d+ bytes over a limit of 8388608: write the fetched form/,
    );
  });
});

describe("the markup", () => {
  test("the validator passes the built page", async () => {
    expect(await markupProblems(html)).toEqual([]);
  });

  test("a control with no name and a duplicate id fail it", async () => {
    const planted = html.replace(
      '<div id="root"></div>',
      '<div id="root"></div><input type="text"><p id="a"></p><p id="a"></p>',
    );
    const found = (await markupProblems(planted)).join("\n");
    expect(found).toMatch(/input-missing-label/);
    expect(found).toMatch(/no-dup-id/);
  });

  test("unclosed markup fails it", async () => {
    const planted = html.replace("</body>", "<div><p></div></body>");
    expect((await markupProblems(planted)).length).toBeGreaterThan(0);
  });
});
