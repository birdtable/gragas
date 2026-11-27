package main

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
)

// stateFake is the cmd/gragas of a module the member builds in place of
// gragas's own: plugin approve writes a file named for the plugin into the
// folder the state variable names, and is refused when it names none;
// --json preset --check answers report, a Result document. No gragas is built
// or run outside the journeys (AGENTS.md, The Go code).
func stateFake(stateVar, report string) string {
	return `package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	switch {
	case len(os.Args) > 3 && os.Args[1] == "plugin" && os.Args[2] == "approve":
		dir := os.Getenv(` + strconv.Quote(stateVar) + `)
		if dir == "" {
			fmt.Fprintln(os.Stderr, "the fake: no state folder is named")
			os.Exit(1)
		}
		if err := os.WriteFile(filepath.Join(dir, "approved-"+os.Args[3]), nil, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case len(os.Args) == 4 && os.Args[1] == "--json" && os.Args[2] == "preset" && os.Args[3] == "--check":
		fmt.Println(` + strconv.Quote(report) + `)
	default:
		fmt.Fprintln(os.Stderr, "the fake: not a run the member makes:", os.Args[1:])
		os.Exit(2)
	}
}
`
}

// fakeReport is the fake's answer to --json preset --check: the tree's one
// preset, golangci-lint's, compared and the same.
const fakeReport = `{"schema": "gragas.result/v1", "command": "preset", "ended": "done", "msg": "the fake: 1 preset file compared",
"data": {"files": [{"path": "presets/golangci-lint/.golangci.yml", "tool": "golangci-lint", "state": "same"}], "compared": 1}}`

