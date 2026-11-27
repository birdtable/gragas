// The clock (S55): the one place the host's time enters the explorer.
// Attention, review-by and zero-by are judged against a day, and gragas keeps no
// timer, so the view asks this function once when it draws and shows the day it
// used (web.content.4). A test replaces the source with `setClock` and restores
// it with the function it returns; no `Date` is read outside this file.

/** A day as `YYYY-MM-DD`, in UTC, the form the bundle's dates have. */
export type Day = string;

export type ClockSource = () => number;

const hostClock: ClockSource = () => Date.now();
let source: ClockSource = hostClock;

/** Today, as a day, from the current source. */
export function today(): Day {
  return new Date(source()).toISOString().slice(0, 10);
}

/** The instant a day starts at, in UTC milliseconds, for the formatter. */
export function dayStart(day: Day): number {
  return Date.parse(`${day}T00:00:00Z`);
}

/** Replace the source of time; the returned function puts the previous one back. */
export function setClock(next: ClockSource): () => void {
  const previous = source;
  source = next;
  return () => {
    source = previous;
  };
}
