package audit

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestTheStagedShapeIsExcusedAtTheCommitTierAndNowhereElse: the commit calls its members with
// STAGED_ONLY 1, and a member whose status: is the guard and then a test that none of its own INPUTS is staged
// is skipped by that, on purpose. The audit reads that one shape, at the commit tier only; the same text at the
// push and at the release, with a tail (&& true, || true), with another test, with no INPUTS of the task's own
// (none, or an empty one), or with the variable on in the file, stays a finding. The control is the shape at the
// commit tier, which passes. Risk: a skip the audit cannot judge excused everywhere, or the commit's four
// members counted as held by nothing.
func TestTheStagedShapeIsExcusedAtTheCommitTierAndNowhereElse(t *testing.T) {
	t.Parallel()
	shape := `[ "{{.CLI_FORCE}}" != true ] && [ "{{.STAGED_ONLY}}" = 1 ] && [ -z "$(git diff --cached --name-only -- {{.INPUTS}})" ]`
	finds := func(tier Tier, status, inputs, fileOnly string) bool {
		task := "version: '3'\nvars:\n  STAGED_ONLY: \"" + fileOnly + "\"\n  DIRS: docs src\n  BAD: \"docs > /x\"\n  NEST: \"'{{.DIRS}}' a\"\n  NESTBAD: \"{{.BAD}}\"\ntasks:\n  " + tier.Name + ":\n    cmds:\n      - task: format\n        vars: {STAGED_ONLY: \"1\"}\n" +
			"  format:\n" + inputs + "    status:\n      - '" + status + "'\n    cmds:\n      - gofmt -l .\n"
		tier.Members = []Member{{ID: "format", Rule: "t.3", Any: []string{"gofmt"}}}
		in := TierInput{
			Repo:    fstest.MapFS{"Taskfile.yml": {Data: []byte(task)}},
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers:   Tiers{Tiers: []Tier{tier}, Rules: TierRules{Tier: "t.1", Runs: "t.2", Timeout: "t.5"}},
		}
		return len(CheckTiers(in).Findings) > 0
	}
	own := "    vars:\n      INPUTS: \"*.go\"\n"
	commit, push, release := Tier{Name: "pre-commit", Staged: true}, Tier{Name: "pre-push"}, Tier{Name: "release", Whole: true}
	for name, c := range map[string]struct {
		tier         Tier
		status, vars string
		fileOnly     string
		red          bool
	}{
		"the shape at the commit tier":          {commit, shape, own, "0", false},
		"the shape at the push tier":            {push, shape, own, "0", true},
		"the shape at the release tier":         {release, shape, own, "0", true},
		"a tail of && true at the commit tier":  {commit, shape + " && true", own, "0", true},
		"a tail of || true at the commit tier":  {commit, shape + " || true", own, "0", true},
		"another test: the unstaged diff":       {commit, strings.Replace(shape, "--cached ", "", 1), own, "0", true},
		"no INPUTS of the task's own":           {commit, shape, "", "0", true},
		"an empty INPUTS":                       {commit, shape, "    vars:\n      INPUTS: \"\"\n", "0", true},
		"quoted globs":                          {commit, shape, "    vars:\n      INPUTS: \"'*.go' '*go.mod' Taskfile.yml\"\n", "0", false},
		"a redirect in INPUTS":                  {commit, shape, "    vars:\n      INPUTS: \"*.go >sink.log\"\n", "0", true},
		"a second command in INPUTS":            {commit, shape, "    vars:\n      INPUTS: \"*.go; true\"\n", "0", true},
		"a substitution in INPUTS":              {commit, shape, "    vars:\n      INPUTS: \"$(true)\"\n", "0", true},
		"an unclosed quote in INPUTS":           {commit, shape, "    vars:\n      INPUTS: \"'*.go\"\n", "0", true},
		"INPUTS by a var of the file":           {commit, shape, "    vars:\n      INPUTS: \"{{.DIRS}}\"\n", "0", false},
		"INPUTS by a var that holds a redirect": {commit, shape, "    vars:\n      INPUTS: \"{{.BAD}}\"\n", "0", true},
		"INPUTS by a var that is not there":     {commit, shape, "    vars:\n      INPUTS: \"{{.NOPE}}\"\n", "0", true},
		"INPUTS by a var by a var":              {commit, shape, "    vars:\n      INPUTS: \"{{.NEST}} x\"\n", "0", false},
		"INPUTS by a var by a bad var":          {commit, shape, "    vars:\n      INPUTS: \"{{.NESTBAD}}\"\n", "0", true},
		"the variable on in the file":           {commit, shape, own, "1", true},
		"no force test first":                   {commit, strings.Replace(shape, `[ "{{.CLI_FORCE}}" != true ] && `, "", 1), own, "0", true},
	} {
		if got := finds(c.tier, c.status, c.vars, c.fileOnly); got != c.red {
			t.Errorf("%s: a finding = %v, want %v", name, got, c.red)
		}
	}
}

