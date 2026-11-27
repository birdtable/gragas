package plugins_test

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/core/plugins"
	"github.com/birdtable/gragas/internal/digest"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/pluginbundle"
	"github.com/birdtable/gragas/internal/port"
)

func addModule() []byte { return []byte("\x00asm\x01\x00\x00\x00standin") }

func addManifest(wasm []byte) []byte {
	return []byte(`contract: 1
needs: []
name: standin
version: 1.0.0
tool: standin
tool_versions: ">=1.0.0 <2.0.0"
targets: [source]
provides: [entries]
extends: false
warning: false
programs:
  - id: standin
    path: bin/standin
    version_argv: [--version]
    versions: ">=1.0.0 <2.0.0"
wasm: ` + string(digest.Prefixed(wasm)) + "\n")
}

func addFiles() []model.File {
	return []model.File{
		{Path: "testdata/b", Data: []byte("b")},
		{Path: "plugin.wasm", Data: addModule()},
		{Path: "plugin.yaml", Data: addManifest(addModule())},
		{Path: "testdata/a", Data: []byte("a")},
	}
}

func addRef() model.PluginRef { return model.PluginRef{Name: "standin", Version: "1.0.0"} }

func addInput() plugins.AddInput {
	return plugins.AddInput{Ref: addRef(), Source: "file:standin", Files: addFiles(),
		Bounds: pluginbundle.Bounds{Module: 1024, File: 256, Total: 4096, Lock: 4096}}
}

// TestPlanAddWritesTheBundleFirstAndTheLockLast: the targets are the bundle's
// files by path and the lock after them all, so a run that stops before the
// lock leaves a folder no lock names. The lock reads back as the entry and
// the files pass the judge as locked. Planted: the lock first.
func TestPlanAddWritesTheBundleFirstAndTheLockLast(t *testing.T) {
	t.Parallel()
	plan, err := plugins.PlanAdd(addInput())
	if err != nil || plan.Nothing {
		t.Fatalf("PlanAdd = %+v, %v", plan, err)
	}
	var paths []string
	for _, tg := range plan.Targets {
		paths = append(paths, tg.Path)
	}
	want := "plugins/standin/1.0.0/plugin.wasm plugins/standin/1.0.0/plugin.yaml plugins/standin/1.0.0/testdata/a plugins/standin/1.0.0/testdata/b plugins/lock.yaml"
	if strings.Join(paths, " ") != want {
		t.Fatalf("targets = %v, want %s", paths, want)
	}
	entries, err := pluginbundle.DecodePluginLock(plan.Targets[len(plan.Targets)-1].Data)
	if err != nil || len(entries) != 1 || entries[0].Ref != addRef() || entries[0].Digest != pluginbundle.Digest(addFiles()) || entries[0].Source != "file:standin" {
		t.Errorf("lock = %v, %v, want the one entry of the files' digest", entries, err)
	}
	if _, err := pluginbundle.Judge(addRef(), pluginbundle.BundleOf(addFiles())); err != nil {
		t.Errorf("the judge refuses the bundle PlanAdd planned: %v", err)
	}
}

// TestPlanAddOfTheSameBundleIsNothingToDo and of other bytes under a locked
// ref is refused, never replaced.
func TestPlanAddOfALockedRefIsNothingOrRefused(t *testing.T) {
	t.Parallel()
	in := addInput()
	first, _ := plugins.PlanAdd(in)
	in.LockFile = first.Targets[len(first.Targets)-1].Data
	in.LockDigest = model.Digest("sha256:" + strings.Repeat("a", 64))
	for _, f := range addFiles() {
		in.Present = append(in.Present, "plugins/standin/1.0.0/"+f.Path)
	}
	if plan, err := plugins.PlanAdd(in); err != nil || !plan.Nothing || len(plan.Targets) != 0 {
		t.Errorf("a second PlanAdd = %+v, %v, want nothing to do", plan, err)
	}
	in.Files = append(addFiles(), model.File{Path: "testdata/c", Data: []byte("c")})
	if _, err := plugins.PlanAdd(in); !errors.Is(err, fs.ErrExist) {
		t.Errorf("PlanAdd of other bytes = %v, want fs.ErrExist", err)
	}
}

