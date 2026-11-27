package internal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/birdtable/go-proc/proc"
	configtest "github.com/birdtable/gragas/internal/config/testing"
)

// The fake npm registry on the loopback and the pinned pnpm that the install tests share
// (TestABuildScriptMakesTheToolsInstallRed): what is left of the release-age tests after the pnpm change.

// npmPackage is one version a fake registry serves: its dependencies, its
// scripts and the time the registry says it was published, or, when moved is
// set, the time it answers at each request, which no real registry changes.
type npmPackage struct {
	deps      map[string]string
	scripts   map[string]string
	published time.Time
	moved     func() time.Time
}

// npmRegistry serves pkgs on the loopback as the npm registry does: the
// package document with its time map, and the tarball the document names.
// It answers its base URL, or false when the gate's network block refuses
// the listener.
func npmRegistry(t *testing.T, pkgs map[string]npmPackage) (string, bool) {
	t.Helper()
	ln, ok := loopback(t)
	if !ok {
		return "", false
	}
	base := "http://" + ln.Addr().String()
	mux := http.NewServeMux()
	for name, p := range pkgs {
		tgz := npmTarball(t, name, p.deps, p.scripts)
		sum := sha512.Sum512(tgz)
		file := name + "-1.0.0.tgz"
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, _ *http.Request) {
			published := p.published
			if p.moved != nil {
				published = p.moved()
			}
			when := published.UTC().Format(time.RFC3339)
			doc, err := json.Marshal(map[string]any{
				"name":      name,
				"dist-tags": map[string]string{"latest": "1.0.0"},
				"versions": map[string]any{"1.0.0": map[string]any{
					"name": name, "version": "1.0.0", "dependencies": p.deps,
					"scripts": p.scripts, "hasInstallScript": len(p.scripts) > 0,
					"dist": map[string]string{"tarball": base + "/" + name + "/-/" + file, "integrity": "sha512-" + base64.StdEncoding.EncodeToString(sum[:])},
				}},
				"time": map[string]string{"created": when, "modified": when, "1.0.0": when},
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(doc)
		})
		mux.HandleFunc("GET /"+name+"/-/"+file, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(tgz)
		})
	}
	serve(t, ln, mux)
	return base + "/", true
}

// loopback is a listener on 127.0.0.1, or false when the gate's network
// block refuses it: a refusal of permission, EPERM or EACCES on Unix, which
// the standard library matches to fs.ErrPermission (gragas#1605, row X 174).
func loopback(t *testing.T) (net.Listener, bool) {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp4", "127.0.0.1:0")
	if errors.Is(err, fs.ErrPermission) {
		t.Logf("ended early: the gate's network block refused a listener on 127.0.0.1 (%v); this test runs in full outside the block", err)
		return nil, false
	}
	if err != nil {
		t.Fatalf("a listener on 127.0.0.1: %v", err)
	}
	return ln, true
}

// serve serves h on ln until the test ends.
func serve(t *testing.T, ln net.Listener, h http.Handler) {
	t.Helper()
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

// npmProxy is a proxy on the loopback that forwards nothing: it refuses
// every request and keeps the host each one was for. pnpm is sent through
// it for every host but the loopback, so a request meant for a registry
// outside ends here, and seen says it was made.
func npmProxy(t *testing.T) (proxy string, seen func() []string, ok bool) {
	t.Helper()
	ln, ok := loopback(t)
	if !ok {
		return "", nil, false
	}
	var mu sync.Mutex
	var hosts []string
	serve(t, ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hosts = append(hosts, r.Method+" "+r.Host)
		mu.Unlock()
		http.Error(w, "the test's proxy forwards nothing", http.StatusForbidden)
	}))
	return "http://" + ln.Addr().String() + "/", func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(hosts)
	}, true
}

