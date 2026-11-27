// The one freshness policy (react.server-state.2): both documents
// are written once by the engine and never change under a running page, so
// neither goes stale by the clock and neither is collected. A reload or the
// Retry button is the only thing that asks again.
export const freshness = {
  staleTime: Infinity,
  gcTime: Infinity,
  retry: false,
} as const;

/**
 * The size above which the standalone packaging no longer inlines the bundle
 * and writes the fetched form instead (docs/architecture.md, the data module).
 * The handbook page that names the number is not written yet, so no number is
 * kept here: `bytes` stays null, and `waitsFor` names the handbook line this
 * constant is read from once it exists.
 */
export const inlineBundleLimit = {
  bytes: null,
  waitsFor:
    "the handbook line that names the inlined bundle's size limit (docs/architecture.md)",
} as const;
