// What the data regions say. Plain functions of plain values: this
// module imports nothing, so every screen words a state the same way.
export const unreleasedMark = "Unreleased bundle: not a published release.";

export function missingFrame(file: string): string {
  return `${file} was not found. The explorer has nothing to show until it exists.`;
}

export function noRowsFrame(file: string): string {
  return `${file} holds no rules.`;
}

export function notReadyFrame(
  file: string,
  documentVersion: string,
  readerVersion: string,
): string {
  return `${file} is contract ${documentVersion}, and this explorer reads ${readerVersion}. Use the explorer that matches the bundle.`;
}

export function otherReleaseBanner(
  reportRelease: string,
  bundleRelease: string,
): string {
  return `The report is from release ${reportRelease}; the bundle is ${bundleRelease}.`;
}

export const otherReleaseMark = "Not in this report's release.";

export function staleBanner(ageMs: number): string {
  const minutes = Math.floor(ageMs / 60_000);
  const age = minutes < 1 ? "under a minute" : `${String(minutes)} min`;
  return `Showing data from ${age} ago: the last refresh failed.`;
}

export const retryLabel = "Retry";
