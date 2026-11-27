// Package treepin is the adapter of the pinned digest of an installed folder:
// the one package that links go-files' treesum. The browsers Playwright places
// are folders of thousands of files and some links, and the gate holds the tree
// to one digest checked in (supply.provenance.7) before a test runs one. The
// walk goes by handle beneath an os.Root and follows no link; it refuses a link
// that leaves the folder, a loop, a device, a FIFO, a socket and a file with a
// second name, and a tree past its caps.
//
// Check is a look before a use, as treesum's own is: a local writer of the
// folder between the check and the launch of a browser is outside this pin's
// threat model, which is the CDN and drift, not a hand on the disk.
package treepin

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/birdtable/go-files/treesum"
	"github.com/birdtable/gragas/internal/digest"
)

// The caps of a browsers folder. Measured on the real folder (Chromium's
// headless shell, its ffmpeg and WebKit, for macOS): about 7100
// entries and 540 MB. A folder past either is refused, so a hostile archive
// cannot make the walk run without end.
const (
	MaxEntries = 20_000
	MaxBytes   = 1 << 30
)

// Prefix opens a digest of this package: the sum of the browser folders' own
// treesum digests, each with its name. It is browsers2: since go-files
// v0.4.0, whose treesum lines start with t2:, so a folder's digest is
// another number than the one browsers1: pinned, which no longer covers the
// executable bit, so a pin of the older format is refused by name (Older).
const Prefix = "browsers2:"

// Older is the prefixes of the formats before Prefix, which Check refuses
// as such: the tree must be measured again.
func Older() []string { return []string{"browsers1:"} }

// ErrOlder is a pin of an older format than Prefix.
var ErrOlder = errors.New("the pin is of an older format than " + Prefix + ": measure the folder again")

// metadata is the folder Playwright keeps beside the browsers: a file named for
// the hash of the checkout's path that holds that absolute path. It differs
// between two clones of one tree, so it is no part of what is pinned, and the
// browsers' own folders are.
const metadata = ".links"

// MaxFolders is how many browser folders dir may hold: Chromium's headless
// shell, its ffmpeg and WebKit, with room for one more engine.
const MaxFolders = 8

// Sum is the digest of the browsers in dir: each folder of it summed by treesum,
// and the names and sums summed again, in name order. The metadata folder is
// left out. A file beside the folders, a link there, or more folders than
// MaxFolders is refused, as is a folder whose name is not plain (plainName), so the listing has one reading and nothing placed next to the browsers goes unpinned.
func Sum(dir string) (pin string, err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", fmt.Errorf("treepin: open %s: %w", dir, err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	top, err := root.Open(".")
	if err != nil {
		return "", fmt.Errorf("treepin: %s: %w", dir, err)
	}
	defer func() { err = errors.Join(err, top.Close()) }()
	// One batch: the metadata folder and MaxFolders folders, and one more to
	// show there are too many, so the top level is capped as the folders are.
	entries, err := top.ReadDir(MaxFolders + 2)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("treepin: %s: %w", dir, err)
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	var lines strings.Builder
	folders := 0
	for _, e := range entries {
		if e.Name() == metadata && e.IsDir() {
			continue
		}
		if !e.IsDir() {
			return "", fmt.Errorf("treepin: %s holds %q, which is not a folder of browsers", dir, e.Name())
		}
		if !plainName(e.Name()) {
			return "", fmt.Errorf("treepin: %s holds a folder named %q, which is not UTF-8 without a space or a control character", dir, e.Name())
		}
		if folders++; folders > MaxFolders {
			return "", fmt.Errorf("treepin: %s holds more than %d folders", dir, MaxFolders)
		}
		sum, _, err := treesum.Sum(root, e.Name(), treesum.Caps{Entries: MaxEntries, Bytes: MaxBytes})
		if err != nil {
			return "", fmt.Errorf("treepin: %s: %w", filepath.Join(dir, e.Name()), err)
		}
		lines.WriteString(e.Name() + " " + sum + "\n")
	}
	return Prefix + digest.Hex([]byte(lines.String())), nil
}

// plainName is a folder name the outer listing can hold without two folder
// sets giving one listing: UTF-8, with no space (the separator) and no control
// character (the newline that ends a line).
func plainName(name string) bool {
	return utf8.ValidString(name) && !strings.ContainsFunc(name, func(r rune) bool { return r == ' ' || unicode.IsControl(r) })
}

// Check refuses the folder at dir unless its digest is the one pinned. The
// refusal names both digests, so a person who bumped the browsers reads the
// new one from it. A pin of an older format is refused before the walk, with
// ErrOlder: it could never match, and a plain mismatch would hide why.
func Check(dir, pinned string) error {
	for _, old := range Older() {
		if strings.HasPrefix(pinned, old) {
			return fmt.Errorf("treepin: tools.yaml pins %s: %w", pinned, ErrOlder)
		}
	}
	got, err := Sum(dir)
	if err != nil {
		return err
	}
	if got != pinned {
		return fmt.Errorf("treepin: %s has digest %s, and tools.yaml pins %s", dir, got, pinned)
	}
	return nil
}
