// Prints the notices of the packages the explorer ships: each production
// dependency of each workspace package, once, with its licence and the licence
// text from its folder. `pnpm run notices` writes it to NOTICES.md; the licences
// test fails when the file is not what this prints. It reads the installed
// tree and touches no network. It is a gate (ts.gates.1): the guard is imported
// first, an empty subject is refused, and the exit code is set, never process.exit.
// An optional first argument names another workspace folder, `--pnpm <script>` names a node
// script to run in place of pnpm, for the plant tests, and `--write <file>` writes the notices to
// that file with node:fs in place of stdout, so no shell redirect is needed.
import "./gate-guard.ts";
import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import * as z from "zod";
import { pnpmEnv } from "./pnpm-env.ts";

const ListSchema = z.record(
  z.string(),
  z.array(
    z.object({
      name: z.string(),
      versions: z.array(z.string()),
      paths: z.array(z.string()),
      license: z.string(),
    }),
  ),
);

interface Notice {
  name: string;
  version: string;
  license: string;
  text: string;
}

function licenseText(folder: string): string {
  const file = readdirSync(folder).find((f) =>
    /^(licen[cs]e|copying)(\.|$)/i.test(f),
  );
  return file === undefined
    ? "(the package carries no licence file)"
    : readFileSync(join(folder, file), "utf8")
        .replaceAll("\r\n", "\n")
        .replace(/[ \t]+$/gm, "")
        .trim();
}

const ManifestSchema = z.object({
  dependencies: z.record(z.string(), z.string()).optional(),
});

// declared names the production dependencies the package at folder declares
// from the registry: a workspace: one is another package of this workspace,
// which the listing leaves out and whose own folder is listed in its turn.
function declared(folder: string): string[] {
  const manifest = ManifestSchema.parse(
    JSON.parse(readFileSync(join(folder, "package.json"), "utf8")),
  );
  return Object.entries(manifest.dependencies ?? {})
    .filter(([, spec]) => !spec.startsWith("workspace:"))
    .map(([name]) => name);
}

// production reads the licence listing of the package at cwd. A listing that
// leaves out a dependency the package declares is refused, never read as
// nothing to print (supply.licences.2): pnpm prints {} with exit
// 0 when it cannot list, and a package whose notices went missing must not
// pass because another package lists some.
function production(
  cwd: string,
  env: NodeJS.ProcessEnv,
  pnpm: [string, string[]],
): Notice[] {
  const out = execFileSync(
    pnpm[0],
    [...pnpm[1], "licenses", "list", "--prod", "--json"],
    { cwd, env, encoding: "utf8" },
  );
  const start = out.indexOf("{");
  if (start === -1)
    throw new Error(`${cwd}: pnpm licenses list printed no JSON listing`);
  const listing = ListSchema.parse(JSON.parse(out.slice(start)));
  const listed = new Set(
    Object.values(listing)
      .flat()
      .map((entry) => entry.name),
  );
  const missing = declared(cwd).filter((name) => !listed.has(name));
  if (missing.length > 0)
    throw new Error(
      `${cwd}: the licence listing leaves out ${missing.join(", ")}, which package.json declares; install the workspace and run it again`,
    );
  return Object.values(listing)
    .flat()
    .flatMap((entry) =>
      entry.versions.map((version, i) => ({
        name: entry.name,
        version,
        license: entry.license,
        text: licenseText(entry.paths[i] ?? ""),
      })),
    );
}

const ProvenanceSchema = z.object({
  copies: z.array(
    z.object({
      package: z.string(),
      version: z.string(),
      license: z.string(),
      folder: z.string(),
    }),
  ),
});

// vendored reads the copies a package carries in its fonts/PROVENANCE.json (the
// ones scripts/vendor-fonts.ts wrote): they are no dependency, so the licence
// listing does not see them, but they ship, so their notice is printed too.
function vendored(folder: string): Notice[] {
  const file = join(folder, "fonts", "PROVENANCE.json");
  if (!existsSync(file)) return [];
  const record = ProvenanceSchema.parse(JSON.parse(readFileSync(file, "utf8")));
  return record.copies.map((copy) => ({
    name: copy.package,
    version: copy.version,
    license: copy.license,
    text: licenseText(join(folder, "fonts", copy.folder)),
  }));
}

const args = process.argv.slice(2);
const at = args.indexOf("--pnpm");
const stub = at === -1 ? undefined : args.splice(at, 2)[1];
const to = args.indexOf("--write");
const target = to === -1 ? undefined : args.splice(to, 2)[1];
const pnpm: [string, string[]] =
  stub === undefined ? ["pnpm", []] : [process.execPath, [stub]];
const root = args[0] ?? fileURLToPath(new URL("..", import.meta.url));
const seen = new Map<string, Notice>();
const packages = readdirSync(join(root, "packages"), { withFileTypes: true })
  .filter((e) => e.isDirectory())
  .map((e) => e.name)
  .sort();
if (packages.length === 0)
  throw new Error("no package under packages/: nothing to print notices for");
const env = pnpmEnv(root);
for (const name of packages) {
  const folder = join(root, "packages", name);
  for (const notice of [
    ...production(folder, env, pnpm),
    ...vendored(folder),
  ]) {
    seen.set(`${notice.name}@${notice.version}`, notice);
  }
}

if (seen.size === 0)
  throw new Error(
    "the licence listing of every package is empty: nothing to print notices for",
  );

const lines = [
  "# Notices",
  "",
  "The packages the explorer ships, and their licences. Generated by `pnpm run notices`; do not edit.",
  "",
];
for (const key of [...seen.keys()].sort()) {
  const n = seen.get(key);
  if (n === undefined) continue;
  lines.push(
    `## ${key}`,
    "",
    `Licence: ${n.license}`,
    "",
    "```",
    n.text,
    "```",
    "",
  );
}
if (target === undefined) process.stdout.write(lines.join("\n"));
else writeFileSync(target, lines.join("\n"));
process.exitCode = 0;
