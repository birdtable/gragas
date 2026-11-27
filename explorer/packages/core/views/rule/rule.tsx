// The rule view's page (gragas#2396, on the selectors of gragas#294 and the
// contract fields of gragas#1425): one rule's statement and standing, its
// anatomy, the bindings that hold it, the decisions and sources behind it, its
// history, and each term it uses as a link to that term's entry in the terms
// view (`#/terms?term=<slug>`, gragas#1685 line 1). `RuleRegion` is plain
// props and imports no data layer, so an embed mounts it with a bundle in
// hand (react.components.1); `RulePage` is the one orchestrator that asks the
// cache. A value the bundle does not state is a dash with its reason
// (web.states.5); every word is the message module's.
import type { BundleDocument } from "@gragas/contract";
import { skipToken, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactElement } from "react";
import * as z from "zod";
import {
  destinationPath,
  keys,
  retryLoad,
  type DataState,
  type Destination,
  type Loaded,
} from "../../data/index.ts";
import {
  createSay,
  missingFrame,
  noRowsFrame,
  staleBanner,
  type Dashable,
} from "../../messages/index.ts";
import {
  Banner,
  Dash,
  EmptyFrame,
  ErrorFrame,
  Link,
} from "../../parts/index.ts";
import { bundleRegion } from "../../selectors/index.ts";
import { termsUsedBy } from "../terms/selectors.ts";
import { ruleView, type RuleView } from "./selectors.ts";
import "../../parts/base.css";
import "./rule.css";

type Bundle = z.output<typeof BundleDocument>;

/** The rule's address names it; nothing is searched or filtered on this page. */
export const ruleSearchSchema = z.strictObject({});

const say = createSay("en");

export const states = [
  "default",
  "loading",
  "empty",
  "error",
  "phone390",
  "stale",
] as const;

const href = (destination: Destination): string =>
  `#${destinationPath(destination)}`;

/** The hash of a term's entry in the terms view: the one link of line 1 of gragas#1685. */
export const termHref = (slug: string): string =>
  `#/terms?term=${encodeURIComponent(slug)}`;

const proofWord = {
  pass: "proof.pass",
  fail: "proof.fail",
  "not run": "proof.not-run",
  reviewed: "proof.reviewed",
} as const;

function Cell({
  value,
  children,
}: Readonly<{
  value: Dashable<string>;
  children?: (text: string) => ReactElement;
}>): ReactElement {
  if (value.kind === "dash") return <Dash reason={say(value.reason)} />;
  return children === undefined ? <>{value.value}</> : children(value.value);
}

function Panel({
  name,
  title,
  children,
}: Readonly<{
  name: string;
  title: string;
  children: ReactElement | readonly ReactElement[];
}>): ReactElement {
  return (
    <section data-part={`rule-${name}`} aria-label={title}>
      <h2>{title}</h2>
      {children}
    </section>
  );
}

function Anatomy({ view }: Readonly<{ view: RuleView }>): ReactElement {
  return (
    <Panel name="anatomy" title={say("rule.anatomy")}>
      <p data-part="rule-statement">{view.anatomy.definition}</p>
      <dl className="gr-rule-fields">
        <dt>{say("rule.reasoning")}</dt>
        <dd>
          <Cell value={view.anatomy.reasoning} />
        </dd>
        {view.aliases.length === 0 ? null : (
          <>
            <dt>{say("rule.aliases")}</dt>
            <dd>{view.aliases.join(", ")}</dd>
          </>
        )}
        {view.pairs.length === 0 ? null : (
          <>
            <dt>{say("rule.pairs")}</dt>
            <dd>
              {view.pairs
                .map((p) =>
                  say("rule.pairs.line", {
                    number: p.number,
                    target: p.targetType,
                  }),
                )
                .join("; ")}
            </dd>
          </>
        )}
      </dl>
    </Panel>
  );
}

