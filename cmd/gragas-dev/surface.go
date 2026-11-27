package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/dev/say"
	"github.com/birdtable/gragas/internal/dev/surface"
)

// surfaced are the packages whose exports the listing holds, from the
// module root: the ports and the model leaf they carry. The
// TypeScript packages join when they exist (M1-17, M8). Add only your own row.
func surfaced() []string {
	return []string{"internal/port", "internal/model", "internal/model/contract"}
}

// errSurface is the listing disagreeing with the code or the marks file.
var errSurface = errors.New("the surface listing does not hold")

// runSurface is the surface command with its flags.
func runSurface(args []string, stdout, stderr io.Writer) int {
	talk := &say.Speaker{W: stderr}
	fl := flag.NewFlagSet("surface", flag.ContinueOnError)
	fl.SetOutput(stderr)
	check := fl.Bool("check", false, "write nothing; fail when "+surface.File+" is stale or a change has no reason")
	dir := fl.String("C", ".", "the repository holding the packages")
	if err := fl.Parse(args); err != nil || fl.NArg() != 0 {
		talk.Text(usage)
		return exitUsage
	}
	if err := surfaceCommand(*dir, *check, &say.Speaker{W: stdout}); err != nil {
		talk.Text("gragas-dev surface: " + err.Error() + "\n")
		return exitFailed
	}
	return exitDone
}

// readSurfaced is the non-test Go files of every surfaced package in dir.
func readSurfaced(root *os.Root) ([]surface.Source, error) {
	var srcs []surface.Source
	for _, pkg := range surfaced() {
		entries, err := fs.ReadDir(root.FS(), pkg)
		if err != nil {
			return nil, err
		}
		src := surface.Source{Dir: pkg, Files: map[string][]byte{}}
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
				continue
			}
			data, err := root.ReadFile(path.Join(pkg, n))
			if err != nil {
				return nil, err
			}
			src.Files[n] = data
		}
		srcs = append(srcs, src)
	}
	return srcs, nil
}

// readOptional is the file at path in root, nil when it does not exist.
func readOptional(root *os.Root, path string) ([]byte, error) {
	data, err := root.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// surfaceCommand renders the listing of dir from its code and its marks file
// and writes it, or under check says what is wrong with the one on disk and
// writes none. It never writes the marks file: that is the author's. A listing
// whose digest is not the newest line of the change record is written all the
// same, and then fails, saying the line to add.
func surfaceCommand(dir string, check bool, out *say.Speaker) (err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	srcs, err := readSurfaced(root)
	if err != nil {
		return err
	}
	secs, err := surface.Collect(srcs)
	if err != nil {
		return err
	}
	if secs, err = withContractPackage(root, secs); err != nil {
		return err
	}
	var mf *surface.MarkFile
	marks, err := readOptional(root, surface.MarksFile)
	if err != nil {
		return err
	}
	if marks != nil {
		if mf, err = surface.ParseMarks(marks); err != nil {
			return err
		}
	}
	have, err := readOptional(root, surface.File)
	if err != nil {
		return err
	}
	if check {
		return checkSurface(secs, mf, have, out)
	}
	return writeSurface(root, secs, mf, have, out)
}

// checkSurface says what is wrong with the listing on disk, and writes nothing.
// When nothing is, it ends on its count line (gragas#695): the one listing it
// compared.
func checkSurface(secs []surface.Section, mf *surface.MarkFile, have []byte, out *say.Speaker) error {
	problems := surface.Problems(secs, mf, have)
	for _, p := range problems {
		out.Text(p + "\n")
	}
	if len(problems) > 0 {
		return errors.Join(errSurface, out.Err)
	}
	out.Text("the surface listing is current\n")
	out.Text(countLine(1, "files") + "\n")
	return out.Err
}

// writeSurface writes the listing of secs to its file in root.
func writeSurface(root *os.Root, secs []surface.Section, mf *surface.MarkFile, have []byte, out *say.Speaker) error {
	surface.Mark(secs, mf)
	var changes []surface.Entry
	if mf != nil {
		changes = mf.Changes
	}
	data := surface.Render(secs, changes)
	if string(data) != string(have) {
		if err := root.MkdirAll(path.Dir(surface.File), 0o755); err != nil {
			return err
		}
		if err := replaceIn(root, surface.File, data); err != nil {
			return err
		}
		out.Text("wrote " + surface.File + "\n")
	} else {
		out.Text("the surface listing is current\n")
	}
	if surface.Unrecorded(secs, mf) {
		return errors.Join(errors.New("the surface or a mark changed: "+surface.Why(secs, mf)), out.Err)
	}
	return out.Err
}

// contractExports is the file gragas-generate writes beside the TypeScript
// contract package, naming each export. It is read as data: this program links
// no generator.
const contractExports = "explorer/packages/contract/exports.gen.json"

// withContractPackage adds the TypeScript contract package's exports to the
// sections.
func withContractPackage(root *os.Root, secs []surface.Section) ([]surface.Section, error) {
	data, err := root.ReadFile(contractExports)
	if err != nil {
		return nil, err
	}
	var names []struct{ Name, Kind, Signature, Summary string }
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, fmt.Errorf("%s: %w", contractExports, err)
	}
	sec := surface.Section{Dir: path.Dir(contractExports)}
	for _, n := range names {
		sec.Exports = append(sec.Exports, surface.Export{Name: n.Name, Kind: n.Kind, Signature: n.Signature, Summary: n.Summary})
	}
	slices.SortFunc(sec.Exports, func(a, b surface.Export) int { return strings.Compare(a.Name, b.Name) })
	return append(secs, sec), nil
}
