// Shared by the data tests: the fixture bundle served over MSW
// and the variants the named cases need, each a change to the fixture's own
// documents, never a hand-written document.
import { fixtureDocuments, type Json } from "@gragas/contract/fixtures";
import type { QueryClient } from "@tanstack/react-query";
import type { BundleDocument, ReportDocument } from "@gragas/contract";
import type * as z from "zod";
import type {
  DocumentFile,
  InlineBlocks,
  Loaded,
  QuerySnapshot,
} from "../index.ts";

export const base = "https://explorer.test/";

export type Bundle = z.output<typeof BundleDocument>;
export type Report = z.output<typeof ReportDocument>;

function isObject(value: Json): value is Record<string, Json> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** One document with some top-level fields replaced. */
export function patched(
  doc: Json | undefined,
  fields: Record<string, Json>,
): Json {
  if (doc === undefined || !isObject(doc)) throw new Error("no such fixture");
  return { ...doc, ...fields };
}

/** The field of a fixture object, for building a patch from its own parts. */
export function field(doc: Json | undefined, name: string): Json {
  if (doc === undefined || !isObject(doc)) throw new Error("no such fixture");
  const found = doc[name];
  if (found === undefined) throw new Error(`no field ${name}`);
  return found;
}

/** The fixture folder's documents, with the standalone report named `report`. */
export function documents(
  change: (name: string, doc: Json) => Json = (_, doc) => doc,
): ReadonlyMap<string, Json> {
  const all = new Map(fixtureDocuments());
  const clean = all.get("report-clean");
  if (clean !== undefined) all.set("report", clean);
  return new Map([...all].map(([name, doc]) => [name, change(name, doc)]));
}

/** A reader of script blocks over a fixed set of texts, the fake the node tests pass. */
export function blocks(
  found: Partial<Record<DocumentFile, Json | undefined>>,
): InlineBlocks {
  return {
    read: (file) => {
      const doc = found[file];
      return doc === undefined ? undefined : JSON.stringify(doc);
    },
  };
}

export const noBlocks: InlineBlocks = { read: () => undefined };

/** What a view's query hook hands the data region, read from the client's cache. */
export function snapshotOf<T>(
  client: QueryClient,
  key: readonly string[],
): QuerySnapshot<T> {
  const state = client.getQueryState<Loaded<T>>(key);
  if (state === undefined) {
    return {
      status: "pending",
      data: undefined,
      error: null,
      dataUpdatedAt: 0,
    };
  }
  return {
    status: state.status,
    data: state.data,
    error: state.error,
    dataUpdatedAt: state.dataUpdatedAt,
  };
}
