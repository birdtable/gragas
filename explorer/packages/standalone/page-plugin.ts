// The page's one HTML hook (gragas#306): it rewrites tags and reads
// or inlines no file. The entry becomes a classic deferred script from a file,
// the policy is a meta tag, and the three data blocks stand empty for the
// engine to fill (data-file, never an id). Trusted types are required, so no
// string reaches a script sink.
import type { Plugin } from "vite";

export const policy = [
  "default-src 'none'",
  "script-src 'self'",
  "style-src 'self'",
  "font-src 'self'",
  "img-src 'self' data:",
  "connect-src 'self'",
  "base-uri 'none'",
  "form-action 'none'",
  "object-src 'none'",
  "require-trusted-types-for 'script'",
  "trusted-types 'none'",
].join("; ");

export const dataFiles = ["bundle.json", "report.json", "proof.json"] as const;

export function standalonePage(): Plugin {
  return {
    name: "gragas-standalone-page",
    apply: "build",
    transformIndexHtml: {
      order: "post",
      handler(html) {
        const classic = html
          .replace(
            /<script\s+type="module"(?:\s+crossorigin)?\s+src=/g,
            "<script defer src=",
          )
          .replace(
            /<link rel="stylesheet" crossorigin /g,
            '<link rel="stylesheet" ',
          );
        const csp = `<meta http-equiv="Content-Security-Policy" content="${policy}" />`;
        const blocks = dataFiles
          .map(
            (file) =>
              `<script type="application/json" data-file="${file}"></script>`,
          )
          .join("\n    ");
        return classic
          .replace(/<head>/, `<head>\n    ${csp}`)
          .replace(/<\/body>/, `  ${blocks}\n  </body>`);
      },
    },
  };
}
