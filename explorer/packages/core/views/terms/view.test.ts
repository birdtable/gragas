// The terms view's selectors over the fixture bundle, in node: every
// term with its literal and definition as the bundle states them; the stale
// French translation marked and the missing one falling back to the authoritative
// text with its own mark; the rules that use a term joined by id; and, by
// absence, no field the list does not name.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  BundleDocument,
  FieldListSchema,
  parse,
  recordReads,
  unlisted,
} from "@gragas/contract";
import { describe, expect, it } from "vitest";
import { documents, type Bundle } from "../../test/support.ts";
import { termsUsedBy, termsView } from "./selectors.ts";

function fixture(): Bundle {
  const parsed = parse(BundleDocument, documents().get("bundle"));
  if (parsed.status !== "ok") throw new Error("the fixture must parse");
  return parsed.value;
}

const bundle = fixture();
const row = (
  view: ReturnType<typeof termsView>,
  slug: string,
): ReturnType<typeof termsView>["rows"][number] | undefined =>
  view.rows.find((r) => r.slug === slug);

describe("the authoritative language", () => {
  const view = termsView(bundle);

  it("lists every term of the bundle, in the bundle's order", () => {
    expect(view.rows.map((r) => r.slug)).toEqual(
      bundle.terms.map((t) => t.slug),
    );
    expect(view.rows.length).toBeGreaterThan(0);
  });

  it("states each term's definition and literal as the bundle does", () => {
    expect(view.authoritative).toBe(bundle.identity.language);
    expect(view.chosen).toBe(view.authoritative);
    for (const term of bundle.terms) {
      const r = row(view, term.slug);
      expect(r?.definition).toEqual({
        text: term.definition,
        mark: "authoritative",
      });
      expect(r?.literal).toEqual(
        term.literal === ""
          ? { kind: "dash", reason: "absent.none-recorded" }
          : { kind: "value", value: term.literal },
      );
    }
  });

  it("gives a term with no literal a dash that says so, never an empty string", () => {
    expect(row(view, "wrap")?.literal).toEqual({
      kind: "dash",
      reason: "absent.none-recorded",
    });
    expect(row(view, "gate")?.literal).toEqual({
      kind: "value",
      value: "make check",
    });
  });

  it("lists every translation of a term, stale or not", () => {
    expect(row(view, "gate")?.translations).toEqual([
      {
        language: "fr",
        text: "Un contrôle qui arrête un envoi quand il échoue.",
        stale: false,
      },
    ]);
    expect(row(view, "wrap")?.translations.map((t) => t.stale)).toEqual([true]);
  });

  it("counts the translations and the stale ones from the bundle", () => {
    const all = bundle.terms.flatMap((t) => t.translations);
    expect(view.translationCount).toBe(all.length);
    expect(view.staleCount).toBe(all.filter((t) => t.stale).length);
    expect(view.staleCount).toBe(1);
  });

  it("offers the authoritative language first, then each translated one", () => {
    expect(view.languages).toEqual(["en", "fr"]);
  });
});

describe("a language chosen in the URL", () => {
  const view = termsView(bundle, "fr");

  it("shows a current translation as translated", () => {
    expect(row(view, "gate")?.definition).toEqual({
      text: "Un contrôle qui arrête un envoi quand il échoue.",
      mark: "translated",
    });
  });

  it("shows a stale translation with the stale mark, still as the bundle states it", () => {
    expect(row(view, "wrap")?.definition).toEqual({
      text: "Ajouter ce qui était en cours à une erreur avant de la rendre.",
      mark: "stale",
    });
  });

  it("falls back to the authoritative text, marked, when the term has no translation", () => {
    const without = termsView(
      {
        ...bundle,
        terms: bundle.terms.map((t) => ({ ...t, translations: [] })),
      },
      "fr",
    );
    for (const r of without.rows) {
      expect(r.definition.mark).toBe("fallback");
      expect(r.definition.text).toBe(
        bundle.terms.find((t) => t.slug === r.slug)?.definition,
      );
    }
  });

  it("a language nobody translated into falls back on every term", () => {
    const de = termsView(bundle, "de");
    expect(de.rows.every((r) => r.definition.mark === "fallback")).toBe(true);
    expect(de.chosen).toBe("de");
  });
});

describe("the rules that use a term", () => {
  const view = termsView(bundle);

  it("joins by id: a listed rule exists and says the term in its statement or reasoning", () => {
    const byId = new Map(bundle.rules.map((r) => [r.id, r]));
    for (const r of view.rows) {
      for (const id of r.usedBy) {
        const rule = byId.get(id);
        expect(rule).toBeDefined();
        const prose = `${rule?.statement ?? ""} ${rule?.reasoning ?? ""}`;
        expect(prose.toLowerCase()).toContain(
          r.slug.replace(/-/g, " ").toLowerCase(),
        );
      }
    }
  });

  it("names the rule whose statement uses 'wrap'", () => {
    expect(row(view, "wrap")?.usedBy).toContain("go.errors.1");
  });

  it("searches nothing beyond the bundle: a term the rules never say has no user", () => {
    const alone = termsView({ ...bundle, rules: [] });
    expect(alone.rows.every((r) => r.usedBy.length === 0)).toBe(true);
  });
});

describe("the bundle's own words", () => {
  it("reads only the bundle, and only fields the list names", () => {
    const recorder = recordReads(bundle);
    termsView(recorder.value, "fr");
    const reads = recorder.reads();
    expect(reads.length).toBeGreaterThan(3);
    const list = FieldListSchema.parse(
      JSON.parse(
        readFileSync(
          fileURLToPath(
            new URL("../../../../contract/fields.json", import.meta.url),
          ),
          "utf8",
        ),
      ),
    );
    expect(unlisted(list, "terms", "bundle", reads)).toEqual([]);
  });
});

describe("the terms a rule uses (gragas#1685, line 1)", () => {
  it("are the terms whose users name the rule, read from the other side", () => {
    const view = termsView(bundle);
    for (const rule of bundle.rules) {
      const fromTerms = view.rows
        .filter((r) => r.usedBy.includes(rule.id))
        .map((r) => r.slug);
      expect(termsUsedBy(bundle, rule.id)).toEqual(fromTerms);
    }
  });

  it("is empty for a rule the bundle does not hold", () => {
    expect(termsUsedBy(bundle, "no.such.rule")).toEqual([]);
  });
});
