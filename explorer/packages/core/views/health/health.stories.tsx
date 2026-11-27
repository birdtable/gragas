import type { Meta, Story } from "../../parts/stories.ts";
import { bundleOf, snapshot } from "./fixture.ts";
import { HealthRegion, states, type HealthRegionProps } from "./health.tsx";

const meta: Meta<HealthRegionProps> = {
  title: "Views/Health",
  states,
  component: HealthRegion,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<HealthRegionProps>;

const retry = (): void => undefined;
const ready = {
  state: "ready",
  file: "bundle.json",
  value: bundleOf(),
} as const;

export const Default: S = { args: { region: ready, onRetry: retry } };
export const Loading: S = {
  args: { region: { state: "loading", file: "bundle.json" }, onRetry: retry },
};
export const Empty: S = {
  args: {
    region: { state: "empty", file: "bundle.json", why: "no-rows" },
    onRetry: retry,
  },
};
export const Error: S = {
  args: {
    region: {
      state: "error",
      file: "bundle.json",
      cause: snapshot(),
      retry: true,
    },
    onRetry: retry,
  },
};
export const Phone390: S = { args: { region: ready, onRetry: retry } };
export const Stale: S = {
  args: {
    region: {
      state: "stale",
      file: "bundle.json",
      value: bundleOf(),
      ageMs: 120_000,
      cause: snapshot(),
      retry: true,
    },
    onRetry: retry,
  },
};
