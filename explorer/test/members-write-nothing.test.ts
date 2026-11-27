// The explorer's members (typecheck, lint, test) write nothing under the checkout, outside
// node_modules: cmd/gragas-new's copy test copies the tree and then walks the live one again while
// the members run beside it, so a file a member writes in between is a stray.
// The typecheck and the lint run through scripts/write-nothing.ts, which lists the files before and
// after the member and goes red naming each one added, changed or removed. This test
// checks the wrapper, on planted members and a folder of its own under the temp folder, so it
// writes nothing under the checkout itself and runs in a second.
import { spawnSync } from "node:child_process";
import {
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

const script = fileURLToPath(
  new URL("../scripts/write-nothing.ts", import.meta.url),
);

let root = "";
beforeEach(() => {
  root = mkdtempSync(join(tmpdir(), "write-nothing-"));
  mkdirSync(join(root, "src"));
  writeFileSync(join(root, "src", "a.ts"), "export {};\n");
  writeFileSync(join(root, "src", "b.ts"), "export {};\n");
  mkdirSync(join(root, "node_modules"));
});
afterEach(() => {
  rmSync(root, { recursive: true, force: true });
});

// member runs the wrapper over a node program, as `node write-nothing.ts --root <root> -- node -e <program>`.
function member(program: string): {
  code: number | null;
  out: string;
  err: string;
} {
  const run = spawnSync(
    process.execPath,
    [script, "--root", root, "--", process.execPath, "-e", program],
    { encoding: "utf8" },
  );
  return { code: run.status, out: run.stdout, err: run.stderr };
}

// at is a program that runs inside the planted member, with the root in `root`.
const at = (body: string): string =>
  `const fs = require("node:fs"); const root = ${JSON.stringify(root)}; ${body}`;

describe("scripts/write-nothing.ts", () => {
  it("passes a member that writes nothing, and its output", () => {
    const run = member(at(`console.log("the member ran");`));
    expect(run.code).toBe(0);
    expect(run.out).toContain("the member ran");
    expect(run.err).toBe("");
  });

  it("is red naming a file the member added", () => {
    const run = member(at(`fs.writeFileSync(root + "/src/dist.js", "x");`));
    expect(run.code).toBe(1);
    expect(run.err).toContain("added src/dist.js");
  });

  it("is red naming a file the member changed, and one it removed", () => {
    const run = member(
      at(
        `fs.writeFileSync(root + "/src/a.ts", "export const a = 1;\\n"); fs.unlinkSync(root + "/src/b.ts");`,
      ),
    );
    expect(run.code).toBe(1);
    expect(run.err).toContain("changed src/a.ts");
    expect(run.err).toContain("removed src/b.ts");
  });

  it("is red on a file rewritten with the same size and a new time", () => {
    const when = new Date(Date.now() + 60_000);
    const run = member(
      at(
        `fs.utimesSync(root + "/src/a.ts", ${String(when.getTime() / 1000)}, ${String(when.getTime() / 1000)});`,
      ),
    );
    expect(run.code).toBe(1);
    expect(run.err).toContain("changed src/a.ts");
  });

  it("passes a member that writes only under node_modules", () => {
    const run = member(
      at(`fs.writeFileSync(root + "/node_modules/cache", "x");`),
    );
    expect(run.code).toBe(0);
  });

  it("keeps the exit code of a member that fails, and still names what it wrote", () => {
    const run = member(
      at(`fs.writeFileSync(root + "/src/c.ts", "x"); process.exit(3);`),
    );
    expect(run.code).toBe(3);
    expect(run.err).toContain("added src/c.ts");
  });

  it("names each file by its path from the root, with / between segments and no drive or backslash", () => {
    const run = member(
      at(
        `fs.mkdirSync(root + "/src/deep/er", { recursive: true }); fs.writeFileSync(root + "/src/deep/er/x.ts", "x"); fs.writeFileSync(root + "/src/a.ts", "export const a = 1;\\n");`,
      ),
    );
    expect(run.code).toBe(1);
    const names = run.err
      .split("\n")
      .map((line) => /^ {2}(?:added|changed|removed) (.*)$/.exec(line)?.[1])
      .filter((name) => name !== undefined);
    expect(names.sort()).toEqual(["src/a.ts", "src/deep/er/x.ts"]);
    for (const name of names) {
      expect(name).not.toMatch(/\\|^[A-Za-z]:|^\//);
    }
  });

  it("runs the member in the folder it was started in", () => {
    const run = spawnSync(
      process.execPath,
      [
        script,
        "--root",
        root,
        "--",
        process.execPath,
        "-e",
        "console.log(process.cwd())",
      ],
      { encoding: "utf8", cwd: join(root, "src") },
    );
    expect(run.status).toBe(0);
    expect(realpathSync(run.stdout.trim())).toBe(
      realpathSync(join(root, "src")),
    );
  });

  it("is red without a command", () => {
    const run = spawnSync(process.execPath, [script, "--root", root], {
      encoding: "utf8",
    });
    expect(run.status).not.toBe(0);
    expect(run.stderr).toContain("usage");
  });
});
