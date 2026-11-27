package internal

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// The browsers lines of the Taskfile (gragas#2006). With gragas#1984 `gragas-dev browsers --unmeasured-ok` ends green on a
// system whose browsers row says unmeasured, which the tools step needs to place nothing there and end green. Only the
// tools step may pass it: a member that runs a browser checks the pin first, without the flag, so a browser never runs
// from a folder whose tree was not checked against the pin (supply.provenance.7). Both are held here by reading the
// Taskfile as data, as go-task does, so a comment that names the flag is no line of a member.

// browsersTools is the tools step, by its task name: task tools, the setup the tiers never run (gragas#1215).
const browsersTools = "tools"

// browsersFlag is the flag only the tools step passes.
const browsersFlag = "--unmeasured-ok"

// browsersCheck is a line that calls the browsers check, and browsersRuns one that runs a browser: it sets the folder
// Playwright reads its browsers from, or starts Playwright itself. Both are read from the line, never from a list of
// task names.
var (
	browsersCheck = regexp.MustCompile(`\{\{\.GATE\}\} browsers\b`)
	browsersRuns  = regexp.MustCompile(`(?i)playwright`)
)

// browsersPinCmd is the one command of a pin check: the call with --platform and nothing after it, so no flag, no
// other folder (-C), no digest printed in place of a check (--print) and no refusal swallowed (|| true).
const browsersPinCmd = "{{.GATE}} browsers --platform {{OS}}/{{ARCH}}"

// browsersPathEnv is the one key a browser member's env may hold. A member's env reaches every one of its commands, the
// pin check's go run among them, so any other key (GOFLAGS, PATH, GOTOOLCHAIN, GOWORK, GOENV) could build or run
// another program under the pinned text.
const browsersPathEnv = "PLAYWRIGHT_BROWSERS_PATH"

// browsersPathValue is the one value of browsersPathEnv, wherever the file sets it (gragas#2107): the folder the pin
// check reads is the constant browsersFolder of cmd/gragas-dev/browsers.go, so a browser started from any other folder
// runs browsers no check read.
const browsersPathValue = "{{.ROOT_DIR}}/.tools/playwright"

// browsersPathSet is browsersPathEnv set to browsersPathValue in a command line: a shell assignment of the value
// quoted, or the --env of gragas-dev pnpm, whose quote closes after the value.
var browsersPathSet = regexp.MustCompile(regexp.QuoteMeta(browsersPathEnv+"=") + `"?` + regexp.QuoteMeta(browsersPathValue) + `"`)

// browsersPinVars is the variables browsersPinCmd reads (GATE), read from its text: a member that sets one, or a call
// of it that passes one, keeps the pinned text and runs another program.
func browsersPinVars() []string {
	var out []string
	for _, m := range regexp.MustCompile(`\{\{\s*\.(\w+)`).FindAllStringSubmatch(browsersPinCmd, -1) {
		out = append(out, m[1])
	}
	return out
}

// browsersMemberKey is why a browser member may carry key k, and "" for a key it may not: the set is closed, so a key
// of go-task's that nobody thought of here is refused until it is named with its reason. ignore_error, platforms,
// dotenv, dir, defer and the rest are outside it.
func browsersMemberKey(k string) string {
	switch k {
	case "cmds":
		return "the pin check and the browser"
	case "deps":
		return "what the tests read is built first; a dep that runs a browser is a member of its own"
	case "desc":
		return "words only"
	case "env":
		return "the browsers path may be set for the member alone, and nothing else: the env reaches the pin check's own go run"
	case "status":
		return "the member is skipped whole when its inputs are unchanged, the pin check with the browser; its lines run before the pin check, so none may start the browser"
	case "vars":
		return "the member's own values, none of which the pin check reads"
	}
	return ""
}

// browsersIsPin says whether an entry of a task's commands is a pin check: an entry of cmd and timeout alone, so no
// key of go-task's skips it (platforms), swallows its refusal (ignore_error) or runs it after the rest (defer), and its
// command is browsersPinCmd.
func browsersIsPin(c any) bool {
	m, ok := c.(*ordered.Map)
	if !ok || !slices.Equal(slices.Sorted(slices.Values(m.Keys())), []string{"cmd", "timeout"}) {
		return false
	}
	s, ok := m.Val("cmd").(string)
	return ok && s == browsersPinCmd
}

