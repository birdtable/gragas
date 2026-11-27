// The coverage view: rules to entries and entries to rules, by
// part and layer and by tool, each as a bar with its numbers written beside it,
// coloured by the standing the reader chose (the choice is in the address, so a
// reload finds it). The upstream set is one covered unit that expands to its
// entries. `CoverageRegion` is plain props and imports no data layer, so the
// element and the iframe can mount it (react.components.1); `CoveragePage` is
// the one orchestrator that asks the cache. Every word is the message module's,
// every number the formatter's.
import { Collapsible } from "@base-ui/react/collapsible";
import type { BundleDocument } from "@gragas/contract";
import { skipToken, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactElement } from "react";
import * as z from "zod";
import {
  keys,
  retryLoad,
  type DataState,
  type Loaded,
} from "../../data/index.ts";
import { createFormatter } from "../../formatter/index.ts";
import {
  createSay,
  missingFrame,
  noRowsFrame,
  staleBanner,
} from "../../messages/index.ts";
import {
  Banner,
  Dash,
  EmptyFrame,
  ErrorFrame,
  Link,
  StackedStandingBar,
  StatusMark,
} from "../../parts/index.ts";
import { bundleRegion } from "../../selectors/index.ts";
import {
  coverageView,
  standingOrder,
  type Chosen,
  type CoverageView as CoverageData,
  type PartRow,
  type Ratio,
  type ToolRow,
} from "./selectors.ts";
import "../../parts/base.css";
import "./coverage.css";

type Bundle = z.output<typeof BundleDocument>;

/** The address's search: the one standing the colour follows, or none for all. */
export const coverageSearch = z.strictObject({
  standing: z.enum(standingOrder).optional(),
});

const say = createSay("en");
const format = createFormatter();

export const states = [
  "default",
  "loading",
  "empty",
  "error",
  "phone390",
  "stale",
] as const;

const figure = (r: Ratio): { n: string; m: string } => ({
  n: format.number(r.n, 0),
  m: format.number(r.of, 0),
});

function Meter({
  ratio,
  chosen,
  label,
}: Readonly<{ ratio: Ratio; chosen: Chosen; label: string }>): ReactElement {
  return (
    <meter
      className="gr-coverage-meter"
      data-standing={chosen ?? "all"}
      min={0}
      max={ratio.of}
      value={ratio.n}
      aria-label={label}
    />
  );
}

