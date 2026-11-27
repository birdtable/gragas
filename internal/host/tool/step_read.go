package tool

// The read of a plugin's step: the files a glob matches
// in the scratch folder, where the config set is mirrored, held to the caps
// of the contract at the read, never after it.

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/birdtable/go-files/confine"

	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	"github.com/birdtable/gragas/internal/readglob"
)

// globParts is a glob of the contract split at its slashes, in the grammar
// internal/readglob holds: each part a name that may hold * and ?, or ** alone.
// Any glob character beyond those, a ** beside other characters, and a part
// that is no name once its * and ? are read as letters (empty, . or .., as
// confine.Local holds a path) are refused, matching port.ErrRefused, as the
// engine's codec refuses them; this is the host's own hold, so a step that
// came another way is held the same.
func globParts(g string) ([]string, error) {
	parts, err := readglob.Parts(g)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", err, port.ErrRefused)
	}
	sample := strings.NewReplacer("**", "x", "*", "x", "?", "x").Replace(g)
	if _, err := stepPath(sample); err != nil {
		return nil, fmt.Errorf("the glob %q is no path below the scratch folder: %w", g, err)
	}
	return parts, nil
}

// readGlob is the files of the folder root holds that glob matches, by path
// in order, each read through confine with no link followed at any part of
// its path, so a link in the scratch folder reads nothing out of it. A match
// that is no regular file, a link, a pipe or a device, is refused, matching
// port.ErrRefused, before anything is opened, so a pipe whose writer stays
// never holds the read. The caps hold as it reads: a match past
// plugin.MaxReadFiles, a file past plugin.MaxFile and the files past
// plugin.MaxFiles in all are refused, matching port.ErrRefused, having read
// at most one byte past the cap, never the whole file. A folder is walked,
// never read; a link to a folder is not walked.
func readGlob(root *os.Root, glob string) ([]plugin.File, error) {
	parts, err := globParts(glob)
	if err != nil {
		return nil, err
	}
	var names []string
	// The walk row of gate/shared.yaml (host/fs Tree.List) bans nothing yet,
	// and host/tool may not import host/fs, so the scratch folder
	// is walked here, through its os.Root, which no name leaves.
	err = fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." || d.IsDir() || !readglob.Match(parts, strings.Split(name, "/")) {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("the glob %q matches %s, which is no regular file but %v: %w", glob, name, d.Type(), port.ErrRefused)
		}
		if len(names) == plugin.MaxReadFiles {
			return fmt.Errorf("the glob %q matches more than %d files: %w", glob, plugin.MaxReadFiles, port.ErrRefused)
		}
		names = append(names, name)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("the read of %q: %w", glob, err)
	}
	files := make([]plugin.File, 0, len(names))
	total := 0
	for _, name := range names {
		data, err := readCapped(root, name, min(plugin.MaxFile, plugin.MaxFiles-total))
		if err != nil {
			return nil, err
		}
		total += len(data)
		files = append(files, plugin.File{Path: name, Data: stepBytes(data)})
	}
	return files, nil
}

// readCapped is the file name of root, refused, matching port.ErrRefused, when
// it holds more than capBytes: it reads at most one byte past the cap. The
// cap is the smaller of the file's and what is left of the read's in all. A
// name that is no regular file is refused before it is opened, and what was
// opened is looked at again before a byte is read, as a collect names its
// files with no walk before it.
func readCapped(root *os.Root, name string, capBytes int) ([]byte, error) {
	info, err := confine.NoLinks(root, name)
	if err == nil && info != nil && !info.Mode().IsRegular() {
		return nil, notRegular(name, info.Mode())
	}
	var data []byte
	if err == nil {
		data, err = confine.ReadCapped(root, name, int64(capBytes))
	}
	switch {
	case errors.Is(err, confine.ErrOverCap):
		return nil, fmt.Errorf("the file %s of the read holds more than %d bytes, the most one file gives (%d) or what is left of the read's %d in all: %w",
			name, capBytes, plugin.MaxFile, plugin.MaxFiles, port.ErrRefused)
	case errors.Is(err, confine.ErrNotRegular):
		// A pipe swapped in after the look is refused by the read, never waited on.
		return nil, fmt.Errorf("the file %s of the read is no regular file: %w: %w", name, port.ErrRefused, err)
	case errors.Is(err, confine.ErrOutside):
		return nil, fmt.Errorf("the file %s of the read is a link or leads out of the scratch folder: %w: %w", name, port.ErrRefused, err)
	case err != nil:
		return nil, fmt.Errorf("the file %s of the read: %w", name, err)
	}
	return data, nil
}

// stepBytes is data in the contract's form: text when it is valid UTF-8, and
// base64 when it is not.
func stepBytes(data []byte) plugin.Bytes {
	if utf8.Valid(data) {
		s := string(data)
		return plugin.Bytes{Text: &s}
	}
	s := base64.StdEncoding.EncodeToString(data)
	return plugin.Bytes{Base64: &s}
}

// notRegular is the refusal, matching port.ErrRefused, of the file name of a
// read or a collect that is no regular file: a pipe, a socket or a device
// may wait for a writer that never comes, and a read takes no ctx to end it.
func notRegular(name string, mode fs.FileMode) error {
	return fmt.Errorf("the file %s of the read is no regular file but %v: %w", name, mode.Type(), port.ErrRefused)
}