// browsersProblems says what the Taskfile's text breaks of the two lines, each in its own words, and how many members
// run a browser.
func browsersProblems(text string) (problems []string, members int, err error) {
	v, err := ordered.ParseYAML([]byte(text))
	if err != nil {
		return nil, 0, err
	}
	doc, ok := v.(*ordered.Map)
	if !ok || doc.Sub("tasks") == nil {
		return nil, 0, fmt.Errorf("the Taskfile holds no tasks")
	}
	tasks := doc.Sub("tasks")
	problems = browsersFileProblems(doc)
	var browserMembers []string
	for _, name := range tasks.Keys() {
		task := tasks.Sub(name)
		if name == browsersTools || task == nil {
			continue
		}
		cmds := task.List("cmds")
		lines, pins := make([]string, len(cmds)), make([]bool, len(cmds))
		for i, c := range cmds {
			lines[i], pins[i] = strings.Join(browsersTexts(c), "\n"), browsersIsPin(c)
		}
		first := browsersFirstRun(task, lines, pins)
		if first < 0 {
			continue
		}
		members++
		browserMembers = append(browserMembers, name)
		problems = append(problems, browsersMemberProblems(name, task, lines, pins, first)...)
	}
	return append(problems, browsersCallProblems(tasks, browserMembers)...), members, nil
}

// browsersFileProblems is what the file's own lines break: the flag outside the tools step, Playwright in the file's
// own env or vars, and a dotenv or an include.
func browsersFileProblems(doc *ordered.Map) []string {
	var problems []string
	for _, s := range browsersStrings(doc, "") {
		if strings.Contains(s.text, browsersFlag) && !strings.HasPrefix(s.path, "tasks."+browsersTools+".cmds.") {
			problems = append(problems, fmt.Sprintf("%s passes %s outside the tools step (task %s): only the tools step may end green with no browser placed", s.path, browsersFlag, browsersTools))
		}
		// gragas#2107: every place that names the browsers path sets it to the folder the pin check reads, the tools
		// step's install among them; an env's key is held by its value (browsersEnvProblems)
		if s.text == browsersPathEnv && strings.HasSuffix(s.path, ".env."+browsersPathEnv) {
			continue
		}
		if n, held := strings.Count(s.text, browsersPathEnv), len(browsersPathSet.FindAllString(s.text, -1)); n != held {
			problems = append(problems, fmt.Sprintf("%s names %s %d times and sets it to %s %d: the browsers path is that folder wherever it is set, the one the pin check reads", s.path, browsersPathEnv, n, browsersPathValue, held))
		}
	}
	// The file's own env and vars reach every member, so a member cannot be told from another by them; a dotenv file
	// reaches every member too, with values this test cannot read, and an included file holds tasks it never reads.
	for _, k := range []string{"env", "vars"} {
		for _, s := range browsersStrings(doc.Val(k), k) {
			if browsersRuns.MatchString(s.text) {
				problems = append(problems, fmt.Sprintf("%s names Playwright in the file's own %s, which reaches every member: set the browsers path in the member that runs the browser, so it is told from the others", s.path, k))
			}
		}
	}
	for _, k := range []string{"dotenv", "includes"} {
		if slices.Contains(doc.Keys(), k) {
			problems = append(problems, fmt.Sprintf("the file has %s: it brings in what this test cannot read (an env from a file, or tasks from another Taskfile), so a member that runs a browser could not be seen", k))
		}
	}
	return problems
}

// browsersCallsCheck says whether entry i calls the browsers check and is no pin check.
func browsersCallsCheck(lines []string, pins []bool, i int) bool {
	return !pins[i] && browsersCheck.MatchString(lines[i])
}

// browsersFirstRun is the index of the task's first entry that runs a browser, and -1 for a task that runs none.
func browsersFirstRun(task *ordered.Map, lines []string, pins []bool) int {
	first := slices.IndexFunc(lines, browsersRuns.MatchString)
	for _, k := range slices.DeleteFunc(slices.Clone(task.Keys()), func(k string) bool { return k == "cmds" }) {
		if slices.ContainsFunc(browsersTexts(task.Val(k)), browsersRuns.MatchString) {
			// named in the task's own env or vars, by a key or a value: any line of it may start one, so the first
			// entry that is no call of the check does
			first = 0
			for first < len(lines) && (pins[first] || browsersCallsCheck(lines, pins, first)) {
				first++
			}
			first = min(first, max(0, len(lines)-1))
		}
	}
	return first
}

