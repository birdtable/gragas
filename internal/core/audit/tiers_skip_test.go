package audit

import (
	"strings"
	"testing"
	"testing/fstest"
)

// tierFinds reads repo's tier, which asks for one vet member, and answers whether the member is counted and
// the findings that name a skip.
func tierFinds(t *testing.T, repo fstest.MapFS, tier Tier) (found bool, skips []string) {
	t.Helper()
	tier.Members = []Member{{ID: "vet", Rule: "t.3", All: []string{"go vet"}}}
	in := TierInput{
		Repo:    repo,
		Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
		Tiers:   Tiers{Tiers: []Tier{tier}, Rules: TierRules{Tier: "t.1", Runs: "t.2"}},
	}
	found = true
	for _, f := range CheckTiers(in).Findings {
		switch {
		case strings.Contains(f.Missing, "has no vet member"):
			found = false
		case f.Rule == "t.2":
			skips = append(skips, f.Key)
		}
	}
	return found, skips
}

const guard = `[ "{{.CLI_FORCE}}" != true ] && [ "{{.SKIP_UNCHANGED}}" = 1 ] && git diff --quiet origin/main -- .`

// forcedTier is the tier task that refuses to run without --force.
const forcedTier = "    preconditions:\n      - sh: '[ \"{{.CLI_FORCE}}\" = true ]'\n"

// withFile is repo with one more file.
func withFile(repo fstest.MapFS, name, text string) fstest.MapFS {
	repo[name] = &fstest.MapFile{Data: []byte(text)}
	return repo
}

// withTaskVars is repo with a vars: block on the vet task, which sets what the lines say.
func withTaskVars(repo fstest.MapFS, line string) fstest.MapFS {
	text := string(repo["Taskfile.yml"].Data)
	repo["Taskfile.yml"] = &fstest.MapFile{Data: []byte(strings.Replace(text, "  vet:\n", "  vet:\n    vars:\n      "+line+"\n", 1))}
	return repo
}

// TestADeclaredSkipIsHeldByTheCommitAndThePushAndByNoOtherTier: a status: whose first line is the CLI_FORCE guard, with an enabling
// variable that the file declares as a literal string that is not 1 or true and holds no template, and that the
// task does not set again, is a declared skip, so the member behind it counts at the commit and the push. Any
// other status:, a guard that is on by default or whose variable the audit cannot read (a command, a template,
// a number, one the file lacks, one the task sets again), one that does not put CLI_FORCE first, and the same
// guard in the release (whole) do not. Under a tier task that refuses to run without --force, only a status:
// that is the guard is excused: a plain status:, sources: or generates: is judged by go-task's own checks for a
// called task when gentle force is on, which a .taskrc.yml or a .env turns on. Risk: a skip the audit cannot
// read passing as a member that runs.
func TestADeclaredSkipIsHeldByTheCommitAndThePushAndByNoOtherTier(t *testing.T) {
	t.Parallel()
	file := func(vars, status, tierTask string) fstest.MapFS {
		return fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\n" + vars + "tasks:\n  vet:\n    status:\n      - '" + status + "'\n    cmds: [go vet ./...]\n  pre-push:\n    deps: [vet]\n" + tierTask)}}
	}
	push, release := Tier{Name: "pre-push"}, Tier{Name: "pre-push", Whole: true}
	const off, on = "vars:\n  SKIP_UNCHANGED: \"0\"\n", "vars:\n  SKIP_UNCHANGED: \"1\"\n"
	taskrc := "experiments:\n  GENTLE_FORCE: 1\n"
	for name, c := range map[string]struct {
		repo  fstest.MapFS
		tier  Tier
		found bool
	}{
		"declared, off by default, in a push":                  {file(off, guard, ""), push, true},
		"the variable on by default":                           {file(on, guard, ""), push, false},
		"the variable not in the file":                         {file("", guard, ""), push, false},
		"the variable a command (sh:)":                         {file("vars:\n  SKIP_UNCHANGED: {sh: echo 1}\n", guard, ""), push, false},
		"the variable a template":                              {file("vars:\n  ONE: \"1\"\n  SKIP_UNCHANGED: \"{{.ONE}}\"\n", guard, ""), push, false},
		"the variable a number, not a string":                  {file("vars:\n  SKIP_UNCHANGED: 0\n", guard, ""), push, false},
		"the variable set again by the task":                   {withTaskVars(file(off, guard, ""), `SKIP_UNCHANGED: "1"`), push, false},
		"the variable set again to 0 by the task":              {withTaskVars(file(off, guard, ""), `SKIP_UNCHANGED: "0"`), push, false},
		"a status of another shape":                            {file(off, `test -f stamp`, ""), push, false},
		"a guard with CLI_FORCE not first":                     {file(off, `[ "{{.SKIP_UNCHANGED}}" = 1 ] && [ "{{.CLI_FORCE}}" != true ]`, ""), push, false},
		"a guard with no enabling variable":                    {file(off, `[ "{{.CLI_FORCE}}" != true ] && git diff --quiet`, ""), push, false},
		"declared, in a whole tier":                            {file(off, guard, ""), release, false},
		"whole, held to --force, the guard":                    {file(off, guard, forcedTier), release, true},
		"whole, held to --force, the guard, no declared var":   {file("", guard, forcedTier), release, true},
		"whole, held to --force, the guard, a .taskrc.yml":     {withFile(file(off, guard, forcedTier), ".taskrc.yml", taskrc), release, true},
		"whole, held to --force, a plain status:":              {file(off, `test -f stamp`, forcedTier), release, false},
		"whole, held to --force, a status: of true":            {file(off, `true`, forcedTier), release, false},
		"whole, a plain status:, a .taskrc.yml":                {withFile(file(off, `test -f stamp`, forcedTier), ".taskrc.yml", taskrc), release, false},
		"whole, a plain status:, a .env":                       {withFile(file(off, `test -f stamp`, forcedTier), ".env", "TASK_X_GENTLE_FORCE=1\n"), release, false},
		"whole, a precondition that is not --force, the guard": {file(off, guard, "    preconditions:\n      - sh: 'true'\n"), release, false},
	} {
		found, skips := tierFinds(t, c.repo, c.tier)
		if found != c.found {
			t.Errorf("%s: member counted = %v (skips %v), want %v", name, found, skips, c.found)
		}
	}
}

