// The eleven rulebook views the shell lists (spec S16), by the name each goes
// by in the manifest map, the message key of its heading and its address.
import type { MessageKey } from "@gragas/core";

export const viewIds = [
  "rules",
  "rule",
  "relations",
  "rulebooks",
  "tools",
  "coverage",
  "standing",
  "health",
  "changelog",
  "terms",
  "attention",
] as const;

export type ViewId = (typeof viewIds)[number];

export const viewTitles = {
  rules: "shell.view.rules",
  rule: "shell.view.rule",
  relations: "shell.view.relations",
  rulebooks: "shell.view.rulebooks",
  tools: "shell.view.tools",
  coverage: "shell.view.coverage",
  standing: "shell.view.standing",
  health: "shell.view.health",
  changelog: "shell.view.changelog",
  terms: "shell.view.terms",
  attention: "shell.view.attention",
} as const satisfies Readonly<Record<ViewId, MessageKey>>;
