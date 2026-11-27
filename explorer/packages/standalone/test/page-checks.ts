// The checks over the built page: each reads the output's bytes,
// never the config that made it, and returns what it found; an empty list is a
// pass. page.test.ts runs them over the real build and over a plant each.
import { mkdtempSync, readdirSync, readFileSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFile } from "node:child_process";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const run = promisify(execFile);
export const packageDir = fileURLToPath(new URL("..", import.meta.url));
export const budgetsFile = fileURLToPath(
  new URL("../../../../gate/budgets.yaml", import.meta.url),
);

/**
 * Builds the page as the package's build script does, in a child process with
 * NODE_ENV=production (a test run's own NODE_ENV would build React's dev copy).
 * `extra` plugins are the plants' hands, loaded through a config file.
 */
export async function buildPage(
  extra: string | undefined = undefined,
): Promise<string> {
  const outDir = mkdtempSync(join(tmpdir(), "gragas-page-"));
  const vite = join(packageDir, "node_modules", "vite", "bin", "vite.js");
  const args = [vite, "build", "--outDir", outDir, "--logLevel", "silent"];
  if (extra !== undefined) args.push("--config", extra);
  await run(process.execPath, args, {
    cwd: packageDir,
    env: { ...process.env, NODE_ENV: "production" },
  });
  return outDir;
}

type Tag = Readonly<{
  name: string;
  attrs: Record<string, string>;
  text: string;
}>;

/** Every script and meta tag of a page, by a plain scan of its text. */
export function tags(html: string, name: "script" | "meta" | "link"): Tag[] {
  const found: Tag[] = [];
  const re =
    name === "script"
      ? /<script\b([^>]*)>([\s\S]*?)<\/script>/g
      : new RegExp(`<${name}\\b([^>]*?)\\/?>`, "g");
  for (const m of html.matchAll(re)) {
    const attrs: Record<string, string> = {};
    for (const a of (m[1] ?? "").matchAll(/([\w:-]+)(?:="([^"]*)")?/g)) {
      attrs[a[1] ?? ""] = a[2] ?? "";
    }
    found.push({ name, attrs, text: m[2] ?? "" });
  }
  return found;
}

export function read(out: string, file: string): string {
  return readFileSync(join(out, file), "utf8");
}

