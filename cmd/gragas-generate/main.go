// Command gragas-generate writes the models of the contract from its
// schemas: Go types, TypeScript types, zod schemas, the field reference and
// the explorer's field list. It is its own program, apart from gragas-dev, so
// the program that judges a change links no generator (A30).
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
	"path/filepath"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/dev/generate"
	"github.com/birdtable/gragas/internal/dev/generate/pluginwire"
	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/exit"
	"github.com/birdtable/gragas/internal/host/term"

	whole "github.com/birdtable/go-files/write"
)

const usage = `usage: gragas-generate [--check] [-C dir]

  Writes the Go types, the TypeScript types, the zod schemas, the field
  reference and the explorer's field list from the contract schemas. With
  --check, writes nothing and fails on a generated file that differs, is
  missing or is left over.
`

func main() {
	// Ctrl+C and SIGTERM are the kit's: its Main cancels the context the run is
	// given, on every system (gragas#1614). The generators take no context.
	os.Exit(term.Main(context.Background(), "gragas-generate", os.Stderr, func(_ context.Context, args []string) int {
		return run(args, os.Stdout, os.Stderr)
	}))
}

// run is the program with its streams, so a test drives it without a process.
func run(args []string, stdout, stderr io.Writer) int {
	return runGenerate(args, stdout, stderr)
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

const (
	exitDone   = exit.Done
	exitFailed = exit.Failed
	exitUsage  = exit.Usage
)

// errGenerate is --check finding a generated file that is not what the
// schemas make.
var errGenerate = errors.New("stale; run task gen")

// runGenerate is the generate command with its flags.
func runGenerate(args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("generate", flag.ContinueOnError)
	fl.SetOutput(stderr)
	check := fl.Bool("check", false, "write nothing; fail when a generated file is not what the schemas make")
	dir := fl.String("C", ".", "the repository holding the contract schemas")
	if err := fl.Parse(args); err != nil || fl.NArg() != 0 {
		talk.Text(usage)
		return exitUsage
	}
	if err := generateCommand(*dir, *check, &say.Speaker{W: stdout}); err != nil {
		talk.Text("gragas-generate: " + err.Error() + "\n")
		return exitFailed
	}
	return exitDone
}

// generatedOwned says whether the generator owns the name of path: a generated file in
// the contract model package, or a generated file or the index of the contract package.
// One of them that the schemas no longer make is a leftover, and goes.
func generatedOwned(path string) bool {
	switch filepath.ToSlash(filepath.Dir(path)) + "/" {
	case generate.GoDir, pluginwire.GoDir:
		return strings.HasSuffix(path, ".gen.go")
	case generate.TSDir:
		return strings.HasSuffix(path, ".gen.ts") || filepath.Base(path) == "index.ts"
	}
	return false
}

func generateCommand(dir string, check bool, out *say.Speaker) (err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	in, err := readGenerateInput(root)
	if err != nil {
		return err
	}
	made, err := generate.Generate(in)
	if err != nil {
		return err
	}
	if err := refuseHandWritten(root, made, out); err != nil {
		return err
	}
	paths, stale, err := staleFiles(root, made)
	if err != nil {
		return err
	}
	gone, err := leftovers(root, made)
	if err != nil {
		return err
	}
	if len(stale)+len(gone) == 0 {
		out.Text("the generated files are current\n")
		if check {
			// The count line (gragas#695): the files it compared, which the
			// gate runner holds to the generated-files member's floor.
			out.Text(fmt.Sprintf("count: %d files\n", len(paths)))
		}
		return out.Err
	}
	if check {
		for _, p := range stale {
			out.Text("stale " + p + "\n")
		}
		for _, p := range gone {
			out.Text("leftover " + p + "\n")
		}
		return errors.Join(errGenerate, out.Err)
	}
	return writeGenerated(root, made, stale, gone, out)
}

// staleFiles lists the paths the generator makes, sorted, and those of them
// whose file is missing or differs from what it makes.
func staleFiles(root *os.Root, made map[string][]byte) (paths, stale []string, err error) {
	paths = make([]string, 0, len(made))
	for p := range made {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range paths {
		have, err := root.ReadFile(filepath.FromSlash(p))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
		if err != nil || !bytes.Equal(have, made[p]) {
			stale = append(stale, p)
		}
	}
	return paths, stale, nil
}

func writeGenerated(root *os.Root, made map[string][]byte, stale, gone []string, out *say.Speaker) error {
	for _, p := range stale {
		full := filepath.FromSlash(p)
		if err := root.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := replaceIn(root, full, made[p]); err != nil {
			return err
		}
		out.Text("wrote " + p + "\n")
	}
	for _, p := range gone {
		if err := root.Remove(filepath.FromSlash(p)); err != nil {
			return err
		}
		out.Text("removed " + p + "\n")
	}
	return out.Err
}

// refuseHandWritten fails on each Go file directly in the contract model's folder,
// tests aside, that the generator does not make and does not own by name. The
// decides walk does not walk that package, for its generated codec
// (internal/decides_test.go, displayOnly), so a hand file there could read a
// display-only field in a decision nobody walks. Both modes refuse one and
// write nothing, and neither removes it, as it is somebody's work.
func refuseHandWritten(root *os.Root, made map[string][]byte, out *say.Speaker) error {
	hand, err := handWritten(root, made)
	if err != nil || len(hand) == 0 {
		return err
	}
	for _, p := range hand {
		out.Text("hand-written " + p + ": " + generate.GoDir + " holds only the Go gragas-generate makes, and tests\n")
	}
	return errors.Join(errGenerate, out.Err)
}

// handWritten is refuseHandWritten's list.
func handWritten(root *os.Root, made map[string][]byte) ([]string, error) {
	folder := strings.TrimSuffix(generate.GoDir, "/")
	entries, err := fs.ReadDir(root.FS(), folder)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hand []string
	for _, e := range entries {
		p := folder + "/" + e.Name()
		if _, ok := made[p]; ok || e.IsDir() || generatedOwned(p) || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			continue
		}
		hand = append(hand, p)
	}
	return hand, nil
}

// leftovers lists the files in the generated folders that the generator owns
// by name and no longer makes.
func leftovers(root *os.Root, made map[string][]byte) ([]string, error) {
	var gone []string
	for _, folder := range []string{strings.TrimSuffix(generate.GoDir, "/"), strings.TrimSuffix(pluginwire.GoDir, "/"), strings.TrimSuffix(generate.TSDir, "/")} {
		entries, err := fs.ReadDir(root.FS(), folder)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			p := folder + "/" + e.Name()
			if _, ok := made[p]; !ok && !e.IsDir() && generatedOwned(p) {
				gone = append(gone, p)
			}
		}
	}
	more, err := mirrorLeftovers(root, made)
	return append(gone, more...), err
}

