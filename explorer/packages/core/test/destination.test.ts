// The one route form the palette writes and the relations route reads:
// `parseDestination` is the inverse of `destinationPath`, and a path that is
// none is null.
import { describe, expect, it } from "vitest";
import {
  destinationPath,
  parseDestination,
  relationKinds,
  type Destination,
} from "../index.ts";

const samples: readonly Destination[] = [
  { route: "rule", id: "go.errors.1" },
  { route: "rule", id: "odd id/with%chars" },
  ...relationKinds.map((kind): Destination => ({
    route: "relations",
    kind,
    id: "a-b.c",
  })),
  {
    route: "relations",
    kind: "tool-entry",
    id: "golangci-lint/revive.var-naming",
  },
  { route: "relations", kind: "tool-entry", id: "golangci-lint" },
  {
    route: "relations",
    kind: "tool-entry",
    id: "eslint/@typescript-eslint/no-explicit-any",
  },
  { route: "relations", kind: "term", id: "wrap ping" },
];

describe("destination paths", () => {
  it("round trip: the parser reads what the writer wrote", () => {
    for (const d of samples) {
      expect(parseDestination(destinationPath(d))).toEqual(d);
    }
  });

  it("write a tool entry with a slash between its tool and its key", () => {
    expect(
      destinationPath({
        route: "relations",
        kind: "tool-entry",
        id: "golangci-lint/errorlint",
      }),
    ).toBe("/relations/tool-entry/golangci-lint/errorlint");
    expect(destinationPath({ route: "rule", id: "a b" })).toBe("/rules/a%20b");
  });

  it("refuse a path that is no destination", () => {
    for (const path of [
      "",
      "/",
      "rules/x",
      "/rules",
      "/rules/",
      "/rules/a/b",
      "/rules/%E0%A4%A",
      "/relations",
      "/relations/rule",
      "/relations/nosuch/x",
      "/relations/rule/a/b",
      "/relations/tool-entry//b",
      "/relations/tool-entry/a/",
      "/elsewhere/rule/x",
    ]) {
      expect([path, parseDestination(path)]).toEqual([path, null]);
    }
  });
});

describe("the id of an entity that is not a tool entry", () => {
  it("is one segment, its own slash encoded, and reads back whole", () => {
    const binding: Destination = {
      route: "relations",
      kind: "binding",
      id: "go.errors.1@golangci-lint/wrapcheck",
    };
    const path = destinationPath(binding);
    expect(path).toBe(
      "/relations/binding/go.errors.1%40golangci-lint%2Fwrapcheck",
    );
    expect(parseDestination(path)).toEqual(binding);
    expect(parseDestination("/relations/binding/a/b")).toBeNull();
  });
});
