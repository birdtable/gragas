// The marks every view carries: an unreleased bundle and a report from another release than the bundle.
import type { BundleDocument, ReportDocument } from "@gragas/contract";
import type * as z from "zod";

type Bundle = z.output<typeof BundleDocument>;
type Report = z.output<typeof ReportDocument>;

export interface ViewMarks {
  readonly unreleased: boolean;
}

/** Computed once, here, so no view decides it alone. */
export function viewMarks(bundle: Bundle): ViewMarks {
  return { unreleased: !bundle.released };
}

export interface OtherRelease {
  readonly reportRelease: string;
  readonly bundleRelease: string;
}

/** The banner's content: set when the report's release is not the bundle's. */
export function otherRelease(
  bundle: Bundle,
  report: Report,
): OtherRelease | null {
  if (report.release.version === bundle.release.version) return null;
  return {
    reportRelease: report.release.version,
    bundleRelease: bundle.release.version,
  };
}

export interface PreviousMajor {
  readonly contract: string;
  readonly reader: string;
}

/**
 * The banner's content when the bundle speaks an older contract major than the
 * reader's (the explorer reads the current and previous major, spec 9); null
 * for the current major or a newer one.
 */
export function previousMajor(
  bundle: Bundle,
  readerMajor: number,
  reader: string,
): PreviousMajor | null {
  const major = Number(bundle.versions.contract.split(".")[0]);
  if (!Number.isInteger(major) || major >= readerMajor) return null;
  return { contract: bundle.versions.contract, reader };
}
