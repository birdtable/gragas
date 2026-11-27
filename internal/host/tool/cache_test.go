package tool_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/birdtable/go-files/trust/trusttest"
	"github.com/birdtable/go-proc/proc"
	testenv "github.com/birdtable/go-testkit"

	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/host/tool"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
)

// cacheNames are the cache variables the grant of cached gives.
func cacheNames() []string { return []string{"LINT_CACHE", "LINT_MOD_CACHE"} }

// cached is the scope of a read of lint-a in root, its grant giving
// cacheNames, the plugin lint-a-plugin at version.
func cached(t *testing.T, root, version string) plugin.Scope {
	t.Helper()
	s := stepScope(t, root, false)
	s.Manifest.Name, s.Manifest.Version = "lint-a-plugin", version
	s.Manifest.Needs = []string{plugin.FeatureCacheVars}
	s.Manifest.Programs[0].Found.CacheVars = append([]string(nil), cacheNames()...)
	return s
}

// cacheTool is a Tool whose tools cache is cache, finding prog for each root.
func cacheTool(cache, prog string, roots ...string) *tool.Tool {
	paths := map[string]map[string]string{}
	for _, r := range roots {
		paths[r] = map[string]string{"lint-a@1.2.3": prog}
	}
	return tool.New(tool.Config{Paths: paths, Path: pathFolder(), Lang: "C", Cache: cache})
}

// cacheVars is the values of cacheNames one step of s on tl was given.
func cacheVars(t *testing.T, tl *tool.Tool, s plugin.Scope) []string {
	t.Helper()
	res, err := stepRunOf(t, bind(t, tl, s))
	if err != nil {
		t.Fatalf("the step: %v", err)
	}
	return strings.Split(text(t, res.Stdout), "|")
}

// TestAGrantsCacheVarsAreFoldersOfItsOwnKeyedByPluginVersionGrantAndProject:
// each name a grant gives by cache_vars reaches its step as a folder of its own
// below the tools cache, made with no access for group or others, the same for
// a later step of the same plugin, version, grant and project, so what one
// step keeps there the next finds; and another for a step of another project
// or another version of the plugin. The key moves with each of the four.
// Planted: cacheKey leaving the root out (the other project row red);
// cacheEnv making its folders with os.MkdirAll at 0o755 (the mode row red).
func TestAGrantsCacheVarsAreFoldersOfItsOwnKeyedByPluginVersionGrantAndProject(t *testing.T) {
	t.Parallel()
	cache := private(t) + "/tools"
	root, other := private(t), private(t)
	prog := program(t, private(t)+"/lint-a", answering(configtest.WriteUnder("LINT_CACHE", "marker", "x"), configtest.Vars(cacheNames()...)))
	tl := cacheTool(cache, prog, root, other)

	first := cacheVars(t, tl, cached(t, root, "1.0.0"))
	if len(first) != len(cacheNames()) || first[0] == first[1] {
		t.Fatalf("the cache variables = %q; want a folder of its own for each of %v", first, cacheNames())
	}
	for i, dir := range first {
		if rel, err := filepath.Rel(filepath.FromSlash(cache), dir); err != nil || rel == "." || !filepath.IsLocal(rel) || filepath.Base(dir) != cacheNames()[i] {
			t.Errorf("%s = %q; want a folder named for it below %s", cacheNames()[i], dir, cache)
		}
		for _, d := range []string{dir, filepath.Dir(dir)} {
			if fi, err := os.Lstat(d); err != nil || !fi.IsDir() {
				t.Errorf("%s: %v, %v; want a folder", d, fi, err)
			}
			trusttest.Perm(t, d, 0o700)
		}
	}
	if _, err := os.Stat(filepath.Join(first[0], "marker")); err != nil {
		t.Errorf("the step's write to its cache is not there: %v", err)
	}

	if again := cacheVars(t, tl, cached(t, root, "1.0.0")); strings.Join(again, "|") != strings.Join(first, "|") {
		t.Errorf("a later step of the same grant and project = %q; want the folders of the first, %q", again, first)
	}
	if elsewhere := cacheVars(t, tl, cached(t, other, "1.0.0")); filepath.Dir(elsewhere[0]) == filepath.Dir(first[0]) {
		t.Errorf("a step of another project shares the folder %s; want one of its own", elsewhere[0])
	}
	if newer := cacheVars(t, tl, cached(t, root, "1.1.0")); filepath.Dir(newer[0]) == filepath.Dir(first[0]) {
		t.Errorf("a step of another version of the plugin shares the folder %s; want one of its own", newer[0])
	}

	base, err := tool.CacheKey("lint-a-plugin", "1.0.0", "lint-a", root)
	if err != nil {
		t.Fatal(err)
	}
	if base != filepath.Base(filepath.Dir(first[0])) {
		t.Errorf("the key folder = %s; want the key %s", filepath.Dir(first[0]), base)
	}
	for name, k := range map[string][4]string{
		"another plugin":  {"lint-b-plugin", "1.0.0", "lint-a", root},
		"another version": {"lint-a-plugin", "1.0.1", "lint-a", root},
		"another grant":   {"lint-a-plugin", "1.0.0", "lint-b", root},
		"another project": {"lint-a-plugin", "1.0.0", "lint-a", other},
		"a shifted word":  {"lint-a-plugin1", ".0.0", "lint-a", root},
	} {
		if got, err := tool.CacheKey(k[0], k[1], k[2], k[3]); err != nil || got == base {
			t.Errorf("%s: the key %s, %v; want another than %s", name, got, err, base)
		}
	}
}

