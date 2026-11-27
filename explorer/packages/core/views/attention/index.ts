// The attention view's public entry: the page and the region, which
// the shell's compose.ts is the one file to import.
export {
  AttentionPage,
  attentionSearchSchema,
  AttentionRegion,
  type AttentionRegionProps,
} from "./attention.tsx";
export { attentionView, type AttentionView } from "./selectors.ts";
