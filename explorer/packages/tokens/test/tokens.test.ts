// The token guards (web.tokens.*): every --gr-* token the generated
// styles define is read, every one a style reads is defined, and no colour
// literal stands outside the generated file. Each guard is a function of a tree
// of files, so a plant can hand it a broken one and see it fail.
import {
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { extname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const explorer = fileURLToPath(new URL("../../../", import.meta.url));
const generated = "packages/tokens/src/tokens.gen.css";
// The separator of a key of walked names: a map key, never a file path.
const keySeparator = "/";
const skipped = new Set(["node_modules", "dist", "coverage", ".turbo"]);
const styled = new Set([".css", ".html", ".ts", ".tsx", ".js", ".mjs", ".svg"]);

// files lists the text files of a folder as path to content, paths relative to it.
export function files(folder: string): Map<string, string> {
  const out = new Map<string, string>();
  const walk = (dir: string, at: string[]): void => {
    for (const name of readdirSync(dir)) {
      if (skipped.has(name)) continue;
      const path = join(dir, name);
      if (statSync(path).isDirectory()) walk(path, [...at, name]);
      else if (styled.has(extname(name)))
        out.set([...at, name].join(keySeparator), readFileSync(path, "utf8"));
    }
  };
  walk(folder, []);
  return out;
}

// defined is the --gr-* names a CSS text declares.
export function defined(css: string): Set<string> {
  return new Set(
    [...css.matchAll(/^\s*(--gr-[a-z0-9-]+)\s*:/gm)].map((m) => m[1] ?? ""),
  );
}

// read is the --gr-* names any text reads through var().
export function read(text: string): Set<string> {
  return new Set(
    [...text.matchAll(/var\(\s*(--gr-[a-z0-9-]+)/g)].map((m) => m[1] ?? ""),
  );
}

// unread and undefinedReads are the two halves of the guard over a tree.
export function unread(tree: Map<string, string>): string[] {
  const reads = new Set<string>();
  for (const text of tree.values()) for (const n of read(text)) reads.add(n);
  const css = tree.get(generated) ?? "";
  return [...defined(css)].filter((n) => !reads.has(n)).sort();
}

export function undefinedReads(tree: Map<string, string>): string[] {
  const have = defined(tree.get(generated) ?? "");
  const missing = new Set<string>();
  for (const text of tree.values())
    for (const n of read(text)) if (!have.has(n)) missing.add(n);
  return [...missing].sort();
}

const functional = /\b(?:rgb|rgba|hsl|hsla|hwb|lab|lch|oklab|oklch|color)\(/g;
const hex = "#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{4}|[0-9a-fA-F]{3})";
// In a style or a page a hex colour is a value: after a colon, a comma, a
// bracket or a quote. In a script it is a whole string. A colour named in a
// comment is neither, and a selector (#app) follows no colon.
const hexValue = new RegExp(`(?:[:,(]|["'])\\s*${hex}(?![0-9a-zA-Z])`, "g");
const hexString = new RegExp(`(["'\`])${hex}\\1`, "g");
const scripts = new Set([".ts", ".tsx", ".js", ".mjs"]);

// literals lists "path: text" for each colour literal outside the generated file.
export function literals(tree: Map<string, string>): string[] {
  const found: string[] = [];
  for (const [path, text] of tree) {
    if (path === generated) continue;
    const hexes = scripts.has(extname(path)) ? hexString : hexValue;
    for (const re of [functional, hexes])
      for (const m of text.matchAll(re))
        found.push(`${path}: ${m[0].replace(/^[:,(]\s*/, "")}`);
  }
  return found;
}

const shipped = files(explorer);

describe("the keys of files()", () => {
  it("are slash-separated on every system", () => {
    const root = mkdtempSync(join(tmpdir(), "tokens-keys-"));
    try {
      mkdirSync(join(root, "a"));
      writeFileSync(join(root, "a", "b.css"), "a { }");
      expect([...files(root).keys()]).toEqual(["a/b.css"]);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});

describe("the tokens the styles define and read", () => {
  it("defines tokens at all", () => {
    expect(defined(shipped.get(generated) ?? "").size).toBeGreaterThan(30);
  });

  it("reads every token the generated styles define", () => {
    expect(unread(shipped)).toEqual([]);
  });

  it("defines every token the styles read", () => {
    expect(undefinedReads(shipped)).toEqual([]);
  });

  it("fails on a token defined and read by no one", () => {
    const tree = new Map(shipped);
    tree.set(generated, `${tree.get(generated)}\n  --gr-stray: 1px;\n`);
    expect(unread(tree)).toEqual(["--gr-stray"]);
  });

  it("fails on a token read and defined nowhere", () => {
    const tree = new Map(shipped);
    const name = ["--gr", "no-such"].join("-");
    tree.set("packages/element/src/x.css", `a { color: var(${name}); }`);
    expect(undefinedReads(tree)).toEqual([name]);
  });
});

describe("the colour literals outside the generated file", () => {
  it("finds none in a style, a page or a script", () => {
    expect(literals(shipped)).toEqual([]);
  });

  it("fails on a hex colour in a style", () => {
    const tree = new Map([
      ["packages/element/src/x.css", "a { color: #fff; }"],
    ]);
    expect(literals(tree)).toEqual(["packages/element/src/x.css: #fff"]);
  });

  it("fails on a hex colour in a page", () => {
    const tree = new Map([
      ["packages/element/index.html", '<i style="color:#ff0000">'],
    ]);
    expect(literals(tree)).toHaveLength(1);
  });

  it("fails on a functional colour in a script", () => {
    const call = ["rg", "b("].join("");
    const tree = new Map([
      ["packages/core/src/x.ts", `const c = '${call}0 0 0)';`],
    ]);
    expect(literals(tree)).toEqual([`packages/core/src/x.ts: ${call}`]);
  });

  it("fails on a hex colour string in a script", () => {
    const quoted = ["'#", "ff", "0000'"].join("");
    const tree = new Map([["packages/core/src/x.ts", `const c = ${quoted};`]]);
    expect(literals(tree)).toHaveLength(1);
  });

  it("lets a hex colour named in a comment and a selector pass", () => {
    const tree = new Map([
      ["packages/core/src/x.ts", "// the colour (#ff0000) is only named here"],
      ["packages/element/src/x.css", "#add { margin: 0; }"],
    ]);
    expect(literals(tree)).toEqual([]);
  });

  it("passes the generated file its own colours", () => {
    const tree = new Map([[generated, "a { color: #fff; }"]]);
    expect(literals(tree)).toEqual([]);
  });
});