// homeCacheOf is the Home cache go-proc's HomeEnv gives the Home whose first
// variable (homeName) a step was told is home: the one a step whose Home cache
// is not kept is given. Where each lies in the Home is read from go-proc, so
// no test names its layout.
func homeCacheOf(t *testing.T, home string) string {
	t.Helper()
	base := proc.NeutralDir()
	values := map[string]string{}
	for _, kv := range proc.HomeEnv(base) {
		name, value, _ := strings.Cut(kv, "=")
		values[name] = value
	}
	relHome, err := filepath.Rel(base, values[homeName()])
	if err != nil {
		t.Fatal(err)
	}
	relCache, err := filepath.Rel(base, values[homeCacheName(t)])
	if err != nil {
		t.Fatal(err)
	}
	root := home
	for up := relHome; up != "."; up = filepath.Dir(up) {
		root = filepath.Dir(root)
	}
	return filepath.Join(root, relCache)
}

// TestAGrantsHomeCacheIsAFolderOfItsKeyKeptBetweenSteps: a step of a grant
// that gives cache_vars has its Home's cache (homeCacheName) pointed at tool.HomeCacheFolder below
// the grant's key folder, outside the step's Home, made with no access for
// group or others, so what one step keeps there the next finds.
// Planted: cacheFolders leaving the stepCache's home empty (the folder row
// red, the Home's cache inside the Home); the Home cache's folder made by
// os.MkdirAll at 0o755 (the mode row red).
func TestAGrantsHomeCacheIsAFolderOfItsKeyKeptBetweenSteps(t *testing.T) {
	t.Parallel()
	root := private(t)
	hc := homeCacheName(t)
	prog := program(t, private(t)+"/lint-a", answering(configtest.WriteUnder(hc, "marker", "x"), configtest.Vars(homeName(), hc, cacheNames()[0])))
	tl := cacheTool(private(t)+"/tools", prog, root)
	first := cacheVars(t, tl, cached(t, root, "1.0.0"))
	if len(first) != 3 {
		t.Fatalf("the step printed %q; want the Home, its cache and %s", first, cacheNames()[0])
	}
	home, kept := first[0], first[1]
	if want := filepath.Join(filepath.Dir(first[2]), tool.HomeCacheFolder); kept != want {
		t.Errorf("%s = %q; want the grant's own folder %s", hc, kept, want)
	}
	if fresh := homeCacheOf(t, home); kept == fresh {
		t.Errorf("%s = %q is the fresh one of the step's Home; want the grant's kept folder", hc, kept)
	}
	trusttest.Perm(t, kept, 0o700)
	if _, err := os.Stat(filepath.Join(kept, "marker")); err != nil {
		t.Errorf("the step's write to its Home cache is not there: %v", err)
	}
	if again := cacheVars(t, tl, cached(t, root, "1.0.0")); again[1] != kept {
		t.Errorf("a later step of the same grant has %s = %q; want the first's, %q", hc, again[1], kept)
	}
}

