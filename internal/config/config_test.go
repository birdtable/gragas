package config_test

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-files/appdirs"

	"github.com/birdtable/gragas/internal/config"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// layout is the layout config hands appdirs.HereWith for gragas's folders.
// Which folder a layout gives on which system is go-files' test; config's
// tests prove config takes appdirs' folders and refuses a bad home, so a
// wanted folder is appdirs' answer, never a shape written here.
const layout = appdirs.XDG

// folders is appdirs' answer for gragas with e: the folders config must take.
// ErrRun is taken past, as config takes it: gragas keeps no Run folder, and
// appdirs gives the other four with it.
func folders(t *testing.T, e map[string]string) appdirs.Folders {
	t.Helper()
	f, err := appdirs.HereWith("gragas", layout, env(e))
	if err != nil && !errors.Is(err, appdirs.ErrRun) {
		t.Fatalf("appdirs.HereWith with %q: %v", e, err)
	}
	return f
}

// moving is e with each variable appdirs.Sources names for the config, state
// and cache folders set to at(the folder's name): the variables that move
// gragas's folders on the system the test runs on, none named here. Where
// two folders share one variable, the later one's value stands.
func moving(t *testing.T, e map[string]string, at func(folder string) string) map[string]string {
	t.Helper()
	s, err := appdirs.Sources(layout)
	if err != nil {
		t.Fatalf("appdirs.Sources: %v", err)
	}
	out := maps.Clone(e)
	for _, f := range []struct{ name, variable string }{{"config", s.Config}, {"state", s.State}, {"cache", s.Cache}} {
		if f.variable != "" {
			out[f.variable] = at(f.name)
		}
	}
	return out
}

