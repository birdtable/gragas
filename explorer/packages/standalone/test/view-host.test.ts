// What the host does with a view's search: a value its schema
// accepts is handed on parsed, and one it refuses is not. Pure, in node; the
// browser test beside it holds that an accepted value reaches the address.
import { expect, test } from "vitest";
import * as z from "zod";
import { acceptSearch } from "../src/view-host.tsx";

const schema = z.strictObject({
  q: z.string().optional(),
  strength: z.enum(["must", "should", "may"]).optional(),
});

test("a search the schema accepts is returned parsed", () => {
  expect(acceptSearch(schema, { q: "wrap", strength: "must" })).toEqual({
    ok: true,
    search: { q: "wrap", strength: "must" },
  });
});

test("a value outside the schema's values is refused", () => {
  expect(acceptSearch(schema, { strength: "always" })).toEqual({ ok: false });
});

test("a key the schema does not name is refused", () => {
  expect(acceptSearch(schema, { bogus: "1" })).toEqual({ ok: false });
});
