package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/birdtable/gragas/internal/dev/gate"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/host/jsonschema"
)

// reportMark starts a line that says what a check counted and fails nothing.
const reportMark = "report: "

// errGate is the gate check finding something wrong.
var errGate = errors.New("the gate registry does not hold")

// baseRef is the revision the head's change record of the surface is held to
// (#825). It is the remote branch named in full: git resolves a bare
// origin/main as a tag of that name before the remote branch, so anyone who
// can push a tag could choose the base (gragas#589).
const baseRef = "refs/remotes/origin/main"

// schemaNames are the schemas of gate/schema, by file.
func schemaNames() []string {
	return []string{"crash", "entries", "exemptions", "modules", "subsets"}
}

// machine is what the check reads from the machine: the go tool's view of
// the shipped binary, and git's view of the base's surface marks. A test
// hands in its own.
type machine struct {
	// environ is the environment gragas-dev was started with, whole, which
	// main reads and the steps it starts are given (cli.config.3).
	environ []string
	modules func(ctx context.Context, dir string) ([]string, error)
	// baseMarks is the surface marks file at the base revision, nil when the
	// base has none: the head keeps its change record's base line (#825).
	baseMarks func(ctx context.Context, dir, ref string) ([]byte, error)
	// baseFile is a file of the repository as the base revision has it, nil
	// when the base holds none: the subset list only shrinks against it
	// (gragas#341). Nil is no base.
	baseFile func(ctx context.Context, dir, ref, name string) ([]byte, error)
	// bases are the revisions the head is held to, each a ref the two readers
	// above take (basesFor, gragas#1866); nil is main alone.
	bases func(ctx context.Context, dir string) []string
	// env reads the environment, which only main may do (cli.config.3): main
	// sets it to os.LookupEnv.
	env func(string) (string, bool)
	// today is the day an exemption's expiry is judged on, YYYY-MM-DD: the
	// host's clock (clockToday). Nil gives no day, which is a finding, never
	// a pass.
	today func() string
}

// realMachine reads the go tool and git, each started with environ, the
// environment main read.
func realMachine(environ []string) machine {
	return machine{
		environ: environ,
		modules: func(ctx context.Context, dir string) ([]string, error) { return goLinkedModules(ctx, environ, dir) },
		baseMarks: func(ctx context.Context, dir, ref string) ([]byte, error) {
			return gitBaseMarks(ctx, environ, dir, ref)
		},
		baseFile: func(ctx context.Context, dir, ref, name string) ([]byte, error) {
			return gitBaseFile(ctx, environ, dir, ref, name)
		},
		bases: func(ctx context.Context, dir string) []string { return basesFor(ctx, environ, dir, "HEAD") },
		today: clockToday,
	}
}

