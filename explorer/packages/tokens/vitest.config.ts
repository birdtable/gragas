import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["test/**/*.test.ts"],
    environment: "node",
    // The count reporter appends the run's test count for make explorer-test;
    // the default reporter prints the run as before.
    reporters: ["default", "../../count-reporter.ts"],
  },
});
