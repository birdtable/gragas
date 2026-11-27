// The render harness (react.testing.1): the one entry through which
// every part spec and every story renders. There is no provider: a part takes
// plain props, its words come from the message module's defaults (`words`
// below is that object), and the theme is the tokens' custom properties under
// the root's data-theme attribute.
import type { ReactElement } from "react";
import { flushSync } from "react-dom";
import { createRoot } from "react-dom/client";
import "@gragas/tokens/fonts.css";
import "@gragas/tokens/tokens.css";
import "../parts/page.css";
import { defaultMessages } from "../messages/index.ts";

export const themes = ["light", "dark"] as const;
export type Theme = (typeof themes)[number];

/** The words a part says by default, as the message module's defaults object. */
export const words = defaultMessages;

export type RenderOptions = Readonly<{ theme?: Theme }>;

export type Rendered = Readonly<{
  container: HTMLElement;
  theme: Theme;
  /** Waits for the frames a portal or a focus move needs to settle. */
  settled: () => Promise<void>;
  rerender: (ui: ReactElement) => void;
  unmount: () => void;
}>;

const frame = (): Promise<void> =>
  new Promise((resolve) => {
    requestAnimationFrame(() => {
      resolve();
    });
  });

export function render(
  ui: ReactElement,
  options: RenderOptions = {},
): Rendered {
  const theme = options.theme ?? "light";
  document.documentElement.dataset["theme"] = theme;
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  flushSync(() => {
    root.render(ui);
  });
  return {
    container,
    theme,
    settled: async () => {
      await frame();
      await frame();
    },
    rerender: (next) => {
      flushSync(() => {
        root.render(next);
      });
    },
    unmount: () => {
      flushSync(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}
