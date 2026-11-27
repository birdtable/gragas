import { createFileRoute } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { manifests } from "../compose.ts";
import { ViewHost } from "../view-host.tsx";

const manifest = manifests.health;

export const Route = createFileRoute("/health")({
  validateSearch: manifest.search,
  component: function HealthRoute(): ReactElement {
    return (
      <ViewHost manifest={manifest} search={Route.useSearch()} params={{}} />
    );
  },
});
