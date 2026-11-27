// scripts/ustar.ts reads the files of a gzipped ustar archive without a tar program.
// The archives are built here, block by block, so no tar program makes them.
import { gzipSync } from "node:zlib";
import { describe, expect, it } from "vitest";
import { readUstar } from "../scripts/ustar.ts";

function header(name: string, size: number, type: string, prefix = ""): Buffer {
  const block = Buffer.alloc(512);
  block.write(name, 0, "utf8");
  block.write(`${size.toString(8).padStart(11, "0")}\0`, 124, "ascii");
  block.write(type, 156, "ascii");
  block.write("ustar\0", 257, "ascii");
  block.write(prefix, 345, "utf8");
  return block;
}

function entry(name: string, body: string, type = "0", prefix = ""): Buffer {
  const bytes = Buffer.from(body, "utf8");
  const padded = Buffer.alloc(Math.ceil(bytes.length / 512) * 512);
  bytes.copy(padded);
  return Buffer.concat([header(name, bytes.length, type, prefix), padded]);
}

const end = Buffer.alloc(1024);
const archive = (...entries: Buffer[]): Buffer =>
  gzipSync(Buffer.concat([...entries, end]));

describe("scripts/ustar.ts", () => {
  it("reads each regular file by its path, with its bytes", () => {
    const files = readUstar(
      archive(
        entry("package/", "", "5"),
        entry("package/a.txt", "alpha\n"),
        entry("package/big.bin", "b".repeat(1300)),
      ),
    );
    expect([...files.keys()]).toEqual(["package/a.txt", "package/big.bin"]);
    expect(files.get("package/a.txt")?.toString()).toBe("alpha\n");
    expect(files.get("package/big.bin")?.length).toBe(1300);
  });

  it("joins a ustar prefix to the name", () => {
    const files = readUstar(archive(entry("f.txt", "x", "0", "package/deep")));
    expect([...files.keys()]).toEqual(["package/deep/f.txt"]);
  });

  it("takes a long name from a pax path record, for the next file only", () => {
    const long = `package/${"d".repeat(120)}/long.txt`;
    const record = ` path=${long}\n`;
    const pax = `${String(record.length + String(record.length + 2).length)}${record}`;
    const files = readUstar(
      archive(
        entry("PaxHeader", pax, "x"),
        entry("short", "one"),
        entry("package/next.txt", "two"),
      ),
    );
    expect(files.get(long)?.toString()).toBe("one");
    expect(files.get("package/next.txt")?.toString()).toBe("two");
  });

  it("takes a long name from a GNU long-name block", () => {
    const long = `package/${"e".repeat(150)}.txt`;
    const files = readUstar(
      archive(entry("././@LongLink", `${long}\0`, "L"), entry("trunc", "g")),
    );
    expect(files.get(long)?.toString()).toBe("g");
  });

  it("skips a link and refuses a header that is no archive's", () => {
    expect(readUstar(archive(entry("package/l", "", "2"))).size).toBe(0);
    expect(() => readUstar(gzipSync(Buffer.alloc(512, 65)))).toThrow(
      /octal size/,
    );
  });
});