// browsersMemberKeysProblems is what a member that runs a browser breaks by its keys: a name go-task matches as a
// wildcard, a key outside its closed set, a var the pin check reads, an env key other than the browsers path, and a
// status line or a sh: value that names Playwright, which go-task runs before the first command.
func browsersMemberKeysProblems(name string, task *ordered.Map) []string {
	var problems []string
	if strings.Contains(name, "*") {
		problems = append(problems, fmt.Sprintf("task %s runs a browser and is named with *: go-task matches a call of another name to it, which the call check compares exactly", name))
	}
	for _, k := range task.Keys() {
		if browsersMemberKey(k) == "" {
			problems = append(problems, fmt.Sprintf("task %s runs a browser and carries the key %s, which a browser member may not: its keys are cmds, deps, desc, env, status and vars, each for a reason the test names", name, k))
		}
	}
	if vars := task.Sub("vars"); vars != nil {
		for _, v := range vars.Keys() {
			if slices.Contains(browsersPinVars(), v) {
				problems = append(problems, fmt.Sprintf("task %s runs a browser and sets %s in its own vars, which the pin check reads: the pinned text would run another program", name, v))
			}
		}
	}
	if env, ok := task.Get("env"); ok {
		problems = append(problems, browsersEnvProblems(name, env)...)
	}
	for _, s := range browsersStrings(task, "") {
		before := s.path == "status" || strings.HasPrefix(s.path, "status.") || s.path == "sh" || strings.HasSuffix(s.path, ".sh")
		if before && !strings.HasPrefix(s.path, "cmds.") && browsersRuns.MatchString(s.text) {
			problems = append(problems, fmt.Sprintf("task %s starts a browser before the pin check, at %s: go-task runs a sh: value as it compiles the task and the status lines before the first command", name, s.path))
		}
	}
	return problems
}

// browsersEnvProblems is what a browser member's env breaks: it holds the browsers path alone, as a plain string, since
// it reaches the pin check's own go run and a sh: value runs a program before the pin check.
func browsersEnvProblems(name string, env any) []string {
	m, ok := env.(*ordered.Map)
	if !ok {
		return []string{fmt.Sprintf("task %s runs a browser and its env is no map of names: it may hold %s alone", name, browsersPathEnv)}
	}
	var problems []string
	for _, k := range m.Keys() {
		if k != browsersPathEnv {
			problems = append(problems, fmt.Sprintf("task %s runs a browser and sets %s in its env, which reaches the pin check's own go run: a browser member's env holds %s alone", name, k, browsersPathEnv))
		} else if s, ok := m.Val(k).(string); !ok {
			problems = append(problems, fmt.Sprintf("task %s runs a browser and sets %s in its env to no plain string: a sh: value runs a program before the pin check", name, k))
		} else if s != browsersPathValue {
			problems = append(problems, fmt.Sprintf("task %s runs a browser and sets %s in its env to %q, not %s: the pin check reads that folder alone (gragas#2107)", name, k, s, browsersPathValue))
		}
	}
	return problems
}

// browsersMemberProblems is what a member that runs a browser at entry first breaks: its keys (browsersMemberKeysProblems),
// and the pin check missing, loose or late.
func browsersMemberProblems(name string, task *ordered.Map, lines []string, pins []bool, first int) []string {
	problems := browsersMemberKeysProblems(name, task)
	near := -1 // the first call of the check that is no pin check, up to the line that runs the browser
	for i := range min(first+1, len(lines)) {
		if browsersCallsCheck(lines, pins, i) {
			near = i
			break
		}
	}
	switch {
	case slices.Contains(pins[:first], true):
	case near >= 0 && strings.Contains(lines[near], browsersFlag):
		problems = append(problems, fmt.Sprintf("task %s checks the browsers pin with %s before it runs a browser: the check must refuse an unmeasured system", name, browsersFlag))
	case near >= 0:
		problems = append(problems, fmt.Sprintf("task %s calls the browsers check before it runs a browser in a form that may check nothing (%q): a pin check is an entry of cmd and timeout alone, whose command is the call with --platform and nothing after it", name, lines[near]))
	case slices.Contains(pins[first:], true):
		problems = append(problems, fmt.Sprintf("task %s checks the browsers pin after it runs a browser: the check must come first", name))
	default:
		problems = append(problems, fmt.Sprintf("task %s runs a browser with no browsers pin check before it", name))
	}
	return problems
}

