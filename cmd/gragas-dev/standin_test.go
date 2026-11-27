package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The tests start this test binary under three other names: as
// .tools/bin/task, the stand-in task each hook runs, and as hookrun, gragas-dev itself, which
// a scratch clone's one-line hook runs in place of go run ./cmd/gragas-dev; the tools-check
// tests start it as node, which prints a planted version line. TestMain looks at
// the name it was started under first. The stand-in is a child and no test: its answer is
// its exit code and the line it logs, which holds the environment it was handed.

// standinFile is the stand-in task's settings, beside it, one key=value a line: log, the file
// each call appends a line to; root, the clone, where a call is @here and anywhere else
// @away; red, a file of the tree that, holding "red", fails pre-commit and pre-push;
// fail, the tasks that fail, by spaces; hang, the tasks that wait an hour, by spaces;
// and show, the variables whose values each call logs, by spaces.
const standinFile = "task.cfg"

// standInChild is this test binary started as one of the hook tests' programs: as task, it
// answers as the stand-in; as hookrun, it is gragas-dev, whose main exits. ok is false when
// it was started under its own name, as a run of the tests, and it read nothing but
// os.Args[0].
func standInChild() (code int, ok bool) {
	switch startedAs() {
	case "task":
		return standinTask(), true
	case "hookrun":
		main()
	case "node":
		return standinNode(), true
	}
	return 0, false
}

// standinNodeFile is what the stand-in node prints for node --version, beside it, byte for
// byte: the tools-check tests plant node's Windows line, a carriage return before its newline.
const standinNodeFile = "node.out"

// standinNode is the stand-in node of the tools-check tests (gragas#1655, X106): it answers
// --version with the bytes of standinNodeFile beside it, and anything else with exit 9.
func standinNode() int {
	// tools-check starts node by its path with the bare word as Args[0], so the file is found
	// beside the program itself.
	self, err := os.Executable()
	if err != nil {
		return 9
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(self), standinNodeFile))
	if err != nil || len(os.Args) != 2 || os.Args[1] != "--version" {
		return 9
	}
	if _, err := os.Stdout.Write(b); err != nil {
		return 9
	}
	return 0
}

// startedAs is the name this process was started under, less the system's suffix: task,
// hookrun, node, or "" for the test binary's own.
func startedAs() string {
	base := filepath.Base(os.Args[0])
	for _, name := range []string{"task", "hookrun", "node"} {
		if base == name || strings.HasPrefix(base, name+".") {
			return name
		}
	}
	return ""
}

// standinTask is the stand-in task. It logs its arguments, for tools-pnpm the store and the
// offline setting pnpm is given, each of plantedSwitches and plantedLeaks it is given, the
// values of the variables the settings show, and where it ran; it
// refuses a throwaway tree with a node_modules in it (pnpm refuses one that is a link,
// gragas#1033) or without the clone's .tools linked in, logging why; and it fails a task the
// settings name, or a pre-commit or pre-push over a red tree. The hook starts task by its absolute path, so the settings are
// beside os.Args[0].
func standinTask() int {
	b, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), standinFile))
	if err != nil || len(os.Args) < 2 {
		return 9
	}
	cfg := map[string]string{}
	for line := range strings.SplitSeq(string(b), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			cfg[k] = v
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return 9
	}
	args := os.Args[1:]
	where := "@away"
	if sameDir(wd, cfg["root"]) {
		where = "@here"
	}
	// A task the settings kill ends itself by a signal and logs nothing: its exit code is -1 to the hook.
	if slices.Contains(strings.Fields(cfg["kill"]), args[0]) {
		if self, err := os.FindProcess(os.Getpid()); err == nil {
			_ = self.Kill()
		}
		select {} // a kill is not instant
	}
	line := strings.Join(args, " ")
	if args[0] == "tools-pnpm" {
		line += " store=" + os.Getenv("pnpm_config_store_dir") + " offline=" + os.Getenv("pnpm_config_offline")
	}
	// A planted variable is given when it holds its planted value: the hook sets GOENV
	// itself, to off.
	for _, kv := range append(plantedSwitches(), plantedLeaks()...) {
		if k, v, _ := strings.Cut(kv, "="); os.Getenv(k) == v {
			line += " given " + k
		}
	}
	for k := range strings.FieldsSeq(cfg["show"]) {
		line += " " + k + "=" + os.Getenv(k)
	}
	// A task the settings hang waits past any run's timeout a test gives it, so the
	// run is ended, its tree killed (gragas#2263); it logs nothing.
	if slices.Contains(strings.Fields(cfg["hang"]), args[0]) {
		time.Sleep(time.Hour)
		return 9
	}
	code := 0
	switch {
	case slices.Contains(strings.Fields(cfg["fail"]), args[0]):
		code = 1
	case where == "@away" && exists(filepath.Join(wd, "explorer", "node_modules")):
		line, code = line+" refused: the throwaway tree holds an explorer/node_modules", 3
	case where == "@away" && !exists(filepath.Join(wd, ".tools", "bin")):
		line, code = line+" refused: the throwaway tree has no .tools linked in", 4
	case cfg["red"] != "" && (args[0] == "pre-commit" || args[0] == "pre-push"):
		if red, _ := os.ReadFile(filepath.Join(wd, cfg["red"])); strings.Contains(string(red), "red") {
			code = 1
		}
	}
	// As go-task does, TASK_DRY makes any task a dry run that exits 0 (gragas#2161).
	if os.Getenv("TASK_DRY") != "" {
		code = 0
	}
	// The log is read and written back with the line after it: the one open that
	// appends is Writer.Append's (store.state-files.5). One run of this program
	// writes at a time: each test has its own log, and the hooks of a git
	// operation run one after another, so no two processes write one log at once.
	old, err := os.ReadFile(cfg["log"])
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 9
	}
	if err := os.WriteFile(cfg["log"], append(old, []byte(line+" "+where+"\n")...), 0o600); err != nil {
		return 9
	}
	return code
}

// exists is whether p is there, as a link or anything else.
func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// sameDir is whether a and b are one folder, links resolved.
func sameDir(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
