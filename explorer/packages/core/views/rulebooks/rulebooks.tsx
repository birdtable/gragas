// The rulebooks view: the chain this bundle
// belongs to as boxes with their pins, its imports by part, the rule maps in and
// out per mapped rulebook, its governance and its licences, and for an archived
// rulebook its successor. It reads the one bundle and nothing else: gragas never
// knows which projects pin the chain, and the withdrawn list is a separate signed
// file, so neither is drawn here. `RulebooksRegion` is plain props: it imports no
// data layer, so the element and the iframe mount it with a bundle in hand
// (react.components.1). `RulebooksPage` is the one orchestrator that asks the
// cache. Every word is the message module's.
import { type BundleDocument } from "@gragas/contract";
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
import { Banner, Dash, EmptyFrame, ErrorFrame } from "../../parts/index.ts";
import { bundleRegion } from "../../selectors/index.ts";
import {
  rulebooksView,
  type ChainBox,
  type RuleMapRow,
  type RulebooksView,
} from "./selectors.ts";
import "../../parts/base.css";
import "./rulebooks.css";

type Bundle = z.output<typeof BundleDocument>;

export const rulebooksSearchSchema = z.strictObject({});

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

function Chain({
  chain,
}: Readonly<{ chain: readonly ChainBox[] }>): ReactElement {
  return (
    <section data-part="rulebooks-chain" aria-labelledby="rulebooks-chain">
      <h2 id="rulebooks-chain">{say("rulebooks.chain")}</h2>
      <ol className="gr-rulebooks-chain">
        {chain.map((box) => (
          <li
            key={`${box.role}:${box.name}`}
            data-part="rulebooks-box"
            data-role={box.role}
          >
            <p className="gr-rulebooks-role">
              {say(`rulebooks.chain.role.${box.role}`)}
            </p>
            <p className="gr-rulebooks-name">{box.name}</p>
            <p>{say("rulebooks.chain.pin", { version: box.version })}</p>
            {box.parts.length === 0 ? null : (
              <p>
                {say("rulebooks.imports.parts", {
                  parts: box.parts.join(", "),
                })}
              </p>
            )}
          </li>
        ))}
      </ol>
    </section>
  );
}

