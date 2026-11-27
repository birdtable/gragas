// What the tools address carries (web.states.13): the filters, the grouping and
// the open tool, each one optional so `#/tools` alone is the whole view. A key
// nobody knows is refused, so a mistyped address says so in place of showing a
// view that ignores it.
import { TargetType } from "@gragas/contract";
import * as z from "zod";

export const targetTypes = TargetType.options;

export const searchSchema = z.strictObject({
  tool: z.string().min(1).optional(),
  kind: TargetType.optional(),
  unmapped: z.literal(true).optional(),
  group: z.enum(["tool", "kind"]).optional(),
  open: z.string().min(1).optional(),
});

export type ToolsSearch = z.output<typeof searchSchema>;
