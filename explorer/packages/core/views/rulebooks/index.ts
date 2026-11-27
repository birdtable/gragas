// The rulebooks view's public entry: the page and the region,
// which the shell's compose.ts is the one file to import.
export {
  RulebooksPage,
  rulebooksSearchSchema,
  RulebooksRegion,
  type RulebooksRegionProps,
} from "./rulebooks.tsx";
export { rulebooksView, type RulebooksView } from "./selectors.ts";
