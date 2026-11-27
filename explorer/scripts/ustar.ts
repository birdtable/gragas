// A reader for the files of a gzipped ustar archive, which is what an npm package's tarball is:
// node:zlib for the gzip and 512-byte blocks for the rest, so no tar program is run and none is
// needed on any system. It reads regular files only, names each by its path in the
// archive, and takes a long name from a pax `path` record or a GNU long-name block. A link, a
// folder, a device and anything else is skipped, and a block that is not a header is refused.
import { gunzipSync } from "node:zlib";

const BLOCK = 512;

function text(block: Buffer, from: number, length: number): string {
  const field = block.subarray(from, from + length);
  const end = field.indexOf(0);
  return field.subarray(0, end === -1 ? length : end).toString("utf8");
}

function octal(block: Buffer, from: number, length: number): number {
  const value = text(block, from, length).trim();
  if (!/^[0-7]+$/.test(value))
    throw new Error(
      `a tar header holds ${JSON.stringify(value)}, not an octal size`,
    );
  return parseInt(value, 8);
}

// paxPath is the path a pax extended header names, if it names one.
function paxPath(body: string): string | undefined {
  let rest = body;
  while (rest.length > 0) {
    const space = rest.indexOf(" ");
    const length = Number(rest.slice(0, space));
    if (!Number.isInteger(length) || length <= space) return undefined;
    const record = rest.slice(space + 1, length - 1);
    if (record.startsWith("path=")) return record.slice("path=".length);
    rest = rest.slice(length);
  }
  return undefined;
}

// readUstar maps each regular file of the archive to its bytes.
export function readUstar(archive: Buffer): Map<string, Buffer> {
  const tar = gunzipSync(archive);
  const files = new Map<string, Buffer>();
  let long: string | undefined;
  let at = 0;
  while (at + BLOCK <= tar.length) {
    const header = tar.subarray(at, at + BLOCK);
    if (header.every((byte) => byte === 0)) break;
    const size = octal(header, 124, 12);
    const type = text(header, 156, 1);
    const prefix = text(header, 345, 155);
    const own = text(header, 0, 100);
    const body = tar.subarray(at + BLOCK, at + BLOCK + size);
    if (body.length !== size) throw new Error("the archive ends inside a file");
    at += BLOCK + Math.ceil(size / BLOCK) * BLOCK;
    if (type === "x") {
      long = paxPath(body.toString("utf8")) ?? long;
    } else if (type === "L") {
      long = text(body, 0, body.length);
    } else if (type === "g") {
      continue;
    } else {
      if (type === "0" || type === "") {
        files.set(
          long ?? (prefix === "" ? own : `${prefix}/${own}`),
          Buffer.from(body),
        );
      }
      long = undefined;
    }
  }
  return files;
}
