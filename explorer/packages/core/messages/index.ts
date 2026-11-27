// The message module (web.content.1, .3, .9, .10): typed keys, plain English
// defaults as data, plurals through one function, a sentence as one key with
// parameters. A pure layer (web.structure.5): no document, window or storage.
import { defaultMessages, type Entry } from "./defaults.ts";
import { pluralForm } from "./plural.ts";

export type { Entry, PluralForms } from "./defaults.ts";
export { defaultMessages } from "./defaults.ts";
export { pluralForm } from "./plural.ts";
export * from "./absent.ts";
export * from "./symbols.ts";
export * from "./data.ts";

export type MessageKey = keyof typeof defaultMessages;

type Defaults = typeof defaultMessages;

type Names<S extends string> = S extends `${string}{${infer P}}${infer R}`
  ? P | Names<R>
  : never;

type Template<E> = E extends string
  ? E
  : E extends { readonly one: infer O; readonly other: infer T }
    ? O | T
    : never;

type ParamNames<K extends MessageKey> = Names<Template<Defaults[K]>>;

export type Params<K extends MessageKey> = Readonly<
  Record<ParamNames<K>, string | number>
>;

type Args<K extends MessageKey> = [ParamNames<K>] extends [never]
  ? []
  : [params: Params<K>];

export type Messages = <K extends MessageKey>(
  key: K,
  ...args: Args<K>
) => string;

export type Catalogue = Readonly<Record<string, Entry>>;

const names = (text: string): string[] =>
  [...text.matchAll(/\{([^{}]*)\}/g)].map((m) => m[1] ?? "");

const forms = (entry: Entry): string[] =>
  typeof entry === "string" ? [entry] : Object.values(entry);

const placeholders = (entry: Entry): string[] =>
  [...new Set(forms(entry).flatMap(names))].sort();

// Why a catalogue differs from the defaults, one sentence each: a key it lacks,
// a key the defaults do not have, or a key whose placeholders differ. Every
// locale has the same keys and placeholders (web.content.10).
export function differences(
  catalogue: Catalogue,
  reference: Catalogue = defaultMessages,
): string[] {
  const found: string[] = [];
  for (const [key, entry] of Object.entries(reference)) {
    const other = catalogue[key];
    if (other === undefined) {
      found.push(`missing key ${key}`);
    } else if (
      placeholders(other).join(",") !== placeholders(entry).join(",")
    ) {
      found.push(
        `placeholders of ${key} are {${placeholders(other).join(", ")}}, want {${placeholders(entry).join(", ")}}`,
      );
    } else if ((typeof other === "string") !== (typeof entry === "string")) {
      found.push(`${key} is a plural in one catalogue and not in the other`);
    }
  }
  for (const key of Object.keys(catalogue)) {
    if (!(key in reference)) {
      found.push(`unknown key ${key}`);
    }
  }
  return found;
}

export type Values = Readonly<Record<string, string | number>>;

const fill = (key: string, text: string, params: Values): string =>
  text.replace(/\{([^{}]*)\}/g, (_whole, name: string) => {
    const value = params[name];
    if (value === undefined) {
      throw new RangeError(`message ${key} needs {${name}}`);
    }
    return String(value);
  });

// A host's override may carry some keys: each key it carries must keep the
// default's placeholders, or the module refuses it at boot.
export type Say = (key: string, params?: Values) => string;

export function createSay(locale: string, overrides: Catalogue = {}): Say {
  const catalogue: Catalogue = { ...defaultMessages, ...overrides };
  const problems = differences(catalogue);
  if (problems.length > 0) {
    throw new RangeError(`the messages for ${locale}: ${problems.join("; ")}`);
  }
  const say = (key: string, params: Values = {}): string => {
    const entry = catalogue[key];
    if (entry === undefined) {
      throw new RangeError(`no message ${key}`);
    }
    if (typeof entry === "string") {
      return fill(key, entry, params);
    }
    const count = params["count"];
    if (typeof count !== "number") {
      throw new RangeError(
        `message ${key} is a plural and needs a numeric {count}`,
      );
    }
    return fill(key, pluralForm(locale, count, entry), params);
  };
  return say;
}

// The typed face of the same function: the keys and parameters are checked
// where the call is written.
export function createMessages(
  locale: string,
  overrides: Catalogue = {},
): Messages {
  return createSay(locale, overrides);
}
