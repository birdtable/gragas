package audit

import (
	"strings"
	"testing"
	"testing/fstest"
)

// memberFound reads repo's tier pre-push, which asks for one vet member, and
// says whether the audit counted it: no finding "has no vet member".
func memberFound(t *testing.T, repo fstest.MapFS) bool {
	t.Helper()
	in := TierInput{
		Repo:    repo,
		Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
		Tiers: Tiers{Tiers: []Tier{{Name: "pre-push", Members: []Member{{ID: "vet", Rule: "t.3", All: []string{"go vet"}}}}},
			Rules: TierRules{Tier: "t.1", Runs: "t.2"}},
	}
	for _, f := range CheckTiers(in).Findings {
		if strings.Contains(f.Missing, "has no vet member") {
			return false
		}
	}
	return true
}

// TestAnIncludeThatFlattensAliasesOrExcludesIsReadAsGoTaskReadsIt:
// flatten: true puts the include's tasks at the root, so a
// member there is found; an alias of the namespace reaches the member; a
// member the include excludes is not there, so it counts for nothing. Risk: a
// false red on a shape go-task runs, and a false green on a member it does not.
func TestAnIncludeThatFlattensAliasesOrExcludesIsReadAsGoTaskReadsIt(t *testing.T) {
	t.Parallel()
	const inc = "version: '3'\ntasks:\n  vet:\n    cmds: [go vet ./...]\n  lint:\n    cmds: [go vet ./...]\n"
	repo := func(include, deps string) fstest.MapFS {
		return fstest.MapFS{
			"Taskfile.yml": {Data: []byte("version: '3'\nincludes:\n  inc: " + include + "\ntasks:\n  pre-push:\n    deps: [" + deps + "]\n")},
			"inc.yml":      {Data: []byte(inc)},
		}
	}
	cases := []struct {
		name  string
		repo  fstest.MapFS
		found bool
	}{
		{"control: the namespaced member", repo("{taskfile: inc.yml}", "inc:vet"), true},
		{"flatten: the member at the root", repo("{taskfile: inc.yml, flatten: true}", "vet"), true},
		{"flatten: the member by its namespace is not there", repo("{taskfile: inc.yml, flatten: true}", "inc:vet"), false},
		{"no flatten: the member at the root is not there", repo("{taskfile: inc.yml}", "vet"), false},
		{"alias: the member by the alias", repo("{taskfile: inc.yml, aliases: [i]}", "i:vet"), true},
		{"alias: the member by the name", repo("{taskfile: inc.yml, aliases: [i]}", "inc:vet"), true},
		{"no alias: an unnamed prefix finds nothing", repo("{taskfile: inc.yml}", "i:vet"), false},
		{"excludes: the member it removes is not counted", repo("{taskfile: inc.yml, excludes: [vet]}", "inc:vet"), false},
		{"excludes: a member it keeps is", repo("{taskfile: inc.yml, excludes: [lint]}", "inc:vet"), true},
		{"flatten with excludes: the removed member is not at the root", repo("{taskfile: inc.yml, flatten: true, excludes: [vet]}", "vet"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := memberFound(t, c.repo); got != c.found {
				t.Errorf("member counted = %v, want %v", got, c.found)
			}
		})
	}
}

