// The terms view: every term with its literal, its definition in
// the chosen language, the rules that use it (each a link to its rule view) and
// its translations, a stale one marked as a notice. The language is chosen in
// the URL (`#/terms?lang=fr`); a term with no translation in it shows the
// authoritative text with the fallback mark. `TermsRegion` is plain props: it
// imports no data layer, so the element and the iframe mount it with a bundle
// in hand (react.components.1). `TermsPage` is the one orchestrator that asks
// the cache. Every word is the message module's.
import { Language, type BundleDocument } from "@gragas/contract";
import { skipToken, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, type ReactElement } from "react";
import * as z from "zod";
import {
  destinationPath,
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
} from "../../parts/index.ts";
import { bundleRegion } from "../../selectors/index.ts";
import { termsView, type TermRow, type TermsView } from "./selectors.ts";
import "../../parts/base.css";
import "./terms.css";

type Bundle = z.output<typeof BundleDocument>;

export const termsSearchSchema = z.strictObject({
  lang: Language.optional(),
  /** The term a rule view linked to: scrolled to and marked (gragas#2396). */
  term: z.string().min(1).optional(),
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

const termId = (slug: string): string => `term-${slug}`;

function Switch({ view }: Readonly<{ view: TermsView }>): ReactElement {
  return (
    <nav data-part="terms-switch" aria-label={say("terms.switch")}>
      <ul className="gr-terms-switch">
        {view.languages.map((language) => (
          <li key={language}>
            <Link
              href={
                language === view.authoritative
                  ? "#/terms"
                  : `#/terms?lang=${encodeURIComponent(language)}`
              }
            >
              <span
                aria-current={language === view.chosen ? "true" : undefined}
              >
                {language === view.authoritative
                  ? say("terms.switch.authoritative", { language })
                  : language}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function Definition({
  row,
  view,
}: Readonly<{ row: TermRow; view: TermsView }>): ReactElement {
  const { definition } = row;
  return (
    <div data-part="terms-definition" data-mark={definition.mark}>
      <p
        lang={definition.mark === "fallback" ? view.authoritative : view.chosen}
      >
        {definition.text}
      </p>
      {definition.mark === "stale" ? (
        <p className="gr-terms-mark" data-part="terms-mark" role="note">
          <span aria-hidden="true">≠ </span>
          {say("terms.mark.stale")}
        </p>
      ) : null}
      {definition.mark === "fallback" ? (
        <p className="gr-terms-mark" data-part="terms-mark" role="note">
          <span aria-hidden="true">↩ </span>
          {say("terms.mark.fallback", { language: view.chosen })}
        </p>
      ) : null}
    </div>
  );
}

function TermItem({
  row,
  view,
  focused,
}: Readonly<{
  row: TermRow;
  view: TermsView;
  focused: boolean;
}>): ReactElement {
  useEffect(() => {
    if (focused) {
      document
        .getElementById(termId(row.slug))
        ?.scrollIntoView({ block: "start" });
    }
  }, [focused, row.slug]);
  return (
    <li
      data-part="terms-term"
      id={termId(row.slug)}
      data-focused={focused ? "true" : undefined}
    >
      <h2>{row.slug}</h2>
      {focused ? (
        <p className="gr-terms-mark" data-part="terms-mark" role="note">
          {say("terms.focused")}
        </p>
      ) : null}
      <dl className="gr-terms-fields">
        <dt>{say("terms.literal")}</dt>
        <dd>
          {row.literal.kind === "dash" ? (
            <Dash reason={say(row.literal.reason)} />
          ) : (
            <code>{row.literal.value}</code>
          )}
        </dd>
        <dt>{say("terms.definition.in", { language: view.chosen })}</dt>
        <dd>
          <Definition row={row} view={view} />
        </dd>
        <dt>{say("terms.translations")}</dt>
        <dd>
          {row.translations.length === 0 ? (
            <p>{say("terms.translations.none")}</p>
          ) : (
            <ul className="gr-terms-translations">
              {row.translations.map((t) => (
                <li key={t.language} lang={t.language}>
                  {say("terms.translation.line", {
                    language: t.language,
                    text: t.text,
                  })}
                  {t.stale ? (
                    <span className="gr-terms-mark" data-part="terms-mark">
                      {" "}
                      <span aria-hidden="true">≠ </span>
                      {say("terms.mark.translation-stale")}
                    </span>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </dd>
        <dt>{say("terms.used-by")}</dt>
        <dd>
          {row.usedBy.length === 0 ? (
            <p>{say("terms.used-by.none")}</p>
          ) : (
            <ul className="gr-terms-used">
              {row.usedBy.map((id) => (
                <li key={id}>
                  <Link href={`#${destinationPath({ route: "rule", id })}`}>
                    {id}
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </dd>
      </dl>
    </li>
  );
}

function Content({
  bundle,
  language,
  focus,
}: Readonly<{
  bundle: Bundle;
  language: string | undefined;
  focus: string | undefined;
}>): ReactElement {
  const view = termsView(bundle, language);
  if (view.rows.length === 0) {
    return (
      <EmptyFrame title={say("terms.no-terms")}>
        <p>{say("terms.handbook")}</p>
      </EmptyFrame>
    );
  }
  return (
    <>
      <Switch view={view} />
      <p data-part="terms-counts">
        {say("terms.count", { count: view.rows.length })}
        {". "}
        {say("terms.translations.count", {
          count: view.translationCount,
          stale: format.number(view.staleCount, 0),
        })}
      </p>
      <ul className="gr-terms-list" aria-label={say("shell.view.terms")}>
        {view.rows.map((row) => (
          <TermItem
            key={row.slug}
            row={row}
            view={view}
            focused={row.slug === focus}
          />
        ))}
      </ul>
    </>
  );
}

export type TermsRegionProps = Readonly<{
  region: DataState<Bundle>;
  /** The language chosen in the URL; the authoritative one when absent. */
  language?: string | undefined;
  /** The term a rule linked to; scrolled to and marked. */
  focus?: string | undefined;
  onRetry: () => void;
}>;

export function TermsRegion({
  region,
  language,
  focus,
  onRetry,
}: TermsRegionProps): ReactElement {
  return (
    <section data-view="terms" data-state={region.state}>
      <h1>{say("shell.view.terms")}</h1>
      <p className="gr-terms-question">{say("terms.question")}</p>
      {region.state === "loading" ? (
        <p role="status">{say("shell.loading")}</p>
      ) : null}
      {region.state === "empty" ? (
        <EmptyFrame
          title={
            region.why === "no-rows"
              ? say("terms.no-terms")
              : missingFrame(region.file)
          }
        >
          {region.why === "no-rows" ? (
            <>
              <p>{noRowsFrame(region.file)}</p>
              <p>{say("terms.handbook")}</p>
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
        <Content bundle={region.value} language={language} focus={focus} />
      ) : null}
    </section>
  );
}

/** The page: reads the one cached bundle the shell loaded and hands it down. */
export function TermsPage({
  search,
}: Readonly<{ search: z.output<typeof termsSearchSchema> }>): ReactElement {
  const client = useQueryClient();
  const bundle = useQuery<Loaded<Bundle>>({
    queryKey: keys.bundle(),
    queryFn: skipToken,
    staleTime: Infinity,
  });
  return (
    <TermsRegion
      region={bundleRegion(bundle, bundle.dataUpdatedAt)}
      language={search.lang}
      focus={search.term}
      onRetry={() => {
        void retryLoad(client, "bundle.json");
      }}
    />
  );
}
