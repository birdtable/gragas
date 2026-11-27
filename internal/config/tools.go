package config

// The per-machine tools file, <config>/gragas/tools.yaml or the file the
// settings' tools key names: one YAML document a person writes, saying where
// each tool version and sandbox runtime lives on this machine. host/fs
// reads it after it checked who may write it, and Load parses it here.
// The tool adapter tries it last, after the settings and the adapter's paths.

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/birdtable/gragas/internal/ordered"
)

// ToolsVersion is the tools file's version, which this build reads. A file
// with a newer one ends the run not ready, naming both; an older one, or
// none, is refused (store.schemas.2). It goes up when a key's meaning
// changes.
const ToolsVersion = 1

// Machine is what the tools file says. The zero Machine is no file.
type Machine struct {
	Version int
	// Tools is each tool@version to the absolute path of its program.
	Tools map[string]string
	// Container is the container runtime and the image it runs.
	Container Container
	// Sandbox is the sandbox programs, by the sandbox they serve.
	Sandbox SandboxPrograms
}

// Container is the container runtime's absolute path and the image it runs,
// named by its digest, "" where the file names none.
type Container struct {
	Runtime, Image string
}

// SandboxPrograms are the absolute paths of the user-namespace and the macOS
// sandbox programs, "" where the file names none.
type SandboxPrograms struct {
	Userns, Macos string
}

// ToolsKeys are the keys of the tools file, in the order the doc lists them.
// Each is refused at its file and line when it is not of its shape, and a
// key that is none of them is refused (cli.config.2).
func ToolsKeys() []Key {
	k := toolsKeys()
	out := make([]Key, len(k))
	for i, x := range k {
		out[i] = Key{Name: x.name, Default: "none", Holds: x.holds, Refused: x.refused}
	}
	return out
}

// toolsKey is one key of the tools file and its reader.
type toolsKey struct {
	name, holds, refused string
	read                 func(m *Machine, p place, v any) *Problem
}

func toolsKeys() []toolsKey {
	return []toolsKey{
		{"version", "the tools file's version, 1", "missing, not a whole number, or below 1; a newer one is not ready, naming both versions", readToolsVersion},
		{"tools", "a table: tool@version to the absolute path of that tool's program on this machine",
			"not a table, a name that is not tool@version, or a path that is not absolute and clean", readMachineTools},
		{"container", "a table: runtime, the container runtime's absolute path, and image, the image it runs as name@sha256: and its 64 hex digits",
			"not a table, a key that is not one of the two, a path that is not absolute and clean, or an image not named by its digest", readContainer},
		{"sandbox", "a table: the absolute path of the userns and the macos sandbox programs",
			"not a table, a key that is not one of the two, or a path that is not absolute and clean", readSandboxPrograms},
	}
}

// ParseTools reads the tools file's bytes. file is its path, for the
// refusals, which each name the file and the line. A key written twice is
// refused, as in the settings file.
func ParseTools(file string, data []byte) (Machine, error) {
	v, err := ordered.ParseYAML(data)
	if err != nil {
		return Machine{}, at(file, 0, "is not YAML: %v", err)
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return Machine{}, at(file, 0, "is not a table of tools: it must start with version: %d", ToolsVersion)
	}
	if _, ok := doc.Get("version"); !ok {
		return Machine{}, at(file, doc.Line, "has no version: write version: %d first", ToolsVersion)
	}
	known := toolsKeys()
	var m Machine
	// version first, so a newer file is not ready before any key of it is
	// judged by this build's rules.
	order := append([]string{"version"}, slices.DeleteFunc(slices.Clone(doc.Keys()), func(k string) bool { return k == "version" })...)
	for _, k := range order {
		p := place{file: file, line: doc.KeyLine(k), key: k}
		i := slices.IndexFunc(known, func(x toolsKey) bool { return x.name == k })
		if i < 0 {
			return Machine{}, unknown(p, toolsNames(known))
		}
		val, _ := doc.Get(k)
		if prob := known[i].read(&m, p, val); prob != nil {
			return Machine{}, prob
		}
		if k != "version" {
			continue
		}
		if prob := repeated(file, doc, ""); prob != nil {
			return Machine{}, prob
		}
	}
	return m, nil
}

func toolsNames(k []toolsKey) []string {
	out := make([]string, len(k))
	for i, x := range k {
		out[i] = x.name
	}
	return out
}

func readToolsVersion(m *Machine, p place, v any) *Problem {
	n, prob := p.version(v, ToolsVersion, "tools file", "the tools file")
	if prob == nil {
		m.Version = n
	}
	return prob
}

func readMachineTools(m *Machine, p place, v any) *Problem {
	tools, prob := table(p, v)
	if prob != nil {
		return prob
	}
	out := map[string]string{}
	for _, tool := range tools.Keys() {
		q := within(p, tools, tool)
		if !toolAt(tool) {
			return q.refuse("is not tool@version")
		}
		val, _ := tools.Get(tool)
		str, prob := pathAt(q, val)
		if prob != nil {
			return prob
		}
		out[tool] = filepath.ToSlash(str)
	}
	m.Tools = out
	return nil
}

func readContainer(m *Machine, p place, v any) *Problem {
	t, prob := table(p, v)
	if prob != nil {
		return prob
	}
	for _, k := range t.Keys() {
		q := within(p, t, k)
		val, _ := t.Get(k)
		switch k {
		case "runtime":
			str, prob := pathAt(q, val)
			if prob != nil {
				return prob
			}
			m.Container.Runtime = str
		case "image":
			str, ok := val.(string)
			if !ok || !byDigest(str) {
				return q.refuse("must be an image named by its digest: a name that starts with a letter or digit and holds no space, control character or @, then @sha256: and 64 lower-case hex digits, and it is %s", ordered.Repr(val))
			}
			m.Container.Image = str
		default:
			return unknown(q, []string{"runtime", "image"})
		}
	}
	return nil
}

func readSandboxPrograms(m *Machine, p place, v any) *Problem {
	t, prob := table(p, v)
	if prob != nil {
		return prob
	}
	fields := map[string]*string{"userns": &m.Sandbox.Userns, "macos": &m.Sandbox.Macos}
	for _, k := range t.Keys() {
		q := within(p, t, k)
		field, ok := fields[k]
		if !ok {
			return unknown(q, []string{"userns", "macos"})
		}
		val, _ := t.Get(k)
		str, prob := pathAt(q, val)
		if prob != nil {
			return prob
		}
		*field = str
	}
	return nil
}

// byDigest is whether s names an image by its digest: a name that starts
// with a letter or digit and holds no space, control character or @, then
// @sha256: and 64 lower-case hex digits. A tag alone could name other bytes
// tomorrow, so it is refused. The image is an argument of the container
// runtime, so a name that starts with "-" would be an option, and one with a
// space or a second @ is not one name.
func byDigest(s string) bool {
	name, digest, ok := strings.Cut(s, "@sha256:")
	if !ok || name == "" || !isAlnum(rune(name[0])) || len(digest) != 64 {
		return false
	}
	if strings.IndexFunc(name, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r == '@' }) >= 0 {
		return false
	}
	return strings.Trim(digest, "0123456789abcdef") == ""
}
