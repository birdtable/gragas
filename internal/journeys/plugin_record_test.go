package journeys

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/birdtable/go-console/uitest"

	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// sha256Of is the prefixed SHA-256 of b, as a manifest's wasm key spells it.
func sha256Of(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// pluginBuilt is the standin plugin's module and the program it asks for, built
// once from the tree: the module with the command the design names (GOOS=wasip1
// GOARCH=wasm go build -trimpath), the program for this machine.
var pluginBuilt struct { //nolint:gochecknoglobals // one build for the whole run
	once       sync.Once
	dir        string
	wasm, prog string
	said       string
	err        error
}

func pluginBuild(t *testing.T) (wasm, prog string) {
	t.Helper()
	pluginBuilt.once.Do(func() {
		root := moduleRoot()
		dir, err := os.MkdirTemp("", "gragas-plugin-journey-")
		if err != nil {
			pluginBuilt.err = err
			return
		}
		pluginBuilt.dir = dir
		pluginBuilt.wasm, pluginBuilt.prog = filepath.Join(dir, "plugin.wasm"), filepath.Join(dir, "standin")
		for _, b := range []struct {
			out, pkg string
			env      []string
		}{
			{pluginBuilt.wasm, "./internal/dev/standin", []string{"GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0"}},
			{pluginBuilt.prog, "./internal/host/tool/testdata/standin/prog", nil},
		} {
			c := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-o", b.out, b.pkg) //nolint:gosec // this tree's own sources
			c.Dir, c.Env = root, append(configtest.Inherited(), b.env...)
			if said, err := c.CombinedOutput(); err != nil {
				pluginBuilt.err, pluginBuilt.said = err, string(said)
				return
			}
		}
	})
	if pluginBuilt.err != nil {
		t.Fatalf("could not build the standin plugin: %v\n%s", pluginBuilt.err, pluginBuilt.said)
	}
	return pluginBuilt.wasm, pluginBuilt.prog
}

// recordOpts vary the plugin a recording journey installs.
type recordOpts struct {
	// versions is the tool_versions of the manifest, and of its one program.
	versions string
	// grant is the id of the one program the manifest grants.
	grant string
	// unapproved leaves the plugin added and not approved.
	unapproved bool
	// contract and needs replace the manifest's contract number and needs list.
	contract, needs string
	// unadded leaves the author's folder written and the plugin not added.
	unadded bool
	// loadsCode writes config_loads_code: true into the manifest.
	loadsCode bool
}

// authorFolder is the author's folder, inside the project so that the lines a
// golden holds are the same on every machine.
const authorFolder = "author/standin/1.0.0"

// recordProject is a rulebook with the standin plugin added from the author's
// folder and approved. The folder's plugin.wasm is the installed module.
func recordProject(t *testing.T, o recordOpts) (dir string, env map[string]string) {
	t.Helper()
	if o.versions == "" {
		o.versions = ">=1.0.0 <2.0.0"
	}
	if o.grant == "" {
		o.grant = "standin"
	}
	if o.contract == "" {
		o.contract = "1"
	}
	if o.needs == "" {
		o.needs = "[]"
	}
	wasm, prog := pluginBuild(t)
	dir, env = promoteRepo(t), ruleEnv(t)
	module, err := os.ReadFile(wasm)
	if err != nil {
		t.Fatal(err)
	}
	manifest := "contract: " + o.contract + "\nneeds: " + o.needs + "\nname: standin\nversion: 1.0.0\ntool: standin\ntool_versions: \"" + o.versions + "\"\ntargets: [source]\nprovides: [entries, findings]\nextends: false\nwarning: false\n"
	if o.loadsCode {
		manifest += "config_loads_code: true\n"
	}
	manifest += manifestPrograms + "\n  - id: " + o.grant + "\n    path: bin/standin\n    version_argv: [--version]\n    versions: \"" + o.versions + "\"\nwasm: " + sha256Of(module) + "\n"
	if err := os.MkdirAll(filepath.Join(dir, authorFolder), 0o755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(dir, authorFolder), map[string]string{"plugin.yaml": manifest, "plugin.wasm": string(module)})
	put(t, dir, map[string]string{"src/a.txt": "BAD\n"})
	binary, err := os.ReadFile(prog)
	if err != nil {
		t.Fatal(err)
	}
	// A pinned find passes over a place at or below the project, so the program
	// is named by the settings from a folder of its own.
	tools := t.TempDir()
	if err := os.WriteFile(filepath.Join(tools, "standin"), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	tools, err = filepath.EvalSymlinks(tools)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := os.ReadFile(env["GRAGAS_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	settings = append(settings, "tool-paths:\n  "+physical+":\n    standin@1.4.2: "+filepath.Join(tools, "standin")+"\n"...)
	if err := os.WriteFile(env["GRAGAS_CONFIG"], settings, 0o600); err != nil {
		t.Fatal(err)
	}
	if o.unadded {
		return dir, env
	}
	if r := run(t, dir, env, "plugin", "add", authorFolder, "--apply"); r.code != 0 {
		t.Fatalf("plugin add: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	if o.unapproved {
		return dir, env
	}
	shown := run(t, dir, env, "plugin", "approve", "standin@1.0.0")
	m := recordDigest.FindString(shown.stdout + shown.stderr)
	if m == "" {
		t.Fatalf("plugin approve shows no digest\n%s%s", shown.stdout, shown.stderr)
	}
	if r := run(t, dir, env, "plugin", "approve", "standin@1.0.0", "--digest", m, "--apply"); r.code != 0 {
		t.Fatalf("plugin approve: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	return dir, env
}

var (
	recordDigest = regexp.MustCompile(`sha256:[0-9a-f]{64}`)
	recordRun    = regexp.MustCompile(`run [0-9a-f]{16}`)
)

// recordMasked is a transcript with what moves from one build or run to the
// next put back as a word: a digest (the module is built by this toolchain) and
// a run's id.
func recordMasked(args []string, r result) string {
	return recordRun.ReplaceAllString(recordDigest.ReplaceAllString(transcript(args, r), "sha256:<digest>"), "run <id>")
}

// TestPluginRecordFilesTheCaseAndTheRawOutputAndTheDryRunWritesNothing: the dry
// run names the steps of the first round and the files and writes nothing;
// --apply runs the tool through the installed plugin and files, in the author's
// folder, the case (request, answer) and the raw output of each run step under
// the tool's version, the scratch folder as /scratch; the installed copy is not
// written. A second run to the same case number has nothing to write. Risk: a
// recording that writes on a dry run, holds a path of this machine, or lands in
// the installed copy.
func TestPluginRecordFilesTheCaseAndTheRawOutputAndTheDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	dir, env := recordProject(t, recordOpts{})
	dry := []string{"plugin", "record", authorFolder, "--op", "findings", "--fixture", "src/a.txt"}
	for _, cols := range []string{"80", "40"} {
		env["COLUMNS"] = cols
		r := run(t, dir, env, dry...)
		if r.code != 6 || !strings.Contains(r.stdout, "Add --apply") {
			t.Errorf("dry run at %s: exit %d, want declined (6) on the apply line\n%s%s", cols, r.code, r.stdout, r.stderr)
		}
		uitest.Golden(t, "plugin-record-dry-"+cols, recordMasked(dry, r))
	}
	env["COLUMNS"] = "80"
	if _, err := os.Stat(filepath.Join(dir, authorFolder, "testdata")); err == nil {
		t.Fatal("the dry run wrote testdata")
	}
	apply := append(slices.Clone(dry), "--apply")
	if r := run(t, dir, env, apply...); r.code != 0 || !strings.Contains(r.stdout, "done") {
		t.Fatalf("apply: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	req := read(t, dir, authorFolder+"/testdata/cases/1-findings/request.json")
	for _, want := range []string{`"scratch":"/scratch"`, `"tool_version":"1.4.2"`, `"fixtures":["src/a.txt"]`, `"op":"findings"`} {
		if !strings.Contains(req, want) {
			t.Errorf("request.json lacks %s:\n%s", want, req)
		}
	}
	if !strings.Contains(read(t, dir, authorFolder+"/testdata/cases/1-findings/response.json"), `"findings"`) {
		t.Error("response.json holds no findings")
	}
	if exit := read(t, dir, authorFolder+"/testdata/recordings/1.4.2/0-0.exit"); strings.TrimSpace(exit) == "" {
		t.Error("the exit of the run step is not recorded")
	}
	for _, f := range []string{"testdata/cases/1-findings/request.json", "testdata/recordings/1.4.2/0-0.stderr"} {
		if got := read(t, dir, authorFolder+"/"+f); strings.Contains(got, os.TempDir()) || strings.Contains(got, dir) {
			t.Errorf("%s holds a path of this machine:\n%s", f, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "plugins", "standin", "1.0.0", "testdata")); err == nil {
		t.Error("a recording was written into the installed copy")
	}
	same := append(slices.Clone(dry), "--case", "1", "--apply")
	r := run(t, dir, env, same...)
	if r.code != 0 || !strings.Contains(r.stdout, "nothing") {
		t.Errorf("a second run to case 1: exit %d, want done with nothing to write\n%s%s", r.code, r.stdout, r.stderr)
	}
	next := run(t, dir, env, apply...)
	if next.code != 0 {
		t.Fatalf("a run with no --case: exit %d\n%s%s", next.code, next.stdout, next.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, authorFolder, "testdata/cases/2-findings/request.json")); err != nil {
		t.Errorf("a run with no --case did not take the next free number: %v", err)
	}
}

// TestPluginRecordRefusesBeforeAnyStepRuns: a folder whose plugin.wasm is not
// the installed module is refused naming both digests and the line that fixes
// it; a plugin that is added and not approved is the host's own refusal; a tool outside
// the plugin's version range is refused. Nothing is written by any of them.
// Risk: a recording made beside a module that did not make it, or by an
// unapproved plugin, or a refusal that leaves files behind.
func TestPluginRecordRefusesBeforeAnyStepRuns(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// folder is what is recorded; "" is the author's folder.
		folder string
		opts   recordOpts
		setup  func(t *testing.T, dir string)
		args   []string
		code   int
		wants  []string
		// golden is whether the transcript is a golden: the not-ready line of a
		// program the settings name holds that program's path on this machine.
		golden bool
	}{
		{"plugin-record-other-module", "", recordOpts{}, func(t *testing.T, dir string) {
			t.Helper()
			wasm := filepath.Join(dir, authorFolder, "plugin.wasm")
			body, err := os.ReadFile(wasm)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(wasm, append(body, 0), 0o644); err != nil {
				t.Fatal(err)
			}
		}, []string{"--op", "entries", "--apply"}, 5, []string{"plugin.wasm", "gragas plugin add " + authorFolder}, true},
		{"plugin-record-not-approved", "", recordOpts{unapproved: true}, nil, []string{"--op", "entries", "--apply"}, 5, []string{"approved", "gragas plugin approve standin@1.0.0"}, false},
		{"plugin-record-outside-range", "", recordOpts{versions: ">=2.0.0 <3.0.0"}, nil, []string{"--op", "entries", "--apply"}, 4, []string{"2.0.0"}, false},
		{"plugin-record-installed-copy", "plugins/standin/1.0.0", recordOpts{}, nil, []string{"--op", "entries", "--apply"}, 5, []string{"installed copy"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, env := recordProject(t, tc.opts)
			if tc.setup != nil {
				tc.setup(t, dir)
			}
			folder := authorFolder
			if tc.folder != "" {
				folder = tc.folder
			}
			args := append([]string{"plugin", "record", folder}, tc.args...)
			r := run(t, dir, env, args...)
			said := strings.Join(strings.Fields(r.stdout+r.stderr), " ")
			if r.code != tc.code {
				t.Errorf("exit %d, want %d\n%s%s", r.code, tc.code, r.stdout, r.stderr)
			}
			for _, w := range tc.wants {
				if !strings.Contains(said, w) {
					t.Errorf("output lacks %q\n%s%s", w, r.stdout, r.stderr)
				}
			}
			for _, f := range []string{authorFolder, "plugins/standin/1.0.0"} {
				if _, err := os.Stat(filepath.Join(dir, f, "testdata")); err == nil {
					t.Errorf("a refused recording wrote testdata in %s", f)
				}
			}
			if tc.golden {
				uitest.Golden(t, tc.name+"-80", recordMasked(args, r))
			}
		})
	}
}
