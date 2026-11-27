// The rules view's filters: the eight the spec names for `find`
// (spec 20, "rules by filter: part, kind, strength, standing, mark, tool,
// owner, status"), each a validated search param, and `q`, the free-text
// search the view adds. The names are one list here, so the route's schema, the
// chips and the counts cannot drift apart, and a test pins the list to the
// spec's eight. Pure: no document, window or storage (web.structure.5).
import { Standing, StandingMark, Status, Strength } from "@gragas/contract";
import * as z from "zod";

/** The spec's eight filters, in the spec's order; `find`'s flags carry the same names. */
export const filterNames = [
  "part",
  "kind",
  "strength",
  "standing",
  "mark",
  "tool",
  "owner",
  "status",
] as const;

export type FilterName = (typeof filterNames)[number];

/** Every key of the route's search: the eight filters and the text search. */
export const searchNames = [...filterNames, "q"] as const;

export type SearchName = (typeof searchNames)[number];

const wordSchema = z.string().min(1);

/** The route's search schema: strict, so a key the view does not name is refused. */
export const filtersSchema = z.strictObject({
  part: wordSchema.optional(),
  kind: wordSchema.optional(),
  strength: Strength.optional(),
  standing: Standing.optional(),
  mark: StandingMark.optional(),
  tool: wordSchema.optional(),
  owner: wordSchema.optional(),
  status: Status.optional(),
  q: wordSchema.optional(),
});

export type Filters = z.output<typeof filtersSchema>;

/** The filters in force, as name and value pairs in the list's order; `q` last. */
export function applied(
  filters: Filters,
): readonly { readonly name: SearchName; readonly value: string }[] {
  return searchNames.flatMap((name) => {
    const value = filters[name];
    return value === undefined ? [] : [{ name, value }];
  });
}

/** The filters without one name: the search a chip's remove button leads to. */
export function without(filters: Filters, name: SearchName): Filters {
  return filtersSchema.parse(
    Object.fromEntries(Object.entries(filters).filter(([key]) => key !== name)),
  );
}
