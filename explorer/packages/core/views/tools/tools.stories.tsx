// The tools view in each state a data region has, over the fixture bundle the Go
// side ships. The body takes plain props, so each story is one state.
import { BundleDocument, parse } from "@gragas/contract";
import bundleText from "../../../../../internal/host/assets/contract/fixture/bundle.json?raw";
import type { Meta, Story } from "../../parts/stories.ts";
import { toolsView } from "./selectors.ts";
import { ToolsBody, type ToolsBodyProps } from "./tools.tsx";

function fixture(): ReturnType<typeof toolsView> {
  const parsed = parse(BundleDocument, JSON.parse(bundleText));
  if (parsed.status !== "ok")
    throw new globalThis.Error("the fixture must parse");
  return toolsView(parsed.value);
}

const view = fixture();
const down = {
  kind: "unexpected",
  file: "bundle.json",
  message: "the server did not answer",
} as const;
const none = (): void => undefined;
const base = { query: {}, onQuery: none, onRetry: none } as const;

const meta: Meta<ToolsBodyProps> = {
  title: "Views/Tools",
  states: [
    "ready",
    "loading",
    "empty",
    "error",
    "stale",
    "disconnected",
    "phone-390",
  ],
  component: ToolsBody,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<ToolsBodyProps>;

export const Ready: S = {
  args: {
    ...base,
    state: { state: "ready", file: "bundle.json", value: view },
  },
};
export const Loading: S = {
  args: { ...base, state: { state: "loading", file: "bundle.json" } },
};
export const Empty: S = {
  args: {
    ...base,
    state: { state: "empty", file: "bundle.json", why: "no-rows" },
  },
};
export const Error: S = {
  args: {
    ...base,
    state: {
      state: "error",
      file: "bundle.json",
      cause: down,
      retry: true,
    },
  },
};
export const Stale: S = {
  args: {
    ...base,
    state: {
      state: "stale",
      file: "bundle.json",
      value: view,
      ageMs: 180_000,
      cause: down,
      retry: true,
    },
  },
};
export const Disconnected: S = {
  args: {
    ...base,
    state: { state: "disconnected", file: "bundle.json", retry: true },
  },
};
export const Phone390: S = {
  args: {
    ...base,
    state: { state: "ready", file: "bundle.json", value: view },
  },
};
