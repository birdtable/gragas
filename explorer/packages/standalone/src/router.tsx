// The router factory (react.routing.4): every setting is set here, once. Hash
// history keeps the page openable from a folder and an iframe's route clear of
// its host's; scroll is restored per history entry; the error and not-found
// pages are the defaults every route inherits; the one query client reaches
// the shell through the router's wrapper.
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import {
  createBrowserHistory,
  createMemoryHistory,
  createRouter,
  type RouterHistory,
} from "@tanstack/react-router";
import type { DataSource } from "@gragas/core";
import * as z from "zod";
import type { ReactElement, ReactNode } from "react";
import { NotFound, RouteError } from "./default-pages.tsx";
import { routeTree } from "./routeTree.gen.ts";

// What the history keeps in window.history.state: the entry's key and index.
const StateSchema = z.looseObject({
  __TSR_key: z.string(),
  __TSR_index: z.number(),
  key: z.string(),
});

/**
 * Hash history whose route is the hash's alone. The library's own hash history
 * appends the page's real query string to the route's search, so a page opened
 * as index.html?utm=x would show every route its error view; the query string
 * is the host's, and the route never reads it.
 */
export function createPageHistory(): RouterHistory {
  return createBrowserHistory({
    parseLocation: () => {
      const hash = window.location.hash;
      const route = hash.length > 1 ? hash.slice(1) : "/";
      const parsed = createMemoryHistory({ initialEntries: [route] }).location;
      const kept = StateSchema.safeParse(window.history.state);
      return { ...parsed, state: kept.success ? kept.data : parsed.state };
    },
    createHref: (href) =>
      `${window.location.pathname}${window.location.search}#${href}`,
  });
}

export interface AppOptions {
  readonly client: QueryClient;
  readonly source: DataSource;
  readonly history?: RouterHistory;
}

// eslint-disable-next-line @typescript-eslint/explicit-function-return-type -- the router's type is the one inferred from the route tree
function build({ client, source, history }: AppOptions) {
  return createRouter({
    routeTree,
    history: history ?? createPageHistory(),
    context: { client, source },
    scrollRestoration: true,
    defaultPreload: false,
    notFoundMode: "root",
    defaultNotFoundComponent: NotFound,
    defaultErrorComponent: RouteError,
    Wrap: ({ children }: { children?: ReactNode }): ReactElement => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
}

export type AppRouter = ReturnType<typeof build>;

export function createAppRouter(options: AppOptions): AppRouter {
  return build(options);
}
