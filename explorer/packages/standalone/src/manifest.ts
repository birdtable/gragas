// What a view hands the shell (arch.ports.13): its search schema, and the page
// it draws from the validated search and the address's parameters. A view
// satisfies this by shape; it imports nothing from the shell.
import type { ComponentType } from "react";
import * as z from "zod";

export type Search = z.ZodObject<z.ZodRawShape, z.core.$strict>;

export type Params = Readonly<Record<string, string>>;

export interface PageProps<S extends Search> {
  readonly search: z.output<S>;
  readonly params: Params;
  /**
   * Writes a new search to the URL; the host refuses a value the view's own
   * schema does not accept. Absent where nothing owns an address (an embed).
   */
  readonly onSearch?: (next: z.input<S>) => void;
}

export interface ViewManifest<S extends Search = Search> {
  readonly search: S;
  readonly Page: ComponentType<PageProps<S>>;
  /** Set only by the shell's stand-in for a view that has not landed. */
  readonly placeholder?: true;
}

/** Keeps each view's own schema type, which a wider annotation would lose. */
export function defineView<S extends Search>(
  manifest: ViewManifest<S>,
): ViewManifest<S> {
  return manifest;
}

export const noSearchSchema = z.strictObject({});