function Switch({ chosen }: Readonly<{ chosen: Chosen }>): ReactElement {
  return (
    <nav data-part="coverage-switch" aria-label={say("coverage.switch")}>
      <ul className="gr-coverage-switch">
        <li>
          <Link href="#/coverage">
            <span aria-current={chosen === null ? "true" : undefined}>
              {say("coverage.switch.all")}
            </span>
          </Link>
        </li>
        {standingOrder.map((standing) => (
          <li key={standing}>
            <Link href={`#/coverage?standing=${standing}`}>
              <span aria-current={chosen === standing ? "true" : undefined}>
                <StatusMark standing={standing} />
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function Pair({
  title,
  value,
  chosen,
  held,
}: Readonly<{
  title: string;
  value: Ratio;
  chosen: Chosen;
  held: boolean;
}>): ReactElement {
  const { n, m } = figure(value);
  const text = say(held ? "coverage.held.of" : "coverage.explained.of", {
    n,
    m,
  });
  return (
    <li data-part="coverage-ratio">
      <h3>{title}</h3>
      <Meter ratio={value} chosen={chosen} label={text} />
      <p>{text}</p>
    </li>
  );
}

function PartLine({
  row,
  chosen,
}: Readonly<{ row: PartRow; chosen: Chosen }>): ReactElement {
  return (
    <li data-part="coverage-part" data-part-slug={row.slug}>
      <h3>{say("coverage.part.row", { name: row.name, layer: row.layer })}</h3>
      <div className="gr-coverage-pair">
        <div data-direction="rules-to-entries">
          {row.rules.kind === "dash" ? (
            <Dash reason={say(row.rules.reason)} />
          ) : (
            <>
              <Meter
                ratio={row.rules.value}
                chosen={chosen}
                label={say("coverage.rules-held", figure(row.rules.value))}
              />
              <p>{say("coverage.rules-held", figure(row.rules.value))}</p>
            </>
          )}
        </div>
        <div data-direction="entries-to-rules">
          {row.entries.kind === "dash" ? (
            <Dash reason={say(row.entries.reason)} />
          ) : (
            <>
              <Meter
                ratio={row.entries.value}
                chosen={chosen}
                label={say(
                  "coverage.entries-traced",
                  figure(row.entries.value),
                )}
              />
              <p>{say("coverage.entries-traced", figure(row.entries.value))}</p>
            </>
          )}
        </div>
      </div>
      <StackedStandingBar segments={row.segments} />
    </li>
  );
}

function ToolLine({
  row,
  chosen,
}: Readonly<{ row: ToolRow; chosen: Chosen }>): ReactElement {
  const { n, m } = figure(row.entries);
  const text = say("coverage.tool.row", { tool: row.tool, n, m });
  return (
    <li data-part="coverage-tool">
      <Meter ratio={row.entries} chosen={chosen} label={text} />
      <p>{text}</p>
    </li>
  );
}

function Upstream({ view }: Readonly<{ view: CoverageData }>): ReactElement {
  const { upstream } = view;
  if (upstream.kind === "dash") {
    return (
      <p data-part="coverage-upstream">
        <Dash reason={say(upstream.reason)} />
      </p>
    );
  }
  const set = upstream.value;
  return (
    <Collapsible.Root data-part="coverage-upstream">
      <Collapsible.Trigger className="gr-button gr-target">
        {say("coverage.upstream", {
          rulebook: set.rulebook,
          version: set.version,
        })}
        {": "}
        {say("coverage.upstream.count", { count: set.rules.length })}
      </Collapsible.Trigger>
      <Collapsible.Panel>
        <ul>
          {set.rules.map((r) => (
            <li key={r.id}>{r.id}</li>
          ))}
        </ul>
      </Collapsible.Panel>
    </Collapsible.Root>
  );
}

function Content({
  bundle,
  chosen,
}: Readonly<{ bundle: Bundle; chosen: Chosen }>): ReactElement {
  const view = coverageView(bundle, chosen);
  return (
    <>
      <Switch chosen={chosen} />
      <section data-part="coverage-ratios" aria-labelledby="coverage-ratios">
        <h2 id="coverage-ratios">{say("coverage.ratios")}</h2>
        <ul className="gr-coverage-ratios">
          <Pair
            title={say("coverage.held")}
            value={view.held}
            chosen={chosen}
            held
          />
          <Pair
            title={say("coverage.explained")}
            value={view.explained}
            chosen={chosen}
            held={false}
          />
        </ul>
      </section>
      <section aria-labelledby="coverage-parts">
        <h2 id="coverage-parts">{say("coverage.by-part")}</h2>
        <ul className="gr-coverage-list">
          {view.parts.map((row) => (
            <PartLine key={row.slug} row={row} chosen={chosen} />
          ))}
        </ul>
      </section>
      <section aria-labelledby="coverage-tools">
        <h2 id="coverage-tools">{say("coverage.by-tool")}</h2>
        <ul className="gr-coverage-list">
          {view.tools.map((row) => (
            <ToolLine key={row.tool} row={row} chosen={chosen} />
          ))}
        </ul>
        <Upstream view={view} />
      </section>
    </>
  );
}

export type CoverageRegionProps = Readonly<{
  region: DataState<Bundle>;
  chosen: Chosen;
  onRetry: () => void;
}>;

export function CoverageRegion({
  region,
  chosen,
  onRetry,
}: CoverageRegionProps): ReactElement {
  return (
    <section data-view="coverage" data-state={region.state}>
      <h1>{say("shell.view.coverage")}</h1>
      <p className="gr-coverage-question">{say("coverage.question")}</p>
      {region.state === "loading" ? (
        <p role="status">{say("shell.loading")}</p>
      ) : null}
      {region.state === "empty" ? (
        <EmptyFrame
          title={
            region.why === "no-rows"
              ? say("coverage.no-rules")
              : missingFrame(region.file)
          }
        >
          {region.why === "no-rows" ? (
            <>
              <p>{noRowsFrame(region.file)}</p>
              <p>{say("coverage.handbook")}</p>
            </>
          ) : null}
        </EmptyFrame>
      ) : null}
      {region.state === "error" || region.state === "disconnected" ? (
        <ErrorFrame
          message={say("shell.data.failed", { file: region.file })}
          onRetry={onRetry}
        />
      ) : null}
      {region.state === "stale" ? (
        <Banner tone="warning">{staleBanner(region.ageMs)}</Banner>
      ) : null}
      {region.state === "ready" || region.state === "stale" ? (
        <Content bundle={region.value} chosen={chosen} />
      ) : null}
    </section>
  );
}

/** The page: reads the one cached bundle the shell loaded and hands it down. */
export function CoveragePage({
  search,
}: Readonly<{ search: z.output<typeof coverageSearch> }>): ReactElement {
  const client = useQueryClient();
  const bundle = useQuery<Loaded<Bundle>>({
    queryKey: keys.bundle(),
    queryFn: skipToken,
    staleTime: Infinity,
  });
  return (
    <CoverageRegion
      region={bundleRegion(bundle, bundle.dataUpdatedAt)}
      chosen={search.standing ?? null}
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}
