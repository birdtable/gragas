package fs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/birdtable/go-files/confine"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// Locate opens the nearest folder at or above start that holds marker. The
// walk is go-files' confine.Nearest up to the volume's top, held
// to the answers Locate gave by os.Stat before it, so the move changes no
// caller: Nearest asks each folder with Lstat, and two guards put back what
// os.Stat said. A marker that is a link to nothing counts to Nearest and not
// to Locate, so the walk goes on above it (and a marker os.Stat cannot follow
// for any other cause ends the walk on that error); and a folder where the
// marker's inner name is a file sends Nearest up, where Locate ends the walk
// on that error (innerFile).
func (t *Tree) Locate(start, marker string) (model.Root, error) {
	if _, err := local(marker); err != nil || marker == "" {
		return "", fmt.Errorf("locate %q: a marker is a path inside the folder: %w", marker, port.ErrOutside)
	}
	from, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("locate %s: %w", marker, err)
	}
	if info, err := os.Stat(from); err == nil && !info.IsDir() {
		return "", fmt.Errorf("locate %s: the start %s is a file, not a folder", marker, start)
	}
	for {
		dir, err := confine.Nearest(from, marker, "")
		if inner := innerFile(from, dir, marker); inner != nil {
			return "", inner
		}
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("locate %s from %s: %w", marker, start, fs.ErrNotExist)
		}
		if err != nil {
			return "", fmt.Errorf("locate %s: %w", marker, err)
		}
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return t.Open(dir)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("locate %s: %w", marker, err)
		}
		up := filepath.Dir(dir)
		if up == dir {
			return "", fmt.Errorf("locate %s from %s: %w", marker, start, fs.ErrNotExist)
		}
		from = up
	}
}

// innerFile is the error os.Stat gives in the first folder Nearest walked
// past, from from up to the folder below to (to "" when it found none, so up
// to the volume's top), that is not "no such file": in such a folder Lstat said
// the marker is not there or a part of the path to it is not a folder, and
// os.Stat says the same, so the error is "not a directory", which ended
// Locate's walk where it sends Nearest's up. That part is the marker's inner
// name, or, for a marker of one name too, the start itself when it is a file,
// so every marker is asked again.
func innerFile(from, to, marker string) error {
	for dir := from; dir != to; {
		if _, err := os.Stat(filepath.Join(dir, marker)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("locate %s: %w", marker, err)
		}
		if err := fileOnTheWay(dir, marker); err != nil {
			return err
		}
		up := filepath.Dir(dir)
		if up == dir {
			return nil
		}
		dir = up
	}
	return nil
}

// fileOnTheWay is the error for a marker whose inner name, below dir, runs
// through a file: dir/.gragas is a file where the marker .gragas/pin.yaml
// asks for a folder. Systems answer a path below a file differently (Linux
// says it is not a folder, Windows that it does not exist), so the walk asks
// for each folder of the marker's path itself and says the same on every
// system. It is nil when each one that is there is a folder.
func fileOnTheWay(dir, marker string) error {
	parts := strings.Split(marker, "/")
	for i := 1; i < len(parts); i++ {
		at := filepath.Join(dir, filepath.FromSlash(strings.Join(parts[:i], "/")))
		info, err := os.Stat(at)
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return fmt.Errorf("locate %s: %s is a file, not a folder", marker, at)
		}
	}
	return nil
}
