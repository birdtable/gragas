// The route tree is committed, and regenerating it from the route files changes
// nothing.
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { expect, test } from "vitest";

const here = fileURLToPath(new URL("..", import.meta.url));

test("regenerating the route tree leaves it unchanged", () => {
  const scratch = mkdtempSync(join(tmpdir(), "gragas-routes-"));
  try {
    mkdirSync(join(scratch, "src"));
    cpSync(join(here, "src/routes"), join(scratch, "src/routes"), {
      recursive: true,
    });
    cpSync(join(here, "tsr.config.json"), join(scratch, "tsr.config.json"));
    const tsr = join(here, "node_modules/@tanstack/router-cli/bin/tsr.cjs");
    execFileSync(process.execPath, [tsr, "generate"], {
      cwd: scratch,
      stdio: "pipe",
    });
    expect(readFileSync(join(scratch, "src/routeTree.gen.ts"), "utf8")).toBe(
      readFileSync(join(here, "src/routeTree.gen.ts"), "utf8"),
    );
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
});
