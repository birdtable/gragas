package main

import (
	"bytes"
	"errors"
	"github.com/birdtable/go-files/filestest"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/proc"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// fakeTaskConfig is what a test asks of the stand-in task (standin_test.go).
type fakeTaskConfig struct {
	Red  string   // a file of the tree that, holding "red", fails pre-commit and pre-push
	Fail []string // the tasks that fail
	Hang []string // the tasks that wait an hour, past any timeout a test gives
	Show []string // the variables whose values each call logs, in this order
	Kill []string // the tasks that end themselves by a signal, logging nothing (gragas#2408)
}

// installAs puts this test binary into dir as name, under the file name a program
// named so has here (proc.LocalFileName), and answers its path: a hard link to the
// binary where the file system makes one, which writes nothing, and else a copy,
// through WriteProgram (gragas#1233).
func installAs(t *testing.T, dir, name string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, proc.LocalFileName(name))
	if os.Link(exe, p) == nil {
		return p
	}
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	filestest.WriteProgram(t, p, b)
	return p
}

// fakeTaskIn puts the stand-in task at dir's .tools/bin with cfg, dir its root, and answers
// the log it writes. .tools, explorer/node_modules and the log stay out of git's sight.
func fakeTaskIn(t *testing.T, dir string, cfg fakeTaskConfig) string {
	t.Helper()
	bin := filepath.Join(dir, ".tools", "bin")
	installAs(t, bin, "task")
	log := filepath.Join(t.TempDir(), "calls")
	put(t, bin, standinFile, "log="+log+"\nroot="+dir+"\nred="+cfg.Red+"\nfail="+strings.Join(cfg.Fail, " ")+"\nhang="+strings.Join(cfg.Hang, " ")+"\nshow="+strings.Join(cfg.Show, " ")+"\nkill="+strings.Join(cfg.Kill, " ")+"\n")
	put(t, dir, ".git/info/exclude", ".tools\nexplorer/node_modules\n")
	return log
}

