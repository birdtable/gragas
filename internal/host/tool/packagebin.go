package tool

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/go-proc/proc"
	hosttrust "github.com/birdtable/gragas/internal/host/trust"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/ordered"
)

// maxPackageJSON is the most bytes of a package.json, or of a script grant's
// manifest, that are read.
const maxPackageJSON = 1 << 20

// binFolder is the folder a package manager fills with shims and links: never
// the script that runs. A script grant's manifest is refused in
// any spelling of it, by model.SameName, as the opener refuses it.
const binFolder = ".bin"

// packageSubject is how the trust walk names a file of a package.
func packageSubject(last hosttrust.Kind) hosttrust.Subject {
	return hosttrust.Subject{Name: "the package", File: "the package", Last: last, Point: "reinstall the package where this account owns it"}
}

// interpreterSubject is how the trust walk names the program a script runs as.
func interpreterSubject(name string) hosttrust.Subject {
	return hosttrust.Subject{Name: name, File: name, Last: hosttrust.Program, Point: "put " + name + " in a folder only this account and root can write"}
}

// manifestScript is the script a script grant names: the
// string at its script_at, a JSON pointer, in the manifest at its manifest,
// relative to the project, read below the manifest's folder as scriptBelow
// holds it, by its spelling and through links. A manifest whose path passes
// through a .bin folder, in any spelling model.SameName reads as one, is
// refused before anything is read, as the opener refuses it. A manifest
// folder that is not there matches fs.ErrNotExist, so the next place is
// tried; a manifest that is no document, a pointer at nothing and a value that
// is no string are refused by name.
func manifestScript(root model.Root, s *plugin.ProgramScript, mounts []string) (string, error) {
	what := "the script grant " + hosttrust.Quote(s.ID) + "'s manifest " + hosttrust.Quote(s.Manifest)
	at := path.Join(string(root), s.Manifest)
	if relFault(s.Manifest) || !below(string(root), at) {
		return "", refused(what+" is empty, not a relative slash path or outside the project", "name the manifest by a path below the project")
	}
	for step := range strings.SplitSeq(s.Manifest, "/") {
		if model.SameName(step, binFolder) {
			return "", refused(what+" passes through "+hosttrust.Quote(step)+", a .bin folder, which holds shims and links and never the script that runs",
				"name the package's own manifest, not one below a .bin folder")
		}
	}
	dir, err := trust(packageSubject(hosttrust.Folder), path.Dir(at), mounts)
	if err != nil {
		return "", err
	}
	b, err := manifestBytes(dir, path.Base(at))
	if err != nil {
		return "", err
	}
	doc, err := ordered.ParseStrict(b)
	if err != nil {
		return "", refused(what+" is not a document: "+err.Error(), "reinstall the package")
	}
	v, ok := ordered.Pointer(doc, s.ScriptAt)
	if !ok {
		return "", refused(what+" holds nothing at its script_at "+hosttrust.Quote(s.ScriptAt), "point script_at at the manifest's script")
	}
	rel, ok := v.(string)
	if !ok {
		return "", refused(what+" holds no string at its script_at "+hosttrust.Quote(s.ScriptAt), "point script_at at a script's path")
	}
	return scriptBelow(dir, rel, what+" names the script "+hosttrust.Quote(rel)+" at "+hosttrust.Quote(s.ScriptAt), "the manifest's folder", mounts)
}

// relFault is whether rel is no relative slash path: empty, absolute, with a
// backslash or a NUL, or, unless it climbs, not local on this system once
// cleaned. A climb out of the folder is the caller's to refuse, by its own
// words, so ./NUL and a/../C:x are asked as NUL and C:x.
func relFault(rel string) bool {
	clean := path.Clean(rel)
	climbs := clean == ".." || strings.HasPrefix(clean, "../")
	return rel == "" || strings.ContainsAny(rel, "\\\x00") || path.IsAbs(rel) || (!climbs && !filepath.IsLocal(clean))
}

