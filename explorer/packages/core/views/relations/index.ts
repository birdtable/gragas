// The relations view's public entry: the page and the region,
// which the shell's compose.ts is the one file to import.
export {
  RelationsPage,
  relationsSearchSchema,
  RelationsRegion,
  type RelationsRegionProps,
} from "./relations.tsx";
export { relationsView } from "./selectors.ts";
