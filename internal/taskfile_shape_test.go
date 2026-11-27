package internal

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/ordered"
	"mvdan.cc/sh/v3/syntax"
)

// The commands of a Taskfile member (gragas#1228): go-task runs a member in its own shell
// interpreter, whose builtins are the same on every system, but a program it names is whatever
// the PATH finds, and on Windows find, cp, mktemp, xargs and sed are not GNU's (or not there).
// So a member may call only what is pinned: the bare names go and git, a pinned tool as
// {{.TOOLS}}/<name>, the gate's own commands through {{.GATE}} (go run), and the interpreter's
// builtins. A command word that is not a literal is red. Each exception is named below with its
// reason. The scripts are parsed with the parser go-task's interpreter uses, so they split as the
// run does.

// builtinCommands run inside the interpreter, whatever the system.
func builtinCommands() []string {
	return []string{
		"[", ":", "true", "false", "echo", "printf", "exit", "return", "read", "cd", "export", "unset", "set", "shift",
		"local", "test", "continue", "break", "wait", "pwd",
	}
}

// pinnedTools are the programs of tools.yaml a member may run, as {{.TOOLS}}/<name> only.
func pinnedTools() []string {
	return []string{
		"changie", "gitleaks", "go-licenses", "golangci-lint", "goreleaser", "govulncheck", "hatch", "node", "pnpm", "ast-grep", "task", "vale", "actionlint", "zizmor",
	}
}

// builtTools are the programs a tier builds from this tree and runs as {{.TOOLS}}/<name>: not a download a pin holds
// (gragas#2367, the gate-build task).
func builtTools() []string { return []string{"gragas-dev"} }

// shapeExceptions are the tasks that name a program outside the list, each with its reason.
func shapeExceptions() map[string]map[string]string {
	return map[string]map[string]string{
		// release-build needs none since gragas#1655 (X211): it is gragas-release build.
		"release-undraft": {
			"gh": "the release recipe publishes with the GitHub CLI of the machine that releases (not Windows)",
		},
		// secrets-scan needs none since gragas#1655 (X171): the pass is gragas-dev secrets.
	}
}

// shapeProblems parses the shell script (its templates already rendered) and names every command
// word that is not allowed: allow adds the programs the task is excepted from.
func shapeProblems(script string, allow map[string]string) []string {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "member")
	if err != nil {
		return []string{"does not parse: " + err.Error()}
	}
	var problems []string
	funcs := map[string]bool{}
	syntax.Walk(file, func(n syntax.Node) bool {
		if f, ok := n.(*syntax.FuncDecl); ok {
			funcs[f.Name.Value] = true
		}
		return true
	})
	var walk func(*syntax.File)
	check := func(call *syntax.CallExpr) {
		if len(call.Args) == 0 {
			return
		}
		word, ok := literal(call.Args[0])
		if !ok {
			problems = append(problems, "a command word that is not a literal: "+printed(call.Args[0]))
			return
		}
		args := call.Args[1:]
		switch word {
		case "command":
			if len(args) == 0 {
				return
			}
			if v, _ := literal(args[0]); v == "-v" {
				return
			}
			problems = append(problems, "command runs its argument: only command -v is allowed")
			return
		case "exec":
			if len(args) == 0 {
				return
			}
			next, ok := literal(args[0])
			if !ok || !allowedWord(next, allow, funcs) {
				problems = append(problems, "exec runs "+printed(args[0]))
			}
			return
		case "trap":
			if len(args) > 0 {
				if code, ok := literal(args[0]); ok && code != "-" && code != "" {
					inner, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(code), "trap")
					if err != nil {
						problems = append(problems, "a trap whose code does not parse")
						return
					}
					walk(inner)
				} else if !ok {
					problems = append(problems, "a trap whose code is not a literal")
				}
			}
			return
		case "env", "eval", "source", ".", "xargs", "sh", "bash", "zsh":
			problems = append(problems, word+" runs other code")
			return
		}
		if !allowedWord(word, allow, funcs) {
			problems = append(problems, "runs "+word)
		}
	}
	walk = func(f *syntax.File) {
		syntax.Walk(f, func(n syntax.Node) bool {
			if call, ok := n.(*syntax.CallExpr); ok {
				check(call)
			}
			return true
		})
	}
	walk(file)
	return problems
}

