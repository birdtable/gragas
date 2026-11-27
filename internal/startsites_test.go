package internal

import (
	"go/ast"
	"slices"
	"strings"
	"testing"
)

// startSites holds the one place each process runner starts a program
// (gragas#920, cli.process.1): the shipped code of internal/host/tool and
// internal/host/git calls go-proc's proc.Run in the one function the table
// names and nowhere else, calls none of os/exec's Command, CommandContext
// or LookPath, and imports neither syscall nor golang.org/x/sys, so a
// process group, a signal or a limit is go-proc's alone. os/exec may still
// be imported for its ExitError, which proc.Run hands back for a death by a
// signal not the run's.
func startSites(all []source) []string {
	sites := map[string]string{"internal/host/tool": "start", "internal/host/git": "runWith"}
	runs := map[string]int{}
	var out []string
	for _, s := range all {
		at, ok := sites[s.pkg]
		if !ok || s.test {
			continue
		}
		names := imports(s.file)
		for name, p := range names {
			if p == "syscall" || p == "golang.org/x/sys" || strings.HasPrefix(p, "golang.org/x/sys/") {
				out = append(out, s.path+" imports "+p+" ("+name+"): a process group, a signal or a limit is go-proc's")
			}
		}
		for _, d := range s.file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				x, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				switch p := names[x.Name]; {
				case p == "os/exec" && slices.Contains([]string{"Command", "CommandContext", "LookPath"}, sel.Sel.Name):
					out = append(out, s.path+": "+fn.Name.Name+" calls exec."+sel.Sel.Name+", and a program starts only through proc.Run in "+at)
				case p == "github.com/birdtable/go-proc/proc" && sel.Sel.Name == "Run":
					runs[s.pkg]++
					if fn.Name.Name != at {
						out = append(out, s.path+": "+fn.Name.Name+" calls proc.Run, and "+s.pkg+" starts a program only in "+at)
					}
				}
				return true
			})
		}
	}
	for pkg, at := range sites {
		if n := runs[pkg]; n > 1 {
			out = append(out, pkg+": "+at+" calls proc.Run more than once")
		}
	}
	slices.Sort(out)
	return out
}

// TestEachRunnerHasOneStartSite: a runner that calls proc.Run once, in its
// own function, passes; a second call, a call elsewhere, an exec.Command
// and an import of syscall each fail by name. Planted failures: each row.
func TestEachRunnerHasOneStartSite(t *testing.T) {
	t.Parallel()
	const head = "package tool\n\nimport (\n\t\"os/exec\"\n\t\"syscall\"\n\n\t\"github.com/birdtable/go-proc/proc\"\n)\n\nvar _ = syscall.Getpid\nvar _ *exec.ExitError\n\n"
	good := parse(t, "internal/host/tool", "run.go", "package tool\n\nimport (\n\t\"os/exec\"\n\n\t\"github.com/birdtable/go-proc/proc\"\n)\n\nvar _ *exec.ExitError\n\nfunc start() { _, _ = proc.Run(nil, proc.Cmd{}) }\n")
	rows := []struct {
		claim string
		all   []source
		want  string
	}{
		{"one call, in start", []source{good}, ""},
		{"a test file that starts a program its own way", []source{good, parse(t, "internal/host/tool", "x_test.go", "package tool\n\nimport \"os/exec\"\n\nfunc f() { _ = exec.Command(\"x\") }\n")}, ""},
		{"a second call in start", []source{parse(t, "internal/host/tool", "run.go", "package tool\n\nimport \"github.com/birdtable/go-proc/proc\"\n\nfunc start() {\n\t_, _ = proc.Run(nil, proc.Cmd{})\n\t_, _ = proc.Run(nil, proc.Cmd{})\n}\n")}, "start calls proc.Run more than once"},
		{"a call elsewhere", []source{good, parse(t, "internal/host/tool", "find.go", "package tool\n\nimport \"github.com/birdtable/go-proc/proc\"\n\nfunc find() { _, _ = proc.Run(nil, proc.Cmd{}) }\n")}, "find calls proc.Run"},
		{"an exec.Command in git", []source{parse(t, "internal/host/git", "run.go", "package git\n\nimport \"os/exec\"\n\nfunc runWith() { _ = exec.Command(\"git\") }\n")}, "runWith calls exec.Command"},
		{"an exec.CommandContext under another name", []source{parse(t, "internal/host/git", "run.go", "package git\n\nimport x \"os/exec\"\n\nfunc other() { _ = x.CommandContext(nil, \"git\") }\n")}, "other calls exec.CommandContext"},
		{"an import of syscall", []source{parse(t, "internal/host/tool", "group.go", head+"func start() { _, _ = proc.Run(nil, proc.Cmd{}) }\n")}, "imports syscall"},
		{"an import of x/sys/unix", []source{parse(t, "internal/host/git", "kill.go", "package git\n\nimport \"golang.org/x/sys/unix\"\n\nvar _ = unix.Kill\n")}, "imports golang.org/x/sys/unix"},
		{"a package that is no runner", []source{parse(t, "internal/host/fs", "lock.go", "package fs\n\nimport \"syscall\"\n\nvar _ = syscall.Flock\n")}, ""},
	}
	for _, r := range rows {
		got := startSites(r.all)
		if r.want != "" {
			fires(t, got, r.want)
		} else if len(got) > 0 {
			t.Errorf("%s: %q, want no finding", r.claim, got)
		}
	}
}
