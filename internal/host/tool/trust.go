package tool

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	gotrust "github.com/birdtable/go-files/trust"

	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
)

// subject is how the trust walk names a tool, and the line that moves it
// and points the setting there.
func subject() hosttrust.Subject {
	return hosttrust.Subject{Name: "the tool", File: "the tool", Last: hosttrust.Program, Point: "point the setting at it there"}
}

// trusted resolves the tool at p, a link or not, to the file it ends at,
// and answers that file when the trust walk passes every name on the way
// (trust-walk; go-files trust.Path). Nothing at p matches
// fs.ErrNotExist. The tool is given no group's members but macOS's admin,
// so a folder another group can write is refused, as any name it can write.
func (t *Tool) trusted(p string) (string, error) { return trust(subject(), p, t.cfg.Mounts) }

// trust is the walk of the program at p as s names it, with its refusals
// the adapter's own. A name that is not there answers the walk's error,
// which matches fs.ErrNotExist, so the next place is tried; a name the walk
// cannot read refuses the tool, so nothing runs unchecked.
//
// The adapter holds a path in the slash form of the model, and
// the walk takes the system's own: p is turned to it here, the one place a
// stored path becomes a program path to check, and the target answered is
// turned back, so every caller of a found path holds the slash form.
// mounts are the Config's Mounts, the walk's Who.Mounts.
func trust(s hosttrust.Subject, p string, mounts []string) (string, error) {
	target, err := hosttrust.Walk(s, filepath.FromSlash(p), hosttrust.Who{Mounts: mounts})
	if err != nil {
		return "", walkRefusal(err)
	}
	return filepath.ToSlash(target), nil
}

// walkRefusal is what the walk's err comes to: a name it does not trust or
// cannot read refuses the tool in the adapter's words, with the fix, and any
// other error, a name that is not there among them, is the walk's own. It
// reads nothing of the system, so a test plants the walk's answers to hold
// it on every system (walk_test.go), where go-files' own tests hold which
// names the walk trusts.
func walkRefusal(err error) error {
	if u, ok := errors.AsType[*hosttrust.Unsafe](err); ok {
		return judged(u)
	}
	if f, ok := errors.AsType[*hosttrust.Failed](err); ok {
		return refused(f.Error(), "point the setting at a program in a folder this account can read")
	}
	return err
}

// judged is the refusal of a name the walk does not trust, with the line
// that fixes it, or the one that points the setting elsewhere.
func judged(u *hosttrust.Unsafe) error {
	fix := u.Fix
	if fix == "" {
		fix = "point the setting at the program itself"
	}
	return refused(u.Observed, fix)
}

// maxShebang is the most bytes of a program's first line that are read.
const maxShebang = 256

// interpreterOf is the subject of the program a script starts, and the
// line that moves it.
func interpreterOf() hosttrust.Subject {
	return hosttrust.Subject{Name: "the interpreter", File: "the interpreter", Last: hosttrust.Program,
		Point: "put the interpreter there, or point the setting at a program that needs none"}
}

// maxHops is how many interpreters deep a script is followed: a shim that
// starts a shim that starts a program.
const maxHops = 4

// interpreter holds the program a found script starts to the same walk the
// script passed, and the program that one starts in turn, to maxHops. A
// script's first line names it: an absolute path, or /usr/bin/env and a name,
// which a run finds on the PATH it is given, the first folder that holds the
// name. A file with no such line is a program that needs none. A line the
// walk cannot follow, a relative interpreter, an option of env, a name on no
// folder of the PATH, a PATH entry that is empty or relative (env reads it
// from the folder the run is in), or a chain past maxHops, refuses the tool by
// name: what runs is never a program nobody judged. in is the PATH the find's
// runs are given, and for a pinned find the root: a folder env would take the
// name from that lies at or below it refuses the tool (envOutside), though
// the pinned PATH leaves every such folder out.
func (t *Tool) interpreter(target string, in within) error { return t.hop(target, in, 0) }

func (t *Tool) hop(target string, in within, depth int) error {
	// Whether the system reads a #! line, and what chain of programs it starts,
	// is go-files' to say: no chain is a program that needs none, and
	// every system that reads none answers none.
	chain, err := gotrust.Interpreters(filepath.FromSlash(target))
	if err != nil {
		return refused("the tool at "+hosttrust.Quote(target)+" starts a line the system would not run: "+err.Error(), "point the setting at the program itself")
	}
	if len(chain) == 0 {
		return nil
	}
	if depth+len(chain) > maxHops {
		return refused("the tool at "+hosttrust.Quote(target)+" starts a chain of interpreters more than "+strconv.Itoa(maxHops)+" deep", "point the setting at the program itself")
	}
	if err := chainHeld(target, chain, t.cfg.Mounts); err != nil {
		return err
	}
	if path.Base(filepath.ToSlash(chain[len(chain)-1])) != "env" {
		return nil
	}
	return t.envTakes(target, chain, in, depth)
}

