// What the tools view hands the shell (arch.ports.13): its search schema and its
// page. The shell registers this in compose.ts and imports nothing else of the
// view; a host that mounts the view alone imports `ToolsBody` instead.
import { searchSchema } from "./search.ts";
import { ToolsPage } from "./tools.tsx";

export { searchSchema, type ToolsSearch } from "./search.ts";
export { ToolsBody, ToolsPage, type ToolsBodyProps } from "./tools.tsx";
export const toolsManifest = { search: searchSchema, Page: ToolsPage } as const;
