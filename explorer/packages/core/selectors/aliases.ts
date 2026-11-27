// A deep link names a rule by its id or by an alias it once had.
// The answer is the rule's current id, or null when no rule has the name.
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";

type Bundle = z.output<typeof BundleDocument>;

export function currentRuleId(bundle: Bundle, name: string): string | null {
  const held = bundle.rules.find((rule) => rule.id === name);
  if (held !== undefined) return held.id;
  const renamed = bundle.rules.find((rule) => rule.aliases.includes(name));
  return renamed?.id ?? null;
}
