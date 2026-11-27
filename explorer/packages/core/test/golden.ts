// Runs one case of the golden table through the formatter. The table is
// explorer/contract/testdata/formats.json and nothing copies it.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { createFormatter, instant } from "../formatter/index.ts";

export interface Case {
  readonly kind: string;
  readonly input: unknown;
  readonly locale: string;
  readonly zone?: string;
  readonly expected: string;
}

export const tablePath = fileURLToPath(
  new URL("../../../contract/testdata/formats.json", import.meta.url),
);

const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null;

export function readTable(): { now: string; cases: Case[] } {
  const raw: unknown = JSON.parse(readFileSync(tablePath, "utf8"));
  if (
    !isRecord(raw) ||
    typeof raw["now"] !== "string" ||
    !Array.isArray(raw["cases"])
  ) {
    throw new TypeError("formats.json has no now and cases");
  }
  const cases: Case[] = raw["cases"].map((c: unknown) => {
    if (
      !isRecord(c) ||
      typeof c["kind"] !== "string" ||
      typeof c["locale"] !== "string" ||
      typeof c["expected"] !== "string"
    ) {
      throw new TypeError(`bad case ${JSON.stringify(c)}`);
    }
    const zone = c["zone"];
    return {
      kind: c["kind"],
      input: c["input"],
      locale: c["locale"],
      expected: c["expected"],
      ...(typeof zone === "string" ? { zone } : {}),
    };
  });
  return { now: raw["now"], cases };
}

export function actual(c: Case): string {
  const f = createFormatter({ locale: c.locale, timeZone: c.zone });
  const i = c.input;
  switch (c.kind) {
    case "number":
      if (typeof i === "number") return f.number(i);
      break;
    case "size":
      if (typeof i === "number") return f.size(i);
      break;
    case "duration":
      if (typeof i === "number") return f.duration(i);
      break;
    case "relative":
      if (
        isRecord(i) &&
        typeof i["at"] === "string" &&
        typeof i["now"] === "string"
      ) {
        return f.relative(instant(i["at"]), instant(i["now"]));
      }
      break;
    case "date":
      if (typeof i === "string") return f.date(instant(i));
      break;
    case "datetime":
      if (typeof i === "string") return f.dateTime(instant(i));
      break;
    case "list":
      if (
        Array.isArray(i) &&
        i.every((x): x is string => typeof x === "string")
      ) {
        return f.list(i);
      }
      break;
    default:
      throw new RangeError(`unknown kind ${c.kind}`);
  }
  throw new TypeError(`bad input for ${c.kind}: ${JSON.stringify(i)}`);
}

export const mismatches = (cases: readonly Case[]): string[] =>
  cases.flatMap((c) => {
    const got = actual(c);
    return got === c.expected
      ? []
      : [
          `${c.kind} ${JSON.stringify(c.input)} ${c.locale}: want ${JSON.stringify(c.expected)}, got ${JSON.stringify(got)}`,
        ];
  });
