// What the contract asks of `parse`: the fixture documents parse, a minor's extra
// field is tolerated and a removed required field is not, a major
// other than this reader's returns not-ready (naming both versions), and a document with no usable version is an error value.
import { describe, expect, it } from "vitest";
import { fixtureDocuments } from "../src/fixtures/index.js";
import {
  BundleDocument,
  CONTRACT_VERSION,
  ChangelogDocument,
  RuleMap,
  ReportDocument,
  parse,
} from "../src/main.js";

const fixtures = fixtureDocuments();

function fixture(name: string): Record<string, unknown> {
  const document = fixtures.get(name);
  if (
    typeof document !== "object" ||
    document === null ||
    Array.isArray(document)
  ) {
    throw new Error(`fixture ${name} is not a document`);
  }
  return Object.fromEntries(Object.entries(document));
}

describe("the fixture documents", () => {
  it("parse: the bundle, both reports, the changelog and the rule map", () => {
    const bundle = parse(BundleDocument, fixture("bundle"));
    expect(bundle.status).toBe("ok");
    for (const name of ["report-clean", "report-weakened"]) {
      expect(parse(ReportDocument, fixture(name)).status).toBe("ok");
    }
    expect(parse(ChangelogDocument, fixture("changelog")).status).toBe("ok");
    if (bundle.status !== "ok") return;
    expect(bundle.contract).toBe(CONTRACT_VERSION);
    expect(bundle.value.rule_map.length).toBeGreaterThan(0);
    for (const map of bundle.value.rule_map) {
      expect(RuleMap.safeParse(map).success).toBe(true);
    }
  });
});

describe("a minor's added fields", () => {
  it("are tolerated in a newer minor, at any depth, and kept out of the value", () => {
    const doc = fixture("report-clean");
    const pin = doc["pin"];
    if (typeof pin !== "object" || pin === null) throw new Error("no pin");
    const newer = {
      ...doc,
      contract: "1.7",
      added_in_one_seven: true,
      pin: { ...pin, also_added: 1 },
    };
    const read = parse(ReportDocument, newer);
    expect(read.status).toBe("ok");
    if (read.status !== "ok") return;
    expect(read.contract).toBe("1.7");
    expect(read.value).not.toHaveProperty("added_in_one_seven");
  });

  it("are refused in this reader's own minor", () => {
    const read = parse(ReportDocument, {
      ...fixture("report-clean"),
      added_in_one_seven: true,
    });
    expect(read.status).toBe("error");
  });

  it("do not excuse a removed required field", () => {
    const without = Object.fromEntries(
      Object.entries(fixture("bundle")).filter(([key]) => key !== "released"),
    );
    for (const contract of [CONTRACT_VERSION, "1.7"]) {
      const read = parse(BundleDocument, { ...without, contract });
      expect(read.status).toBe("error");
      if (read.status !== "error") continue;
      expect(read.reason).toBe("invalid");
      expect(read.problems.map((p) => p.path)).toContain("released");
    }
  });
});

describe("the major", () => {
  it("of the previous major is not ready either, naming both versions, and is not read", () => {
    const read = parse(ReportDocument, {
      ...fixture("report-clean"),
      contract: "0.9",
    });
    expect(read).toEqual({
      status: "not-ready",
      documentVersion: "0.9",
      readerVersion: CONTRACT_VERSION,
    });
  });

  it("of a newer major is not ready, naming both versions, and is not read", () => {
    const read = parse(ReportDocument, {
      contract: "2.0",
      anything: ["at all"],
    });
    expect(read).toEqual({
      status: "not-ready",
      documentVersion: "2.0",
      readerVersion: CONTRACT_VERSION,
    });
  });
});

describe("a document with no usable version", () => {
  it.each([
    ["is not an object", null, "not-a-document"],
    ["is an array", [], "not-a-document"],
    ["names no version", {}, "no-version"],
    ["names a version that is not text", { contract: 1 }, "bad-version"],
    [
      "names a version that is not major.minor",
      { contract: "one" },
      "bad-version",
    ],
    ["names a version with a patch", { contract: "1.0.0" }, "bad-version"],
  ] as const)("%s: an error value, not a throw", (_name, input, reason) => {
    const read = parse(ReportDocument, input);
    expect(read.status).toBe("error");
    if (read.status !== "error") return;
    expect(read.reason).toBe(reason);
  });
});
