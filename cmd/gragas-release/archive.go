package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// entry is one file of an archive.
type entry struct {
	Name string
	Mode int64
	Data []byte
}

// ReadArchive answers the files of a gzipped tar. A name that is absolute or
// climbs out of the folder it would be unpacked into is refused by name, and
// so is anything that is not a plain file or a folder.
func ReadArchive(data []byte) ([]entry, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var out []entry
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		skip, err := admit(h)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		out = append(out, entry{Name: h.Name, Mode: h.Mode, Data: b})
	}
}

// admit says whether a header is a file to read (false), or a folder entry,
// which holds no bytes, to skip (true). A name that leaves its folder, and
// anything that is not a plain file or a folder, is an error that names it.
func admit(h *tar.Header) (skip bool, err error) {
	switch h.Typeflag {
	case tar.TypeDir:
		if !safeName(strings.TrimSuffix(h.Name, "/")) {
			return false, fmt.Errorf("the archive holds %s, which leaves its folder", h.Name)
		}
		return true, nil
	case tar.TypeReg:
		if !safeName(h.Name) {
			return false, fmt.Errorf("the archive holds %s, which leaves its folder", h.Name)
		}
		return false, nil
	}
	return false, fmt.Errorf("the archive holds %s, which is not a plain file", h.Name)
}

// safeName says whether name stays inside the folder it is unpacked into: a
// relative, clean, slash-separated path that never climbs above it.
func safeName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return false
	}
	clean := path.Clean(name)
	return clean == name && clean != ".." && !strings.HasPrefix(clean, "../")
}