// TestTheFoldersFollowXDG: config's three folders are the ones appdirs
// answers for the run's environment, in config's layout, with or without the
// variables that move them; a home that is unset, relative, climbing or
// holds a control character gives no folder, moving variables or not.
// Planted failure: folders() keeping a home appdirs does not read, or
// joining the folders itself.
func TestTheFoldersFollowXDG(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	home := map[string]string{appdirs.HomeVar: filepath.Join(dir, "h")}
	under := func(f string) string { return filepath.Join(dir, f) }
	tests := []struct {
		name  string
		env   map[string]string
		ready bool
	}{
		{"the variables that move the folders", moving(t, home, under), true},
		{"a relative moving variable is as appdirs takes it", moving(t, home, func(f string) string { return f }), true},
		{"the home alone", home, true},
		{"neither: a run with no home is not ready", map[string]string{}, false},
		{"a relative HOME is never joined", map[string]string{appdirs.HomeVar: "h"}, false},
		{"a HOME that climbs is never joined", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h") + "/../x"}, false},
		{"a HOME with a control character is never joined", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h\x1b[0m")}, false},
		{"a relative HOME is never joined, moving variables or not", moving(t, map[string]string{appdirs.HomeVar: "h"}, under), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var want appdirs.Folders
			if tc.ready {
				if want = folders(t, tc.env); want.Config == "" || want.State == "" || want.Cache == "" {
					t.Fatalf("appdirs answers %+v with %q: a folder is empty", want, tc.env)
				}
			}
			c := config.FromEnv(env(tc.env))
			if c.ConfigDir != want.Config || c.StateDir != want.State || c.CacheDir != want.Cache {
				t.Errorf("folders %q %q %q, want %q %q %q", c.ConfigDir, c.StateDir, c.CacheDir, want.Config, want.State, want.Cache)
			}
		})
	}
}

// TestEveryFileIsMadeByOneFunction: the lock, the run log, the settings
// file and the tools file each come from Path, under its folder, and the
// one variable and the one key that move a file move it.
func TestEveryFileIsMadeByOneFunction(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	home := map[string]string{appdirs.HomeVar: filepath.Join(dir, "h")}
	e := moving(t, home, func(f string) string { return filepath.Join(dir, f) })
	at := folders(t, e)
	c := config.FromEnv(env(e))
	for f, want := range map[config.File]string{
		config.FileSettings: filepath.Join(at.Config, "settings.yaml"), config.FileTools: filepath.Join(at.Config, "tools.yaml"),
		config.FileLock: filepath.Join(at.State, "gragas.lock"), config.FileRunLog: filepath.Join(at.State, "runs.jsonl"),
	} {
		if got := c.Path(f); got != want {
			t.Errorf("Path(%d) = %q, want %q", f, got, want)
		}
	}
	if c.LockPath() != c.Path(config.FileLock) || c.RunLog() != c.Path(config.FileRunLog) || c.SettingsFile != c.Path(config.FileSettings) {
		t.Errorf("lock %q, log %q, settings %q: not Path's", c.LockPath(), c.RunLog(), c.SettingsFile)
	}
	moved := config.FromEnv(env(map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.ConfigVar: filepath.Join(dir, "elsewhere", "s.yaml")}))
	if moved.SettingsFile != filepath.Join(dir, "elsewhere", "s.yaml") || !moved.Named || moved.Path(config.FileSettings) != filepath.Join(dir, "elsewhere", "s.yaml") {
		t.Errorf("GRAGAS_CONFIG: settings file %q, named %v", moved.SettingsFile, moved.Named)
	}
	moved.Settings.Tools = filepath.Join(dir, "t", "tools.yaml")
	if moved.Path(config.FileTools) != filepath.Join(dir, "t", "tools.yaml") {
		t.Errorf("the tools key: %q", moved.Path(config.FileTools))
	}
	if none := config.FromEnv(env(nil)); none.Path(config.FileLock) != "" || none.SettingsFile != "" {
		t.Errorf("no folder: lock %q, settings %q, want none", none.Path(config.FileLock), none.SettingsFile)
	}
}

// TestEveryVariableReadIsListed: FromEnv asks for exactly the variables
// Vars lists, so each has its doc row (cli.config.6).
func TestEveryVariableReadIsListed(t *testing.T) {
	t.Parallel()
	asked := map[string]bool{}
	home := t.TempDir()
	config.FromEnv(func(k string) string {
		asked[k] = true
		if k == appdirs.HomeVar {
			return home
		}
		return ""
	})
	if len(asked) == 0 {
		t.Fatal("FromEnv asked for nothing")
	}
	listed := map[string]bool{}
	systemRows := systemRowNames(t)
	for _, v := range config.Vars() {
		listed[v.Name] = true
		if v.Default == "" || v.Holds == "" || v.Refused == "" {
			t.Errorf("%s: default %q, holds %q, refused %q: each needs words", v.Name, v.Default, v.Holds, v.Refused)
		}
		// A variable of the other system's layout is listed and not read here.
		// appdirs asks for the ones of the system it is built for, and gragas
		// names no system: the rows of any layout are exempt.
		if !asked[v.Name] && !systemRows[v.Name] {
			t.Errorf("%s is listed and never read", v.Name)
		}
	}
	for k := range asked {
		// appdirs reads the data and run folders' variables too, and gragas
		// keeps nothing in those folders.
		if !listed[k] && !strings.HasPrefix(k, "XDG_") {
			t.Errorf("%s is read and not listed", k)
		}
	}
}

// TestEveryKeyHasADefaultAReaderADocRowAndARefusal: the table names each
// key's default and refused-when, each key is read at a value that is not
// its default, and each has a planted value that is refused naming the key
// and its line (cli.config.6). The doc is held to the table in
// TestTheDocSaysWhatTheTableSays.
func TestEveryKeyHasADefaultAReaderADocRowAndARefusal(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	keys := config.Keys()
	if len(keys) < 10 {
		t.Fatalf("only %d keys", len(keys))
	}
	for _, k := range keys {
		if k.Default == "" || k.Holds == "" || k.Refused == "" {
			t.Errorf("%s: default %q, holds %q, refused %q: each needs words", k.Name, k.Default, k.Holds, k.Refused)
		}
	}
	// A bad value of each key, on line 2 of a file whose line 1 is the version.
	bad := map[string]string{
		"version":    "version: two",
		"language":   "language: fr",
		"colour":     "colour: no",
		"shell":      "shell: bash",
		"programs":   "programs: {git: usr/bin/git}",
		"sandbox":    "sandbox: docker",
		"container":  "container: x",
		"userns":     "userns: yes",
		"macos":      "macos: []",
		"tools":      "tools: " + filepath.Join(dir, "etc") + "/../tools.yaml",
		"tool-paths": fmt.Sprintf("tool-paths: {%q: {golangci: %q}}", filepath.Join(dir, "src"), filepath.Join(dir, "bin", "golangci")),
		"mirrors":    "mirrors: {\"https://example.com/rules\": \"https://alex:hunter2@example.com/rules\"}",
		"keep":       "keep: 30 days",
		"names-file": "names-file: names.txt",
		"base-ref":   "base-ref: --upload-pack=x",
		"mounts":     "mounts: [srv/data]",
	}
	for _, k := range keys {
		line, ok := bad[k.Name]
		if !ok {
			t.Errorf("%s has no planted bad value", k.Name)
			continue
		}
		data := "version: 1\n" + line + "\n"
		if k.Name == "version" {
			data = line + "\n"
		}
		_, err := config.Parse(filepath.Join(dir, "c", "settings.yaml"), []byte(data))
		p, isProblem := errors.AsType[*config.Problem](err)
		want := filepath.Join(dir, "c", "settings.yaml") + ":2: " + k.Name
		if k.Name == "version" {
			want = filepath.Join(dir, "c", "settings.yaml") + ":1: version"
		}
		if !isProblem || p.Kind != config.Refused || !strings.HasPrefix(p.What, want) {
			t.Errorf("%s: %q, want refused starting %q (%v)", k.Name, data, want, err)
		}
	}
	if len(bad) != len(keys) {
		t.Errorf("%d planted values for %d keys", len(bad), len(keys))
	}
}

// TestEachKeyIsReadAtItsNonDefault: a file that sets every key to a value
// that is not its default gives exactly those settings.
func TestEachKeyIsReadAtItsNonDefault(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	data := `version: 1
language: en
colour: never
shell: PowerShell
programs:
  git: %s
  container: %s
  userns: %s
  macos: %s
  ssh: %s
  credential: %s
sandbox: userns
userns: {}
tools: %s
tool-paths:
  %s:
    golangci-lint@2.1.6: %s
mirrors:
  https://example.com/rulebook: %s
  ssh://git@example.com/rulebook: file:///srv/mirror/rulebook
keep: 12h
names-file: %s
base-ref: origin/main
mounts: [%s]
`
	data = fmt.Sprintf(data, filepath.Join(dir, "opt", "git", "bin", "git"),
		filepath.Join(dir, "usr", "bin", "podman"), filepath.Join(dir, "usr", "bin", "bwrap"), filepath.Join(dir, "usr", "bin", "sandbox-exec"),
		filepath.Join(dir, "usr", "bin", "ssh"), filepath.Join(dir, "usr", "libexec", "git-core", "git-credential-store"),
		filepath.Join(dir, "home", "alex", "tools.yaml"),
		filepath.Join(dir, "src", "app"), filepath.Join(dir, "opt", "golangci", "bin", "golangci-lint"),
		filepath.Join(dir, "srv", "mirror", "rulebook"), filepath.Join(dir, "home", "alex", "names.txt"), filepath.Join(dir, "srv", "data"))
	s, err := config.Parse(filepath.Join(dir, "c", "settings.yaml"), []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	// The shell key is held as the kit's String writes the name, whatever its case.
	want := config.Settings{Version: 1, Language: "en", Colour: "never", Shell: "powershell",
		Programs: config.Programs{Git: filepath.Join(dir, "opt", "git", "bin", "git"), Container: filepath.Join(dir, "usr", "bin", "podman"), Userns: filepath.Join(dir, "usr", "bin", "bwrap"), Macos: filepath.Join(dir, "usr", "bin", "sandbox-exec"),
			SSH: filepath.Join(dir, "usr", "bin", "ssh"), Credential: filepath.Join(dir, "usr", "libexec", "git-core", "git-credential-store")},
		Sandbox: "userns", Tools: filepath.Join(dir, "home", "alex", "tools.yaml"),
		// A tool's root and path are held in the slash form.
		ToolPaths: map[string]map[string]string{filepath.ToSlash(filepath.Join(dir, "src", "app")): {"golangci-lint@2.1.6": filepath.ToSlash(filepath.Join(dir, "opt", "golangci", "bin", "golangci-lint"))}},
		Mirrors:   map[string]string{"https://example.com/rulebook": filepath.Join(dir, "srv", "mirror", "rulebook"), "ssh://git@example.com/rulebook": "file:///srv/mirror/rulebook"},
		Keep:      "12h", NamesFile: filepath.Join(dir, "home", "alex", "names.txt"), BaseRef: "origin/main",
		Mounts: []string{filepath.Join(dir, "srv", "data")}}
	if !equal(s, want) {
		t.Errorf("settings\n%+v\nwant\n%+v", s, want)
	}
	d := config.Defaults()
	if d.Colour == s.Colour || d.Shell == s.Shell || d.Sandbox == s.Sandbox || d.Keep == s.Keep || d.NamesFile == s.NamesFile || d.BaseRef == s.BaseRef || d.Programs == s.Programs {
		t.Errorf("a value read is its default: %+v", d)
	}
	if s, err := config.Parse(filepath.Join(dir, "c", "settings.yaml"), []byte("version: 1\n")); err != nil || !equal(s, d) {
		t.Errorf("version alone: %+v, %v; want the defaults", s, err)
	}
}

func equal(a, b config.Settings) bool { return reflect.DeepEqual(a, b) }

// TestAnUnknownKeyIsRefusedNamingItAndItsLine (cli.config.2): at the top,
// in a table, and in another case, which is told its spelling.
func TestAnUnknownKeyIsRefusedNamingItAndItsLine(t *testing.T) {
	dir := t.TempDir()
	gitBin := filepath.Join(dir, "usr", "bin", "git")
	t.Parallel()
	tests := []struct{ name, data, says string }{
		{"a misspelled key", "version: 1\n\ncolr: never\n", filepath.Join(dir, "c", "s.yaml") + `:3: unknown key "colr": the keys are version, language, colour,`},
		{"a key in another case", "version: 1\nColour: never\n", filepath.Join(dir, "c", "s.yaml") + `:2: unknown key "Colour": keys are lower case, so write colour`},
		{"an unknown program", fmt.Sprintf("version: 1\nprograms:\n  git: %s\n  gti: %s\n", gitBin, gitBin), filepath.Join(dir, "c", "s.yaml") + `:4: unknown key "programs.gti": the keys are git, container, userns, macos, ssh, credential`},
		{"the signing key, which is gone", fmt.Sprintf("version: 1\nsigning-key: %s\n", filepath.Join(dir, "home", "alex", ".ssh", "release")), filepath.Join(dir, "c", "s.yaml") + `:2: signing-key is gone: gragas signs no release and reads no key, since a release is pinned by its content hash (docs/reasons/release-pin-by-hash.md); remove it`},
		{"the signing key in another case", fmt.Sprintf("version: 1\nSigning-Key: %s\n", filepath.Join(dir, "k")), filepath.Join(dir, "c", "s.yaml") + `:2: Signing-Key is gone: gragas signs no release`},
		{"the signing program, which is gone", fmt.Sprintf("version: 1\nprograms:\n  git: %s\n  signer: %s\n", gitBin, filepath.Join(dir, "usr", "bin", "ssh-keygen")), filepath.Join(dir, "c", "s.yaml") + `:4: programs.signer is gone: gragas runs no signing program, since a release is pinned by its content hash (docs/reasons/`},
		{"a program in another case", fmt.Sprintf("version: 1\nprograms:\n  Git: %s\n", gitBin), filepath.Join(dir, "c", "s.yaml") + `:3: unknown key "programs.Git": keys are lower case, so write git`},
		{"an option no sandbox has", "version: 1\nsandbox: container\ncontainer:\n  image: x\n", filepath.Join(dir, "c", "s.yaml") + `:4: unknown key "image" in container`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse(filepath.Join(dir, "c", "s.yaml"), []byte(tc.data))
			if p, ok := errors.AsType[*config.Problem](err); !ok || p.Kind != config.Refused || !strings.HasPrefix(p.What, tc.says) {
				t.Errorf("%v, want refused starting %q", err, tc.says)
			}
		})
	}
}

// TestTheVersionIsInsideTheFile (store.schemas.2): a newer one is not ready
// naming both versions, an older one or none is refused, and the version is
// judged before any key, so a newer file's keys are not read by this
// build's rules.
func TestTheVersionIsInsideTheFile(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	tests := []struct {
		name, data string
		kind       config.Kind
		says       string
	}{
		{"a newer version", "colour: sparkly\nversion: 2\n", config.NotReady, filepath.Join(dir, "c", "s.yaml") + ":2: the settings file is version 2, and this gragas reads version 1"},
		{"no version", "colour: never\n", config.Refused, filepath.Join(dir, "c", "s.yaml") + ":1: has no version: write version: 1 first"},
		{"version 0", "version: 0\n", config.Refused, filepath.Join(dir, "c", "s.yaml") + ":1: version 0 is not a settings version"},
		{"an empty file", "", config.Refused, filepath.Join(dir, "c", "s.yaml") + ": is not a table of settings"},
		{"not YAML", "version: [1\n", config.Refused, filepath.Join(dir, "c", "s.yaml") + ": is not YAML"},
		{"a list, not a table", "- version: 1\n", config.Refused, filepath.Join(dir, "c", "s.yaml") + ": is not a table of settings"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse(filepath.Join(dir, "c", "s.yaml"), []byte(tc.data))
			p, ok := errors.AsType[*config.Problem](err)
			if !ok || p.Kind != tc.kind || !strings.HasPrefix(p.What, tc.says) {
				t.Fatalf("%v, want kind %d starting %q", err, tc.kind, tc.says)
			}
			if tc.kind == config.NotReady && !strings.Contains(p.Next, "version 2") {
				t.Errorf("next = %q, want it to name version 2", p.Next)
			}
		})
	}
}

