// What every route's loader is handed: the one query client and the data
// source. A loader asks the client; the router keeps no cache of its own
// (react.routing.5).
import type { DataSource } from "@gragas/core";
import type { QueryClient } from "@tanstack/react-query";

export interface RouterContext {
  readonly client: QueryClient;
  readonly source: DataSource;
}
