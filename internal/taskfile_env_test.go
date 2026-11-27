package internal

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
)

// The env and the held variables of the Taskfile (gragas#2107). The paths are go-task v3.54.0's, by file and line:
//
//   - The file's env reaches every member's commands, and a task's env reaches its own: variables.go:195-197 merges the
//     file's env, the task's dotenv and the task's env into the env each command runs with, so a key such as GOFLAGS
//     (-overlay), PATH, GOTOOLCHAIN or GOENV changes which program a pinned line runs. The file's env is also read as
//     template values, after go-task's own (compiler.go:108-112), so an env key named ROOT_DIR replaces it.
//   - A file-level dotenv fills the file's env for the keys it does not set (setup.go:229-249), and a task's dotenv is
//     merged into that task's (variables.go:196), with values this test cannot read.
//   - The variables a line reads come, in order, from the caller's environment (compiler.go:49), go-task's own
//     (compiler.go:50-56, getSpecialVars at 206-258), the file's env and vars (108-113), the included file's (119-124),
//     the call's vars (135) and the task's own vars (140), each over the last. So a task's vars or a call's vars
//     replace GATE, TOOLS or ROOT_DIR for every line of the task called.
//   - A call is a cmds entry of task: and vars: (taskfile/ast/cmd.go:113-121), a deferred one whose vars are its own
//     (cmd.go:102-108, run at task.go:342-359), or a dep with vars (taskfile/ast/dep.go:40-66). A call by a template
//     is resolved before it runs (variables.go:264, 281 in cmds; 314, 321 in deps), so a test cannot read which task
//     it calls; its vars are refused here whatever it calls.
//   - A for's as names a value each entry's cmd, task and vars are made with, over the file's (variables.go:246-266 in
//     cmds, 299-315 in deps), so `as: GATE` runs another program under the pinned text.
//   - The command line's NAME=value pairs are merged into the file's vars (cmd/task/task.go:183, args/args.go:29-41):
//     a person's own words, which no file holds. gragas-dev hook passes SKIP_UNCHANGED alone (cmd/gragas-dev/hook.go).
//   - The caller's own environment wins over every env of the file (internal/env/env.go:36-51), since the
//     ENV_PRECEDENCE experiment is off (.taskrc.yml names none): this test holds what the file sets.

// envHeldVars are the variables that name the programs the members run: GATE, the gate's own commands built from this
// tree (the browsers pin check's text reads it), and TOOLS, the folder of the pinned tools. Only the file's own vars
// set them.
func envHeldVars() []string { return []string{"GATE", "TOOLS"} }

// envSpecialVars are the variables go-task sets itself: getSpecialVars (compiler.go:206-258), the command line's
// (cmd/task/task.go:191-197), a deferred command's EXIT_CODE (task.go:351), a wildcard's MATCH (task.go:591) and a
// for's ITEM and KEY (variables.go:250, 258). A vars block, a call or an as that names one replaces it for every line
// that reads it: ROOT_DIR is where every path of the file is made from.
func envSpecialVars() []string {
	return []string{
		"TASK_EXE", "ROOT_TASKFILE", "ROOT_DIR", "USER_WORKING_DIR", "TASK_VERSION", "PATH_LIST_SEPARATOR",
		"FILE_PATH_SEPARATOR", "TASK", "TASK_DIR", "TASKFILE", "TASKFILE_DIR", "ALIAS",
		"CLI_ARGS", "CLI_ARGS_LIST", "CLI_FORCE", "CLI_SILENT", "CLI_VERBOSE", "CLI_OFFLINE", "CLI_ASSUME_YES",
		"EXIT_CODE", "MATCH", "ITEM", "KEY",
	}
}

// envAnyTask is the Where of a row any task's env may carry.
const envAnyTask = "tasks.*.env"

// envAnyTaskVars is the Where of a key a task's vars, a call's vars or a for's as may set. Its rows hold the key
// alone, so the value cell is envAnyValue. Every other key there is refused, since a task's or a call's vars replace
// the file's for every line of the task called (compiler.go:135, 140): GOFLAGS_GUARD, BUDGET and HATCH_TESTS_FLAGS
// are whole command lines or the flags of one, and a key added to the file's vars later is held with them unnamed.
const envAnyTaskVars = "tasks.*.vars"

// envAnyValue is the value cell of a tasks.*.vars row.
const envAnyValue = "*"

