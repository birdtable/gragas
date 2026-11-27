// The health view: the seven numbers a release claims about its
// rulebook, with how each is derived written out as text (and as the tile's
// title, for the pointer), the change of the release from the changelog, and,
// for an overlay, two labelled sets. `HealthRegion` is plain props: it imports
// no data layer, so the element and the iframe mount it with a bundle in hand
// (react.components.1). `HealthPage` is the one orchestrator that asks the
// cache. Every word is the message module's, every number the formatter's.
import type { BundleDocument } from "@gragas/contract";
import { skipToken, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactElement } from "react";
import type * as z from "zod";
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
  Tile,
} from "../../parts/index.ts";
import { bundleRegion } from "../../selectors/index.ts";
import {
  healthView,
  type HealthSet,
  type HealthTile,
  type HealthView as HealthData,
} from "./selectors.ts";
import "../../parts/base.css";
import "./health.css";

type Bundle = z.output<typeof BundleDocument>;

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

function shown(tile: HealthTile): ReactElement {
  if (tile.value.kind === "dash") {
    return <Dash reason={say(tile.value.reason)} />;
  }
  const { value } = tile.value;
  if (tile.unit === "ratio") {
    return (
      <>
        {say("health.value.percent", { value: format.number(value * 100, 1) })}
      </>
    );
  }
  if (tile.unit === "days") {
    return <>{say("health.value.days", { count: value })}</>;
  }
  return <>{format.number(value, 0)}</>;
}

function TileItem({ tile }: Readonly<{ tile: HealthTile }>): ReactElement {
  const derivation = say(tile.derivation);
  return (
    <li data-part="health-tile" title={derivation}>
      <Tile label={say(tile.label)} value={shown(tile)} note={derivation} />
      {tile.proof === null ? null : (
        <p className="gr-health-detail">
          {say("health.proof.counts", {
            passed: format.number(tile.proof.passed, 0),
            ran: format.number(tile.proof.ran, 0),
            notRun: format.number(tile.proof.notRun, 0),
          })}
        </p>
      )}
      <p className="gr-health-detail">
        {say("health.change.delta")}{" "}
        {tile.change.kind === "dash" ? (
          <Dash reason={say(tile.change.reason)} />
        ) : (
          format.number(tile.change.value, 4)
        )}
      </p>
    </li>
  );
}

function SetBlock({
  set,
  labelled,
}: Readonly<{ set: HealthSet; labelled: boolean }>): ReactElement {
  const heading = say(`health.set.${set.role}`);
  const id = `health-set-${set.role}`;
  return (
    <section
      data-part="health-set"
      aria-labelledby={labelled ? id : undefined}
      aria-label={labelled ? undefined : say("health.tiles")}
    >
      {labelled ? <h2 id={id}>{heading}</h2> : null}
      {set.tiles.kind === "dash" ? (
        <p>
          <Dash reason={say(set.tiles.reason)} />
        </p>
      ) : (
        <ul className="gr-health-tiles">
          {set.tiles.value.map((tile) => (
            <TileItem key={tile.name} tile={tile} />
          ))}
        </ul>
      )}
    </section>
  );
}

function Changes({ view }: Readonly<{ view: HealthData }>): ReactElement {
  const { changes } = view;
  return (
    <section data-part="health-changes" aria-labelledby="health-changes">
      <h2 id="health-changes">{say("health.change.heading")}</h2>
      {changes.total === 0 ? (
        <p>{say("health.change.none", { release: changes.release })}</p>
      ) : (
        <>
          <p>
            {say("health.change.line", {
              count: changes.total,
              release: changes.release,
            })}
          </p>
          <ul className="gr-health-kinds">
            {changes.byKind.map((c) => (
              <li key={c.change}>
                {say("health.change.kind", { kind: c.change, count: c.count })}
              </li>
            ))}
          </ul>
        </>
      )}
      <p>
        {changes.previous.kind === "value" ? (
          say("health.change.previous", { version: changes.previous.value })
        ) : (
          <Dash reason={say(changes.previous.reason)} />
        )}
      </p>
    </section>
  );
}

export type HealthRegionProps = Readonly<{
  region: DataState<Bundle>;
  onRetry: () => void;
}>;

function Content({ bundle }: Readonly<{ bundle: Bundle }>): ReactElement {
  const view = healthView(bundle);
  return (
    <>
      {view.sets.map((set) => (
        <SetBlock key={set.role} set={set} labelled={view.sets.length > 1} />
      ))}
      <Changes view={view} />
    </>
  );
}

export function HealthRegion({
  region,
  onRetry,
}: HealthRegionProps): ReactElement {
  return (
    <section data-view="health" data-state={region.state}>
      <h1>{say("shell.view.health")}</h1>
      <p className="gr-health-question">{say("health.question")}</p>
      {region.state === "loading" ? (
        <p role="status">{say("shell.loading")}</p>
      ) : null}
      {region.state === "empty" ? (
        <EmptyFrame
          title={
            region.why === "no-rows"
              ? say("health.no-rules")
              : missingFrame(region.file)
          }
        >
          {region.why === "no-rows" ? noRowsFrame(region.file) : null}
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
        <Content bundle={region.value} />
      ) : null}
    </section>
  );
}

/** The page: reads the one cached bundle the shell loaded and hands it down. */
export function HealthPage(): ReactElement {
  const client = useQueryClient();
  const bundle = useQuery<Loaded<Bundle>>({
    queryKey: keys.bundle(),
    queryFn: skipToken,
    staleTime: Infinity,
  });
  return (
    <HealthRegion
      region={bundleRegion(bundle, bundle.dataUpdatedAt)}
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}