// TestTheApprovalStaysOutOfTheAccountsStateFolder: the member runs with the state folder's variable
// set, as an account's own may be, to a folder that stands in for the
// account's. It builds a fake gragas that approves by writing into the folder
// the variable names, so the approval lands in the folder of the member's run,
// and the account's folder holds nothing after.
func TestTheApprovalStaysOutOfTheAccountsStateFolder(t *testing.T) {
	t.Parallel()
	v := config.StateVar()
	if v == "" {
		t.Fatal("no variable moves the state folder on this system")
	}
	src := t.TempDir()
	configtest.MkdirPrivate(t, src, "cmd/gragas")
	for name, body := range map[string]string{
		"go.mod":             "module example.com/fake\n\ngo 1.22\n",
		"cmd/gragas/main.go": stateFake(v, fakeReport),
	} {
		if err := os.WriteFile(filepath.Join(src, filepath.FromSlash(name)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	account := t.TempDir()
	environ := append(slices.Clone(configtest.Inherited()), v+"="+account)
	n, marked, said, err := presetPlugins(t.Context(), environ, src, "../..")
	// The member names each pinned plugin no rule binds before the check's
	// answer, so the fake's line is the last.
	lines := strings.Split(said, "\n")
	if err != nil || n == 0 || marked == 0 || lines[len(lines)-1] != "the fake: 1 preset file compared" {
		t.Fatalf("the member over the tree with a fake gragas: %d approved, %d marked, said %q, error %v; want each pin approved, the fake's check, and the tree's marked preset held to the mark", n, marked, said, err)
	}
	left, err := os.ReadDir(account)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range left {
		t.Errorf("the account's state folder holds %s after the member's run: its approval was not kept to the folder of its run", e.Name())
	}
}

// The pins of tools.yaml's plugins: block and the plugin lock held to them
// (gragas#2340): a pin that cannot be read pins nothing, and a lock that is
// not the pins is named, so the member is red before any plugin is approved.

const (
	pinObject = "7d7815d3b3337821ab5a73baddc98e3c0cdfd32a"
	pinCommit = "551279a0851e330dade85aea42de2defea493e63"
	pinDigest = "sha256:8052dcb8c388997558e6b591c357d3413a55d2fc731a2e9fd86f586411364cf9"
)

func pinLine(name, version, tag, object, commit, digest string) string {
	return "  " + name + "@" + version + ": {tag: " + tag + ", object: " + object + ", commit: " + commit + `, digest: "` + digest + `"}`
}

func TestThePluginPinsAreReadStrictly(t *testing.T) {
	good := pinLine("golangci-lint", "1.0.0", "golangci-lint/v1.0.0", pinObject, pinCommit, pinDigest)
	pins, err := readPluginPins([]byte("go: 1.27.4\n\nplugins:\n  # the release's own\n" + good + "\nnext: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 1 || pins[0] != (pluginPin{"golangci-lint", "1.0.0", "golangci-lint/v1.0.0", pinObject, pinCommit, pinDigest}) {
		t.Fatalf("the pins are %+v, want golangci-lint@1.0.0 as written", pins)
	}
	for _, c := range []struct{ name, yaml, want string }{
		{"no block", "go: 1.27.4\n", "pins no plugin"},
		{"an empty block", "plugins:\nnext: 1\n", "pins no plugin"},
		{"a second block", "plugins:\n" + good + "\nplugins:\n", "a second plugins: block"},
		{"pinned twice", "plugins:\n" + good + "\n" + good + "\n", "pinned twice"},
		{"not a pin", "plugins:\n  golangci-lint: 1.0.0\n", "is not"},
		// The version is the manifest schema's and the lock's: no leading zero (PR 2394).
		{"a leading zero", "plugins:\n" + pinLine("golangci-lint", "1.0.01", "golangci-lint/v1.0.01", pinObject, pinCommit, pinDigest) + "\n", "is not"},
		{"a field short", "plugins:\n  golangci-lint@1.0.0: {tag: golangci-lint/v1.0.0, object: " + pinObject + ", commit: " + pinCommit + "}\n", "has 3 fields"},
		{"fields out of order", "plugins:\n  golangci-lint@1.0.0: {object: " + pinObject + ", tag: golangci-lint/v1.0.0, commit: " + pinCommit + `, digest: "` + pinDigest + `"}` + "\n", "in that order"},
		{"another tag", "plugins:\n" + pinLine("golangci-lint", "1.0.0", "golangci-lint/v1.0.1", pinObject, pinCommit, pinDigest) + "\n", "tagged golangci-lint/v1.0.0"},
		{"a short object", "plugins:\n" + pinLine("golangci-lint", "1.0.0", "golangci-lint/v1.0.0", pinObject[:39], pinCommit, pinDigest) + "\n", "object"},
		{"an upper commit", "plugins:\n" + pinLine("golangci-lint", "1.0.0", "golangci-lint/v1.0.0", pinObject, strings.ToUpper(pinCommit), pinDigest) + "\n", "commit"},
		{"a digest of another sum", "plugins:\n" + pinLine("golangci-lint", "1.0.0", "golangci-lint/v1.0.0", pinObject, pinCommit, "sha512:"+pinDigest[7:]) + "\n", "digest"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := readPluginPins([]byte(c.yaml)); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%q: error %v, want one saying %q", c.yaml, err, c.want)
			}
		})
	}
}

// The preset: block of a plugin's manifest, read by lines: the tool, the path and the mark, and nothing deeper in the block; a
// manifest with no preset: makes none, and one the member cannot read is
// refused, naming what it is.
func TestThePluginsPresetIsReadStrictly(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "plugins", "golangci-lint", "1.0.1", "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pre, ok, err := readPluginPreset(raw)
	if err != nil || !ok || pre != (pluginPreset{tool: "golangci-lint", path: ".golangci.yml", mark: true}) {
		t.Fatalf("the pinned manifest: %+v, %v, %v; want golangci-lint's .golangci.yml, marked", pre, ok, err)
	}
	head := "name: x\ntool: x\n"
	block := "preset:\n  path: x.yml\n  # a note\n  readback:\n    mark: false\n"
	for _, c := range []struct {
		name, yaml string
		want       pluginPreset
	}{
		{"no mark", head + block, pluginPreset{tool: "x", path: "x.yml"}},
		{"marked, quoted", head + block + "  mark: \"true\"\nwiring: {}\n", pluginPreset{tool: "x", path: "x.yml", mark: true}},
		{"a mark below the block", head + block + "wiring:\n  mark: true\n", pluginPreset{tool: "x", path: "x.yml"}},
		{"a path in a folder", head + "preset:\n  path: conf/.x.yml\n", pluginPreset{tool: "x", path: "conf/.x.yml"}},
		// A line of spaces in the block is a blank, as YAML reads it, not a
		// key to refuse, and never a slice past its end.
		{"a line of one space in the block", head + "preset:\n  path: x.yml\n \n  mark: true\n", pluginPreset{tool: "x", path: "x.yml", mark: true}},
		{"a line of two spaces in the block", head + "preset:\n  path: x.yml\n  \n  mark: true\n", pluginPreset{tool: "x", path: "x.yml", mark: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, ok, err := readPluginPreset([]byte(c.yaml))
			if err != nil || !ok || got != c.want {
				t.Errorf("%q: %+v, %v, %v; want %+v", c.yaml, got, ok, err, c.want)
			}
		})
	}
	if _, ok, err := readPluginPreset([]byte(head)); ok || err != nil {
		t.Errorf("no preset: block: %v, %v; want none and no error", ok, err)
	}
	for _, c := range []struct{ name, yaml, want string }{
		{"a flow block", head + "preset: {path: x.yml, mark: true}\n", "line 3: \"preset: {path: x.yml, mark: true}\" is not the one preset block"},
		{"two blocks", head + block + "preset:\n", "is not the one preset block"},
		{"two tools", head + "tool: y\n" + block, "line 3: a second tool key"},
		{"two paths", head + block + "  path: y.yml\n", "a second path key"},
		{"two marks", head + block + "  mark: true\n  mark: false\n", "is not the one mark"},
		{"a mark not a bool", head + block + "  mark: yes\n", "true or false"},
		{"a path out of the folder", head + "preset:\n  path: ../x.yml\n", "none of them . or .. alone"},
		{"a path with a backslash", head + "preset:\n  path: a\\x.yml\n", "none of them . or .. alone"},
		{"no path", head + "preset:\n  mark: true\n", "none of them . or .. alone"},
		{"no tool", "preset:\n  path: x.yml\n", "names its tool by one name"},
		// A quoted key is the same key to YAML, and a reader of bare keys
		// would pass over it, so it is refused.
		{"a quoted top-level key", head + "\"preset\":\n  path: x.yml\n  mark: true\n", "line 3: \"\\\"preset\\\":\" is not a bare key"},
		{"a quoted key in the block", head + "preset:\n  path: x.yml\n  'mark': true\n", "line 5: \"  'mark': true\" is not a bare key"},
		{"a key spaced from its colon", head + "preset :\n  path: x.yml\n", "line 3: \"preset :\" is not a bare key"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := readPluginPreset([]byte(c.yaml)); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%q: error %v, want one saying %q", c.yaml, err, c.want)
			}
		})
	}
}