// runGate is the gate command with its flags.
func runGate(ctx context.Context, m machine, args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	if len(args) == 0 || args[0] != "check" {
		talk.Text(usage)
		return exitUsage
	}
	fl := flag.NewFlagSet("gate check", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("C", ".", "the repository holding gate/")
	if err := fl.Parse(args[1:]); err != nil || fl.NArg() != 0 {
		talk.Text(usage)
		return exitUsage
	}
	if err := gateCommand(ctx, m, *dir, &say.Speaker{W: stdout}); err != nil {
		talk.Text("gragas-dev gate: " + err.Error() + "\n")
		return exitFailed
	}
	return exitDone
}

// gateCommand reads the registry in dir and the tree it holds, and says each
// finding.
func gateCommand(ctx context.Context, m machine, dir string, out *say.Speaker) (err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	in, err := gateInputs(ctx, m, root)
	if err != nil {
		return err
	}
	read := countReads(&in)
	bases := []string{baseRef}
	if m.bases != nil {
		bases = m.bases(ctx, root.Name())
	}
	// The head is held to each base it carries, and a loss against any is a
	// finding, said once (gragas#1866).
	var rep gate.Report
	seen := map[string]bool{}
	for i, ref := range bases {
		if m.baseFile != nil {
			in.Base = func(p string) ([]byte, error) { return m.baseFile(ctx, root.Name(), ref, p) }
		}
		kept, err := marksKept(ctx, m, root, ref)
		if err != nil {
			return err
		}
		one := gate.Check(in)
		if i == 0 {
			rep.Lengths = one.Lengths
		}
		for _, f := range append(one.Findings, kept...) {
			if !seen[f] {
				seen[f] = true
				rep.Findings = append(rep.Findings, f)
			}
		}
	}
	today := ""
	if m.today != nil {
		today = m.today()
	}
	ex := holdExemptions(root.FS(), today)
	rep.Findings = append(rep.Findings, ex.findings...)
	for _, l := range append(rep.Lengths, ex.line) {
		out.Text(reportMark + l + "\n")
	}
	for _, f := range rep.Findings {
		out.Text(f + "\n")
	}
	if len(rep.Findings) != 0 {
		return errors.Join(fmt.Errorf("%w: %d findings", errGate, len(rep.Findings)), out.Err)
	}
	out.Text("the gate registry holds\n")
	out.Text(countLine(len(read), "files") + "\n")
	return out.Err
}

// countReads makes in's reads counted: the set it returns holds each path a
// read gave content for, once. The gate's count line (gragas#695) is its size,
// the files of the registry and the tree the checks read. Check reads on one
// goroutine, so the set needs no lock.
func countReads(in *gate.Inputs) map[string]bool {
	read := map[string]bool{}
	inner := in.Read
	in.Read = func(p string) ([]byte, error) {
		data, err := inner(p)
		if err == nil {
			read[p] = true
		}
		return data, err
	}
	return read
}

// gateInputs gathers what the checks read. Every read of a path the registry
// names goes through root, so no row reaches a file outside the repository,
// by a path that climbs out, starts at a root or follows a link out of it: a
// count or an existence a finding prints would tell a head about a file of
// the machine that runs the gate (gragas#498).
func gateInputs(ctx context.Context, m machine, root *os.Root) (gate.Inputs, error) {
	dir, fsys := root.Name(), root.FS()
	set, err := jsonschema.Compile(fsys, gate.SchemaDir, withExt(schemaNames())...)
	if err != nil {
		return gate.Inputs{}, fmt.Errorf("the schemas of %s: %w", gate.SchemaDir, err)
	}
	mods, err := m.modules(ctx, dir)
	if err != nil {
		return gate.Inputs{}, err
	}
	in := gate.Inputs{
		Read: func(p string) ([]byte, error) { return fs.ReadFile(fsys, p) },
		Validate: func(schema string, doc any) []string {
			var out []string
			for _, p := range set.Validate(schema+".schema.json", doc) {
				if p.Line > 0 {
					out = append(out, fmt.Sprintf("line %d: %s", p.Line, p.Msg))
				} else {
					out = append(out, p.Msg)
				}
			}
			return out
		},
		Closure: mods,
		Names: func(d string) ([]string, error) {
			ents, err := fs.ReadDir(fsys, d)
			names := make([]string, 0, len(ents))
			for _, e := range ents {
				if e.Type().IsRegular() { // a folder or a link named like a record is none
					names = append(names, e.Name())
				}
			}
			return names, err
		},
	}
	return in, nil
}

func withExt(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = n + ".schema.json"
	}
	return out
}

// outputTimeout ends a command output runs that hangs (gragas#2263). It is a
// backstop: the longest is a go build of a tool with an empty build cache,
// minutes under load; a read of git or go takes seconds.
const outputTimeout = 30 * time.Minute

// output runs the program name with args in dir, with environ (the
// environment main read, cli.config.3) and env over it, and returns what it
// wrote to stdout; an error names the command, how it failed and its stderr.
func output(ctx context.Context, environ []string, dir string, env []string, name string, args ...string) ([]byte, error) {
	return outputIn(ctx, dir, overVars(environ, env), name, args...)
}

// overVars is environ with each variable of env in place of its own, so each
// name is given once and env's value wins, as os/exec gave a later one (proc
// hands the environment on as it is given).
func overVars(environ, env []string) []string {
	keys := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		keys = append(keys, k)
	}
	return append(withoutVars(environ, keys), env...)
}

