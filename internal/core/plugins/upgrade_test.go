package plugins_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

// The standin grant of every version, and a second grant only 1.1.0 asks for.
const (
	upGrant = `  - id: standin
    path: bin/standin
    version_argv: [--version]
    versions: ">=1.0.0 <2.0.0"
`
	upGrantLint = `  - id: standin-lint
    path: bin/standin-lint
    version_argv: [version]
    versions: ">=1.0.0 <2.0.0"
`
)

// upFiles is the standin bundle of version, its manifest asking for programs.
func upFiles(version, programs string) []model.File {
	manifest := `contract: 1
needs: []
name: standin
version: ` + version + `
tool: standin
tool_versions: ">=1.0.0 <2.0.0"
targets: [source]
provides: [entries]
extends: false
warning: false
programs:
` + programs + "wasm: " + string(digest.Prefixed(addModule())) + "\n"
	return []model.File{
		{Path: "plugin.wasm", Data: addModule()},
		{Path: "plugin.yaml", Data: []byte(manifest)},
	}
}

func upRef(version string) model.PluginRef {
	return model.PluginRef{Name: "standin", Version: version}
}

// upLock is the lock of each version, locked at upFiles of it with the one grant.
func upLock(versions ...string) []byte {
	var es []pluginbundle.LockEntry
	for _, v := range versions {
		es = append(es, pluginbundle.LockEntry{Ref: upRef(v), Source: "file:standin", Digest: pluginbundle.Digest(upFiles(v, upGrant))})
	}
	return pluginbundle.EncodePluginLock(es)
}

// upInput is the upgrade to version, its bundle asking for programs, over lock.
func upInput(t *testing.T, lock []byte, version, programs string, named bool) plugins.UpgradeInput {
	t.Helper()
	add := plugins.AddInput{Ref: upRef(version), Source: "file:standin", Files: upFiles(version, programs),
		LockFile: lock, LockDigest: model.Digest(digest.Prefixed(lock)),
		Bounds: pluginbundle.Bounds{Module: 1024, File: 1024, Total: 8192, Lock: 4096}}
	old, err := plugins.UpgradeFrom(lock, "standin", version)
	if err != nil {
		t.Fatalf("UpgradeFrom = %v", err)
	}
	in := plugins.UpgradeInput{Add: add, Named: named, Old: old}
	if old != add.Ref {
		in.OldFiles = upFiles(old.Version, upGrant)
	}
	return in
}

func targetPaths(ts []model.Target) string {
	var out []string
	for _, tg := range ts {
		verb := "write "
		if tg.Delete {
			verb = "remove "
		}
		out = append(out, verb+tg.Path)
	}
	return strings.Join(out, ", ")
}

