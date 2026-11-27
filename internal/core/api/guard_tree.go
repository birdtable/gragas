package coreapi

import (
	"errors"
	"io/fs"
	"path"
	"strconv"
	"strings"

	"github.com/birdtable/gragas/internal/core/format"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// current is the file at rel as it is, whether it exists, and why it may not
// be judged, with the line to type when that refusal has one of its own ("" is
// the caller's, to edit the file by the name it has). Every part of rel, each folder and the file, is held to the
// names the repository and the loader already have: the listing from the
// root, the one listing that carries no spelling of rel's, and every path the
// loader asked for (asked), there or not. A part that folds to one of those
// and is spelled otherwise is refused, whether or not the host finds it, on
// every host, since a host that folds case writes it to the file the guard
// did not read. A part the listing
// does not hold is a new name, and must be one no host reads as another
// (refusedAsNew); a folder of that kind the host still finds is refused too.
// Each part of written, rel as the edit spells it, is held to the same names
// and new-name rules first, so the reason is one on every host, one whose
// Within answers the disk's spelling, an 8.3 short name expanded, included.
//
// The file is read by ReadSoleWithin, so a file with more than one name, by hard
// links, is refused: an edit to it changes it under every name it has, and
// the guard judged it by one. Its names are counted on the
// handle the content is read through, so the bytes judged are those of the
// file counted. A second name made after the read is not seen: the guard
// holds no handle past its judgment, and the host's tool opens the file again
// to write it.
func current(tree port.Tree, file port.FileNames, root model.Root, rel, written string, asked map[string]bool) (data []byte, exists bool, why, hint string) {
	data, err := tree.ReadSoleWithin(root, rel, MaxHookInput)
	exists = err == nil
	if errors.Is(err, port.ErrManyNames) {
		return nil, false, rel + " has more than one name, by hard links, so an edit to it changes a file the guard judged by one name only",
			"give the file one name: remove its other names, or copy it to a new file and edit that"
	}
	if errors.Is(err, pluginbundle.ErrOverBound) {
		return nil, false, rel + " is over the " + strconv.Itoa(MaxHookInput) + " bytes (16 MiB) the guard reads, so an edit to it cannot be judged",
			"edit a file of at most 16 MiB: split " + rel + " into smaller files"
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, false, "the file could not be read: " + err.Error(), ""
	}
	n, err := namesOf(tree, file, root, asked)
	if err != nil {
		return nil, false, "the repository could not be listed: " + err.Error(), ""
	}
	for _, part := range parts(written) {
		if why := n.named(written, part); why != "" {
			return nil, false, why, ""
		}
	}
	for _, part := range parts(rel) {
		if why := n.judge(tree, root, rel, part); why != "" {
			return nil, false, why, ""
		}
	}
	if !exists {
		return nil, false, "", ""
	}
	if !n.listed[rel] {
		return nil, false, rel + " names a file whose own name is spelled otherwise", ""
	}
	return data, true, "", ""
}

// parts is rel and each of its folders, shortest first; none for "".
func parts(rel string) []string {
	var out []string
	for i := 1; i <= len(rel); i++ {
		if i == len(rel) || rel[i] == '/' {
			out = append(out, rel[:i])
		}
	}
	return out
}

// kindOf is what part of rel names: the file, or one of its folders.
func kindOf(part, rel string) string {
	if part == rel {
		return "file"
	}
	return "folder"
}

// names are the names a part of rel is held to: the paths the listing from
// the root holds, and every spelling of them and of the loader's paths and
// their folders, by the key they fold to.
type names struct {
	file   port.FileNames
	listed map[string]bool
	forms  map[string]map[string]bool
}

func namesOf(tree port.Tree, file port.FileNames, root model.Root, asked map[string]bool) (names, error) {
	entries, err := tree.List(root, "", model.Every)
	if err != nil {
		return names{}, err
	}
	n := names{file: file, listed: map[string]bool{}, forms: map[string]map[string]bool{}}
	for _, e := range entries {
		n.listed[e.Path] = true
		n.know(e.Path)
	}
	for p := range asked {
		for ; p != "." && p != "" && p != "/"; p = path.Dir(p) {
			n.know(p)
		}
	}
	return n, nil
}

func (n names) know(p string) {
	k, _ := n.key(p)
	if n.forms[k] == nil {
		n.forms[k] = map[string]bool{}
	}
	n.forms[k][p] = true
}

// key is p as a host that folds case or a normal form finds it: each name of
// it through the FileNames port, joined by "/", so letters that fold together
// meet. A name the port refuses stands for itself, and the names
// beside it are still folded, so a twin in another case below a folder the port
// refuses (aux, v1.) is still one the guard sees; why is the first refusal.
func (n names) key(p string) (key, why string) {
	keys := make([]string, 0, 4)
	for name := range strings.SplitSeq(p, "/") {
		k, err := n.file.Portable(name)
		if err != nil {
			if why == "" {
				why = err.Error()
			}
			k = "\x00" + name
		}
		keys = append(keys, k)
	}
	return strings.Join(keys, "/"), why
}

// named is why part, written or one of its folders as the edit spells it,
// may not be judged by its name alone: spelled otherwise than a name it
// folds to, or a new name a host may read as another. The disk is not read:
// that is judge's, on the path the host resolved.
func (n names) named(written, part string) string {
	k, _ := n.key(part)
	if why := spelled(part, kindOf(part, written), n.forms[k]); why != "" || n.listed[part] {
		return why
	}
	if why := n.refusedAsNew(path.Base(part)); why != "" {
		return part + " is a new name a host may read as another: " + why
	}
	return ""
}

// judge is why part, rel or one of its folders, may not be judged: spelled
// otherwise than a name it folds to, a new name a host may read as another,
// or a new folder the host still finds by another name.
func (n names) judge(tree port.Tree, root model.Root, rel, part string) string {
	if why := n.named(rel, part); why != "" || n.listed[part] {
		return why
	}
	if part == rel {
		return ""
	}
	_, err := tree.Stat(root, part)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return ""
	case err != nil:
		return "the file's folder could not be read: " + err.Error()
	}
	return rel + " names a folder whose own name is spelled otherwise"
}

