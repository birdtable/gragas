// The shared table: the hand-authored documents and planted breaks under
// internal/testdata/contract-v1/ are the same files the Go side runs through
// the schema adapter (internal/host/jsonschema), so the zod schemas and the
// Go side accept and reject the same documents.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import * as z from "zod";
import {
  BundleDocument,
  ChangelogDocument,
  PlanDocument,
  ProposalDocument,
  ReportDocument,
  SarifDocument,
} from "../src/index.js";

const examples = fileURLToPath(
  new URL("../../../../internal/testdata/contract-v1/", import.meta.url),
);

const documents: Record<string, z.ZodType> = {
  bundle: BundleDocument,
  report: ReportDocument,
  changelog: ChangelogDocument,
  sarif: SarifDocument,
  plan: PlanDocument,
  proposal: ProposalDocument,
};

const PatchOpSchema = z.object({
  op: z.string(),
  path: z.string(),
  value: z.unknown().optional(),
});
type PatchOp = z.infer<typeof PatchOpSchema>;

const PlantedCasesSchema = z.array(
  z.object({
    name: z.string(),
    patch: z.array(PatchOpSchema),
    problem: z.string(),
  }),
);
const DocumentsSchema = z.array(z.unknown());

const Json = (name: string): unknown =>
  JSON.parse(readFileSync(examples + name, "utf8"));

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

// apply does one op of a patch the way the Go harness does: add puts a new
// key in an object, remove and replace need the key to be there.
function apply(doc: unknown, op: PatchOp): unknown {
  const copy: unknown = structuredClone(doc);
  const parts = op.path.split("/").slice(1);
  const last = parts.pop();
  let at: unknown = copy;
  for (const p of parts) {
    at = Array.isArray(at) ? at[Number(p)] : isRecord(at) ? at[p] : undefined;
  }
  if (last === undefined)
    throw new Error(`patch path ${op.path} names nothing`);
  const holder = at;
  if (Array.isArray(holder)) {
    if (op.op === "add") throw new Error(`add inside a list at ${op.path}`);
    const i = Number(last);
    if (!(i in holder)) throw new Error(`no ${op.path}`);
    if (op.op === "remove") holder.splice(i, 1);
    else if (op.op === "replace") holder[i] = op.value;
    else throw new Error(`unknown op ${op.op}`);
    return copy;
  }
  if (!isRecord(holder)) throw new Error(`no ${op.path}`);
  if (op.op === "add") holder[last] = op.value;
  else if (op.op === "remove" || op.op === "replace") {
    if (!(last in holder)) throw new Error(`no ${op.path}`);
    if (op.op === "remove") Reflect.deleteProperty(holder, last);
    else holder[last] = op.value;
  } else throw new Error(`unknown op ${op.op}`);
  return copy;
}

function kept(input: unknown, output: unknown): boolean {
  if (isRecord(input)) {
    return (
      isRecord(output) &&
      Object.entries(input).every(([k, v]) => kept(v, output[k]))
    );
  }
  if (Array.isArray(input)) {
    return (
      Array.isArray(output) &&
      input.length === output.length &&
      input.every((v, i) => kept(v, output[i]))
    );
  }
  return input === output;
}

describe.each(Object.entries(documents))("the %s document", (kind, schema) => {
  const valid = Json(`${kind}.valid.json`);
  const cases = PlantedCasesSchema.parse(Json(`${kind}.invalid.json`));

  it("parses each hand-authored document and gives it back whole", () => {
    for (const doc of DocumentsSchema.parse(valid)) {
      const parsed = schema.parse(doc);
      expect(kept(doc, parsed)).toBe(true);
      expect(JSON.parse(JSON.stringify(parsed))).toEqual(parsed);
    }
  });

  it("has the planted breaks the Go side has", () => {
    expect(cases.length).toBeGreaterThanOrEqual(3);
  });

  it.each(cases.map((c) => [c.name, c] as const))(
    "rejects %s",
    (_name, planted) => {
      let doc: unknown = valid;
      for (const op of planted.patch) doc = apply(doc, op);
      const results = DocumentsSchema.parse(doc).map(
        (d) => schema.safeParse(d).success,
      );
      expect(results).toContain(false);
    },
  );
});

describe("the harness", () => {
  it("fails on a patch of a missing field and on an add inside a list", () => {
    expect(() =>
      apply([{ a: 1 }], { op: "remove", path: "/0/nothing" }),
    ).toThrow();
    expect(() =>
      apply([{ a: [] }], { op: "add", path: "/0/a/0", value: 1 }),
    ).toThrow();
  });

  it("rejects a document that is not an object at all", () => {
    for (const schema of Object.values(documents)) {
      expect(schema.safeParse(null).success).toBe(false);
      expect(schema.safeParse({}).success).toBe(false);
    }
  });
});
