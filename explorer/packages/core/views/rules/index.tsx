// The rules view's entry: the manifest the shell registers and the
// plain-props `RulesView` an embed mounts with filters its host holds. The page
// is the one place that reads the data module: it takes the bundle the shell
// already loaded from the cache, words its state, and hands the view plain
// props. It never fetches (the shell owns the load) and never shows a
// project's overrides or report (spec: the view states the rulebook only).
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type ReactElement } from "react";
import { keys, retryLoad, type BundleValue } from "../../data/index.js";
import { bundleRegion } from "../../selectors/regions.ts";
import { filtersSchema, type Filters } from "./filters.ts";
import { RulesView } from "./rules-view.tsx";

export { filtersSchema, filterNames, searchNames } from "./filters.ts";
export type { Filters } from "./filters.ts";
export { RulesView, states } from "./rules-view.tsx";
export type { RulesViewProps } from "./rules-view.tsx";
export { countsFor, factsFor, filterRules, groupByPart } from "./selectors.ts";

function Page({
  search,
  onSearch,
}: Readonly<{
  search: Filters;
  params: Readonly<Record<string, string>>;
  onSearch?: (next: Filters) => void;
}>): ReactElement {
  const client = useQueryClient();
  // The shell's query holds the bundle; this one only reads that entry.
  const snapshot = useQuery<{ value: BundleValue; from: "inline" | "network" }>(
    {
      queryKey: keys.bundle(),
      enabled: false,
    },
  );
  // The clock is state, not a render-time read, so a stale banner's age moves.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const tick = setInterval(() => {
      setNow(Date.now());
    }, 30_000);
    return () => {
      clearInterval(tick);
    };
  }, []);
  const region = bundleRegion(snapshot, now);
  return (
    <RulesView
      region={region}
      filters={search}
      onFilters={(next) => {
        onSearch?.(next);
      }}
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}

export const rulesManifest = { search: filtersSchema, Page } as const;
