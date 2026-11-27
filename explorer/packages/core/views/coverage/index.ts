// The coverage view's public entry: the page and the region,
// which the shell's compose.ts is the one file to import.
export {
  CoveragePage,
  coverageSearch,
  CoverageRegion,
  type CoverageRegionProps,
} from "./coverage.tsx";
export { coverageView, type CoverageView } from "./selectors.ts";
