import { createFileRoute } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { manifests } from "../compose.ts";
import { ViewHost } from "../view-host.tsx";

const manifest = manifests.tools;

export const Route = createFileRoute("/tools")({
  validateSearch: manifest.search,
  component: function ToolsRoute(): ReactElement {
    return (
      <ViewHost manifest={manifest} search={Route.useSearch()} params={{}} />
    );
  },
});
