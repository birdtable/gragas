// The part of the explorer's content policy the fonts need (web.tokens.*):
// fonts load from the page's own origin and from nowhere else, so
// no font host is named. A page that serves the explorer puts this directive in
// its Content-Security-Policy; test/fonts.test.ts holds it to 'self' alone and
// holds every url() of the package's styles to a path inside the package.
export const fontSrc = "font-src 'self'";
