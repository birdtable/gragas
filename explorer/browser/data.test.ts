// The data module's cases in real engines:
// the real script-block reader over a real document, with no DOM shim and no
// stub of fetch. MSW's browser worker answers the hosted requests and counts
// them, and the request counter is the assertion for "zero" and "one". Each case
// is a function a plant below runs against a fault, and each must fail there.
import { http, HttpResponse } from "msw";
import { setupWorker } from "msw/browser";
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
} from "vitest";
import {
  bundleRegion,
  createExplorerClient,
  documentBlocks,
  keys,
  loadProofGrid,
  loadRoot,
  missingFrame,
  notReadyFrame,
  retryLoad,
  type BundleValue,
  type DataSource,
  type DataState,
  type InlineBlocks,
  type Loaded,
} from "../packages/core/index.ts";
import type { QueryClient } from "@tanstack/react-query";

type Doc = Record<string, unknown>;
const files = import.meta.glob(
  "../../internal/host/assets/contract/fixture/*.json",
  { eager: true, import: "default" },
);
function fixture(name: string): Doc {
  const doc = files[`../../internal/host/assets/contract/fixture/${name}.json`];
  if (typeof doc !== "object" || doc === null) throw new Error(`no ${name}`);
  return { ...doc };
}

const base = `${location.origin}/hosted/`;
const bundle = fixture("bundle");
const worker = setupWorker();
const requests: string[] = [];
worker.events.on("request:start", ({ request }) => {
  const at = new URL(request.url);
  if (at.pathname.startsWith("/hosted/")) requests.push(at.pathname);
});

function serve(doc: Doc | number): void {
  worker.use(
    http.get(`${base}bundle.json`, () =>
      typeof doc === "number"
        ? new HttpResponse(null, { status: doc })
        : HttpResponse.json(doc),
    ),
  );
}

beforeAll(async () => {
  await worker.start({
    onUnhandledRequest: "error",
    quiet: true,
    serviceWorker: { url: "/mockServiceWorker.js" },
  });
});
afterAll(() => worker.stop());
beforeEach(() => {
  requests.length = 0;
  serve(bundle);
});
afterEach(() => {
  worker.resetHandlers();
  for (const block of document.querySelectorAll("script[data-case]")) {
    block.remove();
  }
});

/** Writes a block the way the standalone packaging does, into the live page. */
function writeBlock(file: string, text: string): void {
  const block = document.createElement("script");
  block.type = "application/json";
  block.dataset["file"] = file;
  block.dataset["case"] = "";
  block.textContent = text;
  document.head.append(block);
}

const reader = documentBlocks(document);
const page: DataSource = { base, inline: reader };

function stateOf(client: QueryClient, now = 0): DataState<BundleValue> {
  const held = client.getQueryState<Loaded<BundleValue>>(keys.bundle());
  return bundleRegion(
    {
      status: held?.status ?? "pending",
      data: held?.data,
      error: held?.error ?? null,
      dataUpdatedAt: held?.dataUpdatedAt ?? 0,
    },
    now,
  );
}

/**
 * One round trip through the worker: requests pass it in order, so a request
 * an earlier call left unawaited has been counted when this returns.
 */
async function settled(): Promise<void> {
  worker.use(http.get(`${base}settle.json`, () => HttpResponse.json({})));
  await fetch(`${base}settle.json`);
  requests.splice(requests.indexOf("/hosted/settle.json"), 1);
}

// The cases ---------------------------------------------------------------

async function zeroRequestsWithBlock(inline: InlineBlocks): Promise<void> {
  writeBlock("bundle.json", JSON.stringify(bundle));
  const client = createExplorerClient();
  await loadRoot(client, { base, inline }, false);
  expect(stateOf(client).state).toBe("ready");
  await settled();
  expect(requests).toEqual([]);
}

async function staleKeepsData(
  refresh: (client: QueryClient) => Promise<void>,
): Promise<void> {
  const client = createExplorerClient();
  await loadRoot(client, page, false);
  const good = stateOf(client);
  if (good.state !== "ready") throw new Error("the first load is ready");
  serve(503);
  await refresh(client);
  const held = client.getQueryState(keys.bundle());
  const state = stateOf(client, (held?.dataUpdatedAt ?? 0) + 90_000);
  expect(state).toMatchObject({
    state: "stale",
    retry: true,
    ageMs: 90_000,
    cause: { kind: "http", status: 503 },
  });
  expect("value" in state ? state.value : null).toEqual(good.value);
}

async function readyHosted(status: number): Promise<void> {
  serve(status === 200 ? bundle : status);
  const client = createExplorerClient();
  await loadRoot(client, page, false);
  expect(stateOf(client).state).toBe("ready");
  expect(requests).toEqual(["/hosted/bundle.json"]);
}

async function oneFetchWithoutBlock(inline: InlineBlocks): Promise<void> {
  const client = createExplorerClient();
  await loadRoot(client, { base, inline }, false);
  await settled();
  expect(requests).toHaveLength(1);
}

async function proofGridNotAtLoad(inline: InlineBlocks): Promise<void> {
  worker.use(http.get(`${base}proof.json`, () => HttpResponse.json([])));
  await loadRoot(createExplorerClient(), { base, inline }, true);
  writeBlock("bundle.json", JSON.stringify(bundle));
  await loadRoot(createExplorerClient(), { base, inline }, false);
  await settled();
  expect(requests).not.toContain("/hosted/proof.json");
}

