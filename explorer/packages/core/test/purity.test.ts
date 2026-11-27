// web.structure.5 and web.content.5: the formatter and the messages touch no
// document, window or storage; no Date outside the one date module.
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { keyedFiles, read } from "./guard.ts";

const core = fileURLToPath(new URL("../", import.meta.url));
// keyed is each pure file by its logical key, the layer then the names under it.
const keyed = new Map(
  ["formatter", "messages"].flatMap((layer) =>
    [...keyedFiles(`${core}${layer}`, [".ts"])].map(
      ([key, path]): [string, string] => [`${layer}/${key}`, path],
    ),
  ),
);
const pure = [...keyed.values()];
const strip = (s: string): string =>
  s.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/.*$/gm, "");

describe("the pure layers", () => {
  it("read no document, window, storage, navigator, clock or network", () => {
    expect(pure.length).toBeGreaterThanOrEqual(7);
    const touching = pure.filter((f) =>
      /\b(document|window|localStorage|sessionStorage|indexedDB|navigator|location|globalThis|fetch|Date\.now|performance)\b/.test(
        strip(read(f)),
      ),
    );
    expect(touching).toEqual([]);
  });

  it("do no date arithmetic outside the one date module", () => {
    const users = [...keyed]
      .filter(([, f]) =>
        /\bDate\b|getTime|getTimezoneOffset|setDate/.test(strip(read(f))),
      )
      .map(([key]) => key);
    expect(users).toEqual(["formatter/date.ts"]);
  });

  it("import only their own module and no node module", () => {
    const outside = pure.flatMap((f) =>
      [...strip(read(f)).matchAll(/from\s+"([^"]+)"/g)]
        .map((m) => m[1] ?? "")
        .filter((spec) => !/^\.\.?\//.test(spec))
        .map((spec) => f + ": " + spec),
    );
    expect(outside).toEqual([]);
  });

  it("say the viewer's locale and zone nowhere", () => {
    const reading = pure.filter((f) =>
      /resolvedOptions|navigator\.language|getTimezoneOffset/.test(
        strip(read(f)),
      ),
    );
    expect(reading).toEqual([]);
  });
});