func allowedWord(word string, allow map[string]string, funcs map[string]bool) bool {
	if slices.Contains(builtinCommands(), word) || funcs[word] || word == "go" || word == "git" {
		return true
	}
	if _, ok := allow[word]; ok {
		return true
	}
	if name, ok := strings.CutPrefix(word, "/T/"); ok && (slices.Contains(pinnedTools(), name) || slices.Contains(builtTools(), name)) {
		return true
	}
	return false
}

// literal is the text of a word made only of literal, single-quoted and literal double-quoted parts.
func literal(w *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, p := range w.Parts {
		switch p := p.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				l, ok := q.(*syntax.Lit)
				if !ok {
					return "", false
				}
				b.WriteString(l.Value)
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

func printed(w *syntax.Word) string {
	var b strings.Builder
	_ = syntax.NewPrinter().Print(&b, w)
	return b.String()
}

var action = regexp.MustCompile(`\{\{-?\s*([^}]*?)\s*-?\}\}`)

// render makes a Taskfile string a script: {{.TOOLS}} is /T, {{exeExt}} is nothing, a variable of the file is its text
// (rendered the same way), a branch's tags are dropped so both branches are read, and any other
// template is a literal word.
func render(s string, vars map[string]string, depth int) string {
	return action.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.TrimSpace(strings.Trim(action.FindStringSubmatch(m)[1], "-"))
		switch {
		case inner == ".TOOLS":
			return "/T"
		case inner == "exeExt":
			// The program's suffix on Windows, nothing elsewhere (gragas#1566): /T/<name> stays the pinned tool's word.
			return ""
		case strings.HasPrefix(inner, "if "), strings.HasPrefix(inner, "else"), inner == "end", strings.HasPrefix(inner, "range "), strings.HasPrefix(inner, "with "):
			return ""
		}
		if name, ok := strings.CutPrefix(inner, "."); ok {
			if v, ok := vars[name]; ok && depth < 5 {
				return render(v, vars, depth+1)
			}
		}
		return "X"
	})
}

// taskfileScripts returns, for each task, every script the task runs: its commands, the dynamic
// variables (sh:) and the environment (env: with sh:) of the file and the task, and its
// preconditions, status and defer. The file's own dynamic variables and environment are held under
// the name "(vars)".
func taskfileScripts(t *testing.T) map[string][]string {
	t.Helper()
	return scriptsOf(t, read(t, "Taskfile.yml"))
}