async function missingIsEmptyFrame(status: number): Promise<void> {
  serve(status);
  const client = createExplorerClient();
  await loadRoot(client, page, false);
  const state = stateOf(client);
  expect(state).toEqual({
    state: "empty",
    file: "bundle.json",
    why: "missing",
    retry: true,
  });
  expect(missingFrame(state.file)).toContain("bundle.json");
}

async function notReadyForMajor(version: string): Promise<void> {
  serve({ ...bundle, contract: version });
  const client = createExplorerClient();
  await loadRoot(client, page, false);
  expect(stateOf(client)).toMatchObject({
    state: "error",
    retry: true,
    cause: {
      kind: "not-ready",
      documentVersion: version,
      readerVersion: "1.3",
    },
  });
  const frame = notReadyFrame("bundle.json", version, "1.3");
  expect(frame).toContain(version);
  expect(frame).toContain("1.3");
}

// The named cases ---------------------------------------------------------

describe("ready", () => {
  test("a hosted copy is fetched once and is ready", async () => {
    await expect(readyHosted(200)).resolves.toBeUndefined();
  });
});

describe("the inline script blocks, read from a real document", () => {
  test("with the bundle in a block: zero network requests", async () => {
    await expect(zeroRequestsWithBlock(reader)).resolves.toBeUndefined();
  });

  test("without the block: exactly one fetch", async () => {
    await expect(oneFetchWithoutBlock(reader)).resolves.toBeUndefined();
  });

  test("the reader finds a block by file and leaves other scripts alone", () => {
    writeBlock("report.json", "{}");
    const other = document.createElement("script");
    other.type = "text/plain";
    other.dataset["file"] = "bundle.json";
    other.dataset["case"] = "";
    other.textContent = "not a data block";
    document.head.append(other);
    const byId = document.createElement("script");
    byId.type = "application/json";
    byId.id = "proof.json";
    byId.dataset["case"] = "";
    byId.textContent = "{}";
    document.head.append(byId);
    expect(reader.read("report.json")).toBe("{}");
    expect(reader.read("bundle.json")).toBeUndefined();
    expect(reader.read("proof.json")).toBeUndefined();
  });

  test("a block that is not JSON is an error and is not fetched around", async () => {
    writeBlock("bundle.json", "{not json");
    const client = createExplorerClient();
    await loadRoot(client, page, false);
    expect(stateOf(client)).toMatchObject({
      state: "error",
      cause: { kind: "invalid" },
    });
    expect(requests).toEqual([]);
  });
});

describe("the proof grid", () => {
  test("is not fetched at load, hosted or inline", async () => {
    await expect(proofGridNotAtLoad(reader)).resolves.toBeUndefined();
  });

  test("is fetched when a view asks for it", async () => {
    worker.use(
      http.get(`${base}proof.json`, () =>
        HttpResponse.json(bundle["proof_grid"]),
      ),
    );
    const loaded = await loadProofGrid(page);
    expect(loaded.value.length).toBeGreaterThan(0);
    expect(requests).toEqual(["/hosted/proof.json"]);
  });
});

describe("a missing bundle", () => {
  test("is an empty frame naming bundle.json, with Retry", async () => {
    await expect(missingIsEmptyFrame(404)).resolves.toBeUndefined();
  });
});

describe("the contract's majors", () => {
  test.each([
    ["a previous major", "0.9"],
    ["a newer major", "2.0"],
  ])("%s is the not-ready frame naming both versions", async (_, version) => {
    await expect(notReadyForMajor(version)).resolves.toBeUndefined();
  });
});

describe("a failed refresh", () => {
  test("keeps the last good data, stale with its age, and offers Retry", async () => {
    await staleKeepsData((client) => retryLoad(client, "bundle.json"));
    serve(bundle);
    const client = createExplorerClient();
    await loadRoot(client, page, false);
    serve(503);
    await retryLoad(client, "bundle.json");
    serve(bundle);
    await retryLoad(client, "bundle.json");
    expect(stateOf(client).state).toBe("ready");
  });
});

// The plants (gates.plants.1): each case above fails on its fault ----------

describe("plants: the cases fail on a fault", () => {
  test("a reader that fetches when a block is present fails the zero-request case", async () => {
    const fetching: InlineBlocks = {
      read(file) {
        void fetch(`${base}${file}`);
        return reader.read(file);
      },
    };
    await expect(zeroRequestsWithBlock(fetching)).rejects.toThrow();
  });

  test("a hosted copy that answers an error fails the ready case", async () => {
    await expect(readyHosted(500)).rejects.toThrow();
  });

  test("a reader that fetches beside the real fetch fails the one-fetch case", async () => {
    const twice: InlineBlocks = {
      read(file) {
        void fetch(`${base}${file}`);
        return reader.read(file);
      },
    };
    await expect(oneFetchWithoutBlock(twice)).rejects.toThrow();
  });

  test("a reader that fetches the proof grid at load fails the proof-grid case", async () => {
    const eager: InlineBlocks = {
      read(file) {
        void fetch(`${base}proof.json`);
        return reader.read(file);
      },
    };
    await expect(proofGridNotAtLoad(eager)).rejects.toThrow();
  });

  test("a missing file answered as a server error fails the empty-frame case", async () => {
    await expect(missingIsEmptyFrame(500)).rejects.toThrow();
  });

  test("a document of the reader's own major fails the not-ready case", async () => {
    await expect(notReadyForMajor("1.0")).rejects.toThrow();
  });

  test("a refresh that drops the stale data fails the stale case", async () => {
    await expect(
      staleKeepsData(async (client) => {
        client.removeQueries({ queryKey: keys.bundle(), exact: true });
        await retryLoad(client, "bundle.json");
      }),
    ).rejects.toThrow();
  });
});
