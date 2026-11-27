import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { createServer } from "node:net";
import { fileURLToPath } from "node:url";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig, type Plugin } from "vitest/config";

// MSW's browser worker script is served from the package itself at the path the
// worker registers, so no copy of a third-party file is kept here.
function mswWorker(): Plugin {
  const script = createRequire(import.meta.url).resolve(
    "msw/mockServiceWorker.js",
    { paths: [fileURLToPath(new URL("./packages/core/", import.meta.url))] },
  );
  return {
    name: "msw-worker",
    configureServer(server) {
      server.middlewares.use("/mockServiceWorker.js", (_, res) => {
        res.setHeader("Content-Type", "text/javascript");
        res.end(readFileSync(script));
      });
    },
  };
}

// A port nothing listens on: the OS hands one out for port 0, and the server
// closes at once, so a request to it is refused whatever machine runs the test.
// A browser test cannot bind a port, so it asks the node side.
function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (address === null || typeof address === "string") {
        server.close();
        reject(new Error("the probe server has no port"));
        return;
      }
      server.close(() => {
        resolve(address.port);
      });
    });
  });
}

// The browser pre-bundles each library the first time a test imports it, and a
// run that finds one late reloads the page with two copies of React. Naming the
// parts' tests as entries has Vite's scanner find every library they import
// (all installed under core) before the first test runs.
const scanned = [
  "packages/core/parts/**/*.test.tsx",
  "packages/core/views/**/*.test.tsx",
  "packages/standalone/test/**/*.test.tsx",
];

export default defineConfig({
  optimizeDeps: { entries: scanned },
  // The journey inlines the fixture bundle the Go side ships.
  server: { fs: { allow: ["../.."] } },
  test: {
    // The node tests read config and run the linter; the browser tests run a
    // real component in Chromium's headless shell and in WebKit, with no DOM
    // shim. Playwright's browsers are the folder the gate pins
    // (tools.yaml, browsers:), placed in .tools/playwright by task tools.
    reporters: ["default", "./count-reporter.ts"],
    projects: [
      {
        extends: true,
        test: {
          name: "node",
          include: ["test/**/*.test.ts"],
          environment: "node",
          // The workspace's own tests read config and run the linter: they are
          // slow enough that one at a time keeps the planted files from meeting.
          fileParallelism: false,
          testTimeout: 60_000,
        },
      },
      {
        extends: true,
        plugins: [mswWorker()],
        // The data tests read the engine's fixture folder, one level above the
        // workspace, as text the page imports.
        server: { fs: { allow: [".."] } },
        test: {
          name: "browser",
          // The browser/ folder, and the parts: each part's test and the
          // catalogue test render through core/test's harness.
          include: [
            "browser/**/*.test.ts",
            "packages/core/parts/**/*.test.tsx",
            "packages/core/views/**/*.test.tsx",
            "packages/standalone/test/**/*.test.tsx",
          ],
          testTimeout: 30_000,
          // Each test file runs in an iframe of one page per engine; files side
          // by side, and beside the node projects' own load, lose that iframe
          // under load ("Frame was detached", "Cannot find vitest-iframe").
          // One file at a time per engine, and after the node
          // projects: no retry and no longer timeout hides it.
          fileParallelism: false,
          sequence: { groupOrder: 1 },
          browser: {
            enabled: true,
            headless: true,
            provider: playwright(),
            instances: [{ browser: "chromium" }, { browser: "webkit" }],
            commands: { freePort },
          },
        },
      },
    ],
  },
});
