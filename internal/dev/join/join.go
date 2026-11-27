// Package join writes the join of the command packages: the one generated file
// of internal/cli/root that names every package below internal/cli/cmd, so the
// root wires nothing by hand and adding a command is a new folder, not a new
// line in a shared file. The folders are read through fs.FS and the file comes
// back as bytes; the generator's main is the one place that touches the disk.
package join

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"regexp"
	"slices"
	"strings"
)

// Paths of the folder the join reads and of the file it makes.
const (
	CmdDir = "internal/cli/cmd"
	File   = "internal/cli/root/join_gen.go"
)

// ErrJoin is a command folder the join cannot take.
var ErrJoin = errors.New("a command folder the join cannot take")

// name is a folder a command package may have: a lowercase word, which is also
// its package name, so the join imports it by the folder.
var name = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// Names are the command packages below CmdDir in fsys, sorted: the folders that
// hold a Go file that is no test. A folder with none holds no command, and a
// folder whose name is not a lowercase word is refused.
func Names(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, CmdDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out, bad []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, err := fs.ReadDir(fsys, CmdDir+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(files, func(f fs.DirEntry) bool {
			return !f.IsDir() && strings.HasSuffix(f.Name(), ".go") && !strings.HasSuffix(f.Name(), "_test.go")
		}) {
			continue
		}
		if !name.MatchString(e.Name()) {
			bad = append(bad, e.Name())
			continue
		}
		out = append(out, e.Name())
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("%w: %s must be a lowercase word, which is its package name", ErrJoin, strings.Join(bad, ", "))
	}
	slices.Sort(out)
	return out, nil
}

// Render is the join file for module and the command packages names, which
// Names sorted. Each package must export Row, the function that returns its
// row, and New, the function that returns its cobra command, or the file does
// not compile. Each import is named by the folder and cmd, so no package
// clashes with a name the root declares.
func Render(module string, names []string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Written by gragas-dev gen commands from " + CmdDir + ", which task gen runs; edit no line of it.\n\npackage root\n\n")
	if len(names) > 0 {
		b.WriteString("import (\n")
		for _, n := range names {
			fmt.Fprintf(&b, "\t%scmd %q\n", n, module+"/"+CmdDir+"/"+n)
		}
		b.WriteString(")\n\n")
	}
	b.WriteString("// built are the command packages below " + CmdDir + ", by name: the row each\n// declares and the cobra command it makes.\nfunc built() []builtCommand {\n\treturn []builtCommand{\n")
	for _, n := range names {
		fmt.Fprintf(&b, "\t\t{Row: %scmd.Row(), New: %scmd.New},\n", n, n)
	}
	b.WriteString("\t}\n}\n")
	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("the join could not be formatted: %w", err)
	}
	return out, nil
}