// readGenerateInput reads the schemas and the consumers' lists under dir.
func readGenerateInput(root *os.Root) (generate.Input, error) {
	in := generate.Input{Schemas: map[string][]byte{}, Consumers: map[string][]byte{}}
	for _, r := range []struct {
		folder string
		into   map[string][]byte
		suffix string
	}{{generate.SchemaDir, in.Schemas, ".schema.json"}, {generate.ConsumersDir, in.Consumers, ".json"}} {
		entries, err := fs.ReadDir(root.FS(), strings.TrimSuffix(r.folder, "/"))
		if err != nil {
			return in, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), r.suffix) {
				continue
			}
			b, err := root.ReadFile(filepath.Join(filepath.FromSlash(r.folder), e.Name()))
			if err != nil {
				return in, fmt.Errorf("%s: %w", e.Name(), err)
			}
			r.into[e.Name()] = b
		}
	}
	plugin, err := readTree(root, pluginwire.SchemaDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return in, err
	}
	if len(plugin) > 0 {
		in.PluginSchemas = map[string][]byte{}
		for _, f := range pluginwire.Files() {
			in.PluginSchemas[f] = plugin[f]
		}
	}
	fixture, err := readTree(root, generate.FixtureDir)
	in.Fixture = fixture
	return in, err
}

// readTree reads every file below folder, by its path below it.
func readTree(root *os.Root, folder string) (map[string][]byte, error) {
	out := map[string][]byte{}
	base := strings.TrimSuffix(folder, "/")
	err := fs.WalkDir(root.FS(), base, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, err := root.ReadFile(filepath.FromSlash(p))
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		out[strings.TrimPrefix(p, folder)] = b
		return nil
	})
	return out, err
}

// mirrorLeftovers lists the files of the fixture's mirror that no file of the
// fixture makes any more.
func mirrorLeftovers(root *os.Root, made map[string][]byte) ([]string, error) {
	have, err := readTree(root, generate.FixtureMirrorDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var gone []string
	for p := range have {
		if _, ok := made[generate.FixtureMirrorDir+p]; !ok {
			gone = append(gone, generate.FixtureMirrorDir+p)
		}
	}
	slices.Sort(gone)
	return gone, err
}
