package jwrite_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/birdtable/gragas/internal/apply/jwrite"
	configtest "github.com/birdtable/gragas/internal/config/testing"
	"github.com/birdtable/gragas/internal/digest"
)

func TestMain(m *testing.M) { os.Exit(configtest.Main(m)) }

type fake struct {
	log      []string
	recs     []jwrite.Record
	files    map[string][]byte
	keepErr  error
	writeErr error
	corrupt  bool
}

func (f *fake) Append(r jwrite.Record) error {
	f.log = append(f.log, "journal "+r.Kind)
	f.recs = append(f.recs, r)
	return nil
}

func (f *fake) Keep(run, path string, old []byte) error {
	f.log = append(f.log, "keep "+path)
	return f.keepErr
}

func (f *fake) Replace(path string, data []byte) error {
	f.log = append(f.log, "replace "+path)
	if f.writeErr != nil {
		return f.writeErr
	}
	f.files[path] = data
	if f.corrupt {
		f.files[path] = []byte("other")
	}
	return nil
}

func (f *fake) AppendLine(path string, line []byte) error {
	f.log = append(f.log, "append "+path)
	f.files[path] = jwrite.Expected(f.files[path], jwrite.File{Data: line, Append: true})
	return nil
}

func (f *fake) Remove(path string) error {
	f.log = append(f.log, "remove "+path)
	if f.writeErr != nil {
		return f.writeErr
	}
	if !f.corrupt {
		delete(f.files, path)
	}
	return nil
}

func (f *fake) Read(path string) ([]byte, error) {
	b, ok := f.files[path]
	if !ok {
		return nil, jwrite.ErrAbsent
	}
	return b, nil
}

// TestOneDeletesAFileAfterKeepingItAndJournallingTheStep: a delete keeps the
// old bytes and journals the copy and a step of op "delete" with no digest
// after, before the file is taken away, and a file still there afterwards ends
// at the verify stage. Risk: a delete with no copy kept, so a crash loses the
// file for good, or one that reports done with the file still there.
func TestOneDeletesAFileAfterKeepingItAndJournallingTheStep(t *testing.T) {
	t.Parallel()
	f := &fake{files: map[string][]byte{"a": []byte("old")}}
	run := jwrite.Run{ID: "r1", Command: "gragas x", Plan: "p", Engine: "v"}
	w, _, err := jwrite.One(f, f, run, jwrite.File{Path: "a", Delete: true}, jwrite.Seen{Data: []byte("old"), Exists: true, Digest: "d0"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(f.log, ","), "keep a,journal copy,journal step,remove a"; got != want {
		t.Fatalf("log %q, want %q", got, want)
	}
	if _, there := f.files["a"]; there || w.Before != "d0" || w.After != "" {
		t.Fatalf("written %+v, file there: %v", w, there)
	}
	if f.recs[1].Detail["op"] != "delete" || f.recs[1].Detail["after"] != "" {
		t.Fatalf("step record %+v", f.recs[1])
	}
	still := &fake{files: map[string][]byte{"a": []byte("old")}, corrupt: true}
	if _, stage, err := jwrite.One(still, still, run, jwrite.File{Path: "a", Delete: true}, jwrite.Seen{Data: []byte("old"), Exists: true, Digest: "d0"}); err == nil || stage != jwrite.StageVerify {
		t.Fatalf("a file still there: stage %q, err %v, want the verify stage", stage, err)
	}
}

func TestOneJournalsBeforeItWrites(t *testing.T) {
	t.Parallel()
	f := &fake{files: map[string][]byte{}}
	run := jwrite.Run{ID: "r1", Command: "gragas x", Plan: "p", Engine: "v"}
	w, _, err := jwrite.One(f, f, run, jwrite.File{Path: "a", Data: []byte("new")}, jwrite.Seen{Data: []byte("old"), Exists: true, Digest: "d0"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"keep a", "journal copy", "journal step", "replace a"}
	if len(f.log) != len(want) {
		t.Fatalf("log %v, want %v", f.log, want)
	}
	for i := range want {
		if f.log[i] != want[i] {
			t.Fatalf("log %v, want %v", f.log, want)
		}
	}
	if w.Before != "d0" || w.After != digest.Prefixed([]byte("new")) {
		t.Fatalf("written %+v", w)
	}
	if f.recs[1].Detail["op"] != "replace" || f.recs[1].Run != "r1" {
		t.Fatalf("step record %+v", f.recs[1])
	}
}

func TestOneStopsAtTheStage(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	for _, c := range []struct {
		name  string
		f     *fake
		stage jwrite.Stage
	}{
		{"keep", &fake{files: map[string][]byte{}, keepErr: boom}, jwrite.StageJournal},
		{"write", &fake{files: map[string][]byte{}, writeErr: boom}, jwrite.StageWrite},
		{"verify", &fake{files: map[string][]byte{}, corrupt: true}, jwrite.StageVerify},
	} {
		_, stage, err := jwrite.One(c.f, c.f, jwrite.Run{ID: "r"}, jwrite.File{Path: "a", Data: []byte("x")}, jwrite.Seen{})
		if err == nil || stage != c.stage {
			t.Errorf("%s: stage %q err %v, want %q", c.name, stage, err, c.stage)
		}
	}
	// a copy that cannot be kept stops the write: nothing else is done.
	f := &fake{files: map[string][]byte{}, keepErr: boom}
	_, _, _ = jwrite.One(f, f, jwrite.Run{ID: "r"}, jwrite.File{Path: "a"}, jwrite.Seen{})
	if len(f.log) != 1 {
		t.Errorf("after a failed keep: %v", f.log)
	}
}

func TestExpectedEndsATornLine(t *testing.T) {
	t.Parallel()
	got := jwrite.Expected([]byte("a\nb"), jwrite.File{Data: []byte("c"), Append: true})
	if string(got) != "a\nb\nc\n" {
		t.Fatalf("%q", got)
	}
	if string(jwrite.Expected([]byte("old"), jwrite.File{Data: []byte("new")})) != "new" {
		t.Fatal("replace")
	}
}