// envHeading is the heading of docs/checks.md the table is under.
const envHeading = "### What the Taskfile's env and vars may set"

// envRow is a row of the table: where the key is set (env and vars at the top of the file, tasks.<name>.env in one
// task, tasks.*.env in any), the key, and its one value.
type envRow struct{ where, key, value string }

// envRows is the table of docs/checks.md, under envHeading, up to the next heading.
func envRows(doc string) ([]envRow, error) {
	_, after, ok := strings.Cut(doc, "\n"+envHeading+"\n")
	if !ok {
		return nil, fmt.Errorf("docs/checks.md has no heading %q", envHeading)
	}
	if i := strings.Index(after, "\n#"); i >= 0 {
		after = after[:i]
	}
	unquote := func(cell string) (string, bool) {
		s, ok := strings.CutPrefix(strings.TrimSpace(cell), "`")
		if !ok {
			return "", false
		}
		s, ok = strings.CutSuffix(s, "`")
		return s, ok && !strings.Contains(s, "`")
	}
	var rows []envRow
	for line := range strings.SplitSeq(after, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|"), " | ")
		if len(cells) != 4 {
			return nil, fmt.Errorf("a row of the table is not four cells (where, key, value, why): %q", line)
		}
		var r envRow
		for i, p := range []*string{&r.where, &r.key, &r.value} {
			v, ok := unquote(cells[i])
			if !ok {
				return nil, fmt.Errorf("a row's cell %d is not one code span: %q", i+1, line)
			}
			*p = v
		}
		if strings.TrimSpace(cells[3]) == "" {
			return nil, fmt.Errorf("the row of %s %s says no why", r.where, r.key)
		}
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("docs/checks.md has no row under %q", envHeading)
	}
	return rows, nil
}

// envProblems says what the Taskfile's text breaks of the table and of the held variables, each in its own words.
func envProblems(text string, rows []envRow) ([]string, error) {
	v, err := ordered.ParseYAML([]byte(text))
	if err != nil {
		return nil, err
	}
	doc, ok := v.(*ordered.Map)
	if !ok || doc.Sub("tasks") == nil {
		return nil, fmt.Errorf("the Taskfile holds no tasks")
	}
	tasks := doc.Sub("tasks")
	problems := envRowProblems(rows, tasks)
	sets := envTaskVars{seen: map[string]bool{}}
	for _, r := range rows {
		if r.where == envAnyTaskVars {
			sets.allowed = append(sets.allowed, r.key)
		}
	}
	for _, k := range []string{"dotenv", "includes"} {
		if slices.Contains(doc.Keys(), k) {
			problems = append(problems, fmt.Sprintf("the file has %s: it brings in an env or tasks this test cannot read", k))
		}
	}
	problems = append(problems, envBlockProblems(rows, "env", doc.ValOr("env", ordered.NewMap(0)), "")...)
	vars := doc.Sub("vars")
	if vars == nil {
		vars = ordered.NewMap(0)
	}
	problems = append(problems, envBlockProblems(rows, "vars", vars, "")...)
	for _, k := range vars.Keys() {
		if slices.Contains(envSpecialVars(), k) {
			problems = append(problems, fmt.Sprintf("vars sets %s, a variable go-task sets itself, for every line that reads it", k))
		}
	}
	for _, name := range tasks.Keys() {
		task := tasks.Sub(name)
		if task == nil {
			continue
		}
		at := "tasks." + name
		if slices.Contains(task.Keys(), "dotenv") {
			problems = append(problems, fmt.Sprintf("%s has dotenv: it brings in an env this test cannot read", at))
		}
		env, ok := task.Get("env")
		if !ok && slices.ContainsFunc(rows, func(r envRow) bool { return r.where == at+".env" }) {
			env, ok = ordered.NewMap(0), true // a row of the task, which sets no env: each row is unset
		}
		if ok {
			problems = append(problems, envBlockProblems(rows, at+".env", env, envAnyTask)...)
		}
		problems = append(problems, sets.problems(task, at)...)
	}
	for _, k := range sets.allowed {
		if !sets.seen[k] {
			problems = append(problems, fmt.Sprintf("the table's row %s %s is set by no task's vars, call's vars or for's as", envAnyTaskVars, k))
		}
	}
	return problems, nil
}