// TestPlanAddRefusesWhatReadWouldRefuse: each file over its own bound by one
// byte, and the files together over the total, are refused before any write.
// Planted: the Holds call gone (the module row goes red).
func TestPlanAddRefusesWhatReadWouldRefuse(t *testing.T) {
	t.Parallel()
	rows := map[string]func(*plugins.AddInput){
		"a module one byte over its bound": func(in *plugins.AddInput) { in.Bounds.Module = int64(len(addModule())) - 1 },
		"a file one byte over its bound":   func(in *plugins.AddInput) { in.Bounds.File = 0 },
		"the bundle one byte over the total": func(in *plugins.AddInput) {
			var n int64
			for _, f := range in.Files {
				n += int64(len(f.Data))
			}
			in.Bounds.Total = n - 1
		},
	}
	for name, plant := range rows {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := addInput()
			plant(&in)
			if plan, err := plugins.PlanAdd(in); !errors.Is(err, pluginbundle.ErrOverBound) || len(plan.Targets) != 0 {
				t.Errorf("PlanAdd = %+v, %v, want pluginbundle.ErrOverBound and no targets", plan, err)
			}
		})
	}
}

// TestPlanAddRefusesAFolderThatIsThereAndNotLocked: a stray file under the
// version's folder with no lock entry is refused, naming the folder, as is
// whatever the judge refuses and a source or lock the lock cannot hold.
func TestPlanAddRefusesAFolderThatIsThereAndNotLocked(t *testing.T) {
	t.Parallel()
	in := addInput()
	in.Present = []string{"plugins/standin/1.0.0/testdata/c"}
	_, err := plugins.PlanAdd(in)
	if !errors.Is(err, fs.ErrExist) || !strings.Contains(err.Error(), "plugins/standin/1.0.0") {
		t.Errorf("PlanAdd over a stray folder = %v, want fs.ErrExist naming the folder", err)
	}
}

func TestPlanAddRefusesABundleTheJudgeRefusesAndALockItCannotWrite(t *testing.T) {
	t.Parallel()
	in := addInput()
	in.Files = []model.File{{Path: "plugin.wasm", Data: []byte("other")}, {Path: "plugin.yaml", Data: addManifest(addModule())}}
	if _, err := plugins.PlanAdd(in); !errors.Is(err, port.ErrDigest) {
		t.Errorf("PlanAdd of a module the manifest does not name = %v, want port.ErrDigest", err)
	}
	in = addInput()
	in.Source = "file:\nstandin"
	if _, err := plugins.PlanAdd(in); err == nil {
		t.Error("PlanAdd of a source the lock cannot hold was planned")
	}
	in = addInput()
	in.Bounds.Lock = 64
	if _, err := plugins.PlanAdd(in); !errors.Is(err, pluginbundle.ErrOverBound) {
		t.Errorf("PlanAdd over the lock's bound = %v, want pluginbundle.ErrOverBound", err)
	}
}

// TestPlanAddOfALockedRefIsNothingOnlyWhereTheFolderHoldsTheBundle: the ref
// locked at these files' digest with the folder gone, or with a file missing
// or a stray one, is refused naming the folder, never nothing to do (nothing
// ignores the folder). Planted: the folder check gone (each row is nothing).
func TestPlanAddOfALockedRefIsNothingOnlyWhereTheFolderHoldsTheBundle(t *testing.T) {
	t.Parallel()
	first, _ := plugins.PlanAdd(addInput())
	locked := first.Targets[len(first.Targets)-1].Data
	var all []string
	for _, f := range addFiles() {
		all = append(all, "plugins/standin/1.0.0/"+f.Path)
	}
	for name, present := range map[string][]string{
		"the folder gone": nil,
		"a file missing":  all[1:],
		"a stray file":    append(slices.Clone(all), "plugins/standin/1.0.0/mine"),
	} {
		in := addInput()
		in.LockFile, in.LockDigest, in.Present = locked, model.Digest("sha256:"+strings.Repeat("a", 64)), present
		_, err := plugins.PlanAdd(in)
		if !errors.Is(err, fs.ErrExist) || !strings.Contains(err.Error(), "plugins/standin/1.0.0") {
			t.Errorf("%s: PlanAdd = %v, want fs.ErrExist naming the folder", name, err)
		}
	}
}
