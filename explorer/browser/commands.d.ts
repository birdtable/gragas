// The commands vitest.config.ts gives the browser project: node-side code a
// test in the page calls through `commands` (ts.modules.2: a module block
// belongs in an ambient declaration file).
import "vitest/browser";

declare module "vitest/browser" {
  interface BrowserCommands {
    /** A port nothing listens on. */
    freePort: () => Promise<number>;
  }
}
