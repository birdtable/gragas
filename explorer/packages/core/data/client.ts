// The one query client (react.server-state.1): the single cache of
// the two documents, keys from keys.ts and freshness from policy.ts. The root
// route's loader is `loadRoot`, a plain function that asks the client; no
// component fetches, and no effect does.
import {
  QueryClient,
  type QueryExecuteOptions,
  type QueryKey,
} from "@tanstack/react-query";
import { keys } from "./keys.ts";
import {
  loadBundle,
  loadProofGrid,
  loadReport,
  type BundleValue,
  type DataSource,
  type DocumentFile,
  type Loaded,
  type ProofGridValue,
  type ReportValue,
} from "./load.ts";
import { freshness } from "./policy.ts";

export type DocumentQuery<T> = QueryExecuteOptions<Loaded<T>>;

export function createExplorerClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: freshness } });
}

export function bundleQuery(source: DataSource): DocumentQuery<BundleValue> {
  return {
    queryKey: keys.bundle(),
    queryFn: () => loadBundle(source),
    ...freshness,
  };
}

export function reportQuery(source: DataSource): DocumentQuery<ReportValue> {
  return {
    queryKey: keys.report(),
    queryFn: () => loadReport(source),
    ...freshness,
  };
}

/** Never part of the root load: a view that wants the grid asks for it. */
export function proofQuery(source: DataSource): DocumentQuery<ProofGridValue> {
  return {
    queryKey: keys.proof(),
    queryFn: () => loadProofGrid(source),
    ...freshness,
  };
}

// A load that failed is read from the cache, so the loader itself never throws.
async function settled(load: Promise<unknown>): Promise<void> {
  await load.then(
    () => undefined,
    () => undefined,
  );
}

/**
 * The root route's loader: starts the bundle and, in the standalone
 * packaging, the report, and never throws; a failure is read from the cache
 * by the data region.
 */
export async function loadRoot(
  client: QueryClient,
  source: DataSource,
  standalone: boolean,
): Promise<void> {
  await Promise.all([
    settled(client.query(bundleQuery(source))),
    standalone ? settled(client.query(reportQuery(source))) : undefined,
  ]);
}

const keyOf: Record<DocumentFile, () => QueryKey> = {
  "bundle.json": keys.bundle,
  "report.json": keys.report,
  "proof.json": keys.proof,
};

/** Retry, offered on every failed load: asks again whatever the freshness says. */
export async function retryLoad(
  client: QueryClient,
  file: DocumentFile,
): Promise<void> {
  await client.refetchQueries({ queryKey: keyOf[file](), exact: true });
}
