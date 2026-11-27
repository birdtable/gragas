import type { PluralForms } from "./defaults.ts";

// One plural function (web.content.3): the form is the locale's own plural
// category for the count, and `other` when the table lacks that form.
export function pluralForm(
  locale: string,
  count: number,
  forms: PluralForms,
): string {
  const category = new Intl.PluralRules(locale).select(count);
  return forms[category] ?? forms.other;
}
