package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/go-proc/proc"
	"github.com/birdtable/gragas/internal/dev/say"
)

// hostPlatform is a platform whose program names are this system's, the first of
// two whose names differ for which programName gives a program the name
// proc.LocalFileName gives it here (programName reads only the system). The
// tools-check tests that start a program name it so.
func hostPlatform(t *testing.T) string {
	t.Helper()
	for _, p := range []string{"linux/amd64", "windows/amd64"} {
		if programName("tool", p) == proc.LocalFileName("tool") {
			return p
		}
	}
	t.Fatalf("no platform names a program %s, as this system does", proc.LocalFileName("tool"))
	return ""
}

// TestToolsCheckHoldsNodeToNodeVersionWhateverTheLineEnds (gragas#1655, X106; PR 1124,
// W-2 A): node prints its version with a carriage return on Windows, and a checkout may
// hold .node-version with CRLF, so both sides are trimmed before they are compared. The
// stand-in node prints the planted bytes, the same on every system. A node of another
// version, no node at all and an empty pin file are each red and named.
func TestToolsCheckHoldsNodeToNodeVersionWhateverTheLineEnds(t *testing.T) {
	t.Parallel()
	platform := hostPlatform(t)
	for name, c := range map[string]struct {
		prints, pin string
		node        bool
		want        string
	}{
		"Windows's carriage return":  {prints: "v22.11.0\r\n", pin: "22.11.0\n", node: true},
		"a CRLF pin file":            {prints: "v22.11.0\n", pin: "22.11.0\r\n", node: true},
		"both, and no newline":       {prints: "v22.11.0\r\n", pin: "22.11.0", node: true},
		"another version":            {prints: "v0.0.1\r\n", pin: "22.11.0\n", node: true, want: "tools-check: node is 'v0.0.1', .node-version pins v22.11.0: run task tools"},
		"no node":                    {pin: "22.11.0\n", want: "tools-check: node in .tools/bin did not print its version"},
		"an empty pin":               {prints: "v22.11.0\n", pin: "\r\n", node: true, want: "tools-check: .node-version is empty, so it pins no node"},
		"a node that prints nothing": {prints: "", pin: "22.11.0\n", node: true, want: "tools-check: node is '', .node-version pins v22.11.0"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			bin := filepath.Join(dir, ".tools", "bin")
			put(t, dir, ".node-version", c.pin)
			if c.node {
				installAs(t, bin, "node")
				put(t, bin, standinNodeFile, c.prints)
			}
			got := toolsCheck(t.Context(), toolsCheckJob{environ: os.Environ(), dir: dir, platform: platform, nodeVersion: ".node-version"}, &say.Speaker{W: &bytes.Buffer{}})
			switch {
			case c.want == "" && len(got) != 0:
				t.Errorf("node printing %q against a pin of %q: %q, want it held", c.prints, c.pin, got)
			case c.want != "" && (len(got) != 1 || !strings.HasPrefix(got[0], c.want)):
				t.Errorf("node printing %q against a pin of %q: %q, want one line starting %q", c.prints, c.pin, got, c.want)
			}
		})
	}
}

// buildLog is a stand-in go build for tools-go: it records each build, holds
// that the target was removed before it (gragas#1480: go keeps a target whose
// build ID it reads as current), writes the target, and fails the packages
// of fail.
type buildLog struct {
	builds []goToolBuild
	kept   []string
	fail   map[string]bool
}

func (l *buildLog) build(_ context.Context, b goToolBuild) error {
	l.builds = append(l.builds, b)
	if _, err := os.Stat(b.out); err == nil {
		l.kept = append(l.kept, filepath.Base(b.out))
	}
	if l.fail[b.pkg] {
		return errors.New("exit status 1: cannot find module")
	}
	return os.WriteFile(b.out, []byte("built"), 0o600)
}

// built is the names of the files the log built, in order.
func (l *buildLog) built() []string {
	var out []string
	for _, b := range l.builds {
		out = append(out, filepath.Base(b.out))
	}
	return out
}

