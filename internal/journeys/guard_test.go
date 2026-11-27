package journeys

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-console/uitest"
	testenv "github.com/birdtable/go-testkit"
)

// hookEdit is the input an agent's PreToolUse hook gives on standard input
// for one Edit of file in dir.
func hookEdit(t *testing.T, dir, file, old, new string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"session_id": "journey", "hook_event_name": "PreToolUse", "cwd": dir, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": filepath.Join(dir, file), "old_string": old, "new_string": new},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// guardRun runs guard edit in dir with input on standard input, as the hook
// runs it.
func guardRun(t *testing.T, dir string, env map[string]string, input string, args ...string) result {
	t.Helper()
	c := command(t, dir, env, append([]string{"guard", "edit"}, args...)...)
	c.Stdin = strings.NewReader(input)
	var out, errb strings.Builder
	c.Stdout, c.Stderr = &out, &errb
	code := 0
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("could not run gragas: %v", err)
		}
		code = ee.ExitCode()
	}
	return toolchainMasked(t, result{stdout: out.String(), stderr: errb.String(), code: code})
}

const guardRules = "language/go/rules/errors.yaml"

// TestTheGuardEditGoldens: a hand edit of a status, refused, and an edit of
// the statement's wording, done, as a pipe at 80, 60 and 40 columns and under
// --json, each against its golden, written only with -update by a person who
// reads the diff (tests.generated.5, cli.look.8).
// Risk: the refusal an agent reads changes shape unseen, or stops naming the
// field and the command.
func TestTheGuardEditGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, old, new string
		code           int
	}{
		{"guard-edit-refused", "status: proposed", "status: active", 5},
		{"guard-edit-done", "statement: Wrap an error.", "statement: Wrap an error with what was being done.", 0},
	}
	for _, tc := range cases {
		for _, cols := range []string{"80", "60", "40", "json"} {
			dir := historyRepo(t, "")
			env, _ := folders(t)
			env["GRAGAS_NAMES_FILE"] = ""
			var args []string
			name := tc.name + "-" + cols
			if cols == "json" {
				env["COLUMNS"], args, name = "80", []string{"--json"}, "json-"+tc.name
			} else {
				env["COLUMNS"] = cols
			}
			r := guardRun(t, dir, env, hookEdit(t, dir, guardRules, tc.old, tc.new), args...)
			if r.code != tc.code {
				t.Fatalf("%s: exit %d, want %d\n%s%s", name, r.code, tc.code, r.stdout, r.stderr)
			}
			if tc.code == 5 && !strings.Contains(r.stderr, "refused: ") {
				t.Errorf("%s: the refusal is not on standard error, where the hook hands it to the agent:\n%s", name, r.stderr)
			}
			uitest.Golden(t, name, transcript(append([]string{"guard", "edit"}, args...), r))
		}
	}
}

