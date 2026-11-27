package audit

import (
	"context"
	"errors"
	"fmt"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// isOSTag is whether a build term names an operating system: every GOOS that
// go build knows (go/build's syslist at the go go.mod pins), unix, and the
// wasm architecture, whose only systems are js and wasip1.
func isOSTag(t string) bool {
	switch t {
	case "aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "js", "linux", "nacl",
		"netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows", "zos", "unix", "wasm":
		return true
	}
	return false
}

// isArch is whether a file-name element is a GOARCH go build knows.
func isArch(t string) bool {
	switch t {
	case "386", "amd64", "amd64p32", "arm", "armbe", "arm64", "arm64be", "loong64", "mips", "mipsle", "mips64",
		"mips64le", "mips64p32", "mips64p32le", "ppc", "ppc64", "ppc64le", "riscv", "riscv64", "s390", "s390x",
		"sparc", "sparc64", "wasm":
		return true
	}
	return false
}

// nameNamesAnOS is the file-name half, read as go build reads a name
// (goodOSArchFile): cut at its first dot, past its first element and before
// _test, the last element is a GOOS (or wasm, or
// unix and other by convention), or the last two are a GOOS and a GOARCH. A
// GOARCH alone names no system.
func nameNamesAnOS(name string) bool {
	stem, _, _ := strings.Cut(name, ".")
	parts := strings.Split(stem, "_")
	if n := len(parts); n > 1 && parts[n-1] == "test" {
		parts = parts[:n-1]
	}
	parts = parts[min(1, len(parts)):]
	n := len(parts)
	if n >= 1 && (isOSTag(parts[n-1]) || parts[n-1] == "other") {
		return true
	}
	return n >= 2 && isOSTag(parts[n-2]) && isArch(parts[n-1])
}

// NamesAnOS reports whether a Go file of that name and text names an operating
// system: by its name, as go build reads one (nameNamesAnOS), or by a build
// line whose expression has an operating-system term, negated or not, among
// the line comments go/parser finds before the package clause, a block comment
// that holds the word package included. A word of the body, a feature tag and
// a build line after the clause name none. A file that does not parse as far
// as its package clause, or whose //go:build line go build refuses, is an
// error, never a pass.
func NamesAnOS(name string, body []byte) (bool, error) {
	if nameNamesAnOS(name) {
		return true, nil
	}
	f, err := parser.ParseFile(token.NewFileSet(), name, body, parser.PackageClauseOnly|parser.ParseComments)
	if f == nil || !f.Package.IsValid() {
		if err == nil {
			err = errors.New("no package clause")
		}
		return false, fmt.Errorf("%s does not parse as far as its package clause: %w", name, err)
	}
	for _, g := range f.Comments {
		for _, c := range g.List {
			if c.End() > f.Package {
				continue
			}
			if yes, err := lineNamesAnOS(name, c.Text); yes || err != nil {
				return yes, err
			}
		}
	}
	return false, nil
}

// lineNamesAnOS is whether one comment before the package clause is a build
// line naming an operating system. A //go:build line go build refuses is an
// error; a +build line it refuses is skipped, as go build skips one.
func lineNamesAnOS(name, text string) (bool, error) {
	if !constraint.IsGoBuild(text) && !constraint.IsPlusBuild(text) {
		return false, nil
	}
	e, err := constraint.Parse(text)
	if err != nil {
		if constraint.IsGoBuild(text) {
			return false, fmt.Errorf("%s carries a //go:build line go build refuses: %w", name, err)
		}
		return false, nil
	}
	return namesTag(e), nil
}

// namesTag is whether a build expression has an operating-system term.
func namesTag(e constraint.Expr) bool {
	switch x := e.(type) {
	case *constraint.TagExpr:
		return isOSTag(x.Tag)
	case *constraint.NotExpr:
		return namesTag(x.X)
	case *constraint.AndExpr:
		return namesTag(x.X) || namesTag(x.Y)
	case *constraint.OrExpr:
		return namesTag(x.X) || namesTag(x.Y)
	}
	return false
}

// readOSFiles is the file-name and build-line half of arch.ports.24: the Go
// files of the repository outside its host layer that name an operating
// system, or that go/parser cannot read as far as their package clause, less
// those a live exemption row names by path. With no config, or no such row,
// the repository has no host layer and every such file is a finding.
func readOSFiles(_ context.Context, in Input, c config) (verdict, string) {
	layer, why := layerOf(c)
	if why != "" {
		return nil, why
	}
	named, unknown, err := osFiles(in.Repo, layer)
	if err != nil {
		return nil, "the repository's Go files could not be listed: " + err.Error()
	}
	return func(b Binding) string {
		if b.Key != KindOSFiles {
			return "the " + KindOSFiles + " reader holds no key " + b.Key
		}
		return osFinding(unwaived(in, b.Rule, named), unwaived(in, b.Rule, unknown), layer)
	}, ""
}

// unwaived is the files no live exemption row of the rule names by path.
func unwaived(in Input, rule string, files []string) []string {
	var out []string
	for _, f := range files {
		if in.ExemptAt == nil {
			out = append(out, f)
		} else if _, ok := in.ExemptAt(rule, f); !ok {
			out = append(out, f)
		}
	}
	return out
}

// layerOf is the host layer of the config: the folders its row at the
// Source's Host key spares, each written "!**/<folder>/**". A file the row
// spares alone is not the layer, so it needs its own row. Why says why the
// layer is not known; none when there is no config.
func layerOf(c config) ([]string, string) {
	if !c.Present {
		return nil, ""
	}
	if !configFormat(path.Ext(c.File)) {
		return nil, c.File + " is a format this check does not read, so the host layer is not known"
	}
	settings, why := configPaths(c.File, c.Data)
	if why != "" {
		return nil, why
	}
	return hostLayer(settings[strings.ToLower(c.Host)]), ""
}

// osFinding is what is missing when files are named outside the layer, or
// cannot be read far enough to know: none when there are none. A file that
// does not parse is a finding, never a pass and never a not-checked, which
// would leave every other file of the repository unread.
func osFinding(named, unknown, layer []string) string {
	var parts []string
	if len(named) > 0 {
		where := "the repository names no host layer"
		if len(layer) > 0 {
			where = "the host layer is " + strings.Join(layer, ", ")
		}
		parts = append(parts, strings.Join(named, ", ")+plural(named, " names", " name")+
			" an operating system ("+where+")")
	}
	if len(unknown) > 0 {
		parts = append(parts, strings.Join(unknown, ", ")+plural(unknown, " does", " do")+
			" not parse as far as the package clause, or carries a //go:build line go build refuses, so whether it names an operating system is not known")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ") + ": a product has no host layer, so reach what differs between systems through go-proc or go-files, or give each file a row of gate/exemptions.yaml"
}

// plural is one when list holds one item, else many.
func plural(list []string, one, many string) string {
	if len(list) == 1 {
		return one
	}
	return many
}

// hostLayer is the folders a files list spares, each "!**/<folder>/**".
func hostLayer(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, g := range list {
		s, isText := g.(string)
		if !isText {
			continue
		}
		if f, ok := strings.CutPrefix(s, "!**/"); ok {
			if f, ok = strings.CutSuffix(f, "/**"); ok && f != "" && !strings.Contains(f, "*") {
				out = append(out, f)
			}
		}
	}
	return out
}

// osFiles is each Go file of repo outside the folders of layer that names an
// operating system (named), and each that NamesAnOS cannot read far enough to
// know (unknown), sorted. Hidden folders, testdata, vendor and node_modules,
// and files whose names start with a dot or _, are not read, as go build
// reads none of them. A file that cannot be read at all ends the walk, and
// the not-checked it gives names that file alone.
func osFiles(repo fs.FS, layer []string) (named, unknown []string, err error) {
	err = fs.WalkDir(repo, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != "." && (skipped(d.Name()) || inLayer(p+"/", layer)) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		body, err := fs.ReadFile(repo, p)
		if err != nil {
			return err
		}
		switch yes, perr := NamesAnOS(d.Name(), body); {
		case perr != nil:
			unknown = append(unknown, p)
		case yes:
			named = append(named, p)
		}
		return nil
	})
	slices.Sort(named)
	slices.Sort(unknown)
	return named, unknown, err
}

// skipped is whether go build skips a folder of that name.
func skipped(n string) bool {
	return strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") || n == "testdata" || n == "vendor" || n == "node_modules"
}

// inLayer is whether the slash path p, a folder ending in a slash, is a
// folder of the layer at any depth, as the glob "**/<folder>/**" matches.
func inLayer(p string, layer []string) bool {
	for _, f := range layer {
		if strings.Contains("/"+p, "/"+f+"/") {
			return true
		}
	}
	return false
}
