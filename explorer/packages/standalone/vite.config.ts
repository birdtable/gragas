// The standalone build (gragas#306): one classic script, one stylesheet,
// fonts as files, no dynamic import. The hook in page-plugin.ts only rewrites tags.
import { defineConfig } from "vite";
import { standalonePage } from "./page-plugin.ts";

export default defineConfig({
  plugins: [standalonePage()],
  base: "./",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2022",
    cssCodeSplit: false,
    assetsInlineLimit: 0,
    modulePreload: false,
    rolldownOptions: {
      output: {
        format: "iife",
        entryFileNames: "explorer.js",
        assetFileNames: (info) =>
          /\.(woff2?)$/.test(info.names[0] ?? "")
            ? "fonts/[name][extname]"
            : "explorer[extname]",
      },
    },
  },
});
