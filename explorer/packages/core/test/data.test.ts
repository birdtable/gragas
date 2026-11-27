// The data module's six states and its named cases, over the fixture
// bundle served by MSW's request handlers. The request counter is the assertion
// for "no request" and "one request". The browser half runs in the browser tests.
import { fixtureHandlers } from "@gragas/contract/fixtures";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
} from "vitest";
import {
  bundleRegion,
  createExplorerClient,
  keys,
  loadProofGrid,
  loadRoot,
  missingFrame,
  notReadyFrame,
  noRowsFrame,
  reportRegion,
  retryLoad,
  staleBanner,
  viewMarks,
  type DataSource,
  type DataState,
} from "../index.ts";
import type { Json } from "@gragas/contract/fixtures";
import {
  base,
  blocks,
  documents,
  field,
  noBlocks,
  patched,
  snapshotOf,
  type Bundle,
  type Report,
} from "./support.ts";

const served = documents();
const server = setupServer(...fixtureHandlers(base, served));
const requests: string[] = [];
server.events.on("request:start", ({ request }) => {
  requests.push(new URL(request.url).pathname);
});
beforeAll(() => server.listen({ onUnhandledFrame: "error" }));
beforeEach(() => {
  requests.length = 0;
});
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

const hosted: DataSource = { base, inline: noBlocks };

async function bundleState(
  source: DataSource = hosted,
  now = 0,
): Promise<DataState<Bundle>> {
  const client = createExplorerClient();
  await loadRoot(client, source, false);
  return bundleRegion(snapshotOf<Bundle>(client, keys.bundle()), now);
}

function bundleAs(fields: Record<string, Json>): void {
  server.use(
    ...fixtureHandlers(
      base,
      documents((name, doc) =>
        name === "bundle" ? patched(doc, fields) : doc,
      ),
    ),
  );
}

describe("ready", () => {
  it("the fixture bundle served by the request handlers is ready", async () => {
    const state = await bundleState();
    expect(state.state).toBe("ready");
    if (state.state !== "ready") return;
    expect(state.value.rules.length).toBeGreaterThan(0);
    expect(requests).toEqual(["/bundle.json"]);
  });
});

describe("the inline script blocks", () => {
  it("with the bundle in a block: zero requests, and the data is ready", async () => {
    const bundle = served.get("bundle");
    const state = await bundleState({
      base,
      inline: blocks({ "bundle.json": bundle }),
    });
    expect(state.state).toBe("ready");
    expect(requests).toEqual([]);
  });

  it("with no block: exactly one request, for the bundle", async () => {
    await bundleState();
    expect(requests).toEqual(["/bundle.json"]);
  });

  it("standalone with both blocks: zero requests; hosted: one each", async () => {
    const both = blocks({
      "bundle.json": served.get("bundle"),
      "report.json": served.get("report"),
    });
    const inline = createExplorerClient();
    await loadRoot(inline, { base, inline: both }, true);
    expect(requests).toEqual([]);
    const remote = createExplorerClient();
    await loadRoot(remote, hosted, true);
    expect([...requests].sort()).toEqual(["/bundle.json", "/report.json"]);
    expect(
      reportRegion(snapshotOf<Report>(remote, keys.report()), 0).state,
    ).toBe("ready");
  });

  it("a block that is not JSON is an error, not a fetch", async () => {
    const state = await bundleState({
      base,
      inline: {
        read: (file) => (file === "bundle.json" ? "{not json" : undefined),
      },
    });
    expect(state).toMatchObject({
      state: "error",
      retry: true,
      cause: { kind: "invalid" },
    });
    expect(requests).toEqual([]);
  });
});

describe("the proof grid", () => {
  it("is not fetched at load, in either packaging", async () => {
    const client = createExplorerClient();
    await loadRoot(client, hosted, true);
    await loadRoot(
      createExplorerClient(),
      { base, inline: blocks({ "bundle.json": served.get("bundle") }) },
      false,
    );
    expect(requests).not.toContain("/proof.json");
  });

  it("is fetched when a view asks, and only then", async () => {
    server.use(
      http.get(`${base}proof.json`, () =>
        HttpResponse.json(field(served.get("bundle"), "proof_grid")),
      ),
    );
    const loaded = await loadProofGrid(hosted);
    expect(loaded.value.length).toBeGreaterThan(0);
    expect(requests).toEqual(["/proof.json"]);
  });
});

describe("a missing bundle", () => {
  it("is an empty frame naming bundle.json, with Retry", async () => {
    server.use(
      http.get(
        `${base}bundle.json`,
        () => new HttpResponse(null, { status: 404 }),
      ),
    );
    const state = await bundleState();
    expect(state).toEqual({
      state: "empty",
      file: "bundle.json",
      why: "missing",
      retry: true,
    });
    expect(missingFrame(state.file)).toContain("bundle.json");
  });
});

