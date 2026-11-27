// The chrome around every view: the skip control, the navigation of the views,
// the marks and banners the bundle asks for, and the footer with the
// versions. It holds no view logic: what a view draws is its manifest's page.
import { CONTRACT_VERSION, CURRENT_MAJOR } from "@gragas/contract";
import {
  bundleQuery,
  bundleRegion,
  missingFrame,
  notReadyFrame,
  previousMajor,
  retryLoad,
  viewMarks,
  type DataSource,
  type LoadFailure,
} from "@gragas/core";
import { Banner, ErrorFrame } from "@gragas/core/parts";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useRouter } from "@tanstack/react-router";
import { useEffect, useRef, type ReactElement } from "react";
import { say } from "./say.ts";
import { viewTitles } from "./views.ts";
import "@gragas/tokens/fonts.css";
import "@gragas/tokens/tokens.css";
import "./shell.css";

function describe(cause: LoadFailure): string {
  if (cause.kind === "missing") return missingFrame(cause.file);
  if (cause.kind === "not-ready") {
    return notReadyFrame(
      cause.file,
      cause.documentVersion,
      cause.readerVersion,
    );
  }
  return say("shell.data.failed", { file: cause.file });
}

export type ShellProps = Readonly<{ source: DataSource }>;

export function Shell({ source }: ShellProps): ReactElement {
  const router = useRouter();
  const client = useQueryClient();
  const main = useRef<HTMLElement>(null);
  const bundle = useQuery(bundleQuery(source));
  const region = bundleRegion(bundle, bundle.dataUpdatedAt);
  const value = bundle.data?.value ?? null;

  // On every route change the title follows the page heading and focus moves
  // to it (react.routing.9); the first render sets the title and keeps focus.
  useEffect(
    () =>
      router.subscribe("onRendered", ({ hrefChanged }) => {
        const heading = main.current?.querySelector<HTMLElement>("h1") ?? null;
        const view = heading?.textContent ?? "";
        document.title =
          view === "" ? say("shell.name") : say("shell.title", { view });
        if (hrefChanged && heading !== null) {
          heading.tabIndex = -1;
          heading.focus();
        }
      }),
    [router],
  );

  const marks = value === null ? null : viewMarks(value);
  const older =
    value === null
      ? null
      : previousMajor(value, CURRENT_MAJOR, CONTRACT_VERSION);

  return (
    <div data-shell="">
      <button
        type="button"
        className="gr-skip"
        onClick={() => {
          main.current?.focus();
        }}
      >
        {say("shell.skip")}
      </button>
      <header>
        <p className="gr-name">{say("shell.name")}</p>
        <nav aria-label={say("shell.nav")}>
          <ul>
            <li>
              <Link to="/rules" activeProps={{ "aria-current": "page" }}>
                {say(viewTitles.rules)}
              </Link>
            </li>
            <li>
              <Link to="/rulebooks" activeProps={{ "aria-current": "page" }}>
                {say(viewTitles.rulebooks)}
              </Link>
            </li>
            <li>
              <Link to="/tools" activeProps={{ "aria-current": "page" }}>
                {say(viewTitles.tools)}
              </Link>
            </li>
            <li>
              <Link to="/coverage" activeProps={{ "aria-current": "page" }}>
                {say(viewTitles.coverage)}
              </Link>
            </li>
            <li>
              <Link to="/standing" activeProps={{ "aria-current": "page" }}>
                {say(viewTitles.standing)}
              </Link>
            </li>
            <li>
              <Link to="/health" activeProps={{ "aria-current": "page" }}>
                {say(viewTitles.health)}
              </Link>
            </li>
            <li>
              <Link to="/changelog" activeProps={{ "aria-current": "page" }}>
                {say(viewTitles.changelog)}
              </Link>
            </li>
            <li>
              <Link to="/terms" activeProps={{ "aria-current": "page" }}>
                {say(viewTitles.terms)}
              </Link>
            </li>
            <li>
              <Link to="/attention" activeProps={{ "aria-current": "page" }}>
                {say(viewTitles.attention)}
              </Link>
            </li>
          </ul>
        </nav>
      </header>
      {marks?.unreleased === true ? (
        <Banner tone="warning">{say("shell.unreleased")}</Banner>
      ) : null}
      {older === null ? null : (
        <Banner tone="info">
          {say("shell.previous-major", {
            contract: older.contract,
            reader: older.reader,
          })}
        </Banner>
      )}
      <main id="main" ref={main} tabIndex={-1}>
        {value !== null ? (
          <Outlet />
        ) : (
          <section data-view="data">
            <h1>{say("shell.name")}</h1>
            {region.state === "loading" ? (
              <p role="status">{say("shell.loading")}</p>
            ) : region.state === "error" ? (
              <ErrorFrame
                message={describe(region.cause)}
                onRetry={() => {
                  void retryLoad(client, "bundle.json");
                }}
              />
            ) : (
              <ErrorFrame
                message={missingFrame("bundle.json")}
                onRetry={() => {
                  void retryLoad(client, "bundle.json");
                }}
              />
            )}
          </section>
        )}
      </main>
      <footer>
        {value === null
          ? null
          : say("shell.footer", {
              rulebook: value.release.version,
              engine: value.versions.engine,
            })}
      </footer>
    </div>
  );
}