// TestAValueIsHeldToItsShape: the refusals beyond one per key.
func TestAValueIsHeldToItsShape(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	// src and root are a mirror source and a tool-paths root under the test's
	// folder, so the URL or tool under them reaches the shape check a subtest
	// means to test. climb is an absolute path that is not
	// clean. A path is embedded through %q, whose escaping of a backslash is
	// also a YAML double-quoted scalar's.
	src, root, climb := filepath.Join(dir, "a"), filepath.Join(dir, "src"), filepath.Join(dir, "usr")+"/../git"
	tests := []struct{ name, data, says string }{
		{"a table for a sandbox not named", "version: 1\nsandbox: userns\ncontainer: {}\n", filepath.Join(dir, "c", "s.yaml") + ":3: container is the options of sandbox: container, and sandbox is userns"},
		{"a table with the default sandbox", "version: 1\nmacos: {}\n", filepath.Join(dir, "c", "s.yaml") + ":2: macos is the options of sandbox: macos, and sandbox is none"},
		{"an unknown sandbox names the ones there are", "version: 1\nsandbox: docker\n", `sandbox must be none, container, userns or macos, and it is 'docker'`},
		{"a relative program", "version: 1\nprograms: {git: git}\n", `programs.git must be an absolute path, and it is "git"`},
		{"a program that climbs", fmt.Sprintf("version: 1\nprograms: {git: %q}\n", climb), fmt.Sprintf("programs.git must be a clean path: write %q", filepath.Clean(climb))},
		{"a path with a newline", fmt.Sprintf("version: 1\nnames-file: %q\n", filepath.Join(dir, "n\nx")), "names-file holds a control character"},
		{"a relative project root", "version: 1\ntool-paths: {src: {a@1: /a}}\n", `tool-paths.src must be an absolute path`},
		{"a tool with no version", fmt.Sprintf("version: 1\ntool-paths: {%q: {golangci: %q}}\n", root, filepath.Join(dir, "a")), fmt.Sprintf("tool-paths.%s.golangci is not tool@version", root)},
		{"a tool that starts with a dash", fmt.Sprintf("version: 1\ntool-paths: {%q: {-x@1: %q}}\n", root, filepath.Join(dir, "a")), fmt.Sprintf("tool-paths.%s.-x@1 is not tool@version", root)},
		{"a mirror over http", fmt.Sprintf("version: 1\nmirrors: {%q: %q}\n", src, "http://example.com/r"), "must be an absolute path or an https, ssh or file URL"},
		{"a mirror that starts with a dash", fmt.Sprintf("version: 1\nmirrors: {%q: %q}\n", src, "-oProxyCommand=x"), "must be an absolute path or a URL"},
		{"a mirror with a password", fmt.Sprintf("version: 1\nmirrors: {%q: %q}\n", src, "https://alex:pw@example.com/r"), "holds a password"},
		{"an https mirror with a user", fmt.Sprintf("version: 1\nmirrors: {%q: %q}\n", src, "https://alex@example.com/r"), "names a user, which only an ssh URL may: write https://example.com/r"},
		{"a file mirror with a user", fmt.Sprintf("version: 1\nmirrors: {%q: %q}\n", src, "file://alex@/x/r"), "names a user, which only an ssh URL may"},
		{"an ssh host that starts with a dash", fmt.Sprintf("version: 1\nmirrors: {%q: %q}\n", src, "ssh://-oProxyCommand=x/r"), `names the host "-oProxyCommand=x"`},
		{"an ssh user that starts with a dash", fmt.Sprintf("version: 1\nmirrors: {%q: %q}\n", src, "ssh://-oProxyCommand=x@example.com/r"), `names the user "-oProxyCommand=x"`},
		{"an ssh user escaped to a dash", fmt.Sprintf("version: 1\nmirrors: {%q: %q}\n", src, "ssh://%2DoProxyCommand=x@example.com/r"), `names the user "-oProxyCommand=x"`},
		{"an ssh host escaped to a dash", fmt.Sprintf("version: 1\nmirrors: {%q: %q}\n", src, "ssh://%2DoProxyCommand=x/r"), "is not a URL"},
		{"an ssh user that is empty", fmt.Sprintf("version: 1\nmirrors: {%q: %q}\n", src, "ssh://@example.com/r"), `names the user ""`},
		{"a source host that starts with a dash", "version: 1\nmirrors: {\"ssh://-oProxyCommand=x/r\": /a}\n", `names the host "-oProxyCommand=x"`},
		{"a source that is relative", "version: 1\nmirrors: {rules: /a}\n", "must be an absolute path or an https, ssh or file URL"},
		{"keep of zero", "version: 1\nkeep: 0d\n", "keep must be a whole number from 1 to 9999"},
		{"keep in weeks", "version: 1\nkeep: 2w\n", "keep must be a whole number"},
		{"keep too long", "version: 1\nkeep: 10000d\n", "keep must be a whole number"},
		{"keep with two units", "version: 1\nkeep: 3dh\n", "keep must be a whole number"},
		{"programs not a table", "version: 1\nprograms: /x/git\n", "programs must be a table"},
		// git hands these two to its shell in single quotes
		{"an ssh path with a single quote", fmt.Sprintf("version: 1\nprograms: {ssh: %q}\n", filepath.Join(dir, "opt", "it's", "ssh")), `programs.ssh holds a single quote`},
		{"a credential path with a single quote", fmt.Sprintf("version: 1\nprograms: {credential: %q}\n", filepath.Join(dir, "opt", "it's", "helper")), `programs.credential holds a single quote`},
		{"an ssh path with a newline", fmt.Sprintf("version: 1\nprograms: {ssh: %q}\n", filepath.Join(dir, "opt", "a\nb", "ssh")), `programs.ssh holds a control character`},
		{"a relative credential helper", "version: 1\nprograms: {credential: git-credential-store}\n", `programs.credential must be an absolute path`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Parse(filepath.Join(dir, "c", "s.yaml"), []byte(tc.data))
			if p, ok := errors.AsType[*config.Problem](err); !ok || p.Kind != config.Refused || !strings.Contains(p.What, tc.says) {
				t.Errorf("%v, want refused saying %q", err, tc.says)
			}
		})
	}
	for _, ok := range []string{"keep: 1h", "keep: 9999d", "sandbox: container\ncontainer: {}", "sandbox: none",
		fmt.Sprintf("mirrors: {%q: %q}", src, "ssh://git@example.com/r"), fmt.Sprintf("mirrors: {%q: %q}", src, "https://example.com/r"), fmt.Sprintf("mirrors: {%q: %q}", src, "file:///srv/r"), fmt.Sprintf("tool-paths: {%q: {a.b_c-d@1.2.3+x: %q}}", root, filepath.Join(dir, "a")),
		fmt.Sprintf("programs: {ssh: %q, credential: %q}", filepath.Join(dir, "opt", "a b$c`d;e", "ssh"), filepath.Join(dir, "opt", "a b$c`d;e", "helper")), fmt.Sprintf("programs: {git: %q}", filepath.Join(dir, "opt", "it's", "git"))} {
		if _, err := config.Parse(filepath.Join(dir, "c", "s.yaml"), []byte("version: 1\n"+ok+"\n")); err != nil {
			t.Errorf("%q: %v, want it read", ok, err)
		}
	}
}

