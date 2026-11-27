// The build-time names the page knows, read once and typed (react.config.1).
// Nothing else reads import.meta.env or process.env, and nothing
// reads a variable at run time: the bundler writes these values into the
// script.
import * as z from "zod";

const BuildEnvSchema = z.object({
  /** Where a hosted copy's documents sit, when they are not beside the page. */
  VITE_GRAGAS_DATA_BASE: z.url().optional(),
});

export interface Build {
  readonly dataBase: string | undefined;
}

export function readBuild(raw: Readonly<Record<string, unknown>>): Build {
  const found = BuildEnvSchema.parse(raw);
  return { dataBase: found.VITE_GRAGAS_DATA_BASE };
}

export const build: Build = readBuild(import.meta.env);
