// The terms view's public entry: the page and the region, which
// the shell's compose.ts is the one file to import.
export {
  TermsPage,
  termsSearchSchema,
  TermsRegion,
  type TermsRegionProps,
} from "./terms.tsx";
export { termsUsedBy, termsView, type TermsView } from "./selectors.ts";