function Imports({ view }: Readonly<{ view: RulebooksView }>): ReactElement {
  return (
    <section data-part="rulebooks-imports" aria-labelledby="rulebooks-imports">
      <h2 id="rulebooks-imports">{say("rulebooks.imports")}</h2>
      {view.imports.length === 0 ? (
        <p>{say("rulebooks.imports.none")}</p>
      ) : (
        <ul className="gr-rulebooks-list">
          {view.imports.map((imp) => (
            <li key={imp.rulebook}>
              <strong>{imp.rulebook}</strong>{" "}
              {say("rulebooks.chain.pin", { version: imp.version })}
              {". "}
              {say("rulebooks.imports.parts", { parts: imp.parts.join(", ") })}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function RuleMap({ row }: Readonly<{ row: RuleMapRow }>): ReactElement {
  const n = (count: number): string => format.number(count, 0);
  return (
    <li data-part="rulebooks-map">
      <strong>{row.rulebook}</strong>
      <p>
        {say("rulebooks.maps.counts", {
          kept: n(row.kept),
          tighter: n(row.tighter),
          weaker: n(row.weaker),
          gone: n(row.gone),
          new: n(row.new),
        })}
      </p>
      <p>
        {say("rulebooks.maps.moved", {
          arrived: n(row.arrived),
          left: n(row.left),
        })}
      </p>
    </li>
  );
}

function Maps({ view }: Readonly<{ view: RulebooksView }>): ReactElement {
  return (
    <section data-part="rulebooks-maps" aria-labelledby="rulebooks-maps">
      <h2 id="rulebooks-maps">{say("rulebooks.maps")}</h2>
      {view.ruleMaps.length === 0 ? (
        <p>{say("rulebooks.maps.none")}</p>
      ) : (
        <ul className="gr-rulebooks-list">
          {view.ruleMaps.map((row) => (
            <RuleMap key={row.rulebook} row={row} />
          ))}
        </ul>
      )}
    </section>
  );
}

function Governance({ view }: Readonly<{ view: RulebooksView }>): ReactElement {
  const { governance } = view;
  return (
    <section
      data-part="rulebooks-governance"
      aria-labelledby="rulebooks-governance"
    >
      <h2 id="rulebooks-governance">{say("rulebooks.governance")}</h2>
      <dl className="gr-rulebooks-fields">
        <dt>{say("rulebooks.kind")}</dt>
        <dd>{view.kind}</dd>
        <dt>{say("rulebooks.governance.contribution")}</dt>
        <dd>{governance.contribution}</dd>
        <dt>{say("rulebooks.governance.decision")}</dt>
        <dd>
          {say(
            governance.decisionRequired
              ? "rulebooks.governance.decision.yes"
              : "rulebooks.governance.decision.no",
          )}
        </dd>
        <dt>{say("rulebooks.governance.trial")}</dt>
        <dd>{format.number(governance.trialReleases, 0)}</dd>
        <dt>{say("rulebooks.governance.review")}</dt>
        <dd>{format.number(governance.reviewDays, 0)}</dd>
      </dl>
    </section>
  );
}

function Licences({ view }: Readonly<{ view: RulebooksView }>): ReactElement {
  return (
    <section
      data-part="rulebooks-licences"
      aria-labelledby="rulebooks-licences"
    >
      <h2 id="rulebooks-licences">{say("rulebooks.licences")}</h2>
      <dl className="gr-rulebooks-fields">
        {view.licences.map((row) => (
          <div key={row.kind} className="gr-rulebooks-pair">
            <dt>{say(`rulebooks.licence.${row.kind}`)}</dt>
            <dd>
              {row.licence.kind === "dash" ? (
                <Dash reason={say(row.licence.reason)} />
              ) : (
                row.licence.value
              )}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

function Archived({
  view,
}: Readonly<{ view: RulebooksView }>): ReactElement | null {
  const { archived } = view;
  if (archived === null) return null;
  return (
    <section
      data-part="rulebooks-archived"
      aria-labelledby="rulebooks-archived"
    >
      <h2 id="rulebooks-archived">{say("rulebooks.archived")}</h2>
      <dl className="gr-rulebooks-fields">
        <dt>{say("rulebooks.archived.successor")}</dt>
        <dd>
          {archived.successor.kind === "dash" ? (
            <Dash reason={say(archived.successor.reason)} />
          ) : (
            archived.successor.value
          )}
        </dd>
      </dl>
    </section>
  );
}

function Content({ bundle }: Readonly<{ bundle: Bundle }>): ReactElement {
  const view = rulebooksView(bundle);
  return (
    <>
      <Chain chain={view.chain} />
      <Imports view={view} />
      <Maps view={view} />
      <Governance view={view} />
      <Licences view={view} />
      <Archived view={view} />
    </>
  );
}

export type RulebooksRegionProps = Readonly<{
  region: DataState<Bundle>;
  onRetry: () => void;
}>;

export function RulebooksRegion({
  region,
  onRetry,
}: RulebooksRegionProps): ReactElement {
  return (
    <section data-view="rulebooks" data-state={region.state}>
      <h1>{say("shell.view.rulebooks")}</h1>
      <p className="gr-rulebooks-question">{say("rulebooks.question")}</p>
      {region.state === "loading" ? (
        <p role="status">{say("shell.loading")}</p>
      ) : null}
      {region.state === "empty" ? (
        <EmptyFrame
          title={
            region.why === "no-rows"
              ? say("rulebooks.no-rulebook")
              : missingFrame(region.file)
          }
        >
          {region.why === "no-rows" ? (
            <>
              <p>{noRowsFrame(region.file)}</p>
              <p>{say("rulebooks.handbook")}</p>
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
        <Content bundle={region.value} />
      ) : null}
    </section>
  );
}

/** The page: reads the one cached bundle the shell loaded and hands it down. */
export function RulebooksPage(): ReactElement {
  const client = useQueryClient();
  const bundle = useQuery<Loaded<Bundle>>({
    queryKey: keys.bundle(),
    queryFn: skipToken,
    staleTime: Infinity,
  });
  return (
    <RulebooksRegion
      region={bundleRegion(bundle, bundle.dataUpdatedAt)}
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}