// chainHeld judges each program the system starts for the script at target
// as the script was, and refuses an env that is not the last of the chain.
func chainHeld(target string, chain []string, mounts []string) error {
	for i, p := range chain {
		// A program that is not the last of the chain is a script, and an env that
		// is one reads a line of its own: refuse it, failing closed.
		if i < len(chain)-1 && path.Base(filepath.ToSlash(p)) == "env" {
			return refused("the tool at "+hosttrust.Quote(target)+" starts an env at "+hosttrust.Quote(p)+" that is itself a script", "point the setting at the program itself")
		}
		if _, err := trust(interpreterOf(), filepath.ToSlash(p), mounts); err != nil {
			if errors.Is(err, fs.ErrNotExist) { // the chain named it, and it is not there now
				return refused("the interpreter at "+hosttrust.Quote(p)+" is not there: "+err.Error(), "point the setting at the program itself")
			}
			return err
		}
	}
	return nil
}

// envTakes follows the program env takes from the PATH of the run, when env
// is the last of the chain at target: the line that names env is the last
// script of the chain's, which is the target when it is the only one.
func (t *Tool) envTakes(target string, chain []string, in within, depth int) error {
	holder := target
	if len(chain) > 1 {
		holder = filepath.ToSlash(chain[len(chain)-2])
	}
	line, err := firstLine(filepath.FromSlash(holder))
	if err != nil {
		return refused("the tool at "+hosttrust.Quote(holder)+" could not be read for its interpreter: "+err.Error(), "point the setting at the program itself")
	}
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) != 2 || strings.HasPrefix(fields[1], "-") || strings.Contains(fields[1], "/") {
		return refused("the tool at "+hosttrust.Quote(target)+" starts "+hosttrust.Quote(line)+", and only env and one program name are read", "point the setting at the program itself")
	}
	for _, dir := range filepath.SplitList(in.path) {
		if err := pathFolderHeld(dir); err != nil {
			return err
		}
		// The program env takes is the chain's next, in env's own place: it is
		// followed at the depth of the programs before env, those of this chain
		// counted, so the total of every level stays within maxHops.
		err := t.follow(path.Join(path.Clean(dir), fields[1]), in, depth+len(chain)-1)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		return t.envOutside(target, line, path.Clean(dir), in.root)
	}
	return refused("the tool at "+hosttrust.Quote(target)+" starts "+hosttrust.Quote(line)+", and "+fields[1]+" is in no folder of the PATH a run is given",
		"put "+fields[1]+" in a folder of the PATH, or point the setting at the program itself")
}

// pathFolderHeld refuses a folder of the PATH a run is given that env would
// read from the folder the run is in, or reach through a link before a ..
func pathFolderHeld(dir string) error {
	if !filepath.IsAbs(dir) {
		return refused("the PATH a run is given holds "+hosttrust.Quote(dir)+", which is empty or relative, and env reads it from the folder the run is in",
			"give the run a PATH of absolute folders")
	}
	if slices.Contains(strings.Split(dir, "/"), "..") {
		return refused("the PATH a run is given holds "+hosttrust.Quote(dir)+", which has a .. segment, and env reaches it through any link before it",
			"give the run a PATH of absolute folders without ..")
	}
	return nil
}

// envOutside refuses the tool at target, whose first line is line, when
// root is set and the folder dir env takes its program from lies, links
// followed, at or below root.
func (t *Tool) envOutside(target, line, dir string, root model.Root) error {
	if root == "" {
		return nil
	}
	in, err := t.folderInRoot(dir, root, "the folder "+hosttrust.Quote(dir)+" env takes a program from")
	if err != nil {
		return err
	}
	if in {
		return refused("the tool at "+hosttrust.Quote(target)+" starts "+hosttrust.Quote(line)+", and env would take its program from "+hosttrust.Quote(dir)+", at or below the root "+hosttrust.Quote(string(root)),
			"take the project's folders off the PATH gragas runs with, or point the setting at the program itself")
	}
	return nil
}

// follow passes p to the walk as an interpreter, with its refusals the
// adapter's own, and then reads its first line in turn; a name that is not
// there matches fs.ErrNotExist.
func (t *Tool) follow(p string, in within, depth int) error {
	target, err := trust(interpreterOf(), p, t.cfg.Mounts)
	if err != nil {
		return err
	}
	err = t.hop(target, in, depth+1)
	if errors.Is(err, fs.ErrNotExist) { // the interpreter is there; one it starts is not
		return refused("the interpreter at "+hosttrust.Quote(target)+" starts a program that is not there: "+err.Error(), "point the setting at the program itself")
	}
	return err
}

// firstLine is the first line of the file at p, without its newline, at most
// maxShebang bytes of it. p is the walk's target, links resolved, and it is
// opened inside its own folder as a root, so a name changed to a link since
// the walk does not lead the read out of that folder (supply.scans.3, G304).
func firstLine(p string) (line string, err error) {
	dir, err := os.OpenRoot(filepath.Dir(p))
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, dir.Close()) }()
	f, err := dir.Open(filepath.Base(p))
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	buf := make([]byte, maxShebang)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	line, _, _ = strings.Cut(string(buf[:n]), "\n")
	return line, nil
}
