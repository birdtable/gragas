// The bundle the Go side ships as its fixture, parsed once for the view's
// stories and its browser test. Not part of the view's entry: nothing in the
// shipped package imports it.
import { BundleDocument, parse } from "@gragas/contract";
import type * as z from "zod";
import bundleText from "../../../../../internal/host/assets/contract/fixture/bundle.json?raw";

export type Bundle = z.output<typeof BundleDocument>;

function load(): Bundle {
  const parsed = parse(BundleDocument, JSON.parse(bundleText));
  if (parsed.status !== "ok") throw new Error("the fixture bundle must parse");
  return parsed.value;
}

export const fixtureBundle: Bundle = load();
