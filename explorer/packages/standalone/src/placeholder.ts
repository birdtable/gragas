// The shell's stand-in for a view that has not landed: the schema the route
// will take, and a page saying the view is not built. A view ticket replaces
// its row in compose.ts and this goes away for it.
import { createElement, type ReactElement } from "react";
import {
  defineView,
  type PageProps,
  type Search,
  type ViewManifest,
} from "./manifest.ts";
import { NotBuiltPage } from "./not-built.tsx";
import type { ViewId } from "./views.ts";

export function notBuilt<S extends Search>(
  id: ViewId,
  search: S,
): ViewManifest<S> {
  function Page({ params }: PageProps<S>): ReactElement {
    return createElement(NotBuiltPage, { id, params });
  }
  return defineView({ search, Page, placeholder: true });
}