// TestADepWithAForCountsOnlyWhereTheAuditCanReadAnItemToRun: a list with an item, a matrix
// whose every axis is one (written out or a ref to a variable of the file that
// is one) and a variable that is one run the callee, which counts once. An
// empty list, an empty variable, a variable that is a string, an empty axis, a
// ref to an empty list and sources run it never, as far as the audit can tell,
// so the call stays red: each was run against go-task 3.54.0, which starts the
// callee zero times. The shape is gragas's own fan-out of vet, vuln and
// licences. Risk: a project that copies the fan-out is told it has no vet
// member, or one go-task never runs is counted.
func TestADepWithAForCountsOnlyWhereTheAuditCanReadAnItemToRun(t *testing.T) {
	t.Parallel()
	repo := func(forKey, call string) fstest.MapFS {
		return fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\nvars:\n  CROSS: [linux/amd64, windows/amd64]\n  NONE: []\n  EMPTY: ''\n  WORDS: a b\ntasks:\n  vet-system:\n    cmds: [go vet ./...]\n  pre-push:\n" + call + forKey)}}
	}
	deps := func(forBody string) fstest.MapFS {
		return repo("", "    deps:\n      - task: vet-system\n        for: "+forBody+"\n")
	}
	cmds := func(forBody string) fstest.MapFS {
		return repo("", "    cmds:\n      - for: "+forBody+"\n        task: vet-system\n")
	}
	for name, body := range map[string]string{
		"a matrix over a ref":          "{matrix: {PLATFORM: {ref: .CROSS}}}",
		"a matrix with a written axis": "{matrix: {OS: [linux, windows]}}",
		"a matrix of two axes":         "{matrix: {OS: [linux], ARCH: {ref: .CROSS}}}",
		"a variable that is a list":    "{var: CROSS}",
		"a list with items":            "[linux/amd64]",
	} {
		for kind, mk := range map[string]func(string) fstest.MapFS{"deps": deps, "cmds": cmds} {
			if !memberFound(t, mk(body)) {
				t.Errorf("%s, %s: the member the call runs is not counted", kind, name)
			}
		}
	}
	for name, body := range map[string]string{
		"an empty list":                    "[]",
		"a variable that is empty":         "{var: EMPTY}",
		"a variable that is an empty list": "{var: NONE}",
		"a variable that is a string":      "{var: WORDS}",
		"a variable the file lacks":        "{var: LACKING}",
		"a matrix with an empty axis":      "{matrix: {OS: []}}",
		"a matrix with one empty axis":     "{matrix: {OS: [linux], ARCH: []}}",
		"a matrix over a ref to nothing":   "{matrix: {PLATFORM: {ref: .NONE}}}",
		"a matrix over a lacking ref":      "{matrix: {PLATFORM: {ref: .LACKING}}}",
		"a matrix with no axis":            "{matrix: {}}",
		"sources":                          "sources",
	} {
		for kind, mk := range map[string]func(string) fstest.MapFS{"deps": deps, "cmds": cmds} {
			if memberFound(t, mk(body)) {
				t.Errorf("%s, %s: the call runs the callee never, as far as the audit can tell, and its member is counted", kind, name)
			}
		}
	}
	if !memberFound(t, repo("", "    deps:\n      - task: vet-system\n")) {
		t.Error("control: a plain dep is not counted")
	}
	if memberFound(t, repo("", "    deps:\n      - task: vet-system\n        if: ok\n")) {
		t.Error("control: a dep with if: still marks its callee")
	}
}

// TestAGitIncludeWrittenAsAnHttpsAddressWithADotGitPathNeedsARefToo:
// go-task reads https://host/repo.git//Taskfile.yml as a git address,
// so one without ref= is a finding as the git:: spelling is; one with ref= is
// not, and a plain https file is not a git address. Risk: a git include that
// reads the default branch, unseen.
func TestAGitIncludeWrittenAsAnHttpsAddressWithADotGitPathNeedsARefToo(t *testing.T) {
	t.Parallel()
	refFinding := func(address string) bool {
		in := TierInput{
			Repo:    fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\nincludes:\n  r: {taskfile: '" + address + "', checksum: abc}\ntasks:\n  pre-push:\n    cmds: [go vet ./...]\n")}},
			Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
			Tiers:   Tiers{Tiers: []Tier{{Name: "pre-push"}}, Rules: TierRules{Tier: "t.1"}},
		}
		for _, f := range CheckTiers(in).Findings {
			if strings.Contains(f.Key, "include r ref") {
				return true
			}
		}
		return false
	}
	for address, want := range map[string]bool{
		"git::https://example.org/repo.git//Taskfile.yml":          true,
		"https://example.org/repo.git//Taskfile.yml":               true,
		"https://example.org/repo.git//dir/Taskfile.yml":           true,
		"https://example.org/repo.git//Taskfile.yml?ref=v1.2.3":    false,
		"git::https://example.org/repo.git//Taskfile.yml?ref=main": false,
		"https://example.org/Taskfile.yml":                         false,
	} {
		if got := refFinding(address); got != want {
			t.Errorf("%s: a ref finding = %v, want %v", address, got, want)
		}
	}
}