// taskCalls is the stand-in task's log, one call a line, or nothing when it was never called.
func taskCalls(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// modulesNaming is a .modules.yaml, which pnpm writes as JSON, that names store. A Go quoted
// string of a path is a JSON one.
func modulesNaming(store string) string {
	return `{"storeDir": ` + strconv.Quote(store) + "}"
}

// hookClone is a repository on branch topic with the hook sources committed and two commits,
// the stand-in task at its .tools/bin with cfg, and, with store, the store its explorer install
// recorded (gragas#1522). It answers the folder, the task's log, the first commit and HEAD.
func hookClone(t *testing.T, cfg fakeTaskConfig, store string) (dir, log, first, second string) {
	t.Helper()
	dir = t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "topic")
	for _, f := range hookSources() {
		put(t, dir, f, "the hook source "+f+"\n")
	}
	put(t, dir, "a.txt", "one\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "one")
	first = gitIn(t, dir, "rev-parse", "HEAD")
	gitIn(t, dir, "update-ref", mainRef, first)
	put(t, dir, "a.txt", "two\n")
	gitIn(t, dir, "commit", "-q", "-am", "two")
	second = gitIn(t, dir, "rev-parse", "HEAD")
	log = fakeTaskIn(t, dir, cfg)
	if store != "" {
		put(t, dir, "explorer/node_modules/.modules.yaml", modulesNaming(store))
	}
	return dir, log, first, second
}

// prePushIn runs the pre-push hook in dir, in process, as git would for a push of sha to each
// of refs, and answers its code and what it printed.
func prePushIn(t *testing.T, dir, sha string, refs ...string) (int, string) {
	t.Helper()
	var stdin strings.Builder
	for _, ref := range refs {
		stdin.WriteString("HEAD " + sha + " " + ref + " 0000000000000000000000000000000000000000\n")
	}
	var out bytes.Buffer
	h, err := newHook(t.Context(), dir, configtest.Inherited(), &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	code := h.prePush(t.Context(), strings.NewReader(stdin.String()))
	return code, out.String()
}

// TestThePrePushHookTurnsTheSkipOnUnlessMainIsPushed holds the ruling of gragas#1311 on the hook: it
// passes SKIP_UNCHANGED=1 to task pre-push, for a pushed branch whether the tree is the checked-out one
// or a throwaway worktree, and 0 when any pushed ref is refs/heads/main, so the push that lands runs every member.
// A throwaway worktree first gets its own packages, by task tools-pnpm with pnpm_config_offline from the store the
// clone's install recorded (gragas#1522); the checked-out tree has its own already, and gets no install. The
// throwaway tree has the clone's .tools linked in and no node_modules (the stand-in refuses either), and when
// it is gone the clone's .tools is whole and git lists no worktree but the clone's.
func TestThePrePushHookTurnsTheSkipOnUnlessMainIsPushed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		away bool     // the pushed commit is not the checked-out one
		refs []string // remote refs pushed
		want string
	}{
		{"a branch from the checked-out tree", false, []string{"refs/heads/topic"}, "SKIP_UNCHANGED=1"},
		{"main from the checked-out tree", false, []string{"refs/heads/main"}, "SKIP_UNCHANGED=0"},
		{"a branch and main together", false, []string{"refs/heads/topic", "refs/heads/main"}, "SKIP_UNCHANGED=0"},
		{"a branch from a throwaway worktree", true, []string{"refs/heads/topic"}, "SKIP_UNCHANGED=1"},
		{"main from a throwaway worktree", true, []string{"refs/heads/main"}, "SKIP_UNCHANGED=0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			// A store with a space and a quote, as a path may hold, reaches pnpm as one value.
			store := filepath.Join(t.TempDir(), "the store's v11")
			dir, log, first, second := hookClone(t, fakeTaskConfig{}, store)
			sha := second
			if c.away {
				sha = first
			}
			if code, out := prePushIn(t, dir, sha, c.refs...); code != 0 {
				t.Fatalf("the hook failed with %d:\n%s", code, out)
			}
			want := []string{"pre-push " + c.want + " @here"}
			if c.away {
				want = []string{"tools-pnpm store=" + store + " offline=true @away", "pre-push " + c.want + " @away"}
			}
			if got := taskCalls(t, log); !slices.Equal(got, want) {
				t.Errorf("the hook called task with %q, want %q", got, want)
			}
			if _, err := os.Stat(filepath.Join(dir, ".tools", "bin", standinFile)); err != nil {
				t.Errorf("the clone's .tools is not whole after the hook: %v", err)
			}
			if got := gitIn(t, dir, "worktree", "list", "--porcelain"); strings.Count(got, "worktree ") != 1 {
				t.Errorf("a throwaway worktree is left:\n%s", got)
			}
		})
	}
}

// plantedSwitches are one variable of each kind switchPrefixes names, as a caller would set it for a
// commit or a push to green a red tier (gragas#2161), two of them in another case than their tool's.
func plantedSwitches() []string {
	return []string{
		"TASK_DRY=true", "task_silent=true",
		"GOLANGCI_DIFF_PROCESSOR_PATCH=none", "GOLANGCI_LINT_CACHEPROG=x",
		"pnpm_config_script_shell=x", "NPM_CONFIG_SCRIPT_SHELL=x",
		"NODE_OPTIONS=--require=x", "GOCACHEPROG=x", "GOENV=x",
	}
}