// envFind is the table's row of key at where.
func envFind(rows []envRow, where, key string) (envRow, bool) {
	i := slices.IndexFunc(rows, func(r envRow) bool { return r.where == where && r.key == key })
	if i < 0 {
		return envRow{}, false
	}
	return rows[i], true
}

// envRowProblems is what the rows themselves break: each place is one the file has, no row names a variable of
// go-task's, and each held variable has its row in the file's vars.
func envRowProblems(rows []envRow, tasks *ordered.Map) []string {
	var problems []string
	for _, r := range rows {
		switch {
		case slices.Contains(envSpecialVars(), r.key):
			problems = append(problems, fmt.Sprintf("the table's row %s %s names a variable go-task sets itself", r.where, r.key))
		case r.where == "env" || r.where == envAnyTask:
		case r.where == envAnyTaskVars:
			switch {
			case slices.Contains(envHeldVars(), r.key):
				problems = append(problems, fmt.Sprintf("the table's row %s %s names a held variable, which only the file's own vars set", r.where, r.key))
			case r.value != envAnyValue:
				problems = append(problems, fmt.Sprintf("the table's row %s %s has the value %q: a key a task or a call sets is held by its name, and its value cell is %s", r.where, r.key, r.value, envAnyValue))
			}
		case r.where == "vars":
			if !slices.Contains(envHeldVars(), r.key) {
				problems = append(problems, fmt.Sprintf("the table's row vars %s is no held variable (%s): the file's other vars are not the table's", r.key, strings.Join(envHeldVars(), ", ")))
			}
		default:
			name, ok := strings.CutPrefix(r.where, "tasks.")
			name, ok2 := strings.CutSuffix(name, ".env")
			if !ok || !ok2 || tasks.Sub(name) == nil {
				problems = append(problems, fmt.Sprintf("the table's row %s %s names a place the file does not have: a row's where is env, vars, tasks.<name>.env of a task the file has, or %s", r.where, r.key, envAnyTask))
			}
		}
	}
	for _, k := range envHeldVars() {
		if _, ok := envFind(rows, "vars", k); !ok {
			problems = append(problems, fmt.Sprintf("the table has no row vars %s: a held variable's value is the table's", k))
		}
	}
	return problems
}

// envBlockProblems is what an env or vars block breaks of the table at where (env, vars or tasks.<name>.env), with
// also's rows as the fallback: a key the table does not name there, another value, no plain string, and a row of the
// place the block does not set. The file's vars may hold keys the table does not name.
func envBlockProblems(rows []envRow, where string, block any, also string) []string {
	m, ok := block.(*ordered.Map)
	if !ok {
		return []string{fmt.Sprintf("%s is no map of names: it holds the keys of the table alone (docs/checks.md)", where)}
	}
	var problems []string
	for _, k := range m.Keys() {
		r, ok := envFind(rows, where, k)
		if !ok && also != "" {
			r, ok = envFind(rows, also, k)
		}
		s, isString := m.Val(k).(string)
		switch {
		case !ok && where == "vars":
		case !ok:
			problems = append(problems, fmt.Sprintf("%s.%s is set, and the table of docs/checks.md has no row of it: an env key reaches every command it is set for, the go run lines among them, so it could run another program under the pinned text", where, k))
		case !isString:
			problems = append(problems, fmt.Sprintf("%s.%s is no plain string: a sh: value runs a program as go-task compiles the task, and a ref: reads another variable", where, k))
		case s != r.value:
			problems = append(problems, fmt.Sprintf("%s.%s is %q, not the table's %q", where, k, s, r.value))
		}
	}
	for _, r := range rows {
		if r.where == where && !slices.Contains(m.Keys(), r.key) {
			problems = append(problems, fmt.Sprintf("the table's row %s %s is not set by the file", r.where, r.key))
		}
	}
	return problems
}

// envTaskVars is the keys the table's tasks.*.vars rows let a task, a call or an as set, and those a task has set.
type envTaskVars struct {
	allowed []string
	seen    map[string]bool
}

