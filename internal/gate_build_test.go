package internal

import (
	"regexp"
	"strings"
	"testing"
)

// sectionOf is a task's text in the given Taskfile text, as taskSection reads it from the file.
func sectionOf(t *testing.T, text, name string) string {
	t.Helper()
	i := strings.Index(text, "\n  "+name+":\n")
	if i < 0 {
		t.Fatalf("the Taskfile has no task %s", name)
	}
	body := text[i+1:]
	if j := regexp.MustCompile(`\n  [a-z][a-z-]*:\n`).FindStringIndex(body); j != nil {
		body = body[:j[0]+1]
	}
	return body
}

// TestEachTierBuildsTheGatesProgramOnceBeforeItsFirstGateCall (gragas#2367): GATE is the binary gate-build builds, never
// go run, which hashed gragas-dev's import graph at the start of every member and timed out a member's step on a cold
// cache; no go run of ./cmd/gragas-dev is left in the Taskfile's commands, gate-build is one go build of it, and the first
// step of each tier, and of each task a person runs on its own that calls GATE, is gate-build. Planted: a go run of the
// gate's program, and a tier that begins with a gate call.
func TestEachTierBuildsTheGatesProgramOnceBeforeItsFirstGateCall(t *testing.T) {
	t.Parallel()
	taskfile := read(t, "Taskfile.yml")
	problems := func(taskfile string, section func(string) string) []string {
		var bad []string
		for n, l := range strings.Split(taskfile, "\n") {
			code, _, _ := strings.Cut(l, " #")
			if strings.Contains(code, "go run") && strings.Contains(code, "cmd/gragas-dev") && !strings.HasPrefix(strings.TrimSpace(l), "#") {
				bad = append(bad, "Taskfile.yml:"+itoa(n+1)+" runs gragas-dev with go run: "+strings.TrimSpace(l))
			}
		}
		first := taskCmds(section("gate-build"))
		if len(first) == 0 || first[0] != `cmd: "{{.GOFLAGS_GUARD}}"` {
			bad = append(bad, "gate-build's first command is not the GOFLAGS_GUARD, so a caller's GOFLAGS, GOWORK, GOTOOLCHAIN or GOPROXY reaches the build")
		}
		if len(first) < 2 || !strings.Contains(first[1], "go build -trimpath -o") || !strings.Contains(first[1], "./cmd/gragas-dev") {
			bad = append(bad, "gate-build's second command is not go build -trimpath -o of ./cmd/gragas-dev")
		}
		for _, name := range []string{"pre-commit", "pre-push", "full", "landing", "release", "release-publish", "gen", "tools", "scan-names", "self-check-issues"} {
			if cmds := taskCmds(section(name)); len(cmds) == 0 || cmds[0] != "task: gate-build" {
				bad = append(bad, name+" does not begin with task: gate-build")
			}
		}
		return bad
	}
	whole := func(name string) string { return taskSection(t, name) }
	if bad := problems(taskfile, whole); len(bad) != 0 {
		t.Errorf("the tiers (gragas#2367): %v", bad)
	}
	for _, p := range []struct{ what, from, to string }{
		{"a go run of the gate's program", "go build -trimpath -o \"{{.TOOLS}}/gragas-dev{{exeExt}}\" ./cmd/gragas-dev", "go run -trimpath ./cmd/gragas-dev"},
		{"a gate-build that builds before the guard", "      - cmd: \"{{.GOFLAGS_GUARD}}\"\n        timeout: 1m\n      - cmd: 'go build -trimpath -o \"{{.TOOLS}}/gragas-dev{{exeExt}}\" ./cmd/gragas-dev'\n        timeout: 5m\n", "      - cmd: 'go build -trimpath -o \"{{.TOOLS}}/gragas-dev{{exeExt}}\" ./cmd/gragas-dev'\n        timeout: 5m\n      - cmd: \"{{.GOFLAGS_GUARD}}\"\n        timeout: 1m\n"},
		{"a tier that begins with a gate call", "      - task: gate-build\n        timeout: 5m\n      - cmd: '{{.GATE}} landing begin", "      - cmd: '{{.GATE}} landing begin"},
	} {
		planted := strings.Replace(taskfile, p.from, p.to, 1)
		if planted == taskfile {
			t.Fatalf("%s: the Taskfile has no %q", p.what, p.from)
		}
		if len(problems(planted, func(name string) string { return sectionOf(t, planted, name) })) == 0 {
			t.Errorf("%s is not red", p.what)
		}
	}
}
