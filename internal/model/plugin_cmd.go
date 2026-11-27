package model

// PluginTestSchema is the schema number of the document `plugin test` answers
// with under --json.
const PluginTestSchema = 1

// The status of one check of `plugin test`.
const (
	// CheckPassed is a check that ran and found nothing wrong.
	CheckPassed = "passed"
	// CheckFailed is a check that ran and found something wrong.
	CheckFailed = "failed"
	// CheckNotRun is a check that did not run, and says why in its What.
	CheckNotRun = "not run"
	// CheckNotAsked is a check the bundle asks nothing of: it is for an
	// operation the manifest does not list in provides. It names the operation
	// in its What, and counts toward done as a passed check does.
	CheckNotAsked = "not asked"
)

// PluginCheck is one numbered check of the plugin design's 24, with where it
// stands over the bundle. A failure names the check by N, so a person reads
// which of the 24 it was.
type PluginCheck struct {
	// N is the check's number, 1 to 24.
	N int `json:"n"`
	// Name is what the check holds, in a few words.
	Name string `json:"name"`
	// Status is passed, failed or not run.
	Status string `json:"status"`
	// What is what is wrong, or why the check did not run; empty when it passed.
	What []string `json:"what,omitempty"`
}

// PluginTest is what `plugin test` found in one bundle: the checks it holds
// the bundle to, in order. A check that cannot run, such as one that calls a
// module the host refused, is not run and says why; none is faked.
type PluginTest struct {
	Schema int    `json:"schema"`
	Bundle string `json:"bundle"`
	// Cases are the folders of testdata/cases the wire checks read.
	Cases  []string      `json:"cases"`
	Checks []PluginCheck `json:"checks"`
}

// PluginChecks is the number of checks C1 to C23 that `plugin test` holds a
// bundle to; done is said only when each of them passed once. C24 holds gragas
// itself and is the host's.
const PluginChecks = 23

// Word is the closing word: findings when any check failed, not ready when
// none failed and some did not run or the list is not C1 to C23, each once,
// done only when each of those twenty-three passed or was not asked.
func (p PluginTest) Word() Word {
	notRun := false
	seen := make(map[int]bool, PluginChecks)
	for _, c := range p.Checks {
		switch c.Status {
		case CheckFailed:
			return Findings
		case CheckPassed, CheckNotAsked:
			// Only the evidence checks, C18 to C23, are for an operation the
			// manifest may leave out; not asked on any other is not a pass.
			switch {
			case c.Status == CheckNotAsked && (c.N < 18 || c.N > PluginChecks):
				notRun = true
			case c.N >= 1 && c.N <= PluginChecks:
				seen[c.N] = true
			}
		default:
			notRun = true
		}
	}
	if notRun || len(seen) != PluginChecks || len(p.Checks) != PluginChecks {
		return NotReady
	}
	return Done
}
