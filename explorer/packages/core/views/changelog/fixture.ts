// The fixture bundle the Go side ships, parsed, for this view's stories and
// browser journey; no hand-written document.
import { BundleDocument, parse } from "@gragas/contract";
import type * as z from "zod";
import type { LoadFailure } from "../../data/index.ts";
import bundleText from "../../../../../internal/host/assets/contract/fixture/bundle.json?raw";

type Bundle = z.output<typeof BundleDocument>;

export function bundleOf(): Bundle {
  const parsed = parse(BundleDocument, JSON.parse(bundleText));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

/** The bundle with no change recorded, for the Empty story's content path. */
export function bareBundle(): Bundle {
  return { ...bundleOf(), changelog: [] };
}

/** The failure the Error and Stale stories show. */
export function snapshot(): LoadFailure {
  return { kind: "http", file: "bundle.json", status: 500 };
}