// TestAGrantWithNoCacheVarsOrNoToolsCacheMakesNothing: a grant that gives no
// cache variable, and a Tool with no tools cache, make no folder, and the step is given no
// such variable, and its Home's cache (homeCacheName) is the fresh one
// go-proc's HomeEnv gives with its fresh Home (homeCacheOf), as before
// cache_vars.
// Planted: cacheEnv going on with an empty Cache (the no-cache row red, a
// folder made relative to the step's folder or refused); cacheEnv keeping
// a Home cache for every grant when the config has a Cache (the no-variable
// row red: the tools cache made, and the Home's cache the grant's kept one).
func TestAGrantWithNoCacheVarsOrNoToolsCacheMakesNothing(t *testing.T) {
	t.Parallel()
	root := private(t)
	cache := private(t) + "/tools"
	prog := program(t, private(t)+"/lint-a", answering(configtest.Vars(append(cacheNames(), homeName(), homeCacheName(t))...)))
	plain := cached(t, root, "1.0.0")
	plain.Manifest.Needs, plain.Manifest.Programs[0].Found.CacheVars = nil, nil
	fresh := func(what string, got []string) {
		t.Helper()
		if len(got) != 4 || strings.Join(got[:2], "|") != "unset|unset" {
			t.Fatalf("%s: %q; want no cache variable given, then the Home and its cache", what, got)
		}
		if want := homeCacheOf(t, got[2]); got[3] != want {
			t.Errorf("%s: the Home's cache %q; want the fresh one of the step's Home, %q", what, got[3], want)
		}
	}
	fresh("a grant with no cache variables", cacheVars(t, cacheTool(cache, prog, root), plain))
	if _, err := os.Lstat(cache); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a grant with no cache variables made the tools cache: %v", err)
	}
	fresh("no tools cache", cacheVars(t, cacheTool("", prog, root), cached(t, root, "1.0.0")))
}

// TestAToolsCacheTheWalkDoesNotTrustIsRefusedBeforeTheStepStarts (a cache
// that steps share is a path from one step into the next): a tools cache inside the project, the project spelled through a
// link, below a folder others may write, a link others may have left, or no
// absolute clean path, refuses the step before it starts, and nothing is made
// there or through the link. A link this account left in a folder only it may
// write is followed, as host/trust's walk follows one on the way to the state
// folder (gate/shared.yaml, private-dir), and the folder made at its end.
// Planted: cacheEnv handing Judge no folders apart (the project rows red);
// cacheEnv making its folders with os.MkdirAll (the writable rows red).
func TestAToolsCacheTheWalkDoesNotTrustIsRefusedBeforeTheStepStarts(t *testing.T) {
	t.Parallel()
	root, outside := private(t), private(t)
	prog := program(t, private(t)+"/lint-a", answering(configtest.Vars(cacheNames()...)))
	for _, d := range []string{outside + "/real", outside + "/open", outside + "/elsewhere"} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	testenv.Link(t, outside+"/real", outside+"/link")
	testenv.Link(t, root, outside+"/rootlink")
	testenv.Link(t, outside+"/elsewhere", outside+"/open/left")
	trusttest.Loosen(t, outside+"/open", trusttest.OthersWrite)
	for name, cache := range map[string]string{
		"inside the project":         root + "/tools",
		"the project itself":         root,
		"the project through a link": outside + "/rootlink/tools",
		"below a writable":           outside + "/open/tools",
		"a link in a writable":       outside + "/open/left/tools",
		"relative":                   "relative/tools",
		"not clean":                  outside + "/real/../real/tools",
		"with a trailing slash":      outside + "/real/tools/",
		"a control character":        outside + "/to\nols",
	} {
		tl := cacheTool(cache, prog, root)
		_, err := stepRunOf(t, bind(t, tl, cached(t, root, "1.0.0")))
		if !errors.Is(err, port.ErrRefused) || len(tl.Ran()) != 1 {
			t.Errorf("%s (%s): %v, runs %d; want a refusal before the step starts", name, cache, err, len(tl.Ran()))
		}
	}
	for _, d := range []string{root, outside + "/real", outside + "/open", outside + "/elsewhere"} {
		if entries, err := os.ReadDir(d); err != nil || len(entries) > 1 || len(entries) == 1 && entries[0].Name() != "left" {
			t.Errorf("%s holds %v, %v; want nothing made there", d, entries, err)
		}
	}

	if got := cacheVars(t, cacheTool(outside+"/link/tools", prog, root), cached(t, root, "1.0.0")); !strings.HasPrefix(got[0], filepath.FromSlash(outside+"/link/tools/")) {
		t.Errorf("a link this account left in its own folder: %q; want the folder made at its end", got)
	}
}

// TestAnEarlierStepsLinkInTheCacheIsRefusedByTheNext: what one
// step leaves in its cache the next is handed, so a step that leaves the
// cache variable's own folder a link into the project, or into the scratch
// folder of a later step, has the next step refused before it starts, and
// nothing is written through it.
// Planted: cacheFolders making each variable's folder by os.MkdirAll and
// opening it by name, both of which follow the link (the link row red, the
// next step run and written through it).
func TestAnEarlierStepsLinkInTheCacheIsRefusedByTheNext(t *testing.T) {
	t.Parallel()
	root := private(t)
	cache := private(t) + "/tools"
	prog := program(t, private(t)+"/lint-a", answering(configtest.WriteUnder("LINT_CACHE", "marker", "x"), configtest.Vars(cacheNames()...)))
	tl := cacheTool(cache, prog, root)
	first := cacheVars(t, tl, cached(t, root, "1.0.0"))
	if err := os.RemoveAll(first[0]); err != nil {
		t.Fatal(err)
	}
	testenv.Link(t, root, first[0])
	runs := len(tl.Ran())
	_, err := stepRunOf(t, bind(t, tl, cached(t, root, "1.0.0")))
	if !errors.Is(err, port.ErrRefused) || len(tl.Ran()) != runs+1 {
		t.Errorf("a cache folder an earlier step left a link into the project: %v, runs %d; want a refusal before the step starts", err, len(tl.Ran())-runs)
	}
	if _, err := os.Lstat(root + "/marker"); err == nil {
		t.Error("the step wrote into the project through its cache")
	}
}

