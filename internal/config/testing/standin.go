package configtest

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/confine"
	"github.com/birdtable/go-files/filestest"
)

// standinSource is the folder of standin.wasm's source, from the module's root.
const standinSource = "internal/dev/standin"

// StandinWasm is standin.wasm, built from the tree into t's temporary folder
// with the command the design names, GOOS=wasip1 GOARCH=wasm go build -trimpath
// -buildvcs=false -ldflags=-buildid= (the flags the digest is written with);
// the test logs its size and the toolchain that built it. It is never
// committed: a toolchain bump would break a committed copy, and a gate pins its
// digest. It reads standin's source first, since Go keys the test cache on the
// files a test reads, so a change of the source runs the test again. The one
// builder: the host's tests and the journeys both take the module from here, so
// no test keeps a copy of the command.
func StandinWasm(t testing.TB) []byte {
	t.Helper()
	return buildStandin(t, "", "", "")
}

// StandinWasmWithout is standin.wasm built to leave op out: it answers op
// unsupported, as a manifest that does not provide it promises.
// The change goes through the overlay StandinWasmChanged uses, on the one
// constant leaves.go holds, so one source builds every module and the digest
// pins the only build the tree holds. An empty op is the module StandinWasm
// gives.
func StandinWasmWithout(t testing.TB, op string) []byte {
	t.Helper()
	if op == "" {
		return StandinWasm(t)
	}
	return buildStandin(t, "leaves.go", `const leaves = ""`, `const leaves = "`+op+`"`)
}

// StandinWasmChanged is StandinWasm built from the source as it would be with
// old in the file named (a file of standin's folder) replaced by new: the
// change goes through go build's -overlay, so the tree is not written. It is
// the plant of the gate's digest check: a source that changed
// while the digest the tree holds did not. A file that holds no old fails the
// test, so a plant cannot pass for want of an edit.
func StandinWasmChanged(t testing.TB, file, old, replacement string) []byte {
	t.Helper()
	return buildStandin(t, file, old, replacement)
}

// The flags that make the module's bytes depend on its source alone, the ones the plugins'
// digests are written with: -trimpath drops the folders, -buildvcs=false the
// revision go stamps in a checkout, and an empty build id the one go derives
// from the build's inputs and the folder.
func buildStandin(t testing.TB, file, old, replacement string) []byte {
	t.Helper()
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, filepath.FromSlash(standinSource))
	files, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if !f.IsDir() {
			if _, err := os.ReadFile(filepath.Clean(filepath.Join(src, f.Name()))); err != nil {
				t.Fatal(err)
			}
		}
	}
	env := append(Inherited(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	out := filepath.Join(t.TempDir(), "standin.wasm")
	args := []string{"build", "-trimpath", "-buildvcs=false", "-ldflags=-buildid="}
	if file != "" {
		path := filepath.Join(src, file)
		text, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(text), old) {
			t.Fatalf("%s holds no %q, so the planted change changes nothing", file, old)
		}
		changed := filepath.Join(t.TempDir(), file)
		filestest.WriteFile(t, changed, []byte(strings.Replace(string(text), old, replacement, 1)))
		overlay := filepath.Join(t.TempDir(), "overlay.json")
		doc, err := json.Marshal(map[string]map[string]string{"Replace": {path: changed}})
		if err != nil {
			t.Fatal(err)
		}
		filestest.WriteFile(t, overlay, doc)
		args = append(args, "-overlay", overlay)
	}
	build := exec.CommandContext(t.Context(), "go")
	build.Args = append(build.Args, append(args, "-o", out, ".")...)
	build.Dir, build.Env = src, env
	if said, err := build.CombinedOutput(); err != nil {
		t.Fatalf("could not build standin.wasm: %v\n%s", err, GoSaid(string(said)))
	}
	ver := exec.CommandContext(t.Context(), "go", "env", "GOVERSION")
	ver.Dir, ver.Env = src, env
	said, err := ver.Output()
	if err != nil {
		t.Fatalf("could not read the toolchain's version: %v", err)
	}
	wasm, err := os.ReadFile(filepath.Clean(out))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("standin.wasm: %d bytes, built by %s", len(wasm), strings.TrimSpace(string(said)))
	return wasm
}

// moduleRoot is the folder holding go.mod, from the test's own folder up: a
// test runs in its package's folder, which is inside the module.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return moduleRootFrom(dir)
}

// errNoModule is moduleRootFrom's answer when no folder above holds a go.mod.
var errNoModule = errors.New("no go.mod above the test's folder, so standin.wasm has no source to build from")

// moduleRootFrom is the nearest folder at or above dir holding a go.mod that
// os.Stat follows. The walk is go-files' confine.Nearest, held to
// the answer the os.Stat walk before it gave: Nearest asks each folder with
// Lstat, so a go.mod that is a link to nothing counts to it, and here the walk
// goes on above such a go.mod, as it does above any os.Stat cannot follow.
func moduleRootFrom(dir string) (string, error) {
	for {
		root, err := confine.Nearest(dir, "go.mod", "")
		if errors.Is(err, fs.ErrNotExist) {
			return "", errNoModule
		}
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root, nil
		}
		up := filepath.Dir(root)
		if up == root {
			return "", errNoModule
		}
		dir = up
	}
}
