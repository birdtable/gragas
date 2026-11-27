// The consumer-driven field list (arch.ports.7): the file
// explorer/contract/fields.json names, per view, every field of every document
// the view reads. Three checks hold it to the contract, both ways:
//
//  1. a listed field must exist in the schema (`missingFromSchema`);
//  2. a view's test records what the view really read (`recordReads`) and the
//     list must name each of those fields (`unlisted`);
//  3. a schema field no view reads is not an error: the views fill the list.
//
// Field paths use the Go side's spelling: `rules[].id`, `identity.owners[]`.
import * as z from "zod";
import { documentNames, documents, type DocumentName } from "./documents.js";

export const FieldListSchema = z.strictObject({
  views: z.array(
    z.strictObject({
      view: z.string().min(1),
      reads: z.array(
        z.strictObject({
          document: z.enum(documentNames),
          fields: z.array(z.string().min(1)),
        }),
      ),
    }),
  ),
});
export type FieldList = z.infer<typeof FieldListSchema>;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function segmentsOf(field: string): readonly string[] {
  return field
    .split(".")
    .flatMap((part) =>
      part.endsWith("[]") ? [part.slice(0, -2), "[]"] : [part],
    );
}

// What a JSON Schema node stands for once references and the alternatives of a
// nullable or defaulted field are followed.
function expand(
  root: unknown,
  node: unknown,
  seen: number,
): readonly unknown[] {
  if (!isRecord(node) || seen > 32) return [];
  const ref = node["$ref"];
  if (typeof ref === "string" && ref.startsWith("#/") && isRecord(root)) {
    let at: unknown = root;
    for (const part of ref.slice(2).split("/")) {
      at = isRecord(at) ? at[part] : undefined;
    }
    return expand(root, at, seen + 1);
  }
  const branches: unknown[] = [];
  for (const key of ["anyOf", "oneOf", "allOf"]) {
    const list = node[key];
    if (Array.isArray(list)) {
      for (const item of list) {
        const alternative: unknown = item;
        branches.push(...expand(root, alternative, seen + 1));
      }
    }
  }
  return branches.length > 0 ? [node, ...branches] : [node];
}

function step(
  root: unknown,
  nodes: readonly unknown[],
  segment: string,
): readonly unknown[] {
  const next: unknown[] = [];
  for (const node of nodes.flatMap((n) => expand(root, n, 0))) {
    if (!isRecord(node)) continue;
    if (segment === "[]") {
      if (node["items"] !== undefined) next.push(node["items"]);
      continue;
    }
    const properties = node["properties"];
    if (isRecord(properties) && properties[segment] !== undefined) {
      next.push(properties[segment]);
    }
  }
  return next;
}

/** Whether the schema has the field at that path. */
export function fieldExists(schema: z.ZodType, field: string): boolean {
  const root: unknown = z.toJSONSchema(schema, {
    io: "output",
    unrepresentable: "any",
  });
  let nodes: readonly unknown[] = [root];
  for (const segment of segmentsOf(field)) {
    nodes = step(root, nodes, segment);
    if (nodes.length === 0) return false;
  }
  return true;
}

export interface FieldProblem {
  readonly view: string;
  readonly document: DocumentName;
  readonly field: string;
}

/** Every listed field the schema does not have. */
export function missingFromSchema(
  list: FieldList,
  schemas: Readonly<Record<DocumentName, z.ZodType>> = documents,
): readonly FieldProblem[] {
  const problems: FieldProblem[] = [];
  for (const row of list.views) {
    for (const read of row.reads) {
      for (const field of read.fields) {
        if (!fieldExists(schemas[read.document], field)) {
          problems.push({ view: row.view, document: read.document, field });
        }
      }
    }
  }
  return problems;
}

export interface ReadRecorder<T> {
  /** The value to give the view: it reads through this and is recorded. */
  readonly value: T;
  /** The leaf fields read so far, in the list's spelling, sorted and unique. */
  reads(): readonly string[];
}

/** Wrap a document so every leaf a view reads is recorded as a field path. */
export function recordReads<T extends object>(document: T): ReadRecorder<T> {
  const seen = new Set<string>();
  const wrap = <V extends object>(target: V, path: string): V =>
    new Proxy(target, {
      get(inner, key, receiver): unknown {
        const got: unknown = Reflect.get(inner, key, receiver);
        if (typeof key === "symbol") return got;
        const isItem = Array.isArray(inner);
        if (isItem && !/^\d+$/.test(key)) return got;
        const here = isItem
          ? `${path}[]`
          : path === ""
            ? key
            : `${path}.${key}`;
        if (typeof got === "object" && got !== null) {
          return wrap(got, here);
        }
        if (typeof got === "function") return got;
        seen.add(here);
        return got;
      },
    });
  return {
    value: wrap(document, ""),
    reads: () => [...seen].sort(),
  };
}

/** The recorded reads the view's row of the list does not name. */
export function unlisted(
  list: FieldList,
  view: string,
  document: DocumentName,
  reads: readonly string[],
): readonly FieldProblem[] {
  const named = new Set(
    list.views
      .filter((row) => row.view === view)
      .flatMap((row) => row.reads)
      .filter((read) => read.document === document)
      .flatMap((read) => read.fields),
  );
  return reads
    .filter((field) => !named.has(field))
    .map((field) => ({ view, document, field }));
}
