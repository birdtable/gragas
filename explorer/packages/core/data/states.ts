// The six states of a data region (web.states.6), a closed union
// the helper returns and never throws: ready, loading, empty, error, stale and
// disconnected. Every failure state offers Retry; stale keeps the last good
// value with its age.
import {
  LoadError,
  type DocumentFile,
  type Loaded,
  type LoadFailure,
} from "./load.ts";

/** The part of a query's result the helper reads, so it is pure and tested in node. */
export interface QuerySnapshot<T> {
  readonly status: "pending" | "error" | "success";
  readonly data: Loaded<T> | undefined;
  readonly error: unknown;
  readonly dataUpdatedAt: number;
}

export type DataState<T> =
  | {
      readonly state: "ready";
      readonly file: DocumentFile;
      readonly value: T;
    }
  | { readonly state: "loading"; readonly file: DocumentFile }
  | {
      readonly state: "empty";
      readonly file: DocumentFile;
      readonly why: "missing";
      readonly retry: true;
    }
  | {
      readonly state: "empty";
      readonly file: DocumentFile;
      readonly why: "no-rows";
    }
  | {
      readonly state: "error";
      readonly file: DocumentFile;
      readonly cause: LoadFailure;
      readonly retry: true;
    }
  | {
      readonly state: "stale";
      readonly file: DocumentFile;
      readonly value: T;
      readonly ageMs: number;
      readonly cause: LoadFailure;
      readonly retry: true;
    }
  | {
      readonly state: "disconnected";
      readonly file: DocumentFile;
      readonly retry: true;
    };

function failureOf(file: DocumentFile, error: unknown): LoadFailure {
  if (error instanceof LoadError) return error.failure;
  const message = error instanceof Error ? error.message : "unknown error";
  return { kind: "unexpected", file, message };
}

/**
 * Reads a query's snapshot as one of the six states. `isEmpty` says when a
 * value that loaded has nothing to draw (a bundle with zero rules).
 */
export function dataRegion<T>(
  file: DocumentFile,
  snapshot: QuerySnapshot<T>,
  now: number,
  isEmpty: (value: T) => boolean = () => false,
): DataState<T> {
  const held = snapshot.data;
  if (snapshot.status === "error") {
    const cause = failureOf(file, snapshot.error);
    if (held !== undefined) {
      return {
        state: "stale",
        file,
        value: held.value,
        ageMs: Math.max(0, now - snapshot.dataUpdatedAt),
        cause,
        retry: true,
      };
    }
    if (cause.kind === "missing") {
      return { state: "empty", file, why: "missing", retry: true };
    }
    if (cause.kind === "disconnected") {
      return { state: "disconnected", file, retry: true };
    }
    return { state: "error", file, cause, retry: true };
  }
  if (snapshot.status === "pending" || held === undefined) {
    return { state: "loading", file };
  }
  if (isEmpty(held.value)) return { state: "empty", file, why: "no-rows" };
  return { state: "ready", file, value: held.value };
}
