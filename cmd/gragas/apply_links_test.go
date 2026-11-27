package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	testenv "github.com/birdtable/go-testkit"

	"github.com/birdtable/gragas/internal/apply"
	"github.com/birdtable/gragas/internal/host/clock"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
)

// gitFolder is every entry under dir by path, a file as its bytes.
func gitFolder(t *testing.T, dir string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			got[p+"/"] = ""
			return nil
		}
		b, err := os.ReadFile(p)
		got[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestALinkInsideTheRootThatLeadsIntoDotGitIsRefusedByTheWritePath runs the
// write path over the real host/fs, with the two links a hostile checkout can
// carry: a folder the plan writes in that is a link to .git, and the history
// file that is a link to .git/config. Each ends refused, with nothing under
// .git changed and the link still a link. The control, the same plan over a
// checkout with no link, ends done, so a refusal is the link's and not the
// plan's.
func TestALinkInsideTheRootThatLeadsIntoDotGitIsRefusedByTheWritePath(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		links   map[string]string
		path    string
		appends bool
	}{
		{"a folder that is a link to .git", map[string]string{"state": ".git"}, "state/waive.yaml", false},
		{"the history file that is a link to .git/config", map[string]string{"history.jsonl": ".git/config"}, "history.jsonl", true},
		{"the control, no link", nil, "state/waive.yaml", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for p, data := range map[string]string{".git/config": "[core]\n\tbare = false\n", ".git/HEAD": "ref: refs/heads/main\n"} {
				if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, p), []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if len(tc.links) == 0 {
				if err := os.MkdirAll(filepath.Join(dir, "state"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for link, target := range tc.links {
				testenv.Link(t, filepath.FromSlash(target), filepath.Join(dir, link))
			}
			before := gitFolder(t, filepath.Join(dir, ".git"))

			tr := newTree(t)
			root, err := tr.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			w := hostfs.Writer{Tree: tr}
			ports := port.Ports{
				Tree: tr, FileNames: tr, Writer: w,
				Lock:    &hostfs.Lock{Dir: t.TempDir(), Clock: clock.Clock{}},
				Journal: &hostfs.Journal{Tree: tr, Writer: w, Clock: clock.Clock{}, Dir: t.TempDir()},
			}
			o := apply.Run(t.Context(), ports, root, apply.Spec{
				Command: "gragas test --apply", Engine: "v0.0.0-test", Run: "run-1", Scope: []string{"state/*", "history.jsonl"},
				Plan: func(context.Context) (model.Plan, error) {
					return model.Plan{Delta: model.Delta{Changes: []model.Change{{Entity: "file:" + tc.path}}}}, nil
				},
				Render: func(model.Delta) ([]model.Target, error) {
					return []model.Target{{Path: tc.path, Data: []byte("planted"), Append: tc.appends}}, nil
				},
			})

			if len(tc.links) == 0 {
				if o.Word != model.Done {
					t.Fatalf("the control ended %q (refusal %v, failure %v, cause %v), want done", o.Word, o.Refusal, o.Failure, o.Cause)
				}
				return
			}
			if o.Word != model.Refused || o.Refusal == nil {
				t.Fatalf("ended %q (failure %v, cause %v), want refused", o.Word, o.Failure, o.Cause)
			}
			if len(o.Changes) != 0 {
				t.Errorf("wrote %v", o.Changes)
			}
			if after := gitFolder(t, filepath.Join(dir, ".git")); len(after) != len(before) {
				t.Errorf(".git changed: %v, was %v", after, before)
			} else {
				for p, b := range before {
					if after[p] != b {
						t.Errorf("%s changed to %q, was %q", p, after[p], b)
					}
				}
			}
			for link := range tc.links {
				if fi, err := os.Lstat(filepath.Join(dir, link)); err != nil || fi.Mode().Type() != fs.ModeSymlink {
					t.Errorf("%s is no longer the link: %v, %v", link, fi, err)
				}
			}
		})
	}
}
