package coreapi_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

func addAsk(root model.Root, run string) coreapi.PluginAddAsk {
	mod := []byte("\x00asm\x01\x00\x00\x00standin")
	man := []byte("contract: 1\nneeds: []\nname: standin\nversion: 1.0.0\ntool: standin\ntool_versions: \">=1.0.0 <2.0.0\"\ntargets: [source]\nprovides: [entries]\nextends: false\nwarning: false\nprograms:\n  - id: standin\n    path: bin/standin\n    version_argv: [--version]\n    versions: \">=1.0.0 <2.0.0\"\nwasm: " + string(digest.Prefixed(mod)) + "\n")
	a := changeAsk(root, "gragas plugin add")
	a.Run = run
	return coreapi.PluginAddAsk{
		Ask: a, Ref: model.PluginRef{Name: "standin", Version: "1.0.0"}, Source: "file:standin",
		Files:  []model.File{{Path: "plugin.wasm", Data: mod}, {Path: "plugin.yaml", Data: man}, {Path: "testdata/a", Data: []byte("a")}},
		Bounds: pluginbundle.Bounds{Module: 1024, File: 256, Total: 4096, Lock: 4096},
	}
}

const addFolder = "plugins/standin/1.0.0/"

// TestPluginAddWritesTheBundleAndTheLockAndASecondRunHasNothingToDo: through
// the one write path; the lock is there only with every file.
func TestPluginAddWritesTheBundleAndTheLockAndASecondRunHasNothingToDo(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{"keep": "k"})
	ctx := context.Background()
	c, ref, err := coreapi.PluginAdd(ctx, w.Ports(), addAsk(root, "run-1"))
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	for _, p := range []string{addFolder + "plugin.wasm", addFolder + "plugin.yaml", addFolder + "testdata/a", pluginbundle.PluginLockPath} {
		if read(t, w, root, p) == "" {
			t.Errorf("%s was not written", p)
		}
	}
	if !strings.Contains(read(t, w, root, pluginbundle.PluginLockPath), "file:standin") {
		t.Error("the lock does not name the source")
	}
	again, _, err := coreapi.PluginAdd(ctx, w.Ports(), addAsk(root, "run-2"))
	if err != nil {
		t.Fatal(err)
	}
	if out := again.Apply(ctx, ""); out.Word != model.Done || !out.Nothing {
		t.Errorf("the second run ended %q, nothing %v, want done with nothing", out.Word, out.Nothing)
	}
}

// TestPluginAddRefusesAFolderTheLockDoesNotNameBeforeAnyPlan: a stray file
// below the version's folder is refused, and it is still there.
func TestPluginAddRefusesAFolderTheLockDoesNotNameBeforeAnyPlan(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	root := w.Seed("repo", map[string]string{addFolder + "mine": "mine"})
	_, _, err := coreapi.PluginAdd(context.Background(), w.Ports(), addAsk(root, "run-1"))
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("PluginAdd over a stray folder = %v, want fs.ErrExist", err)
	}
	if read(t, w, root, addFolder+"mine") != "mine" || read(t, w, root, addFolder+"plugin.yaml") != "" {
		t.Error("a refused add changed the folder")
	}
}

// TestPluginAddRefusesALockOverItsBoundBeforeReadingIt: a lock of Lock+1
// bytes is refused with ErrOverBound by its size alone, and one of Lock bytes
// goes on to be decoded (and refused as no lock). Planted: the Stat gone.
func TestPluginAddRefusesALockOverItsBoundBeforeReadingIt(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	limit := addAsk("", "run-1").Bounds.Lock
	root := w.Seed("repo", map[string]string{pluginbundle.PluginLockPath: strings.Repeat("x", int(limit)+1)})
	ask := addAsk(root, "run-1")
	if _, _, err := coreapi.PluginAdd(context.Background(), w.Ports(), ask); !errors.Is(err, pluginbundle.ErrOverBound) {
		t.Fatalf("PluginAdd over a lock of Lock+1 bytes = %v, want ErrOverBound", err)
	}
}

// TestPluginAddOfALockedBundleWhoseFolderIsPartialIsRefused: the lock holds the
// bundle and the folder holds one file of three, so add does not say nothing to do.
func TestPluginAddOfALockedBundleWhoseFolderIsPartialIsRefused(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	ctx := context.Background()
	first := w.Seed("first", map[string]string{"keep": "k"})
	c, _, err := coreapi.PluginAdd(ctx, w.Ports(), addAsk(first, "run-1"))
	if err != nil {
		t.Fatal(err)
	}
	if out := c.Apply(ctx, ""); out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	// A second clone with the lock and one file of three: the folder is partial.
	root := w.Seed("second", map[string]string{
		pluginbundle.PluginLockPath: read(t, w, first, pluginbundle.PluginLockPath),
		addFolder + "plugin.wasm":   read(t, w, first, addFolder+"plugin.wasm"),
	})
	if _, _, err := coreapi.PluginAdd(ctx, w.Ports(), addAsk(root, "run-2")); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("PluginAdd with a partial folder = %v, want fs.ErrExist", err)
	}
}

