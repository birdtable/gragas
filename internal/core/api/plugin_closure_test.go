package coreapi_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	coreapi "github.com/birdtable/gragas/internal/core/api"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/model/plugin"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// A config's set (gragas#1173): the files the plan's read globs match are
// mirrored into the scratch folder, and only those; a manifest's resolve_from
// is linked into it, and a read with one runs at the sandboxed level only.

const scratchDir = "/cache/scratch/run-1"

// closureWorld is auditWorld with a project holding files (all tracked but
// those in untracked), a scratch folder the fake writer can write, the plan
// reading glob and then finishing, and the sandbox reaching level.
func closureWorld(t *testing.T, files map[string]string, untracked []string, glob string, level model.Level) (*porttest.World, coreapi.AuditPlugins, *porttest.StandinPlugin, *scriptedSandbox) {
	t.Helper()
	rounds := 0
	w, a, p := auditWorld(t, someEntry, func(plugin.PlanRequest) (plugin.PlanResponse, error) {
		rounds++
		if rounds == 1 {
			return plugin.PlanResponse{Steps: []plugin.Step{{Run: &plugin.Run{Program: "ast-grep", Argv: []string{}}}, {Read: &plugin.Read{Glob: glob}}}}, nil
		}
		return plugin.PlanResponse{Final: true}, nil
	})
	root := w.Seed("/repo", files)
	var tracked []string
	for f := range files {
		if !slices.Contains(untracked, f) {
			tracked = append(tracked, f)
		}
	}
	w.Track(root, tracked...)
	w.Seed(scratchDir, nil)
	w.SeedRead(glob, nil)
	sb := &scriptedSandbox{level: level}
	a.Sandbox, a.Tree, a.Writer = sb, w.Ports().Tree, w.Ports().Writer
	return w, a, p, sb
}

func readConfig(t *testing.T, a coreapi.AuditPlugins, name string) error {
	t.Helper()
	call, _, why := coreapi.PluginAuditEntries(a, nil)("ast-grep")
	if call == nil {
		t.Fatalf("no call: %q", why)
	}
	text := "{}\n"
	_, err := call(context.Background(), plugin.PlanRequest{Config: []plugin.Config{{Path: name, Data: &plugin.Bytes{Text: &text}}}, For: plugin.ForConfig, Target: plugin.TargetSource})
	return err
}

// TestOnlyTheFilesAReadGlobMatchesAreMirrored: a tsconfig that extends a
// sibling reaches it, the .env beside it, which no glob names, stays in the
// project, and so does a file the history does not hold.
// Planted: the whole folder mirrored (the .env and untracked.json lines red).
func TestOnlyTheFilesAReadGlobMatchesAreMirrored(t *testing.T) {
	t.Parallel()
	w, a, _, _ := closureWorld(t, map[string]string{
		"pkg/tsconfig.json":  "{}",
		"pkg/base.json":      `{"compilerOptions":{}}`,
		"pkg/untracked.json": "{}",
		"pkg/.env":           "SECRET=1",
		"base.json":          "outside the config's folder",
	}, []string{"pkg/untracked.json"}, "*.json", model.Isolated)
	if err := readConfig(t, a, "pkg/tsconfig.json"); err != nil {
		t.Fatal(err)
	}
	tree := w.Ports().Tree
	got, err := tree.Read(model.Root(scratchDir), "base.json")
	if err != nil || string(got) != `{"compilerOptions":{}}` {
		t.Errorf("base.json in scratch = %q, %v, want the sibling the glob matches, at the path the glob sees", got, err)
	}
	for _, name := range []string{".env", "untracked.json"} {
		if _, err := tree.Read(model.Root(scratchDir), name); err == nil {
			t.Errorf("%s was mirrored into the scratch folder, and no glob of the plan matches it as a tracked file", name)
		}
	}
}

