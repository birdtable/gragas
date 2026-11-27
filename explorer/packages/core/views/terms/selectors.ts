// The terms view's selectors: every term of the bundle with its
// literal, the definition in the chosen language, its translations with the
// stale mark, and the rules that use it. They read the one parsed bundle and
// take nothing else (web.structure.5). The join of a term to its rules is the
// one the relations view draws: a rule uses a term when the term's slug, as a
// word, is in the rule's statement or reasoning; the page searches nothing
// beyond the bundle. A translation is shown as the bundle states it, never
// made here; a stale one is a notice, never a finding (S16). A value the bundle
// does not state is a dash with its reason (web.states.5).
import type { BundleDocument } from "@gragas/contract";
import type * as z from "zod";
import { dashable, type Dashable } from "../../messages/index.ts";

type Bundle = z.output<typeof BundleDocument>;

/** Which text the definition column holds, and why. */
export type DefinitionMark =
  /** The authoritative language was chosen: the text is the term's own. */
  | "authoritative"
  /** A translation in the chosen language, current. */
  | "translated"
  /** A translation in the chosen language whose authoritative text has changed. */
  | "stale"
  /** No translation in the chosen language: the authoritative text stands in. */
  | "fallback";

export interface TermTranslation {
  readonly language: string;
  readonly text: string;
  readonly stale: boolean;
}

export interface TermDefinition {
  readonly text: string;
  readonly mark: DefinitionMark;
}

export interface TermRow {
  readonly slug: string;
  readonly literal: Dashable<string>;
  readonly definition: TermDefinition;
  /** Every translation of the definition, by language. */
  readonly translations: readonly TermTranslation[];
  /** The ids of the rules that use the term, in the bundle's order. */
  readonly usedBy: readonly string[];
}

export interface TermsView {
  /** The rulebook's authoritative language. */
  readonly authoritative: string;
  /** The language the definitions are shown in (the authoritative one by default). */
  readonly chosen: string;
  /** The authoritative language, then every language a translation exists in. */
  readonly languages: readonly string[];
  readonly rows: readonly TermRow[];
  readonly translationCount: number;
  readonly staleCount: number;
}

/** The field of a term a translation renders. */
const definitionField = "definition";

function wordsOf(slug: string): RegExp {
  const escaped = slug
    .replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
    .replace(/-/g, "[- ]");
  return new RegExp(`(?<![A-Za-z0-9])${escaped}(?![A-Za-z0-9])`, "i");
}

/** The ids of the rules that use each term, joined through one map by id. */
function usage(bundle: Bundle): ReadonlyMap<string, readonly string[]> {
  const prose = new Map<string, string>();
  for (const rule of bundle.rules) {
    prose.set(rule.id, `${rule.statement} ${rule.reasoning}`);
  }
  const out = new Map<string, readonly string[]>();
  for (const term of bundle.terms) {
    const words = wordsOf(term.slug);
    const ids: string[] = [];
    for (const [id, text] of prose) {
      if (words.test(text)) ids.push(id);
    }
    out.set(term.slug, ids);
  }
  return out;
}

function definitionIn(
  text: string,
  language: string,
  authoritative: string,
  translations: readonly TermTranslation[],
): TermDefinition {
  if (language === authoritative) return { text, mark: "authoritative" };
  const found = translations.find((t) => t.language === language);
  if (found === undefined) return { text, mark: "fallback" };
  return { text: found.text, mark: found.stale ? "stale" : "translated" };
}

/** Every term of the bundle, its definition in `language`, and its usage. */
export function termsView(bundle: Bundle, language?: string): TermsView {
  const authoritative = bundle.identity.language;
  const used = usage(bundle);
  const rows = bundle.terms.map((term): TermRow => {
    const translations = term.translations
      .filter((t) => t.field === definitionField)
      .map((t) => ({ language: t.language, text: t.text, stale: t.stale }))
      .sort((a, b) => a.language.localeCompare(b.language));
    return {
      slug: term.slug,
      literal: dashable(
        term.literal === "" ? null : term.literal,
        "absent.none-recorded",
      ),
      definition: definitionIn(
        term.definition,
        language ?? authoritative,
        authoritative,
        translations,
      ),
      translations,
      usedBy: used.get(term.slug) ?? [],
    };
  });
  const spoken = new Set(
    rows.flatMap((r) => r.translations.map((t) => t.language)),
  );
  spoken.delete(authoritative);
  const all = rows.flatMap((r) => r.translations);
  return {
    authoritative,
    chosen: language ?? authoritative,
    languages: [authoritative, ...[...spoken].sort()],
    rows,
    translationCount: all.length,
    staleCount: all.filter((t) => t.stale).length,
  };
}

/**
 * The slugs of the terms a rule's statement or reasoning uses, in the bundle's
 * order: the join `usage` draws, read from the rule's side, so the rule view
 * links each one back to its entry in the terms view (gragas#1685, line 1).
 */
export function termsUsedBy(bundle: Bundle, ruleId: string): readonly string[] {
  const rule = bundle.rules.find((r) => r.id === ruleId);
  if (rule === undefined) return [];
  const prose = `${rule.statement} ${rule.reasoning}`;
  return bundle.terms
    .filter((term) => wordsOf(term.slug).test(prose))
    .map((term) => term.slug);
}
