// The explorer-test member's count (gates.measures.6): a vitest
// reporter that, at the end of a package's run, appends one line to the file
// GRAGAS_TEST_COUNT_FILE names, the number of tests the run reported on, passed,
// failed or skipped. `task explorer-test` sums the lines, one a package, and
// ends on `count: <n> tests`, which the gate runner holds to the member's floor
// in gate/counts.yaml. With the variable unset, as in a run by hand, it writes
// nothing. The default reporter still prints the run.
import { appendFileSync } from "node:fs";
import type { Reporter, TestModule } from "vitest/node";

export default class CountReporter implements Reporter {
  onTestRunEnd(testModules: readonly TestModule[]): void {
    const file = process.env["GRAGAS_TEST_COUNT_FILE"];
    if (!file) {
      return;
    }
    let n = 0;
    for (const m of testModules) {
      n += Array.from(m.children.allTests()).length;
    }
    appendFileSync(file, `${n}\n`);
  }
}