// TestTheHookGivesTaskNoSwitchOfATool holds gragas#2161: a variable that makes a tool of the tiers pass
// what it should not (go-task's TASK_DRY runs nothing and exits 0, golangci-lint's diff patch drops every
// finding, pnpm's script-shell and NODE_OPTIONS replace what runs, GOCACHEPROG answers for the test cache,
// and GOENV names a file go reads GOCACHEPROG from),
// set for a commit or a push, reaches no task the hook starts, in the checked-out tree or a throwaway
// worktree, whatever its case. Nor does any of plantedLeaks, which no prefix names and the allowlist drops
// (gragas#2181): GIT_CONFIG_COUNT, GOFLAGS, GOMODCACHE and GOEXPERIMENT among them. The hook's own pnpm
// settings for the offline install still reach it. Planted failure: the cut by switchPrefixes in place of
// allowed, and every one of plantedLeaks is given.
func TestTheHookGivesTaskNoSwitchOfATool(t *testing.T) {
	t.Parallel()
	store := filepath.Join(t.TempDir(), "store")
	dir, log, first, second := hookClone(t, fakeTaskConfig{}, store)
	var out bytes.Buffer
	planted := append(append(configtest.Inherited(), plantedSwitches()...), plantedLeaks()...)
	h, err := newHook(t.Context(), dir, planted, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if code := h.preCommit(t.Context()); code != 0 {
		t.Fatalf("the pre-commit hook failed with %d:\n%s", code, out.String())
	}
	const zero = "0000000000000000000000000000000000000000"
	push := "HEAD " + second + " refs/heads/topic " + zero + "\nHEAD " + first + " refs/heads/older " + zero + "\n"
	if code := h.prePush(t.Context(), strings.NewReader(push)); code != 0 {
		t.Fatalf("the pre-push hook failed with %d:\n%s", code, out.String())
	}
	want := []string{
		"pre-commit @here",
		"pre-push SKIP_UNCHANGED=1 @here",
		"tools-pnpm store=" + store + " offline=true @away",
		"pre-push SKIP_UNCHANGED=1 @away",
	}
	if got := taskCalls(t, log); !slices.Equal(got, want) {
		t.Errorf("the hook called task with %q, want %q: a caller's switch reached it", got, want)
	}
}

// plantedLeaks are variables the allowlist drops that no prefix of switchPrefixes names (gragas#2181), as a
// caller would set them for a commit or a push: git's config from the environment, go's own flags, caches,
// experiments and cgo, one of them in another case than go's.
func plantedLeaks() []string {
	return []string{
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.pager", "GIT_CONFIG_VALUE_0=planted", "GIT_CONFIG_PARAMETERS='core.pager'='planted'",
		"GOFLAGS=-planted", "GOMODCACHE=planted", "GOCACHE=planted", "gocache=planted", "GOEXPERIMENT=planted", "GODEBUG=planted=1", "CGO_ENABLED=planted",
	}
}

// TestGoenvOffIsNoSwitch (gragas#2389): GOENV=off, exactly, is what allowed gives every member and so what landing run
// gives the landing; it makes go read no file of variables, and is no switch. Any other GOENV is one: a path, an
// empty value, another case of the value, and the name in lower case. Planted failure: GOENV=off refused as any GOENV
// is, as before.
func TestGoenvOffIsNoSwitch(t *testing.T) {
	t.Parallel()
	for kv, want := range map[string]bool{
		"GOENV=off": false, "GOENV=/some/file": true, "GOENV=": true, "GOENV": true, "goenv=x": true, "GOENV=OFF": true, "goenv=off": true,
	} {
		if got := isSwitch(kv); got != want {
			t.Errorf("isSwitch(%q) = %v, want %v", kv, got, want)
		}
	}
}

// TestIsSwitchNamesOnlyTheToolsPrefixes holds the refusal of landing begin, check and merge to its prefixes: a
// name that only holds one, or is one less a letter, is no switch, and neither is a variable a member needs from its
// caller.
func TestIsSwitchNamesOnlyTheToolsPrefixes(t *testing.T) {
	t.Parallel()
	keep := []string{"GOWORK=off", "SKIP_UNCHANGED=1", "GOFLAGS=-mod=readonly", "GOCACHE=c", "PNPM_HOME=p", "NODE_ENV=test",
		"MY_TASK_DRY=1", "TASKS=1", "GOLANGCI=1", "NPM_CONFIG=1", "pnpm_config=1"}
	got := slices.DeleteFunc(append(slices.Clone(keep), plantedSwitches()...), isSwitch)
	if !slices.Equal(got, keep) {
		t.Errorf("the switch cut kept %q, want %q", got, keep)
	}
}

// TestHookKeptGivesEachNameItsReason holds the gate's table (gragas#2181): each name has a reason, is listed once
// in any case, and is none of the system's, which go-proc names (proc.Needed, proc.HomeEnv).
func TestHookKeptGivesEachNameItsReason(t *testing.T) {
	t.Parallel()
	system := proc.Needed()
	for _, kv := range proc.HomeEnv(proc.NeutralDir()) {
		k, _, _ := strings.Cut(kv, "=")
		system = append(system, k)
	}
	seen := map[string]bool{}
	for _, k := range hookKept() {
		if strings.TrimSpace(k.why) == "" {
			t.Errorf("%s is kept with no reason", k.name)
		}
		if seen[strings.ToUpper(k.name)] {
			t.Errorf("%s is listed twice", k.name)
		}
		seen[strings.ToUpper(k.name)] = true
		if slices.ContainsFunc(system, func(n string) bool { return strings.EqualFold(n, k.name) }) {
			t.Errorf("%s is a system variable, which go-proc names, not the gate", k.name)
		}
	}
}

// TestThePreCommitHookGivesTaskTheIndexGitNames holds the one GIT_ name pre-commit hands on (gragas#2181): git
// commit -a or with paths commits from a temporary index it names in GIT_INDEX_FILE, and pre-commit's staged checks
// read that index. Planted failure: preCommit run with h.env alone, and the stand-in is given none.
func TestThePreCommitHookGivesTaskTheIndexGitNames(t *testing.T) {
	t.Parallel()
	dir, log, _, _ := hookClone(t, fakeTaskConfig{Show: []string{"GIT_INDEX_FILE"}}, "")
	index := filepath.Join(dir, ".git", "index")
	var out bytes.Buffer
	h, err := newHook(t.Context(), dir, append(configtest.Inherited(), "GIT_INDEX_FILE="+index), &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if code := h.preCommit(t.Context()); code != 0 {
		t.Fatalf("the pre-commit hook failed with %d:\n%s", code, out.String())
	}
	want := []string{"pre-commit GIT_INDEX_FILE=" + index + " @here"}
	if got := taskCalls(t, log); !slices.Equal(got, want) {
		t.Errorf("the hook called task with %q, want %q", got, want)
	}
}

// TestThePrePushHookGivesNoGitTheCallersIndex holds an index route shut: git
// names an index to pre-commit alone, so a GIT_INDEX_FILE at pre-push is the caller's, and an empty one, a path to no
// file, lists no tracked file. It reaches neither the hook's own git, which then finds the clean checkout clean and
// runs task pre-push there, nor task, in the checked-out tree or a throwaway worktree. Planted failure: GIT_INDEX_FILE
// back in hookKept, and the push is refused before any member runs: the hook's own git diff of the hook sources reads
// the empty index, so every source is untracked and the pushed hook reads as another.
func TestThePrePushHookGivesNoGitTheCallersIndex(t *testing.T) {
	t.Parallel()
	for _, away := range []bool{false, true} {
		store := filepath.Join(t.TempDir(), "store")
		dir, log, first, second := hookClone(t, fakeTaskConfig{Show: []string{"GIT_INDEX_FILE"}}, store)
		sha, want := second, []string{"pre-push SKIP_UNCHANGED=1 GIT_INDEX_FILE= @here"}
		if away {
			sha, want = first, []string{"tools-pnpm store=" + store + " offline=true GIT_INDEX_FILE= @away", "pre-push SKIP_UNCHANGED=1 GIT_INDEX_FILE= @away"}
		}
		var out bytes.Buffer
		callers := "GIT_INDEX_FILE=" + filepath.Join(t.TempDir(), "callers-index")
		h, err := newHook(t.Context(), dir, append(configtest.Inherited(), callers), &out, &out)
		if err != nil {
			t.Fatal(err)
		}
		push := "HEAD " + sha + " refs/heads/topic 0000000000000000000000000000000000000000\n"
		if code := h.prePush(t.Context(), strings.NewReader(push)); code != 0 {
			t.Fatalf("the pre-push hook failed with %d:\n%s", code, out.String())
		}
		if got := taskCalls(t, log); !slices.Equal(got, want) {
			t.Errorf("the hook called task with %q, want %q: the caller's index reached a git", got, want)
		}
	}
}

// TestTheHookReadsGoPrivateFromThePersonsOwnFileAlone holds the private-module names:
// GOPRIVATE, GONOPROXY and GONOSUMDB reach a tier as go reads them from the person's own go env -w file at
// its default place under the home, and a GOENV the caller sets names no file they are read from; every member's go
// has GOENV off. Planted failure: goPrivate run with the caller's GOENV, and the caller's file's GONOSUMDB reaches the
// tier, with no GOPRIVATE beside it.
func TestTheHookReadsGoPrivateFromThePersonsOwnFileAlone(t *testing.T) {
	t.Parallel()
	home := proc.HomeEnv(t.TempDir())
	var names []string
	for _, kv := range home {
		k, v, _ := strings.Cut(kv, "=")
		names = append(names, k)
		if err := os.MkdirAll(v, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// GONOPROXY=none is testenv.GoOffline's (gragas#2283): the person's own file is what this test reads it from.
	env := append(withoutVars(configtest.Inherited(), append(names, "GOENV", "GONOPROXY")), home...)
	cmd := exec.CommandContext(t.Context(), "go", "env", "GOENV")
	cmd.Env = env
	b, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Dir(strings.TrimSpace(string(b))), filepath.Base(strings.TrimSpace(string(b))), "GOPRIVATE=person.example/*\n")
	callers := filepath.Join(t.TempDir(), "env")
	put(t, filepath.Dir(callers), filepath.Base(callers), "GONOSUMDB=planted.example/*\n")
	show := append(goPrivateNames(), "GOENV")
	dir, log, _, _ := hookClone(t, fakeTaskConfig{Show: show}, "")
	var out bytes.Buffer
	h, err := newHook(t.Context(), dir, append(env, "GOENV="+callers), &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if code := h.preCommit(t.Context()); code != 0 {
		t.Fatalf("the pre-commit hook failed with %d:\n%s", code, out.String())
	}
	want := []string{"pre-commit GOPRIVATE=person.example/* GONOPROXY=person.example/* GONOSUMDB=person.example/* GOENV=off @here"}
	if got := taskCalls(t, log); !slices.Equal(got, want) {
		t.Errorf("the hook called task with %q, want %q", got, want)
	}
}

// TestThePrePushHookRefusesAThrowawayTreeItCannotInstallOffline holds the hook's install of a throwaway
// worktree's packages (gragas#1522): with no store recorded in the clone, or with an offline install
// that fails, the push is refused with the fix, and task pre-push never runs on a tree with no packages.
func TestThePrePushHookRefusesAThrowawayTreeItCannotInstallOffline(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		fail    []string // the tasks the stand-in fails
		store   string   // the store the clone's install recorded; "" records none
		modules string   // the clone's .modules.yaml in place of the one the store makes, when not ""
		say     string
		calls   []string
	}{
		{"no install in the clone", nil, "", "", "names no pnpm store", nil},
		{"an install that recorded no store", nil, "/store", `{"layoutVersion": 5}`, "names no pnpm store", nil},
		{"an install that recorded an empty store", nil, "/store", `{"storeDir": ""}`, "names no pnpm store", nil},
		{"a store that is not text", nil, "/store", `{"storeDir": 5}`, "names no pnpm store", nil},
		{"a file that is not JSON", nil, "/store", "storeDir: /store\n", "names no pnpm store", nil},
		{"an offline install that fails", []string{"tools-pnpm"}, "/store", "", "the offline install of", []string{"tools-pnpm store=/store offline=true @away"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir, log, first, _ := hookClone(t, fakeTaskConfig{Fail: c.fail}, c.store)
			if c.modules != "" {
				put(t, dir, "explorer/node_modules/.modules.yaml", c.modules)
			}
			code, out := prePushIn(t, dir, first, "refs/heads/topic")
			if code == 0 {
				t.Fatalf("the hook passed a tree it could not install:\n%s", out)
			}
			if !strings.Contains(out, c.say) || !strings.Contains(out, "run  task tools  in this clone") {
				t.Errorf("the refusal does not say %q and name task tools:\n%s", c.say, out)
			}
			if got := taskCalls(t, log); !slices.Equal(got, c.calls) {
				t.Errorf("the hook called task with %q, want %q", got, c.calls)
			}
		})
	}
}

// TestThePrePushHookRefusesACommitWhoseHookDiffersFromItsOwn: a hook judges only a commit that carries
// its sources as the checkout holds them (gragas#1297), the code that runs being the checkout's. A branch
// whose hook differs, pushed from another checkout, is refused, and no task runs; the same branch pushed
// from its own checkout passes. Each of the hook sources is planted in turn. A push that only deletes
// refs runs no task.
func TestThePrePushHookRefusesACommitWhoseHookDiffersFromItsOwn(t *testing.T) {
	t.Parallel()
	for _, src := range hookSources() {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			dir, log, _, _ := hookClone(t, fakeTaskConfig{}, filepath.Join(t.TempDir(), "store"))
			gitIn(t, dir, "switch", "-q", "-c", "other")
			put(t, dir, src, "another hook\n")
			gitIn(t, dir, "commit", "-q", "-am", "another hook")
			other := gitIn(t, dir, "rev-parse", "HEAD")
			gitIn(t, dir, "switch", "-q", "topic")
			code, out := prePushIn(t, dir, other, "refs/heads/other")
			if code == 0 || !strings.Contains(out, "(its hook differs)") {
				t.Fatalf("a branch whose %s differs from the checkout's was not refused for it (code %d):\n%s", src, code, out)
			}
			if got := taskCalls(t, log); got != nil {
				t.Errorf("a refused push ran task: %q", got)
			}
			gitIn(t, dir, "switch", "-q", "other")
			if code, out := prePushIn(t, dir, other, "refs/heads/other"); code != 0 {
				t.Fatalf("the same branch pushed from its own checkout was refused (code %d):\n%s", code, out)
			}
		})
	}
	t.Run("only deletes", func(t *testing.T) {
		t.Parallel()
		dir, log, _, _ := hookClone(t, fakeTaskConfig{}, "")
		code, out := prePushIn(t, dir, "0000000000000000000000000000000000000000", "refs/heads/gone")
		if code != 0 || !strings.Contains(out, "only deletes refs/heads/gone") {
			t.Errorf("a push that only deletes was not passed as such (code %d):\n%s", code, out)
		}
		if got := taskCalls(t, log); got != nil {
			t.Errorf("a push that only deletes ran task: %q", got)
		}
	})
}

// TestATaskEndedByASignalIsRefusedAndSaysSo: a task that does not run to its end is refused with
// exitFailed, and the hook names the task and logs nothing for it, never a silent red (gragas#2408).
// The words that say what ended it are a system's own (Unix says "signal: killed"), so none is asserted.
func TestATaskEndedByASignalIsRefusedAndSaysSo(t *testing.T) {
	t.Parallel()
	dir, log, _, second := hookClone(t, fakeTaskConfig{Kill: []string{"pre-push"}}, "")
	code, out := prePushIn(t, dir, second, "refs/heads/topic")
	// go-proc answers a run a signal ended as an error, which taskEnd gives as a run that did not reach its end.
	if code != exitFailed || !strings.Contains(out, "task pre-push did not run to its end") {
		t.Errorf("a pre-push that did not run to its end gave code %d, want %d and the words 'task pre-push did not run to its end':\n%s", code, exitFailed, out)
	}
	if got := taskCalls(t, log); got != nil {
		t.Errorf("a task that ended itself logged %q", got)
	}
}

// TestTheHookCommandRefusesWhatItDoesNotUnderstand:a hook it does not know, or pre-commit with
// arguments, is the usage, and runs nothing.
func TestTheHookCommandRefusesWhatItDoesNotUnderstand(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"post-commit"}, {"pre-commit", "x"}, {"pre-push", "origin", "url", "more"}} {
		var out, errs bytes.Buffer
		if code := runHook(t.Context(), configtest.Inherited(), args, strings.NewReader(""), &out, &errs); code != exitUsage || !strings.Contains(errs.String(), "gragas-dev hook pre-commit") {
			t.Errorf("hook %q: code %d, want the usage:\n%s", args, code, errs.String())
		}
	}
}