// TestPlanUpgradeWritesTheNewThenTheLockThenRemovesTheOld: the new files,
// then the lock with the old entry swapped for the new, then the old files
// removed, each by the digest of the bytes judged. The lock names the new
// version alone, and the two manifests are those whose grants the screen
// compares. Planted: the old files removed before the lock, or kept.
func TestPlanUpgradeWritesTheNewThenTheLockThenRemovesTheOld(t *testing.T) {
	t.Parallel()
	in := upInput(t, upLock("1.0.0"), "1.1.0", upGrant+upGrantLint, false)
	u, err := plugins.PlanUpgrade(in)
	if err != nil {
		t.Fatalf("PlanUpgrade = %v", err)
	}
	want := "write plugins/standin/1.1.0/plugin.wasm, write plugins/standin/1.1.0/plugin.yaml, write plugins/lock.yaml, remove plugins/standin/1.0.0/plugin.wasm, remove plugins/standin/1.0.0/plugin.yaml"
	if got := targetPaths(u.Targets); got != want {
		t.Fatalf("targets = %s\nwant      %s", got, want)
	}
	for _, tg := range u.Targets[3:] {
		f := upFiles("1.0.0", upGrant)[slices.IndexFunc(upFiles("1.0.0", upGrant), func(f model.File) bool { return "plugins/standin/1.0.0/"+f.Path == tg.Path })]
		if tg.Before != model.Digest(digest.Prefixed(f.Data)) || len(tg.Data) != 0 {
			t.Errorf("%s: before %s, %d bytes; want the old file's digest and no data", tg.Path, tg.Before, len(tg.Data))
		}
	}
	written, err := pluginbundle.DecodePluginLock(u.Targets[2].Data)
	if err != nil || len(written) != 1 || written[0].Ref != upRef("1.1.0") || written[0].Digest != u.Digest {
		t.Errorf("lock = %v, %v; want 1.1.0 alone at the new digest", written, err)
	}
	if u.Old != upRef("1.0.0") || u.OldDigest != pluginbundle.Digest(upFiles("1.0.0", upGrant)) || len(u.Stays) != 0 || u.Older {
		t.Errorf("plan = old %v at %s, stays %v, older %v", u.Old, u.OldDigest, u.Stays, u.Older)
	}
	if len(u.OldManifest.Programs) != 1 || len(u.NewManifest.Programs) != 2 || u.NewManifest.Version != "1.1.0" {
		t.Errorf("manifests: old %d programs, new %d of %s; want 1 and 2 of 1.1.0", len(u.OldManifest.Programs), len(u.NewManifest.Programs), u.NewManifest.Version)
	}
}

// TestPlanUpgradeKeepsTheOtherVersionsTheLockHolds: the version replaced is
// the newest the lock holds; an older one keeps its entry and its folder, and
// the plan names it as staying. Planted: every version of the name dropped.
func TestPlanUpgradeKeepsTheOtherVersionsTheLockHolds(t *testing.T) {
	t.Parallel()
	u, err := plugins.PlanUpgrade(upInput(t, upLock("0.9.0", "1.0.0"), "1.1.0", upGrant, false))
	if err != nil {
		t.Fatalf("PlanUpgrade = %v", err)
	}
	if u.Old != upRef("1.0.0") || !slices.Equal(u.Stays, []model.PluginRef{upRef("0.9.0")}) {
		t.Errorf("old %v, stays %v; want 1.0.0 replaced and 0.9.0 staying", u.Old, u.Stays)
	}
	written, _ := pluginbundle.DecodePluginLock(u.Targets[2].Data)
	var refs []string
	for _, e := range written {
		refs = append(refs, e.Ref.Version)
	}
	if strings.Join(refs, " ") != "0.9.0 1.1.0" {
		t.Errorf("lock holds %v, want 0.9.0 and 1.1.0", refs)
	}
	if strings.Contains(targetPaths(u.Targets), "0.9.0") {
		t.Errorf("targets touch 0.9.0: %s", targetPaths(u.Targets))
	}
}

// TestPlanUpgradeToTheVersionInstalledIsNothingToDo: the version the lock
// holds at these files, with its folder whole, is nothing to write and reads
// no old bundle. Planted: an installed version rewritten.
func TestPlanUpgradeToTheVersionInstalledIsNothingToDo(t *testing.T) {
	t.Parallel()
	in := upInput(t, upLock("1.0.0"), "1.0.0", upGrant, false)
	in.Add.Present = []string{"plugins/standin/1.0.0/plugin.wasm", "plugins/standin/1.0.0/plugin.yaml"}
	u, err := plugins.PlanUpgrade(in)
	if err != nil || !u.Nothing || len(u.Targets) != 0 {
		t.Errorf("PlanUpgrade = nothing %v, %d targets, %v; want nothing to do", u.Nothing, len(u.Targets), err)
	}
}

