// Done when: a test fails a string literal sentence in a component, and a key
// used in code exists (gates.plants.1: each planted violation fails, the clean
// control passes).
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { defaultMessages } from "../messages/index.ts";
import { read, sentenceLiterals, sourceFiles, usedKeys } from "./guard.ts";

const packages = fileURLToPath(new URL("../../", import.meta.url));

describe("sentence literals in a component", () => {
  it("fails JSX text, a prop and an expression that hold a sentence", () => {
    expect(
      sentenceLiterals("const A = () => <p>Nothing matches your filters</p>;"),
    ).toEqual(["Nothing matches your filters"]);
    expect(
      sentenceLiterals(
        'const A = () => <button aria-label="Open the rule" />;',
      ),
    ).toEqual(["Open the rule"]);
    expect(
      sentenceLiterals('const A = () => <p>{ok ? "No rules found" : x}</p>;'),
    ).toEqual(["No rules found"]);
  });

  it("passes words from the message module, names and paths", () => {
    const clean = [
      'import { x } from "./some file.ts";',
      "// a comment with a few words",
      'const A = () => <p className="a b c" data-part="rule card">{msg("standing.enforced")}</p>;',
      "const B = () => <Row id={id} />;",
    ].join("\n");
    expect(sentenceLiterals(clean)).toEqual([]);
  });

  it("finds none in the tree's own components", () => {
    // Stories and tests hold sample data, not shipped words; the parts they
    // render take every word of their own from the messages.
    const files = sourceFiles(packages, [".tsx"]).filter(
      (f) => !/\.(stories|test)\.tsx$/.test(f),
    );
    const bad = files.flatMap((f) =>
      sentenceLiterals(read(f)).map((s) => `${f}: ${s}`),
    );
    expect(bad).toEqual([]);
  });
});

describe("a key used in code exists", () => {
  it("reads the literal keys of msg calls", () => {
    expect(
      usedKeys(
        'const a = msg("standing.enforced"); // msg("gone")\nmsg( "made.up" )',
      ),
    ).toEqual(["standing.enforced", "made.up"]);
  });

  it("names only keys the defaults hold", () => {
    const files = sourceFiles(packages, [".ts", ".tsx"]);
    expect(files.length).toBeGreaterThan(5);
    const unknown = files.flatMap((f) =>
      usedKeys(read(f))
        .filter((k) => !(k in defaultMessages))
        .map((k) => `${join(f)}: ${k}`),
    );
    expect(unknown).toEqual([]);
  });

  it("would fail a key the defaults lack", () => {
    expect(
      usedKeys('msg("made.up")').filter((k) => !(k in defaultMessages)),
    ).toEqual(["made.up"]);
  });
});
