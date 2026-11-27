package main

import (
	"context"
	"errors"
	"flag"
	"github.com/birdtable/gragas/internal/dev/say"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// runToolsGo is `tools-go --platform goos/goarch id=package...`: the go tools'
// part of task tools in one process (gragas#1655, X107). Each tool that is
// missing from .tools/bin, is not built from tools/go.sum or is not the sha256
// tools.yaml pins (tools-check's own judgement, goToolProblem) is removed and
// built again with the stated build of tools.yaml's header. The file is
// removed first: go reads the build ID inside an existing target and, when
// that is current, writes nothing, so a tool changed past its build ID would
// stay as it was (gragas#1480). A tool that holds is left alone, its
// modification time too. Every tool is tried, and each that does not build is
// named with the command that failed.
func runToolsGo(ctx context.Context, environ, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("tools-go", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "the folder the other paths are read from")
	lock := fl.String("lock", "tools/go.sum", "the lock file the go tools are built from, as go.sum lines")
	pins := fl.String("pins", "tools.yaml", "the file pinning the go tools' builds by sha256, and their ldflags")
	bin := fl.String("bin", ".tools/bin", "the folder the tools are built into")
	module := fl.String("module", "tools", "the folder of the tools module, which go build runs in")
	platform := fl.String("platform", "", "the system, goos/goarch, whose pins are read")
	pairs, err := parseInterspersed(fl, args)
	if err != nil || *platform == "" || len(pairs) == 0 {
		talk.Text(usage)
		return exitUsage
	}
	tools, err := goToolPairs(pairs)
	if err != nil {
		talk.Text("tools-go: " + err.Error() + "\n")
		return exitUsage
	}
	j := toolsGoJob{
		check:  toolsCheckJob{dir: *dir, lock: *lock, pins: *pins, bin: *bin, platform: *platform, environ: environ},
		module: *module,
		tools:  tools,
		build:  goBuild,
	}
	problems := toolsGo(ctx, j, &say.Speaker{W: stdout})
	for _, p := range problems {
		talk.Text(p + "\n")
	}
	if len(problems) > 0 {
		return exitFailed
	}
	return exitDone
}

// goTool is one go tool of the Taskfile's GO_TOOLS: its id, the name it is
// placed under in .tools/bin, and the package go builds it from.
type goTool struct{ id, pkg string }

// goToolPairs reads each id=package word of GO_TOOLS; a word without both
// halves is refused, so a typo never builds the wrong thing.
func goToolPairs(words []string) ([]goTool, error) {
	out := make([]goTool, 0, len(words))
	for _, w := range words {
		id, pkg, ok := strings.Cut(w, "=")
		if !ok || id == "" || pkg == "" || strings.ContainsAny(id, `/\`) {
			return nil, errors.New("a go tool is id=package, and " + w + " is not")
		}
		out = append(out, goTool{id: id, pkg: pkg})
	}
	return out, nil
}

// toolsGoJob is one tools-go: the paths and platform tools-check reads, the
// tools module's folder, the tools, and the build it runs for each that does
// not hold (goBuild, or a test's stand-in).
type toolsGoJob struct {
	check  toolsCheckJob
	module string
	tools  []goTool
	build  func(ctx context.Context, b goToolBuild) error
}

// goToolBuild is one stated build: go build in module, the row's ldflags, the
// package, and the file it writes, under environ, the environment main read.
type goToolBuild struct {
	module, ldflags, pkg, out string
	environ                   []string
}

// words is the go build command line of b, as tools.yaml's header states it.
func (b goToolBuild) words() []string {
	w := []string{"build", "-trimpath", "-buildvcs=false"}
	if b.ldflags != "" {
		w = append(w, "-ldflags="+b.ldflags)
	}
	return append(w, "-o", b.out, b.pkg)
}

// statedBuildEnv is what else reaches a go tool's file, fixed (gragas#1428): no
// cgo, the baseline architecture level, no experiment, no FIPS module, so two
// machines of one system print one sha256. GOTOOLCHAIN, GOFLAGS and GOWORK
// are the Taskfile's, which its guard holds before this runs.
func statedBuildEnv() []string {
	return []string{"CGO_ENABLED=0", "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOFIPS140=off"}
}

// goBuild runs the stated build b.
func goBuild(ctx context.Context, b goToolBuild) error {
	_, err := output(ctx, b.environ, b.module, statedBuildEnv(), "go", b.words()...)
	return err
}

// toolsGo builds again each tool of j that does not hold, and answers a line
// for each that could not be removed or built; none when every tool holds or
// was built. What holds and what was built is said on out.
func toolsGo(ctx context.Context, j toolsGoJob, out *say.Speaker) (problems []string) {
	c := j.check
	if c.lock == "" {
		c.lock = "tools/go.sum"
	}
	if c.bin == "" {
		c.bin = ".tools/bin"
	}
	if c.pins == "" {
		c.pins = "tools.yaml"
	}
	var files goToolFiles
	files.sums, files.sumErr = lockLines(under(c.dir, c.lock))
	files.pins, files.pinsErr = os.ReadFile(under(c.dir, c.pins))
	binDir, err := filepath.Abs(under(c.dir, c.bin))
	if err != nil {
		return []string{"tools-go: " + err.Error()}
	}
	module := under(c.dir, j.module)
	for _, tool := range j.tools {
		name := programName(tool.id, c.platform)
		if goToolProblem(ctx, c, tool.id, files) == "" {
			out.Text("tools-go: " + name + " holds its pins\n")
			continue
		}
		target := filepath.Join(binDir, name)
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			problems = append(problems, "tools-go: "+name+" fails its pins and cannot be removed before its build: "+err.Error())
			continue
		}
		b := goToolBuild{module: module, ldflags: goToolLdflags(files.pins, tool.id), pkg: tool.pkg, out: target, environ: c.environ}
		if err := j.build(ctx, b); err != nil {
			problems = append(problems, "tools-go: "+name+" did not build: go "+strings.Join(b.words(), " ")+" (in "+j.module+"): "+err.Error())
			continue
		}
		out.Text("tools-go: built " + name + "\n")
	}
	return problems
}

// goToolLdflags is the ldflags of the go tool id's row in tools.yaml, its one
// "    ldflags:" key, read from it and written nowhere else; "" when the row
// has none or the file is unread (the build then fails its pin, and
// tools-check names it).
func goToolLdflags(pins []byte, id string) string {
	lines, _ := goRowLines(pins, id)
	for _, line := range lines {
		if v, ok := strings.CutPrefix(line, "    ldflags: "); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
