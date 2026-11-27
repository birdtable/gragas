// The bare address goes to the rules table.
import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/")({
  beforeLoad: () => {
    // eslint-disable-next-line @typescript-eslint/only-throw-error -- the router's redirect is thrown by design
    throw redirect({ to: "/rules", replace: true });
  },
});