// TestAnInputsVarThatAnyTaskOrCallSetsIsNotTheFilesAndIsRefused: go-task ranks the vars
// of a call and of the task itself above the file's, so a {{.NAME}} in INPUTS that either sets is not the value
// the audit read from the file's vars; the one spliced into the shell is theirs ("*.go; true"). Each task is read
// once, so the first caller would decide for every later one: the tier calling format plainly and then again with
// DIRS passed was clean, and the reverse order was red. The reference is a finding whatever the order, the value,
// the call and the task that sets it, in this file or an include; the control is the same reference to a var of the
// file that nothing sets, and a call passing another name, which are excused.
func TestAnInputsVarThatAnyTaskOrCallSetsIsNotTheFilesAndIsRefused(t *testing.T) {
	t.Parallel()
	shape := `[ "{{.CLI_FORCE}}" != true ] && [ "{{.STAGED_ONLY}}" = 1 ] && [ -z "$(git diff --cached --name-only -- {{.INPUTS}})" ]`
	finds := func(calls, own, more string) bool {
		task := "version: '3'\nvars:\n  STAGED_ONLY: \"0\"\n  DIRS: docs\ntasks:\n  pre-commit:\n    cmds:\n" + calls +
			"  format:\n    vars:\n" + own + "      INPUTS: \"{{.DIRS}}\"\n    status:\n      - '" + shape + "'\n    cmds:\n      - gofmt -l .\n" + more
		in := TierInput{
			Repo:    fstest.MapFS{"Taskfile.yml": {Data: []byte(task)}},
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers: Tiers{Tiers: []Tier{{Name: "pre-commit", Staged: true, Members: []Member{{ID: "format", Rule: "t.3", Any: []string{"gofmt"}}}}},
				Rules: TierRules{Tier: "t.1", Runs: "t.2", Timeout: "t.5"}},
		}
		return len(CheckTiers(in).Findings) > 0
	}
	plain := "      - task: format\n        vars: {STAGED_ONLY: \"1\"}\n"
	with := func(extra string) string {
		return "      - task: format\n        vars: {STAGED_ONLY: \"1\", " + extra + "}\n"
	}
	other := "  other:\n    cmds:\n      - task: format\n        vars: {DIRS: \"*.go; true\"}\n"
	for name, c := range map[string]struct {
		calls, own, more string
		red              bool
	}{
		"the file's own var, nothing sets it":       {plain, "", "", false},
		"a call passing another name, and not this": {with("OTHER: x"), "", "", false},
		"a call passing it an injection":            {with("DIRS: \"*.go; true\""), "", "", true},
		"a call passing it a harmless value":        {with("DIRS: src"), "", "", true},
		"the task setting it to an injection":       {plain, "      DIRS: \"*.go; true\"\n", "", true},
		"the task setting it to a harmless value":   {plain, "      DIRS: src\n", "", true},
		"plain call, then the call that passes it":  {plain + with("DIRS: \"*.go; true\""), "", "", true},
		"the call that passes it, then plain call":  {with("DIRS: \"*.go; true\"") + plain, "", "", true},
		"another task's call that passes it":        {plain, "", other, true},
		"another task's call, the tier not calling": {plain, "", other + "  unused:\n    cmds: [echo]\n", true},
	} {
		if got := finds(c.calls, c.own, c.more); got != c.red {
			t.Errorf("%s: a finding = %v, want %v", name, got, c.red)
		}
	}
}

