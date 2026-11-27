// The views of the explorer: one folder each, one entry each. The
// standalone's compose.ts is the only file that imports this.
export { rulesManifest } from "./rules/index.tsx";
export {
  RulesView,
  countsFor,
  filterRules,
  groupByPart,
  type Filters,
  type RulesViewProps,
} from "./rules/index.tsx";