// TestPluginAddFromReadsTheFolderAndNamesItsRefFromTheLayout: the bundle in
// <name>/<version> is added, the lock names the folder as its source, the
// plan says it approves nothing, and a file over its bound is refused unread.
func TestPluginAddFromReadsTheFolderAndNamesItsRefFromTheLayout(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	src := map[string]string{}
	for _, f := range addAsk("", "x").Files {
		src[f.Path] = string(f.Data)
	}
	w.Seed("/work/repo/vendor/standin/1.0.0", src)
	root := w.Seed("/work/repo", map[string]string{"keep": "k"})
	ctx := context.Background()
	c, ref, err := coreapi.PluginAddFrom(ctx, w.Ports(), "/work/repo/vendor/standin/1.0.0", addAsk(root, "run-1").Ask)
	if err != nil || ref != nil {
		t.Fatal(ref, err)
	}
	out := c.Apply(ctx, "")
	if out.Word != model.Done {
		t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
	}
	if lock := read(t, w, root, pluginbundle.PluginLockPath); !strings.Contains(lock, "file:vendor/standin/1.0.0") || read(t, w, root, addFolder+"testdata/a") != "a" {
		t.Errorf("the lock or the files are not as added: %q", lock)
	}
	w.Seed("/standin", src)
	if _, _, err := coreapi.PluginAddFrom(ctx, w.Ports(), "/standin", addAsk(root, "run-2").Ask); err == nil {
		t.Error("a folder that is not <name>/<version> was added")
	}
	big := map[string]string{"plugin.yaml": src["plugin.yaml"], "plugin.wasm": src["plugin.wasm"], "huge": strings.Repeat("x", 9<<20)}
	w.Seed("/work/big/standin/1.0.0", big)
	if _, _, err := coreapi.PluginAddFrom(ctx, w.Ports(), "/work/big/standin/1.0.0", addAsk(root, "run-3").Ask); !errors.Is(err, pluginbundle.ErrOverBound) || !strings.Contains(err.Error(), "not read") {
		t.Errorf("a file over its bound = %v, want ErrOverBound before the read", err)
	}
}

// bundleSeed is the files of a standin bundle as a folder holds them.
func bundleSeed() map[string]string {
	src := map[string]string{}
	for _, f := range addAsk("", "x").Files {
		src[f.Path] = string(f.Data)
	}
	return src
}

// addFromRefused: the add of the folder dir is refused with an error that
// says it was not read (or could not be read), and matches want when not nil.
func addFromRefused(t *testing.T, w *porttest.World, root model.Root, dir string, want error) error {
	t.Helper()
	_, _, err := coreapi.PluginAddFrom(context.Background(), w.Ports(), dir, addAsk(root, "run-x").Ask)
	if err == nil || (want != nil && !errors.Is(err, want)) {
		t.Fatalf("PluginAddFrom(%s) = %v, want a refusal matching %v", dir, err, want)
	}
	return err
}

// TestPluginAddFromRefusesABundleOverItsTotalBeforeReadingItAll: nine files
// that are each within their bound hold more than MaxBundle together, and the
// ninth is not read. Planted: the total check dropped, which leaves the judge
// to refuse it after every byte is in memory, in other words.
func TestPluginAddFromRefusesABundleOverItsTotalBeforeReadingItAll(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	src := bundleSeed()
	for i := range 9 {
		src[fmt.Sprintf("f%d", i)] = strings.Repeat("x", 8<<20)
	}
	w.Seed("/work/total/standin/1.0.0", src)
	root := w.Seed("/work/repo", nil)
	if err := addFromRefused(t, w, root, "/work/total/standin/1.0.0", pluginbundle.ErrOverBound); !strings.Contains(err.Error(), "not read further") {
		t.Errorf("a bundle over its total = %v, want it refused as not read further", err)
	}
}

// TestPluginAddFromRefusesALinkAndAFileWithASecondName: a link in the folder
// is not followed and a file with a second name is not read. Planted: the
// link check dropped (the link is read or listed as a file), ReadSoleWithin
// swapped for a read that counts no names.
func TestPluginAddFromRefusesALinkAndAFileWithASecondName(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	linked := w.Seed("/work/linked/standin/1.0.0", bundleSeed())
	w.Link(linked, "testdata/a", "../../../elsewhere")
	named := w.Seed("/work/named/standin/1.0.0", bundleSeed())
	w.Name(named, "testdata/a", "testdata/twin")
	root := w.Seed("/work/repo", nil)
	if err := addFromRefused(t, w, root, "/work/linked/standin/1.0.0", nil); !strings.Contains(err.Error(), "link") {
		t.Errorf("a link in the folder = %v, want it refused by the read, as a link", err)
	}
	ports := w.Ports()
	ports.Tree = marksLinks{Tree: ports.Tree, path: "testdata/a"}
	w.Seed("/work/marked/standin/1.0.0", bundleSeed())
	if _, _, err := coreapi.PluginAddFrom(context.Background(), ports, "/work/marked/standin/1.0.0", addAsk(root, "run-m").Ask); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("a listed link = %v, want it refused as not a regular file", err)
	}
	if err := addFromRefused(t, w, root, "/work/named/standin/1.0.0", port.ErrManyNames); err == nil {
		t.Error("a file with a second name was added")
	}
}