describe("the contract's majors", () => {
  it("a previous major is the same not-ready frame, naming both versions", async () => {
    bundleAs({ contract: "0.9" });
    const state = await bundleState();
    expect(state).toMatchObject({
      state: "error",
      retry: true,
      cause: {
        kind: "not-ready",
        documentVersion: "0.9",
        readerVersion: "1.3",
      },
    });
    expect(notReadyFrame("bundle.json", "0.9", "1.3")).toContain("0.9");
  });

  it("a newer major is the not-ready frame naming both versions", async () => {
    bundleAs({ contract: "2.0" });
    const state = await bundleState();
    expect(state).toMatchObject({
      state: "error",
      retry: true,
      cause: {
        kind: "not-ready",
        documentVersion: "2.0",
        readerVersion: "1.3",
      },
    });
    const frame = notReadyFrame("bundle.json", "2.0", "1.3");
    expect(frame).toContain("2.0");
    expect(frame).toContain("1.3");
  });
});

describe("an unreleased bundle", () => {
  it("carries its mark to every view", async () => {
    bundleAs({ released: false });
    const state = await bundleState();
    expect(state.state).toBe("ready");
    if (state.state !== "ready") return;
    expect(viewMarks(state.value).unreleased).toBe(true);
  });

  it("a released bundle carries none", async () => {
    const state = await bundleState();
    if (state.state !== "ready") throw new Error("the fixture bundle is ready");
    expect(viewMarks(state.value)).toEqual({ unreleased: false });
  });
});

describe("a bundle with zero rules", () => {
  it("is empty, not ready", async () => {
    bundleAs({ rules: [] });
    const state = await bundleState();
    expect(state).toEqual({
      state: "empty",
      file: "bundle.json",
      why: "no-rows",
    });
    expect(noRowsFrame(state.file)).toContain("bundle.json");
  });
});

describe("loading", () => {
  it("is the state before the first answer", () => {
    const client = createExplorerClient();
    expect(bundleRegion(snapshotOf<Bundle>(client, keys.bundle()), 0)).toEqual({
      state: "loading",
      file: "bundle.json",
    });
  });
});

describe("a failed load always offers Retry", () => {
  const failures: [string, () => Parameters<typeof server.use>[0], string][] = [
    [
      "a server error",
      () =>
        http.get(
          `${base}bundle.json`,
          () => new HttpResponse(null, { status: 500 }),
        ),
      "error",
    ],
    [
      "a refused connection",
      () => http.get(`${base}bundle.json`, () => HttpResponse.error()),
      "disconnected",
    ],
    [
      "a body that is not a bundle",
      () =>
        http.get(`${base}bundle.json`, () =>
          HttpResponse.json({ contract: "1.0" }),
        ),
      "error",
    ],
    [
      "a missing bundle",
      () =>
        http.get(
          `${base}bundle.json`,
          () => new HttpResponse(null, { status: 404 }),
        ),
      "empty",
    ],
  ];
  it.each(failures)(
    "%s, then Retry reads the bundle",
    async (_, handler, expected) => {
      server.use(handler());
      const client = createExplorerClient();
      await loadRoot(client, hosted, false);
      const failed = bundleRegion(snapshotOf<Bundle>(client, keys.bundle()), 0);
      expect(failed.state).toBe(expected);
      expect(failed).toMatchObject({ retry: true });
      server.resetHandlers();
      await retryLoad(client, "bundle.json");
      expect(
        bundleRegion(snapshotOf<Bundle>(client, keys.bundle()), 0).state,
      ).toBe("ready");
    },
  );
});

describe("a failed refresh", () => {
  it("keeps the last good data, marked stale with its age, and offers Retry", async () => {
    const client = createExplorerClient();
    await loadRoot(client, hosted, false);
    const good = snapshotOf<Bundle>(client, keys.bundle());
    server.use(
      http.get(
        `${base}bundle.json`,
        () => new HttpResponse(null, { status: 503 }),
      ),
    );
    await retryLoad(client, "bundle.json");
    const held = snapshotOf<Bundle>(client, keys.bundle());
    expect(held.status).toBe("error");
    const state = bundleRegion(held, held.dataUpdatedAt + 90_000);
    expect(state).toMatchObject({
      state: "stale",
      retry: true,
      ageMs: 90_000,
      cause: { kind: "http", status: 503 },
    });
    expect("value" in state ? state.value : null).toEqual(good.data?.value);
    expect(staleBanner(90_000)).toContain("1 min");
    server.resetHandlers();
    await retryLoad(client, "bundle.json");
    expect(
      bundleRegion(snapshotOf<Bundle>(client, keys.bundle()), 0).state,
    ).toBe("ready");
  });

  it("a refused connection with data held is stale too, not disconnected", async () => {
    const client = createExplorerClient();
    await loadRoot(client, hosted, false);
    server.use(http.get(`${base}bundle.json`, () => HttpResponse.error()));
    await retryLoad(client, "bundle.json");
    const state = bundleRegion(snapshotOf<Bundle>(client, keys.bundle()), 0);
    expect(state).toMatchObject({
      state: "stale",
      cause: { kind: "disconnected" },
    });
  });
});

describe("one cache", () => {
  it("asks the network once however often the root is loaded", async () => {
    const client = createExplorerClient();
    await loadRoot(client, hosted, false);
    await loadRoot(client, hosted, false);
    expect(requests).toEqual(["/bundle.json"]);
  });
});