// TestATaskLevelTimeoutHoldsNothingAndACommandsDoes: go-task 3.54.0 reads a timeout on a
// command and on a dep and ignores a task's own timeout: without a word (a task with timeout: 1s ran its
// command for 3 s and ended green), so a member that starts a process tree and has only a task-level timeout:
// is a finding. Risk: ten members that no timeout holds counted as held.
func TestATaskLevelTimeoutHoldsNothingAndACommandsDoes(t *testing.T) {
	t.Parallel()
	finds := func(task string) bool {
		repo := fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\ntasks:\n  pre-push:\n" + task)}}
		in := TierInput{
			Repo:    repo,
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers: Tiers{Tiers: []Tier{{Name: "pre-push", Members: []Member{{ID: "vet", Rule: "t.3", All: []string{"go vet"}, Tree: true}}}},
				Rules: TierRules{Tier: "t.1", Runs: "t.2", Timeout: "t.5"}},
		}
		for _, f := range CheckTiers(in).Findings {
			if f.Rule == "t.5" {
				return true
			}
		}
		return false
	}
	for name, c := range map[string]struct {
		task string
		red  bool
	}{
		"no timeout":                   {"    cmds: [go vet ./...]\n", true},
		"a task-level timeout":         {"    timeout: 5m\n    cmds: [go vet ./...]\n", true},
		"a command's own timeout":      {"    cmds:\n      - {cmd: go vet ./..., timeout: 5m}\n", false},
		"a task and a command timeout": {"    timeout: 5m\n    cmds:\n      - {cmd: go vet ./..., timeout: 5m}\n", false},
		// A limit on a call, where go-task 3.54.0 reads it as on a command: the member in the called
		// task counts as limited, and a call without one does not.
		"a call's own timeout":               {"    cmds:\n      - {task: vet, timeout: 5m}\n  vet:\n    cmds: [go vet ./...]\n", false},
		"a call with no timeout":             {"    cmds:\n      - {task: vet}\n  vet:\n    cmds: [go vet ./...]\n", true},
		"a task-level timeout on the callee": {"    cmds:\n      - {task: vet}\n  vet:\n    timeout: 5m\n    cmds: [go vet ./...]\n", true},
	} {
		if got := finds(c.task); got != c.red {
			t.Errorf("%s: a timeout finding = %v, want %v", name, got, c.red)
		}
	}
}