// problems is what the vars blocks and the as keys of a task set that they may not: a held variable, which only the
// file's own vars set, a variable go-task sets itself, and any key no tasks.*.vars row names. Every map of the task is
// read, so a task's own vars, a call's at the top of an entry, under defer, in deps and by any name or a template, and
// a for's as, in cmds or deps, are each seen.
func (s envTaskVars) problems(v any, path string) []string {
	why := func(k string) string {
		switch {
		case slices.Contains(envHeldVars(), k):
			return "which only the file's own vars set: it names a program the members run"
		case slices.Contains(envSpecialVars(), k):
			return "a variable go-task sets itself, replaced for every line that reads it"
		case !slices.Contains(s.allowed, k):
			return "which no row " + envAnyTaskVars + " of the table names: it replaces the file's var of that name for every line of the task called, a whole command line such as GOFLAGS_GUARD or BUDGET among them"
		}
		s.seen[k] = true
		return ""
	}
	var problems []string
	switch v := v.(type) {
	case *ordered.Map:
		for _, k := range v.Keys() {
			at := path + "." + k
			switch val := v.Val(k).(type) {
			case *ordered.Map:
				if k == "vars" {
					for _, name := range val.Keys() {
						if w := why(name); w != "" {
							problems = append(problems, fmt.Sprintf("%s sets %s, %s", at, name, w))
						}
					}
				}
			case string:
				if k != "as" {
					break
				}
				if w := why(val); w != "" {
					problems = append(problems, fmt.Sprintf("%s names %s, %s", at, val, w))
				}
			}
			problems = append(problems, s.problems(v.Val(k), at)...)
		}
	case []any:
		for i, e := range v {
			problems = append(problems, s.problems(e, fmt.Sprintf("%s.%d", path, i))...)
		}
	}
	return problems
}

