// Wiring only: the client, the source, the router, the render. Nothing here
// reads a variable; the build's names are env.ts's.
import "./jitless.ts";
import {
  createExplorerClient,
  documentBlocks,
  type DataSource,
} from "@gragas/core";
import { RouterProvider } from "@tanstack/react-router";
import { StrictMode, type ReactElement } from "react";
import { createRoot } from "react-dom/client";
import { build } from "./env.ts";
import { createAppRouter } from "./router.tsx";

const source: DataSource = {
  base: build.dataBase ?? "",
  inline: documentBlocks(document),
};
const router = createAppRouter({ client: createExplorerClient(), source });

function App(): ReactElement {
  return (
    <StrictMode>
      <RouterProvider router={router} />
    </StrictMode>
  );
}

const root = document.getElementById("root");
if (root !== null) createRoot(root).render(<App />);