// TestAGuardWithAnythingAfterItThatEndsTheLineGreenIsNotDeclared: the guard is read
// as one statement whose top node is an && chain with its two tests leftmost. A tail inside the chain may stay,
// since the guard false keeps the whole line false; `|| true`, `; true`, a second line and a & do not, since
// the line then ends green (or runs on) when the guard is false. go-task 3.54.0 answers "up to date" for the
// first of these at the push, and for all of them in the release when gentle force is on. Risk: a member that
// is skipped counted as one that runs.
func TestAGuardWithAnythingAfterItThatEndsTheLineGreenIsNotDeclared(t *testing.T) {
	t.Parallel()
	const off = "vars:\n  SKIP_UNCHANGED: \"0\"\n"
	const head = `[ "{{.CLI_FORCE}}" != true ] && [ "{{.SKIP_UNCHANGED}}" = 1 ]`
	repo := func(block, tierTask string) fstest.MapFS {
		lines := "        " + strings.ReplaceAll(block, "\n", "\n        ") + "\n"
		return fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\n" + off + "tasks:\n  vet:\n    status:\n      - |\n" + lines + "    cmds: [go vet ./...]\n  pre-push:\n    deps: [vet]\n" + tierTask)}}
	}
	push, release := Tier{Name: "pre-push"}, Tier{Name: "pre-push", Whole: true}
	taskrc := "experiments:\n  GENTLE_FORCE: 1\n"
	for name, c := range map[string]struct {
		line  string
		found bool
	}{
		"the guard alone":                   {head, true},
		"the guard and a command":           {head + ` && git diff --quiet origin/main -- .`, true},
		"the guard and a group that has ||": {head + ` && { test -f a || test -f b; }`, true},
		"the guard, || true":                {head + ` || true`, false},
		"the guard, a command, || true":     {head + ` && test -f a || true`, false},
		"the guard, ; true":                 {head + ` ; true`, false},
		"the guard, a second line":          {head + "\ntrue", false},
		"the guard, in the background":      {head + ` &`, false},
		"the guard, negated":                {`! ` + head, false},
		"the guard with a redirect":         {head + ` > sink.log`, false},
		"the tests the other way round":     {`[ "{{.SKIP_UNCHANGED}}" = 1 ] && [ "{{.CLI_FORCE}}" != true ]`, false},
		"a command before the guard":        {`true && ` + head, false},
		"a line that does not parse":        {head + ` && (`, false},
	} {
		if found, skips := tierFinds(t, repo(c.line, ""), push); found != c.found {
			t.Errorf("a push, %s: member counted = %v (skips %v), want %v", name, found, skips, c.found)
		}
		// In the release, held to --force, with gentle force on: only a line that is the guard is excused.
		gentle := withFile(repo(c.line, forcedTier), ".taskrc.yml", taskrc)
		if found, skips := tierFinds(t, gentle, release); found != c.found {
			t.Errorf("a gentle-force release, %s: member counted = %v (skips %v), want %v", name, found, skips, c.found)
		}
	}
}

