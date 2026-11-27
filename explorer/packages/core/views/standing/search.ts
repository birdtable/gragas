// What the standing address carries (web.states.13): the grouping and the two
// filters, each optional so `#/standing` alone is the whole view. A key nobody
// knows is refused.
import * as z from "zod";

export const searchSchema = z.strictObject({
  by: z.enum(["part", "kind"]).optional(),
  part: z.string().min(1).optional(),
  kind: z.string().min(1).optional(),
});

export type StandingSearch = z.output<typeof searchSchema>;
