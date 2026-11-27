// Joins are by id through a map (arch.ports.7): never by position.
// The test beside this folder fails any selector that indexes an array with a
// counter or zips two arrays by their index.
export interface IdIndex<K extends string, T> {
  readonly byId: ReadonlyMap<K, T>;
  /** Ids that appear more than once; the first row of each is kept. */
  readonly duplicates: readonly K[];
}

export function indexById<K extends string, T>(
  items: readonly T[],
  idOf: (item: T) => K,
): IdIndex<K, T> {
  const byId = new Map<K, T>();
  const duplicates = new Set<K>();
  for (const item of items) {
    const id = idOf(item);
    if (byId.has(id)) duplicates.add(id);
    else byId.set(id, item);
  }
  return { byId, duplicates: [...duplicates] };
}

/** Every item under its id, in the order met: nothing is dropped. */
export function groupById<K extends string, T>(
  items: readonly T[],
  idOf: (item: T) => K,
): ReadonlyMap<K, readonly T[]> {
  const groups = new Map<K, T[]>();
  for (const item of items) {
    const id = idOf(item);
    const group = groups.get(id);
    if (group === undefined) groups.set(id, [item]);
    else group.push(item);
  }
  return groups;
}

export interface Joined<L, R> {
  readonly row: L;
  readonly match: R | undefined;
}

/** Each left row with the right row of the same id, or undefined. */
export function joinById<K extends string, L, R>(
  rows: readonly L[],
  keyOf: (row: L) => K,
  index: ReadonlyMap<K, R>,
): readonly Joined<L, R>[] {
  return rows.map((row) => ({ row, match: index.get(keyOf(row)) }));
}
