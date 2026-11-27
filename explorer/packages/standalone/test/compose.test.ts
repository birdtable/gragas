// compose.ts is the one file that names every view (arch.ports.13): its list of
// landed views is held to the folders of core/views, the layer layers.yaml
// names, and every one of the eleven has a manifest.
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { expect, test } from "vitest";
import { manifests, registered } from "../src/compose.ts";
import { viewIds } from "../src/views.ts";

const here = fileURLToPath(new URL("..", import.meta.url));
const views = join(here, "../core/views");

function folders(): string[] {
  try {
    return (
      readdirSync(views, { withFileTypes: true })
        .filter((e) => e.isDirectory())
        .map((e) => e.name)
        // A folder holds a landed view once it holds its page (a .tsx file); one
        // with only its pure selectors and their test is a view still to build.
        .filter((name) =>
          readdirSync(join(views, name)).some((f) => f.endsWith(".tsx")),
        )
        .sort()
    );
  } catch {
    return [];
  }
}

test("every view has a manifest, and the landed ones are the folders of core/views", () => {
  expect(Object.keys(manifests).sort()).toEqual([...viewIds].sort());
  expect([...registered].sort()).toEqual(folders());
});

test("core/views is a layer layers.yaml names, and the shell may import it", () => {
  const layers = readFileSync(join(here, "../../../layers.yaml"), "utf8");
  expect(layers).toContain("path: explorer/packages/core/views/*");
});

test("compose.ts is the only file that imports a view", () => {
  const sources = readdirSync(join(here, "src"), { recursive: true })
    .map(String)
    .filter((f) => /\.tsx?$/.test(f) && f !== "compose.ts");
  const offenders = sources.filter((f) =>
    /from\s+"[^"]*core\/views\//.test(
      readFileSync(join(here, "src", f), "utf8"),
    ),
  );
  expect(offenders).toEqual([]);
});
