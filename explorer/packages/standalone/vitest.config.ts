import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    // The package's own node tests. The parts' tests render in a real browser
    // and run from the workspace's browser project (../../vitest.config.ts).
    include: ["test/**/*.test.ts"],
    environment: "node",
    // The type-check tests build a whole program.
    testTimeout: 60_000,
    // The count reporter appends the run's test count for make explorer-test.
    reporters: ["default", "../../count-reporter.ts"],
  },
});