// TestADotfileTheGlobDoesNotNameIsNotMirrored: a * glob takes a name that
// starts with a dot (the contract's reading), so a .env is mirrored only by a
// glob that matches it, and this one asks for yml files.
func TestADotfileTheGlobDoesNotNameIsNotMirrored(t *testing.T) {
	t.Parallel()
	w, a, _, _ := closureWorld(t, map[string]string{"c.json": "{}", ".env": "SECRET=1", "shared.yml": "a: 1"}, nil, "*.yml", model.Isolated)
	if err := readConfig(t, a, "c.json"); err != nil {
		t.Fatal(err)
	}
	tree := w.Ports().Tree
	if _, err := tree.Read(model.Root(scratchDir), "shared.yml"); err != nil {
		t.Errorf("shared.yml was not mirrored: %v", err)
	}
	if _, err := tree.Read(model.Root(scratchDir), ".env"); err == nil {
		t.Error(".env was mirrored by a glob that does not match it")
	}
}

// TestAMatchPastTheFileCapIsRefused: a file past plugin.MaxFile is refused
// whole, matching port.ErrRefused, and nothing of it is mirrored.
func TestAMatchPastTheFileCapIsRefused(t *testing.T) {
	t.Parallel()
	big := make([]byte, plugin.MaxFile+1)
	w, a, _, _ := closureWorld(t, map[string]string{"c.json": "{}", "big.yml": string(big)}, nil, "*.yml", model.Isolated)
	err := readConfig(t, a, "c.json")
	if !errors.Is(err, port.ErrRefused) {
		t.Fatalf("err = %v, want a refusal matching port.ErrRefused", err)
	}
	if _, rerr := w.Ports().Tree.Read(model.Root(scratchDir), "big.yml"); rerr == nil {
		t.Error("the file past the cap was mirrored")
	}
}

// TestAManifestsResolveFromIsLinkedAndForcesTheSandboxedLevel: a plugin whose
// manifest names resolve_from has that folder, below the config's own, linked
// into the scratch folder, and the read runs at the sandboxed level: it passes
// there and is refused below it, with the reason. A plugin without one keeps
// the floor of the name.
// Planted: the link made and the floor left isolated (the refusal row red).
func TestAManifestsResolveFromIsLinkedAndForcesTheSandboxedLevel(t *testing.T) {
	t.Parallel()
	files := map[string]string{"pkg/eslint.config.json": "{}"}
	rf := "node_modules"
	for _, c := range []struct {
		name    string
		resolve *string
		level   model.Level
		wantErr bool
		want    model.Level
	}{
		{"sandboxed", &rf, model.Sandboxed, false, model.Sandboxed},
		{"isolated", &rf, model.Isolated, true, model.Sandboxed},
		{"none", nil, model.Isolated, false, model.Isolated},
	} {
		_, a, p, sb := closureWorld(t, files, nil, "*.json", c.level)
		p.Facts.ResolveFrom = c.resolve
		a.Writer = nil // no mirror here: only the Prepare is under test
		err := readConfig(t, a, "pkg/eslint.config.json")
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, want an error: %v", c.name, err, c.wantErr)
		}
		if c.wantErr && !errors.Is(err, port.ErrStepNotReady) {
			t.Errorf("%s: err = %v, want the step refused as not ready below sandboxed", c.name, err)
		}
		if len(sb.prepared) != 1 {
			t.Fatalf("%s: %d scratch folders prepared, want 1", c.name, len(sb.prepared))
		}
		got := sb.prepared[0]
		if got.Level != c.want {
			t.Errorf("%s: scratch asked at %s, want %s", c.name, got.Level, c.want)
		}
		wantLink := model.Link{}
		if c.resolve != nil {
			wantLink = model.Link{Path: "node_modules", Dir: "/repo/pkg/node_modules"}
		}
		if got.Resolve != wantLink {
			t.Errorf("%s: resolve = %+v, want %+v", c.name, got.Resolve, wantLink)
		}
	}
}
