// The root route: the shell around every page, and the one loader that asks the
// client for the documents (react.routing.5).
import { loadRoot } from "@gragas/core";
import { createRootRouteWithContext } from "@tanstack/react-router";
import type { ReactElement } from "react";
import type { RouterContext } from "../context.ts";
import { Shell } from "../shell.tsx";

export const Route = createRootRouteWithContext<RouterContext>()({
  loader: ({ context }) => loadRoot(context.client, context.source, true),
  component: function Root(): ReactElement {
    const { source } = Route.useRouteContext();
    return <Shell source={source} />;
  },
});