// reader is a settings reader over a map of files: a path it does not hold
// is missing.
func reader(files map[string]string, errs map[string]error) func(string, []string) ([]byte, error) {
	return func(p string, _ []string) ([]byte, error) {
		if err, ok := errs[p]; ok {
			return nil, err
		}
		if data, ok := files[p]; ok {
			return []byte(data), nil
		}
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
}

// TestLoadEndsOnWhatTheReaderFound (cli.config.10): a missing file
// gives the defaults, an unreadable one fails, an unsafe one is refused with
// its fix, one this system cannot check is not ready, and the tools file is
// held to the same.
func TestLoadEndsOnWhatTheReaderFound(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	home := map[string]string{appdirs.HomeVar: filepath.Join(dir, "h")}
	file := filepath.Join(folders(t, home).Config, "settings.yaml")
	tools := filepath.Join(folders(t, home).Config, "tools.yaml")
	unsafe := refusing{"can be written by its group", "chmod go-w '" + file + "'"}
	tests := []struct {
		name  string
		env   map[string]string
		files map[string]string
		errs  map[string]error
		kind  config.Kind
		says  string
	}{
		{"a file that cannot be read fails", home, nil, map[string]error{file: &fs.PathError{Op: "open", Path: file, Err: fs.ErrPermission}},
			config.Failed, "the settings file " + file + " could not be read: open " + file + ": permission denied"},
		{"an unsafe file is refused with its fix", home, nil, map[string]error{file: unsafe},
			config.Refused, "the settings file " + file + " can be written by its group, and it names the programs gragas runs, so it is not read. Fix it with `chmod go-w '" + file + "'`"},
		{"a file this system cannot check is not ready", home, nil, map[string]error{file: fmt.Errorf("%s: no owner here: %w", file, errors.ErrUnsupported)},
			config.NotReady, "the settings file " + file + " exists, and its owner and mode cannot be checked"},
		{"an unsafe tools file is refused", home, map[string]string{file: "version: 1\n"}, map[string]error{tools: refusing{"is not a regular file", ""}},
			config.Refused, "the tools file " + tools + " is not a regular file"},
		{"the tools key moves the file checked", home, map[string]string{file: fmt.Sprintf("version: 1\ntools: %q\n", filepath.Join(dir, "t", "tools.yaml"))}, map[string]error{filepath.Join(dir, "t", "tools.yaml"): refusing{"is owned by account 7", ""}},
			config.Refused, "the tools file " + filepath.Join(dir, "t", "tools.yaml") + " is owned by account 7"},
		{"an unsafe tools file with no settings file is refused", home, nil, map[string]error{tools: refusing{"can be written by others", ""}},
			config.Refused, "the tools file " + tools + " can be written by others"},
		{"a GRAGAS_CONFIG that climbs is refused", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.ConfigVar: filepath.Join(dir, "x") + "/../s.yaml"}, nil, nil,
			config.Refused, fmt.Sprintf("GRAGAS_CONFIG must be a clean path: write %q", filepath.Join(dir, "s.yaml"))},
		{"a relative GRAGAS_NAMES_FILE is refused", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.NamesFileVar: "names.txt"}, nil, nil,
			config.Refused, "GRAGAS_NAMES_FILE must be an absolute path (the path is not shown"},
		{"a GRAGAS_NAMES_FILE that climbs is refused", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.NamesFileVar: filepath.Join(dir, "n") + "/../names.txt"}, nil, nil,
			config.Refused, "GRAGAS_NAMES_FILE must be a clean path, with no . or .. part"},
		{"a GRAGAS_NAMES_FILE with a newline is refused", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.NamesFileVar: filepath.Join(dir, "n\nx")}, nil, nil,
			config.Refused, "GRAGAS_NAMES_FILE holds a control character"},
		{"a GRAGAS_BASE_REF that git would read as an option is refused", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.BaseRefVar: "--output=/x"}, nil, nil,
			config.Refused, "GRAGAS_BASE_REF must be a revision, and it starts with '-'"},
		{"a GRAGAS_BASE_REF that names a range is refused", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.BaseRefVar: "main..HEAD"}, nil, nil,
			config.Refused, `GRAGAS_BASE_REF must be one revision, and it is "main..HEAD"`},
		{"a GRAGAS_SHELL that names no shell is refused, naming the ones there are", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.ShellVar: "bash"}, nil, nil,
			config.Refused, `GRAGAS_SHELL must be auto, posix, powershell or cmd, and it is "bash"`},
		{"a GRAGAS_SHELL with an escape is refused, the escape quoted", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.ShellVar: "posix\x1b[2J"}, nil, nil,
			config.Refused, `GRAGAS_SHELL must be auto, posix, powershell or cmd, and it is "posix\x1b[2J"`},
		{"a relative GRAGAS_CONFIG is refused", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.ConfigVar: "s.yaml"}, nil, nil,
			config.Refused, `GRAGAS_CONFIG must be an absolute path, and it is "s.yaml"`},
		{"a GRAGAS_CONFIG that names no file is not ready", map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.ConfigVar: filepath.Join(dir, "x", "s.yaml")}, nil, nil,
			config.NotReady, "GRAGAS_CONFIG names " + filepath.Join(dir, "x", "s.yaml") + ", which does not exist"},
		{"a refusal inside the file", home, map[string]string{file: "version: 1\ncolr: x\n"}, nil,
			config.Refused, file + `:2: unknown key "colr"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := config.Load(env(tc.env), "", reader(tc.files, tc.errs))
			p, ok := errors.AsType[*config.Problem](err)
			if !ok || p.Kind != tc.kind || !strings.HasPrefix(p.What, tc.says) {
				t.Fatalf("%v, want kind %d starting %q", err, tc.kind, tc.says)
			}
			if !equal(c.Settings, config.Defaults()) {
				t.Errorf("a run that ends on its settings carries %+v, want the defaults", c.Settings)
			}
		})
	}
	t.Run("a missing file gives the defaults", func(t *testing.T) {
		t.Parallel()
		c, err := config.Load(env(home), "", reader(nil, nil))
		if err != nil || !equal(c.Settings, config.Defaults()) || c.SettingsFile != file {
			t.Errorf("%+v, %v; want the defaults from %s", c.Settings, err, file)
		}
	})
	machine := fmt.Sprintf("version: 1\ntools:\n  lint-a@1.2.3: %s\n", filepath.Join(dir, "opt", "lint-a", "1.2.3", "lint-a"))
	t.Run("a safe tools file with no settings file gives the defaults and its tools", func(t *testing.T) {
		t.Parallel()
		c, err := config.Load(env(home), "", reader(map[string]string{tools: machine}, nil))
		if err != nil || !equal(c.Settings, config.Defaults()) || c.Machine.Tools["lint-a@1.2.3"] != filepath.ToSlash(filepath.Join(dir, "opt", "lint-a", "1.2.3", "lint-a")) {
			t.Errorf("%+v %+v, %v; want the defaults and lint-a 1.2.3", c.Settings, c.Machine, err)
		}
	})
	t.Run("a safe file is read and the tools file the tools key names is parsed", func(t *testing.T) {
		t.Parallel()
		c, err := config.Load(env(home), "", reader(map[string]string{file: fmt.Sprintf("version: 1\nkeep: 2d\ntools: %q\n", filepath.Join(dir, "t", "tools.yaml")), filepath.Join(dir, "t", "tools.yaml"): machine}, nil))
		if err != nil || c.Settings.Keep != "2d" || len(c.Machine.Tools) != 1 {
			t.Errorf("%+v %+v, %v; want keep 2d and one tool", c.Settings, c.Machine, err)
		}
	})
	t.Run("a tools file refused inside ends the run with nothing of either file", func(t *testing.T) {
		t.Parallel()
		c, err := config.Load(env(home), "", reader(map[string]string{file: "version: 1\nkeep: 2d\n", tools: "version: 1\ntool:\n"}, nil))
		p, ok := errors.AsType[*config.Problem](err)
		if !ok || p.Kind != config.Refused || !strings.HasPrefix(p.What, tools+`:2: unknown key "tool"`) {
			t.Fatalf("%v, want refused at %s:2", err, tools)
		}
		if !equal(c.Settings, config.Defaults()) || c.Machine.Tools != nil {
			t.Errorf("a run that ends on its tools file carries %+v %+v, want the defaults", c.Settings, c.Machine)
		}
	})
}

// TestTheEnvironmentDecidesOverTheFile: GRAGAS_NAMES_FILE over names-file,
// and NO_COLOR or CLICOLOR_FORCE over colour.
func TestTheEnvironmentDecidesOverTheFile(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	file := filepath.Join(folders(t, map[string]string{appdirs.HomeVar: filepath.Join(dir, "h")}).Config, "settings.yaml")
	files := map[string]string{file: fmt.Sprintf("version: 1\nnames-file: %s\ncolour: always\nbase-ref: origin/main\n", filepath.Join(dir, "from", "file"))}
	c, err := config.Load(env(map[string]string{appdirs.HomeVar: filepath.Join(dir, "h")}), "", reader(files, nil))
	if err != nil || c.NamesFile != filepath.Join(dir, "from", "file") || c.Colour() != "always" || c.BaseRef != "origin/main" {
		t.Errorf("the file alone: names %q, colour %q, base %q, %v", c.NamesFile, c.Colour(), c.BaseRef, err)
	}
	c, err = config.Load(env(map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.NamesFileVar: filepath.Join(dir, "from", "env"), config.BaseRefVar: "origin/release", "NO_COLOR": "1"}), "", reader(files, nil))
	if err != nil || c.NamesFile != filepath.Join(dir, "from", "env") || c.Colour() != "auto" || c.BaseRef != "origin/release" {
		t.Errorf("with the environment: names %q, colour %q, base %q, %v", c.NamesFile, c.Colour(), c.BaseRef, err)
	}
	c, _ = config.Load(env(map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), "CLICOLOR_FORCE": "1"}), "", reader(map[string]string{file: "version: 1\ncolour: never\n"}, nil))
	if c.Colour() != "auto" {
		t.Errorf("CLICOLOR_FORCE: colour %q, want the environment to decide", c.Colour())
	}
	if c := config.FromEnv(env(map[string]string{"CI": "true"})); !c.CI {
		t.Error("CI set, and not read")
	}
}

// TestTheDocSaysWhatTheTableSays: docs/settings.md has one row per key, per
// key of the tools file and per variable, in the table's order and with its words (cli.config.6).
func TestTheDocSaysWhatTheTableSays(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "settings.md"))
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, k := range config.Keys() {
		e := ""
		if k.Env != "" {
			e = "`" + k.Env + "`"
		}
		want = append(want, "| `"+k.Name+"` | "+k.Default+" | "+k.Holds+" | "+k.Refused+" | "+e+" |")
	}
	for _, k := range config.ToolsKeys() {
		want = append(want, "| `"+k.Name+"` | "+k.Default+" | "+k.Holds+" | "+k.Refused+" |")
	}
	for _, v := range config.Vars() {
		want = append(want, "| `"+v.Name+"` | "+v.Default+" | "+v.Holds+" | "+v.Refused+" |")
	}
	var got []string
	for l := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(l, "| `") {
			got = append(got, l)
		}
	}
	if len(got) == 0 {
		t.Fatal("docs/settings.md has no rows")
	}
	if !slices.Equal(got, want) {
		t.Errorf("docs/settings.md rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestTheTableHoldsWhatItLists: a kept file is held by its row in its place,
// and a file no row lists is not held.
func TestTheTableHoldsWhatItLists(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	table := config.Persisted()
	for _, tc := range []struct {
		place config.Place
		name  string
		held  bool
	}{
		{config.InState, "gragas.lock", true},
		{config.InState, "runs.jsonl", true},
		{config.InRepository, "rulebook/rules.json", true},
		{config.InRepository, "./rulebook/ids.lock", true},
		{config.InRepository, "rulebook/lang/go/README.md", false},
		{config.InRepository, "rulebook/lang/go/extra.md", false},
		{config.InRepository, "gragas.lock", false},
		{config.InState, "rulebook/rules.json", false},
		{config.InState, "notes.txt", false},
	} {
		if _, held := config.Holds(table, tc.place, tc.name); held != tc.held {
			t.Errorf("%s in %s: held %v, want %v", tc.name, tc.place, held, tc.held)
		}
	}
	for _, k := range table {
		if len(k.Names) == 0 || k.WrittenBy == "" || k.Rebuilt == "" || k.Durability == "" {
			t.Errorf("row %+v: every column needs words", k)
		}
	}
	e := moving(t, map[string]string{appdirs.HomeVar: filepath.Join(dir, "h")}, func(f string) string { return filepath.Join(dir, f) })
	c := config.FromEnv(env(e))
	if place, name, ok := c.Where(c.LockPath()); !ok || place != config.InState || name != "gragas.lock" {
		t.Errorf("the lock is at %v %q %v", place, name, ok)
	}
	if _, _, ok := c.Where(filepath.Join(dir, "elsewhere", "gragas.lock")); ok {
		t.Error("a path in none of the folders has a place")
	}
	// A path that climbs out of the state folder has no place only where it
	// is outside all three: a layout that keeps State inside Config
	// (appdirs' Windows one) climbs into Config, which is a place.
	f := folders(t, e)
	climbed := f.State + "/../outside"
	var holders []config.Place
	for _, d := range []struct {
		place config.Place
		dir   string
	}{{config.InState, f.State}, {config.InConfig, f.Config}, {config.InCache, f.Cache}} {
		if rel, err := filepath.Rel(d.dir, climbed); err == nil && filepath.IsLocal(rel) {
			holders = append(holders, d.place)
		}
	}
	switch place, name, ok := c.Where(climbed); {
	case len(holders) == 0 && ok:
		t.Errorf("%s climbs out of the state folder and is in none of the folders, and has the place %v %q", climbed, place, name)
	case len(holders) > 0 && (!ok || !slices.Contains(holders, place)):
		t.Errorf("%s climbs out of the state folder into %v, and has the place %v %q %v", climbed, holders, place, name, ok)
	}
}

// refusing is a reader's refusal, as host/fs's Unsafe gives one.
type refusing struct{ why, fix string }

func (r refusing) Error() string              { return r.why }
func (r refusing) Refusal() (why, fix string) { return r.why, r.fix }

// TestAKeyWrittenTwiceIsRefusedAtItsSecondLine: a file that says two things
// is ambiguous, so neither is taken, at the top and in a table (cli.config.2).
// A newer file is still not ready first.
func TestAKeyWrittenTwiceIsRefusedAtItsSecondLine(t *testing.T) {
	dir := t.TempDir()
	gitBin := filepath.Join(dir, "usr", "bin", "git")
	t.Parallel()
	tests := []struct{ name, data, says string }{
		{"a key at the top", "version: 1\ncolour: never\nkeep: 2d\ncolour: always\n", filepath.Join(dir, "c", "s.yaml") + ":4: colour is written twice, first on line 2: keep one"},
		{"the same value twice", "version: 1\ncolour: never\ncolour: never\n", filepath.Join(dir, "c", "s.yaml") + ":3: colour is written twice, first on line 2: keep one"},
		{"a key in a table", fmt.Sprintf("version: 1\nprograms:\n  git: %s\n  git: %s\n", gitBin, filepath.Join(dir, "opt", "git")), filepath.Join(dir, "c", "s.yaml") + ":4: programs.git is written twice, first on line 3: keep one"},
		{"the version", "version: 1\nversion: 1\n", filepath.Join(dir, "c", "s.yaml") + ":2: version is written twice, first on line 1: keep one"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := config.Parse(filepath.Join(dir, "c", "s.yaml"), []byte(tc.data))
			if p, ok := errors.AsType[*config.Problem](err); !ok || p.Kind != config.Refused || p.What != tc.says || !equal(s, config.Defaults()) {
				t.Errorf("%v, want refused %q with the defaults", err, tc.says)
			}
		})
	}
	t.Run("a newer file is not ready before a repeat is judged", func(t *testing.T) {
		t.Parallel()
		_, err := config.Parse(filepath.Join(dir, "c", "s.yaml"), []byte("version: 2\ncolour: never\ncolour: always\n"))
		if p, ok := errors.AsType[*config.Problem](err); !ok || p.Kind != config.NotReady {
			t.Errorf("%v, want not ready", err)
		}
	})
	t.Run("the clean control: each key once, the same key in two tables", func(t *testing.T) {
		t.Parallel()
		data := fmt.Sprintf("version: 1\nsandbox: container\nprograms:\n  container: %s\ncontainer: {}\n", filepath.Join(dir, "usr", "bin", "podman"))
		if _, err := config.Parse(filepath.Join(dir, "c", "s.yaml"), []byte(data)); err != nil {
			t.Errorf("refused: %v", err)
		}
	})
}

// TestTheFlagNamesTheFileOverTheVariable: --config, handed to Load by the
// root, names the settings file over GRAGAS_CONFIG and the config folder's,
// and a refusal about it names the flag (cli.config.5).
func TestTheFlagNamesTheFileOverTheVariable(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	both := map[string]string{appdirs.HomeVar: filepath.Join(dir, "h"), config.ConfigVar: filepath.Join(dir, "e", "s.yaml")}
	files := map[string]string{filepath.Join(dir, "e", "s.yaml"): "version: 1\ncolour: always\n", filepath.Join(dir, "f", "s.yaml"): "version: 1\ncolour: never\n"}
	c, err := config.Load(env(both), filepath.Join(dir, "f", "s.yaml"), reader(files, nil))
	if err != nil || c.SettingsFile != filepath.Join(dir, "f", "s.yaml") || !c.Named || c.Settings.Colour != "never" {
		t.Errorf("%+v, %v; want the flag's file", c, err)
	}
	c, err = config.Load(env(both), "", reader(files, nil))
	if err != nil || c.SettingsFile != filepath.Join(dir, "e", "s.yaml") || c.Settings.Colour != "always" {
		t.Errorf("%+v, %v; with no flag, want the variable's file", c, err)
	}
	_, err = config.Load(env(both), filepath.Join(dir, "f", "none.yaml"), reader(files, nil))
	if p, ok := errors.AsType[*config.Problem](err); !ok || p.Kind != config.NotReady ||
		p.What != "--config names "+filepath.Join(dir, "f", "none.yaml")+", which does not exist" || p.Next != "leave out --config, or write the file" {
		t.Errorf("%+v, want not ready naming the flag", err)
	}
	_, err = config.Load(env(both), "s.yaml", reader(files, nil))
	if p, ok := errors.AsType[*config.Problem](err); !ok || p.Kind != config.Refused || p.What != `--config must be an absolute path, and it is "s.yaml"` {
		t.Errorf("%+v, want refused naming the flag", err)
	}
}

func TestTheLookFollowsTheStreamTheEnvironmentAndTheFlags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		env   map[string]string
		tty   bool
		flags config.Flags
		want  config.Look
	}{
		{"a pipe has no width and no colour", map[string]string{"COLUMNS": "60"}, false, config.Flags{}, config.Look{}},
		{"a terminal that said nothing", nil, true, config.Flags{}, config.Look{Columns: 80, ColorBits: 8}},
		{"a terminal with its width and 24-bit colour", map[string]string{"COLUMNS": "60", "COLORTERM": "truecolor"}, true, config.Flags{}, config.Look{Columns: 60, ColorBits: 24}},
		{"NO_COLOR", map[string]string{"NO_COLOR": "1"}, true, config.Flags{}, config.Look{Columns: 80}},
		{"TERM=dumb", map[string]string{"TERM": "dumb"}, true, config.Flags{}, config.Look{Columns: 80}},
		{"CI", map[string]string{"CI": "1"}, true, config.Flags{}, config.Look{Columns: 80}},
		{"--no-color", nil, true, config.Flags{NoColor: true}, config.Look{Columns: 80}},
		{"--plain", nil, true, config.Flags{Plain: true}, config.Look{Columns: 80}},
		{"--accessible", nil, true, config.Flags{Accessible: true}, config.Look{Columns: 80, Accessible: true}},
		{"ACCESSIBLE", map[string]string{"ACCESSIBLE": "1"}, false, config.Flags{}, config.Look{Accessible: true}},
		{"a COLUMNS that is not a number", map[string]string{"COLUMNS": "wide"}, true, config.Flags{}, config.Look{Columns: 80, ColorBits: 8}},
		{"CLICOLOR_FORCE paints a pipe", map[string]string{"CLICOLOR_FORCE": "1"}, false, config.Flags{}, config.Look{Columns: 80, ColorBits: 8}},
		{"NO_COLOR over CLICOLOR_FORCE", map[string]string{"CLICOLOR_FORCE": "1", "NO_COLOR": "1"}, false, config.Flags{}, config.Look{}},
		{"CI over CLICOLOR_FORCE", map[string]string{"CLICOLOR_FORCE": "1", "CI": "1"}, false, config.Flags{}, config.Look{}},
		{"--no-color over CLICOLOR_FORCE", map[string]string{"CLICOLOR_FORCE": "1"}, false, config.Flags{NoColor: true}, config.Look{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := config.FromEnv(env(tc.env)).Look(tc.tty, 0, tc.flags); got != tc.want {
				t.Errorf("Look = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTheColourSettingMovesTheLook(t *testing.T) {
	t.Parallel()
	look := func(colour string, e map[string]string, tty bool) config.Look {
		c := config.FromEnv(env(e))
		c.Settings.Colour = colour
		return c.Look(tty, 0, config.Flags{})
	}
	if got := look("never", nil, true); got.ColorBits != 0 || got.Columns != 80 {
		t.Errorf("never on a terminal = %+v, want a width and no colour", got)
	}
	if got := look("always", nil, false); got.ColorBits != 8 {
		t.Errorf("always on a pipe = %+v, want colour", got)
	}
	if got := look("always", map[string]string{"NO_COLOR": "1"}, false); got.ColorBits != 0 {
		t.Errorf("always with NO_COLOR = %+v, want none: the environment decides over the file", got)
	}
	if got := look("auto", nil, false); got != (config.Look{}) {
		t.Errorf("auto on a pipe = %+v, want the zero look", got)
	}
}

// TestTheWidthIsTheTerminalsAndCOLUMNSOnlyOverridesIt: with COLUMNS unset a
// terminal draws at the width the system measured, and COLUMNS wins when it
// is set; a terminal that did not say is 80.
func TestTheWidthIsTheTerminalsAndCOLUMNSOnlyOverridesIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		env      map[string]string
		measured int
		want     int
	}{
		{"COLUMNS unset, a wide terminal", nil, 132, 132},
		{"COLUMNS unset, a narrow terminal", nil, 52, 52},
		{"COLUMNS set", map[string]string{"COLUMNS": "100"}, 132, 100},
		{"COLUMNS not a number", map[string]string{"COLUMNS": "wide"}, 132, 132},
		{"nothing measured", nil, 0, 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := config.FromEnv(env(tc.env)).Look(true, tc.measured, config.Flags{})
			if got.Columns != tc.want {
				t.Errorf("Columns = %d, want %d", got.Columns, tc.want)
			}
		})
	}
	if got := config.FromEnv(env(nil)).Look(false, 132, config.Flags{}); got.Columns != 0 {
		t.Errorf("a pipe has width %d, want none", got.Columns)
	}
}

// TestOnlyTruecolorOr24bitIsDeepColour: COLORTERM says 24-bit colour in those
// two words and no other, as the settings doc says.
func TestOnlyTruecolorOr24bitIsDeepColour(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, value string
		want        int
	}{
		{"truecolor", "truecolor", 24},
		{"24bit", "24bit", 24},
		{"any other word", "yes", 8},
		{"unset", "", 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := config.FromEnv(env(map[string]string{"COLORTERM": tc.value})).Look(true, 0, config.Flags{})
			if got.ColorBits != tc.want {
				t.Errorf("COLORTERM=%q: %d bits, want %d", tc.value, got.ColorBits, tc.want)
			}
		})
	}
}

