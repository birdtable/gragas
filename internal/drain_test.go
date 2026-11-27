package internal

import (
	"go/ast"
	"slices"
	"strconv"
	"testing"
)

// drainNamed holds every run's Drain said where it is made
// (gragas#1450): each proc.Cmd literal of the repository, shipped or test,
// names Drain. Since go-proc v0.4.0 a zero Drain reads no pipe a process that
// left the tree still holds once the tree is killed, and Windows refuses it;
// on Unix a site that misses it only cuts its output short with no error, so
// only a check of the source finds it. A Cmd built field by field with no
// literal is the named residual.
func drainNamed(all []source) []string {
	var out []string
	for _, s := range all {
		names := imports(s.file)
		ast.Inspect(s.file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Cmd" {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); !ok || names[x.Name] != "github.com/birdtable/go-proc/proc" {
				return true
			}
			keys := make([]string, 0, len(lit.Elts))
			for _, e := range lit.Elts {
				if kv, ok := e.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); ok {
						keys = append(keys, k.Name)
					}
				}
			}
			if !slices.Contains(keys, "Drain") {
				out = append(out, s.path+":"+strconv.Itoa(s.fset.Position(lit.Pos()).Line)+": a proc.Cmd names no Drain, so a pipe held past the kill is read for none of it")
			}
			return true
		})
	}
	slices.Sort(out)
	return out
}

// TestEveryProcCmdNamesItsDrain: a proc.Cmd literal that names Drain passes,
// under any name its file gives go-proc's proc; one that names none fails by
// its file and line, in a test file as in shipped code; a Cmd of another
// package passes. Planted failures: the rows that drop Drain.
func TestEveryProcCmdNamesItsDrain(t *testing.T) {
	t.Parallel()
	const head = "package tool\n\nimport (\n\t\"time\"\n\n\t\"github.com/birdtable/go-proc/proc\"\n)\n\n"
	rows := []struct {
		claim string
		src   source
		want  string
	}{
		{"a Cmd that names Drain", parse(t, "internal/host/tool", "run.go", head+"var _ = proc.Cmd{Path: \"/x\", Drain: time.Second}\n"), ""},
		{"a Cmd under another name", parse(t, "internal/host/git", "run.go", "package git\n\nimport p \"github.com/birdtable/go-proc/proc\"\n\nvar _ = p.Cmd{Drain: 1}\n"), ""},
		{"a Cmd of another package", parse(t, "internal/host/git", "run.go", "package git\n\nimport \"os/exec\"\n\nvar _ = exec.Cmd{Path: \"/x\"}\n"), ""},
		{"a Cmd that names none", parse(t, "internal/host/tool", "run.go", head+"var _ = time.Second\n\nvar _ = proc.Cmd{Path: \"/x\"}\n"), "internal/host/tool/run.go:11: a proc.Cmd names no Drain"},
		{"an empty Cmd in a test", parse(t, "internal/host/tool", "x_test.go", "package tool\n\nimport \"github.com/birdtable/go-proc/proc\"\n\nvar _ = proc.Cmd{}\n"), "internal/host/tool/x_test.go:5: a proc.Cmd names no Drain"},
		{"a pointer to a Cmd that names none", parse(t, "internal/config/testing", "rerun.go", "package configtest\n\nimport \"github.com/birdtable/go-proc/proc\"\n\nvar _ = &proc.Cmd{Timeout: 1}\n"), "internal/config/testing/rerun.go:5: a proc.Cmd names no Drain"},
	}
	for _, r := range rows {
		got := drainNamed([]source{r.src})
		if r.want != "" {
			fires(t, got, r.want)
		} else if len(got) > 0 {
			t.Errorf("%s: %q, want no finding", r.claim, got)
		}
	}
}
