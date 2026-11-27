package audit

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas/internal/ordered"
)

// changeFile is the name of a change file: <issue>-<kind>, with the extension
// the tool writes.
var changeFile = regexp.MustCompile(`^[0-9]+-[a-z][a-z0-9-]*\.ya?ml$`)

// noticeFiles are the files each archive of a release must carry.
func noticeFiles() []string { return []string{"LICENSE", "NOTICE"} }

// readYAMLKeys holds a release tool's YAML config to its bindings. Three keys
// are known: archives (every archive carries LICENSE and NOTICE,
// supply.licences.6), changelog.disable (the tool's own changelog is not turned
// off, supply.release.5) and unreleasedDir (a change file is named
// <issue>-<kind>, supply.release.5). Any other key is a finding that the reader
// does not know it, so a binding is never passed over in silence.
//
// A tool is chosen per repository: with no config file there is nothing to
// hold, and no key is a finding. The file is the repository's own and is only
// read; the tool is never run.
func readYAMLKeys(_ context.Context, in Input, c config) (verdict, string) {
	if !c.Present {
		return func(Binding) string { return "" }, ""
	}
	v, err := ordered.ParseStrict(c.Data)
	doc, ok := v.(*ordered.Map)
	if err != nil {
		return nil, fmt.Sprintf("%s cannot be read as a mapping: %v", c.File, err)
	}
	if !ok {
		return nil, c.File + " is not a mapping"
	}
	return func(b Binding) string { return judgeYAMLKeys(in, doc, c, b) }, ""
}

// judgeYAMLKeys is the finding, "" for none, of one binding over the document.
func judgeYAMLKeys(in Input, doc *ordered.Map, c config, b Binding) string {
	switch b.Key {
	case "archives":
		return judgeArchives(doc, c.File)
	case "changelog.disable":
		return judgeChangelog(doc, c.File)
	case "unreleasedDir":
		return judgeChangeFiles(in.Repo, doc, c.File)
	}
	return "gragas does not read the key " + b.Key + " of " + c.File
}

// judgeArchives is the finding of the first archive that does not list every
// notice file. A tool's default archive carries no NOTICE, so a config with no
// archives is a finding too. An archive of the bare binary bundles nothing.
func judgeArchives(doc *ordered.Map, file string) string {
	archives := ordered.Maps(doc.List("archives"))
	if len(archives) == 0 {
		return file + " has no archives, and the default archive carries no NOTICE"
	}
	for i, a := range archives {
		if onlyBinary(a) {
			continue
		}
		have := filesOf(a)
		for _, want := range noticeFiles() {
			if !slices.Contains(have, want) {
				return fmt.Sprintf("archive %d of %s does not list %s in files", i+1, file, want)
			}
		}
	}
	return ""
}

// onlyBinary is whether the archive is made of the bare binary alone.
func onlyBinary(a *ordered.Map) bool {
	f := ordered.Strings(a.List("formats"))
	return len(f) > 0 && !slices.ContainsFunc(f, func(s string) bool { return s != "binary" })
}

// filesOf is the names an archive lists in files: a plain name, or the src of a
// mapping.
func filesOf(a *ordered.Map) []string {
	var out []string
	for _, f := range a.List("files") {
		switch v := f.(type) {
		case string:
			out = append(out, v)
		case *ordered.Map:
			out = append(out, v.Str("src"))
		}
	}
	return out
}

// judgeChangelog is the finding when the config turns the tool's changelog
// off, which drops the entry a person wrote beside it. A templated value is
// the tool's to resolve, so only a plain true is a finding: the boolean, or
// the string "true", which the tool renders to the same.
func judgeChangelog(doc *ordered.Map, file string) string {
	cl := doc.Sub("changelog")
	if cl == nil {
		return ""
	}
	off := false
	switch v := cl.Val("disable").(type) {
	case bool:
		off = v
	case string:
		off = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	if off {
		return file + " sets changelog.disable: the entry a person wrote would be dropped"
	}
	return ""
}

// judgeChangeFiles is the finding of the first change file of the repository
// not named <issue>-<kind>. The folder is the config's changesDir and
// unreleasedDir, each a path inside the repository written with slashes: one
// that is not is not read, and a folder that is not there holds no change file.
func judgeChangeFiles(repo fs.FS, doc *ordered.Map, file string) string {
	dir := path.Join(dirOf(doc, "changesDir", ".changes"), dirOf(doc, "unreleasedDir", "unreleased"))
	// A backslash is a separator on one system and a name's character on
	// another, so a folder written with one is not read, as extend.go does not
	// read an extend path written with one.
	if strings.Contains(dir, `\`) || !fs.ValidPath(dir) || dir == "." || repo == nil {
		return ""
	}
	entries, err := fs.ReadDir(repo, dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || strings.HasPrefix(n, ".") {
			continue
		}
		if !changeFile.MatchString(n) {
			return fmt.Sprintf("the change file %s is not named <issue>-<kind>", path.Join(dir, n))
		}
	}
	return ""
}

// dirOf is a folder setting of the config, or its default.
func dirOf(doc *ordered.Map, key, def string) string {
	if s := doc.Str(key); s != "" {
		return s
	}
	return def
}
