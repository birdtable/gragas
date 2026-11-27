// Where a link in the explorer goes: a typed value, not a
// string somebody built, so a view and the palette agree on the routes and a
// test can compare the two. The path is the hash route of the standalone
// bundle; it has no scheme and no host. An id is encoded one segment at a time,
// and a tool entry's id, "tool/key", is two segments with a slash between them:
// `/relations/tool-entry/<tool>/<key>`, and `/relations/tool-entry/<tool>` is the
// tool alone. One writer and one parser live here, so the palette's links and the
// relations route read the same form.

/** The entities the relations view (#/relations/:kind/:id) can be asked about. */
export type RelationKind =
  "rule" | "decision" | "source" | "binding" | "tool-entry" | "part" | "term";

export const relationKinds: readonly RelationKind[] = [
  "rule",
  "decision",
  "source",
  "binding",
  "tool-entry",
  "part",
  "term",
];

export type Destination =
  | { readonly route: "rule"; readonly id: string }
  | {
      readonly route: "relations";
      readonly kind: RelationKind;
      readonly id: string;
    };

// The separator of a rule id's segments in a URL: a key, never a file path.
const idSeparator = "/";

const segments = (id: string): string =>
  id.split(idSeparator).map(encodeURIComponent).join(idSeparator);

const idPath = (kind: RelationKind, id: string): string =>
  kind === "tool-entry" ? segments(id) : encodeURIComponent(id);

/** The hash path of a destination: `/rules/<id>` or `/relations/<kind>/<id>`. */
export function destinationPath(destination: Destination): string {
  return destination.route === "rule"
    ? `/rules/${encodeURIComponent(destination.id)}`
    : `/relations/${destination.kind}/${idPath(destination.kind, destination.id)}`;
}

function decoded(parts: readonly string[]): string | null {
  try {
    return parts.map(decodeURIComponent).join(idSeparator);
  } catch {
    return null;
  }
}

/**
 * The destination a hash path names, or null for a path that is none: the
 * inverse of `destinationPath`. A rule id is one segment; the id of a tool
 * entry is the rest of the path after its kind, so its slash is kept.
 */
export function parseDestination(path: string): Destination | null {
  const parts = path.split("/");
  if (parts[0] !== "") return null;
  const [, head, kind, ...rest] = parts;
  if (head === "rules" && kind !== undefined && rest.length === 0) {
    const id = decoded([kind]);
    return id === null || id === "" ? null : { route: "rule", id };
  }
  const found = relationKinds.find((k) => k === kind);
  if (head !== "relations" || found === undefined || rest.length === 0) {
    return null;
  }
  const id = decoded(rest);
  const oneSegment = found !== "tool-entry" && rest.length > 1;
  if (id === null || oneSegment || rest.includes("")) {
    return null;
  }
  return { route: "relations", kind: found, id };
}