// npmTarball is a package's tarball: package/package.json alone.
func npmTarball(t *testing.T, name string, deps, scripts map[string]string) []byte {
	t.Helper()
	pj, err := json.Marshal(map[string]any{"name": name, "version": "1.0.0", "dependencies": deps, "scripts": scripts})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "package/package.json", Mode: int64(fs.FileMode(0o644)), Size: int64(len(pj)), ModTime: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(pj); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pinnedPnpm is the pnpm task tools installed and a PATH that finds it and the
// pinned node first. It sets PATH to that, so its caller runs alone.
func pinnedPnpm(t *testing.T) (pnpm, path string) {
	t.Helper()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(abs, ".tools", "bin")
	pnpm = filepath.Join(bin, proc.LocalFileName("pnpm"))
	if _, err := os.Stat(pnpm); err != nil {
		t.Fatal("the pinned pnpm is not installed: run task tools")
	}
	path = bin + string(os.PathListSeparator) + inheritedPath()
	t.Setenv("PATH", path)
	return pnpm, path
}

// pnpmRegistryKey is the spelling pnpm 12 reads its registry by from the
// environment.
const pnpmRegistryKey = "pnpm_config_registry="

// pnpmRun is the pinned pnpm, its PATH, the fake registry and a home folder
// (empty: a new one for each run), and env, set after every other variable, as
// a person's shell would have them.
type pnpmRun struct {
	pnpm, path, reg, home string
	env                   []string
}

// as runs the pinned pnpm in dir as task tools does, with a store, cache and
// config of the test's own, the registry set by the variable registry, and
// every host but the loopback sent to the proxy px. pnpm 12 reads its
// settings from the environment only as pnpm_config_*: an npm_config_* one
// is not read (the registry one sent the first run of this test to the
// public registry). Any inherited setting of either spelling, and any
// inherited proxy, is dropped, since one in the environment wins over the
// workspace file's window. No fetch is retried, so a refused one fails at
// once. The caller writes the arguments out, as lint's callers do.
func (p pnpmRun) as(t *testing.T, dir string, cmd *exec.Cmd, registry, px string) (string, error) {
	t.Helper()
	home := p.home
	if home == "" {
		home = t.TempDir()
	}
	if cmd.Err != nil || cmd.Path != p.pnpm {
		t.Fatalf("pnpm is %s, not the pinned %s: %v", cmd.Path, p.pnpm, cmd.Err)
	}
	cmd.Dir = dir
	cmd.Env = slices.DeleteFunc(configtest.Inherited(), func(kv string) bool {
		k, _, _ := strings.Cut(strings.ToLower(kv), "=")
		return strings.HasPrefix(k, "pnpm_config_") || strings.HasPrefix(k, "npm_config_") || k == "http_proxy" || k == "https_proxy" || k == "no_proxy"
	})
	cmd.Env = append(cmd.Env,
		"PATH="+p.path,
		registry,
		"pnpm_config_https_proxy="+px,
		"pnpm_config_proxy="+px,
		"pnpm_config_noproxy=127.0.0.1",
		"HTTPS_PROXY="+px,
		"HTTP_PROXY="+px,
		"NO_PROXY=127.0.0.1",
		"pnpm_config_fetch_retries=0",
		"pnpm_config_store_dir="+filepath.Join(home, "store"),
		"pnpm_config_cache_dir="+filepath.Join(home, "cache"),
		"pnpm_config_update_notifier=false")
	// Every variable a program reads for a home, config, cache, state or temp folder (the names go-proc's
	// HomeEnv gives, whatever the system) points under the case's own folder (the home
	// itself for the home variable), so none is the person's. Each folder that lies beneath
	// home is made first; a value that does not (a volume) is not a folder.
	for _, kv := range proc.HomeEnv(home) {
		_, v, _ := strings.Cut(kv, "=")
		if rel, err := filepath.Rel(home, v); err == nil && filepath.IsLocal(rel) {
			if err := os.MkdirAll(v, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		cmd.Env = append(cmd.Env, kv)
	}
	cmd.Env = append(cmd.Env, p.env...)
	// The run is bounded and its tree killed whole at the end (go-proc): a
	// pnpm that stalls fails its own case with what it printed, where an
	// unbounded one held the package's whole -timeout once (PR 1388's push).
	// It has no Grace: a stopped pnpm is killed at once, with no TERM first,
	// which go-proc refuses on a system that sends none it can catch
	// (gragas#1590). pnpm needs none to stop cleanly here: its home, store,
	// cache and project are the test's own temporary folders, dropped with
	// the case, so a run killed mid-install leaves nothing a later run reads.
	res, err := proc.Run(t.Context(), proc.Cmd{
		Path:      cmd.Path,
		Args:      cmd.Args[1:],
		Dir:       cmd.Dir,
		Env:       cmd.Env,
		Timeout:   pnpmDeadline,
		Drain:     10 * time.Second,
		MaxOutput: 4 << 20,
	})
	out := string(res.Stdout) + string(res.Stderr)
	switch {
	case err != nil:
		return out, fmt.Errorf("pnpm %s, stopped: %w", strings.Join(cmd.Args[1:], " "), err)
	case res.Code != 0:
		return out, fmt.Errorf("pnpm %s exited %d", strings.Join(cmd.Args[1:], " "), res.Code)
	}
	return out, nil
}

// pnpmDeadline is how long one pnpm run of these tests may take. Each is a
// few hundred milliseconds against the loopback registry; the margin is for
// a loaded machine.
const pnpmDeadline = 2 * time.Minute

// asks is nil when pnpm, so set up, says it will ask the fake registry.
func (p pnpmRun) asks(t *testing.T, registry, px string) error {
	t.Helper()
	out, err := p.as(t, t.TempDir(), exec.CommandContext(t.Context(), "pnpm", "config", "get", "registry"), registry, px)
	if err != nil {
		return fmt.Errorf("the pinned pnpm would not say which registry it asks: %w\n%s", err, out)
	}
	if got := strings.TrimSpace(out); got != p.reg {
		return fmt.Errorf("the pinned pnpm would not ask the test's registry %s, it answers %q", p.reg, got)
	}
	return nil
}

// toolsInstall are the arguments task tools gives pnpm in tools/.node.
func toolsInstall() []string {
	return []string{"install", "--frozen-lockfile", "--config.dangerously-allow-all-builds=false", "--config.strict-dep-builds=true"}
}

// freeze is one frozen install of a project that depends on dep at 1.0.0:
// its lock is written under lockWith (with no window, as a lock written
// before the window, or on a machine without it, would be), between runs
// when it is set, and the project is installed from the lock alone under
// settings. Cold, the install has a cache of its own, as on a fresh clone;
// warm, it shares the one the lock was written with, which holds every
// locked version's metadata, as on a machine that has resolved them before.
// With scripts, the install is tools/.node's line, which runs scripts;
// without, the explorer's, which runs none. args, when set, are the install's
// arguments instead, for a control.
type freeze struct {
	dep, lockWith, settings string
	warm, scripts           bool
	args                    []string
	between                 func()
}

// frozen runs f, and answers the project's folder and the install's output.
func (p pnpmRun) frozen(t *testing.T, px string, f freeze) (string, string, error) {
	t.Helper()
	if f.warm {
		p.home = t.TempDir()
	}
	dir := t.TempDir()
	pj := `{"name": "plant", "version": "0.0.0", "private": true, "dependencies": {"` + f.dep + `": "1.0.0"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pj), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pnpm-workspace.yaml"), []byte(f.lockWith), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := p.as(t, dir, exec.CommandContext(t.Context(), "pnpm", "install", "--lockfile-only", "--ignore-scripts"), pnpmRegistryKey+p.reg, px); err != nil {
		t.Fatalf("writing the lock: %v\n%s", err, out)
	}
	if f.between != nil {
		f.between()
	}
	if err := os.WriteFile(filepath.Join(dir, "pnpm-workspace.yaml"), []byte(f.settings), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"install", "--frozen-lockfile", "--ignore-scripts"}
	switch {
	case f.args != nil:
		args = f.args
	case f.scripts:
		args = toolsInstall()
	}
	install := exec.CommandContext(t.Context(), "pnpm", args...)
	out, err := p.as(t, dir, install, pnpmRegistryKey+p.reg, px)
	return dir, out, err
}
