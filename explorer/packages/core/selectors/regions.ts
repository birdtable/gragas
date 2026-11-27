// The two documents' data regions: the helper of data/states.ts with the
// bundle's one emptiness (zero rules) named.
import type { BundleDocument, ReportDocument } from "@gragas/contract";
import {
  dataRegion,
  type DataState,
  type QuerySnapshot,
} from "../data/index.ts";
import type * as z from "zod";

type Bundle = z.output<typeof BundleDocument>;
type Report = z.output<typeof ReportDocument>;

export function bundleRegion(
  snapshot: QuerySnapshot<Bundle>,
  now: number,
): DataState<Bundle> {
  return dataRegion("bundle.json", snapshot, now, (b) => b.rules.length === 0);
}

export function reportRegion(
  snapshot: QuerySnapshot<Report>,
  now: number,
): DataState<Report> {
  return dataRegion("report.json", snapshot, now);
}
