package coreapi_test

import (
	"context"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// The grants of the standin bundles: 1.0.0 and 0.9.0 ask for standin's
// version with --version; 1.1.0 asks with version, and adds standin-lint.
const (
	grantsOld = "  - id: standin\n    path: bin/standin\n    version_argv: [--version]\n    versions: \">=1.0.0 <2.0.0\"\n"
	grantsNew = "  - id: standin\n    path: bin/standin\n    version_argv: [version]\n    versions: \">=1.0.0 <2.0.0\"\n" +
		"  - id: standin-lint\n    path: bin/standin-lint\n    version_argv: [version]\n    versions: \">=1.0.0 <2.0.0\"\n"
)

// upBundle is the standin bundle of version as a folder holds it.
func upBundle(version, grants string) map[string]string {
	mod := "\x00asm\x01\x00\x00\x00standin"
	man := "contract: 1\nneeds: []\nname: standin\nversion: " + version + "\ntool: standin\ntool_versions: \">=1.0.0 <2.0.0\"\ntargets: [source]\nprovides: [entries]\nextends: false\nwarning: false\nprograms:\n" + grants + "wasm: " + string(digest.Prefixed([]byte(mod))) + "\n"
	return map[string]string{"plugin.wasm": mod, "plugin.yaml": man}
}

// upSource seeds the folder at as a source holding each version's bundle,
// below standin/<version>.
func upSource(w *porttest.World, at string, versions map[string]string) {
	files := map[string]string{}
	for v, grants := range versions {
		for p, data := range upBundle(v, grants) {
			files["standin/"+v+"/"+p] = data
		}
		// the fake opens only a root seeded as one: plugin add opens the
		// version's folder
		w.Seed(at+"/standin/"+v, upBundle(v, grants))
	}
	w.Seed(at, files)
}

// upAsk is the ask of a plan command run under run.
func upAsk(root model.Root, cmd, run string) model.ChangeAsk {
	a := changeAsk(root, cmd)
	a.Run = run
	return a
}

// installUp adds each version of the source at to the rulebook at root.
func installUp(t *testing.T, w *porttest.World, root model.Root, at string, versions ...string) {
	t.Helper()
	for _, v := range versions {
		c, ref, err := coreapi.PluginAddFrom(context.Background(), w.Ports(), at+"/standin/"+v, upAsk(root, "gragas plugin add", "add-"+strings.ReplaceAll(v, ".", "-")))
		if err != nil || ref != nil {
			t.Fatal(ref, err)
		}
		if out := c.Apply(context.Background(), ""); out.Word != model.Done {
			t.Fatalf("add of %s ended %q (%v)", v, out.Word, out.Cause)
		}
	}
}

func upPlanText(t *testing.T, c *coreapi.Change) string {
	t.Helper()
	shown, ok := c.Preview(context.Background())
	if !ok {
		if shown.Word != model.Done || !shown.Nothing {
			t.Fatalf("the preview ended %q, cause %v", shown.Word, shown.Cause)
		}
		// nothing to do ends the preview done, and asks no yes
		return "nothing to do"
	}
	var all []string
	for _, s := range shown.Plan.Sections {
		all = append(all, s.Lines...)
	}
	return strings.Join(all, "\n")
}

// TestPluginUpgradeShowsTheGrantsWritesTheNewAndRemovesTheOld: with no
// version the newest folder of the source is taken; the dry run shows both
// digests, each grant changed or added with how its version is asked, the
// lock entry swapped and the old files removed, and that it approves nothing.
// --apply writes the new bundle and the lock, removes the old files, and the
// closing words say the new version is installed and not approved and name
// plugin approve. A second run is nothing to do. Planted: the old files kept,
// the closing words dropped.
func TestPluginUpgradeShowsTheGrantsWritesTheNewAndRemovesTheOld(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	upSource(w, "/work/src", map[string]string{"1.0.0": grantsOld, "1.1.0": grantsNew})
	root := w.Seed("/work/repo", map[string]string{"keep": "k"})
	installUp(t, w, root, "/work/src", "1.0.0")
	ctx := context.Background()
	c, ref, err := coreapi.PluginUpgradeFrom(ctx, w.Ports(), "/work/src", "standin", upAsk(root, "gragas plugin upgrade", "up-1"))
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	plan := upPlanText(t, c)
	for _, want := range []string{
		"from standin@1.0.0: sha256:",
		"to standin@1.1.0: sha256:",
		"grant standin changed: its version is asked with version (was --version)",
		"grant standin-lint added: its version is asked with version",
		"file:plugins/standin/1.1.0/plugin.yaml: added",
		"plugin:standin@1.0.0: lock entry removed",
		"file:plugins/standin/1.0.0/plugin.yaml: removed",
		"this approves nothing: standin@1.1.0 is installed and not approved by this upgrade",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("the plan does not say %q:\n%s", want, plan)
		}
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done || out.Nothing {
		t.Fatalf("apply ended %q, nothing %v (%v)", out.Word, out.Nothing, out.Cause)
	}
	for _, p := range []string{"plugins/standin/1.0.0/plugin.yaml", "plugins/standin/1.0.0/plugin.wasm"} {
		if read(t, w, root, p) != "" {
			t.Errorf("%s is still there", p)
		}
	}
	if read(t, w, root, "plugins/standin/1.1.0/plugin.yaml") == "" {
		t.Error("the new manifest was not written")
	}
	entries, err := pluginbundle.DecodePluginLock([]byte(read(t, w, root, pluginbundle.PluginLockPath)))
	if err != nil || len(entries) != 1 || entries[0].Ref.Version != "1.1.0" || entries[0].Source != coreapi.LocalSource {
		t.Errorf("the lock = %v, %v; want 1.1.0 alone, from a local folder", entries, err)
	}
	if what, next := c.Closing(); what != "standin@1.1.0 is installed and not approved by this upgrade" || next != "gragas plugin approve standin@1.1.0" {
		t.Errorf("closing = %q, next %q", what, next)
	}
	again, ref, err := coreapi.PluginUpgradeFrom(ctx, w.Ports(), "/work/src", "standin@1.1.0", upAsk(root, "gragas plugin upgrade", "up-2"))
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if plan := upPlanText(t, again); !strings.Contains(plan, "nothing to do") {
		t.Errorf("the second plan = %q, want nothing to do", plan)
	}
	if out := again.Apply(ctx, ""); out.Word != model.Done || !out.Nothing {
		t.Errorf("the second run ended %q, nothing %v; want done with nothing", out.Word, out.Nothing)
	}
	if what, _ := again.Closing(); what != "" {
		t.Errorf("nothing to do closes with %q", what)
	}
}

// TestPluginUpgradeTakesTheNewestVersionFolderByVersionNotByBytes: with no
// version, the source's newest folder is taken by version order, and a folder
// that is not a version is no candidate. The source holds 1.8.0, 1.9.0 and
// 1.10.0, a prerelease 2.0.0-rc.1 that version order ranks above them, and
// notes; the lock is at 1.8.0, and the plan is to 1.10.0. Planted: the
// versions compared as bytes (1.9.0 taken), or the VersionFormed filter
// dropped (2.0.0-rc.1 taken).
func TestPluginUpgradeTakesTheNewestVersionFolderByVersionNotByBytes(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	versions := []string{"1.8.0", "1.9.0", "1.10.0", "2.0.0-rc.1"}
	held := map[string]string{}
	for _, v := range versions {
		held[v] = grantsOld
	}
	upSource(w, "/work/src", held)
	// a seed replaces the root: the source again, with notes beside the
	// versions
	files := map[string]string{"standin/notes/plugin.yaml": "not a version\n"}
	for _, v := range versions {
		for p, data := range upBundle(v, grantsOld) {
			files["standin/"+v+"/"+p] = data
		}
	}
	w.Seed("/work/src", files)
	root := w.Seed("/work/repo", map[string]string{"keep": "k"})
	installUp(t, w, root, "/work/src", "1.8.0")
	c, ref, err := coreapi.PluginUpgradeFrom(context.Background(), w.Ports(), "/work/src", "standin", upAsk(root, "gragas plugin upgrade", "up-1"))
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	plan := upPlanText(t, c)
	if !strings.Contains(plan, "from standin@1.8.0: sha256:") || !strings.Contains(plan, "to standin@1.10.0: sha256:") {
		t.Errorf("the plan is not from 1.8.0 to 1.10.0:\n%s", plan)
	}
}

// TestPluginUpgradeLeavesTheOtherVersionsTheLockNames: 0.9.0 and 1.0.0 are
// installed; the upgrade to 1.1.0 replaces 1.0.0, and 0.9.0 keeps its entry
// and its folder, and the plan says it stays. Planted: every version removed.
func TestPluginUpgradeLeavesTheOtherVersionsTheLockNames(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	upSource(w, "/work/src", map[string]string{"0.9.0": grantsOld, "1.0.0": grantsOld, "1.1.0": grantsOld})
	root := w.Seed("/work/repo", map[string]string{"keep": "k"})
	installUp(t, w, root, "/work/src", "0.9.0", "1.0.0")
	ctx := context.Background()
	c, ref, err := coreapi.PluginUpgradeFrom(ctx, w.Ports(), "/work/src", "standin@1.1.0", upAsk(root, "gragas plugin upgrade", "up-1"))
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	plan := upPlanText(t, c)
	for _, want := range []string{"standin@0.9.0 stays: the lock names it", "grants: none added, removed or changed", "from standin@1.0.0"} {
		if !strings.Contains(plan, want) {
			t.Errorf("the plan does not say %q:\n%s", want, plan)
		}
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if read(t, w, root, "plugins/standin/0.9.0/plugin.yaml") == "" || read(t, w, root, "plugins/standin/1.0.0/plugin.yaml") != "" {
		t.Error("0.9.0 is gone or 1.0.0 is still there")
	}
	lock := read(t, w, root, pluginbundle.PluginLockPath)
	if !strings.Contains(lock, "0.9.0") || strings.Contains(lock, "1.0.0") || !strings.Contains(lock, "1.1.0") {
		t.Errorf("the lock = %s; want 0.9.0 and 1.1.0", lock)
	}
}

// TestPluginUpgradeRefusalsSayTheLineToType: a plugin the lock does not hold
// names plugin add; an older version taken as the newest says to name the
// version and names plugin list, never the downgrade to paste, and named it
// is planned; an installed bundle that is not
// what the lock pins names plugin list; a ref that is not <name>[@<version>]
// is usage. Each refusal plans nothing. Planted: the older version taken, the
// source joined into the hint unquoted.
func TestPluginUpgradeRefusalsSayTheLineToType(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	upSource(w, "/work/src", map[string]string{"1.0.0": grantsOld, "1.1.0": grantsNew})
	upSource(w, "/work/old", map[string]string{"1.0.0": grantsOld})
	ctx := context.Background()
	refusal := func(root model.Root, source, ref string) *model.Refusal {
		t.Helper()
		c, r, err := coreapi.PluginUpgradeFrom(ctx, w.Ports(), source, ref, upAsk(root, "gragas plugin upgrade", "up-r"))
		if err != nil || r == nil || c != nil {
			t.Fatalf("upgrade %s %s = change %v, refusal %v, %v; want a refusal", source, ref, c != nil, r, err)
		}
		return r
	}

	empty := w.Seed("/work/empty", map[string]string{"keep": "k"})
	if r := refusal(empty, "/work/src", "standin"); r.Word() != model.Refused || r.Hint() != "gragas plugin add /work/src/standin/1.1.0" {
		t.Errorf("not installed: %q, %q, next %q", r.Word(), r.Observed(), r.Hint())
	}
	// a source a shell would split is quoted as one word, so the line pastes
	upSource(w, "/work/my src", map[string]string{"1.1.0": grantsNew})
	if r := refusal(empty, "/work/my src", "standin"); r.Hint() != "gragas plugin add '/work/my src/standin/1.1.0'" {
		t.Errorf("not installed, a source with a space: next %q", r.Hint())
	}

	newer := w.Seed("/work/newer", map[string]string{"keep": "k"})
	installUp(t, w, newer, "/work/src", "1.1.0")
	if r := refusal(newer, "/work/old", "standin"); r.Word() != model.Refused || r.Hint() != "gragas plugin list" || !strings.Contains(r.Observed(), "name the version") {
		t.Errorf("older: %q, %q, next %q", r.Word(), r.Observed(), r.Hint())
	}
	c, r, err := coreapi.PluginUpgradeFrom(ctx, w.Ports(), "/work/old", "standin@1.0.0", upAsk(newer, "gragas plugin upgrade", "up-o"))
	if err != nil || r != nil {
		t.Fatalf("older, named: %v, %v; want a plan", r, err)
	}
	if plan := upPlanText(t, c); !strings.Contains(plan, "standin@1.0.0 is older than 1.1.0, as named") {
		t.Errorf("the older plan does not say so:\n%s", plan)
	}

	first := w.Seed("/work/first", map[string]string{"keep": "k"})
	installUp(t, w, first, "/work/src", "1.0.0")
	changed := upBundle("1.0.0", grantsNew)
	tampered := w.Seed("/work/tampered", map[string]string{
		pluginbundle.PluginLockPath:         read(t, w, first, pluginbundle.PluginLockPath),
		"plugins/standin/1.0.0/plugin.wasm": changed["plugin.wasm"],
		"plugins/standin/1.0.0/plugin.yaml": changed["plugin.yaml"],
	})
	if r := refusal(tampered, "/work/src", "standin"); r.Word() != model.Refused || r.Hint() != "gragas plugin list" || !strings.Contains(r.Observed(), "not what the lock pins") || strings.Contains(r.Observed(), "grant standin") {
		t.Errorf("not pinned: %q, %q, next %q", r.Word(), r.Observed(), r.Hint())
	}

	for _, bad := range []string{"Standin", "standin@1.0", "../standin", "standin@"} {
		if r := refusal(first, "/work/src", bad); r.Word() != model.Usage {
			t.Errorf("%q: %q, want usage", bad, r.Word())
		}
	}
}

// upBundleWith is upBundle with more top-level lines of the manifest, put
// before its programs.
func upBundleWith(version, grants, more string) map[string]string {
	b := upBundle(version, grants)
	b["plugin.yaml"] = strings.Replace(b["plugin.yaml"], "programs:\n", more+"programs:\n", 1)
	return b
}

// TestPluginUpgradeRefusesAVersionThatIsNotSemver: a version semver has no
// order for, such as 02.0.0, is refused by name with the next line plugin
// list, whether the source's folders hold it or it is named, and a lock that
// holds it is not read, its leading zero named. The lock at 02.0.0 and the source at 1.0.0, with no version named,
// is the row Fable found: unrefused, 02.0.0 ranks below 1.0.0 and the step
// back is planned as an upgrade. Planted: the lock side's check dropped
// (planned 02.0.0 to 1.0.0), the source side's dropped (1.0.0 taken over
// 02.0.0).
func TestPluginUpgradeRefusesAVersionThatIsNotSemver(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	ctx := context.Background()
	refused := func(root model.Root, source, ref, names string) {
		t.Helper()
		c, r, err := coreapi.PluginUpgradeFrom(ctx, w.Ports(), source, ref, upAsk(root, "gragas plugin upgrade", "up-s"))
		if err != nil || r == nil || c != nil {
			t.Fatalf("upgrade %s %s = change %v, refusal %v, %v; want a refusal", source, ref, c != nil, r, err)
		}
		if r.Word() != model.Refused || r.Hint() != "gragas plugin list" || !strings.Contains(r.Observed(), "not a semver version") || !strings.Contains(r.Observed(), names) {
			t.Errorf("upgrade %s %s: %q, %q, next %q; want refused naming %s as not semver", source, ref, r.Word(), r.Observed(), r.Hint(), names)
		}
	}

	upSource(w, "/work/src", map[string]string{"1.0.0": grantsOld})
	// the lock at 02.0.0 pins what its folder holds, so only the version is
	// wrong with it
	zeroed := map[string]string{}
	var held []model.File
	for p, data := range upBundle("02.0.0", grantsOld) {
		zeroed["plugins/standin/02.0.0/"+p] = data
		held = append(held, model.File{Path: p, Data: []byte(data)})
	}
	zeroed[pluginbundle.PluginLockPath] = string(pluginbundle.EncodePluginLock([]pluginbundle.LockEntry{{
		Ref: model.PluginRef{Name: "standin", Version: "02.0.0"}, Source: coreapi.LocalSource, Digest: pluginbundle.Digest(held),
	}}))
	// a lock that holds one is not read at all: its leading zero is named,
	// with the fix, before the upgrade orders anything
	if c, r, err := coreapi.PluginUpgradeFrom(ctx, w.Ports(), "/work/src", "standin", upAsk(w.Seed("/work/zeroed", zeroed), "gragas plugin upgrade", "up-s")); c != nil || r != nil || err == nil || !strings.Contains(err.Error(), `"02.0.0" has a leading zero`) {
		t.Errorf("upgrade over a lock at 02.0.0 = change %v, refusal %v, %v; want the lock refused for its leading zero", c != nil, r, err)
	}

	upSource(w, "/work/both", map[string]string{"1.0.0": grantsOld, "02.0.0": grantsOld})
	installed := w.Seed("/work/repo", map[string]string{"keep": "k"})
	installUp(t, w, installed, "/work/src", "1.0.0")
	refused(installed, "/work/both", "standin", "standin/02.0.0")
	refused(installed, "/work/both", "standin@02.0.0", "standin@02.0.0")
}

// TestPluginUpgradeNamesWhatMovedInAGrantAndPastTheBundle: a grant that kept
// its id says what moved in it, each as it is and as it was, before the
// command that asks its version; resolve_from, the install words and the wiring import are each a line
// of their own when they moved. Planted: the fields dropped (only the version
// command said), the link lines dropped.
func TestPluginUpgradeNamesWhatMovedInAGrantAndPastTheBundle(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	moved := "  - id: standin\n    path: bin/other\n    version_argv: [--version]\n    versions: \">=1.1.0 <2.0.0\"\n"
	newer := upBundleWith("1.1.0", moved, "resolve_from: lib\nwiring:\n  import:\n    at: /extends\n    form: list\n  install: [go, install, standin]\n")
	files := map[string]string{}
	for p, data := range upBundle("1.0.0", grantsOld) {
		files["standin/1.0.0/"+p] = data
	}
	for p, data := range newer {
		files["standin/1.1.0/"+p] = data
	}
	w.Seed("/work/src", files)
	w.Seed("/work/src/standin/1.0.0", upBundle("1.0.0", grantsOld))
	w.Seed("/work/src/standin/1.1.0", newer)
	root := w.Seed("/work/repo", map[string]string{"keep": "k"})
	installUp(t, w, root, "/work/src", "1.0.0")
	c, ref, err := coreapi.PluginUpgradeFrom(context.Background(), w.Ports(), "/work/src", "standin", upAsk(root, "gragas plugin upgrade", "up-1"))
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	plan := upPlanText(t, c)
	for _, want := range []string{
		"grant standin changed: path bin/other (was bin/standin); versions >=1.1.0 <2.0.0 (was >=1.0.0 <2.0.0); its version is asked with --version\n",
		"resolve_from changed: lib (was none)",
		"the install words changed: go install standin (was none)",
		"the wiring import changed: /extends as list (was none)",
	} {
		if !strings.Contains(plan+"\n", want) {
			t.Errorf("the plan does not say %q:\n%s", want, plan)
		}
	}
}

// TestPluginUpgradeTakesOnlyAVersionFolderThatHoldsAFile: a file named as a
// version beside the version folders is no version, so the newest is the
// newest folder that holds a file. Planted: the file counted (9.9.9 taken, and
// its read fails).
func TestPluginUpgradeTakesOnlyAVersionFolderThatHoldsAFile(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	upSource(w, "/work/src", map[string]string{"1.0.0": grantsOld, "1.1.0": grantsOld})
	files := map[string]string{"standin/9.9.9": "a file, not a folder\n"}
	for _, v := range []string{"1.0.0", "1.1.0"} {
		for p, data := range upBundle(v, grantsOld) {
			files["standin/"+v+"/"+p] = data
		}
	}
	w.Seed("/work/src", files)
	root := w.Seed("/work/repo", map[string]string{"keep": "k"})
	installUp(t, w, root, "/work/src", "1.0.0")
	c, ref, err := coreapi.PluginUpgradeFrom(context.Background(), w.Ports(), "/work/src", "standin", upAsk(root, "gragas plugin upgrade", "up-1"))
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if plan := upPlanText(t, c); !strings.Contains(plan, "to standin@1.1.0: sha256:") {
		t.Errorf("the plan is not to 1.1.0:\n%s", plan)
	}
}

// TestPluginUpgradeGuardsTheOldFolderTheSecondPlanAndEveryOldFile: an old
// version whose folder is gone is not what the lock pins; the plan made again
// under the lock refuses an older version not named, as the first does, when
// the lock moved between the two, and ends failed on it; and an old bundle with files beyond its
// manifest and module has every one of them removed. Planted: an old bundle
// with no files left unjudged (planned); the second plan told the version was
// named (refused only for its scope); only the manifest and the module removed.
func TestPluginUpgradeGuardsTheOldFolderTheSecondPlanAndEveryOldFile(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC))
	ctx := context.Background()
	upSource(w, "/work/src", map[string]string{"1.0.0": grantsOld, "1.1.0": grantsOld, "1.2.0": grantsOld})

	first := w.Seed("/work/first", map[string]string{"keep": "k"})
	installUp(t, w, first, "/work/src", "1.0.0")
	gone := w.Seed("/work/gone", map[string]string{pluginbundle.PluginLockPath: read(t, w, first, pluginbundle.PluginLockPath)})
	c, r, err := coreapi.PluginUpgradeFrom(ctx, w.Ports(), "/work/src", "standin@1.1.0", upAsk(gone, "gragas plugin upgrade", "up-g"))
	if err != nil || r == nil || c != nil || !strings.Contains(r.Observed(), "not what the lock pins") || r.Hint() != "gragas plugin list" {
		t.Errorf("the old folder gone = change %v, refusal %v, %v; want not what the lock pins", c != nil, r, err)
	}

	upSource(w, "/work/upto", map[string]string{"1.0.0": grantsOld, "1.1.0": grantsOld})
	moving := w.Seed("/work/moving", map[string]string{"keep": "k"})
	installUp(t, w, moving, "/work/upto", "1.0.0")
	c, r, err = coreapi.PluginUpgradeFrom(ctx, w.Ports(), "/work/upto", "standin", upAsk(moving, "gragas plugin upgrade", "up-m"))
	if err != nil || r != nil {
		t.Fatal(r, err)
	}
	// between the first plan and the one made under the lock, 1.2.0 is
	// installed beside 1.0.0, so 1.1.0 is a step back from the newest
	installUp(t, w, moving, "/work/src", "1.2.0")
	// a plan that cannot be made under the lock ends failed, its cause the
	// first plan's refusal (apply's planFailed); told it was named, the plan
	// is made, and is refused only for files outside the first one's scope
	if out, ok := c.Preview(ctx); ok || out.Word != model.Failed || out.Cause == nil || !strings.Contains(out.Cause.Error(), "name the version to take an older one") {
		t.Errorf("the second plan = %q, ok %v, cause %v, refusal %v; want it to end on the older version not named", out.Word, ok, out.Cause, out.Refusal)
	}

	extra := upBundle("1.0.0", grantsOld)
	extra["README.md"] = "read me\n"
	extra["docs/use.md"] = "use it\n"
	files := map[string]string{}
	for p, data := range extra {
		files["standin/1.0.0/"+p] = data
	}
	for p, data := range upBundle("1.1.0", grantsOld) {
		files["standin/1.1.0/"+p] = data
	}
	w.Seed("/work/more", files)
	w.Seed("/work/more/standin/1.0.0", extra)
	w.Seed("/work/more/standin/1.1.0", upBundle("1.1.0", grantsOld))
	more := w.Seed("/work/withmore", map[string]string{"keep": "k"})
	installUp(t, w, more, "/work/more", "1.0.0")
	if read(t, w, more, "plugins/standin/1.0.0/docs/use.md") == "" {
		t.Fatal("the old bundle's other files were not installed")
	}
	c, r, err = coreapi.PluginUpgradeFrom(ctx, w.Ports(), "/work/more", "standin", upAsk(more, "gragas plugin upgrade", "up-x"))
	if err != nil || r != nil {
		t.Fatal(r, err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	for p := range extra {
		if read(t, w, more, "plugins/standin/1.0.0/"+p) != "" {
			t.Errorf("plugins/standin/1.0.0/%s is still there", p)
		}
	}
}
