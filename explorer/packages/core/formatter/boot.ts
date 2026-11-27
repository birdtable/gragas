// The locale rule and the stated zone, read once at boot from one place
// (web.content.4, web.content.6). Nothing here reads the viewer's browser: the
// zone is the product's, UTC unless the page's boot data says another.
export const locales = ["en-GB", "en-US"] as const;
export type Locale = (typeof locales)[number];

export interface Boot {
  readonly locale: Locale;
  readonly timeZone: string;
  // The page's lang and dir follow the locale (web.content.6).
  readonly lang: Locale;
  readonly dir: "ltr";
}

export interface BootInput {
  readonly locale?: string | undefined;
  readonly timeZone?: string | undefined;
}

const isLocale = (value: string): value is Locale =>
  locales.some((l) => l === value);

export function readBoot(input: BootInput = {}): Boot {
  const wanted = input.locale ?? "en-GB";
  const locale: Locale = isLocale(wanted) ? wanted : "en-GB";
  const timeZone = input.timeZone ?? "UTC";
  try {
    new Intl.DateTimeFormat("en", { timeZone });
  } catch (cause) {
    throw new RangeError(`not a time zone: ${timeZone}`, { cause });
  }
  return { locale, timeZone, lang: locale, dir: "ltr" };
}
