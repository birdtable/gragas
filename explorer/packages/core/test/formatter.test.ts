// web.content.2: the golden table is read from the one file; a case changed in
// the file and not in the formatter fails (the table is held both ways).
import { describe, expect, it } from "vitest";
import { createFormatter, readBoot } from "../formatter/index.ts";
import { mismatches, readTable } from "./golden.ts";

const table = readTable();

describe("the golden table, formats.json", () => {
  it("has every kind in every locale", () => {
    const kinds = new Set(table.cases.map((c) => c.kind));
    expect([...kinds].sort()).toEqual([
      "date",
      "datetime",
      "duration",
      "list",
      "number",
      "relative",
      "size",
    ]);
    expect(new Set(table.cases.map((c) => c.locale))).toEqual(
      new Set(["en-GB", "en-US"]),
    );
    expect(table.cases.length).toBeGreaterThan(50);
  });

  it("passes through the formatter, every case", () => {
    expect(mismatches(table.cases)).toEqual([]);
  });

  it("fails a case changed in the file and not in the formatter", () => {
    const changed = table.cases.map((c, i) =>
      i === 3 ? { ...c, expected: `${c.expected}!` } : c,
    );
    expect(mismatches(changed)).toHaveLength(1);
  });

  it("fails a formatter that drifts from the file", () => {
    const first = table.cases.find((c) => c.kind === "size");
    expect(first).toBeDefined();
    const drifted = table.cases.map((c) =>
      c === first ? { ...c, locale: "en-US", expected: "wrong" } : c,
    );
    expect(mismatches(drifted)).toHaveLength(1);
  });
});

describe("the boot rule", () => {
  it("states the zone and defaults to UTC and en-GB, never the viewer's", () => {
    expect(readBoot()).toEqual({
      locale: "en-GB",
      timeZone: "UTC",
      lang: "en-GB",
      dir: "ltr",
    });
    expect(readBoot({ locale: "fr-FR" }).locale).toBe("en-GB");
    expect(
      readBoot({ locale: "en-US", timeZone: "Europe/London" }).timeZone,
    ).toBe("Europe/London");
  });

  it("refuses a zone that is not one", () => {
    expect(() => readBoot({ timeZone: "Mars/Olympus" })).toThrow(
      /not a time zone/,
    );
  });

  it("draws a date in the stated zone whatever the process zone is", () => {
    const at = Date.parse("2026-07-01T23:30:00Z");
    expect(createFormatter({ timeZone: "Europe/London" }).date(at)).toBe(
      "2 Jul 2026",
    );
    expect(createFormatter({ timeZone: "UTC" }).date(at)).toBe("1 Jul 2026");
  });
});

describe("invariants of the pure derivations (tests.generated.2)", () => {
  const f = createFormatter();
  it("a duration shows at most two units and never a negative", () => {
    let ran = 0;
    for (let ms = 0; ms < 2_000_000_000; ms += 37_000_123) {
      expect(f.duration(ms).split(" ").length).toBeLessThanOrEqual(4);
      ran += 1;
    }
    expect(ran).toBeGreaterThan(50);
    expect(() => f.duration(-1)).toThrow(RangeError);
  });

  it("a size never reaches 1000 of its unit unless it is the last", () => {
    let ran = 0;
    for (let n = 1; n < 1e15; n = Math.ceil(n * 1.37)) {
      const out = f.size(n);
      const [value = "", unit = ""] = out.split(" ");
      const shown = Number(value.replaceAll(",", ""));
      expect(unit === "TB" ? 0 : shown).toBeLessThan(1000);
      ran += 1;
    }
    expect(ran).toBeGreaterThan(40);
    expect(() => f.size(-1)).toThrow(RangeError);
  });
});
