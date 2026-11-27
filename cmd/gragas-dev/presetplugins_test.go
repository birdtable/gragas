package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hostfs "github.com/birdtable/gragas/internal/host/fs"
)

// fakeReport is the fake's answer to --json preset --check: the tree's one
// preset, golangci-lint's, compared and the same.
const fakeReport = `{"schema": "gragas.result/v1", "command": "preset", "ended": "done", "msg": "the fake: 1 preset file compared",
"data": {"files": [{"path": "presets/golangci-lint/.golangci.yml", "tool": "golangci-lint", "state": "same"}], "compared": 1}}`

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
