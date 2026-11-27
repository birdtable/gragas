// The formatter (web.content.2): numbers, sizes, durations, relative times,
// dates and lists through one locale rule read at boot. Pure (web.structure.5):
// it reads no document, window or storage, and no clock; "now" is an argument.
// Its golden table is explorer/contract/testdata/formats.json, the one source
// the Go formatter runs as well.
import { readBoot, type Boot, type BootInput } from "./boot.ts";
import { relativeParts, toDate } from "./date.ts";

export * from "./boot.ts";
export { instant } from "./date.ts";

const SIZE_UNITS = ["B", "kB", "MB", "GB", "TB"] as const;
const MONTHS = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
] as const;
const DURATION_UNITS = [
  ["d", 86_400_000],
  ["h", 3_600_000],
  ["min", 60_000],
  ["s", 1000],
] as const;

export interface Formatter {
  readonly boot: Boot;
  number(value: number, maximumFractionDigits?: number): string;
  size(bytes: number): string;
  duration(milliseconds: number): string;
  relative(thenMs: number, nowMs: number): string;
  date(ms: number): string;
  dateTime(ms: number): string;
  list(items: readonly string[]): string;
}

export function createFormatter(input: BootInput = {}): Formatter {
  const boot = readBoot(input);
  const { locale, timeZone } = boot;

  const number = (value: number, maximumFractionDigits = 2): string =>
    new Intl.NumberFormat(locale, { maximumFractionDigits }).format(value);

  const size = (bytes: number): string => {
    if (!Number.isFinite(bytes) || bytes < 0) {
      throw new RangeError(`not a size: ${String(bytes)}`);
    }
    let value = bytes;
    let step = 0;
    while (
      step < SIZE_UNITS.length - 1 &&
      Math.round(value * 10) / 10 >= 1000
    ) {
      value /= 1000;
      step += 1;
    }
    const unit = SIZE_UNITS[step] ?? "B";
    return `${number(value, step === 0 ? 0 : 1)} ${unit}`;
  };

  const duration = (milliseconds: number): string => {
    if (!Number.isFinite(milliseconds) || milliseconds < 0) {
      throw new RangeError(`not a duration: ${String(milliseconds)}`);
    }
    if (milliseconds < 1000) {
      return `${number(Math.round(milliseconds), 0)} ms`;
    }
    let rest = Math.floor(milliseconds / 1000) * 1000;
    const parts: string[] = [];
    for (const [name, ms] of DURATION_UNITS) {
      const whole = Math.floor(rest / ms);
      rest -= whole * ms;
      if (whole > 0 && parts.length < 2) {
        parts.push(`${number(whole, 0)} ${name}`);
      }
    }
    return parts.join(" ");
  };

  // The calendar fields in the stated zone, never the viewer's. The month
  // names are the explorer's own, so an ICU release cannot change a string.
  const fields = (ms: number): Record<string, number> => {
    const out: Record<string, number> = {};
    for (const part of new Intl.DateTimeFormat("en", {
      year: "numeric",
      month: "numeric",
      day: "numeric",
      hour: "numeric",
      minute: "numeric",
      hourCycle: "h23",
      timeZone,
    }).formatToParts(toDate(ms))) {
      if (part.type !== "literal") {
        out[part.type] = Number(part.value);
      }
    }
    return out;
  };

  const date = (ms: number): string => {
    const f = fields(ms);
    const day = String(f["day"]);
    const month = MONTHS[(f["month"] ?? 1) - 1] ?? "";
    return locale === "en-US"
      ? `${month} ${day}, ${String(f["year"])}`
      : `${day} ${month} ${String(f["year"])}`;
  };

  const dateTime = (ms: number): string => {
    const f = fields(ms);
    const two = (n: number | undefined): string => String(n).padStart(2, "0");
    return `${date(ms)}, ${two(f["hour"])}:${two(f["minute"])} ${timeZone}`;
  };

  const relative = (thenMs: number, nowMs: number): string => {
    const parts = relativeParts(thenMs, nowMs);
    if (parts === null) {
      return date(thenMs);
    }
    return new Intl.RelativeTimeFormat(locale, { numeric: "auto" }).format(
      parts.value,
      parts.unit,
    );
  };

  const list = (items: readonly string[]): string =>
    new Intl.ListFormat(locale, { style: "long", type: "conjunction" }).format(
      items,
    );

  return { boot, number, size, duration, relative, date, dateTime, list };
}
