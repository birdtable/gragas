// What the standing view hands the shell (arch.ports.13): its search schema and
// its page. A host that mounts the view alone imports `StandingBody` instead.
import { searchSchema } from "./search.ts";
import { StandingPage } from "./standing.tsx";

export { searchSchema, type StandingSearch } from "./search.ts";
export {
  StandingBody,
  StandingPage,
  type StandingBodyProps,
} from "./standing.tsx";
export const standingManifest = {
  search: searchSchema,
  Page: StandingPage,
} as const;