// browsersCallProblems is what the calls of the browser members break: a call of a member, in a task's commands, under
// defer in them, or in its deps, may pass vars; none the pin check reads. A call names its task as written, never by a
// template, since this test cannot read which task a template calls.
func browsersCallProblems(tasks *ordered.Map, browserMembers []string) []string {
	var problems []string
	for _, name := range tasks.Keys() {
		task := tasks.Sub(name)
		if task == nil {
			continue
		}
		for _, c := range append(task.List("cmds"), task.List("deps")...) {
			call, ok := c.(*ordered.Map)
			if !ok {
				continue
			}
			if deferred := call.Sub("defer"); deferred != nil {
				call = deferred // go-task runs a deferred call with its own vars, as it does a call at the top of an entry
			}
			callee, _ := call.Val("task").(string)
			if strings.Contains(callee, "{{") {
				problems = append(problems, fmt.Sprintf("task %s calls a task by a template (%q), a call this test cannot read: a call names its task as written, so a call of a browser member is seen", name, callee))
				continue
			}
			vars := call.Sub("vars")
			if !slices.Contains(browserMembers, callee) || vars == nil {
				continue
			}
			for _, v := range vars.Keys() {
				if slices.Contains(browsersPinVars(), v) {
					problems = append(problems, fmt.Sprintf("task %s calls the browser member %s with %s, which the pin check reads: the pinned text would run another program", name, callee, v))
				}
			}
		}
	}
	return problems
}

// browsersString is a string of the document with the path of keys and indexes it is at.
type browsersString struct{ path, text string }

// browsersStrings is every string of v, each key of a map as well as each value, each with its path from the
// document's root: an env's name says what it sets as much as its value does.
func browsersStrings(v any, path string) []browsersString {
	join := func(k string) string { return strings.TrimPrefix(path+"."+k, ".") }
	var out []browsersString
	switch v := v.(type) {
	case string:
		out = append(out, browsersString{path, v})
	case *ordered.Map:
		for _, k := range v.Keys() {
			out = append(out, browsersString{join(k), k})
			out = append(out, browsersStrings(v.Val(k), join(k))...)
		}
	case []any:
		for i, e := range v {
			out = append(out, browsersStrings(e, join(fmt.Sprint(i)))...)
		}
	}
	return out
}

// browsersTexts is the strings of v alone.
func browsersTexts(v any) []string {
	var out []string
	for _, s := range browsersStrings(v, "") {
		out = append(out, s.text)
	}
	return out
}

