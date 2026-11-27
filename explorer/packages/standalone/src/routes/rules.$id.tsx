// A rule by any name it has had: the address resolves to the current rule, and
// the canonical address replaces the old one. A name no rule has gone by is
// the not-found page.
import { bundleQuery, currentRuleId, loadRoot } from "@gragas/core";
import { createFileRoute, notFound, redirect } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { manifests } from "../compose.ts";
import { ViewHost } from "../view-host.tsx";

const manifest = manifests.rule;

export const Route = createFileRoute("/rules/$id")({
  validateSearch: manifest.search,
  beforeLoad: async ({ params, context }) => {
    await loadRoot(context.client, context.source, false);
    const loaded = await context.client
      .query(bundleQuery(context.source))
      .catch(() => null);
    if (loaded === null) return;
    const id = currentRuleId(loaded.value, params.id);
    // eslint-disable-next-line @typescript-eslint/only-throw-error -- the router's notFound and redirect are thrown by design
    if (id === null) throw notFound();
    if (id !== params.id) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- see above
      throw redirect({ to: "/rules/$id", params: { id }, replace: true });
    }
  },
  component: function RuleRoute(): ReactElement {
    const { id } = Route.useParams();
    return (
      <ViewHost
        manifest={manifest}
        search={Route.useSearch()}
        params={{ id }}
      />
    );
  },
});