// TestAForOverAVariableTheTaskOrACallSetsAgainSkips: go-task resolves a for: in the
// scope of the task that holds it, where the task's own vars: outrank the file's, and a call's vars: reach the
// task it calls. A matrix over a ref, or a var:, that the file declares as a list with an item runs the callee
// zero times when the task holding the for:, the call that names it or a call on the way sets the name to []
// or "": the call stays red. A name set that the for: does not read changes nothing.
func TestAForOverAVariableTheTaskOrACallSetsAgainSkips(t *testing.T) {
	t.Parallel()
	const head = "version: '3'\nvars:\n  CROSS: [linux/amd64, windows/amd64]\n  LIST: [a]\ntasks:\n  vet-system:\n    cmds: [go vet ./...]\n"
	repo := func(tasks string) fstest.MapFS { return fstest.MapFS{"Taskfile.yml": {Data: []byte(head + tasks)}} }
	matrix, variable := "{matrix: {P: {ref: .CROSS}}}", "{var: LIST}"
	for name, c := range map[string]struct {
		tasks string
		found bool
	}{
		"control: the file's list, a matrix":        {"  pre-push:\n    cmds:\n      - {task: vet-system, for: " + matrix + "}\n", true},
		"control: the file's list, a var:":          {"  pre-push:\n    cmds:\n      - {task: vet-system, for: " + variable + "}\n", true},
		"the task sets the matrix's name to []":     {"  pre-push:\n    vars: {CROSS: []}\n    cmds:\n      - {task: vet-system, for: " + matrix + "}\n", false},
		"the task sets the var: to an empty string": {"  pre-push:\n    vars: {LIST: ''}\n    cmds:\n      - {task: vet-system, for: " + variable + "}\n", false},
		"the task sets the name to a list":          {"  pre-push:\n    vars: {CROSS: [x]}\n    cmds:\n      - {task: vet-system, for: " + matrix + "}\n", false},
		"the call itself sets the name":             {"  pre-push:\n    cmds:\n      - {task: vet-system, for: " + matrix + ", vars: {CROSS: []}}\n", false},
		"a dep sets the name":                       {"  pre-push:\n    deps:\n      - {task: vet-system, for: " + matrix + ", vars: {CROSS: []}}\n", false},
		"a call on the way sets the name":           {"  pre-push:\n    cmds:\n      - {task: mid, vars: {CROSS: []}}\n  mid:\n    cmds:\n      - {task: vet-system, for: " + matrix + "}\n", false},
		"a dep on the way sets the name":            {"  pre-push:\n    deps:\n      - {task: mid, vars: {LIST: ''}}\n  mid:\n    cmds:\n      - {task: vet-system, for: " + variable + "}\n", false},
		"a task on the way sets the name":           {"  pre-push:\n    cmds:\n      - task: mid\n  mid:\n    vars: {CROSS: []}\n    cmds:\n      - {task: vet-system, for: " + matrix + "}\n", false},
		"a name the for: does not read":             {"  pre-push:\n    vars: {OTHER: x}\n    cmds:\n      - {task: vet-system, for: " + matrix + ", vars: {EXTRA: y}}\n", true},
		"a call on the way sets another name":       {"  pre-push:\n    cmds:\n      - {task: mid, vars: {OTHER: x}}\n  mid:\n    cmds:\n      - {task: vet-system, for: " + matrix + "}\n", true},
		"the name set after the for: is read":       {"  pre-push:\n    cmds:\n      - {task: vet-system, for: " + matrix + "}\n      - {task: mid, vars: {CROSS: []}}\n  mid:\n    cmds: [echo ok]\n", true},
	} {
		if got := memberFound(t, repo(c.tasks)); got != c.found {
			t.Errorf("%s: member counted = %v, want %v", name, got, c.found)
		}
	}
}

