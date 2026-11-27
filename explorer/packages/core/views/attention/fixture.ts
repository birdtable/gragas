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

/** The bundle with no rule left to look at, for the Empty story's content path. */
export function bareBundle(): Bundle {
  return { ...bundleOf(), rules: [], conflicts: [], rendering_manifest: [] };
}

/** The failure the Error and Stale stories show. */
export function snapshot(): LoadFailure {
  return { kind: "http", file: "bundle.json", status: 500 };
}

/** A day after every review-by the fixture holds, so each active rule is overdue. */
export const lateDay = "2027-04-01";
/** A day the fixture's review-by dates have not reached, so none is overdue. */
export const earlyDay = "2026-11-22";
