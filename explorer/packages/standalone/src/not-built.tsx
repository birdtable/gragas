// The page of a view that has not landed: its heading, and a line saying so.
import { EmptyFrame } from "@gragas/core/parts";
import type { ReactElement } from "react";
import { say } from "./say.ts";
import type { Params } from "./manifest.ts";
import { viewTitles, type ViewId } from "./views.ts";

export type NotBuiltProps = Readonly<{ id: ViewId; params: Params }>;

export function NotBuiltPage({ id, params }: NotBuiltProps): ReactElement {
  const named = Object.values(params).join(" ");
  return (
    <section data-view={id}>
      <h1>{say(viewTitles[id])}</h1>
      {named === "" ? null : <p data-region="params">{named}</p>}
      <EmptyFrame title={say("shell.not-built")} />
    </section>
  );
}