// scriptBelow is the script rel names below the folder dir, the trust walk's
// target: refused when rel is no relative slash path, when it lies outside
// dir by its spelling, when nothing is there, and when it is a link whose
// target lies outside dir. named says where rel was read, and folder what dir
// is, in a refusal.
func scriptBelow(dir, rel, named, folder string, mounts []string) (string, error) {
	if relFault(rel) {
		return "", refused(named+", which is empty or not a relative slash path", "name a script below "+folder)
	}
	script := path.Join(dir, rel)
	if !below(dir, script) {
		return "", refused(named+", which lies outside "+folder, "name a script below "+folder)
	}
	target, err := trust(packageSubject(hosttrust.File), script, mounts)
	if errors.Is(err, fs.ErrNotExist) {
		return "", refused(named+", and nothing is there", "reinstall the package")
	}
	if err != nil {
		return "", err
	}
	if !below(dir, target) {
		return "", refused(named+", and it is a link to "+hosttrust.Quote(target)+", outside "+folder, "name a script below "+folder+", not a link out of it")
	}
	return target, nil
}

// below is whether p lies below dir.
func below(dir, p string) bool {
	return strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}

// manifestBytes reads the file name of the folder dir, which is the walk's
// target, opened inside dir as a root, so a name that became a link since the
// walk does not lead the read out of it; at most maxPackageJSON bytes, and
// one more refuses it. A file that is not there matches fs.ErrNotExist.
func manifestBytes(dir, name string) (b []byte, err error) {
	root, err := os.OpenRoot(filepath.FromSlash(dir))
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	what := "the " + name + " of " + hosttrust.Quote(dir)
	b, err = confine.ReadCapped(root, name, maxPackageJSON)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, err
	case errors.Is(err, confine.ErrOverCap):
		return nil, refused(what+" is over "+itoa(maxPackageJSON)+" bytes", "reinstall the package")
	case err != nil:
		return nil, refused(what+" cannot be read: "+err.Error(), "reinstall the package")
	}
	return b, nil
}

// findInterpreter is the program name a script runs as, found the way any
// program is: each folder of the PATH a run is given, in order, that must be
// absolute with no .. segment, through the trust walk, and never a bare
// search. A name that is not one plain file name is refused; a
// name in no folder of the PATH matches fs.ErrNotExist.
func (t *Tool) findInterpreter(name string, in within) (string, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return "", refused("the program a script runs as is named "+hosttrust.Quote(name)+", which is no plain file name", "name the program by its name alone, as the PATH finds it")
	}
	for _, dir := range filepath.SplitList(in.path) {
		if !filepath.IsAbs(dir) {
			return "", refused("the PATH a run is given holds "+hosttrust.Quote(dir)+", which is empty or relative, and "+name+" would be read from the folder the run is in", "give the run a PATH of absolute folders")
		}
		target, err := t.interpreterIn(dir, name, in)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return target, err
	}
	return "", fs.ErrNotExist
}

// interpreterIn is the program name in the one folder dir of the PATH, judged
// as findInterpreter says; fs.ErrNotExist when the folder holds none.
func (t *Tool) interpreterIn(dir, name string, in within) (string, error) {
	slash := filepath.ToSlash(dir)
	if slices.Contains(strings.Split(slash, "/"), "..") {
		return "", refused("the PATH a run is given holds "+hosttrust.Quote(dir)+", which has a .. segment", "give the run a PATH of absolute folders without ..")
	}
	// the program's file name on this system, as go-proc spells it.
	file := proc.LocalFileName(name)
	target, err := trust(interpreterSubject(name), path.Join(path.Clean(slash), file), t.cfg.Mounts)
	if err != nil {
		return "", err
	}
	if err := t.interpreter(target, in); err != nil {
		return "", err
	}
	if in.root != "" {
		if err := t.envOutside(target, file, path.Dir(target), in.root); err != nil {
			return "", err
		}
	}
	return target, nil
}

// resolve is the program a place names, passed through the trust walk, and the
// script it runs when the place names one: for a script grant, the grant's
// interpreter is the program and the script is what its manifest names at
// script_at. The program is found on the PATH a run is given. A place that
// is not there matches fs.ErrNotExist; a script that is there with no program
// to run it is refused by name, so it is never skipped unseen.
func (t *Tool) resolve(root model.Root, p place, in within) (target, script string, err error) {
	var program, what string
	switch {
	case p.script != nil:
		script, err = manifestScript(root, p.script, t.cfg.Mounts)
		program, what = p.script.Interpreter, "the script grant "+hosttrust.Quote(p.script.ID)
	default:
		target, err = t.trusted(p.path)
		return target, "", err
	}
	if err != nil {
		return "", "", err
	}
	target, err = t.findInterpreter(program, in)
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", refused(what+" is a script that runs as "+program+", and "+program+" is in no folder of the PATH a run is given", "put "+program+" in a folder of the PATH")
	}
	return target, script, err
}