// spelled is why part may not be judged by the names forms, the ones of the
// repository and the loader that fold as it does: one spelled otherwise, or
// two, which a host that folds case reads as one.
func spelled(part, what string, forms map[string]bool) string {
	switch {
	case len(forms) == 0, len(forms) == 1 && forms[part]:
		return ""
	case forms[part]:
		return part + " names a " + what + " that another name of the repository folds to, so a host that folds case reads both as one"
	}
	return part + " names a " + what + " whose own name is spelled otherwise"
}

// refusedAsNew is why a part of a path the repository does not have may name
// another file on some host, "" when it names only itself: the FileNames port
// says it for every host, in go-files' words, which nothing matches on.
// The guard keeps one rule of its own, a product choice and no
// host's: a new name holds only printable ASCII. A ~ and a digit is go-files'
// to judge, as an 8.3 short name and nothing broader.
func (n names) refusedAsNew(part string) string {
	for i := range len(part) {
		if c := part[i]; c < 0x20 || c >= 0x7f {
			return "it holds a byte that is not printable ASCII"
		}
	}
	if _, err := n.file.Portable(part); err != nil {
		return err.Error()
	}
	return ""
}

// newFinding is the first finding of after that before did not have, by its
// file and sentence, not its line, which an edit above it moves: so an edit
// that breaks a file is refused for that, and not read as the rules it no
// longer holds.
func newFinding(before, after []format.Finding) (string, bool) {
	had := map[string]int{}
	for _, f := range before {
		had[f.File+"\x00"+f.Msg]++
	}
	for _, f := range after {
		if k := f.File + "\x00" + f.Msg; had[k] > 0 {
			had[k]--
			continue
		}
		return f.String(), true
	}
	return "", false
}
