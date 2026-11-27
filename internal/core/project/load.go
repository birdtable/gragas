package project

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// gitMarker is what the repository's root holds: a folder, or a file in a
// worktree.
const gitMarker = ".git"

// Loaded is a project Load found: the repository's root, the pin, where the
// pin is in that root, and the rulebook's root the pin's source names.
type Loaded struct {
	// Root is the repository's root, the folder that holds .git.
	Root model.Root
	// Pin is the pin, as ParsePin read it.
	Pin model.Pin
	// PinPath is the pin's path, slash-separated and relative to Root.
	PinPath string
	// Rulebook is the rulebook's root, opened inside Root, never above it.
	Rulebook model.Root
}

// NotReady is a project that is not there to load: no repository around
// the working folder, no pin in it, or no folder at the pin's source. Why
// names what was looked for and where. It ends "not ready".
type NotReady struct {
	// Why is what was looked for and not found, in a sentence.
	Why string
	// Err is the cause, when there is one.
	Err error
}

func (e *NotReady) Error() string { return e.Why }

func (e *NotReady) Unwrap() error { return e.Err }

// Word is the closing word a project not ready ends on.
func (e *NotReady) Word() model.Word { return model.NotReady }

// Load finds the project the working folder start, an absolute path, is in,
// and opens its rulebook. The repository's root is the nearest
// folder at or above start that holds .git, opened once by Tree.Locate; the
// pin, at pinFile below a folder (config.PinFile, handed by the caller), is
// looked for by Tree.Stat in start's folder and each one above it, nearest
// first, stopping at the root, so no pin above the repository is ever read.
// A pin that is there and is a folder, or cannot be read, is refused by
// name, never passed over for one higher up. The pin is read by ParsePin.
//
// The rulebook's root is opened by Tree.OpenIn on the repository's root, at
// the pin's source below the folder that holds the pin, and by no other
// call: no host path is built. A source that leaves the root, by .. or
// through a link that leads out, matches port.ErrOutside.
//
// No .git at or above start, no pin up to the root, and a source that is
// not there or is a file are each a *NotReady saying what was looked for.
func Load(tree port.Tree, start, pinFile string) (Loaded, error) {
	root, err := tree.Locate(start, gitMarker)
	if errors.Is(err, fs.ErrNotExist) {
		return Loaded{}, &NotReady{Why: fmt.Sprintf("no %s at or above %s: gragas reads a project in a git repository", gitMarker, start), Err: err}
	} else if err != nil {
		return Loaded{}, fmt.Errorf("find the repository around %s: %w", start, err)
	}
	here, err := place(tree, root, start)
	if err != nil {
		return Loaded{}, err
	}
	pinPath, folder, err := findPin(tree, root, here, pinFile)
	if err != nil {
		return Loaded{}, err
	}
	data, err := tree.Read(root, pinPath)
	if err != nil {
		return Loaded{}, &PinError{Finding{Path: pinPath, Msg: fmt.Sprintf("cannot be read: %v", err)}}
	}
	pin, err := ParsePin(pinPath, data)
	if err != nil {
		return Loaded{}, err
	}
	book, err := openSource(tree, root, pinPath, path.Join(folder, pin.Source))
	if err != nil {
		return Loaded{}, err
	}
	return Loaded{Root: root, Pin: pin, PinPath: pinPath, Rulebook: book}, nil
}

// place answers where start is in root, "" for the root itself. Within
// places no folder that is the root, so a start spelled otherwise than the
// root, through a link above it, is the root when it opens as that root.
func place(tree port.Tree, root model.Root, start string) (string, error) {
	if start == string(root) {
		return "", nil
	}
	rel, err := tree.Within(root, start)
	if err == nil {
		return rel, nil
	}
	if at, oerr := tree.Open(start); oerr == nil && at == root {
		return "", nil
	}
	return "", fmt.Errorf("place %s in the repository %s: %w", start, root, err)
}

// findPin looks for pinFile in here and each folder above it up to the
// root, nearest first, and answers the pin's path and the folder that holds
// it. Only a pin that is not there is passed over.
func findPin(tree port.Tree, root model.Root, here, pinFile string) (pinPath, folder string, err error) {
	for dir := here; ; dir = up(dir) {
		p := path.Join(dir, pinFile)
		e, err := tree.Stat(root, p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return "", "", &PinError{Finding{Path: p, Msg: fmt.Sprintf("cannot be read: %v", err)}}
		case e.Dir:
			return "", "", &PinError{Finding{Path: p, Msg: "is a folder: a pin is a file"}}
		default:
			return p, dir, nil
		}
		if dir == "" {
			break
		}
	}
	return "", "", &NotReady{Why: fmt.Sprintf("no %s in %s or a folder above it, up to the repository's root %s", pinFile, shown(here), root)}
}

// up is the folder above dir, "" being the root's.
func up(dir string) string {
	if d := path.Dir(dir); d != "." {
		return d
	}
	return ""
}

// shown is a folder of the root as a message names it.
func shown(dir string) string {
	if dir == "" {
		return "the repository's root"
	}
	return dir
}

// openSource opens the rulebook's folder dir, relative to the root, as a
// root of its own. A dir that is not there, or is a file, is not ready.
func openSource(tree port.Tree, root model.Root, pinPath, dir string) (model.Root, error) {
	book, err := tree.OpenIn(root, dir)
	switch {
	case err == nil:
		return book, nil
	case errors.Is(err, port.ErrOutside):
		return "", fmt.Errorf("%s: the rulebook's source %s leaves the repository, and only a folder in it is read: %w", pinPath, dir, err)
	case errors.Is(err, fs.ErrNotExist):
		return "", &NotReady{Why: fmt.Sprintf("%s: the rulebook's source %s is not there", pinPath, dir), Err: err}
	}
	if e, serr := tree.Stat(root, dir); serr == nil && !e.Dir {
		return "", &NotReady{Why: fmt.Sprintf("%s: the rulebook's source %s is a file, not a folder", pinPath, dir), Err: err}
	}
	return "", fmt.Errorf("%s: open the rulebook's source %s: %w", pinPath, dir, err)
}