// TestAFreshCloneRunsItsHooksAndAPlantedFailureStopsTheCommitAndThePush runs git itself: a scratch
// clone's hooks are each one line, as the repository's are, that run gragas-dev hook, here this test
// binary started as gragas-dev, and its task is the stand-in, red while state.txt says red.
// Planted: a red state, which stops the commit; a red commit pushed while its fix waits uncommitted in the
// working tree, which stops the push; and a red branch pushed from a green checkout, which stops it too.
// A green commit passes and names the tier in each case; each throwaway tree is installed first, by
// tools-pnpm, and nothing refused reaches the remote.
func TestAFreshCloneRunsItsHooksAndAPlantedFailureStopsTheCommitAndThePush(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	run := filepath.ToSlash(installAs(t, filepath.Join(base, "bin"), "hookrun"))
	env := append(configtest.Inherited(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Alex", "GIT_AUTHOR_EMAIL=alex@example.com", "GIT_COMMITTER_NAME=Alex", "GIT_COMMITTER_EMAIL=alex@example.com")
	attempt := func(dir string, args ...string) (string, error) {
		cmd := withArgs(exec.CommandContext(t.Context(), "git"), args)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := attempt(dir, args...)
		if err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
		}
		return out
	}
	remote := filepath.Join(base, "remote.git")
	git(base, "init", "-q", "--bare", "-b", "main", remote)
	clone := filepath.Join(base, "clone")
	git(base, "clone", "-q", remote, clone)
	git(clone, "switch", "-q", "-c", "main")
	// The hooks are scripts git runs, from testdata/hooks with the installed
	// program's path in place of @RUN@, written through WriteProgram (gragas#1233).
	if err := os.MkdirAll(filepath.Join(clone, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, hook := range []string{"pre-commit", "pre-push"} {
		script, err := os.ReadFile(filepath.Join("testdata", "hooks", hook))
		if err != nil {
			t.Fatal(err)
		}
		filestest.WriteProgram(t, filepath.Join(clone, "hooks", hook), bytes.ReplaceAll(script, []byte("@RUN@"), []byte(run)))
	}
	put(t, clone, "state.txt", "green\n")
	git(clone, "add", "-A")
	git(clone, "commit", "-q", "-m", "base")
	git(clone, "config", "core.hooksPath", "hooks")
	log := fakeTaskIn(t, clone, fakeTaskConfig{Red: "state.txt"})
	put(t, clone, "explorer/node_modules/.modules.yaml", modulesNaming("/store"))
	git(clone, "push", "-q", "--no-verify", "origin", "main")

	put(t, clone, "state.txt", "red\n")
	git(clone, "add", "state.txt")
	if out, err := attempt(clone, "commit", "-q", "-m", "planted"); err == nil {
		t.Fatalf("a red pre-commit task did not stop the commit\n%s", out)
	}
	git(clone, "commit", "-q", "--no-verify", "-m", "red")
	// The fix waits in the working tree; the pushed commit is the red one.
	put(t, clone, "state.txt", "green\n")
	// A push is stopped by the red task, not by any other refusal: its output names task pre-push
	// failing on the pushed commit, and a refusal of another kind is shown whole.
	const redStop = "task pre-push failed on"
	if out, err := attempt(clone, "push", "-q", "origin", "main"); err == nil || !strings.Contains(out, redStop) {
		t.Fatalf("a red commit pushed with a green working tree was not stopped by the red task (err %v):\n%s", err, out)
	}
	git(clone, "branch", "red")
	git(clone, "add", "state.txt")
	git(clone, "commit", "-q", "-m", "fix")
	if out, err := attempt(clone, "push", "-q", "origin", "red"); err == nil || !strings.Contains(out, redStop) {
		t.Fatalf("a red branch pushed from a green checkout was not stopped by the red task (err %v):\n%s", err, out)
	}
	if out := git(remote, "branch", "--list"); strings.Contains(out, "red") || strings.Contains(git(remote, "log", "--format=%s", "main"), "red") {
		t.Fatalf("a refused push reached the remote: %s", out)
	}
	git(clone, "push", "-q", "origin", "main")
	var tiers []string
	for _, c := range taskCalls(t, log) {
		tiers = append(tiers, strings.Fields(c)[0])
	}
	if want := []string{"pre-commit", "tools-pnpm", "pre-push", "pre-commit", "tools-pnpm", "pre-push", "pre-push"}; !slices.Equal(tiers, want) {
		var missing []string
		rest := slices.Clone(tiers)
		for _, w := range want {
			if i := slices.Index(rest, w); i >= 0 {
				rest = slices.Delete(rest, i, i+1)
			} else {
				missing = append(missing, w)
			}
		}
		t.Errorf("the tiers run were %q, want %q: missing %q, unexpected %q", tiers, want, missing, rest)
	}
}

// FuzzParseStore holds parseStore over any bytes (tests.generated.1): it never panics, it
// answers a store exactly when that store is not empty, and it answers the same twice.
func FuzzParseStore(f *testing.F) {
	for _, seed := range []string{modulesNaming("/the store's v11"), `{"layoutVersion": 5}`, `{"storeDir": ""}`, `{"storeDir": 5}`, "storeDir: /store\n", `{"storeDir": null}`, "{"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		store, ok := parseStore(data)
		if ok != (store != "") {
			t.Fatalf("parseStore(%q) = %q, %v: a store is answered exactly when it is not empty", data, store, ok)
		}
		if again, ok2 := parseStore(data); again != store || ok2 != ok {
			t.Fatalf("parseStore(%q) answered %q, %v and then %q, %v", data, store, ok, again, ok2)
		}
	})
}

func FuzzParseGoPrivate(f *testing.F) {
	for _, seed := range []string{
		`{"GOPRIVATE":"example.com/*","GONOPROXY":"","GONOSUMDB":"example.com/*"}`,
		`{}`, `null`, `[]`, `{"GOPRIVATE": 5}`, `{"goprivate":"x"}`, `{"GOPRIVATE":"a=b"}`, "{",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		set, err := parseGoPrivate(data)
		if err != nil && set != nil {
			t.Fatalf("parseGoPrivate(%q) = %q with error %v: an error answers no variables", data, set, err)
		}
		names, next := goPrivateNames(), 0
		for _, kv := range set {
			k, v, _ := strings.Cut(kv, "=")
			j := slices.Index(names[next:], k)
			if j < 0 || v == "" {
				t.Fatalf("parseGoPrivate(%q) = %q: %q is not a set name of goPrivateNames, once and in order", data, set, kv)
			}
			next += j + 1
		}
		if again, err2 := parseGoPrivate(data); !slices.Equal(again, set) || (err2 == nil) != (err == nil) {
			t.Fatalf("parseGoPrivate(%q) answered %q, %v and then %q, %v", data, set, err, again, err2)
		}
	})
}
