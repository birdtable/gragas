// The changelog view's public entry: the page and the region, which
// the shell's compose.ts is the one file to import.
export {
  ChangelogPage,
  changelogSearchSchema,
  ChangelogRegion,
  type ChangelogRegionProps,
} from "./changelog.tsx";
export { changelogView, type ChangelogView } from "./selectors.ts";
