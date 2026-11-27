// The one place a view meets the router (arch.ports.13): a route
// hands it the view's manifest, the validated search and the address's
// parameters, and it gives the page `onSearch`, which writes a new search to
// the URL. A value the view's own schema refuses is not written, so the
// address never holds a search the route would then reject.
import { useLocation, useRouter } from "@tanstack/react-router";
import { useCallback, type ReactElement } from "react";
import type * as z from "zod";
import type { Params, Search, ViewManifest } from "./manifest.ts";

/** What `acceptSearch` does with a value: the parsed search, or a refusal. */
export type Accepted<S extends Search> =
  { readonly ok: true; readonly search: z.output<S> } | { readonly ok: false };

type Accept = <S extends Search>(schema: S, next: unknown) => Accepted<S>;

/** Pure: a search the schema accepts, else a refusal. Tested in node. */
export const acceptSearch: Accept = (schema, next) => {
  const parsed = schema.safeParse(next);
  return parsed.success ? { ok: true, search: parsed.data } : { ok: false };
};

export type ViewHostProps<S extends Search> = Readonly<{
  manifest: ViewManifest<S>;
  search: z.output<S>;
  params: Params;
}>;

export function ViewHost<S extends Search>({
  manifest,
  search,
  params,
}: ViewHostProps<S>): ReactElement {
  const router = useRouter();
  const pathname = useLocation({ select: (l) => l.pathname });
  const { Page } = manifest;
  const onSearch = useCallback(
    (next: z.input<S>): void => {
      const accepted = acceptSearch(manifest.search, next);
      if (!accepted.ok) return;
      // The path is this route's own, so the search is all that changes.
      router.history.replace(
        `${pathname}${router.options.stringifySearch(accepted.search)}`,
      );
    },
    [manifest, router, pathname],
  );
  return <Page search={search} params={params} onSearch={onSearch} />;
}