// TestACallUnderDeferIsReadLikeAnyOtherForTheInputsVarAndTheGuardVar: go-task runs
// a call under defer: with its own vars, so `- defer: {task: format, vars: {DIRS: "*.go; true"}}` splices that
// value into the status of format, and sets the guard's variable as a call of the list does. The vars of a call
// nested under defer: are read where those of a deps or cmds entry are: for the INPUTS var (setAnywhere) and for the
// guard's variable (the tier's reach). Each is red in both orders of the plain call and the deferred one, and each
// is red when the defer: read is taken out; the control is the same deferred call passing another name.
func TestACallUnderDeferIsReadLikeAnyOtherForTheInputsVarAndTheGuardVar(t *testing.T) {
	t.Parallel()
	shape := `[ "{{.CLI_FORCE}}" != true ] && [ "{{.STAGED_ONLY}}" = 1 ] && [ -z "$(git diff --cached --name-only -- {{.INPUTS}})" ]`
	staged := func(calls string) bool {
		task := "version: '3'\nvars:\n  STAGED_ONLY: \"0\"\n  DIRS: docs\ntasks:\n  pre-commit:\n    cmds:\n" + calls +
			"  format:\n    vars:\n      INPUTS: \"{{.DIRS}}\"\n    status:\n      - '" + shape + "'\n    cmds:\n      - gofmt -l .\n"
		in := TierInput{
			Repo:    fstest.MapFS{"Taskfile.yml": {Data: []byte(task)}},
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers: Tiers{Tiers: []Tier{{Name: "pre-commit", Staged: true, Members: []Member{{ID: "format", Rule: "t.3", Any: []string{"gofmt"}}}}},
				Rules: TierRules{Tier: "t.1", Runs: "t.2", Timeout: "t.5"}},
		}
		return len(CheckTiers(in).Findings) > 0
	}
	plain := "      - task: format\n        vars: {STAGED_ONLY: \"1\"}\n"
	deferred := func(extra string) string {
		return "      - defer: {task: format, vars: {STAGED_ONLY: \"1\"" + extra + "}}\n"
	}
	for name, c := range map[string]struct {
		calls string
		red   bool
	}{
		"plain, then a deferred call passing DIRS": {plain + deferred(", DIRS: \"*.go; true\""), true},
		"a deferred call passing DIRS, then plain": {deferred(", DIRS: \"*.go; true\"") + plain, true},
		"a deferred call passing another name":     {plain + deferred(", OTHER: x"), false},
		"a deferred call in a middle task":         {plain + "      - task: mid\n  mid:\n    cmds:\n" + deferred(", DIRS: \"*.go; true\"") + "  mid2:\n    cmds: [echo]\n", true},
	} {
		if got := staged(c.calls); got != c.red {
			t.Errorf("INPUTS, %s: a finding = %v, want %v", name, got, c.red)
		}
	}
	// The guard's variable: lint is held by a declared skip at the push, and a deferred call that passes it 1 turns
	// the skip on, whatever the order.
	const head = "version: '3'\nvars:\n  QUICK: \"0\"\ntasks:\n  lint:\n    status:\n      - '" + `[ "{{.CLI_FORCE}}" != true ] && [ "{{.QUICK}}" = 1 ]` + "'\n    cmds: [go vet ./...]\n"
	callLint := "      - task: lint\n"
	deferQuick := func(v string) string { return "      - defer: {task: lint, vars: {QUICK: \"" + v + "\"}}\n" }
	for name, c := range map[string]struct {
		tasks string
		found bool // the member is counted: no skip is found
	}{
		"lint plainly, then deferred with QUICK 1":                         {"  pre-push:\n    cmds:\n" + callLint + deferQuick("1"), false},
		"deferred with QUICK 1, then lint plainly":                         {"  pre-push:\n    cmds:\n" + deferQuick("1") + callLint, false},
		"lint plainly, then deferred with QUICK 0":                         {"  pre-push:\n    cmds:\n" + callLint + deferQuick("0"), true},
		"lint plainly, then a deferred call in a middle task with QUICK 1": {"  pre-push:\n    cmds:\n" + callLint + "      - task: mid\n  mid:\n    cmds:\n" + deferQuick("1"), false},
	} {
		repo := fstest.MapFS{"Taskfile.yml": {Data: []byte(head + c.tasks)}}
		if found, skips := tierFinds(t, repo, Tier{Name: "pre-push"}); found != c.found {
			t.Errorf("guard, %s: member counted = %v (skips %v), want %v", name, found, skips, c.found)
		}
	}
}
