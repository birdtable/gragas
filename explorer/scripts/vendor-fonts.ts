// Vendors the explorer's fonts (supply.provenance.5): IBM Plex Sans
// 400, 500 and 600 and IBM Plex Mono 400 and 500, as woff2 and nothing else, into
// packages/tokens/fonts/, with their licence, from the two npm tarballs below at
// the versions and sha512 integrities pinned here. A person runs it, at a bump:
// no tier of the gate does (the test reads the vendored copy and PROVENANCE.json
// offline). The packages are no dependency of the workspace: each carries a
// postinstall script that phones a telemetry service, which strictDepBuilds would
// refuse and which nothing here may run.
//
//   node scripts/vendor-fonts.ts
//
// It is a gate's shape (ts.gates.1): the guard is imported first, an empty
// subject is refused, and the exit code is set, never process.exit. It refuses a
// tarball whose sha512 is not the pin and a package that lacks a file it names (no
// release-age window is held). It needs the network.
import "./gate-guard.ts";
import { createHash } from "node:crypto";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { readUstar } from "./ustar.ts";

interface Pin {
  /** The npm package, which is also the provenance record's name. */
  name: string;
  version: string;
  /** The registry's dist.integrity of the tarball, sha512 in base64. */
  integrity: string;
  /** When the registry says that version was published. */
  published: string;
  /** The folder under fonts/ the copy lands in. */
  folder: string;
  /** The woff2 files taken from fonts/complete/woff2/ of the package. */
  files: string[];
}

// The pins. A bump changes a version, its integrity and its date together.
const pins: Pin[] = [
  {
    name: "@ibm/plex-sans",
    version: "1.1.0",
    integrity:
      "sha512-WPgvO6Yfj2w5YbhyAr1tv95RUz4LRJlqN+CmYvBglabXteufP1D1E9BABMde+ZIKdRbFJDoKF5eQzfhpnbgZcQ==",
    published: "2024-11-11",
    folder: "ibm-plex-sans",
    files: [
      "IBMPlexSans-Regular.woff2",
      "IBMPlexSans-Medium.woff2",
      "IBMPlexSans-SemiBold.woff2",
    ],
  },
  {
    name: "@ibm/plex-mono",
    version: "2.5.0",
    integrity:
      "sha512-STBJIPxPomOYPmBMO7z5TKPJUotAF9u3gAUumTqVgwgrAO+K4FRNh0MlhsoJjKhJKsMbBJR10/bk4inkj/wc1w==",
    published: "2026-06-11",
    folder: "ibm-plex-mono",
    files: ["IBMPlexMono-Regular.woff2", "IBMPlexMono-Medium.woff2"],
  },
];

const explorer = fileURLToPath(new URL("..", import.meta.url));
const fonts = join(explorer, "packages", "tokens", "fonts");

function sha256(file: string): string {
  return createHash("sha256").update(readFileSync(file)).digest("hex");
}

async function download(pin: Pin, into: string): Promise<string> {
  const base = pin.name.split("/")[1] ?? pin.name;
  const url = `https://registry.npmjs.org/${pin.name}/-/${base}-${pin.version}.tgz`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${url}: ${res.status}`);
  const bytes = Buffer.from(await res.arrayBuffer());
  const got = `sha512-${createHash("sha512").update(bytes).digest("base64")}`;
  if (got !== pin.integrity)
    throw new Error(
      `${pin.name}@${pin.version}: the tarball is ${got}, the pin is ${pin.integrity}`,
    );
  const file = join(into, `${base}.tgz`);
  writeFileSync(file, bytes);
  return file;
}

async function main(): Promise<void> {
  if (pins.length === 0)
    throw new Error("no font is pinned: nothing to vendor");
  const today = new Date().toISOString().slice(0, 10);
  const scratch = mkdtempSync(join(tmpdir(), "gragas-fonts-"));
  const record: Record<string, unknown>[] = [];
  try {
    for (const pin of pins) {
      if (pin.files.length === 0)
        throw new Error(`${pin.name} names no file to vendor`);
      const tarball = await download(pin, scratch);
      const unpacked = readUstar(readFileSync(tarball));
      const out = join(fonts, pin.folder);
      mkdirSync(out, { recursive: true });
      const copied: Record<string, string> = {};
      for (const name of [
        ...pin.files.map((f) => ({ from: `fonts/complete/woff2/${f}`, to: f })),
        { from: "LICENSE.txt", to: "LICENSE.txt" },
      ]) {
        const bytes = unpacked.get(`package/${name.from}`);
        if (bytes === undefined)
          throw new Error(
            `${pin.name}@${pin.version} holds no package/${name.from}`,
          );
        writeFileSync(join(out, name.to), bytes);
        copied[name.to] = sha256(join(out, name.to));
      }
      record.push({
        package: pin.name,
        version: pin.version,
        integrity: pin.integrity,
        published: pin.published,
        vendored: today,
        license: "OFL-1.1",
        folder: pin.folder,
        files: copied,
      });
    }
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
  writeFileSync(
    join(fonts, "PROVENANCE.json"),
    `${JSON.stringify({ source: "npm", copies: record }, null, 2)}\n`,
  );
  process.stdout.write(
    `vendored ${record.length} packages into ${relative(explorer, fonts)}\n`,
  );
}

await main();
process.exitCode ||= 0;
