// supply.install.*: the workspace's pnpm settings refuse a dependency that wants an install
// script, and hold no release-age window. Each case installs a fixture package from a registry this test serves
// on the loopback, with the workspace's real pnpm-workspace.yaml, so no network
// is touched and no package of the workspace is changed. Each case has its own
// registry, because pnpm keeps a package's metadata per registry address.
import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import {
  cpSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { createServer, type Server } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import * as z from "zod";

const FailureSchema = z.object({
  code: z.number().optional(),
  stdout: z.string().optional(),
  stderr: z.string().optional(),
});

const run = promisify(execFile);
const settings = new URL("../pnpm-workspace.yaml", import.meta.url);
const day = 86_400_000;

// The push gate runs with the network blocked, and its sandbox may refuse even a
// loopback listen (EPERM, EACCES): then the cases below are skipped, and the line
// printed here says so. Any other listen error fails the file. The three cases are
// not run in the gate; they run in the targeted run at the merge sha on L-heavy and
// W-1 (the pnpm-workspace.yaml settings are held in every gate by
// install-settings.test.ts).
const ListenErrorSchema = z.object({ code: z.string().optional() });
const refusal = await new Promise<string | undefined>((resolve, reject) => {
  const probe = createServer();
  probe.once("error", (error) => {
    const { code } = ListenErrorSchema.parse(error);
    if (code === "EPERM" || code === "EACCES") resolve(code);
    else reject(error);
  });
  probe.listen(0, "127.0.0.1", () => {
    probe.close(() => {
      resolve(undefined);
    });
  });
});
const canListen = refusal === undefined;
if (!canListen)
  process.stderr.write(
    `install-refusals: the sandbox refused a loopback listen (${refusal}); the cases are skipped\n`,
  );

let scratch = "";
let tarball: Buffer = Buffer.alloc(0);
let packed = 0;

// cacheOption puts pnpm's metadata cache in dir. pnpm 12 has no --cache-dir flag:
// its cacheDir setting is reached as a universal rc-option, --config.<key>=<value>.
function cacheOption(dir: string): string {
  return `--config.cacheDir=${dir}`;
}

// pack builds the fixture package with the given scripts and returns its tarball.
async function pack(scripts: Record<string, string>): Promise<Buffer> {
  packed += 1;
  const dir = join(scratch, `pkg-${String(packed)}`);
  mkdirSync(dir);
  writeFileSync(
    join(dir, "package.json"),
    JSON.stringify({ name: "fixture-dep", version: "1.0.0", scripts }),
  );
  await run(
    "pnpm",
    ["pack", "--pack-destination", dir, cacheOption(join(dir, "cache"))],
    { cwd: dir },
  );
  return readFileSync(join(dir, "fixture-dep-1.0.0.tgz"));
}

function packument(
  registry: string,
  ageDays: number,
  scripts: Record<string, string>,
): string {
  const when = new Date(Date.now() - ageDays * day).toISOString();
  return JSON.stringify({
    name: "fixture-dep",
    "dist-tags": { latest: "1.0.0" },
    versions: {
      "1.0.0": {
        name: "fixture-dep",
        version: "1.0.0",
        scripts,
        dist: {
          tarball: `${registry}/fixture-dep/-/fixture-dep-1.0.0.tgz`,
          integrity: `sha512-${createHash("sha512").update(tarball).digest("base64")}`,
          shasum: createHash("sha1").update(tarball).digest("hex"),
        },
      },
    },
    time: { created: when, modified: when, "1.0.0": when },
  });
}

async function serve(
  ageDays: number,
  scripts: Record<string, string>,
  port = 0,
): Promise<{ server: Server; registry: string }> {
  let registry = "";
  const server = createServer((req, res) => {
    if (req.url?.endsWith(".tgz") === true) {
      res.setHeader("content-type", "application/octet-stream");
      res.end(tarball);
      return;
    }
    res.setHeader("content-type", "application/json");
    res.end(packument(registry, ageDays, scripts));
  });
  await new Promise<void>((resolve) =>
    server.listen(port, "127.0.0.1", resolve),
  );
  const address = server.address();
  if (address === null || typeof address === "string")
    throw new Error("the registry has no port");
  registry = `http://127.0.0.1:${String(address.port)}`;
  return { server, registry };
}

interface Run {
  code: number;
  out: string;
  lock: string;
}

// install runs pnpm install in a fresh project against a registry that serves the
// fixture package at ageDays. With a lock the project starts from it and pnpm is
// held to it (--frozen-lockfile), as make explorer-deps does. Both the store and
// pnpm's metadata cache live in the project, so no run reads another's; a port
// may be named to serve on, which a run on a reused address then shares, and a
// cache to share between runs of one lock's lineage.
async function install(
  ageDays: number,
  scripts: Record<string, string>,
  lock?: string,
  port?: number,
  cache?: string,
): Promise<Run> {
  tarball = await pack(scripts);
  const { server, registry } = await serve(ageDays, scripts, port);
  const project = mkdtempSync(join(scratch, "project-"));
  cpSync(settings, join(project, "pnpm-workspace.yaml"));
  writeFileSync(
    join(project, "package.json"),
    JSON.stringify({
      name: "fixture-project",
      version: "1.0.0",
      private: true,
      dependencies: { "fixture-dep": "1.0.0" },
    }),
  );
  if (lock !== undefined) writeFileSync(join(project, "pnpm-lock.yaml"), lock);
  const args = [
    "install",
    "--registry",
    registry,
    "--store-dir",
    join(project, "store"),
    cacheOption(cache ?? join(project, "cache")),
  ];
  if (lock !== undefined) args.push("--frozen-lockfile");
  const lockOf = (): string => {
    try {
      return readFileSync(join(project, "pnpm-lock.yaml"), "utf8");
    } catch {
      return "";
    }
  };
  try {
    const done = await run("pnpm", args, { cwd: project });
    return { code: 0, out: done.stdout + done.stderr, lock: lockOf() };
  } catch (error) {
    const failed = FailureSchema.parse(error);
    return {
      code: failed.code ?? 1,
      out: `${failed.stdout ?? ""}${failed.stderr ?? ""}`,
      lock: lockOf(),
    };
  } finally {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
}

// The setup hook has the 60 s the file's tests have (vitest.config.ts); the teardown
// has 120 s, below.
beforeAll(() => {
  scratch = mkdtempSync(join(tmpdir(), "gragas-install-refusals-"));
}, 60_000);

// Every run leaves a store and a metadata cache in the scratch, and removing them
// on a loaded machine took more than the hook's default ten seconds.
afterAll(() => {
  rmSync(scratch, { recursive: true, force: true });
}, 120_000);

describe("pnpm install", () => {
  // No release-age window is held, so a release published a day ago installs as an old one does.
  it.skipIf(!canListen)(
    "installs a release younger than seven days, with no window",
    async () => {
      const { code, out } = await install(1, {});
      expect(out).not.toContain("minimumReleaseAge");
      expect(out).not.toContain("ERR_PNPM");
      expect(code).toBe(0);
    },
  );

  it.skipIf(!canListen)(
    "installs an old release that has no script (the control)",
    async () => {
      const { code, out } = await install(400, {});
      expect(out).not.toContain("ERR_PNPM");
      expect(code).toBe(0);
    },
  );

  it.skipIf(!canListen)(
    "refuses an old release that wants an install script",
    async () => {
      const { code, out } = await install(400, { postinstall: "node -e 0" });
      expect(code).not.toBe(0);
      expect(out).toMatch(/ERR_PNPM_IGNORED_BUILDS|build scripts|ignored/i);
    },
  );
});
