import { createFileRoute } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { manifests } from "../compose.ts";
import { ViewHost } from "../view-host.tsx";

const manifest = manifests.relations;

export const Route = createFileRoute("/relations/$kind/$id")({
  validateSearch: manifest.search,
  component: function RelationsRoute(): ReactElement {
    const { kind, id } = Route.useParams();
    return (
      <ViewHost
        manifest={manifest}
        search={Route.useSearch()}
        params={{ kind, id }}
      />
    );
  },
});