// TestToolsGoBuildsOnlyAToolThatFailsItsPinsAndRemovesItFirst (gragas#1655, X107; the
// loop's own rules, gragas#1480 and #1428): a go tool that holds its pins, here the
// clone's task held by the lock alone, is not built; one that is missing and one that
// is not built from the lock are each built, the second removed before its build, with
// the row's ldflags from tools.yaml and in the tools module. A build that fails names
// the tool and the command, and the others are still built.
func TestToolsGoBuildsOnlyAToolThatFailsItsPinsAndRemovesItFirst(t *testing.T) {
	t.Parallel()
	dir, _ := checkedRepo(t, "gitleaks")
	pins, err := os.ReadFile(filepath.Join(dir, "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	row := "\n  - id: gitleaks\n    from: go\n    ldflags: -X=example.com/version.Version=1.0.0 \r\n    no-sha256: \"a plant\"\n"
	put(t, dir, "tools.yaml", string(pins)+row)
	log := &buildLog{fail: map[string]bool{"example.com/broken": true}}
	j := toolsGoJob{
		check:  toolsCheckJob{environ: os.Environ(), dir: dir, platform: "linux/amd64"},
		module: "tools",
		tools: []goTool{
			{id: "task", pkg: "github.com/go-task/task/v3/cmd/task"},
			{id: "dupl", pkg: "github.com/mibk/dupl"},
			{id: "gitleaks", pkg: "github.com/zricethezav/gitleaks/v8"},
			{id: "broken", pkg: "example.com/broken"},
		},
		build: log.build,
	}
	var out bytes.Buffer
	problems := toolsGo(t.Context(), j, &say.Speaker{W: &out})
	if got, want := log.built(), []string{"dupl", "gitleaks", "broken"}; !slices.Equal(got, want) {
		t.Errorf("built %q, want %q: the task that holds its pins is left alone, every other is built", got, want)
	}
	if len(log.kept) != 0 {
		t.Errorf("%q was still there when go build ran: a tool that fails its pins is removed first", log.kept)
	}
	for _, b := range log.builds {
		if b.module != filepath.Join(dir, "tools") || !filepath.IsAbs(b.out) {
			t.Errorf("%s was built in %q into %q, want the tools module and an absolute target", b.pkg, b.module, b.out)
		}
		want := ""
		if strings.HasSuffix(b.out, "gitleaks") {
			want = "-X=example.com/version.Version=1.0.0"
		}
		if b.ldflags != want {
			t.Errorf("%s was built with ldflags %q, want its row's %q, trimmed", filepath.Base(b.out), b.ldflags, want)
		}
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "tools-go: broken did not build: go build -trimpath -buildvcs=false -o ") || !strings.Contains(problems[0], "example.com/broken (in tools)") {
		t.Errorf("a build that fails: %q, want it alone named with its command", problems)
	}
	if !strings.Contains(out.String(), "tools-go: task holds its pins") {
		t.Errorf("the run did not say the task holds its pins:\n%s", out.String())
	}
}

// TestToolsGoHoldsAWindowsToolByItsExeNameWithoutAnExecuteBit (gragas#1088, W-2 B and C,
// held in Go since the loop left the Taskfile, gragas#1655): on windows/amd64 the tool is
// task.exe and carries no execute bit, and built from the lock it holds, so nothing is
// built; a bare dupl with no .exe is not the program Windows runs, so dupl.exe is built.
func TestToolsGoHoldsAWindowsToolByItsExeNameWithoutAnExecuteBit(t *testing.T) {
	t.Parallel()
	dir, _ := checkedRepo(t)
	bin := filepath.Join(dir, ".tools", "bin")
	task := programName("task", "windows/amd64")
	if err := os.Rename(filepath.Join(bin, "task"), filepath.Join(bin, task)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(bin, task), 0o600); err != nil {
		t.Fatal(err)
	}
	put(t, bin, "dupl", "a program by its unix name")
	log := &buildLog{}
	j := toolsGoJob{
		check: toolsCheckJob{environ: os.Environ(), dir: dir, platform: "windows/amd64"},
		tools: []goTool{{id: "task", pkg: "github.com/go-task/task/v3/cmd/task"}, {id: "dupl", pkg: "github.com/mibk/dupl"}},
		build: log.build,
	}
	if problems := toolsGo(t.Context(), j, &say.Speaker{W: &bytes.Buffer{}}); len(problems) != 0 {
		t.Fatalf("tools-go on windows/amd64: %q", problems)
	}
	if dupl := programName("dupl", "windows/amd64"); !slices.Equal(log.built(), []string{dupl}) {
		t.Errorf("built %q, want %s alone: %s at 0600 is held, and a bare dupl is not Windows's", log.built(), dupl, task)
	}
}

// TestToolsGoRefusesAWordThatIsNotIdEqualsPackage: GO_TOOLS is id=package words, and a
// word without both halves, or an id that is a path, ends on usage with nothing built.
func TestToolsGoRefusesAWordThatIsNotIdEqualsPackage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--platform", "linux/amd64", "dupl"},
		{"--platform", "linux/amd64", "=github.com/mibk/dupl"},
		{"--platform", "linux/amd64", "dupl="},
		{"--platform", "linux/amd64", "../dupl=github.com/mibk/dupl"},
		{"--platform", "linux/amd64"},
		{"dupl=github.com/mibk/dupl"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runToolsGo(t.Context(), os.Environ(), append(args, "-C", t.TempDir()), &stdout, &stderr); code != exitUsage {
			t.Errorf("tools-go %q: exit %d, want usage (%d)\n%s", args, code, exitUsage, stderr.String())
		}
	}
}

// TestTheStatedBuildIsTheHeadersOfToolsYaml holds the build tools-go runs to the one
// tools.yaml's header states (gragas#1428), word for word, so a change to either is a
// change to both.
func TestTheStatedBuildIsTheHeadersOfToolsYaml(t *testing.T) {
	t.Parallel()
	header, err := os.ReadFile(filepath.Join("..", "..", "tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	env := strings.Join(statedBuildEnv(), " ")
	if !strings.Contains(string(header), "#   "+env+" \\\n#     go build -C tools -trimpath -buildvcs=false [-ldflags=<the row's ldflags>] -o .tools/bin/<id> <package>\n") {
		t.Errorf("tools.yaml's header does not state the build tools-go runs: %s go build -C tools -trimpath -buildvcs=false [-ldflags=...] -o ... <package>", env)
	}
	b := goToolBuild{module: "tools", ldflags: "-X=a.b=c", pkg: "example.com/x", out: "/work/x"}
	if got, want := strings.Join(b.words(), " "), "build -trimpath -buildvcs=false -ldflags=-X=a.b=c -o /work/x example.com/x"; got != want {
		t.Errorf("the build's words are %q, want %q", got, want)
	}
}
