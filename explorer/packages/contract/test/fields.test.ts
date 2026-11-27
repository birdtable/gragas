// The consumer-driven field list, held both ways (arch.ports.7):
// a listed field the schema lacks fails, a field a view reads that the list
// does not name fails, and a schema field no view reads is fine.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import * as z from "zod";
import { fixtureDocuments } from "../src/fixtures/index.js";
import {
  BundleDocument,
  FieldListSchema,
  type FieldList,
  documentNames,
  documents,
  explorerReads,
  fieldExists,
  missingFromSchema,
  parse,
  recordReads,
  unlisted,
} from "../src/main.js";

const listed = FieldListSchema.parse(
  JSON.parse(
    readFileSync(
      fileURLToPath(new URL("../../../contract/fields.json", import.meta.url)),
      "utf8",
    ),
  ),
);

const row = (fields: readonly string[]): Listed =>
  FieldListSchema.parse({
    views: [{ view: "rules", reads: [{ document: "bundle", fields }] }],
  });
type Listed = FieldList;

describe("explorer/contract/fields.json", () => {
  it("parses, and every field it lists is in the schema", () => {
    expect(missingFromSchema(listed)).toEqual([]);
  });

  it("holds against the generated reads of the explorer, which are all in the schema", () => {
    for (const [key, fields] of Object.entries(explorerReads)) {
      const name = z.enum(documentNames).parse(key);
      for (const field of fields) {
        expect({
          name,
          field,
          found: fieldExists(documents[name], field),
        }).toEqual({
          name,
          field,
          found: true,
        });
      }
    }
  });
});

describe("a listed field the schema lacks", () => {
  it("fails: a row naming a field that was never there", () => {
    const problems = missingFromSchema(
      row(["rules[].id", "rules[].no_such_field"]),
    );
    expect(problems).toEqual([
      { view: "rules", document: "bundle", field: "rules[].no_such_field" },
    ]);
  });

  it("fails: a schema change that drops a field a row names", () => {
    const list = row(["identity.name", "released"]);
    expect(missingFromSchema(list)).toEqual([]);
    const dropped = {
      ...documents,
      bundle: BundleDocument.omit({ released: true }),
    };
    expect(missingFromSchema(list, dropped)).toEqual([
      { view: "rules", document: "bundle", field: "released" },
    ]);
  });
});

describe("a field a view reads that the list does not name", () => {
  const fixtures = fixtureDocuments();
  const read = parse(BundleDocument, fixtures.get("bundle"));

  it("is recorded as the list's own spelling of the path", () => {
    if (read.status !== "ok") throw new Error("the fixture bundle must parse");
    const recorder = recordReads(read.value);
    expect(recorder.reads()).toEqual([]);
    for (const rule of recorder.value.rules) void rule.id;
    void recorder.value.identity.name;
    for (const owner of recorder.value.identity.owners) void owner;
    expect(recorder.reads()).toEqual([
      "identity.name",
      "identity.owners[]",
      "rules[].id",
    ]);
  });

  it("fails when the row leaves it out, and passes when the row names it", () => {
    if (read.status !== "ok") throw new Error("the fixture bundle must parse");
    const recorder = recordReads(read.value);
    for (const rule of recorder.value.rules) {
      void rule.id;
      void rule.statement;
    }
    const reads = recorder.reads();
    expect(unlisted(row(["rules[].id"]), "rules", "bundle", reads)).toEqual([
      { view: "rules", document: "bundle", field: "rules[].statement" },
    ]);
    expect(
      unlisted(
        row(["rules[].id", "rules[].statement"]),
        "rules",
        "bundle",
        reads,
      ),
    ).toEqual([]);
    expect(
      unlisted(row(["rules[].id"]), "another view", "bundle", reads),
    ).toHaveLength(2);
  });
});

describe("a schema field no view reads", () => {
  it("is not an error", () => {
    expect(missingFromSchema(row(["released"]))).toEqual([]);
    expect(fieldExists(BundleDocument, "rules[].statement")).toBe(true);
    expect(fieldExists(BundleDocument, "rules[].statements")).toBe(false);
  });
});