// outputIn is output with environ the program's whole environment.
func outputIn(ctx context.Context, dir string, environ []string, name string, args ...string) ([]byte, error) {
	res, err := progRun{name: name, dir: dir, args: args, env: environ, timeout: outputTimeout}.do(ctx)
	what := strings.Join(append([]string{name}, args...), " ")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("%s: %s: %s", what, exitWords(res.Code), strings.TrimSpace(string(res.Stderr)))
	}
	return res.Stdout, nil
}

// goOver runs go with args over the tree in dir under goEnv over environ, for
// the host's own platform.
func goOver(ctx context.Context, environ []string, dir string, args ...string) ([]byte, error) {
	return output(ctx, environ, dir, goEnv(), "go", args...)
}

// goFor runs go with args over the tree in dir as goOver does, for the
// platform h.
func goFor(ctx context.Context, environ []string, h host, dir string, args ...string) ([]byte, error) {
	return output(ctx, environ, dir, append(goEnv(), "GOOS="+h.goos, "GOARCH="+h.goarch), "go", args...)
}

// closureHosts are the platforms the shipped binary's modules are read for:
// gate.ShippedPlatforms, every system the binary is built for. The list joins
// what each platform reads, so it is the same whichever host writes or checks
// it, and a module only one system links is on it for every host (A67,
// gragas#909).
func closureHosts() ([]host, error) {
	return hostsOf("gate.ShippedPlatforms", gate.ShippedPlatforms())
}

// hostsOf is each GOOS/GOARCH of platforms, the list named name, as a host.
func hostsOf(name string, platforms []string) ([]host, error) {
	var out []host
	for _, p := range platforms {
		goos, goarch, ok := strings.Cut(p, "/")
		if !ok || goos == "" || goarch == "" || strings.Contains(goarch, "/") {
			return nil, fmt.Errorf("%s names %q, which is not a system and an architecture", name, p)
		}
		out = append(out, host{goos: goos, goarch: goarch})
	}
	if len(out) == 0 {
		return nil, errors.New(name + " names no platform, so no closure can be read")
	}
	return out, nil
}

// goLinkedModules is the external modules the shipped binary links, as go
// list reads them for each platform of closureHosts with cgo off, joined, with
// the host's toolchain. A platform that cannot be read fails the whole read.
func goLinkedModules(ctx context.Context, environ []string, dir string) ([]string, error) {
	hosts, err := closureHosts()
	if err != nil {
		return nil, err
	}
	var mods []string
	for _, h := range hosts {
		out, err := goFor(ctx, environ, h, dir, "list", "-deps", "-f", "{{if and (not .Standard) .Module (not .Module.Main)}}{{.Module.Path}}{{end}}", "./cmd/gragas")
		if err != nil {
			return nil, fmt.Errorf("the linked modules for %s: %w", h.platform(), err)
		}
		for l := range strings.SplitSeq(string(out), "\n") {
			if l = strings.TrimSpace(l); l != "" && !slices.Contains(mods, l) {
				mods = append(mods, l)
			}
		}
	}
	slices.Sort(mods)
	return mods, nil
}

// goEnv is how every go command gragas-dev runs over the tree is run: with
// cgo off, with the host's own toolchain, and with no workspace. A go.mod
// that asks for a newer Go fails by name instead of having go fetch and run a
// toolchain (gragas#391), and a go.work in a folder above is never read
// (gragas#396).
func goEnv() []string {
	return []string{"GOTOOLCHAIN=local", "GOWORK=off", "CGO_ENABLED=0"}
}

// gitBaseFile is the file at name as the revision ref has it, or nil when
// ref holds no file there.
func gitBaseFile(ctx context.Context, environ []string, dir, ref, name string) ([]byte, error) {
	held, err := output(ctx, environ, dir, nil, "git", "ls-tree", "--name-only", ref, "--", name)
	if err != nil || len(held) == 0 {
		return nil, err
	}
	return output(ctx, environ, dir, nil, "git", "show", ref+":"+name)
}