function Bindings({ view }: Readonly<{ view: RuleView }>): ReactElement {
  return (
    <Panel name="bindings" title={say("rule.bindings")}>
      {view.bindings.length === 0 ? (
        <p>{say("rule.bindings.none")}</p>
      ) : (
        <ul className="gr-rule-list">
          {view.bindings.map((b) => (
            <li key={b.id} data-part="rule-binding">
              <Link href={href(b.destination)}>{b.entry}</Link>{" "}
              {say("rule.binding.line", { rung: b.rung, point: b.runPoint })}
              <dl className="gr-rule-fields">
                <dt>{say("rule.binding.remedy")}</dt>
                <dd>
                  <Cell value={b.remedy} />
                </dd>
                <dt>{say("rule.binding.proof")}</dt>
                <dd>
                  {b.proof.kind === "dash" ? (
                    <Dash reason={say(b.proof.reason)} />
                  ) : (
                    b.proof.value.map((c) => (
                      <span key={`${c.level}-${c.toolVersion}`}>
                        {say("rule.binding.proof.line", {
                          result: say(proofWord[c.result]),
                          level: c.level,
                          version: c.toolVersion,
                        })}
                      </span>
                    ))
                  )}
                </dd>
              </dl>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

function Decisions({ view }: Readonly<{ view: RuleView }>): ReactElement {
  return (
    <Panel name="decisions" title={say("rule.decisions")}>
      {view.decisions.length === 0 ? (
        <p>{say("rule.decisions.none")}</p>
      ) : (
        <ul className="gr-rule-list">
          {view.decisions.map((d) => (
            <li key={d.slug} data-part="rule-decision">
              <Link href={href(d.destination)}>{d.slug}</Link>{" "}
              {say("rule.decision.line", {
                title: d.title,
                who: d.who,
                date: d.date,
              })}
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

function Sources({ view }: Readonly<{ view: RuleView }>): ReactElement {
  return (
    <Panel name="sources" title={say("rule.sources")}>
      {view.sources.length === 0 ? (
        <p>{say("rule.sources.none")}</p>
      ) : (
        <ul className="gr-rule-list">
          {view.sources.map((s) => (
            <li key={s.slug} data-part="rule-source">
              <Link href={href(s.destination)}>{s.slug}</Link>{" "}
              <Cell value={s.title} /> (<Cell value={s.kind} />,{" "}
              <Cell value={s.locator} />)
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

function History({ view }: Readonly<{ view: RuleView }>): ReactElement {
  return (
    <Panel name="history" title={say("rule.history")}>
      {view.history.length === 0 ? (
        <p>{say("rule.history.none")}</p>
      ) : (
        <ol className="gr-rule-list">
          {view.history.map((h) => (
            <li key={h.number} data-part="rule-step">
              {say("rule.history.line", {
                number: h.number,
                kind: h.kind,
                when: h.when,
                who: h.who,
              })}
            </li>
          ))}
        </ol>
      )}
    </Panel>
  );
}

function Terms({
  slugs,
}: Readonly<{ slugs: readonly string[] }>): ReactElement {
  return (
    <Panel name="terms" title={say("rule.terms")}>
      {slugs.length === 0 ? (
        <p>{say("rule.terms.none")}</p>
      ) : (
        <ul className="gr-rule-terms">
          {slugs.map((slug) => (
            <li key={slug} data-part="rule-term">
              <Link href={termHref(slug)}>{slug}</Link>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

function Relations({ view }: Readonly<{ view: RuleView }>): ReactElement {
  return (
    <Panel name="relations" title={say("rule.relations")}>
      {view.relations.length === 0 ? (
        <p>{say("rule.relations.none")}</p>
      ) : (
        <ul className="gr-rule-list">
          {view.relations.map((r) => (
            <li key={`${r.kind}-${r.rule}`}>
              {say("rule.relations.line", { kind: r.kind })}{" "}
              <Link href={href(r.destination)}>{r.rule}</Link>
            </li>
          ))}
        </ul>
      )}
      <p>
        <Link href={href(view.relationsLink)}>
          {say("rule.relations-view")}
        </Link>
      </p>
    </Panel>
  );
}

function Content({
  bundle,
  id,
}: Readonly<{ bundle: Bundle; id: string }>): ReactElement {
  const result = ruleView(bundle, id);
  if (result.kind !== "found") {
    return (
      <EmptyFrame title={say("rule.missing", { id })}>
        <p>{say("rule.question")}</p>
      </EmptyFrame>
    );
  }
  const { view } = result;
  return (
    <article data-part="rule" data-status={view.status}>
      <header>
        <h2 data-part="rule-id">{view.id}</h2>
        <p data-part="rule-standing">
          {view.strength}, {view.status}, {view.standing}
        </p>
        {view.retired === null ? null : (
          <p data-part="rule-retired" role="note">
            {say("rule.retired")} {say("rule.retired.replaced")}{" "}
            {view.retired.replacement.kind === "dash" ? (
              <Dash reason={say(view.retired.replacement.reason)} />
            ) : (
              <Link href={href(view.retired.replacement.value)}>
                {view.retired.replacement.value.id}
              </Link>
            )}
          </p>
        )}
      </header>
      <Anatomy view={view} />
      <Terms slugs={termsUsedBy(bundle, view.id)} />
      <Bindings view={view} />
      <Decisions view={view} />
      <Sources view={view} />
      <History view={view} />
      <Relations view={view} />
    </article>
  );
}

export type RuleRegionProps = Readonly<{
  region: DataState<Bundle>;
  /** The rule's id, from the address. */
  id: string;
  onRetry: () => void;
}>;

export function RuleRegion({
  region,
  id,
  onRetry,
}: RuleRegionProps): ReactElement {
  return (
    <section data-view="rule" data-state={region.state}>
      <h1>{say("shell.view.rule")}</h1>
      <p className="gr-rule-question">{say("rule.question")}</p>
      {region.state === "loading" ? (
        <p role="status">{say("shell.loading")}</p>
      ) : null}
      {region.state === "empty" ? (
        <EmptyFrame
          title={
            region.why === "no-rows"
              ? say("rule.missing", { id })
              : missingFrame(region.file)
          }
        >
          {region.why === "no-rows" ? <p>{noRowsFrame(region.file)}</p> : null}
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
        <Content bundle={region.value} id={id} />
      ) : null}
    </section>
  );
}

/** The page: reads the one cached bundle the shell loaded and the rule the address names. */
export function RulePage({
  params,
}: Readonly<{
  search: z.output<typeof ruleSearchSchema>;
  params: Readonly<Record<string, string>>;
}>): ReactElement {
  const client = useQueryClient();
  const bundle = useQuery<Loaded<Bundle>>({
    queryKey: keys.bundle(),
    queryFn: skipToken,
    staleTime: Infinity,
  });
  return (
    <RuleRegion
      region={bundleRegion(bundle, bundle.dataUpdatedAt)}
      id={params["id"] ?? ""}
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}