// TestTheGuardRefusesAStatusEditAndTheRetirePlanMakesIt: the same change of
// a status is refused by guard edit when an agent makes it by hand, and made
// by rule retire with its history entry, in one test (M2-03).
// Risk: the guard refuses what the CLI writes, or lets a hand edit through
// that the CLI would have recorded.
func TestTheGuardRefusesAStatusEditAndTheRetirePlanMakesIt(t *testing.T) {
	t.Parallel()
	dir, env := historyRepo(t, ""), ruleEnv(t)
	env["COLUMNS"] = "80"
	r := guardRun(t, dir, env, hookEdit(t, dir, guardRules, "status: proposed", "status: retired"))
	if r.code != 5 || !strings.Contains(r.stdout, "status") || !strings.Contains(r.stderr, "instead: gragas rule retire go.errors.2") {
		t.Fatalf("the hand edit: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if strings.Contains(read(t, dir, guardRules), "status: retired") {
		t.Fatal("the guard wrote the edit it refused")
	}
	if p := run(t, dir, env, with([]string{"rule", "retire", "go.errors.2"}, append(why(), "--apply")...)...); p.code != 0 {
		t.Fatalf("rule retire: exit %d\n%s%s", p.code, p.stdout, p.stderr)
	}
	if !strings.Contains(read(t, dir, guardRules), "status: retired") || hist(t, dir) != 1 {
		t.Fatal("rule retire did not make the change with its history entry")
	}
	if l := run(t, dir, env, "journal", "list", "--json"); !strings.Contains(l.stdout, "gragas guard edit") {
		t.Errorf("the journal does not list the guard's refusal:\n%s%s", l.stdout, l.stderr)
	}
}

// TestAGuardOverAJournalOfAnOlderFormStillRefusesAndNamesIt: the refusal is
// journalled by an append that reads nothing first, and over a journal with
// a line of an older release's form that append is refused. The
// guard still refuses the edit, writes nothing, and its ending names the
// journal and the fix. Risk: a guard whose journal cannot take its refusal
// lets the edit through, or ends on a refusal that does not say why.
func TestAGuardOverAJournalOfAnOlderFormStillRefusesAndNamesIt(t *testing.T) {
	t.Parallel()
	dir := historyRepo(t, "")
	env, state := folders(t)
	env["COLUMNS"], env["GRAGAS_NAMES_FILE"] = "80", ""
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	journal := sum(filepath.ToSlash(real)) + ".journal"
	put(t, state, map[string]string{journal: `{"v":1,"run":"r1","kind":"intent","at":1779000000,"root":"x"}` + "\n"})
	r := guardRun(t, dir, env, hookEdit(t, dir, guardRules, "status: proposed", "status: active"))
	if r.code != 5 || !strings.Contains(r.stderr, "refused: ") {
		t.Fatalf("exit %d, want the refusal, 5\n%s%s", r.code, r.stdout, r.stderr)
	}
	said := r.stdout + r.stderr
	for _, want := range []string{"could not be journalled", journal, "older release", "gragas journal recover", "remove the journal"} {
		if !strings.Contains(said, want) {
			t.Errorf("the ending does not name %q:\n%s", want, said)
		}
	}
	if strings.Contains(read(t, dir, guardRules), "status: active") {
		t.Error("the guard wrote the edit it refused")
	}
}

// TestTheGuardRefusesWhatItCannotRead: empty input, input that is not JSON and
// a file outside the repository each end refused, exit 5; a prose edit is
// the control and ends done.
func TestTheGuardRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	dir := historyRepo(t, "")
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	outside := t.TempDir()
	cases := map[string]struct {
		input string
		code  int
	}{
		"empty":    {"", 5},
		"not JSON": {"status: active\n", 5},
		"outside":  {hookEdit(t, outside, "errors.yaml", "a", "b"), 5},
		"control":  {hookEdit(t, dir, guardRules, "Wrap an error.", "Wrap an error, and name the step that failed."), 0},
	}
	for name, tc := range cases {
		if r := guardRun(t, dir, env, tc.input); r.code != tc.code {
			t.Errorf("%s: exit %d, want %d\n%s%s", name, r.code, tc.code, r.stdout, r.stderr)
		}
	}
}

