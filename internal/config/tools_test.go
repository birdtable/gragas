package config_test

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/config"
)

// toolsFixtures is the folder of tools files the golden lists.
const toolsFixtures = "testdata/tools"

// rootToken stands for a test's own folder in a fixture, as testdata says.
const rootToken = "{root}"

// aValue is a tools fixture's path value: a scalar after "key: " that starts
// with the root token. A relative path, an image digest and a URL do not, so
// they are left alone.
var aValue = regexp.MustCompile(`(?m)(:[ \t]+)\{root\}(/\S+)`)

// placeholder stands for the test's folder in the golden.
const placeholder = "<dir>"

// underDir is the fixture with each absolute path value put under dir, and the
// replacer that writes the folder back as the placeholder in what the parser
// answers, so one golden holds wherever the test runs. A path with ".." in it
// keeps the ".." and what follows it as written, which Join would clean away.
func underDir(dir string, b []byte) ([]byte, *strings.Replacer) {
	var pairs []string
	out := aValue.ReplaceAllFunc(b, func(m []byte) []byte {
		g := aValue.FindSubmatch(m)
		v := string(g[2])
		segs := strings.Split(strings.TrimPrefix(v, "/"), "/")
		built := filepath.Join(append([]string{dir}, segs...)...)
		if i := slices.Index(segs, ".."); i >= 0 {
			built = filepath.Join(append([]string{dir}, segs[:i]...)...) + "/" + strings.Join(segs[i:], "/")
		}
		shown := placeholder + v
		pairs = append(pairs, built, shown, strconv.Quote(built), strconv.Quote(shown))
		// A tool path is held in the slash form, which differs from the native
		// one on Windows.
		if s := filepath.ToSlash(built); s != built {
			pairs = append(pairs, s, shown, strconv.Quote(s), strconv.Quote(shown))
		}
		if c := filepath.Clean(built); c != built {
			cleaned := placeholder + path.Clean(v)
			pairs = append(pairs, c, cleaned, strconv.Quote(c), strconv.Quote(cleaned))
			if s := filepath.ToSlash(c); s != c {
				pairs = append(pairs, s, cleaned, strconv.Quote(s), strconv.Quote(cleaned))
			}
		}
		return append(append([]byte(nil), g[1]...), built...)
	})
	return out, strings.NewReplacer(longestFirst(pairs)...)
}

// longestFirst orders the old/new pairs of a replacer so that a path that
// begins another is tried first.
func longestFirst(pairs []string) []string {
	type pair struct{ old, new string }
	var ps []pair
	for i := 0; i < len(pairs); i += 2 {
		ps = append(ps, pair{pairs[i], pairs[i+1]})
	}
	slices.SortStableFunc(ps, func(a, b pair) int { return len(b.old) - len(a.old) })
	var out []string
	for _, p := range ps {
		out = append(out, p.old, p.new)
	}
	return out
}

