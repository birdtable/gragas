import { resolve } from "node:path";
import { defineConfig } from "vitest/config";

const contract = (path: string): string => resolve(import.meta.dirname, path);

export default defineConfig({
  resolve: {
    // The contract is read from its source, as tsconfig.json's paths say, so no
    // build of it is needed and none writes into the tree.
    alias: [
      {
        find: /^@gragas\/contract\/fixtures$/,
        replacement: contract("../contract/src/fixtures/index.ts"),
      },
      {
        find: /^@gragas\/contract$/,
        replacement: contract("../contract/src/main.ts"),
      },
    ],
  },
  test: {
    // The package's own node tests: test/ and each view's pure selectors. The parts' tests
    // render in a real browser and run from the workspace's browser project (../../vitest.config.ts).
    include: ["test/**/*.test.ts", "views/**/*.test.ts"],
    environment: "node",
    // The count reporter appends the run's test count for make explorer-test.
    reporters: ["default", "../../count-reporter.ts"],
  },
});
