import { createFileRoute } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { manifests } from "../compose.ts";
import { ViewHost } from "../view-host.tsx";

const manifest = manifests.rules;

export const Route = createFileRoute("/rules/")({
  validateSearch: manifest.search,
  component: function RulesRoute(): ReactElement {
    return (
      <ViewHost manifest={manifest} search={Route.useSearch()} params={{}} />
    );
  },
});
