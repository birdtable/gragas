// Command gragas-dev holds the engine's own generators. It is built and run
// from this repository's Taskfile and is not shipped: the shipped gragas
// links nothing of it, and a hidden command in gragas would put it in that
// binary's closure (A30, supply.intake.2, arch.layers.13).
//
// Its first command, layers, renders docs/layers.md, the ESLint boundary
// golden and .golangci.yml, joined from the generated lint preset, lint/project.yml
// and layers.yaml, or with --check says
// which of them is stale and writes nothing.
//
// main is the only place that exits (go.errors.4).
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/birdtable/gragas/internal/dev/layers"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/host/clock"
	"github.com/birdtable/gragas/internal/host/term"

	whole "github.com/birdtable/go-files/write"
)

// The exit codes: done, stale under --check or failed, and usage, from
// internal/exit (cli.exit.2).
const (
	exitDone   = exit.Done
	exitFailed = exit.Failed
	exitUsage  = exit.Usage
)

const usage = `usage: gragas-dev layers [--check] [-C dir]
       gragas-dev surface [--check] [-C dir]
       gragas-dev gen commands [--check] [-C dir]
       gragas-dev tokens [--check] [-C dir]
       gragas-dev wide [-C dir] base
       gragas-dev boards check --node path [-C root]
       gragas-dev licences list file | ignore file
       gragas-dev fuzz [--fuzztime 60s] [--grace 30s] [--jobs n] [-C dir]
       gragas-dev gate check [-C dir]
       gragas-dev scan names [-C dir]
       gragas-dev census [-C dir]
       gragas-dev fetch id --platform goos/goarch [--check] [-C dir]
       gragas-dev preset-plugins [-C root]
       gragas-dev place src dst
       gragas-dev tools-check --platform goos/goarch [--go name,...] [--pnpm dir,...] [--node-version file] [id...] [-C dir]
       gragas-dev tools-go --platform goos/goarch id=package... [-C dir]
       gragas-dev pnpm [-C dir] [--tools dir] [--program pnpm|node] [--env K=V]... -- args
       gragas-dev lap [--dir folder] reset | begin name | end name | report
       gragas-dev secrets --scan staged|tree|revisions --gitleaks file --config file... [-C dir]
       gragas-dev self-check issues [--repo owner/name] [-C dir]
       gragas-dev names hash < names.txt > hashes.txt
       gragas-dev landing run | begin | check --tested sha | merge N --head sha --tested sha [-C dir]
       gragas-dev hook pre-commit | pre-push [remote url] < refs

  layers   render docs/layers.md, the ESLint boundary golden and the
           .golangci.yml (from the generated lint preset and lint/project.yml)
           from layers.yaml; with --check, write nothing and fail when
           either differs from what the table renders
  surface  write docs/surface.md from the code and docs/surface-marks.yaml; with --check
           write nothing and fail on an export not listed, a listed export gone,
           or a change whose digest is not the newest line of the change record
  gen      commands: write internal/cli/root/join_gen.go, the join of the packages below
           internal/cli/cmd; with --check, write nothing and fail when it differs
  tokens   write the CSS custom properties, the Tailwind 4 theme and the terminal's
           palette.json from explorer/packages/tokens/tokens.json, and refuse a
           document with a pair below its contrast floor; with --check, write
           nothing and fail when a file differs from what the document generates
  fuzz     run every Fuzz function for --fuzztime each, --jobs at once (default a
           quarter of the CPUs, at least one); a target still running --grace past
           its time fails; the first failure is named; each target fuzzes the whole
           --fuzztime with CPUs/jobs workers (at least one)
  gate     check: hold the files under gate/ to their schemas and to the tree:
           the exemptions, the standins and the shipped binary's closure, and
           the surface marks' base line to the base's
  fetch   place the release tool id of tools.yaml (golangci-lint) in .tools/bin: its
           archive and its program each held to the sha256 tools.yaml pins, refused
           and never placed on a difference; with --check, fetch nothing and fail
           unless the program there is the pinned one
  browsers hold the folder Playwright's own install placed (.tools/playwright) to the
           tree digest tools.yaml pins for --platform, refused on any difference
           and before a test starts a browser from it; with --print, print the
           folder's digest and check nothing
  preset-plugins hold root's plugins/lock.yaml to the pins of tools.yaml's plugins:
           block, approve each pinned plugin by its digest in a state folder made for
           the run alone (never the account's), and run gragas preset --check in root
           through them; gragas is built from the module here; any step that cannot
           be done is red, and nothing is skipped
  place    make dst the program src holds and remove src; dst is left alone, its
           modification time too, when it holds those bytes already, so a build
           with no change leaves bin/gragas as it was (Go's test cache reads its mtime)
  tools-check  hold every go tool of .tools/bin to tools/go.sum (its go version -m)
           every release tool to the sha256 tools.yaml pins, and each --pnpm
           folder's node_modules to its pnpm-lock.yaml, and with --node-version
           node's version to that file, in one process;
           it fetches and installs nothing, names each pin that is not held, and
           names task tools as the fix
  tools-go remove and build again, with tools.yaml's stated build, each go tool
           of .tools/bin that tools-check would refuse (missing, not built from
           tools/go.sum, or not its sha256 pin); a tool that holds is left alone
  pnpm     run the pinned pnpm (or node, with --program) in dir with the pinned tools folder first on PATH, joined
           with the system's own list separator, and pnpm kept from managing its own version; after "--" the words
           are the program's own, and its exit code is the command's
  lap      time the members of a tier: reset at its start, begin and end around a member, report at its end (the
           members slowest first, an unfinished one marked); the laps are lines of .tools/lap, and a lap never
           fails the tier, whatever it cannot read or write
  secrets  one pass of the secret scan: gitleaks over the stage (staged), the tracked
           files as they are (tree) or a revision range, once a --config, each named by
           absolute path; red on a finding, a .gitleaksignore, a shallow clone or a
           grafts file, a line git wrote to gitleaks's stderr, or a count of commits
           scanned that is not git's own log's
  scan    names: every tracked file and its path, the branch, and each commit
           past origin/main, its message and every line and path it adds,
           against the hashed list GRAGAS_NAMES_HASHES names, and each of those
           commits' addresses against the no-reply ones; it proves itself on
           plants first and fails closed; no tier runs it until #432, and
           task scan-names runs it by hand
  self-check  issues: read the Issue column of the table in docs/self-check.md that
           lists what gragas check finds in gragas, and ask GitHub (through gh, which
           holds its own login) whether each issue is open; red on a closed or a missing
           one, and on a row that names none; the release tier runs it, the commit and
           the push read no network
  names    hash: read names on stdin, one a line of 1 to 4 words, and print
           the lines of the list the names scan reads, a SHA-256 each; it
           never prints a name
  hook     what git's hooks run, one line each: pre-commit runs the clone's
           .tools/bin/task pre-commit; pre-push reads git's ref lines and runs
           task pre-push on each pushed tree, here when it is the clean
           checkout's, else in a throwaway worktree, and refuses a commit whose
           hook differs from the one that runs
  landing  run: the clone's task landing with no tool's switch from the caller (gragas#2169);
           begin: say which main the tree under landing was merged with and refuse a tree
           that does not hold it; check: fetch main and refuse, naming both shas, when it is
           not the main --tested names; merge: check, then gh pr merge N --squash
           --match-head-commit in the same process (the lander merges with it; gragas#1819)
`

