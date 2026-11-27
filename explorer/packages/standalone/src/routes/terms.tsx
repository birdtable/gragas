import { createFileRoute } from "@tanstack/react-router";
import type { ReactElement } from "react";
import { manifests } from "../compose.ts";
import { ViewHost } from "../view-host.tsx";

const manifest = manifests.terms;

export const Route = createFileRoute("/terms")({
  validateSearch: manifest.search,
  component: function TermsRoute(): ReactElement {
    return (
      <ViewHost manifest={manifest} search={Route.useSearch()} params={{}} />
    );
  },
});
