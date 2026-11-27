// The router's defaults, set once (react.routing.6): what a route shows when
// its address does not match, and when its search is refused or it throws.
// Each has a page heading, so focus and the title have somewhere to go.
import { ErrorFrame } from "@gragas/core/parts";
import {
  SearchParamError,
  useLocation,
  useRouter,
  type ErrorComponentProps,
} from "@tanstack/react-router";
import type { ReactElement } from "react";
import { say } from "./say.ts";

export function NotFound(): ReactElement {
  const path = useLocation({ select: (l) => l.pathname });
  return (
    <section data-view="not-found">
      <h1>{say("shell.not-found.heading")}</h1>
      <p>{say("shell.not-found.body", { path })}</p>
    </section>
  );
}

export function RouteError({
  error,
  reset,
}: ErrorComponentProps): ReactElement {
  const router = useRouter();
  const message =
    error instanceof SearchParamError
      ? say("shell.error.search")
      : say("shell.error.unexpected");
  return (
    <section data-view="error">
      <h1>{say("part.error.heading")}</h1>
      <ErrorFrame
        message={message}
        onRetry={() => {
          reset();
          void router.invalidate();
        }}
      />
    </section>
  );
}
