// A value nobody read shows a dash with its reason (web.states.5), never a
// blank and never a zero.
export type Shown<T> =
  | { readonly kind: "value"; readonly value: T }
  | { readonly kind: "dash"; readonly reason: string };

export function shown<T>(
  value: T | null | undefined,
  reason: string,
): Shown<T> {
  return value === null || value === undefined
    ? { kind: "dash", reason }
    : { kind: "value", value };
}
