package tool_test

import (
	"encoding/json"
	"errors"
	"github.com/birdtable/go-files/filestest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/proc"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
)

// walkFix is the fix line of the trust walk's refusal of the program at, a
// slash path, as the adapter walks a tool: go-files' fix on this system in
// host/trust's words, which the adapter's refusal hands on, and not a line
// of one system's written here. A walk that passes, or
// refuses with no fix, fails the test.
func walkFix(t *testing.T, at string) string {
	t.Helper()
	_, err := hosttrust.Walk(hosttrust.Subject{Name: "the tool", File: "the tool", Last: hosttrust.Program}, filepath.FromSlash(at), hosttrust.Who{})
	u, ok := errors.AsType[*hosttrust.Unsafe](err)
	if !ok || u.Fix == "" {
		t.Fatalf("the trust walk of %s = %v: want a refusal with its fix", at, err)
	}
	return u.Fix
}

func TestMain(m *testing.M) {
	os.Exit(configtest.Main(m))
}

// private is filestest.Private slash-separated: a folder of this account's
// that only it can enter, in the account's own temp folder, with no link in
// its path.
func private(t *testing.T) string {
	t.Helper()
	return filepath.ToSlash(filestest.Private(t))
}

// pathList joins folders in the system's PATH form (filepath.ListSeparator),
// as a run's PATH is given; the folders stay in the model's slash form.
func pathList(folders ...string) string {
	return strings.Join(folders, string(filepath.ListSeparator))
}

// fakeBody marks a body that is a list of configtest steps in JSON,
// not a shell script: program puts a built program there instead.
const fakeBody = "\x00fake:"

// homeVarAt is the name of the i-th variable proc.HomeEnv gives a Home.
func homeVarAt(i int) string {
	name, _, _ := strings.Cut(proc.HomeEnv(filepath.FromSlash("/"))[i], "=")
	return name
}

// homeName is the name of the first variable proc.HomeEnv gives a Home.
func homeName() string {
	name, _ := homeVar("/")
	return name
}

// homeVar is the first variable proc.HomeEnv gives a Home, as a program is
// told it, and the folder it names: a program that writes below it writes
// below the Home, and no test names a variable of the system.
func homeVar(home string) (name, dir string) {
	name, dir, _ = strings.Cut(proc.HomeEnv(filepath.FromSlash(home))[0], "=")
	return name, dir
}

// binDir is the folder the adapters' entries name their programs in, below a
// project: "bin/lint-a". A test builds the path the product finds from it.
const binDir = "bin"

// program puts a program at path, its folders made 0700, and makes it 0700,
// its owner's to run: a built stand-in when body is a fakeBody. A test that
// wants another mode sets it on the path answered.
func program(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if list, ok := strings.CutPrefix(body, fakeBody); ok {
		var steps []configtest.Step
		if err := json.Unmarshal([]byte(list), &steps); err != nil {
			t.Fatal(err)
		}
		path = place(t, filepath.Dir(path), filepath.Base(path), steps...)
	} else {
		t.Fatalf("program %s: a body that is no list of steps would be a shell script, which no system but Unix runs", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(path)
}

// fakeSteps is the body of a built program that does steps in order (fakeBody).
func fakeSteps(steps ...configtest.Step) string {
	list, err := json.Marshal(steps)
	if err != nil {
		panic(err) // a list of steps always marshals
	}
	return fakeBody + string(list)
}

// answering is the body of a program that says lint-a 1.2.3 for --version and
// otherwise does steps.
func answering(steps ...configtest.Step) string {
	return fakeSteps(append([]configtest.Step{configtest.Version("lint-a 1.2.3", "")}, steps...)...)
}

// printing is the body of a program that prints text for any arguments and
// then exits with code, with no version step of its own: the text is what
// --version reads too.
func printing(text string, code int) string {
	return fakeSteps(configtest.Out(text), configtest.Exit(code))
}

// echo is a program that says its version for --version and otherwise
// prints each argument it was given on a line of its own.
const echo = fakeBody + `[{"version":"lint-a 1.2.3"},{"print_args":true}]`

// pathFolder is the one folder of a PATH the tool's run is given: absolute, empty, readable and outside any
// root, in this run's own temp folder (go-proc's isolation of it), made as it is asked for. The trusted walk
// leaves out a PATH folder it cannot compare with the root, and where the system has volumes it can compare
// only a folder that is there, so a folder that is not there would leave no PATH at all.
func pathFolder() string {
	p := filepath.Join(os.TempDir(), "path-folder")
	if err := os.MkdirAll(p, 0o700); err != nil {
		panic(err)
	}
	return p
}

// TestTheVersionIsTheFirstMajorMinorPatchStandingApart.
func TestTheVersionIsTheFirstMajorMinorPatchStandingApart(t *testing.T) {
	t.Parallel()
	for line, want := range map[string]string{
		"lint-a has version 2.15.0 built with go1.27": "2.15.0",
		"v9.1.0": "9.1.0", "Version 5.4.5": "5.4.5", "tool 0.20.1": "0.20.1", "1.2.3.": "1.2.3",
		"1.2.3.4": "", "01.2.3": "", "1.2": "", "x1.2.3": "", "nothing at all": "",
		// more after the patch is not the release
		"1.2.3-rc.1 then 4.5.6": "", "tool 1.2.3-rc.1": "", "v1.2.3+local": "", "1.2.3a1": "", "1.2.3-dirty": "", "1.2.3_dev": "",
		"1.2.3 (rc)": "1.2.3", "1.2.3, built 2026": "1.2.3",
		// the other marks of a prerelease, and the ends a version may have
		"1.2.3.rc1": "", "1.2.3.dev0": "", "1.2.3.post1": "", "1.2.3~rc1": "", "1.2.3é": "", "1.2.3β1": "", "1.2.3@abc": "", "1.2.3:x": "", "1.2.3/x": "", "1.2.3-": "",
		// a word that goes on with a dot and a digit is more after the patch, not
		// skipped for a clean version later on the line
		"1.2.3.4 (api 1.2.3)": "", "1.2.3.4-rc1, compatible with 1.2.3": "", "1.2.4.20261117-nightly (based on 1.2.3)": "", "lint-a 1.2.3.4": "",
		"1.2.3\r": "1.2.3", "v1.2.3)": "1.2.3", `version "1.2.3"`: "1.2.3", "(1.2.3]": "1.2.3", "ends 1.2.3. Then": "1.2.3",
	} {
		if got := tool.VersionOf(line); got != want {
			t.Errorf("VersionOf(%q) = %q, want %q", line, got, want)
		}
	}
	if got := tool.VersionOf(""); got != "" {
		t.Errorf("VersionOf of an empty line = %q, want none", got)
	}
}
