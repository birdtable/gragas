// web.content.1 and web.content.10 as a test that reads source: a sentence typed
// into a component is a string the message module does not own, and a key a call
// names must exist. Text only, no compiler API: the package's suite runs with
// every sibling unresolvable (ts.modules.5).
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

// The separator of a key of walked names: a map key, never a file path.
const keySeparator = "/";

const stripComments = (source: string): string =>
  source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/(^|[^:"'`])\/\/.*$/gm, "$1");

const words = (text: string): number =>
  text.match(/[A-Za-z]{2,}/g)?.length ?? 0;

// Two or more words in JSX text, in a string a prop or an expression holds. A
// className, a key, an import path and a data-* name are not sentences.
export function sentenceLiterals(source: string): string[] {
  const code = stripComments(source)
    .split("\n")
    .filter((line) => !/^\s*(import|export)\b.*\bfrom\b/.test(line))
    .join("\n");
  const found: string[] = [];
  for (const m of code.matchAll(/>([^<>{}=;]*[A-Za-z][^<>{}=;]*)</g)) {
    const text = m[1]?.trim() ?? "";
    if (words(text) >= 2) found.push(text);
  }
  for (const m of code.matchAll(
    /(?<![\w-])(?!className\b|key\b|id\b|role\b|type\b|href\b|rel\b|data-[\w-]+\b|aria-(?:labelledby|describedby|controls)\b)[\w-]+=\{?(["'`])([^"'`\n]*)\1/g,
  )) {
    const text = m[2] ?? "";
    if (words(text) >= 2 && /\s/.test(text.trim())) found.push(text);
  }
  for (const m of code.matchAll(
    /(["'`])([A-Z][a-z]+(?: [A-Za-z]+)+[.!?:]?)\1/g,
  )) {
    found.push(m[2] ?? "");
  }
  return [...new Set(found)];
}

// The key of every call msg("key", ...) written with a literal.
export const usedKeys = (source: string): string[] =>
  [...stripComments(source).matchAll(/\bmsg\(\s*["'`]([^"'`]+)["'`]/g)].map(
    (m) => m[1] ?? "",
  );

// keyedFiles maps each source file under dir to its path. The key is the names the
// walk passed through, joined with a slash: a logical key, never given to the file
// system, which only sees the path.
export function keyedFiles(
  dir: string,
  extensions: readonly string[],
): Map<string, string> {
  const found: [string, string][] = [];
  const walk = (at: string, names: string[]): void => {
    for (const entry of readdirSync(at, { withFileTypes: true })) {
      if (["node_modules", "dist", "test"].includes(entry.name)) continue;
      const path = join(at, entry.name);
      if (entry.isDirectory()) walk(path, [...names, entry.name]);
      else if (
        extensions.some((e) => entry.name.endsWith(e)) &&
        !entry.name.endsWith(".test.ts")
      )
        found.push([[...names, entry.name].join(keySeparator), path]);
    }
  };
  walk(dir, []);
  return new Map(found.sort(([x], [y]) => (x < y ? -1 : x > y ? 1 : 0)));
}

export const sourceFiles = (
  dir: string,
  extensions: readonly string[],
): string[] => [...keyedFiles(dir, extensions).values()];

export const read = (path: string): string => readFileSync(path, "utf8");
