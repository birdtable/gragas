package main

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/birdtable/gragas/internal/apply"
	hostfs "github.com/birdtable/gragas/internal/host/fs"
	"github.com/birdtable/gragas/internal/model"
	"github.com/birdtable/gragas/internal/port"
	porttest "github.com/birdtable/gragas/internal/port/testing"
)

// cancelling cancels its context once left writes have landed: a SIGINT or
// a SIGTERM arriving mid-write, as signal.NotifyContext delivers one.
type cancelling struct {
	port.Writer
	left   *int
	cancel context.CancelFunc
}

func (c cancelling) Replace(r model.Root, p string, data []byte) error {
	defer c.tick()
	return c.Writer.Replace(r, p, data)
}

func (c cancelling) Append(r model.Root, p string, line []byte) error {
	defer c.tick()
	return c.Writer.Append(r, p, line)
}

func (c cancelling) tick() {
	if *c.left--; *c.left == 0 {
		c.cancel()
	}
}

// onDisk is every file below dir with its bytes and mode.
func onDisk(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = info.Mode().String() + " " + string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestACancelledApplyOverHostFsRollsEveryFileBackByteForByte: the write path
// over the host's Tree, Writer, Lock and Journal, in a temporary folder, is
// cancelled once a is replaced and state/new.yaml made. Every file of the
// root then holds its old bytes and mode, the new file is gone, the run ends
// failed, kind cancelled, rolled back, and the journal holds no open record.
// The control is the same run uncancelled, which ends done. Risk: a roll-back
// that holds on the fake and not on a disk (go.concurrency.4, cli.commands.7).
func TestACancelledApplyOverHostFsRollsEveryFileBackByteForByte(t *testing.T) {
	t.Parallel()
	run := func(t *testing.T, cancelAt int) (apply.Outcome, port.Ports, model.Root, map[string]string, string) {
		t.Helper()
		base := t.TempDir()
		dir := filepath.Join(base, "root")
		files := map[string]string{"rules/a.yaml": "status: active\n", "rules/b.yaml": "status: active\n", "history.jsonl": "{\"n\":1}\n"}
		for p, c := range files {
			full := filepath.Join(dir, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(c), 0o640); err != nil {
				t.Fatal(err)
			}
		}
		w := porttest.NewWorld(t)
		w.Instant(time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
		clk := w.Ports().Clock
		tree := newTree(t)
		state := filepath.Join(base, "state")
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		left := cancelAt
		p := port.Ports{
			Tree: tree, FileNames: tree,
			Writer:  cancelling{Writer: hostfs.Writer{Tree: tree}, left: &left, cancel: cancel},
			Lock:    &hostfs.Lock{Dir: state, Clock: clk},
			Journal: hostfs.Journal{Tree: tree, Writer: hostfs.Writer{Tree: tree}, Clock: clk, Dir: state},
		}
		root, err := tree.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		before := onDisk(t, dir)
		digest := func(p string) model.Digest {
			b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
			if errors.Is(err, fs.ErrNotExist) {
				return ""
			}
			if err != nil {
				t.Fatal(err)
			}
			return apply.DigestBytes(b)
		}
		o := apply.Run(ctx, p, root, apply.Spec{
			Command: "gragas test --apply", Engine: "v0.0.0-test", Run: "run-1",
			Scope: []string{"rules/*.yaml", "state/new/*.yaml", "history.jsonl"},
			Plan: func(context.Context) (model.Plan, error) {
				return model.Plan{
					Delta: model.Delta{Changes: []model.Change{{Entity: "rule:a", Field: "status", Before: "active", After: "retired"}}},
					Yes:   &model.Yes{Who: "A Person <a@example.test>", At: 1794132000, Command: "gragas test --apply"},
				}, nil
			},
			Render: func(model.Delta) ([]model.Target, error) {
				return []model.Target{
					{Path: "rules/a.yaml", Before: digest("rules/a.yaml"), Data: []byte("status: retired\n")},
					{Path: "state/new/n.yaml", Data: []byte("made: yes\n")},
					{Path: "rules/b.yaml", Before: digest("rules/b.yaml"), Data: []byte("status: trial\n")},
				}, nil
			},
			Record: func(y model.Yes) (model.Target, error) {
				return model.Target{Path: "history.jsonl", Before: digest("history.jsonl"), Data: []byte(`{"plan":"` + string(y.Plan) + `"}`), Append: true}, nil
			},
		})
		return o, p, root, before, dir
	}

	o, p, root, before, dir := run(t, 2)

	if o.Word != model.Failed || o.Failure == nil || o.Failure.Kind() != apply.FailCancelled || !o.RolledBack {
		t.Fatalf("ended %q %+v, rolled back %v, cause %v; want failed, cancelled, rolled back", o.Word, o.Failure, o.RolledBack, o.Cause)
	}
	if after := onDisk(t, dir); !maps.Equal(after, before) {
		t.Errorf("the root after\n %q\nwant it byte for byte as before\n %q", after, before)
	}
	if open, err := apply.Open(t.Context(), p, root); err != nil || len(open) != 0 {
		t.Errorf("open records %+v, %v; want none", open, err)
	}

	t.Run("the control, uncancelled, ends done", func(t *testing.T) {
		t.Parallel()
		o, _, _, before, dir := run(t, 0)
		if o.Word != model.Done {
			t.Fatalf("ended %q %+v, cause %v", o.Word, o.Failure, o.Cause)
		}
		if after := onDisk(t, dir); maps.Equal(after, before) || after["state/new/n.yaml"] == "" {
			t.Errorf("the control wrote nothing: %q", after)
		}
	})
}
