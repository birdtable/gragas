package coreapi

import (
	"errors"
	"fmt"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
	"github.com/birdtable/gragas/internal/port"
)

// ReadersFile is the file of the Assets port that says which tool configs
// gragas check reads, and how.
const ReadersFile = "audit/readers.yaml"

// Reader is one row of that file: a tool, the names its config may have at a
// repository's root, and the kind of read. Host is the os-files kind's key of
// the config whose files list spares the host layer.
type Reader struct {
	Tool  string
	Files []string
	Kind  string
	Host  string
	// Exclusions and Directive are the row's counted: the key
	// paths, from the config's root, of the lists whose items exclude code
	// from the tool, and the word of a Go comment that silences it at its
	// line. They are the tool's plugin's to give, in its manifest's counted;
	// the row holds them until the plugin is read.
	Exclusions [][]string
	Directive  string
	// Loosening is the row's counted scalars: each a key path
	// and the values at it that exclude more than the tool's default.
	Loosening []Loosening
	// FoldKeys is the row's counted.fold_keys: the tool reads a
	// config's keys without their case.
	FoldKeys bool
}

// Readers is the table of tool configs the check reads, loaded once by the
// entry point. When the file cannot be read, Refused says why and Rows is
// empty: the tool configs are then not checked, never half checked. The zero
// Readers holds none and refuses nothing, as in a build whose Assets port
// answers port.ErrUnavailable.
type Readers struct {
	Rows    []Reader
	Refused string
}

// LoadReaders reads the readers file from the Assets port.
func LoadReaders(a port.Assets) Readers {
	data, err := a.Read(ReadersFile)
	switch {
	case errors.Is(err, port.ErrUnavailable):
		return Readers{}
	case err != nil:
		return Readers{Refused: fmt.Sprintf("%s could not be read: %v", ReadersFile, err)}
	}
	rows, err := readerRows(data)
	if err != nil {
		return Readers{Refused: fmt.Sprintf("%s: %v", ReadersFile, err)}
	}
	return Readers{Rows: rows}
}

// readerRows is the rows of the document: each names a tool, a kind and one or
// more bare file names, and no tool twice.
func readerRows(data []byte) ([]Reader, error) {
	v, err := ordered.ParseStrict(data)
	doc, ok := v.(*ordered.Map)
	if err != nil || !ok {
		return nil, fmt.Errorf("it is no mapping: %w", err)
	}
	var rows []Reader
	seen := map[string]bool{}
	for i, m := range ordered.Maps(doc.List("readers")) {
		r := Reader{Tool: m.Str("tool"), Kind: m.Str("kind"), Host: m.Str("host")}
		for _, f := range m.List("files") {
			name, isName := f.(string)
			if !isName {
				return nil, fmt.Errorf("row %d: a file is not a name", i+1)
			}
			r.Files = append(r.Files, name)
		}
		if err := countedOf(&r, m); err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		if err := checkReader(r); err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		if seen[r.Tool] {
			return nil, fmt.Errorf("row %d: the tool %s is named twice", i+1, r.Tool)
		}
		seen[r.Tool] = true
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		return nil, errors.New("it holds no readers")
	}
	return rows, nil
}

// checkReader holds a row to its shape: a tool, a kind, and file names that
// are names at the root, never paths.
func checkReader(r Reader) error {
	if r.Tool == "" || r.Kind == "" || len(r.Files) == 0 {
		return errors.New("it needs a tool, a kind and a list of files")
	}
	for _, f := range r.Files {
		if f == "" || f == "." || f == ".." || strings.ContainsAny(f, `/\`) {
			return fmt.Errorf("the file %q is not a name at the root", f)
		}
	}
	return nil
}
