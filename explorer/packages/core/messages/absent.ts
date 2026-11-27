// A value the bundle does not state shows a dash with its reason, never a
// zero, a blank or a clean (web.states.5). The reason is a message
// key, so the page words it and a host may say it another way; a selector
// returns the key and no sentence.
import type { MessageKey } from "./index.ts";

export type Dashable<T> =
  | { readonly kind: "value"; readonly value: T }
  | { readonly kind: "dash"; readonly reason: MessageKey };

/** The value, or a dash for `reason` when nobody stated it. */
export function dashable<T>(
  value: T | null | undefined,
  reason: MessageKey,
): Dashable<T> {
  return value === null || value === undefined
    ? { kind: "dash", reason }
    : { kind: "value", value };
}

/** A dash for `reason`, for a value the contract has no field for at all. */
export function dash(reason: MessageKey): Dashable<never> {
  return { kind: "dash", reason };
}