// TestAHomeThatIsNotAnAbsoluteCleanPathIsNotReady: the home is
// held to the rule git's HomeOK holds it to, so a relative HOME, one that
// climbs, and one with a control character end the run not ready, naming HOME,
// with no folder joined to it, moving variables set or not (appdirs wants a home). An
// absolute HOME is the control run. Planted failure: Load's HOME check
// removed, or the folders joined to HOME as it is.
func TestAHomeThatIsNotAnAbsoluteCleanPathIsNotReady(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	xdg := func(home string) map[string]string {
		return moving(t, map[string]string{appdirs.HomeVar: home}, func(f string) string { return filepath.Join(dir, f) })
	}
	for _, tc := range []struct {
		name, home, says string
	}{
		{"a relative HOME", "h", appdirs.HomeVar + ` must be an absolute path, and it is "h"`},
		{"a dot HOME", ".", appdirs.HomeVar + ` must be an absolute path, and it is "."`},
		{"a HOME that climbs", filepath.Join(dir, "h") + "/../x", fmt.Sprintf("%s must be a clean path: write %q", appdirs.HomeVar, filepath.Join(dir, "x"))},
		{"a HOME with a newline", filepath.Join(dir, "h\nx"), appdirs.HomeVar + " holds a control character"},
		{"a HOME with an escape", filepath.Join(dir, "h\x1b[0m"), appdirs.HomeVar + " holds a control character"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := config.Load(env(map[string]string{appdirs.HomeVar: tc.home}), "", reader(nil, nil))
			p, ok := errors.AsType[*config.Problem](err)
			if !ok || p.Kind != config.NotReady || !strings.HasPrefix(p.What, tc.says) || !strings.Contains(p.Next, appdirs.HomeVar) {
				t.Fatalf("%v, want not ready starting %q with a next line naming %s", err, tc.says, appdirs.HomeVar)
			}
			if c.ConfigDir != "" || c.StateDir != "" || c.CacheDir != "" || c.SettingsFile != "" {
				t.Errorf("folders %q %q %q and settings %q joined to a HOME of %q", c.ConfigDir, c.StateDir, c.CacheDir, c.SettingsFile, tc.home)
			}
			// The variables that move the folders, set, do not make the home usable: appdirs wants one.
			if _, err := config.Load(env(xdg(tc.home)), "", reader(nil, nil)); err == nil {
				t.Errorf("with every moving variable set, HOME %q is still refused, yet Load ended ready", tc.home)
			}
		})
	}
	for _, home := range []string{filepath.Join(dir, "h"), filepath.Join(dir, "home", "a b")} {
		c, err := config.Load(env(map[string]string{appdirs.HomeVar: home}), "", reader(nil, nil))
		if err != nil || c.CacheDir != folders(t, map[string]string{appdirs.HomeVar: home}).Cache {
			t.Errorf("HOME %q: %v, cache %q", home, err, c.CacheDir)
		}
	}
}

// systemRowNames are the names of the help table's rows that differ by system,
// read from the data file that holds them.
func systemRowNames(t testing.TB) map[string]bool {
	t.Helper()
	b, err := os.ReadFile("vars_system.txt")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for line := range strings.SplitSeq(string(b), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, _, _ := strings.Cut(line, "\t")
		names[name] = true
	}
	return names
}
