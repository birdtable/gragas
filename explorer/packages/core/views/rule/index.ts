// The rule view's public entry (gragas#2396): the page, its search schema and
// the region an embed mounts, which the shell's compose.ts is the one file to
// import.
export {
  RulePage,
  RuleRegion,
  ruleSearchSchema,
  termHref,
  type RuleRegionProps,
} from "./rule.tsx";
export { ruleView, type RuleView, type RuleViewResult } from "./selectors.ts";