// read is the bytes of a fixture.
func read(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(toolsFixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestTheToolsFileIsReadAtEveryKey: a file that sets every key gives exactly
// what it says, and one with the version alone gives no tools.
func TestTheToolsFileIsReadAtEveryKey(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	lint15 := filepath.Join(dir, "opt", "golangci-lint", "2.15.0", "golangci-lint")
	lint14 := filepath.Join(dir, "opt", "golangci-lint", "2.14.1", "golangci-lint")
	eslint := filepath.Join(dir, "home", "me", "tools", "node_modules", ".bin", "eslint")
	podman := filepath.Join(dir, "usr", "bin", "podman")
	bwrap := filepath.Join(dir, "usr", "bin", "bwrap")
	sandboxExec := filepath.Join(dir, "usr", "bin", "sandbox-exec")
	// The file is written in the slash form a setting takes: the test's
	// folder in it is slashed too, or a system whose paths use \ reads a mixed path
	// that is neither form's clean one.
	text := strings.ReplaceAll(string(read(t, "full.yaml")), rootToken, filepath.ToSlash(dir))
	m, err := config.ParseTools(filepath.Join(dir, "c", "tools.yaml"), []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	// A tool's path is held in the slash form, as the file wrote it.
	want := map[string]string{"golangci-lint@2.15.0": filepath.ToSlash(lint15), "golangci-lint@2.14.1": filepath.ToSlash(lint14), "eslint@9.12.0": filepath.ToSlash(eslint)}
	if m.Version != 1 || !maps.Equal(m.Tools, want) ||
		m.Container != (config.Container{Runtime: filepath.ToSlash(podman), Image: "registry.example/gragas/proof@sha256:" + strings.Repeat("0123456789abcdef", 4)}) ||
		m.Sandbox != (config.SandboxPrograms{Userns: filepath.ToSlash(bwrap), Macos: filepath.ToSlash(sandboxExec)}) {
		t.Errorf("ParseTools = %+v", m)
	}
	if m, err := config.ParseTools(filepath.Join(dir, "c", "tools.yaml"), []byte("version: 1\n")); err != nil || m.Tools != nil || m.Version != 1 {
		t.Errorf("the version alone: %+v, %v", m, err)
	}
}

// TestEveryToolsKeyIsRefusedAtItsLine (cli.config.2, store.schemas.2): a bad
// value of each key, an unknown key and a key written twice are each refused
// naming the file and the line; a newer version is not ready, naming both.
func TestEveryToolsKeyIsRefusedAtItsLine(t *testing.T) {
	dir := t.TempDir()
	t.Parallel()
	keys := config.ToolsKeys()
	bad := map[string]string{
		"version":   "version: two",
		"tools":     "tools: {eslint: " + filepath.Join(dir, "opt", "eslint") + "}",
		"container": "container: {image: alpine}",
		"sandbox":   "sandbox: {userns: bwrap}",
	}
	for _, k := range keys {
		line, ok := bad[k.Name]
		if !ok {
			t.Errorf("%s has no planted bad value", k.Name)
			continue
		}
		data, want := "version: 1\n"+line+"\n", filepath.Join(dir, "c", "tools.yaml")+":2: "+k.Name
		if k.Name == "version" {
			data, want = line+"\n", filepath.Join(dir, "c", "tools.yaml")+":1: version"
		}
		_, err := config.ParseTools(filepath.Join(dir, "c", "tools.yaml"), []byte(data))
		if p, ok := errors.AsType[*config.Problem](err); !ok || p.Kind != config.Refused || !strings.HasPrefix(p.What, want) {
			t.Errorf("%s: %q, want refused starting %q (%v)", k.Name, data, want, err)
		}
	}
	if len(bad) != len(keys) {
		t.Errorf("%d planted values for %d keys", len(bad), len(keys))
	}
	_, err := config.ParseTools(filepath.Join(dir, "c", "tools.yaml"), read(t, "newer.yaml"))
	if p, ok := errors.AsType[*config.Problem](err); !ok || p.Kind != config.NotReady ||
		p.What != filepath.Join(dir, "c", "tools.yaml")+":1: the tools file is version 2, and this gragas reads version 1" {
		t.Errorf("a newer file: %v, want not ready naming both versions", err)
	}
}

// TestTheToolsFilesListingIsTheGolden: each fixture of testdata/tools, read,
// is what testdata/tools.golden says: what it holds, key by key, or how the
// run ends on it, at its file and line. Risk: a refusal that loses its line,
// or a file read into something it did not say.
func TestTheToolsFilesListingIsTheGolden(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(toolsFixtures)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var b strings.Builder
	for _, e := range entries {
		data, shown := underDir(dir, read(t, e.Name()))
		m, err := config.ParseTools(e.Name(), data)
		b.WriteString(shown.Replace(listing(e.Name(), m, err)))
	}
	want, err := os.ReadFile("testdata/tools.golden")
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != b.String() {
		t.Errorf("testdata/tools.golden differs. A person who has read the new listing replaces the file with it:\n%s", b.String())
	}
}

// listing is one fixture's lines of the golden.
func listing(name string, m config.Machine, err error) string {
	if err != nil {
		p, ok := errors.AsType[*config.Problem](err)
		if !ok {
			return name + ": an error that is no Problem: " + err.Error() + "\n"
		}
		end := map[config.Kind]string{config.Refused: "refused", config.NotReady: "not ready", config.Failed: "failed"}[p.Kind]
		if p.Next != "" {
			end += " (" + p.Next + ")"
		}
		return name + ": " + end + ": " + p.What + "\n"
	}
	out := name + ": version " + string(rune('0'+m.Version)) + "\n"
	for _, k := range slices.Sorted(maps.Keys(m.Tools)) {
		out += "  tool " + k + " " + m.Tools[k] + "\n"
	}
	for _, kv := range [][2]string{{"container runtime", m.Container.Runtime}, {"container image", m.Container.Image},
		{"sandbox userns", m.Sandbox.Userns}, {"sandbox macos", m.Sandbox.Macos}} {
		if kv[1] != "" {
			out += "  " + kv[0] + " " + kv[1] + "\n"
		}
	}
	return out
}

// FuzzParseTools (tests.generated.1): whatever the bytes, ParseTools answers
// a Machine or a Problem at the file, never both and never a panic, and a
// Machine it answers is version 1 with only tool@version names, absolute
// clean paths and an image named by its digest.
func FuzzParseTools(f *testing.F) {
	entries, err := os.ReadDir(toolsFixtures)
	if err != nil {
		f.Fatal(err)
	}
	dir := f.TempDir()
	for _, e := range entries {
		f.Add(bytes.ReplaceAll(read(f, e.Name()), []byte(rootToken), []byte(dir)))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := config.ParseTools(filepath.Join(dir, "c", "tools.yaml"), data)
		if err != nil {
			p, ok := errors.AsType[*config.Problem](err)
			if !ok || !strings.HasPrefix(p.What, filepath.Join(dir, "c", "tools.yaml")) {
				t.Fatalf("%v is no Problem at the file", err)
			}
			if m.Version != 0 || m.Tools != nil || m.Container != (config.Container{}) || m.Sandbox != (config.SandboxPrograms{}) {
				t.Fatalf("refused and still answered %+v", m)
			}
			return
		}
		if m.Version != config.ToolsVersion {
			t.Fatalf("read version %d", m.Version)
		}
		paths := []string{m.Container.Runtime, m.Sandbox.Userns, m.Sandbox.Macos}
		for k, p := range m.Tools {
			name, version, ok := strings.Cut(k, "@")
			if !ok || name == "" || version == "" || strings.HasPrefix(k, "-") {
				t.Fatalf("the name %q is not tool@version", k)
			}
			paths = append(paths, p)
		}
		for _, p := range paths {
			if p != "" && (!filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, "\n\r\x00")) {
				t.Fatalf("the path %q is not absolute and clean", p)
			}
		}
		if img := m.Container.Image; img != "" {
			if _, digest, ok := strings.Cut(img, "@sha256:"); !ok || len(digest) != 64 {
				t.Fatalf("the image %q is not named by its digest", img)
			}
		}
	})
}
