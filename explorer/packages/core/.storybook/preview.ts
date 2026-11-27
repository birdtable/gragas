// What every story gets: the tokens' custom properties and fonts, and a theme
// the toolbar switches (the harness sets it the same way, on the root).
import "@gragas/tokens/fonts.css";
import "@gragas/tokens/tokens.css";
import "../parts/page.css";

const preview = {
  globalTypes: {
    theme: {
      description: "Theme",
      toolbar: { title: "Theme", items: ["light", "dark"], dynamicTitle: true },
    },
  },
  initialGlobals: { theme: "light" },
  decorators: [
    (
      story: () => unknown,
      context: { globals: Record<string, unknown> },
    ): unknown => {
      document.documentElement.dataset["theme"] =
        context.globals["theme"] === "dark" ? "dark" : "light";
      return story();
    },
  ],
};

export default preview;