// TestTheGuardRefusesARepeatedKey: a hook input
// that names file_path twice, or a content key twice, ends refused, exit 5.
// Each planted input is one a reader of the last value would allow, a prose
// edit of the rules file, so a guard that read it so would let it through.
func TestTheGuardRefusesARepeatedKey(t *testing.T) {
	t.Parallel()
	dir := historyRepo(t, "")
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	q := func(s string) string {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	rules, prose := q(filepath.Join(dir, guardRules)), q("Wrap an error, and name the step that failed.")
	cases := map[string]string{
		"file_path twice":  `{"tool_name":"Edit","tool_input":{"file_path":` + q(filepath.Join(t.TempDir(), "errors.yaml")) + `,"file_path":` + rules + `,"old_string":"Wrap an error.","new_string":` + prose + `}}`,
		"new_string twice": `{"tool_name":"Edit","tool_input":{"file_path":` + rules + `,"old_string":"Wrap an error.","new_string":"status: active","new_string":` + prose + `}}`,
		"control":          `{"tool_name":"Edit","tool_input":{"file_path":` + rules + `,"old_string":"Wrap an error.","new_string":` + prose + `}}`,
	}
	for name, input := range cases {
		want := 5
		if name == "control" {
			want = 0
		}
		r := guardRun(t, dir, env, input)
		if r.code != want {
			t.Errorf("%s: exit %d, want %d\n%s%s", name, r.code, want, r.stdout, r.stderr)
		}
		if want == 5 && !strings.Contains(r.stdout+r.stderr, "twice") {
			t.Errorf("%s: the refusal does not say the key is there twice\n%s%s", name, r.stdout, r.stderr)
		}
	}
}

// TestTheGuardRefusesAFolderNamedInAnotherCase (review of #971, item 1;
// Fable's finding 1): a status edit of the rules file by a folder named in
// another case, and a new file written through such a folder, end refused,
// exit 5, as spelled otherwise, on every host: the name folds to one the
// repository has, whether or not the host finds it. The control is the
// file's own name.
func TestTheGuardRefusesAFolderNamedInAnotherCase(t *testing.T) {
	t.Parallel()
	dir := historyRepo(t, "")
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	for _, input := range []string{
		hookEdit(t, dir, "Language/go/rules/errors.yaml", "status: proposed", "status: active"),
		hookEdit(t, dir, "language/Go/rules/errors.yaml", "status: proposed", "status: active"),
		hookWrite(t, dir, "language/Go/rules/new.yaml", "format: 3\n"),
	} {
		if r := guardRun(t, dir, env, input); r.code != 5 || !strings.Contains(r.stdout+r.stderr, "spelled otherwise") {
			t.Errorf("%s: exit %d, want 5 as spelled otherwise\n%s%s", input, r.code, r.stdout, r.stderr)
		}
	}
	if r := guardRun(t, dir, env, hookEdit(t, dir, guardRules, "status: proposed", "status: active")); r.code != 5 || strings.Contains(r.stdout+r.stderr, "spelled otherwise") {
		t.Errorf("the file's own name: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestTheGuardRefusesAnEditThroughAFileLink: the check is on the disk. Planted: with ln.yaml a link to the
// rule file, an edit of the statement's wording by the link's path, done at
// the rule file, ends refused, exit 5, naming the file the link leads to.
// With host/fs's ErrLinked check taken out, Within placed it at the rule file
// and it ended done. Control: the same edit by the rule file's own path, with
// the link in place, ends done. Risk: a host tool that writes a new file and
// renames it over replaces the link with a file at ln.yaml that the guard
// never judged there, while the guard judged the rule file.
func TestTheGuardRefusesAnEditThroughAFileLink(t *testing.T) {
	t.Parallel()
	const old, new = "statement: Wrap an error.", "statement: Wrap an error with what was being done."
	dir := historyRepo(t, "")
	testenv.Link(t, filepath.Join(dir, filepath.FromSlash(guardRules)), filepath.Join(dir, "ln.yaml"))
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	r := guardRun(t, dir, env, hookEdit(t, dir, "ln.yaml", old, new))
	if r.code != 5 || !strings.Contains(r.stdout+r.stderr, "is a link") || !strings.Contains(r.stdout+r.stderr, guardRules) {
		t.Errorf("an edit by the link's path: exit %d, want 5 as a link naming %s\n%s%s", r.code, guardRules, r.stdout, r.stderr)
	}
	if r := guardRun(t, dir, env, hookEdit(t, dir, guardRules, old, new)); r.code != 0 {
		t.Errorf("control: the edit by the rule file's own path: exit %d, want 0\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestTheGuardRefusesAnEditToAFileWithASecondName: the check is on the disk. Planted: with twin.yaml a second
// name of the rule file, by a hard link, an edit of the statement's wording
// by either name ends refused, exit 5, saying the file has more than one
// name. With the guard's read by ReadSoleWithin put back to Read, the same edit is
// allowed in core/api's TestTheGuardRefusesAnEditToAFileWithASecondName,
// over the fake; this journey holds the refusal over a real hard link, where
// host/fs counts the names. Control: the same edit, once twin.yaml is removed, ends done. Risk: the
// guard judges the edit by the one name it is given, and the file changes
// under the other too, unjudged.
func TestTheGuardRefusesAnEditToAFileWithASecondName(t *testing.T) {
	t.Parallel()
	const old, new = "statement: Wrap an error.", "statement: Wrap an error with what was being done."
	dir := historyRepo(t, "")
	twin := filepath.Join(dir, "twin.yaml")
	if err := os.Link(filepath.Join(dir, filepath.FromSlash(guardRules)), twin); err != nil {
		t.Fatal(err)
	}
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	for _, at := range []string{guardRules, "twin.yaml"} {
		r := guardRun(t, dir, env, hookEdit(t, dir, at, old, new))
		if r.code != 5 || !strings.Contains(r.stdout+r.stderr, "more than one name") {
			t.Errorf("an edit by %s: exit %d, want 5 as a file with more than one name\n%s%s", at, r.code, r.stdout, r.stderr)
		}
	}
	if err := os.Remove(twin); err != nil {
		t.Fatal(err)
	}
	if r := guardRun(t, dir, env, hookEdit(t, dir, guardRules, old, new)); r.code != 0 {
		t.Errorf("control: the edit once the file has one name: exit %d, want 0\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// hookWrite is the input an agent's PreToolUse hook gives for one Write of
// file in dir.
func hookWrite(t *testing.T, dir, file, content string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"session_id": "journey", "hook_event_name": "PreToolUse", "cwd": dir, "tool_name": "Write",
		"tool_input": map[string]any{"file_path": filepath.Join(dir, file), "content": content},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestTheGuardRefusesANewNameTheLoaderReadsAsAnother (Fable's finding 1 on
// #971): a new file named as one the loader reads, in another case, and a new
// name a host reads as another (an end in a dot, a colon's stream, an 8.3
// short name) each end refused, exit 5, on the disk of every host; the
// disk's tree refuses a colon before the guard reads the name. A backslash
// is a name's character off Windows, refused there, and a separator on
// Windows, where notes\x.md is the plain new file notes/x.md and is done
// (a review finding on Windows). The control is a plain new name.
// Risk: History.jsonl is a new file to the guard and the history to the
// loader once a host that folds case writes it.
func TestTheGuardRefusesANewNameTheLoaderReadsAsAnother(t *testing.T) {
	t.Parallel()
	dir := historyRepo(t, "")
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	const entry = `{"at":"2026-11-15","entity":"rule:go.errors.1","field":"status","from":"proposed","to":"active"}` + "\n"
	for at, why := range map[string]string{
		"History.jsonl":   "spelled otherwise",
		"Exclusions.yaml": "spelled otherwise",
		"Bindings/x.yaml": "spelled otherwise",
		"history.jsonl.":  "ends in a dot or a space",
		"history.jsonl:x": "refused",
		"BINDIN~1/x.yaml": "8.3 short name",
	} {
		if r := guardRun(t, dir, env, hookWrite(t, dir, at, entry)); r.code != 5 || !strings.Contains(r.stdout+r.stderr, why) {
			t.Errorf("%s: exit %d, want 5 naming %s\n%s%s", at, r.code, why, r.stdout, r.stderr)
		}
	}
	if r := guardRun(t, dir, env, hookWrite(t, dir, "notes/keep.md", "x\n")); r.code != 0 {
		t.Errorf("a plain new name: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	backslashed := tableOf(t, "guard-backslash-name.txt")[0][0]
	r := guardRun(t, dir, env, hookWrite(t, dir, backslashed, "x\n"))
	if filepath.ToSlash(backslashed) != backslashed { // the system's separator is the backslash: the name is notes/x.md
		if r.code != 0 {
			t.Errorf("notes\\x.md on Windows, the new file notes/x.md: exit %d\n%s%s", r.code, r.stdout, r.stderr)
		}
	} else if r.code != 5 || !strings.Contains(r.stdout+r.stderr, "refused") {
		t.Errorf("notes\\x.md, a backslash in a name: exit %d, want 5\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestTheGuardRefusesAnEmptyNewStringBeforeALinesEnd (Fable's finding 2 on
// #971): an Edit that takes a comment's text out with nothing in its place,
// where the line ends after it, ends refused, exit 5, since the agent's host
// takes the end out too and joins the next line into the comment. The
// control names the end in old_string.
// Risk: a lifecycle line is joined into a comment by hand and the guard
// judges an edit that kept it.
func TestTheGuardRefusesAnEmptyNewStringBeforeALinesEnd(t *testing.T) {
	t.Parallel()
	dir := historyRepo(t, "")
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	file := filepath.Join(dir, filepath.FromSlash(guardRules))
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	commented := strings.Replace(string(data), "status: proposed\n", "status: proposed #ZZ\n", 1)
	if commented == string(data) {
		t.Fatal("the rules file holds no status: proposed line")
	}
	if err := os.WriteFile(file, []byte(commented), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := guardRun(t, dir, env, hookEdit(t, dir, guardRules, " #ZZ", "")); r.code != 5 || !strings.Contains(r.stdout+r.stderr, "line's end") {
		t.Errorf("an empty new_string before a line's end: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if r := guardRun(t, dir, env, hookEdit(t, dir, guardRules, " #ZZ\n", "\n")); r.code != 0 {
		t.Errorf("the end named in old_string: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// TestInAProjectGuardEditIsNotReady: the guard is not for a project (S1).
func TestInAProjectGuardEditIsNotReady(t *testing.T) {
	t.Parallel()
	dir := newFolder(t)
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	r := guardRun(t, dir, env, hookEdit(t, dir, "README.md", "a", "b"))
	if r.code != 4 || !strings.Contains(r.stdout, "not ready") || !strings.Contains(r.stdout, "not a rulebook") {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
}

// hookLine is the line init prints for the agent's hook, as a person reads it
// on init's screen: the journeys reach gragas only as its binary
// (tests.journeys.2), so the line is spelled here, and init's goldens and
// TestInitJSONCarriesTheGuardLines hold it.
const hookLine = "gragas guard edit || exit 2"

// TestTheHookLineFailsClosed: the hook line turns any exit but 0 into 2, the
// code the agent's hook blocks on, so a missing program blocks too. What is
// left to hold is the program's side: guard edit exits 0 only on an allowed
// edit, and not 0 on a refusal and on input it cannot read. The shell's side
// is the shell's, and is not run here, so the test holds on every system
// (the arch.ports.24 rows of gate/exemptions.yaml only fall).
// Risk: a refusal or a fault the hook reads as a pass.
func TestTheHookLineFailsClosed(t *testing.T) {
	t.Parallel()
	cmd, ok := strings.CutSuffix(hookLine, " || exit 2")
	if !ok || cmd != "gragas guard edit" {
		t.Fatalf("the hook line %q does not end every failure on exit 2", hookLine)
	}
	dir := historyRepo(t, "")
	env, _ := folders(t)
	env["COLUMNS"] = "80"
	cases := map[string]struct {
		input   string
		allowed bool
	}{
		"refused":    {hookEdit(t, dir, guardRules, "status: proposed", "status: active"), false},
		"allowed":    {hookEdit(t, dir, guardRules, "Wrap an error.", "Wrap an error, and say which step."), true},
		"unreadable": {"\x00", false},
	}
	for name, tc := range cases {
		if r := guardRun(t, dir, env, tc.input); (r.code == 0) != tc.allowed {
			t.Errorf("%s: exit %d, allowed %t\n%s%s", name, r.code, tc.allowed, r.stdout, r.stderr)
		}
	}
}

// TestAHookInputPastTheCapIsReadToItsEndBeforeTheRefusal: the hook hands
// guard edit, through a pipe, one Write whose line is a mebibyte past the
// 16 MiB it reads. The host's write of the whole line ends with no error,
// and the edit is refused, exit 5, naming the cap. Planted failure: no drain in hookInput (the write ends on a
// closed pipe).
// Risk: the guard ends while the host is still writing, and a host that
// reads its failed write as the hook broken lets the edit through.
func TestAHookInputPastTheCapIsReadToItsEndBeforeTheRefusal(t *testing.T) {
	t.Parallel()
	dir := historyRepo(t, "")
	env, _ := folders(t)
	env["GRAGAS_NAMES_FILE"] = ""
	line, err := json.Marshal(map[string]any{
		"session_id": "journey", "hook_event_name": "PreToolUse", "cwd": dir, "tool_name": "Write",
		"tool_input": map[string]any{"file_path": filepath.Join(dir, guardRules), "content": strings.Repeat("x", 17<<20)},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	c := command(t, dir, env, "guard", "edit")
	var out, errb strings.Builder
	c.Stdin, c.Stdout, c.Stderr = r, &out, &errb
	if err := c.Start(); err != nil {
		t.Fatalf("could not run gragas: %v", err)
	}
	_ = r.Close() // the child holds its own end
	wrote := make(chan error, 1)
	go func() {
		_, err := w.Write(append(line, '\n'))
		wrote <- errors.Join(err, w.Close())
	}()
	code := 0
	if err := c.Wait(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("could not run gragas: %v", err)
		}
		code = ee.ExitCode()
	}
	if err := <-wrote; err != nil {
		t.Errorf("the host's write of the hook's input = %v, want it read to its end", err)
	}
	if said := out.String() + errb.String(); code != 5 || !strings.Contains(said, "16777216") {
		t.Errorf("exit %d, want 5, refused naming the 16777216 bytes:\n%s", code, said)
	}
}