// markedRoot is a root of two pinned plugins whose manifests ask for the mark:
// golangci-lint, the tree's own manifest, with its preset as given, and
// ast-grep, whose preset is not there. Only the plants write files.
func markedRoot(t *testing.T, golangci string) (string, []pluginPin) {
	t.Helper()
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join("..", "..", "plugins", "golangci-lint", "1.0.1", "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"plugins/golangci-lint/1.0.1/plugin.yaml": string(raw),
		"plugins/ast-grep/1.0.0/plugin.yaml":      "name: ast-grep\ntool: ast-grep\npreset:\n  path: sgconfig.yml\n  mark: true\n",
		"presets/golangci-lint/.golangci.yml":     golangci,
	} {
		configtest.MkdirPrivate(t, dir, path.Dir(name))
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, []pluginPin{{name: "golangci-lint", version: "1.0.1"}, {name: "ast-grep", version: "1.0.0"}}
}

// TestAPinnedPluginNoRuleBindsIsSkippedAndNamed holds the skip to preset
// --check's own list: a pinned plugin whose tool the report does not
// list is skipped and named, and for every tool it lists, a preset that is
// not there and one without the mark stay red; a file the project owns is a
// listed tool's, never a skip.
func TestAPinnedPluginNoRuleBindsIsSkippedAndNamed(t *testing.T) {
	marked := "# " + model.PresetMark + "\nversion: \"2\"\n"
	golangci := presetCheckedFile{Path: "presets/golangci-lint/.golangci.yml", Tool: "golangci-lint", State: "same"}
	t.Run("an unbound pinned plugin with no file passes and is named", func(t *testing.T) {
		dir, pins := markedRoot(t, marked)
		rep := &presetChecked{Files: []presetCheckedFile{golangci}, Compared: 1}
		n, skipped, err := markedPresets(dir, pins, rep)
		if err != nil || n != 1 || !slices.Equal(skipped, []string{"ast-grep"}) {
			t.Fatalf("ast-grep pinned and unlisted: %d marked, skipped %q, error %v; want golangci-lint held and ast-grep skipped", n, skipped, err)
		}
		said := presetCheck{Ended: "done", Msg: "1 preset file compared, none differs", Data: rep}.said(skipped)
		if !strings.Contains(said, "ast-grep: pinned, no rule binds it, no preset checked\n") {
			t.Errorf("the member says %q, want ast-grep named as skipped", said)
		}
	})
	// The skip is for a plugin with no file: a file at an unlisted tool's
	// path is one nothing holds, so it is red.
	t.Run("an unbound pinned plugin with a file there is red", func(t *testing.T) {
		dir, pins := markedRoot(t, marked)
		configtest.MkdirPrivate(t, dir, "presets/ast-grep")
		if err := os.WriteFile(filepath.Join(dir, "presets", "ast-grep", "sgconfig.yml"), []byte("# "+model.PresetMark+"\nruleDirs: []\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		rep := &presetChecked{Files: []presetCheckedFile{golangci}, Compared: 1}
		n, skipped, err := markedPresets(dir, pins, rep)
		if err == nil || !strings.Contains(err.Error(), "presets/ast-grep/sgconfig.yml is there, and preset --check does not list ast-grep") {
			t.Errorf("ast-grep pinned and unlisted, its file there: %d marked, skipped %q, error %v; want the file named as held to nothing", n, skipped, err)
		}
	})
	// A hand-written tool's pinned plugin is skipped as hand-written, never
	// named as unbound, whether its file is there or not.
	t.Run("a hand-written tool's pinned plugin is not named as unbound", func(t *testing.T) {
		dir, pins := markedRoot(t, marked)
		configtest.MkdirPrivate(t, dir, "presets/ast-grep")
		if err := os.WriteFile(filepath.Join(dir, "presets", "ast-grep", "sgconfig.yml"), []byte("ruleDirs: []\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		rep := &presetChecked{Files: []presetCheckedFile{golangci}, Compared: 1, HandWritten: []string{"ast-grep"}}
		n, skipped, err := markedPresets(dir, pins, rep)
		if err != nil || n != 1 || len(skipped) != 0 {
			t.Errorf("ast-grep hand-written and pinned: %d marked, skipped %q, error %v; want golangci-lint held and ast-grep neither skipped as unbound nor red", n, skipped, err)
		}
	})
	t.Run("a bound tool with its file missing is red", func(t *testing.T) {
		dir, pins := markedRoot(t, marked)
		rep := &presetChecked{Files: []presetCheckedFile{golangci, {Path: "presets/ast-grep/sgconfig.yml", Tool: "ast-grep", State: "differs", First: "it is not there"}}, Compared: 2}
		if _, _, err := markedPresets(dir, pins, rep); err == nil || !strings.Contains(err.Error(), "presets/ast-grep/sgconfig.yml could not be read") {
			t.Errorf("ast-grep listed with no file: error %v, want its preset named as not there", err)
		}
	})
	t.Run("a bound tool with its mark removed and a line edited is red", func(t *testing.T) {
		dir, pins := markedRoot(t, "version: \"3\"\n")
		// preset --check reads the unmarked file as the project's: it lists
		// the path under owned, not the tool under files
		rep := &presetChecked{Files: []presetCheckedFile{}, Owned: []string{golangci.Path}}
		if _, _, err := markedPresets(dir, pins, rep); err == nil || !strings.Contains(err.Error(), golangci.Path+" does not open with the mark") {
			t.Errorf("golangci-lint owned, the mark removed: error %v, want the file named as not opening with the mark", err)
		}
	})
}

// The answer of --json preset --check is read strictly: a document with no
// report is no list, and a check that is not done is said with its files.
func TestThePresetCheckAnswerIsRead(t *testing.T) {
	if _, err := readPresetCheck([]byte(fakeReport)); err != nil {
		t.Fatalf("the fake's report: %v", err)
	}
	for name, doc := range map[string]string{
		"not JSON":       "done: 1 preset file compared",
		"no data":        `{"ended": "done", "msg": "x"}`,
		"data, no files": `{"ended": "done", "msg": "x", "data": {"compared": 0}}`,
	} {
		if _, err := readPresetCheck([]byte(doc)); err == nil {
			t.Errorf("%s: read as a report, want it refused", name)
		}
	}
	c, err := readPresetCheck([]byte(`{"ended": "findings", "msg": "1 of 1 preset file differ", "data": {"files": [{"path": "presets/x/x.yml", "tool": "x", "state": "differs", "first": "line 2"}], "compared": 1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.failed(nil, nil, nil); got != "findings: 1 of 1 preset file differ\npresets/x/x.yml: differs: line 2" {
		t.Errorf("a check with findings says %q, want its sentence and the file", got)
	}
	// The verdict on the run: a report that ended done with exit 0 is read;
	// exit 0 with any other closing word is red, as a document the member
	// does not read; a failed run is red with
	// its files.
	if _, err := judgePresetCheck([]byte(fakeReport), nil, nil); err != nil {
		t.Errorf("done, exit 0: %v, want the report read", err)
	}
	findings := []byte(`{"ended": "findings", "msg": "1 of 1 preset file differ", "data": {"files": [{"path": "presets/x/x.yml", "tool": "x", "state": "differs", "first": "line 2"}], "compared": 1}}`)
	if _, err := judgePresetCheck(findings, nil, nil); err == nil || !strings.Contains(err.Error(), `exited 0, and its answer is not a report the member reads`) || !strings.Contains(err.Error(), `it ended "findings" with exit status 0`) {
		t.Errorf("findings, exit 0: error %v, want it refused, naming the closing word and the exit status", err)
	}
	if _, err := judgePresetCheck(findings, errors.New("exit status 1"), []byte("the fake said")); err == nil || !strings.Contains(err.Error(), "the presets are not what the installed plugins make: exit status 1:\nfindings: 1 of 1 preset file differ\npresets/x/x.yml: differs: line 2\nthe fake said") {
		t.Errorf("findings, exit 1: error %v, want the closing word, the file and what the run said", err)
	}
}

// FuzzReadPresetCheck holds readPresetCheck over any bytes (tests.generated.1):
// it never panics; it answers a report exactly when the document carries
// data.files, so a list it cannot read is an error and never an empty list
// that would skip every pin; the verdict on a run that exited 0 is red unless
// that report ended done; and it answers the same twice.
func FuzzReadPresetCheck(f *testing.F) {
	for _, seed := range []string{
		fakeReport,
		`{"ended": "findings", "msg": "x", "data": {"files": [{"path": "presets/x/x.yml", "tool": "x", "state": "differs", "first": "line 2"}], "compared": 1}}`,
		`{"ended": "done", "data": {"files": [], "owned": ["presets/x/x.yml"], "hand_written": ["y"]}}`,
		`{"ended": "done", "msg": "x"}`, `{"data": {"files": null}}`, `{"data": null}`, `null`, `[]`, `{"data": {"files": 5}}`, "{", "",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := readPresetCheck(data)
		if (err == nil) != (c.Data != nil && c.Data.Files != nil) {
			t.Fatalf("readPresetCheck(%q) = data %v, error %v: a report is answered exactly when it carries data.files", data, c.Data, err)
		}
		_ = c.failed(err, data, nil)
		if _, jerr := judgePresetCheck(data, nil, nil); jerr == nil && (err != nil || c.Ended != "done") {
			t.Fatalf("judgePresetCheck(%q) with exit 0 passed, and the report read %v ended %q: only a report that ended done passes", data, err, c.Ended)
		}
		again, err2 := readPresetCheck(data)
		if (err2 == nil) != (err == nil) || again.Ended != c.Ended || again.Msg != c.Msg || (again.Data == nil) != (c.Data == nil) {
			t.Fatalf("readPresetCheck(%q) answered %+v, %v and then %+v, %v", data, c, err, again, err2)
		}
	})
}

func TestTheLockIsHeldToThePins(t *testing.T) {
	lock, err := hostfs.DecodePluginLock([]byte("version: 1\nplugins:\n  -\n    name: golangci-lint\n    version: 1.0.0\n    source: local\n    digest: \"" + pinDigest + "\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	pin := pluginPin{"golangci-lint", "1.0.0", "golangci-lint/v1.0.0", pinObject, pinCommit, pinDigest}
	if got := lockAgainstPins(lock, []pluginPin{pin}); len(got) != 0 {
		t.Fatalf("the lock against its own pin: %q, want nothing", got)
	}
	other := pin
	other.digest = "sha256:" + strings.Repeat("0", 64)
	if got := lockAgainstPins(lock, []pluginPin{other}); len(got) != 1 || !strings.Contains(got[0], "and tools.yaml pins "+other.digest) {
		t.Errorf("a pin of another digest: %q, want the lock's digest and the pin named", got)
	}
	if got := lockAgainstPins(lock, nil); len(got) != 1 || !strings.Contains(got[0], "pins no such plugin") {
		t.Errorf("a plugin nothing pins: %q, want it named", got)
	}
	missing := pin
	missing.version = "1.0.1"
	missing.tag = "golangci-lint/v1.0.1"
	if got := lockAgainstPins(lock, []pluginPin{pin, missing}); len(got) != 1 || !strings.Contains(got[0], "does not hold it") {
		t.Errorf("a pin the lock does not hold: %q, want it named", got)
	}
}

// The member builds gragas under the toolchain go.mod pins, not the host's
// (gragas#2515): `local` ran the machine's own go, and Homebrew's 1.24 fails
// a module that asks for 1.27 in every push tier.

func TestTheToolchainPinIsTheModulesToolchainLine(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, mod, want string }{
		{"a toolchain line", "module m\n\ngo 1.27.1\n\ntoolchain go1.27.4\n", "go1.27.4"},
		{"no toolchain line", "module m\n\ngo 1.22\n", "local"},
	} {
		src := t.TempDir()
		if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte(c.mod), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err := goToolchainPin(src); err != nil || got != c.want {
			t.Errorf("%s: pin %q, error %v; want %q", c.name, got, err, c.want)
		}
	}
	got, err := goToolchainPin("../..")
	if err != nil || got == "local" || got == "" {
		t.Errorf("the tree's own pin is %q, error %v; want go.mod's toolchain line", got, err)
	}
}

func TestTheMemberBuildsUnderThePinNotTheHostsGo(t *testing.T) {
	t.Parallel()
	pinned := func(pin string) string {
		var v string
		for _, e := range append(goEnv(), "GOTOOLCHAIN="+pin) {
			if val, ok := strings.CutPrefix(e, "GOTOOLCHAIN="); ok {
				v = val // the last of a repeated name is the one go reads
			}
		}
		return v
	}
	if got := pinned("go1.27.4"); got != "go1.27.4" {
		t.Errorf("the pinned go runs under GOTOOLCHAIN=%q, want the pin", got)
	}
	raw, err := os.ReadFile("presetplugins.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	if strings.Contains(src, "goOver(") || strings.Contains(src, "goFor(") {
		t.Error("presetplugins.go runs a go command under goEnv's GOTOOLCHAIN=local: the build ignores the pin")
	}
	if n := strings.Count(src, "goPinned(ctx, environ, src, pin, "); n != 2 {
		t.Errorf("presetplugins.go runs %d go commands under the pin, want 2 (go env GOEXE and go build)", n)
	}
}
