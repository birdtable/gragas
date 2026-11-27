// The status words and symbols of spec section 17, as data every screen reads
// (web.tokens.*). The hand-written rows are the product's words:
// test/status.test.ts compares each table with the one in docs/spec.md, so a
// word, a symbol or a colour role changed on one side alone fails.
//
// A colour role names a colour of tokens.json (grounds aside), so a screen paints
// `--gr-standing-<role>`, `--gr-mark-<name>` or `--gr-observation-<name>` and the
// symbol and the word stay with it: nothing is told by colour alone.

/** 17.1: the status a person types. */
export const statuses = [
  { status: "proposed", word: "proposed", symbol: "?" },
  { status: "trial", word: "trial", symbol: "^" },
  { status: "active", word: "active", symbol: "=" },
  { status: "retired", word: "retired", symbol: "x" },
] as const;

/** 17.2: the standing derived from the bindings, with the colour role it paints. */
export const standings = [
  { standing: "proposed", role: "violet", symbol: "?" },
  { standing: "enforced", role: "green", symbol: "+" },
  { standing: "reviewed", role: "blue", symbol: "~" },
  { standing: "planned", role: "amber", symbol: "o" },
  { standing: "advisory", role: "grey", symbol: "-" },
  { standing: "retired", role: "slate", symbol: "x" },
] as const;

/** 17.3: a mark shown beside a standing; its colour is the token of its own name. */
export const marks = [
  { mark: "trial", symbol: "^" },
  { mark: "blocked", symbol: "!" },
  { mark: "unproven", symbol: "*" },
] as const;

/** 17.4: a project's view of an entry; its colour is the token of its own name. */
export const observations = [
  { observation: "as-set", symbol: "=" },
  { observation: "tighter", symbol: ">" },
  { observation: "weaker", symbol: "<" },
  { observation: "absent", symbol: "_" },
  { observation: "unmapped", symbol: "." },
  { observation: "excluded", symbol: "/" },
] as const;

/** 17.5: a proof cell; the run level, i or s, follows it as a suffix. */
export const proofCells = [
  { cell: "pass", symbol: "+" },
  { cell: "fail", symbol: "!" },
  { cell: "not run", symbol: "." },
  { cell: "reviewed", symbol: "~" },
] as const;
