// Tells the router's types which router this is, so a destination, a
// parameter and a search key are checked where a link or navigation is
// written (react.routing.2).
import type { AppRouter } from "./router.tsx";

declare module "@tanstack/react-router" {
  interface Register {
    router: AppRouter;
  }
}
