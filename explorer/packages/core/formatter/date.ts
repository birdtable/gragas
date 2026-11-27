// The one date module (web.content.5): every Date, every clock reading and every
// step of date arithmetic in the explorer lives here. The rest takes epoch
// milliseconds. Pure: it reads no clock, the caller passes "now".
export type RelativeUnit = "second" | "minute" | "hour" | "day";

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

// An instant the contract carries as an ISO 8601 string, as epoch milliseconds.
export function instant(iso: string): number {
  const ms = Date.parse(iso);
  if (Number.isNaN(ms)) {
    throw new RangeError(`not a date: ${iso}`);
  }
  return ms;
}

export function toDate(ms: number): Date {
  return new Date(ms);
}

// How far "then" is from "now", in the largest whole unit under a month, with
// the sign (negative: past). Null when it is a month or more away: the caller
// shows the date.
export function relativeParts(
  thenMs: number,
  nowMs: number,
): { value: number; unit: RelativeUnit } | null {
  const diff = thenMs - nowMs;
  const size = Math.abs(diff);
  const step = (
    unit: RelativeUnit,
    ms: number,
  ): { value: number; unit: RelativeUnit } => ({
    value: Math.trunc(diff / ms) || 0,
    unit,
  });
  if (size < MINUTE) {
    return { value: 0, unit: "second" };
  }
  if (size < HOUR) {
    return step("minute", MINUTE);
  }
  if (size < DAY) {
    return step("hour", HOUR);
  }
  if (size < 30 * DAY) {
    return step("day", DAY);
  }
  return null;
}
