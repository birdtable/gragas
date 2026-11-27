// web.content.1, .3, .9, .10: typed keys, one plural function, a sentence as one
// key with parameters, and every locale with the same keys and placeholders.
import { describe, expect, it } from "vitest";
import {
  createMessages,
  createSay,
  defaultMessages,
  differences,
  pluralForm,
  type Catalogue,
} from "../messages/index.ts";

describe("the message function", () => {
  const msg = createMessages("en-GB");

  it("says a plain word", () => {
    expect(msg("standing.enforced")).toBe("enforced");
  });

  it("builds a sentence from parts as one message", () => {
    expect(
      msg("standing.marked", { standing: "planned", mark: "unproven" }),
    ).toBe("planned, unproven");
    expect(
      msg("observation.unmapped-excluded", {
        overlay: "strict",
        decision: "ruling-1",
      }),
    ).toBe("unmapped, was excluded by strict (ruling-1)");
  });

  it("picks the plural through the one function", () => {
    expect(msg("count.rules", { count: 1 })).toBe("1 rule");
    expect(msg("count.rules", { count: 0 })).toBe("0 rules");
    expect(msg("count.rules", { count: 12 })).toBe("12 rules");
    expect(pluralForm("en-GB", 1, { one: "a", other: "b" })).toBe("a");
    expect(pluralForm("en-GB", 2, { other: "b" })).toBe("b");
  });

  it("refuses a key it does not hold and a plural without a count", () => {
    expect(() => createSay("en-GB")("nope")).toThrow(/no message/);
    expect(() => createSay("en-GB")("count.rules")).toThrow(/numeric/);
    expect(() =>
      createSay("en-GB")("standing.marked", { standing: "x" }),
    ).toThrow(/needs \{mark\}/);
  });
});

describe("a host's override", () => {
  it("replaces a word and keeps the rest", () => {
    const msg = createMessages("en-GB", { "standing.enforced": "held" });
    expect(msg("standing.enforced")).toBe("held");
    expect(msg("standing.planned")).toBe("planned");
  });

  it("is refused when it changes a placeholder, adds a key or turns a plural into a word", () => {
    expect(() =>
      createMessages("xx", { "standing.marked": "{standing}" }),
    ).toThrow(/placeholders of standing.marked/);
    expect(() =>
      createMessages("xx", {
        "standing.marked": "{standing}, {mark}, {other}",
      }),
    ).toThrow(/placeholders/);
    expect(() => createMessages("xx", { "made.up": "x" })).toThrow(
      /unknown key made.up/,
    );
    expect(() => createMessages("xx", { "count.rules": "{count}" })).toThrow(
      /plural/,
    );
  });
});

describe("differences between catalogues (web.content.10)", () => {
  it("finds none in the defaults", () => {
    expect(differences(defaultMessages)).toEqual([]);
  });

  it("finds a missing key, an unknown key and a changed placeholder", () => {
    const kept = Object.entries(defaultMessages).filter(
      ([key]) => key !== "status.active",
    );
    const broken: Catalogue = {
      ...Object.fromEntries(kept),
      extra: "x",
      "standing.marked": "{standing}",
    };
    expect(differences(broken).sort()).toEqual([
      "missing key status.active",
      "placeholders of standing.marked are {standing}, want {mark, standing}",
      "unknown key extra",
    ]);
  });

  it("holds every plural to a count placeholder", () => {
    const plurals = Object.entries(defaultMessages).flatMap(([key, entry]) =>
      typeof entry === "string" ? [] : [[key, entry.other]],
    );
    expect(plurals.length).toBeGreaterThan(0);
    expect(plurals.filter(([, other]) => !other?.includes("{count}"))).toEqual(
      [],
    );
  });
});