// TestAToolPathWithItsExeSuffixHoldsTheMember: go-task's {{exeExt}} is ".exe" on Windows and nothing
// elsewhere, so a pinned tool written {{.TOOLS}}/golangci-lint{{exeExt}} run is the member everywhere. Written inside a
// word, after a word that is not the tool, or with a trim marker that joins two words, it is not. Risk: a false red on
// the spelling Windows needs, and a false green on a word that only matches once the suffix is cut out.
func TestAToolPathWithItsExeSuffixHoldsTheMember(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, cmd string
		found     bool
	}{
		{"control: the bare path", "'{{.TOOLS}}/golangci-lint run ./...'", true},
		{"the suffix", "'{{.TOOLS}}/golangci-lint{{exeExt}} run ./...'", true},
		{"the suffix spaced", "'{{.TOOLS}}/golangci-lint{{ exeExt }} run ./...'", true},
		{"inside a word", "'{{.TOOLS}}/golangci-{{exeExt}}lint run ./...'", false},
		{"a trim marker joins the words", "'{{.TOOLS}}/golangci-lint{{- exeExt -}} run ./...'", false},
		{"another template is not the suffix", "'{{.TOOLS}}/golangci-lint{{.EXT}} run ./...'", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			in := TierInput{
				Repo:    fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\ntasks:\n  pre-push:\n    cmds:\n      - cmd: " + c.cmd + "\n        timeout: 5m\n")}},
				Sources: []Source{{Tool: TaskTool, Files: []string{"Taskfile.yml"}}},
				Tiers: Tiers{Tiers: []Tier{{Name: "pre-push", Members: []Member{{ID: "lint", Rule: "t.3", All: []string{"golangci-lint run"}}}}},
					Rules: TierRules{Tier: "t.1", Runs: "t.2"}},
			}
			found := true
			for _, f := range CheckTiers(in).Findings {
				if strings.Contains(f.Missing, "has no lint member") {
					found = false
				}
			}
			if found != c.found {
				t.Errorf("member counted = %v, want %v", found, c.found)
			}
		})
	}
}

// TestAMemberRunUnderTheBudgetIsTheTiersOwn: a tier whose one command is the budget over
// `-- {{.TASK_EXE}} <members>` has the members' commands as its own, and a command that only names a task
// in its words, with no budget before it, holds none. Risk: a false red on every tier of the Taskfile, or a
// false green on a member nothing runs.
func TestAMemberRunUnderTheBudgetIsTheTiersOwn(t *testing.T) {
	t.Parallel()
	repo := func(cmd string) fstest.MapFS {
		return fstest.MapFS{"Taskfile.yml": {Data: []byte("version: '3'\ntasks:\n  members:\n    cmds: [go vet ./...]\n  pre-push:\n    cmds:\n      - cmd: '" + cmd + "'\n        timeout: 5m\n")}}
	}
	cases := []struct {
		name  string
		cmd   string
		found bool
	}{
		{"control: the budget runs the members", "{{.BUDGET}} --tier pre-push -- {{.TASK_EXE}} members X=1", true},
		{"a command that is no budget's holds no member", "echo -- {{.TASK_EXE}} members", false},
		{"a budget over a task that is not there holds none", "{{.BUDGET}} --tier pre-push -- {{.TASK_EXE}} nothing", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := memberFound(t, repo(c.cmd)); got != c.found {
				t.Errorf("member counted = %v, want %v", got, c.found)
			}
		})
	}
}
