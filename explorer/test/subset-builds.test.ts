// arch.layers.13, ts.modules.5 and arch.layers.7: each package
// gate/subsets.json lists as a TypeScript subset builds, with the compiler and
// its own tsconfig.build.json, with every package the row forbids
// unresolvable, and links none of it: no source file of a forbidden package is
// in the program, its name and folder are nowhere in what the build writes, and
// it resolves neither for the compiler nor for an ES import from the package's
// folder, which is how the package's own suite would reach it. The build
// writes into memory and never to disk. The rows of build "go" are
// internal/subset_test.go's.
import { spawnSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { basename, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import * as z from "zod";

const repo = fileURLToPath(new URL("../../", import.meta.url));
const packagesDir = join(repo, "explorer", "packages");

const SubsetsSchema = z.object({
  subsets: z.array(
    z.object({
      name: z.string(),
      build: z.enum(["go", "ts"]),
      package: z.string().optional(),
      forbidden: z.array(z.string()).min(1),
      markers: z.array(z.string().min(3)).optional(),
    }),
  ),
});
const ManifestSchema = z.object({
  name: z.string(),
  dependencies: z.record(z.string(), z.string()).optional(),
});

const manifestOf = (folder: string): z.infer<typeof ManifestSchema> =>
  ManifestSchema.parse(
    JSON.parse(readFileSync(join(packagesDir, folder, "package.json"), "utf8")),
  );

const subsets = SubsetsSchema.parse(
  JSON.parse(readFileSync(join(repo, "gate/subsets.json"), "utf8")),
).subsets.filter((s) => s.build === "ts");

// The workspace's packages: each package name, with its folder.
const folders = new Map(
  readdirSync(packagesDir).map((folder) => [manifestOf(folder).name, folder]),
);

// forbiddenOf is each package a row forbids, by name, with its folder;
// @gragas/* is every sibling of the subset's own package. A name that is no
// package of the workspace is an error, so a renamed package is not silently
// let in.
export function forbiddenOf(
  own: string,
  forbidden: readonly string[],
  all: ReadonlyMap<string, string>,
): Map<string, string> {
  const out = new Map<string, string>();
  for (const entry of forbidden) {
    if (entry !== "@gragas/*" && !all.has(entry))
      throw new Error(
        `gate/subsets.json forbids ${entry}, which no package of the workspace is`,
      );
    for (const [name, folder] of all)
      if (name === entry || (entry === "@gragas/*" && folder !== own))
        out.set(name, folder);
  }
  return out;
}

// The entries a workspace package's own source may have when its package.json
// names none (main or module), in the order looked for: a library's index, then
// the shell's main.
const entryNames = ["src/index.ts", "index.ts", "src/main.tsx"];

// entryOf is a package's own entry source, from its folder: the package.json's
// main or module when it has one, else the first of entryNames that exists. A
// package with none is an error that names it, not a path that is not there.
function entryOf(folder: string): string {
  const manifest: unknown = JSON.parse(
    readFileSync(join(packagesDir, folder, "package.json"), "utf8"),
  );
  const named = z
    .object({ main: z.string().optional(), module: z.string().optional() })
    .parse(manifest);
  const entry = [named.main, named.module, ...entryNames].find(
    (name) => name !== undefined && existsSync(join(packagesDir, folder, name)),
  );
  if (entry === undefined)
    throw new Error(
      `explorer/packages/${folder} has no entry: its package.json names no main or module, and none of ${entryNames.join(", ")} exists`,
    );
  return entry;
}

// resolvesAtRunTime asks a Node of its own, in the package's folder, to
// resolve each name as an ES import does, and answers which resolved.
// CommonJS require is not asked: under pnpm's command shims NODE_PATH holds the
// store's hoisted node_modules, where every workspace package is, and an ES
// import never reads NODE_PATH.
function resolvesAtRunTime(
  folder: string,
  names: readonly string[],
): Map<string, boolean> {
  const probe =
    'const out = {}; for (const n of JSON.parse(process.argv[1])) { try { import.meta.resolve(n); out[n] = true; } catch (e) { if (e?.code !== "ERR_MODULE_NOT_FOUND") throw e; out[n] = false; } } process.stdout.write(JSON.stringify(out));';
  const run = spawnSync(
    process.execPath,
    ["--input-type=module", "-e", probe, JSON.stringify(names)],
    { cwd: join(packagesDir, folder), encoding: "utf8" },
  );
  if (run.status !== 0)
    throw new Error(`the resolve probe in ${folder} failed: ${run.stderr}`);
  const got: unknown = JSON.parse(run.stdout);
  return new Map(Object.entries(z.record(z.string(), z.boolean()).parse(got)));
}

// A Build is one compile of a package. The type check and the emit run only
// when a test asks for them: the whole check is slow, and a plant needs only
// its own file's.
interface Build {
  diagnostics: () => string[];
  plantDiagnostics: () => string[];
  files: string[];
  outputs: () => Map<string, string>;
  resolves: (name: string) => boolean;
}

// parsedFiles are the source files every build shares, by language and path.
const parsedFiles = new Map<string, ts.SourceFile>();

// build compiles one package with its tsconfig.build.json, into memory. A
// plant is a source file added under its src, as the compiler sees it.
function build(folder: string, plant?: string): Build {
  const dir = join(packagesDir, folder);
  const parsed = ts.getParsedCommandLineOfConfigFile(
    join(dir, "tsconfig.build.json"),
    {},
    {
      ...ts.sys,
      onUnRecoverableConfigFileDiagnostic: (d) => {
        throw new Error(ts.flattenDiagnosticMessageText(d.messageText, "\n"));
      },
    },
  );
  if (parsed === undefined) throw new Error(`${folder}: no build config`);
  const host = ts.createCompilerHost(parsed.options);
  const planted = resolve(dir, "src", "planted-subset.ts");
  const rootNames = [...parsed.fileNames];
  const { getSourceFile, fileExists, readFile } = host;
  host.getSourceFile = (f, lang, onError, fresh) => {
    if (plant !== undefined && resolve(f) === planted)
      return ts.createSourceFile(f, plant, lang);
    const key =
      typeof lang === "object"
        ? `${String(lang.languageVersion)} ${String(lang.impliedNodeFormat)} ${f}`
        : `${String(lang)} ${f}`;
    const known = parsedFiles.get(key);
    if (known !== undefined) return known;
    const got = getSourceFile.call(host, f, lang, onError, fresh);
    if (got !== undefined) parsedFiles.set(key, got);
    return got;
  };
  if (plant !== undefined) {
    rootNames.push(planted);
    host.fileExists = (f) => resolve(f) === planted || fileExists.call(host, f);
    host.readFile = (f) =>
      resolve(f) === planted ? plant : readFile.call(host, f);
  }
  const program = ts.createProgram({
    rootNames,
    options: parsed.options,
    host,
  });
  const text = (ds: readonly ts.Diagnostic[]): string[] =>
    ds.map((d) => ts.flattenDiagnosticMessageText(d.messageText, "\n"));
  let emitted:
    { outputs: Map<string, string>; diagnostics: string[] } | undefined;
  const emit = (): { outputs: Map<string, string>; diagnostics: string[] } => {
    if (emitted === undefined) {
      const outputs = new Map<string, string>();
      const r = program.emit(undefined, (name, data) =>
        outputs.set(resolve(name), data),
      );
      emitted = { outputs, diagnostics: text(r.diagnostics) };
    }
    return emitted;
  };
  const plantedFile = program.getSourceFile(planted);
  return {
    diagnostics: () => [
      ...text(ts.getPreEmitDiagnostics(program)),
      ...emit().diagnostics,
    ],
    plantDiagnostics: () =>
      plantedFile === undefined
        ? []
        : text(ts.getPreEmitDiagnostics(program, plantedFile)),
    files: program.getSourceFiles().map((f) => resolve(f.fileName)),
    outputs: () => emit().outputs,
    resolves: (name) =>
      ts.resolveModuleName(
        name,
        join(dir, "src", "index.ts"),
        parsed.options,
        host,
      ).resolvedModule !== undefined,
  };
}

// linked says what of the forbidden packages a build reaches: a source file of
// one in the program, its name or its folder in an output (the folder as an
// import that climbs into it writes it, ../<folder>/), a marker in an output,
// or its name resolving for the compiler.
export function linked(
  b: Build,
  forbidden: ReadonlyMap<string, string>,
  markers: readonly string[] = [],
): string[] {
  const out: string[] = [];
  for (const [name, folder] of forbidden) {
    const under = resolve(packagesDir, folder);
    for (const f of b.files)
      if (!relative(under, f).startsWith("..") && relative(under, f) !== "")
        out.push(`the program holds ${f}`);
    for (const [file, data] of b.outputs())
      if (data.includes(name) || data.includes(`../${folder}/`))
        out.push(`${file} names ${name}`);
    if (b.resolves(name)) out.push(`${name} resolves`);
  }
  for (const marker of markers)
    for (const [file, data] of b.outputs())
      if (data.includes(marker)) out.push(`${file} holds the marker ${marker}`);
  return out;
}

const firstOf = (m: ReadonlyMap<string, string>): [string, string] => {
  const [first] = m;
  if (first === undefined) throw new Error("the row forbids nothing");
  return first;
};

// escaped is s as a regular expression that matches it alone.
const escaped = (s: string): string =>
  s.replace(/[.*+?^${}()|[\]\\/]/g, "\\$&");

describe.each(subsets)(
  "the subset $name",
  ({ name, package: own, forbidden, markers }) => {
    if (own === undefined)
      throw new Error(`gate/subsets.json: ${name} names no package`);
    const banned = forbiddenOf(own, forbidden, folders);
    firstOf(banned); // a row that forbids nothing proves nothing

    it("builds with each forbidden package unresolvable and links none of it", () => {
      const b = build(own);
      expect(b.diagnostics()).toEqual([]);
      expect(
        [...b.outputs().keys()].some((f) => basename(f) === "index.js"),
      ).toBe(true);
      expect(linked(b, banned, markers)).toEqual([]);
    });

    it("resolves no forbidden package at run time, and a declared one does", () => {
      const [control] = Object.keys(manifestOf(own).dependencies ?? {});
      if (control === undefined)
        throw new Error(
          `${own} declares no dependency to resolve as the control`,
        );
      const got = resolvesAtRunTime(own, [control, ...banned.keys()]);
      expect(got.get(control), `${control}, which ${own} declares`).toBe(true);
      for (const n of banned.keys())
        expect(got.get(n), `${n} from ${own}`).toBe(false);
    });

    it.each([...banned.keys()])(
      "fails the build on a planted import of %s",
      (n) => {
        const b = build(own, `import "${n}";\n`);
        expect(b.plantDiagnostics().join("\n")).toMatch(
          new RegExp(`Cannot find module .*'${escaped(n)}'`),
        );
      },
    );
  },
);

// The checker's own plants, on the first subset: a path into a forbidden
// package's source, its name and its folder written into the output, a
// marker in the output, and its name resolving are each named.
describe("the subset check", () => {
  const [row] = subsets;
  if (row?.package === undefined)
    throw new Error("gate/subsets.json lists no TypeScript subset");
  const own = row.package;
  const banned = forbiddenOf(own, row.forbidden, folders);
  const [firstName, firstFolder] = firstOf(banned);

  it("names a planted path into a forbidden package's source", () => {
    const entry = entryOf(firstFolder);
    const b = build(
      own,
      `export * from "../../${firstFolder}/${entry.replace(/\.tsx?$/, ".js")}";\n`,
    );
    expect(linked(b, banned)).toContain(
      `the program holds ${resolve(packagesDir, firstFolder, entry)}`,
    );
  });

  it.each([
    ["its name", firstName],
    ["its folder as an import climbs to it", `../../${firstFolder}/x.js`],
  ])("names %s written into the output", (_, text) => {
    const b = build(own, `export const shell = "${text}";\n`);
    expect(linked(b, banned).join("\n")).toContain(`names ${firstName}`);
  });

  it("names a marker written into the output", () => {
    const b = build(own, `export const here = "planted-marker";\n`);
    expect(linked(b, banned, ["planted-marker"]).join("\n")).toContain(
      "holds the marker planted-marker",
    );
  });

  it("names a forbidden package that resolves for the compiler", () => {
    const b = { ...build(own), resolves: () => true };
    expect(linked(b, banned)).toContain(`${firstName} resolves`);
  });

  it("refuses a forbidden name that is no package of the workspace", () => {
    expect(() => forbiddenOf(own, ["@gragas/gone"], folders)).toThrow(
      "no package of the workspace",
    );
  });
});
