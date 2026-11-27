import { createFileRoute } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { manifests } from "../compose.ts";
import { ViewHost } from "../view-host.tsx";

const manifest = manifests.rulebooks;

export const Route = createFileRoute("/rulebooks")({
  validateSearch: manifest.search,
  component: function RulebooksRoute(): ReactElement {
    return (
      <ViewHost manifest={manifest} search={Route.useSearch()} params={{}} />
    );
  },
});
