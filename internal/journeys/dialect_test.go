package journeys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEachShellGetsTheApplyLineWrittenForIt: migrate's dry run ends on an
// apply line, and GRAGAS_SHELL, or the shell key under it, names the shell
// that line is written for: posix, powershell and cmd each quote the owner
// their own way, in any case; a word cmd expands in any quoting gives no line
// and the help in its place; a name that is no shell is refused, naming the
// fix. The lines are literals, so the journey does
// not quote them with the code under test. Risk: a line pasted into
// PowerShell or cmd that runs other words than the plan was made for, a
// setting the binary does not read, or a cmd line that expands %off%.
// PowerShell's lines are held here and by the root's unit rows only: no
// journey pastes one into a live pwsh.
func TestEachShellGetsTheApplyLineWrittenForIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, env, key, owner, next string
		code                        int
	}{
		{"posix", "posix", "", "O'Neil", `gragas migrate --name demo --owner 'O'\''Neil' --apply`, 6},
		{"powershell", "powershell", "", "O'Neil", `gragas migrate --name demo --owner 'O''Neil' --apply`, 6},
		{"powershell quotes a percent", "PowerShell", "", "50%off", `gragas migrate --name demo --owner '50%off' --apply`, 6},
		{"cmd", "cmd", "", "A B", `gragas migrate --name demo --owner ^"A B^" --apply`, 6},
		{"cmd leaves a quote bare", "CMD", "", "O'Neil", `gragas migrate --name demo --owner O'Neil --apply`, 6},
		{"cmd gives no line for a percent", "cmd", "", "50%off", "gragas help migrate", 2},
		{"the key, with no variable", "", "cmd", "A B", `gragas migrate --name demo --owner ^"A B^" --apply`, 6},
		{"the variable over the key", "powershell", "cmd", "O'Neil", `gragas migrate --name demo --owner 'O''Neil' --apply`, 6},
		{"a name that is no shell", "bash", "", "A B", "unset GRAGAS_SHELL, or set it to auto, posix, powershell or cmd", 5},
		{"powershell gives no line for a double quote", "powershell", "", `x" --apply "y`, "gragas help migrate", 2},
		{"powershell gives no line for an empty owner", "powershell", "", "", "gragas help migrate", 2},
		{"powershell gives no line for a spaced word ending in a backslash", "powershell", "", `a b\`, "gragas help migrate", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := migrateIn(t, tc.env, tc.key, "--name", "demo", "--owner", tc.owner, "--json")
			var doc struct {
				Next string `json:"next"`
			}
			if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
				t.Fatalf("exit %d, stdout not JSON: %v\n%s%s", r.code, err, r.stdout, r.stderr)
			}
			if r.code != tc.code || doc.Next != tc.next {
				t.Errorf("exit %d, next %q; want exit %d, next %q\n%s", r.code, doc.Next, tc.code, tc.next, r.stderr)
			}
		})
	}
}

// TestAPlanMadeInPowerShellIsThePlanPOSIXChecks: the line a
// plan keeps, in its digest, its journal and the history's yes, is written
// for POSIX whatever shell the run prints its lines for, so a plan made in
// PowerShell or cmd is the same plan, with the same digest, as one made in
// Git Bash, and guard staged splits the yes back into the same words. Only
// the apply line to type moves with the shell. Each shell applies migrate
// over its own copy of the planted rulebook, and the history line each writes
// is read. Risk: a plan that is another plan in each shell, or a history line
// guard staged cannot replay.
func TestAPlanMadeInPowerShellIsThePlanPOSIXChecks(t *testing.T) {
	t.Parallel()
	type entry struct{ Plan, Command string }
	got := map[string]entry{}
	for _, sh := range []string{"posix", "powershell", "cmd"} {
		dir := v2Folder(t)
		dry := migrateAt(t, dir, sh, "", "--name", "demo", "--owner", "O'Neil A", "--json")
		var d struct {
			Next string `json:"next"`
		}
		if err := json.Unmarshal([]byte(dry.stdout), &d); err != nil || d.Next != map[string]string{
			"posix":      `gragas migrate --name demo --owner 'O'\''Neil A' --apply`,
			"powershell": `gragas migrate --name demo --owner 'O''Neil A' --apply`,
			"cmd":        `gragas migrate --name demo --owner ^"O'Neil A^" --apply`,
		}[sh] {
			t.Fatalf("%s: the apply line to type is %q (%v), want it written for %s\n%s", sh, d.Next, err, sh, dry.stderr)
		}
		if r := migrateAt(t, dir, sh, "", "--name", "demo", "--owner", "O'Neil A", "--apply"); r.code != 0 {
			t.Fatalf("%s: --apply ended %d\n%s%s", sh, r.code, r.stdout, r.stderr)
		}
		b, err := os.ReadFile(filepath.Join(dir, "history.jsonl"))
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		var e entry
		if err != nil || json.Unmarshal([]byte(lines[len(lines)-1]), &e) != nil {
			t.Fatalf("%s: the history line could not be read: %v\n%s", sh, err, b)
		}
		got[sh] = e
	}
	want := `gragas migrate --name demo --owner 'O'\''Neil A'`
	for sh, e := range got {
		if e.Command != want || e.Plan == "" || e.Plan != got["posix"].Plan {
			t.Errorf("%s: the yes keeps %q under plan %q; want %q under posix's plan %q", sh, e.Command, e.Plan, want, got["posix"].Plan)
		}
	}
}

// migrateIn runs migrate as migrateRun does, over the planted v2 rulebook,
// with GRAGAS_SHELL set to sh ("" unsets the posix commandOf sets) and the
// settings file's shell key set to key ("" for none).
func migrateIn(t *testing.T, sh, key string, args ...string) result {
	t.Helper()
	return migrateAt(t, v2Folder(t), sh, key, args...)
}

// migrateAt is migrateIn over the folder dir.
func migrateAt(t *testing.T, dir, sh, key string, args ...string) result {
	t.Helper()
	env, _ := folders(t)
	data := "version: 1\nprograms:\n  git: " + strconv.Quote(journeyGit(t)) + "\n" // a Windows path holds backslashes, which a quoted scalar escapes
	if key != "" {
		data += "shell: " + key + "\n"
	}
	settings, _ := settingsFile(t, data)
	env["GRAGAS_CONFIG"] = settings
	env["GRAGAS_NAMES_FILE"] = ""
	env["GRAGAS_SHELL"] = sh
	return run(t, dir, env, append([]string{"migrate"}, args...)...)
}
