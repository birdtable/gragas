// gates.plants.1 for the browser project: a planted component
// test fails on each engine, and a clean control passes on each. The plant is
// written to a folder of its own under node_modules/, which every walk of the
// working tree leaves out (gragas-new's copy test, the formatter, Vitest's own
// include), so a plant that comes or goes mid-run is never a stray file; the
// config's imports still resolve upward to explorer/node_modules. Removed after.
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const root = fileURLToPath(new URL("..", import.meta.url));

// hangBound is the one wall-clock bound in this file. Each case
// waits on an event, the exit of the Vitest run it starts, and the run is
// judged by what it printed, never by how long it took. The bound is there
// only so a run that never ends cannot hold the push slot: an engine that
// never reports (WebKit, once, under a load near 100, while
// Chromium reported in 131 ms) keeps Vitest waiting for ever. A whole run of
// both engines takes about 2 s on an idle machine, so a run still going at
// 120 s is held for a hang, not for a slow machine. The run is killed then,
// and the case fails saying so. Each case's own timeout is a quarter more
// than the bound, so the kill and its finding come first.
const hangBound = 120_000;
const bounded = { timeout: hangBound + hangBound / 4 };

const config = `import { playwright } from "@vitest/browser-playwright";
import { defineConfig } from "vitest/config";
export default defineConfig({
  test: {
    include: ["*.plant.ts"],
    // One engine at a time, so a loaded machine starts WebKit after Chromium
    // has reported rather than beside it.
    fileParallelism: false,
    browser: {
      enabled: true,
      headless: true,
      provider: playwright(),
      instances: [{ browser: "chromium" }, { browser: "webkit" }],
    },
  },
});
`;

function run(body: string): { status: number | null; out: string } {
  const r = runFor(body);
  if (r.killed) {
    throw new Error(
      `the Vitest run did not end in ${hangBound / 1000} s and was killed: an engine never reported. What it printed:\n${r.out}`,
    );
  }
  return r;
}

// colourless is the environment with no FORCE_COLOR, which Vitest takes as colour on whatever it holds, and NO_COLOR set.
function colourless(): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = { ...process.env, NO_COLOR: "1" };
  delete env["FORCE_COLOR"];
  return env;
}

function runFor(body: string): {
  status: number | null;
  out: string;
  killed: boolean;
} {
  const home = join(root, "node_modules", ".plants");
  mkdirSync(home, { recursive: true });
  const dir = mkdtempSync(join(home, "plant-"));
  try {
    writeFileSync(join(dir, "vitest.config.ts"), config);
    writeFileSync(
      join(dir, "a.plant.ts"),
      `import { expect, test } from "vitest";\ntest("plant", () => {\n${body}\n});\n`,
    );
    const done = spawnSync(
      process.execPath,
      [
        join(root, "node_modules/vitest/vitest.mjs"),
        "run",
        "--root",
        dir,
        "--config",
        join(dir, "vitest.config.ts"),
      ],
      // SIGKILL, not the default SIGTERM: Vitest handles SIGTERM by closing
      // gracefully, and a run that spun for an hour held the push slot without
      // ever honouring it. A run past hangBound has no status and its signal
      // is SIGKILL, and run fails it naming the hang.
      // The colour is off: a run under a budget has a terminal, pnpm then sets FORCE_COLOR, and the
      // summary this reads would come with escape codes inside it. Vitest reads the
      // presence of FORCE_COLOR, whatever it holds, so it is taken out, not set to 0.
      {
        encoding: "utf8",
        timeout: hangBound,
        killSignal: "SIGKILL",
        env: colourless(),
      },
    );
    return {
      status: done.status,
      out: done.stdout + done.stderr,
      killed: done.status === null && done.signal === "SIGKILL",
    };
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

describe("the browser project holds on Chromium and WebKit", bounded, () => {
  it("passes a clean control on each engine", () => {
    const r = run(
      `const el = document.createElement("div"); el.style.width = "10px"; document.body.append(el); expect(el.getBoundingClientRect().width).toBe(10);`,
    );
    // One test file per engine: two files passed is both engines.
    expect(r.out).toMatch(/Test Files\s+2 passed \(2\)/);
    expect(r.status).toBe(0);
  });

  it("fails a planted component test on each engine", () => {
    const r = run(
      `const el = document.createElement("div"); el.style.width = "10px"; document.body.append(el); expect(el.getBoundingClientRect().width).toBe(11);`,
    );
    expect(r.status).not.toBe(0);
    // Tell "an engine never ran" from "its planted test did not fail"
    // Each engine's file is named in the output on its own.
    for (const engine of ["chromium", "webkit"]) {
      expect(
        r.out,
        `the ${engine} plant never reported; the run printed:\n${r.out}`,
      ).toMatch(new RegExp(`\\|${engine}\\| a\\.plant\\.ts .*1 failed`));
    }
    expect(r.out).toMatch(/Test Files\s+2 failed \(2\)/);
  });
});