// TestACallThatPassesTheGuardsVariableCanTurnTheSkipOn: go-task ranks a call's
// vars: above the file's and the task's, so a dep, a task: call or a call through a middle task that passes the
// guard's variable as 1 turns the skip on at the push ("vet is up to date"), whatever the file declares. The
// declared skip is off only while no call on the way passes a value that may be 1: a literal string that is
// not 1 or true and holds no template is safe, and so is the forward of the variable itself ({{.NAME}}),
// which is the file's value or one an earlier call passed, and that call is judged on its own. Risk: a member
// that go-task skips at the push counted as one that runs.
func TestACallThatPassesTheGuardsVariableCanTurnTheSkipOn(t *testing.T) {
	t.Parallel()
	const head = "version: '3'\nvars:\n  SKIP_UNCHANGED: \"0\"\ntasks:\n  vet:\n    status:\n      - '" + `[ "{{.CLI_FORCE}}" != true ] && [ "{{.SKIP_UNCHANGED}}" = 1 ]` + "'\n    cmds: [go vet ./...]\n"
	repo := func(tasks string) fstest.MapFS { return fstest.MapFS{"Taskfile.yml": {Data: []byte(head + tasks)}} }
	push := Tier{Name: "pre-push"}
	for name, c := range map[string]struct {
		tasks string
		found bool
	}{
		"control: a plain dep":                               {"  pre-push:\n    deps: [vet]\n", true},
		"a dep passes 1":                                     {"  pre-push:\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"1\"}}\n", false},
		"a dep passes true":                                  {"  pre-push:\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"true\"}}\n", false},
		"a task: call passes 1":                              {"  pre-push:\n    cmds:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"1\"}}\n", false},
		"a call through a middle task passes 1":              {"  pre-push:\n    deps: [mid]\n  mid:\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"1\"}}\n", false},
		"a call to a middle task passes 1":                   {"  pre-push:\n    deps:\n      - {task: mid, vars: {SKIP_UNCHANGED: \"1\"}}\n  mid:\n    deps: [vet]\n", false},
		"a call passes a template":                           {"  pre-push:\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"{{.OTHER}}\"}}\n", false},
		"a call passes a command (sh:)":                      {"  pre-push:\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: {sh: echo 1}}}\n", false},
		"a call passes a number":                             {"  pre-push:\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: 0}}\n", false},
		"a call passes 0 as a literal string":                {"  pre-push:\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"0\"}}\n", true},
		"a call forwards the variable itself":                {"  pre-push:\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"{{.SKIP_UNCHANGED}}\"}}\n", true},
		"a call passes another name as 1":                    {"  pre-push:\n    deps:\n      - {task: vet, vars: {OTHER: \"1\"}}\n", true},
		"a middle task sets it to 1 and forwards it":         {"  pre-push:\n    deps: [mid]\n  mid:\n    vars: {SKIP_UNCHANGED: \"1\"}\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"{{.SKIP_UNCHANGED}}\"}}\n", false},
		"the tier task sets it to 1 and forwards it":         {"  pre-push:\n    vars: {SKIP_UNCHANGED: \"1\"}\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"{{.SKIP_UNCHANGED}}\"}}\n", false},
		"a middle task sets it by a command and forwards it": {"  pre-push:\n    deps: [mid]\n  mid:\n    vars: {SKIP_UNCHANGED: {sh: echo 1}}\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"{{.SKIP_UNCHANGED}}\"}}\n", false},
		"a middle task sets it to 0 and forwards it":         {"  pre-push:\n    deps: [mid]\n  mid:\n    vars: {SKIP_UNCHANGED: \"0\"}\n    deps:\n      - {task: vet, vars: {SKIP_UNCHANGED: \"{{.SKIP_UNCHANGED}}\"}}\n", true},
		"a call passes 1 after the one that reaches":         {"  pre-push:\n    deps: [vet]\n    cmds:\n      - {task: mid, vars: {SKIP_UNCHANGED: \"1\"}}\n  mid:\n    cmds: [echo ok]\n", false},
	} {
		if found, skips := tierFinds(t, repo(c.tasks), push); found != c.found {
			t.Errorf("%s: member counted = %v (skips %v), want %v", name, found, skips, c.found)
		}
	}
}

