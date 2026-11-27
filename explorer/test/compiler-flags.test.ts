// ts.compiler.2, .3, .4, .5: the strict base only grows, a flag left off is
// named with its reason and pinned off, each package's file extends one base
// and sets only lib, types and jsx, and the mode follows how the package ships.
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import * as z from "zod";

const read = (path: string): unknown =>
  JSON.parse(readFileSync(new URL(`../${path}`, import.meta.url), "utf8"));

const BaseSchema = z.object({
  leftOff: z.record(z.string(), z.string().min(10)),
  compilerOptions: z.record(z.string(), z.unknown()),
});
const ConfigSchema = z.object({
  extends: z.string(),
  compilerOptions: z.record(z.string(), z.unknown()).optional(),
});

// The flags the base holds today and the value each has. A flag is added here
// in the change that adds it to the base, and is never taken out (ts.compiler.2).
const floor: Record<string, unknown> = {
  strict: true,
  noUncheckedIndexedAccess: true,
  exactOptionalPropertyTypes: true,
  noImplicitOverride: true,
  noImplicitReturns: true,
  noFallthroughCasesInSwitch: true,
  noPropertyAccessFromIndexSignature: true,
  noUnusedLocals: true,
  noUnusedParameters: true,
  allowUnreachableCode: false,
  allowUnusedLabels: false,
  useUnknownInCatchVariables: true,
  verbatimModuleSyntax: true,
  isolatedModules: true,
  erasableSyntaxOnly: true,
  noUncheckedSideEffectImports: true,
  forceConsistentCasingInFileNames: true,
  skipLibCheck: false,
  resolveJsonModule: false,
  allowJs: false,
};

const published = ["contract", "element", "plugin-docs", "plugin-portal"];
const bundled = ["tokens", "core", "standalone"];

describe("the compiler base", () => {
  const base = BaseSchema.parse(read("tsconfig.base.json"));

  it("holds every flag of the floor at its value", () => {
    for (const [flag, value] of Object.entries(floor)) {
      expect({ [flag]: base.compilerOptions[flag] }).toEqual({ [flag]: value });
    }
  });

  it("pins each flag it leaves off, and gives the reason", () => {
    expect(Object.keys(base.leftOff)).toContain("isolatedDeclarations");
    for (const flag of Object.keys(base.leftOff)) {
      expect({ [flag]: base.compilerOptions[flag] ?? false }).toEqual({
        [flag]: false,
      });
    }
  });

  it("is not a module mode: the library and bundler files are", () => {
    for (const flag of [
      "module",
      "moduleResolution",
      "composite",
      "declaration",
      "noEmit",
      "outDir",
      "rootDir",
    ]) {
      expect(Object.keys(base.compilerOptions)).not.toContain(flag);
    }
  });
});

describe("each package's compiler file", () => {
  for (const name of [...published, ...bundled]) {
    const config = ConfigSchema.parse(read(`packages/${name}/tsconfig.json`));
    const mode = published.includes(name) ? "library" : "bundler";

    it(`${name} extends the ${mode} base and sets only lib, types and jsx`, () => {
      expect(config.extends).toBe(`../../tsconfig.${mode}.json`);
      for (const key of Object.keys(config.compilerOptions ?? {})) {
        expect(["lib", "types", "jsx"], `${name} sets ${key}`).toContain(key);
      }
    });
  }

  it("library mode emits declarations to dist and bundler mode emits nothing", () => {
    const library = BaseSchema.pick({ compilerOptions: true }).parse(
      read("tsconfig.library.json"),
    );
    expect(library.compilerOptions).toMatchObject({
      module: "nodenext",
      composite: true,
      declaration: true,
    });
    expect(library.compilerOptions["outDir"]).toBe("${configDir}/dist");
    const bundler = BaseSchema.pick({ compilerOptions: true }).parse(
      read("tsconfig.bundler.json"),
    );
    expect(bundler.compilerOptions).toMatchObject({
      moduleResolution: "bundler",
      noEmit: true,
    });
  });
});