// scriptsOf is taskfileScripts over the text of a Taskfile.
func scriptsOf(t *testing.T, text string) map[string][]string {
	t.Helper()
	doc, err := ordered.ParseYAML([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	top, ok := doc.(*ordered.Map)
	if !ok {
		t.Fatal("the Taskfile is not a mapping")
	}
	vars := map[string]string{}
	// dynamics are the sh: scripts of a vars mapping; the strings of the file's vars are kept for splicing.
	dynamics := func(m *ordered.Map, into map[string]string) []string {
		var sh []string
		if m == nil {
			return nil
		}
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			switch v := v.(type) {
			case string:
				if into != nil {
					into[k] = v
				}
			case *ordered.Map:
				if s := v.Str("sh"); s != "" {
					sh = append(sh, s)
				}
			}
		}
		return sh
	}
	fileLevel := append(dynamics(top.Sub("vars"), vars), dynamics(top.Sub("env"), nil)...)
	shell := func(v any) []string {
		switch v := v.(type) {
		case string:
			return []string{v}
		case *ordered.Map:
			var out []string
			for _, key := range []string{"cmd", "sh", "defer"} {
				if s := v.Str(key); s != "" {
					out = append(out, s)
				}
			}
			return out
		}
		return nil
	}
	out := map[string][]string{}
	tasks := top.Sub("tasks")
	if tasks == nil {
		t.Fatal("the Taskfile has no tasks")
	}
	for _, name := range tasks.Keys() {
		task := tasks.Sub(name)
		var scripts []string
		for _, key := range []string{"cmds", "preconditions", "status"} {
			for _, c := range task.List(key) {
				scripts = append(scripts, shell(c)...)
			}
		}
		scripts = append(scripts, dynamics(task.Sub("vars"), nil)...)
		scripts = append(scripts, dynamics(task.Sub("env"), nil)...)
		for i, s := range scripts {
			scripts[i] = render(s, vars, 0)
		}
		out[name] = scripts
	}
	for _, s := range fileLevel {
		out["(vars)"] = append(out["(vars)"], render(s, vars, 0))
	}
	return out
}

func TestEveryMemberCommandIsAPinnedProgramOrABuiltin(t *testing.T) {
	t.Parallel()
	scripts := taskfileScripts(t)
	if len(scripts) < 30 {
		t.Fatalf("read %d tasks, want the whole Taskfile", len(scripts))
	}
	if len(scripts["(vars)"]) == 0 {
		t.Fatal("read no dynamic variable of the file: the check is not reading them")
	}
	exceptions := shapeExceptions()
	seen := map[string]bool{}
	for name, list := range scripts {
		for _, s := range list {
			for _, p := range shapeProblems(s, exceptions[name]) {
				t.Errorf("task %s: %s", name, p)
			}
			// An exception is used when the script without it is red for that program: a
			// substring is not enough, since ast-grep holds grep (gragas#1655).
			for _, p := range shapeProblems(s, nil) {
				for prog := range exceptions[name] {
					if p == "runs "+prog || p == "exec runs "+prog {
						seen[name+" "+prog] = true
					}
				}
			}
		}
	}
	for name, progs := range exceptions {
		for prog := range progs {
			if !seen[name+" "+prog] {
				t.Errorf("the exception for %s of task %s is not used: drop it", prog, name)
			}
		}
	}
}

// TestTheShapeCheckReadsTheShVariablesAndEnvOfTheFileAndOfATask plants a program in each place go-task
// runs a shell without a cmds entry: a sh: variable of the file or of a task, and a sh: environment
// entry of the file or of a task. Each is in the scripts the check reads (gragas#1262).
func TestTheShapeCheckReadsTheShVariablesAndEnvOfTheFileAndOfATask(t *testing.T) {
	t.Parallel()
	got := scriptsOf(t, `version: "3"
vars:
  A: {sh: "find . -name a"}
env:
  B: {sh: "find . -name b"}
tasks:
  m:
    vars:
      C: {sh: "find . -name c"}
    env:
      D: {sh: "find . -name d"}
      E: plain
    cmds:
      - go test ./...
`)
	all := strings.Join(append(got["(vars)"], got["m"]...), "\n")
	for _, name := range []string{"a", "b", "c", "d"} {
		if !strings.Contains(all, "find . -name "+name) {
			t.Errorf("the planted find in the %s entry is not read", name)
		}
	}
	if strings.Contains(all, "plain") {
		t.Error("a plain env value is read as a script")
	}
}

// TestAPlantedProgramInAMemberIsRed holds the shape check to each way a program can be named.
func TestAPlantedProgramInAMemberIsRed(t *testing.T) {
	t.Parallel()
	red := []string{
		"find . -name '*.go'",
		"cp a b",
		"mktemp -d",
		"x=$(mktemp)",
		"echo a | xargs echo",
		"sed -i s/a/b/ f",
		"if find . ; then :; fi",
		"while find . ; do :; done",
		"! find .",
		"`find .`",
		"( find . )",
		"{ find . ; }",
		"cat <<EOF\n$(find .)\nEOF",
		"command find .",
		"exec find .",
		"trap 'find .' EXIT",
		"X=1 find .",
		"env find .",
		"$t run",
		"\"$root/bin/gofmt\" -l .",
		"golangci-lint run",
		"vale docs",
		"node build.mjs",
		"/T/unpinned run",
		"sh -c 'find .'",
		"f() { find .; }; f",
	}
	for _, s := range red {
		if len(shapeProblems(s, nil)) == 0 {
			t.Errorf("planted %q is green, want red", s)
		}
	}
	green := []string{
		"go test ./...",
		"git rev-parse HEAD",
		"/T/golangci-lint run ./...",
		"/T/vale docs",
		"/T/node build.mjs",
		"read -r want < .node-version\n[ \"$want\" = x ] || exit 1",
		"command -v git >" + shellNull,
		"trap 'exit 1' EXIT",
		"x=$(go env GOROOT)",
		"guard() { [ \"$1\" = x ]; }\nguard a",
		"for c in a b; do git -C \"$c\" status; done",
	}
	for _, s := range green {
		if p := shapeProblems(s, nil); len(p) != 0 {
			t.Errorf("%q is red, want green: %v", s, p)
		}
	}
	if p := shapeProblems("rm -f x", map[string]string{"rm": "a reason"}); len(p) != 0 {
		t.Errorf("a named exception is red: %v", p)
	}
	_ = fmt.Sprint
}