// TestALinkAnEarlierStepLeftInTheCacheIsRefusedWhereverItPoints: every
// step's program runs as
// this account, so a link in the cache that this account left may be an
// earlier step's. One left at the tools cache, at the key folder, at a
// variable's own folder or at the Home cache's folder, pointing at a private folder of this account's that
// is neither the project nor the scratch folder (another project's key
// folder, a folder below the home), has the next step refused before it
// starts, and nothing is written through it.
// Planted: cacheFolders making each variable's folder by host/trust's Private
// on its full name, which follows a link this account left in its own folder
// (the variable's folder, Home cache's folder and key folder rows red,
// written through the link); cacheTools opening the Cache by Private (the
// tools cache row red).
func TestALinkAnEarlierStepLeftInTheCacheIsRefusedWhereverItPoints(t *testing.T) {
	t.Parallel()
	prog := program(t, private(t)+"/lint-a", answering(configtest.WriteUnder("LINT_CACHE", "marker", "x"), configtest.Vars(cacheNames()...)))
	for name, at := range map[string]func(first string) string{
		"the variable's folder":   func(first string) string { return first },
		"the Home cache's folder": func(first string) string { return filepath.Join(filepath.Dir(first), tool.HomeCacheFolder) },
		"the key folder":          filepath.Dir,
		"the tools cache":         func(first string) string { return filepath.Dir(filepath.Dir(first)) },
	} {
		root, elsewhere := private(t), private(t)
		tl := cacheTool(private(t)+"/tools", prog, root)
		first := cacheVars(t, tl, cached(t, root, "1.0.0"))
		link := at(first[0])
		if err := os.RemoveAll(link); err != nil {
			t.Fatal(err)
		}
		testenv.Link(t, elsewhere, link)
		runs := len(tl.Ran())
		_, err := stepRunOf(t, bind(t, tl, cached(t, root, "1.0.0")))
		if !errors.Is(err, port.ErrRefused) || len(tl.Ran()) != runs+1 {
			t.Errorf("%s left a link to a folder of this account's outside the project: %v, runs %d; want a refusal before the step starts", name, err, len(tl.Ran())-runs)
		}
		if entries, err := os.ReadDir(elsewhere); err != nil || len(entries) != 0 {
			t.Errorf("%s: the link's folder holds %v, %v; want nothing written through it", name, entries, err)
		}
	}
}

// TestACacheFolderAnEarlierStepLeftOpenToOthersIsRefusedByTheNext: a make
// by handle keeps the mode of a folder that
// was there, so a key folder, a variable's folder or the Home cache's folder
// an earlier step left writable by every account, or the tools cache left so,
// has the next step refused before it starts.
// Planted: cacheHeld passing every folder (the variable's folder, Home
// cache's folder and key folder rows red, the next step run; the tools cache
// row stays green on the walk's judgement by name).
func TestACacheFolderAnEarlierStepLeftOpenToOthersIsRefusedByTheNext(t *testing.T) {
	t.Parallel()
	prog := program(t, private(t)+"/lint-a", answering(configtest.Vars(cacheNames()...)))
	for name, at := range map[string]func(first string) string{
		"the variable's folder":   func(first string) string { return first },
		"the Home cache's folder": func(first string) string { return filepath.Join(filepath.Dir(first), tool.HomeCacheFolder) },
		"the key folder":          filepath.Dir,
		"the tools cache":         func(first string) string { return filepath.Dir(filepath.Dir(first)) },
	} {
		root := private(t)
		tl := cacheTool(private(t)+"/tools", prog, root)
		first := cacheVars(t, tl, cached(t, root, "1.0.0"))
		trusttest.Loosen(t, at(first[0]), trusttest.OthersWrite)
		runs := len(tl.Ran())
		_, err := stepRunOf(t, bind(t, tl, cached(t, root, "1.0.0")))
		if !errors.Is(err, port.ErrRefused) || len(tl.Ran()) != runs+1 {
			t.Errorf("%s left writable by every account: %v, runs %d; want a refusal before the step starts", name, err, len(tl.Ran())-runs)
		}
	}
}