func main() {
	// The blocked child is dispatched before anything else of gragas's runs, as
	// gragas-release's is: go-proc's block leaves open a descriptor another
	// goroutine makes without syscall.ForkLock, so sandbox.Run is called from a
	// process that has started no such goroutine (gragas#1759).
	if code, ok := blockedChild(); ok {
		os.Exit(code)
	}
	// Ctrl+C and SIGTERM are the kit's: its Main cancels the context the run is
	// given, on every system (gragas#1614).
	os.Exit(term.Main(context.Background(), "gragas-dev", os.Stderr, func(ctx context.Context, args []string) int {
		return run(ctx, args, os.Stdout, os.Stderr)
	}))
}

// blockedChild is gragas-dev started as the blocked child, when its first
// argument is exit.BlockedWord: it sets go-proc's block and execs the member.
// ok is false when it was started as anything else. The journeys run the
// block through this word (internal/journeys/blocked_test.go, gragas#1455); #911 took the runner that used the word, not the word.
// main and TestMain call it first; the command switch has no case for the word.
func blockedChild() (code int, ok bool) {
	if len(os.Args) < 2 || os.Args[1] != exit.BlockedWord {
		return 0, false
	}
	return runBlocked(os.Args[2:], os.Environ(), os.Stderr), true
}

