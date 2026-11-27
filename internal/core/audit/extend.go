package audit

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

// maxExtendDepth is how many extended files a config may reach through
// [extend] path, one from the next. gitleaks 8.30.1 follows two (config.go:
// maxExtendDepth) and ignores the rest without a word; a chain that is longer
// is not read here either, and is reported.
const maxExtendDepth = 4

// tomlFile is one document of a config's chain: the config's own file, then
// each file it extends, by the name it is read at.
type tomlFile struct {
	name string
	doc  map[string]any
}

// parseTOMLFile reads data as one document of a config: parsed, folded to the
// case the tool reads keys in, and held to the shape of its allowlists. The
// reason is "" when it is read.
func parseTOMLFile(in Input, name string, data []byte) (map[string]any, string) {
	doc, err := in.TOML(data)
	if err != nil {
		return nil, fmt.Sprintf("%s cannot be read: %v", name, err)
	}
	// The tool ignores the case of a key, so the document is read with every
	// key lower-cased; two keys that fold to one are not checked.
	if doc, err = foldTOML(doc); err != nil {
		return nil, fmt.Sprintf("%s: %v", name, err)
	}
	if err = allowlistShape(doc); err != nil {
		return nil, fmt.Sprintf("%s: %v", name, err)
	}
	return doc, ""
}

// extendedFiles reads the files the config extends, in the order the tool
// reaches them, each from the repository and nowhere else. gitleaks 8.30.1
// reads [extend] path as a file whose rules and allowlists join the config's
// own (config.go: extendPath, extend), so what each allows is what the config
// allows. A path that leaves the repository, a URL, a chain that loops or runs
// past maxExtendDepth, a file that cannot be read, or a config that sets both
// path and useDefault (which the tool refuses, config.go: Translate) is the
// reason the config is not checked. The path is read from the repository's
// root, where the tool is run.
func extendedFiles(in Input, top tomlFile) ([]tomlFile, string) {
	var out []tomlFile
	seen := map[string]bool{top.name: true}
	chain := []string{top.name}
	cur := top
	for depth := 1; ; depth++ {
		p, has, why := extendPathOf(cur)
		if why != "" {
			return nil, why
		}
		if !has {
			return out, ""
		}
		rel, why := insideRepository(cur.name, p)
		if why != "" {
			return nil, why
		}
		if depth > maxExtendDepth {
			return nil, fmt.Sprintf("%s: the chain of [extend] path (%s) is longer than %d files, so what it allows is not checked", top.name, strings.Join(chain, " -> "), maxExtendDepth)
		}
		if seen[rel] {
			return nil, fmt.Sprintf("%s: [extend] path loops (%s -> %s), so what it allows is not checked", top.name, strings.Join(chain, " -> "), rel)
		}
		seen[rel] = true
		chain = append(chain, rel)
		data, err := fs.ReadFile(in.Repo, rel)
		if err != nil {
			return nil, fmt.Sprintf("%s: [extend] path = %q cannot be read from inside the repository (%v), so what it allows is not checked", cur.name, p, err)
		}
		doc, why := parseTOMLFile(in, rel, data)
		if why != "" {
			return nil, why
		}
		cur = tomlFile{rel, doc}
		out = append(out, cur)
	}
}

// extendPathOf is the [extend] path of a document: "" and false when there is
// none. A path that is no string, or that is set beside useDefault, is the
// reason the config is not read.
func extendPathOf(f tomlFile) (string, bool, string) {
	ext, ok := f.doc["extend"].(map[string]any)
	if !ok {
		return "", false, ""
	}
	v, has := ext["path"]
	if !has {
		return "", false, ""
	}
	p, isString := v.(string)
	if !isString {
		return "", false, fmt.Sprintf("%s: [extend] path is not a string, and the tool's read of it is not known", f.name)
	}
	if p == "" { // the tool takes an empty path for none (config.go: Translate)
		return "", false, ""
	}
	if ext["usedefault"] == true {
		return "", false, fmt.Sprintf("%s sets [extend] path and [extend] useDefault together, which the tool refuses to load, so it is not checked", f.name)
	}
	return p, true, ""
}

// scheme is a URL's start, or a volume's: a name and a colon before any slash.
var scheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*:`)

// insideRepository is the path as the repository names it, or the reason it is
// not read: a URL, a volume, a backslash, an absolute path or a step up with
// "..". The repository's own reader refuses a link that leaves it besides.
func insideRepository(from, p string) (string, string) {
	no := func(what string) (string, string) {
		return "", fmt.Sprintf("%s: [extend] path = %q is %s, and only a file inside the repository is read, so what it allows is not checked", from, p, what)
	}
	switch {
	case scheme.MatchString(p):
		return no("a URL or names a volume")
	case strings.Contains(p, `\`):
		return no("written with a backslash")
	case strings.HasPrefix(p, "/"):
		return no("an absolute path")
	case strings.HasSuffix(p, "/"):
		return no("written with a trailing slash, which the tool does not load as a file")
	}
	if slices.Contains(strings.Split(p, "/"), "..") {
		return no("a path that leaves the repository")
	}
	clean := path.Clean(p)
	if clean == "." || !fs.ValidPath(clean) {
		return no("no file of the repository")
	}
	return clean, ""
}