// TestATimeoutOnACallHidesNothingAndAnotherKeyOnItStillDoes: a call that carries only a timeout
// still runs its task on every host and cannot swallow its failure, so the member in it is held as on a call
// without one; a call that carries a key that can stop it or swallow its failure beside the timeout is not.
// Risk: the limits go on the calls, where go-task reads them, and every tier member is then found not to run.
func TestATimeoutOnACallHidesNothingAndAnotherKeyOnItStillDoes(t *testing.T) {
	t.Parallel()
	runs := func(entry string) bool {
		task := "    cmds:\n      - " + entry + "\n  vet:\n    cmds:\n      - {cmd: go vet ./..., timeout: 5m}\n"
		repo := fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\ntasks:\n  pre-push:\n" + task)}}
		in := TierInput{
			Repo:    repo,
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers: Tiers{Tiers: []Tier{{Name: "pre-push", Members: []Member{{ID: "vet", Rule: "t.3", All: []string{"go vet"}, Tree: true}}}},
				Rules: TierRules{Tier: "t.1", Runs: "t.2", Timeout: "t.5"}},
		}
		for _, f := range CheckTiers(in).Findings {
			if f.Rule == "t.2" {
				return true
			}
		}
		return false
	}
	for name, c := range map[string]struct {
		entry string
		red   bool
	}{
		"a call":                           {"{task: vet}", false},
		"a call with a timeout":            {"{task: vet, timeout: 5m}", false},
		"a call with a timeout and a skip": {"{task: vet, timeout: 5m, platforms: [linux]}", true},
		"a call with a timeout and an if":  {"{task: vet, timeout: 5m, if: 'true'}", true},
	} {
		if got := runs(c.entry); got != c.red {
			t.Errorf("%s: a finding that the member does not run = %v, want %v", name, got, c.red)
		}
	}
}

// TestACallThatPassesTheGuardsVariableAnywhereHoldsTheSkipWhateverTheCallOrder: a task is read once, so the first caller decided for every later one: lint called plainly and then with
// QUICK: "1" counted the member, though go-task runs the second call with the skip on; the reverse order did not.
// A call anywhere in the files that passes the guard's variable a value that may be 1 holds the skip, in either
// order, in a task the tier reaches through another (one it does not reach runs nothing of the tier and holds
// nothing: per tier, since the commit passes a shared task its guard on where the push does not); a call that passes another name,
// 0 as a literal or the forward of the variable itself holds nothing. Each case is red when the whole-file reading
// (unsafeAnywhere) is taken out.
func TestACallThatPassesTheGuardsVariableAnywhereHoldsTheSkipWhateverTheCallOrder(t *testing.T) {
	t.Parallel()
	const head = "version: '3'\nvars:\n  QUICK: \"0\"\ntasks:\n  lint:\n    status:\n      - '" + `[ "{{.CLI_FORCE}}" != true ] && [ "{{.QUICK}}" = 1 ]` + "'\n    cmds: [go vet ./...]\n"
	plain, one := "      - task: lint\n", "      - {task: lint, vars: {QUICK: \"1\"}}\n"
	push := Tier{Name: "pre-push"}
	for name, c := range map[string]struct {
		tasks string
		found bool // the member is counted: no skip is found
	}{
		"control: lint called plainly":                  {"  pre-push:\n    cmds:\n" + plain, true},
		"lint plainly, then with QUICK 1":               {"  pre-push:\n    cmds:\n" + plain + one, false},
		"lint with QUICK 1, then plainly":               {"  pre-push:\n    cmds:\n" + one + plain, false},
		"a dep plainly, then a cmds entry with QUICK 1": {"  pre-push:\n    deps: [lint]\n    cmds:\n" + one, false},
		"another task, not reached, passes QUICK 1":     {"  pre-push:\n    cmds:\n" + plain + "  other:\n    cmds:\n" + one, true},
		"another task, reached by a middle one":         {"  pre-push:\n    cmds:\n" + plain + "      - task: other\n  other:\n    cmds:\n" + one, false},
		"lint plainly, then QUICK 0 as a literal":       {"  pre-push:\n    cmds:\n" + plain + "      - {task: lint, vars: {QUICK: \"0\"}}\n", true},
		"lint plainly, then another name as 1":          {"  pre-push:\n    cmds:\n" + plain + "      - {task: lint, vars: {OTHER: \"1\"}}\n", true},
		"lint plainly, then the forward of QUICK":       {"  pre-push:\n    cmds:\n" + plain + "      - {task: lint, vars: {QUICK: \"{{.QUICK}}\"}}\n", true},
	} {
		repo := fstest.MapFS{"Taskfile.yml": {Data: []byte(head + c.tasks)}}
		if found, skips := tierFinds(t, repo, push); found != c.found {
			t.Errorf("%s: member counted = %v (skips %v), want %v", name, found, skips, c.found)
		}
	}
}