// TestTheEnvAndTheHeldVarsOfTheTaskfile (gragas#2107): the tree's Taskfile sets each env key and held variable the
// table of docs/checks.md names, at its value, and no other; and Taskfiles made from it are each red in their own
// words: GATE in a member's vars, GOFLAGS=-overlay in the file's env, PATH in a member's env (the ticket's three), and
// GATE by a deferred call, by a call by a template, TOOLS by a dep's vars, GATE by a for's as, ROOT_DIR in a task's or
// the file's vars, GATE at another value or by sh:, an env value by sh:, a task's env at another value, a key the
// table lacks, a row the file does not set, a row of a task the file lacks, the browsers path in a task's env at
// another folder, and a dotenv in a task or at the file. Planted failures: a test that reads no table, holds the keys
// and not the values, reads only a task's own vars, reads no deferred call, dep or as, takes any value as a string,
// reads no variable of go-task's, lets a row go unset, or reads no dotenv.
func TestTheEnvAndTheHeldVarsOfTheTaskfile(t *testing.T) {
	t.Parallel()
	real := read(t, "Taskfile.yml")
	rows, err := envRows(read(t, "docs/checks.md"))
	if err != nil {
		t.Fatal(err)
	}
	problems, err := envProblems(real, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("the tree's Taskfile against the table of docs/checks.md, want no problem:\n%s", strings.Join(problems, "\n"))
	}
	const (
		vuln     = "\n  vuln:\n"
		vulnDep  = `vars: {PLATFORM: "{{.ITEM.PLATFORM}}"}`
		call     = "      - task: push-checks\n        vars: {SKIP_UNCHANGED: \"{{.SKIP_UNCHANGED}}\"}\n"
		gate     = "\n  GATE: \"{{.TOOLS}}/gragas-dev{{exeExt}}\"\n"
		goflags  = "\n  GOFLAGS: -mod=readonly\n"
		crash    = "\n  crash:\n"
		crashEnv = "      CRASH_POINTS: \"400\"\n"
		tasks    = "\ntasks:\n"
		check    = "\n  tools-check:\n"
		callVars = `vars: {SKIP_UNCHANGED: "{{.SKIP_UNCHANGED}}"}`
	)
	for _, s := range []string{vuln, vulnDep, call, gate, goflags, crash, crashEnv, tasks, check, "\nvars:\n", "\nenv:\n", "\n  GOTOOLCHAIN: "} {
		if !strings.Contains(real, s) {
			t.Fatalf("the Taskfile no longer holds %q, which the plants are made from", s)
		}
	}
	for _, plant := range []struct{ name, text, want string }{
		// the ticket's three
		{"GATE in a member's vars", strings.Replace(real, vuln, vuln+"    vars: {GATE: other/gate}\n", 1),
			"tasks.vuln.vars sets GATE, which only the file's own vars set"},
		{"GOFLAGS=-overlay in the file's env", strings.Replace(real, goflags, "\n  GOFLAGS: -overlay=other/overlay.json\n", 1),
			`env.GOFLAGS is "-overlay=other/overlay.json", not the table's "-mod=readonly"`},
		{"PATH in a member's env", strings.Replace(real, vuln, vuln+"    env: {PATH: other/bin}\n", 1),
			"tasks.vuln.env.PATH is set, and the table of docs/checks.md has no row of it"},
		// every other way a held variable is set
		{"GATE by a deferred call", strings.Replace(real, tasks, tasks+"  b:\n    cmds:\n      - defer: {task: lint-diff, vars: {GATE: other/gate}}\n", 1),
			"tasks.b.cmds.0.defer.vars sets GATE, which only the file's own vars set"},
		{"GATE by a call by a template", strings.Replace(real, call, call+"      - task: '{{.M}}'\n        vars: {GATE: other/gate}\n", 1),
			"tasks.pre-push-members.deps.1.vars sets GATE"},
		{"TOOLS by a dep's vars", strings.Replace(real, vulnDep, `vars: {PLATFORM: "{{.ITEM.PLATFORM}}", TOOLS: other/bin}`, 1),
			"tasks.vuln.deps.0.vars sets TOOLS, which only the file's own vars set"},
		{"GATE by a for's as", strings.Replace(real, tasks, tasks+"  b:\n    cmds:\n      - for: [other/gate]\n        as: GATE\n        cmd: '{{.GATE}} version'\n        timeout: 1m\n", 1),
			"tasks.b.cmds.0.as names GATE, which only the file's own vars set"},
		// a key no tasks.*.vars row names, which replaces a whole command line of the file's vars
		{"GOFLAGS_GUARD in tools-check's vars", strings.Replace(real, check, check+"    vars: {GOFLAGS_GUARD: \"true\"}\n", 1),
			"tasks.tools-check.vars sets GOFLAGS_GUARD, which no row tasks.*.vars of the table names"},
		{"BUDGET in a call's vars", strings.Replace(real, call, strings.Replace(call, callVars, `vars: {SKIP_UNCHANGED: "{{.SKIP_UNCHANGED}}", BUDGET: "true"}`, 1), 1),
			"tasks.pre-push-members.deps.0.vars sets BUDGET, which no row tasks.*.vars of the table names"},
		{"HATCH_TESTS_FLAGS by a for's as", strings.Replace(real, tasks, tasks+"  b:\n    cmds:\n      - for: [--red-when failed]\n        as: HATCH_TESTS_FLAGS\n        task: test\n", 1),
			"tasks.b.cmds.0.as names HATCH_TESTS_FLAGS, which no row tasks.*.vars of the table names"},
		{"ROOT_DIR in a task's vars", strings.Replace(real, vuln, vuln+"    vars: {ROOT_DIR: other}\n", 1),
			"tasks.vuln.vars sets ROOT_DIR, a variable go-task sets itself"},
		{"ROOT_DIR in the file's vars", strings.Replace(real, "\nvars:\n", "\nvars:\n  ROOT_DIR: other\n", 1),
			"vars sets ROOT_DIR, a variable go-task sets itself"},
		{"GATE at another value", strings.Replace(real, gate, "\n  GATE: go run ./other\n", 1),
			`vars.GATE is "go run ./other", not the table's "{{.TOOLS}}/gragas-dev{{exeExt}}"`},
		{"GATE by sh:", strings.Replace(real, gate, "\n  GATE: {sh: echo gragas-dev}\n", 1),
			"vars.GATE is no plain string"},
		// the env, key and value
		{"an env value by sh:", strings.Replace(real, "\n  GOTOOLCHAIN: ", "\n  GOTOOLCHAIN: {sh: echo go1.27.4}\n  GOTOOLCHAIN_WAS: ", 1),
			"env.GOTOOLCHAIN is no plain string"},
		{"a task's env at another value", strings.Replace(real, crashEnv, "      CRASH_POINTS: \"1\"\n", 1),
			`tasks.crash.env.CRASH_POINTS is "1", not the table's "400"`},
		{"a key the table lacks in the file's env", strings.Replace(real, "\nenv:\n", "\nenv:\n  GOENV: other/go.env\n", 1),
			"env.GOENV is set, and the table of docs/checks.md has no row of it"},
		{"a row the file does not set", strings.Replace(real, goflags, "\n", 1), "the table's row env GOFLAGS is not set by the file"},
		{"a row of a task the file lacks", strings.Replace(real, crash, "\n  crashes:\n", 1), "the table's row tasks.crash.env CRASH_POINTS names a place the file does not have"},
		{"the browsers path at another folder", strings.Replace(real, vuln, vuln+"    env: {PLAYWRIGHT_BROWSERS_PATH: other/browsers}\n", 1),
			`tasks.vuln.env.PLAYWRIGHT_BROWSERS_PATH is "other/browsers", not the table's "{{.ROOT_DIR}}/.tools/playwright"`},
		{"a dotenv in a task", strings.Replace(real, vuln, vuln+"    dotenv: [.env]\n", 1), "tasks.vuln has dotenv"},
		{"a dotenv at the file", strings.Replace(real, "\nenv:\n", "\ndotenv: [.env]\nenv:\n", 1), "the file has dotenv"},
	} {
		if plant.text == real {
			t.Fatalf("%s: the plant changed nothing", plant.name)
		}
		got, err := envProblems(plant.text, rows)
		if err != nil {
			t.Fatalf("%s: %v", plant.name, err)
		}
		if !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, plant.want) }) {
			t.Errorf("%s: got %q, want a problem saying %q", plant.name, got, plant.want)
		}
	}
	// the tasks.*.vars rows against the tree's Taskfile: a row gone, a row no task sets, a row with a value, a row of a
	// held variable
	without := func(key string) []envRow {
		return slices.DeleteFunc(slices.Clone(rows), func(r envRow) bool { return r.where == envAnyTaskVars && r.key == key })
	}
	valued := slices.Clone(rows)
	for i, r := range valued {
		if r.where == envAnyTaskVars && r.key == "PUBLISH" {
			valued[i].value = "1"
		}
	}
	for _, plant := range []struct {
		name string
		rows []envRow
		want string
	}{
		{"the PUBLISH row gone", without("PUBLISH"), "tasks.release-publish.cmds.4.vars sets PUBLISH, which no row tasks.*.vars of the table names"},
		{"a row no task sets", append(slices.Clone(rows), envRow{envAnyTaskVars, "OTHER", envAnyValue}), "the table's row tasks.*.vars OTHER is set by no task's vars"},
		{"a row with a value", valued, `the table's row tasks.*.vars PUBLISH has the value "1"`},
		{"a row of a held variable", append(slices.Clone(rows), envRow{envAnyTaskVars, "GATE", envAnyValue}), "the table's row tasks.*.vars GATE names a held variable"},
	} {
		if len(plant.rows) == len(rows) && slices.Equal(plant.rows, rows) {
			t.Fatalf("the rows plant %s changed nothing", plant.name)
		}
		got, err := envProblems(real, plant.rows)
		if err != nil {
			t.Fatalf("%s: %v", plant.name, err)
		}
		if !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, plant.want) }) {
			t.Errorf("the rows plant %s: got %q, want a problem saying %q", plant.name, got, plant.want)
		}
	}
	// and the table is read as written: a row with no why, or a value that is no code span, is refused
	doc := read(t, "docs/checks.md")
	const gowork = "| `env` | `GOWORK` | `off` |"
	_, rest, ok := strings.Cut(doc, "\n"+gowork)
	if !ok {
		t.Fatalf("docs/checks.md has no line starting %q, which the table plants are made from", gowork)
	}
	why, _, _ := strings.Cut(rest, "\n")
	for _, plant := range []struct{ name, from, to, want string }{
		{"no table", envHeading, "### Another heading", "has no heading"},
		{"a row with no why", gowork + why + "\n", gowork + "  |\n", "says no why"},
		{"a value that is no code span", gowork, "| `env` | `GOWORK` | off |", "is not one code span"},
		{"a row of three cells", gowork + why + "\n", gowork + "\n", "is not four cells"},
	} {
		text := strings.Replace(doc, plant.from, plant.to, 1)
		if text == doc {
			t.Fatalf("the table plant %s changed nothing", plant.name)
		}
		if _, err := envRows(text); err == nil || !strings.Contains(err.Error(), plant.want) {
			t.Errorf("the table plant %s: %v, want an error saying %q", plant.name, err, plant.want)
		}
	}
}
