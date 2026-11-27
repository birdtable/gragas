// Every query key, from this one file (react.server-state.1): a key is written
// nowhere else, so a document has one cache entry and one name.
export const keys = {
  bundle: () => ["document", "bundle"] as const,
  report: () => ["document", "report"] as const,
  proof: () => ["document", "proof"] as const,
} as const;

export type DocumentKey =
  | ReturnType<typeof keys.bundle>
  | ReturnType<typeof keys.report>
  | ReturnType<typeof keys.proof>;