// run is the program with its streams, so a test drives it without a process.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	m := realMachine(os.Environ())
	m.env = os.LookupEnv
	return runWith(ctx, m, args, stdout, stderr)
}

// runWith is run with the machine it reads from handed in.
func runWith(ctx context.Context, m machine, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "layers":
			return runLayers(args[1:], stdout, stderr)
		case "surface":
			return runSurface(args[1:], stdout, stderr)
		case "tokens":
			return runTokens(args[1:], stdout, stderr)
		case "gen":
			return runGen(args[1:], stdout, stderr)
		case "scan":
			return runScan(ctx, m.env, m.environ, args[1:], stdout, stderr)
		case "place":
			return runPlace(args[1:], stdout, stderr)
		case "fetch":
			return runFetch(ctx, args[1:], stdout, stderr)
		case "browsers":
			return runBrowsers(args[1:], stdout, stderr)
		case "preset-plugins":
			return runPresetPlugins(ctx, m.environ, args[1:], stdout, stderr)
		case "tools-check":
			return runToolsCheck(ctx, m.environ, args[1:], stdout, stderr)
		case "tools-go":
			return runToolsGo(ctx, m.environ, args[1:], stdout, stderr)
		case "lap":
			return runLap(ctx, args[1:], clock.Clock{}.Now, stdout, stderr)
		case "pnpm":
			return runPnpm(ctx, m.environ, args[1:], stdout, stderr)
		case "secrets":
			return runSecrets(ctx, m.environ, args[1:], stdout, stderr)
		case "names":
			return runNames(os.Stdin, args[1:], stdout, stderr)
		case "fuzz":
			return runFuzz(ctx, args[1:], stdout, stderr)
		case "wide":
			return runWide(ctx, m.environ, args[1:], stdout, stderr)
		case "boards":
			return runBoards(ctx, args[1:], stdout, stderr)
		case "licences":
			return runLicences(ctx, args[1:], stdout, stderr)
		case "self-check":
			return runSelfCheck(ctx, args[1:], stdout, stderr)
		case "gate":
			return runGate(ctx, m, args[1:], stdout, stderr)
		case "census":
			return runCensus(ctx, m.environ, args[1:], stdout, stderr)
		case "landing":
			return runLanding(ctx, args[1:], m.environ, ghOver(m.environ), stdout, stderr)
		case "hook":
			return runHook(ctx, m.environ, args[1:], os.Stdin, stdout, stderr)
		}
	}
	(&say.Speaker{W: stderr}).Text(usage)
	return exitUsage
}

// runLayers is the layers command with its flags.
func runLayers(args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("layers", flag.ContinueOnError)
	fl.SetOutput(stderr)
	check := fl.Bool("check", false, "write nothing; fail when a rendered file is stale")
	dir := fl.String("C", ".", "the repository holding layers.yaml")
	if err := fl.Parse(args); err != nil || fl.NArg() != 0 {
		talk.Text(usage)
		return exitUsage
	}
	if err := layersCommand(*dir, *check, &say.Speaker{W: stdout}); err != nil {
		talk.Text("gragas-dev layers: " + err.Error() + "\n")
		return exitFailed
	}
	return exitDone
}

// errStale is --check finding a rendered file out of date.
var errStale = errors.New("stale; run task gen")

// rendered is one file the table makes.
type rendered struct {
	path string
	data []byte
}