// TestPlanUpgradeToAnOlderVersionIsRefusedUnlessNamed: the newest a source
// holds is never a step back; the same version named is planned, and says it
// is older. Planted: an older version taken as the newest.
func TestPlanUpgradeToAnOlderVersionIsRefusedUnlessNamed(t *testing.T) {
	t.Parallel()
	if _, err := plugins.PlanUpgrade(upInput(t, upLock("1.1.0"), "1.0.0", upGrant, false)); !errors.Is(err, plugins.ErrOlder) {
		t.Errorf("older, not named: %v, want ErrOlder", err)
	}
	u, err := plugins.PlanUpgrade(upInput(t, upLock("1.1.0"), "1.0.0", upGrant, true))
	if err != nil || !u.Older || u.Old != upRef("1.1.0") {
		t.Errorf("older, named: older %v, old %v, %v; want it planned", u.Older, u.Old, err)
	}
}

// TestPlanUpgradeFromABundleNotPinnedIsRefused: an installed bundle whose
// files are not what its entry pins, or are gone, is refused before any grant
// is compared, and nothing is planned. Planted: the grants of a changed
// bundle compared as if locked.
func TestPlanUpgradeFromABundleNotPinnedIsRefused(t *testing.T) {
	t.Parallel()
	changed := upInput(t, upLock("1.0.0"), "1.1.0", upGrant, false)
	changed.OldFiles = upFiles("1.0.0", upGrant+upGrantLint)
	gone := upInput(t, upLock("1.0.0"), "1.1.0", upGrant, false)
	gone.OldFiles = nil
	for name, in := range map[string]plugins.UpgradeInput{"changed": changed, "gone": gone} {
		u, err := plugins.PlanUpgrade(in)
		if !errors.Is(err, plugins.ErrNotPinned) || len(u.Targets) != 0 {
			t.Errorf("%s: %d targets, %v; want ErrNotPinned and no plan", name, len(u.Targets), err)
		}
	}
}

// TestUpgradeOfAPluginNotInstalledIsRefused: a lock that holds no version of
// the name, and no lock, match port.ErrNotInstalled. Planted: an upgrade that
// adds a plugin nobody installed.
func TestUpgradeOfAPluginNotInstalledIsRefused(t *testing.T) {
	t.Parallel()
	other := pluginbundle.EncodePluginLock([]pluginbundle.LockEntry{{Ref: model.PluginRef{Name: "other", Version: "1.0.0"}, Source: "file:other", Digest: pluginbundle.Digest(upFiles("1.0.0", upGrant))}})
	for name, lock := range map[string][]byte{"no lock": nil, "another plugin": other} {
		if _, err := plugins.UpgradeFrom(lock, "standin", "1.1.0"); !errors.Is(err, port.ErrNotInstalled) {
			t.Errorf("%s: %v, want port.ErrNotInstalled", name, err)
		}
	}
}

// TestUpgradeFromTakesTheNewestByVersionNotByBytes: 1.10.0 is newer than
// 1.9.0, as model.CompareVersions orders them. Planted: a byte order.
func TestUpgradeFromTakesTheNewestByVersionNotByBytes(t *testing.T) {
	t.Parallel()
	old, err := plugins.UpgradeFrom(upLock("1.10.0", "1.9.0"), "standin", "2.0.0")
	if err != nil || old != upRef("1.10.0") {
		t.Errorf("UpgradeFrom = %v, %v; want 1.10.0", old, err)
	}
}

// TestPlanUpgradeRefusesAnOldItWasNotGiven: the caller's old version must be
// the lock's, so the files judged are those of the version replaced.
// Planted: the bundle of one version judged against another's pin.
func TestPlanUpgradeRefusesAnOldItWasNotGiven(t *testing.T) {
	t.Parallel()
	in := upInput(t, upLock("0.9.0", "1.0.0"), "1.1.0", upGrant, false)
	in.Old, in.OldFiles = upRef("0.9.0"), upFiles("0.9.0", upGrant)
	if _, err := plugins.PlanUpgrade(in); err == nil {
		t.Error("PlanUpgrade took an old version the lock does not replace")
	}
}
