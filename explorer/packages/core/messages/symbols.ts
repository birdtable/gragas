// The symbols of spec 17, which sit beside the words in accessible mode and in
// plain terminals. They are not language: a locale never overrides them.
export const statusSymbols = {
  proposed: "?",
  trial: "^",
  active: "=",
  retired: "x",
} as const;

export const standingSymbols = {
  proposed: "?",
  enforced: "+",
  reviewed: "~",
  planned: "o",
  advisory: "-",
  retired: "x",
} as const;

export const markSymbols = {
  trial: "^",
  blocked: "!",
  unproven: "*",
} as const;

export const observationSymbols = {
  "as-set": "=",
  tighter: ">",
  weaker: "<",
  absent: "_",
  unmapped: ".",
  excluded: "/",
} as const;

export const proofSymbols = {
  pass: "+",
  fail: "!",
  "not-run": ".",
  reviewed: "~",
} as const;

export const runLevelSymbols = { isolated: "i", sandboxed: "s" } as const;
