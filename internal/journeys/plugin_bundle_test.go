package journeys

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The module every journey bundle holds, and its digest as plugin.yaml names
// it. A journey of the list and approve commands runs no module: the commands
// read a bundle and judge it, so any bytes do.
const (
	bundleModule       = "module of the journeys"
	bundleModuleDigest = "sha256:4894e91e6f54a264b7447fef41d1078a2f4ebf43460c8c7fbb95ca2d036891f5"
	noDigest           = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

// bundleOf is a plugin bundle of the contract's valid manifest, named name at
// version: its files by path.
func bundleOf(t *testing.T, name, version string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(moduleRoot(), "internal", "testdata", "plugin-v1", "manifest.valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var docs []map[string]any
	if err := json.Unmarshal(b, &docs); err != nil || len(docs) == 0 {
		t.Fatalf("manifest.valid.json: %d documents, %v", len(docs), err)
	}
	m := docs[0]
	m["name"], m["version"], m["wasm"] = name, version, bundleModuleDigest
	y, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"plugin.yaml": string(y) + "\n", "plugin.wasm": bundleModule}
}

// lockRow is one entry of a plugin lock a journey writes.
type lockRow struct{ name, version, source string }

// lockOf is the plugin lock that names each bundle once, from where it came,
// by the digest in digests (name@version), or by none.
func lockOf(rows []lockRow, digests map[string]string) string {
	var sb strings.Builder
	sb.WriteString("version: 1\nplugins:\n")
	for _, r := range rows {
		d := digests[r.name+"@"+r.version]
		if d == "" {
			d = noDigest
		}
		sb.WriteString("  - name: " + r.name + "\n    version: " + r.version + "\n    source: " + r.source + "\n    digest: " + d + "\n")
	}
	return sb.String()
}

// pluginRulebook is a copy of the base fixture rulebook with the bundles
// installed and locked, and the digest of each by name@version. The lock's
// digests are the ones the binary reads the bundles as: it is asked once
// against a lock of none, so a journey sums nothing itself (tests.journeys.2).
func pluginRulebook(t *testing.T, rows ...lockRow) (dir string, digests map[string]string) {
	t.Helper()
	files := fixtureCopy(t, "base")
	for _, r := range rows {
		for p, body := range bundleOf(t, r.name, r.version) {
			files["plugins/"+r.name+"/"+r.version+"/"+p] = body
		}
	}
	files["plugins/lock.yaml"] = lockOf(rows, nil)
	dir = rulebookFrom(t, files)
	env, _ := folders(t)
	r := run(t, dir, env, "plugin", "list", "--json")
	var l struct {
		Rows []struct{ Name, Version, Digest string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &l); err != nil || len(l.Rows) != len(rows) {
		t.Fatalf("plugin list --json: %d rows for %d bundles, %v\n%s%s", len(l.Rows), len(rows), err, r.stdout, r.stderr)
	}
	digests = map[string]string{}
	for _, x := range l.Rows {
		digests[x.Name+"@"+x.Version] = x.Digest
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins", "lock.yaml"), []byte(lockOf(rows, digests)), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, digests
}
