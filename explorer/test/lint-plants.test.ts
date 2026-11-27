// gates.plants.1 for the lint step: each planted violation fails it, the clean
// control passes, and a path the lint refuses to read says so. Typed lint reads
// a package's own tsconfig, so a plant is a real file, and it is written into a
// throwaway copy of the workspace's config (the real node_modules linked in),
// never into the tree: a plant in the tree would be seen by the type check and
// the lint that run beside this test.
import {
  cpSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { ESLint } from "eslint";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

const real = fileURLToPath(new URL("..", import.meta.url));
let scratch = "";
let explorer = "";

beforeAll(() => {
  scratch = mkdtempSync(join(tmpdir(), "gragas-lint-plants-"));
  explorer = join(scratch, "explorer");
  mkdirSync(join(scratch, "docs"));
  cpSync(
    join(real, "../docs/explorer-boundaries.json"),
    join(scratch, "docs/explorer-boundaries.json"),
  );
  for (const name of [
    "eslint.config.js",
    "package.json",
    "tsconfig.base.json",
    "tsconfig.library.json",
    "tsconfig.bundler.json",
  ]) {
    cpSync(join(real, name), join(explorer, name));
  }
  for (const pkg of readdirSync(join(real, "packages"))) {
    for (const name of ["package.json", "tsconfig.json"]) {
      cpSync(
        join(real, "packages", pkg, name),
        join(explorer, "packages", pkg, name),
      );
    }
  }
  // No type argument: Node reads the target and makes the kind of link it needs.
  symlinkSync(join(real, "node_modules"), join(explorer, "node_modules"));
});

afterAll(() => {
  rmSync(scratch, { recursive: true, force: true });
});

const target = "export function one(): number {\n  return 1;\n}\n";

async function lint(
  relative: string,
  code: string,
  targets: readonly string[] = [],
): Promise<string[]> {
  for (const t of targets) {
    mkdirSync(dirname(join(explorer, t)), { recursive: true });
    writeFileSync(join(explorer, t), target);
  }
  const file = join(explorer, relative);
  mkdirSync(dirname(file), { recursive: true });
  writeFileSync(file, code);
  const eslint = new ESLint({ cwd: explorer });
  const [result] = await eslint.lintFiles([file]);
  return (result?.messages ?? []).map(
    (m) => `${m.ruleId ?? "parse"}: ${m.message}`,
  );
}

describe("the lint step", () => {
  it("passes a clean control", async () => {
    expect(
      await lint(
        "packages/core/data/control.ts",
        "export function one(): number {\n  return 1;\n}\n",
      ),
    ).toEqual([]);
  });

  it("fails an explicit any (ts.type-safety.1)", async () => {
    const found = await lint(
      "packages/core/data/plant-any.ts",
      "export function id(x: any): unknown {\n  return x;\n}\n",
    );
    expect(
      found.some((m) => m.startsWith("@typescript-eslint/no-explicit-any")),
    ).toBe(true);
  });

  it("fails a default export (ts.modules.1)", async () => {
    const found = await lint(
      "packages/core/data/plant-default.ts",
      "export default function one(): number {\n  return 1;\n}\n",
    );
    expect(found.some((m) => m.includes("ts.modules.1"))).toBe(true);
  });

  it("fails an enum (ts.modules.2)", async () => {
    const found = await lint(
      "packages/core/data/plant-enum.ts",
      "export enum Kind {\n  A,\n}\n",
    );
    expect(found.some((m) => m.includes("ts.modules.2"))).toBe(true);
  });

  it("fails a forbidden import: the data module reaching the selectors (arch.ports.8)", async () => {
    const found = await lint(
      "packages/core/data/plant-import.ts",
      'import { one } from "../selectors/plant-target.ts";\n\nexport const two: number = one();\n',
      ["packages/core/selectors/plant-target.ts"],
    );
    expect(found.some((m) => m.startsWith("boundaries/"))).toBe(true);
  });

  it("fails a view reaching into standalone (arch.ports.8)", async () => {
    const found = await lint(
      "packages/core/views/rules/plant.ts",
      'import { one } from "../../../standalone/src/plant-target.ts";\n\nexport const two: number = one();\n',
      ["packages/standalone/src/plant-target.ts"],
    );
    expect(found.some((m) => m.startsWith("boundaries/"))).toBe(true);
  });

  it("fails standalone importing element (arch.ports.8)", async () => {
    const found = await lint(
      "packages/standalone/src/plant.ts",
      'import { one } from "../../element/src/plant-target.ts";\n\nexport const two: number = one();\n',
      ["packages/element/src/plant-target.ts"],
    );
    expect(found.some((m) => m.startsWith("boundaries/"))).toBe(true);
  });

  it("passes standalone importing a view it may import (the control for the two above)", async () => {
    const found = await lint(
      "packages/standalone/src/plant-ok.ts",
      'import { one } from "../../core/views/rules/plant-target.ts";\n\nexport const two: number = one();\n',
      ["packages/core/views/rules/plant-target.ts"],
    );
    expect(found).toEqual([]);
  });

  it("fails a forced assertion and a console write (ts.type-safety.2, ts.errors.4)", async () => {
    const found = await lint(
      "packages/core/data/plant-assert.ts",
      'export function loud(x: unknown): string {\n  console.log("x");\n  return x as string;\n}\n',
    );
    expect(
      found.some((m) =>
        m.startsWith("@typescript-eslint/consistent-type-assertions"),
      ),
    ).toBe(true);
    expect(found.some((m) => m.startsWith("no-console"))).toBe(true);
  });

  // react.components.3, web.structure.2: raw markup is written in the funnel only.
  it.each([
    ["an innerHTML write", 'el.innerHTML = "<b>x</b>";'],
    ["an outerHTML write", 'el.outerHTML = "<b>x</b>";'],
    ["an innerHTML write by name", 'Reflect.set(el, "innerHTML", "<b>x</b>");'],
    [
      "an insertAdjacentHTML call",
      'el.insertAdjacentHTML("beforeend", "<b>x</b>");',
    ],
    ["a setHTMLUnsafe call", 'el.setHTMLUnsafe("<b>x</b>");'],
    ["a document.write call", 'document.write("<b>x</b>");'],
  ])("fails %s outside the funnel (react.components.3)", async (name, body) => {
    const file = `packages/core/views/rules/plant-${name.replaceAll(" ", "-")}.ts`;
    const found = await lint(
      file,
      `export function write(el: HTMLElement): void {\n  ${body}\n}\n`,
    );
    expect(found.some((m) => m.includes("react.components.3"))).toBe(true);
  });

  it.each(["dompurify", "markdown-it"])(
    "fails a direct import of %s outside the funnel (react.components.3)",
    async (name) => {
      const found = await lint(
        `packages/core/views/rules/plant-import-${name}.ts`,
        `import lib from "${name}";\n\nexport const used: unknown = lib;\n`,
      );
      expect(found.some((m) => m.includes("react.components.3"))).toBe(true);
    },
  );

  it("passes the funnel's own sink and imports (the control for the plants above)", async () => {
    const found = await lint(
      "packages/core/markup/plant-ok.ts",
      'import lib from "dompurify";\n\nexport function write(el: HTMLElement): void {\n  el.innerHTML = String(lib);\n}\n',
    );
    expect(found.some((m) => m.includes("react.components.3"))).toBe(false);
  });

  it("fails a style prop on a DOM element (react.components.4)", async () => {
    const found = await lint(
      "packages/core/views/rules/plant-style.tsx",
      'export function Box(): unknown {\n  return <div style={{ color: "red" }}>x</div>;\n}\n',
    );
    expect(found.some((m) => m.includes("react.components.4"))).toBe(true);
  });

  // web.structure.1: one module touches the network or writes a URL.
  const ask =
    'export async function ask(): Promise<number> {\n  const response = await fetch("/bundle.json");\n  return response.status;\n}\n';

  it("passes the network from load.ts, the one module that may touch it (web.structure.1)", async () => {
    expect(await lint("packages/core/data/load.ts", ask)).toEqual([]);
  });

  it("fails fetch in any other module (web.structure.1)", async () => {
    const refused: string[] = [];
    for (const where of [
      "packages/core/selectors/plant-fetch.ts",
      "packages/core/data/plant-fetch.ts",
      "packages/standalone/src/plant-fetch.ts",
    ]) {
      const found = await lint(where, ask);
      if (
        found.some(
          (m) =>
            m.startsWith("no-restricted-globals") &&
            m.includes("web.structure.1"),
        )
      ) {
        refused.push(where);
      }
    }
    expect(refused).toEqual([
      "packages/core/selectors/plant-fetch.ts",
      "packages/core/data/plant-fetch.ts",
      "packages/standalone/src/plant-fetch.ts",
    ]);
  });

  it("fails a URL written outside load.ts, and fetch reached through window (web.structure.1)", async () => {
    const url = await lint(
      "packages/core/data/plant-url.ts",
      'export function where(): string {\n  return new URL("bundle.json", "https://x.test/").href;\n}\n',
    );
    expect(url.some((m) => m.includes("web.structure.1"))).toBe(true);
    const reached = await lint(
      "packages/core/selectors/plant-window.ts",
      'export function ask(): Promise<Response> {\n  return window.fetch("/bundle.json");\n}\n',
    );
    expect(reached.some((m) => m.startsWith("no-restricted-properties"))).toBe(
      true,
    );
  });

  // The explorer's TypeScript names no operating system.
  const plantOs: [string, string, string][] = [
    ["a read of process.platform", "", "return process.platform;"],
    [
      "a call of os.platform()",
      'import { platform } from "node:os";\n',
      "return platform();",
    ],
    [
      "os.platform() by namespace",
      'import * as os from "node:os";\n',
      "return os.platform();",
    ],
    [
      "os.EOL by namespace",
      'import * as os from "node:os";\n',
      "return os.EOL;",
    ],
    [
      "os.type() by namespace",
      'import * as os from "node:os";\n',
      "return os.type();",
    ],
    [
      "a replace of a backslash",
      "",
      'return "a\\\\b".replaceAll("\\\\", "/");',
    ],
    [
      "a replace of a backslash pattern",
      "",
      'return "a\\\\b".replace(/\\\\/g, "/");',
    ],
    ["a .cmd literal", "", 'return "pnpm.cmd";'],
    ["an .exe literal", "", 'return "node.exe";'],
    ["a /dev/null literal", "", 'return "/dev/null";'],
    [
      "an .exe template literal",
      "",
      "const name = String(1);\n  return `${name}.exe`;",
    ],
    ["a /dev/null template literal", "", "return `/dev/null`;"],
    ["path.sep", 'import * as path from "node:path";\n', "return path.sep;"],
    [
      "path.posix",
      'import * as path from "node:path";\n',
      "return path.posix.sep;",
    ],
    [
      "path.win32",
      'import * as path from "node:path";\n',
      "return path.win32.sep;",
    ],
    [
      "a named import of sep",
      'import { sep } from "node:path";\n',
      "return sep;",
    ],
    ["a split on a backslash", "", 'return "a\\\\b".split("\\\\").join("");'],
    ["a join on a slash", "", 'return ["a", "b"].join("/");'],
    [
      "a hand-built path joined on a slash",
      'import { readFileSync } from "node:fs";\n',
      'return readFileSync(["a", "data", "b"].join("/"), "utf8");',
    ],
    ["a join on a backslash", "", 'return ["a", "b"].join("\\\\");'],
    [
      "the pathname of a file URL",
      "",
      'return new URL("..", import.meta.url).pathname;',
    ],
  ];
  it.each(plantOs)("fails %s", async (name, head, body) => {
    const found = await lint(
      `packages/core/test/plant-os-${name.replaceAll(" ", "-")}.ts`,
      `${head}export function where(): string {\n  ${body}\n}\n`,
    );
    expect(found.some((m) => m.includes("no operating system"))).toBe(true);
  });

  it("passes a name that only looks like /dev/null, its dots and slashes being literal", async () => {
    const found = await lint(
      "packages/core/test/plant-os-xdevxnull.ts",
      'export function where(): string {\n  return "xdevxnull";\n}\n',
    );
    expect(found).toEqual([]);
  });

  it("passes a join on a named key separator, which is a key and no path", async () => {
    const found = await lint(
      "packages/core/test/plant-os-key.ts",
      'const keySeparator = "/";\n\nexport function key(at: string[]): string {\n  return [...at, "a"].join(keySeparator);\n}\n',
    );
    expect(found).toEqual([]);
  });

  it("passes the portable forms of the same reads (the control for the plants above)", async () => {
    const found = await lint(
      "packages/core/test/plant-os-ok.ts",
      'import { join } from "node:path";\nimport { fileURLToPath } from "node:url";\n\nexport function where(): string {\n  return join(fileURLToPath(new URL("..", import.meta.url)), "a");\n}\n',
    );
    expect(found).toEqual([]);
  });

  it("passes the pathname of a request URL, which is not a file's", async () => {
    const found = await lint(
      "packages/core/test/plant-os-request.ts",
      "export function where(request: Request): string {\n  return new URL(request.url).pathname;\n}\n",
    );
    expect(found.some((m) => m.includes("no operating system"))).toBe(false);
  });

  it("refuses a file in a folder no tsconfig includes, and says it could not read it", async () => {
    const found = await lint(
      "packages/core/outside/plant.ts",
      "export const x: number = 1;\n",
    );
    expect(found.some((m) => m.startsWith("parse"))).toBe(true);
  });
});
