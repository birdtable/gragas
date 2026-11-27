import type { Meta, Story } from "../../parts/stories.ts";
import { bundleOf, snapshot } from "./fixture.ts";
import {
  CoverageRegion,
  states,
  type CoverageRegionProps,
} from "./coverage.tsx";

const meta: Meta<CoverageRegionProps> = {
  title: "Views/Coverage",
  states,
  component: CoverageRegion,
  parameters: { a11y: { known: [] } },
};

export default meta;
type S = Story<CoverageRegionProps>;

const retry = (): void => undefined;
const ready = {
  state: "ready",
  file: "bundle.json",
  value: bundleOf(),
} as const;

export const Default: S = {
  args: { region: ready, chosen: null, onRetry: retry },
};
export const Loading: S = {
  args: {
    region: { state: "loading", file: "bundle.json" },
    chosen: null,
    onRetry: retry,
  },
};
export const Empty: S = {
  args: {
    region: { state: "empty", file: "bundle.json", why: "no-rows" },
    chosen: null,
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
    chosen: null,
    onRetry: retry,
  },
};
export const Phone390: S = {
  args: { region: ready, chosen: "enforced", onRetry: retry },
};
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
    chosen: null,
    onRetry: retry,
  },
};