// layersCommand renders the files of the table in dir and writes the ones
// that differ, or under check lists them and writes none. A check that finds
// them current ends on its count line (gragas#695): the files it compared.
func layersCommand(dir string, check bool, out *say.Speaker) (err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	stale, compared, wide, err := staleFiles(root)
	if err != nil {
		return err
	}
	for _, line := range wide {
		out.Text(line + "\n")
	}
	if len(stale) == 0 {
		out.Text("the layer files are current\n")
		if check {
			out.Text(countLine(compared, "files") + "\n")
		}
		return out.Err
	}
	if check {
		for _, r := range stale {
			out.Text("stale " + r.path + "\n")
		}
		return errors.Join(errStale, out.Err)
	}
	for _, r := range stale {
		if err := replaceIn(root, r.path, r.data); err != nil {
			return err
		}
		out.Text("wrote " + r.path + "\n")
	}
	return out.Err
}

// staleFiles is the files of the table in root that are missing or differ
// from what the table renders, and how many it compared. Every read goes
// through root, so a path the table names that leaves the repository through
// a link is refused, not followed.
func staleFiles(root *os.Root) (stale []rendered, compared int, wide []string, err error) {
	table, err := root.ReadFile(layers.File)
	if err != nil {
		return nil, 0, nil, err
	}
	t, err := layers.Parse(table)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("%s: %w", layers.File, err)
	}
	pkgs, err := goPackages(root.FS())
	if err != nil {
		return nil, 0, nil, err
	}
	t.WithTree(pkgs)
	for _, p := range []string{t.DocFile, t.LintFile, t.EslintFile, t.LintBase, t.LintProject} {
		if !filepath.IsLocal(p) {
			return nil, 0, nil, fmt.Errorf("%s: %s is not a path inside the repository", layers.File, p)
		}
	}
	base, err := root.ReadFile(t.LintBase)
	if err != nil {
		return nil, 0, nil, err
	}
	project, err := root.ReadFile(t.LintProject)
	if err != nil {
		return nil, 0, nil, err
	}
	if t.Shared, err = sharedOf(root, t); err != nil {
		return nil, 0, nil, err
	}
	lint, err := t.Join(base, project)
	if err != nil {
		return nil, 0, nil, err
	}
	have, err := root.ReadFile(layers.AgentsFile)
	if err != nil {
		return nil, 0, nil, err
	}
	agents, err := t.Shared.Agents(have)
	if err != nil {
		return nil, 0, nil, err
	}
	for _, r := range []rendered{{t.DocFile, []byte(t.Doc())}, {t.LintFile, lint}, {t.EslintFile, []byte(t.Eslint())}, {layers.AgentsFile, agents}, {layers.SharedDocFile, []byte(t.Shared.Page())}} {
		have, err := root.ReadFile(r.path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, 0, nil, err
		}
		compared++
		if err != nil || !bytes.Equal(have, r.data) {
			stale = append(stale, r)
		}
	}
	return stale, compared, t.WideAlsos(), nil
}

// replaceIn writes data to path in root whole, through go-files/write's FileIn:
// a file beside it, then a rename, so an interrupted run leaves the old file
// or the new one. Every step walks from root again, so a path that leaves
// root, or a link or folder that leads out of it at any moment, is refused
// and nothing is written outside it (gragas#837, amending A71's check before
// the write). The file keeps the mode of the one it replaces, or is 0644.
// What it writes is made again from its sources, so neither the file nor the
// folder is flushed (whole.NoSync).
func replaceIn(root *os.Root, path string, data []byte) error {
	return whole.FileIn(root, path, data, whole.KeepMode(), whole.Mode(0o644), whole.NoSync())
}

// writeProgram writes a fetched program to path, executable, through
// go-files/write: a file beside it, then a rename, so a reader sees the old
// program or the whole new one. A fetch is made again by the next task tools,
// so neither the file nor the folder is flushed (whole.NoSync).
func writeProgram(path string, program []byte) error {
	return whole.File(path, program, whole.Mode(0o755), whole.NoSync())
}

// goPackages are the folders of fsys that hold a Go file, a link never followed,
// the ones no measure reads left out: the packages a pattern of the table holds.
func goPackages(fsys fs.FS) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != "." && skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if dir := path.Dir(p); filepath.Ext(p) == ".go" && d.Type().IsRegular() && !seen[dir] {
			seen[dir] = true
			out = append(out, dir)
		}
		return nil
	})
	return out, err
}

// countLine is the last line a check prints when it passes: what it read,
// n of unit (gragas#695).
func countLine(n int, unit string) string { return fmt.Sprintf("count: %d %s", n, unit) }