/** The scripts: exactly one, from a file, classic, with no inline body. */
export function scriptProblems(html: string, out: string): string[] {
  const problems: string[] = [];
  const scripts = tags(html, "script");
  const code = scripts.filter((s) => s.attrs["type"] !== "application/json");
  if (code.length !== 1) problems.push(`${code.length} scripts, not one`);
  for (const s of code) {
    if (s.attrs["type"] === "module") problems.push("a module script");
    if (s.attrs["src"] === undefined) problems.push("an inline script");
    if (s.text.trim() !== "") problems.push("a script with a body");
  }
  for (const s of scripts.filter(
    (s) => s.attrs["type"] === "application/json",
  )) {
    if (s.attrs["data-file"] === undefined)
      problems.push("a block with no data-file");
    if (s.attrs["id"] !== undefined) problems.push("a block named by id");
  }
  const files = readdirSync(out).filter((f) => f.endsWith(".js"));
  if (files.length !== 1)
    problems.push(`${files.length} script files, not one`);
  for (const f of files) {
    if (/\bimport\s*\(/.test(read(out, f))) problems.push(`import( in ${f}`);
  }
  return problems;
}

/** The policy, directive by directive. */
export const policy: Readonly<Record<string, string>> = {
  "default-src": "'none'",
  "script-src": "'self'",
  "style-src": "'self'",
  "font-src": "'self'",
  "img-src": "'self' data:",
  "connect-src": "'self'",
  "base-uri": "'none'",
  "form-action": "'none'",
  "object-src": "'none'",
  "require-trusted-types-for": "'script'",
  "trusted-types": "'none'",
};

/** The page's tags: no crossorigin attribute, which file:// cannot satisfy, and no module. */
export function tagProblems(html: string): string[] {
  const problems: string[] = [];
  for (const name of ["script", "link"] as const) {
    for (const tag of tags(html, name)) {
      if (tag.attrs["crossorigin"] !== undefined)
        problems.push(`crossorigin on a ${name}`);
    }
  }
  if (html.includes('type="module"')) problems.push("a module script");
  return problems;
}

/** The policy: no unsafe-inline, no origin, trusted types required. */
export function policyProblems(html: string): string[] {
  const metas = tags(html, "meta").filter(
    (m) => m.attrs["http-equiv"] === "Content-Security-Policy",
  );
  if (metas.length !== 1) return [`${metas.length} policy tags, not one`];
  const content = metas[0]?.attrs["content"] ?? "";
  const directives = new Map(
    content
      .split(";")
      .map((d) => d.trim().split(/\s+/))
      .map(([name, ...values]) => [name ?? "", values] as const),
  );
  const problems: string[] = [];
  const script = directives.get("script-src") ?? [];
  if (script.length === 0) problems.push("no script-src");
  if (script.includes("'unsafe-inline'"))
    problems.push("script-src has unsafe-inline");
  if (script.includes("'unsafe-eval'"))
    problems.push("script-src has unsafe-eval");
  if (script.includes("'strict-dynamic'")) {
    problems.push("script-src has strict-dynamic, which drops 'self'");
  }
  for (const [name, values] of directives) {
    for (const v of values) {
      if (/^(https?:|wss?:|\/\/|\*)/.test(v))
        problems.push(`${name} names ${v}`);
    }
  }
  if (
    !(directives.get("require-trusted-types-for") ?? []).includes("'script'")
  ) {
    problems.push("trusted types are not required");
  }
  for (const [name, value] of Object.entries(policy)) {
    const held = directives.get(name);
    if (held === undefined) problems.push(`${name} is missing`);
    else if (held.join(" ") !== value) problems.push(`${name} is not ${value}`);
  }
  for (const name of directives.keys()) {
    if (!(name in policy))
      problems.push(`${name} is not a directive of the policy`);
  }
  return problems;
}

/** A dynamic import in the source, which a one-script build would hide. */
export function dynamicImportProblems(dir: string): string[] {
  const problems: string[] = [];
  for (const f of readdirSync(dir, { recursive: true }).map(String)) {
    if (!/\.tsx?$/.test(f) || f.endsWith(".d.ts")) continue;
    if (/\bimport\s*\(/.test(readFileSync(join(dir, f), "utf8"))) {
      problems.push(`import( in ${f}`);
    }
  }
  return problems;
}

/** The limit of a budget row of gate/budgets.yaml, as the Go reader reads one. */
export function budgetLimit(id: string): number {
  const rows = readFileSync(budgetsFile, "utf8")
    .split(/^ {2}- id: /m)
    .slice(1);
  const mine = rows.filter((r) => r.split("\n")[0] === id);
  const limit = /^ {4}limit: ([0-9]+)$/m.exec(mine[0] ?? "")?.[1];
  if (mine.length !== 1 || limit === undefined)
    throw new Error(`no one row ${id}`);
  return Number(limit);
}

const sizeOf = (out: string, pick: (f: string) => boolean): number =>
  readdirSync(out, { recursive: true })
    .map(String)
    .filter((f) => statSync(join(out, f)).isFile() && pick(f))
    .reduce((n, f) => n + statSync(join(out, f)).size, 0);

export const measures: Record<string, (out: string) => number> = {
  "page-script": (out) => sizeOf(out, (f) => f.endsWith(".js")),
  "page-style": (out) => sizeOf(out, (f) => f.endsWith(".css")),
  "page-fonts": (out) => sizeOf(out, (f) => /\.woff2?$/.test(f)),
  "page-html": (out) => sizeOf(out, (f) => f === "index.html"),
};

/** A row's overage: empty when the output is within every limit. */
export function budgetProblems(out: string): string[] {
  const problems: string[] = [];
  for (const [id, measure] of Object.entries(measures)) {
    const bytes = measure(out);
    const limit = budgetLimit(id);
    if (bytes > limit)
      problems.push(`${id}: ${bytes} bytes over a limit of ${limit}`);
  }
  return problems;
}

/** The bundle that stays inline, judged against its row. */
export function inlineDataProblems(text: string): string[] {
  const bytes = Buffer.byteLength(text);
  const limit = budgetLimit("page-inline-data");
  return bytes > limit
    ? [
        `page-inline-data: ${bytes} bytes over a limit of ${limit}: write the fetched form`,
      ]
    : [];
}

/** The page's markup judged by the HTML validator: its recommended rules, which hold control names and ids. */
export async function markupProblems(html: string): Promise<string[]> {
  const { HtmlValidate } = await import("html-validate/node");
  const validator = new HtmlValidate({
    extends: ["html-validate:recommended"],
    rules: {
      // Spelling, which the build's own output and the formatter decide.
      "doctype-style": "off",
      "void-style": "off",
      // Control names are validity here, not style.
      "input-missing-label": "error",
      "wcag/h30": "error",
      "wcag/h32": "error",
      "wcag/h37": "error",
    },
  });
  const report = await validator.validateString(html);
  return report.results.flatMap((r) =>
    r.messages.map(
      (m) => `${m.ruleId ?? "parse"}: ${m.message} (${m.line}:${m.column})`,
    ),
  );
}
