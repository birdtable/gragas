// The vendored fonts (supply.provenance.5), offline: each copy is
// the bytes PROVENANCE.json records, only woff2 and a licence are kept, each
// release is held to no age window, no style names a
// font host, and the content policy names none either. No clock and no network.
import { createHash } from "node:crypto";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { fontSrc } from "../src/index.ts";

const pkg = fileURLToPath(new URL("..", import.meta.url));
const fonts = join(pkg, "fonts");

interface Copy {
  package: string;
  integrity: string;
  published: string;
  vendored: string;
  license: string;
  folder: string;
  files: Record<string, string>;
}
interface Record_ {
  copies: Copy[];
}

const sha256 = (file: string): string =>
  createHash("sha256").update(readFileSync(file)).digest("hex");

// hostUrls lists the url() of a style that is not a path inside the package.
export function hostUrls(css: string): string[] {
  return [...css.matchAll(/url\(\s*["']?([^"')]+)/g)]
    .map((m) => m[1] ?? "")
    .filter(
      (u) => /^(?:[a-z][a-z0-9+.-]*:|\/\/)/i.test(u) || u.startsWith("/"),
    );
}

// policyHosts lists the sources of a font-src directive that are not 'self'.
export function policyHosts(directive: string): string[] {
  return directive
    .replace(/^font-src\s+/, "")
    .split(/\s+/)
    .filter((s) => s !== "" && s !== "'self'");
}

const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);

// copyOf narrows one parsed copy of PROVENANCE.json, failing on a missing field.
function copyOf(v: unknown): Copy {
  if (!isRecord(v)) throw new Error("a copy is not an object");
  const {
    package: name,
    integrity,
    published,
    vendored,
    license,
    folder,
    files,
  } = v;
  if (
    typeof name !== "string" ||
    typeof integrity !== "string" ||
    typeof published !== "string" ||
    typeof vendored !== "string" ||
    typeof license !== "string" ||
    typeof folder !== "string" ||
    !isRecord(files)
  )
    throw new Error(
      `a copy of PROVENANCE.json lacks a field: ${JSON.stringify(v)}`,
    );
  const sums: Record<string, string> = {};
  for (const [file, sum] of Object.entries(files)) {
    if (typeof sum !== "string") throw new Error(`${file} has no sha256`);
    sums[file] = sum;
  }
  return {
    package: name,
    integrity,
    published,
    vendored,
    license,
    folder,
    files: sums,
  };
}

const parsed: unknown = JSON.parse(
  readFileSync(join(fonts, "PROVENANCE.json"), "utf8"),
);
const copiesOf =
  isRecord(parsed) && Array.isArray(parsed["copies"]) ? parsed["copies"] : [];
const record: Record_ = { copies: copiesOf.map(copyOf) };
const css = readFileSync(join(pkg, "src/fonts.css"), "utf8");

// allows is whether licences.json lets a package carry a licence: a plain
// allowed id, or the one id its own adds entry gives it (never every package).
export function allows(
  policy: unknown,
  name: string,
  license: string,
): boolean {
  if (!isRecord(policy)) return false;
  const { allowed, adds } = policy;
  if (Array.isArray(allowed) && allowed.includes(license)) return true;
  const entry = isRecord(adds) ? adds[name] : undefined;
  return isRecord(entry) && entry["id"] === license;
}

const policy: unknown = JSON.parse(
  readFileSync(join(pkg, "../../licences.json"), "utf8"),
);

describe("the licence of the vendored fonts", () => {
  it("is allowed to each copy by its adds entry in explorer/licences.json", () => {
    const refused = record.copies
      .filter((c) => !allows(policy, c.package, c.license))
      .map((c) => `${c.package}: ${c.license}`);
    expect(refused).toEqual([]);
  });

  it("is allowed to no other package", () => {
    expect(allows(policy, "@ibm/plex-sans", "OFL-1.1")).toBe(true);
    expect(allows(policy, "another-package", "OFL-1.1")).toBe(false);
    expect(allows(policy, "@ibm/plex-sans", "GPL-3.0-only")).toBe(false);
  });
});

describe("the vendored fonts", () => {
  it("record two packages and name their licence", () => {
    expect(record.copies.map((c) => c.package)).toEqual([
      "@ibm/plex-sans",
      "@ibm/plex-mono",
    ]);
    for (const c of record.copies) expect(c.license).toBe("OFL-1.1");
  });

  it("are the bytes PROVENANCE.json records", () => {
    for (const c of record.copies)
      for (const [name, sum] of Object.entries(c.files))
        expect(sha256(join(fonts, c.folder, name)), `${c.folder}/${name}`).toBe(
          sum,
        );
  });

  it("hold only woff2 and the licence, each recorded", () => {
    for (const c of record.copies) {
      const have = readdirSync(join(fonts, c.folder)).sort();
      expect(have).toEqual(Object.keys(c.files).sort());
      for (const name of have) expect(name).toMatch(/\.woff2$|^LICENSE\.txt$/);
    }
  });

  it("are the five faces fonts.css declares, each from a woff2 of the package", () => {
    const urls = [...css.matchAll(/url\("([^"]+)"\)/g)].map((m) => m[1] ?? "");
    expect(urls).toHaveLength(5);
    for (const u of urls) {
      expect(u).toMatch(/\.woff2$/);
      expect(() => readFileSync(join(pkg, "src", u))).not.toThrow();
    }
    expect(css).not.toMatch(/\.(ttf|otf|woff)\b/);
  });
});

describe("the font host", () => {
  it("is named by no style", () => {
    expect(hostUrls(css)).toEqual([]);
  });

  it("is named by no source of the content policy", () => {
    expect(policyHosts(fontSrc)).toEqual([]);
    expect(fontSrc).toBe("font-src 'self'");
  });

  it("fails on a style that loads a font from a host", () => {
    expect(hostUrls('src: url("https://fonts.example/a.woff2");')).toEqual([
      "https://fonts.example/a.woff2",
    ]);
    expect(hostUrls('src: url("//fonts.example/a.woff2");')).toHaveLength(1);
  });

  it("fails on a policy that names a host", () => {
    expect(policyHosts("font-src 'self' https://fonts.example")).toEqual([
      "https://fonts.example",
    ]);
  });
});