// guardedGrowth is the tree of a world whose files grow after they are
// listed: Stat and List answer the size they held, and ReadSoleWithin is
// handed more than that.
type grown struct {
	port.Tree
	path string
	more []byte
}

func (g grown) ReadSoleWithin(r model.Root, p string, max int64) ([]byte, error) {
	if p == g.path {
		if int64(len(g.more)) > max {
			return nil, fmt.Errorf("read %s: over the %d bytes allowed: %w", p, max, pluginbundle.ErrOverBound)
		}
		return g.more, nil
	}
	return g.Tree.ReadSoleWithin(r, p, max)
}

// TestPluginAddReadsTheLockAndEachFileAtTheirBoundAndNoMore: both reads are
// handed the bound they have, so a file that grew after its size was checked
// is refused by the read. Planted: the read handed a larger bound (here, the
// whole bundle's) answers the grown file and the add goes on.
func TestPluginAddReadsTheLockAndEachFileAtTheirBoundAndNoMore(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	src := bundleSeed()
	w.Seed("/work/src/standin/1.0.0", src)
	root := w.Seed("/work/repo", nil)
	ports := w.Ports()
	big := make([]byte, 9<<20)
	ports.Tree = grown{Tree: ports.Tree, path: "testdata/a", more: big}
	_, _, err := coreapi.PluginAddFrom(context.Background(), ports, "/work/src/standin/1.0.0", addAsk(root, "run-g").Ask)
	if !errors.Is(err, pluginbundle.ErrOverBound) || !strings.Contains(err.Error(), "bytes allowed") {
		t.Errorf("a file that grew past its bound = %v, want the read's own ErrOverBound", err)
	}
	ports.Tree = grown{Tree: w.Ports().Tree, path: pluginbundle.PluginLockPath, more: make([]byte, 2<<20)}
	if _, _, err := coreapi.PluginAdd(context.Background(), ports, addAsk(root, "run-l")); !errors.Is(err, pluginbundle.ErrOverBound) {
		t.Errorf("a lock that grew past its bound = %v, want ErrOverBound", err)
	}
}

// TestNoLockPluginAddWritesCarriesAnAbsolutePath: a folder in the project is
// recorded relative and in slash form, one outside it as local, and none of
// the locks written holds the folder the person typed. Planted: the folder
// recorded as typed.
func TestNoLockPluginAddWritesCarriesAnAbsolutePath(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, dir, want string }{
		{"inside", "/work/repo/vendor/standin/1.0.0", "file:vendor/standin/1.0.0"},
		{"outside", "/elsewhere/person/src/standin/1.0.0", coreapi.LocalSource},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := porttest.NewWorld(t)
			w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
			w.Seed(c.dir, bundleSeed())
			root := w.Seed("/work/repo", nil)
			ctx := context.Background()
			ch, ref, err := coreapi.PluginAddFrom(ctx, w.Ports(), c.dir, addAsk(root, "run-a").Ask)
			if err != nil || ref != nil {
				t.Fatal(ref, err)
			}
			if out := ch.Apply(ctx, ""); out.Word != model.Done {
				t.Fatalf("apply ended %q (%v)", out.Word, out.Cause)
			}
			lock := read(t, w, root, pluginbundle.PluginLockPath)
			if !strings.Contains(lock, c.want) {
				t.Errorf("the lock records %q, want %q", lock, c.want)
			}
			if strings.Contains(lock, "/work/") || strings.Contains(lock, "/elsewhere/") {
				t.Errorf("the lock carries an absolute path: %q", lock)
			}
		})
	}
}

// TestPluginAddsPlanSaysItApprovesNothing: the plan names plugin approve.
// Planted: the line dropped.
func TestPluginAddsPlanSaysItApprovesNothing(t *testing.T) {
	t.Parallel()
	w := porttest.NewWorld(t)
	root := w.Seed("/work/repo", nil)
	c, _, err := coreapi.PluginAdd(context.Background(), w.Ports(), addAsk(root, "run-p"))
	if err != nil {
		t.Fatal(err)
	}
	shown, ok := c.Preview(context.Background())
	if !ok {
		t.Fatalf("the preview ended %q, cause %v", shown.Word, shown.Cause)
	}
	var all strings.Builder
	for _, s := range shown.Plan.Sections {
		all.WriteString(strings.Join(s.Lines, "\n"))
	}
	if !strings.Contains(all.String(), "this approves nothing: gragas plugin approve") {
		t.Errorf("the plan does not say it approves nothing: %q", all.String())
	}
}

// marksLinks is a tree whose listing shows the file at path as a link, as a
// host that lists its links does: the check of the listing, not the read's
// refusal, is what it reaches.
type marksLinks struct {
	port.Tree
	path string
}

func (m marksLinks) List(r model.Root, dir string, s model.Scope) ([]model.Entry, error) {
	es, err := m.Tree.List(r, dir, s)
	for i := range es {
		if es[i].Path == m.path {
			es[i].Link = true
		}
	}
	return es, err
}