// TestTheBrowsersLinesOfTheTaskfile (gragas#2006): the tree's Taskfile holds both lines, has a member that runs a browser
// and a tools step that passes the flag, so neither half reads nothing; and Taskfiles made from it are each red in
// their own words: a browser member whose pin check carries the flag, one with no pin check, one whose pin check comes
// after the browser, the flag on a line outside the tools step; a check that prints, reads another folder, swallows
// its refusal in the line or by ignore_error, runs on one platform or is deferred; a member known only by its env's or
// vars' key; the browsers path in the file's own env or vars; and (gragas#2066) a member that carries a key outside its
// closed set (ignore_error, platforms, dotenv, dir, one go-task does not have), sets GATE in its vars or is called with
// it, and a file with dotenv or includes; and a deferred call of the member that passes GATE, a
// call by a template, a status line or a sh: value that starts the browser before the pin check, an env key other than
// the browsers path or that path by a sh: value, and a member named with a wildcard; and (gragas#2107) the browsers
// path set to another folder than the pin check's in the member's env (with the run line's --env dropped), in the run
// line's --env (another folder, or one below it), in the tools step's install, and a second assignment after the held
// one. Planted failures: a test that holds the env key and not its value, counts no line's assignments, takes a folder
// below the pin's (gragas#2107), reads no member, takes any check, takes any entry of a check, reads no key, reads no
// order, lets the flag or the file's env anywhere, bans keys in place of a closed set, reads no variable of the pin, reads no dotenv, reads no
// deferred call, takes a call by a template, reads no status line or sh: value, or lets any env key through.
func TestTheBrowsersLinesOfTheTaskfile(t *testing.T) {
	t.Parallel()
	real := read(t, "Taskfile.yml")
	problems, members, err := browsersProblems(real)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 || members == 0 {
		t.Fatalf("the tree's Taskfile: %d members run a browser, want one at least, and these, want none:\n%s", members, strings.Join(problems, "\n"))
	}
	tools, _, err := browsersProblems(strings.Replace(real, "\n  "+browsersTools+":\n", "\n  setup:\n", 1))
	if err != nil || len(tools) == 0 {
		t.Fatalf("the tools step renamed: %v, %q, want the flag refused: the test does not find the tools step by its name", err, tools)
	}
	const check = `      - cmd: "{{.GATE}} browsers --platform {{OS}}/{{ARCH}}"
        timeout: 5m
`
	const run = `      - cmd: '{{.GATE}} pnpm -C explorer --tools "{{.TOOLS}}" --env "PLAYWRIGHT_BROWSERS_PATH={{.ROOT_DIR}}/.tools/playwright" -- run test'
        timeout: 10m
`
	// gragas#2107: the value is the folder of the pin check, read from its own source
	if folder := `const browsersFolder = ".tools/playwright"`; !strings.Contains(read(t, "cmd/gragas-dev/browsers.go"), "\n"+folder+"\n") || browsersPathValue != "{{.ROOT_DIR}}/.tools/playwright" {
		t.Fatalf("cmd/gragas-dev/browsers.go no longer holds %s, or browsersPathValue (%s) is not that folder under ROOT_DIR: the browsers path the Taskfile sets is the folder the pin check reads", folder, browsersPathValue)
	}
	env, _, err := browsersProblems(strings.Replace(real, "    cmds:\n"+check+run, "    env: {PLAYWRIGHT_BROWSERS_PATH: \"{{.ROOT_DIR}}/.tools/playwright\"}\n    cmds:\n"+check+run, 1))
	if err != nil || len(env) != 0 {
		t.Errorf("the browsers path in the task's env with the pin check first: %v, %q, want no problem", err, env)
	}
	const notPin = "task explorer-test calls the browsers check before it runs a browser in a form that may check nothing"
	const (
		member     = "\n  explorer-test:\n"
		memberVars = "    vars:\n      INPUTS: \"{{.EXPLORER_TEST_INPUTS}}\"\n"
		call       = "      - task: explorer-test\n        vars: {SKIP_UNCHANGED: \"{{.SKIP_UNCHANGED}}\"}\n"
	)
	keyOf := func(k string) string {
		return "task explorer-test runs a browser and carries the key " + k + ", which a browser member may not"
	}
	const lint = `run lint --concurrency`
	if !strings.Contains(real, check+run) || !strings.Contains(real, lint) {
		t.Fatal("the Taskfile no longer holds the explorer-test lines or the explorer-lint line the plants are made from")
	}
	for _, plant := range []struct{ name, text, want string }{
		{"the pin check carries the flag", strings.Replace(real, check, strings.Replace(check, `{{ARCH}}"`, `{{ARCH}} `+browsersFlag+`"`, 1), 1),
			"task explorer-test checks the browsers pin with " + browsersFlag + " before it runs a browser"},
		{"no pin check", strings.Replace(real, check+run, run, 1), "task explorer-test runs a browser with no browsers pin check before it"},
		{"the pin check after the browser", strings.Replace(real, check+run, run+check, 1), "task explorer-test checks the browsers pin after it runs a browser"},
		{"the flag outside the tools step", strings.Replace(real, lint, "run lint "+browsersFlag+" --concurrency", 1), "tasks.explorer-lint.cmds.0.cmd passes " + browsersFlag + " outside the tools step"},
		{"the browsers path in the task's env, no pin check", strings.Replace(real, "    cmds:\n"+check+run, "    env: {PLAYWRIGHT_BROWSERS_PATH: \"{{.ROOT_DIR}}/.tools/playwright\"}\n    cmds:\n"+strings.Replace(run, ` --env "PLAYWRIGHT_BROWSERS_PATH={{.ROOT_DIR}}/.tools/playwright"`, "", 1), 1),
			"task explorer-test runs a browser with no browsers pin check before it"},
		// gragas#2107: the browsers path is the pin check's folder in the member's env, its run line and the tools step
		{"the member's env names another folder, the --env dropped", strings.Replace(real, "    cmds:\n"+check+run, "    env: {PLAYWRIGHT_BROWSERS_PATH: other/browsers}\n    cmds:\n"+check+strings.Replace(run, ` --env "PLAYWRIGHT_BROWSERS_PATH={{.ROOT_DIR}}/.tools/playwright"`, "", 1), 1),
			`task explorer-test runs a browser and sets PLAYWRIGHT_BROWSERS_PATH in its env to "other/browsers", not {{.ROOT_DIR}}/.tools/playwright`},
		{"the run line's --env names another folder", strings.Replace(real, run, strings.Replace(run, "{{.ROOT_DIR}}/.tools/playwright", "{{.ROOT_DIR}}/other/browsers", 1), 1),
			"tasks.explorer-test.cmds.1.cmd names PLAYWRIGHT_BROWSERS_PATH 1 times and sets it to {{.ROOT_DIR}}/.tools/playwright 0"},
		{"the run line's --env names a folder below the pin's", strings.Replace(real, run, strings.Replace(run, "{{.ROOT_DIR}}/.tools/playwright", "{{.ROOT_DIR}}/.tools/playwright/other", 1), 1),
			"tasks.explorer-test.cmds.1.cmd names PLAYWRIGHT_BROWSERS_PATH 1 times and sets it to {{.ROOT_DIR}}/.tools/playwright 0"},
		{"the tools step installs into another folder", strings.Replace(real, `PLAYWRIGHT_BROWSERS_PATH=\"{{.ROOT_DIR}}/.tools/playwright\"`, `PLAYWRIGHT_BROWSERS_PATH=\"{{.ROOT_DIR}}/other/browsers\"`, 1),
			"tasks.tools.cmds.16.cmd names PLAYWRIGHT_BROWSERS_PATH 1 times and sets it to {{.ROOT_DIR}}/.tools/playwright 0"},
		{"a second assignment after the held one", strings.Replace(real, run, strings.Replace(run, `-- run test`, `-- run test --env PLAYWRIGHT_BROWSERS_PATH=other`, 1), 1),
			"tasks.explorer-test.cmds.1.cmd names PLAYWRIGHT_BROWSERS_PATH 2 times and sets it to {{.ROOT_DIR}}/.tools/playwright 1"},
		// a call of the check in any other form is no pin check
		{"the check prints", strings.Replace(real, check, strings.Replace(check, `browsers --platform`, `browsers --print --platform`, 1), 1), notPin},
		{"the check reads another folder", strings.Replace(real, check, strings.Replace(check, `{{ARCH}}"`, `{{ARCH}} -C elsewhere"`, 1), 1), notPin},
		{"the check's refusal swallowed in the line", strings.Replace(real, check, strings.Replace(check, `{{ARCH}}"`, `{{ARCH}} || true"`, 1), 1), notPin},
		{"the check's refusal swallowed by ignore_error", strings.Replace(real, check, check+"        ignore_error: true\n", 1), notPin},
		{"the check on one platform", strings.Replace(real, check, check+"        platforms: [linux]\n", 1), notPin},
		{"the check deferred", strings.Replace(real, check, strings.Replace(check, "- cmd:", "- defer:", 1), 1), notPin},
		// and a member is known by an env's name as well as its value, which the file's own env and vars may not hold
		{"the browsers path as a key of a member's env", strings.Replace(real, "\ntasks:\n", "\ntasks:\n  b:\n    env: {PLAYWRIGHT_BROWSERS_PATH: other/browsers}\n    cmds:\n      - cmd: echo b\n", 1),
			"task b runs a browser with no browsers pin check before it"},
		{"the browsers path as a key of a member's vars", strings.Replace(real, "\ntasks:\n", "\ntasks:\n  b:\n    vars: {PLAYWRIGHT_BROWSERS_PATH: other/browsers}\n    cmds:\n      - cmd: echo b\n", 1),
			"task b runs a browser with no browsers pin check before it"},
		{"the browsers path in the file's env", strings.Replace(real, "\nenv:\n", "\nenv:\n  PLAYWRIGHT_BROWSERS_PATH: other/browsers\n", 1), "env.PLAYWRIGHT_BROWSERS_PATH names Playwright in the file's own env"},
		{"the browsers path in the file's vars", strings.Replace(real, "\nvars:\n", "\nvars:\n  PLAYWRIGHT_BROWSERS_PATH: other/browsers\n", 1), "vars.PLAYWRIGHT_BROWSERS_PATH names Playwright in the file's own vars"},
		// gragas#2066: the member's own keys are a closed set, and nothing sets what the pin check reads
		{"the member swallows its refusal", strings.Replace(real, member, member+"    ignore_error: true\n", 1), keyOf("ignore_error")},
		{"the member on one platform", strings.Replace(real, member, member+"    platforms: [linux]\n", 1), keyOf("platforms")},
		{"the member reads a dotenv", strings.Replace(real, member, member+"    dotenv: [.env]\n", 1), keyOf("dotenv")},
		{"the member in another folder, a key nobody named", strings.Replace(real, member, member+"    dir: elsewhere\n", 1), keyOf("dir")},
		{"a key that is no key of go-task's yet", strings.Replace(real, member, member+"    retries: 3\n", 1), keyOf("retries")},
		{"the member sets GATE in its own vars", strings.Replace(real, memberVars, memberVars+"      GATE: other/gate\n", 1),
			"task explorer-test runs a browser and sets GATE in its own vars, which the pin check reads"},
		{"a call of the member passes GATE", strings.Replace(real, call, strings.Replace(call, "}}\"}", "}}\", GATE: other/gate}", 1), 1),
			"calls the browser member explorer-test with GATE, which the pin check reads"},
		{"a dotenv at the file", strings.Replace(real, "\nenv:\n", "\ndotenv: [.env]\nenv:\n", 1), "the file has dotenv"},
		{"an included Taskfile", strings.Replace(real, "\nenv:\n", "\nincludes: {more: ./more.yml}\nenv:\n", 1), "the file has includes"},
		// a deferred call, a call by a template, a status line or a sh: value, and a member's env
		{"a deferred call of the member passes GATE", strings.Replace(real, call, call+"      - defer: {task: explorer-test, vars: {GATE: other/gate}}\n", 1),
			"calls the browser member explorer-test with GATE, which the pin check reads"},
		{"a call by a template passes GATE", strings.Replace(real, call, call+"      - task: '{{.M}}'\n        vars: {GATE: other/gate}\n", 1),
			"calls a task by a template (\"{{.M}}\"), a call this test cannot read"},
		{"a status line starts the browser", strings.Replace(real, memberVars+"    status:\n", memberVars+"    status:\n      - 'pnpm -C explorer exec playwright test'\n", 1),
			"task explorer-test starts a browser before the pin check, at status.0"},
		{"a sh: var starts the browser", strings.Replace(real, memberVars, memberVars+"      RUN: {sh: \"pnpm -C explorer exec playwright test\"}\n", 1),
			"task explorer-test starts a browser before the pin check, at vars.RUN.sh"},
		{"the member's env reaches the pin check's go run", strings.Replace(real, member, member+"    env: {GOFLAGS: -overlay=other/overlay.json}\n", 1),
			"task explorer-test runs a browser and sets GOFLAGS in its env, which reaches the pin check's own go run"},
		{"the browsers path in the member's env by a sh: value", strings.Replace(real, member, member+"    env: {PLAYWRIGHT_BROWSERS_PATH: {sh: echo .tools/playwright}}\n", 1),
			"task explorer-test runs a browser and sets PLAYWRIGHT_BROWSERS_PATH in its env to no plain string"},
		{"the member named with a wildcard", strings.Replace(real, member, "\n  explorer-*:\n", 1), "task explorer-* runs a browser and is named with *"},
	} {
		if plant.text == real {
			t.Fatalf("%s: the plant changed nothing", plant.name)
		}
		got, _, err := browsersProblems(plant.text)
		if err != nil {
			t.Fatalf("%s: %v", plant.name, err)
		}
		if !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, plant.want) }) {
			t.Errorf("%s: got %q, want a problem saying %q", plant.name, got, plant.want)
		}
	}
}
