// Reading a document: `parse` never throws on
// a document. It returns a closed union the banner and error states read, and
// every arm names the contract version it found.
//
// - the current major is read; a newer minor than this reader's keeps the
//   fields it knows and drops the rest, because a minor only adds fields;
// - any other major, newer or older, is not ready, naming the document's
//   version and this one's (no backwards compatibility before v1);
// - anything else (no version, a malformed one) is an error value.
import type * as z from "zod";

export const CURRENT_MAJOR = 1;
export const CURRENT_MINOR = 3;
/** The contract version this reader writes and reads whole. */
export const CONTRACT_VERSION = `${String(CURRENT_MAJOR)}.${String(CURRENT_MINOR)}`;

export interface Problem {
  readonly path: string;
  readonly message: string;
}

export type ParseResult<T> =
  | { readonly status: "ok"; readonly contract: string; readonly value: T }
  | {
      readonly status: "not-ready";
      readonly documentVersion: string;
      readonly readerVersion: string;
    }
  | {
      readonly status: "error";
      readonly reason:
        "not-a-document" | "no-version" | "bad-version" | "invalid";
      readonly problems: readonly Problem[];
    };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function problemsOf(error: z.ZodError): readonly Problem[] {
  return error.issues.map((issue) => ({
    path: issue.path.map(String).join("."),
    message: issue.message,
  }));
}

function failure(
  reason: "not-a-document" | "no-version" | "bad-version",
  message: string,
): ParseResult<never> {
  return { status: "error", reason, problems: [{ path: "contract", message }] };
}

function dropKeys(
  doc: unknown,
  path: readonly PropertyKey[],
  keys: readonly string[],
): void {
  let at: unknown = doc;
  for (const segment of path) {
    if (isRecord(at)) at = at[String(segment)];
    else if (Array.isArray(at) && typeof segment === "number") at = at[segment];
    else return;
  }
  if (!isRecord(at)) return;
  for (const key of keys) Reflect.deleteProperty(at, key);
}

// A minor adds fields and the generated objects are strict, so a document of a
// newer minor is read by dropping, one round at a time, exactly the keys the
// schema reports as unrecognised. A missing or wrong field is never dropped.
function withoutUnknownFields(schema: z.ZodType, input: unknown): unknown {
  const doc: unknown = structuredClone(input);
  for (let round = 0; round < 64; round++) {
    const result = schema.safeParse(doc);
    if (result.success) return doc;
    let dropped = false;
    for (const issue of result.error.issues) {
      if (issue.code !== "unrecognized_keys") continue;
      dropKeys(doc, issue.path, issue.keys);
      dropped = true;
    }
    if (!dropped) return doc;
  }
  return doc;
}

/** Read a document against one of the generated document schemas. */
export function parse<S extends z.ZodType>(
  schema: S,
  input: unknown,
): ParseResult<z.output<S>> {
  if (!isRecord(input)) {
    return failure("not-a-document", "the document is not an object");
  }
  const claimed = input["contract"];
  if (claimed === undefined) {
    return failure("no-version", "the document names no contract version");
  }
  const match =
    typeof claimed === "string" ? /^(\d+)\.(\d+)$/.exec(claimed) : null;
  if (match === null || typeof claimed !== "string") {
    return failure("bad-version", "the contract version is not major.minor");
  }
  const major = Number(match[1]);
  const minor = Number(match[2]);

  if (major !== CURRENT_MAJOR) {
    return {
      status: "not-ready",
      documentVersion: claimed,
      readerVersion: CONTRACT_VERSION,
    };
  }
  const toRead: unknown =
    minor > CURRENT_MINOR ? withoutUnknownFields(schema, input) : input;

  const result = schema.safeParse(toRead);
  if (!result.success) {
    return {
      status: "error",
      reason: "invalid",
      problems: problemsOf(result.error),
    };
  }
  return { status: "ok", contract: claimed, value: result.data };
}
