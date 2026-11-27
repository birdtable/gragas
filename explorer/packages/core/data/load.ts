// The only module that touches the network or writes a URL (web.structure.1).
// It reads a document from the inline JSON script block the
// standalone packaging writes, else fetches it from the hosted copy, and parses
// it with the generated schema. A document that cannot be read is a LoadError
// carrying one closed `LoadFailure`, so the query client keeps the last good
// data and the data region reads the failure; nothing here retries or caches.
import {
  BundleDocument,
  ProofResult,
  ReportDocument,
  parse,
  type Problem,
} from "@gragas/contract";
import * as z from "zod";

export type DocumentFile = "bundle.json" | "report.json" | "proof.json";

export type LoadFailure =
  | { readonly kind: "missing"; readonly file: DocumentFile }
  | {
      readonly kind: "not-ready";
      readonly file: DocumentFile;
      readonly documentVersion: string;
      readonly readerVersion: string;
    }
  | { readonly kind: "disconnected"; readonly file: DocumentFile }
  | {
      readonly kind: "http";
      readonly file: DocumentFile;
      readonly status: number;
    }
  | {
      readonly kind: "invalid";
      readonly file: DocumentFile;
      readonly problems: readonly Problem[];
    }
  | {
      readonly kind: "unexpected";
      readonly file: DocumentFile;
      readonly message: string;
    };

function describe(failure: LoadFailure): string {
  return `${failure.file}: ${failure.kind}`;
}

export class LoadError extends Error {
  readonly failure: LoadFailure;
  constructor(failure: LoadFailure, cause?: unknown) {
    super(describe(failure), { cause });
    this.name = "LoadError";
    this.failure = failure;
  }
}

/** What a load that worked hands back: the value, and where it came from. */
export interface Loaded<T> {
  readonly value: T;
  readonly from: "inline" | "network";
}

/**
 * The port the orchestrator passes in: a reader of the page's JSON script
 * blocks. It returns the block's text, or undefined when the page carries none
 * for the file. Node tests pass a fake; the browser passes the real reader.
 */
export interface InlineBlocks {
  read(file: DocumentFile): string | undefined;
}

export interface DataSource {
  /** The absolute URL the hosted documents sit beside, ending in a slash. */
  readonly base: string;
  readonly inline: InlineBlocks;
}

async function fetched(
  source: DataSource,
  file: DocumentFile,
): Promise<string> {
  let response: Response;
  try {
    response = await fetch(new URL(file, source.base), {
      redirect: "error",
    });
  } catch (cause) {
    throw new LoadError({ kind: "disconnected", file }, cause);
  }
  if (response.status === 404) throw new LoadError({ kind: "missing", file });
  if (!response.ok) {
    throw new LoadError({ kind: "http", file, status: response.status });
  }
  try {
    return await response.text();
  } catch (cause) {
    throw new LoadError({ kind: "disconnected", file }, cause);
  }
}

function json(file: DocumentFile, text: string): unknown {
  try {
    const found: unknown = JSON.parse(text);
    return found;
  } catch (cause) {
    const message = cause instanceof Error ? cause.message : "not JSON";
    throw new LoadError(
      { kind: "invalid", file, problems: [{ path: "", message }] },
      cause,
    );
  }
}

async function document<S extends z.ZodType>(
  source: DataSource,
  file: DocumentFile,
  schema: S,
): Promise<Loaded<z.output<S>>> {
  const inline = source.inline.read(file);
  const text = inline ?? (await fetched(source, file));
  const from = inline === undefined ? "network" : "inline";
  const result = parse(schema, json(file, text));
  switch (result.status) {
    case "ok":
      return { value: result.value, from };
    case "not-ready":
      throw new LoadError({
        kind: "not-ready",
        file,
        documentVersion: result.documentVersion,
        readerVersion: result.readerVersion,
      });
    case "error":
      throw new LoadError({ kind: "invalid", file, problems: result.problems });
  }
}

export type BundleValue = z.output<typeof BundleDocument>;
export type ReportValue = z.output<typeof ReportDocument>;

export function loadBundle(source: DataSource): Promise<Loaded<BundleValue>> {
  return document(source, "bundle.json", BundleDocument);
}

export function loadReport(source: DataSource): Promise<Loaded<ReportValue>> {
  return document(source, "report.json", ReportDocument);
}

const ProofGridSchema = z.array(ProofResult);
export type ProofGridValue = z.output<typeof ProofGridSchema>;

/**
 * The proof grid, fetched on demand and only where the page is hosted: it is
 * never inlined and never part of a load of the bundle.
 */
export async function loadProofGrid(
  source: DataSource,
): Promise<Loaded<ProofGridValue>> {
  const file = "proof.json";
  const found = ProofGridSchema.safeParse(
    json(file, await fetched(source, file)),
  );
  if (!found.success) {
    const problems = found.error.issues.map((issue) => ({
      path: issue.path.map(String).join("."),
      message: issue.message,
    }));
    throw new LoadError({ kind: "invalid", file, problems });
  }
  return { value: found.data, from: "network" };
}
